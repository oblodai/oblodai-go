package oblodai

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The paths where a wrong decision costs money: a re-sent payout, a key the core does not honour,
// a proxy answer mistaken for the core's, a clock correction that wedges the client.

// html scripts an answer from something in front of the core: no error envelope, no JSON at all.
func html(status int, headers ...map[string]string) step {
	s := step{status: status, body: "<html>upstream error</html>",
		headers: map[string]string{"Content-Type": "text/html"}}
	if len(headers) > 0 {
		for k, v := range headers[0] {
			s.headers[k] = v
		}
	}
	return s
}

func TestCallerKeyIsRefusedOnRoutesTheCoreDoesNotDeduplicate(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{}))
	_, err := api.client().Payouts.Approve(context.Background(), &ApproveRequest{UUID: "p1"}, WithIdempotencyKey("k1"))
	if !IsConfig(err) || !IsCode(err, CodeIdempotencyUnsupported) {
		t.Fatalf("expected sdk.idempotency_unsupported, got %v", err)
	}
	if api.count() != 0 {
		t.Fatal("the call must be refused before anything is sent")
	}
}

func TestUnsafeWriteIsNotReSentAfterAProxyAnswer(t *testing.T) {
	// A 503 without an envelope came from a proxy: the core may have received the request and
	// approved the payout. Repeating it could approve twice.
	api := newFakeAPI(t, html(503), ok(map[string]any{}))
	_, err := api.client().Payouts.Approve(context.Background(), &ApproveRequest{UUID: "p1"})
	apiErr := mustError(t, err)
	if apiErr.HTTPStatus != 503 || !apiErr.Synthetic || !apiErr.Retryable {
		t.Fatalf("expected a synthetic retryable 503, got %+v", apiErr)
	}
	if api.count() != 1 {
		t.Fatalf("expected 1 attempt, saw %d", api.count())
	}
	if !strings.Contains(apiErr.Message, "proxy") {
		t.Fatalf("the message should name the proxy: %q", apiErr.Message)
	}
}

func TestReadRouteIsRetriedAfterAProxyAnswerAndHonoursRetryAfter(t *testing.T) {
	api := newFakeAPI(t,
		html(502),
		html(504, map[string]string{"Retry-After": "0"}),
		ok(map[string]any{"balance": map[string]any{"merchant": []any{}}}),
	)
	if _, err := api.client().Account.GetBalance(context.Background()); err != nil {
		t.Fatalf("Account.GetBalance: %v", err)
	}
	if api.count() != 3 {
		t.Fatalf("expected 3 attempts, saw %d", api.count())
	}

	capped := newFakeAPI(t, html(429, map[string]string{"Retry-After": "120"}))
	_, err := capped.client(WithRetry(RetryOptions{MaxRetries: 0})).Account.GetBalance(context.Background())
	apiErr := mustError(t, err)
	if apiErr.RetryAfter == nil || *apiErr.RetryAfter != 120 {
		t.Fatalf("the Retry-After header must reach the caller, got %v", apiErr.RetryAfter)
	}
}

func TestEnvelopeErrorIsRetriedEvenOnAnUnsafeWrite(t *testing.T) {
	// The core answered, so it did not perform the operation: retrying cannot duplicate anything.
	api := newFakeAPI(t,
		apiError(409, map[string]any{"code": "payout.funds_maturing", "retryable": true, "retry_after": 0}),
		ok(map[string]any{"uuid": "p"}),
	)
	if _, err := api.client().Payouts.Approve(context.Background(), &ApproveRequest{UUID: "p1"}); err != nil {
		t.Fatalf("Payouts.Approve: %v", err)
	}
	if api.count() != 2 {
		t.Fatalf("expected 2 attempts, saw %d", api.count())
	}
}

func TestListRequestsNothingUntilConsumedAndNeverCarriesACallerKey(t *testing.T) {
	api := newFakeAPI(t, apiError(404, map[string]any{"code": "payment.not_found", "retryable": false}))
	client := api.client()
	list := client.Payments.ListHistory(context.Background(), nil)
	if api.count() != 0 {
		t.Fatal("a list must not request anything before it is consumed")
	}
	if _, err := list.Page(); !IsCode(err, "payment.not_found") {
		t.Fatalf("expected payment.not_found, got %v", err)
	}
	if api.count() != 1 {
		t.Fatalf("expected 1 request, saw %d", api.count())
	}

	// A key on a list is refused, not dropped: one key per page would make the core replay page
	// one for ever, and a caller who passed one must not be left believing the re-send is keyed.
	keyed := newFakeAPI(t, emptyPage())
	_, err := keyed.client().Payouts.ListHistory(context.Background(), nil,
		WithIdempotencyKey("k")).Page()
	if !IsConfig(err) || !IsCode(err, CodeIdempotencyUnsupported) {
		t.Fatalf("expected sdk.idempotency_unsupported on a list, got %v", err)
	}
	if keyed.count() != 0 {
		t.Fatalf("a refused list must not reach the network, saw %d requests", keyed.count())
	}
}

func TestClockCorrectionIsOnlyAppliedToSignatureFailures(t *testing.T) {
	far := map[string]string{"Date": time.Now().Add(4000 * time.Second).UTC().Format(http.TimeFormat)}
	api := newFakeAPI(t, apiError(401, map[string]any{"code": "auth.ip_not_allowed", "retryable": false}, far))
	client := api.client(WithRetry(RetryOptions{MaxRetries: 0}))
	if _, err := client.Account.GetBalance(context.Background()); !IsCode(err, "auth.ip_not_allowed") {
		t.Fatalf("expected auth.ip_not_allowed, got %v", err)
	}
	if api.count() != 1 {
		t.Fatalf("a 401 that is not a signature failure must not be re-signed (saw %d attempts)", api.count())
	}
	if client.ClockOffset() != 0 {
		t.Fatalf("no offset should have been learned, got %s", client.ClockOffset())
	}
}

func TestClockCorrectionIsRevertedWhenItDoesNotHelp(t *testing.T) {
	// One broken Date header from a proxy must not wedge every later call.
	far := map[string]string{"Date": time.Now().Add(4000 * time.Second).UTC().Format(http.TimeFormat)}
	badSignature := map[string]any{"code": "merchant.bad_signature", "retryable": false}
	api := newFakeAPI(t,
		apiError(401, badSignature, far),
		apiError(401, badSignature, far),
		ok(map[string]any{"balance": map[string]any{"merchant": []any{}}}),
	)
	client := api.client(WithRetry(RetryOptions{MaxRetries: 0}))
	ctx := context.Background()
	if _, err := client.Account.GetBalance(ctx); !IsCode(err, CodeBadSignature) {
		t.Fatalf("expected merchant.bad_signature, got %v", err)
	}
	if _, err := client.Account.GetBalance(ctx); err != nil {
		t.Fatalf("the next call must sign with the local clock again: %v", err)
	}
	if client.ClockOffset() != 0 {
		t.Fatalf("the correction should have been reverted, got %s", client.ClockOffset())
	}
	ts := api.at(2).header.Get(HeaderTimestamp)
	if drift := time.Now().Unix() - parseInt(t, ts); drift > 5 || drift < -5 {
		t.Fatalf("the third attempt is %d s from the local clock", drift)
	}
}

func TestBaseURLPathPrefixIsKeptAndSigned(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"balance": map[string]any{"merchant": []any{}}}))
	client := api.client(WithBaseURL(api.server.URL + "/oblodai/"))
	if _, err := client.Account.GetBalance(context.Background()); err != nil {
		t.Fatalf("Account.GetBalance: %v", err)
	}
	req := api.last()
	if req.path != "/oblodai/v1/balance" {
		t.Fatalf("path = %q, want /oblodai/v1/balance", req.path)
	}
	ts := parseInt(t, req.header.Get(HeaderTimestamp))
	want := SignRequest("secret-1", SignInput{TS: ts, Method: "POST", RequestURI: "/oblodai/v1/balance", Body: []byte("{}")})
	if got := req.header.Get(HeaderSignature); got != want {
		t.Fatal("the signature must cover the prefixed path")
	}
}

func TestCallerHeadersCannotOverrideSignedOnes(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"balance": map[string]any{"merchant": []any{}}}))
	client := api.client(WithHeader(HeaderSignature, "zz"), WithHeader("X-Trace", "t1"))
	if _, err := client.Account.GetBalance(context.Background()); err != nil {
		t.Fatalf("Account.GetBalance: %v", err)
	}
	req := api.last()
	if !hexish(req.header.Get(HeaderSignature), 32) {
		t.Fatalf("the caller header overwrote the signature: %q", req.header.Get(HeaderSignature))
	}
	if got := req.header.Get("X-Trace"); got != "t1" {
		t.Fatalf("an unrelated caller header must survive, got %q", got)
	}
}

func TestPathParametersCannotRewriteTheURL(t *testing.T) {
	api := newFakeAPI(t)
	client := api.client()
	ctx := context.Background()
	for _, bad := range []string{"", ".", "..", "a/b"} {
		_, err := client.Checkout.Get(ctx, bad)
		if !IsCode(err, CodeBadPathParam) {
			t.Fatalf("path parameter %q was accepted (%v)", bad, err)
		}
	}
	if api.count() != 0 {
		t.Fatal("nothing must be sent for a rejected path parameter")
	}
}

func TestDocumentReportsSendTheIDAsUUID(t *testing.T) {
	api := newFakeAPI(t, step{status: 200, body: "%PDF-1.4", headers: map[string]string{"Content-Type": "application/pdf"}})
	document, err := api.client().Documents.GetBatch(context.Background(), &GetBatchDocumentParams{UUID: "b-1", Format: Ptr("csv")})
	if err != nil {
		t.Fatalf("Documents.GetBatch: %v", err)
	}
	if document.ContentType != "application/pdf" || string(document.Bytes) != "%PDF-1.4" {
		t.Fatalf("unexpected document: %+v", document)
	}
	if got := api.last().rawQuery; got != "format=csv&uuid=b-1" {
		t.Fatalf("query = %q, want the batch id as uuid", got)
	}
}

func TestCancellationDuringARetryPause(t *testing.T) {
	api := newFakeAPI(t,
		apiError(503, map[string]any{"code": "db.unavailable", "retryable": true, "retry_after": 2}),
		ok(map[string]any{}),
	)
	client := api.client(WithRetry(RetryOptions{MaxRetries: 2, MaxRetryAfter: 5 * time.Second}))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := client.Account.GetBalance(ctx)
	if !IsCode(err, CodeTransportAborted) {
		t.Fatalf("expected transport.aborted, got %v", err)
	}
}

func TestRetryStopsWhenTheBudgetWouldBeExceeded(t *testing.T) {
	api := newFakeAPI(t,
		apiError(503, map[string]any{"code": "db.unavailable", "retryable": true, "retry_after": 2}),
		ok(map[string]any{}),
	)
	client := api.client(WithCallBudget(100*time.Millisecond),
		WithRetry(RetryOptions{MaxRetries: 2, MaxRetryAfter: 5 * time.Second}))
	_, err := client.Account.GetBalance(context.Background())
	if !IsCode(err, CodeTransportDeadline) {
		t.Fatalf("expected transport.deadline, got %v", err)
	}
	if api.count() != 1 {
		t.Fatalf("expected 1 attempt, saw %d", api.count())
	}
}

func TestRedirectIsReportedWithItsTarget(t *testing.T) {
	api := newFakeAPI(t, step{status: 301, body: "", headers: map[string]string{"Location": "https://www.api.test/v1/balance"}})
	client := api.client(WithRetry(RetryOptions{MaxRetries: 0}))
	_, err := client.Account.GetBalance(context.Background())
	apiErr := mustError(t, err)
	if apiErr.HTTPStatus != 301 || !strings.Contains(apiErr.Message, "www.api.test") {
		t.Fatalf("a redirect must name its target instead of being followed: %+v", apiErr)
	}
}

func parseInt(t *testing.T, s string) int64 {
	t.Helper()
	var v int64
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("not an integer: %q", s)
		}
		v = v*10 + int64(c-'0')
	}
	return v
}
