package oblodai

import (
	"context"
	"sync"
	"testing"
	"time"
)

// The raw side of a call, a copy of the client with other options, and hooks on every attempt.

func TestWithRawResponseExposesStatusHeadersAndRequestID(t *testing.T) {
	api := newFakeAPI(t, step{status: 200, body: map[string]any{"state": 0, "result": map[string]any{"uuid": "p1"}},
		headers: map[string]string{"X-Request-ID": "srv-1", "X-Ratelimit-Remaining": "9"}})
	var raw *RawResponse
	payment, err := api.client().Payments.GetInfo(context.Background(), &LookupRequest{UUID: Ptr("p1")}, WithRawResponse(&raw))
	if err != nil || payment.UUID != "p1" {
		t.Fatalf("GetInfo: %v %+v", err, payment)
	}
	if raw == nil || raw.StatusCode != 200 || raw.RequestID != "srv-1" || raw.Header.Get("X-Ratelimit-Remaining") != "9" {
		t.Fatalf("raw = %+v", raw)
	}
	if len(raw.Body) == 0 {
		t.Fatal("the body must be there")
	}

	// Without a response X-Request-ID it is the id the call was sent with; an error status still
	// fills it and still returns the error.
	failing := newFakeAPI(t, apiError(404, map[string]any{"code": "payment.not_found", "retryable": false}))
	raw = nil
	_, err = failing.client().Payments.GetInfo(context.Background(), &LookupRequest{UUID: Ptr("p1")},
		WithRawResponse(&raw), WithRequestID("mine"))
	_ = requireCode(t, err, "payment.not_found")
	if raw == nil || raw.StatusCode != 404 || raw.RequestID != "mine" {
		t.Fatalf("raw = %+v", raw)
	}
}

func TestWithOptionsCopiesTheClientAndLeavesItAlone(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{}), ok(map[string]any{}))
	base := api.client(WithHeader("X-Team", "payments"))
	quiet, err := base.WithOptions(WithHeader("X-Team", "reports"), WithRetry(RetryOptions{MaxRetries: 0}))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := quiet.Account.GetBalance(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Account.GetBalance(ctx); err != nil {
		t.Fatal(err)
	}
	if got := api.at(0).header.Get("X-Team"); got != "reports" {
		t.Fatalf("copy sent X-Team %q", got)
	}
	if got := api.at(1).header.Get("X-Team"); got != "payments" {
		t.Fatalf("the original changed: X-Team %q", got)
	}
	if quiet.transport.retry.MaxRetries != 0 || base.transport.retry.MaxRetries != 2 {
		t.Fatalf("retries: copy %d, original %d", quiet.transport.retry.MaxRetries, base.transport.retry.MaxRetries)
	}
	if quiet.transport.clock != base.transport.clock {
		t.Fatal("the copy must share the learned clock offset")
	}
	if api.at(0).header.Get(HeaderPublicID) != "pk_test_1" {
		t.Fatal("the copy must keep the credentials")
	}
	if _, err := base.WithOptions(WithBaseURL("ftp://nowhere")); !IsConfig(err) {
		t.Fatalf("an unusable option must be refused: %v", err)
	}
}

func TestHooksSeeEveryAttemptWithSecretsRedacted(t *testing.T) {
	api := newFakeAPI(t,
		apiError(503, map[string]any{"code": "db.unavailable", "retryable": true}),
		ok(map[string]any{}),
	)
	var mu sync.Mutex
	var requests []RequestInfo
	var responses []ResponseInfo
	client := api.client(WithHooks(Hooks{
		OnRequest:  func(r RequestInfo) { mu.Lock(); requests = append(requests, r); mu.Unlock() },
		OnResponse: func(r ResponseInfo) { mu.Lock(); responses = append(responses, r); mu.Unlock() },
	}))
	if _, err := client.Account.GetBalance(context.Background(), WithRequestID("rid")); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || len(responses) != 2 {
		t.Fatalf("%d requests, %d responses: one of each per attempt", len(requests), len(responses))
	}
	first := requests[0]
	if first.OperationID != "getBalance" || first.Method != "POST" || first.Attempt != 1 || requests[1].Attempt != 2 || first.RequestID != "rid" {
		t.Fatalf("request info %+v", first)
	}
	if first.Header.Get(HeaderSignature) != redactedPlaceholder || first.Header.Get(HeaderPublicID) != "pk_test_1" {
		t.Fatalf("hook headers %v", first.Header)
	}
	if responses[0].StatusCode != 503 || !IsCode(responses[0].Err, "db.unavailable") || responses[1].StatusCode != 200 || responses[1].Err != nil {
		t.Fatalf("responses %+v / %+v", responses[0], responses[1])
	}
	if responses[1].Request.Attempt != 2 || responses[1].Elapsed <= 0 {
		t.Fatalf("response info %+v", responses[1])
	}

	// A transport failure reaches OnResponse with status 0.
	var got []ResponseInfo
	slow := newFakeAPI(t, step{delay: 200 * time.Millisecond})
	_, _ = slow.client(WithHooks(Hooks{OnResponse: func(r ResponseInfo) { got = append(got, r) }})).
		Account.GetBalance(context.Background(), WithRequestTimeout(10*time.Millisecond), WithMaxRetries(0))
	if len(got) != 1 || got[0].StatusCode != 0 || !IsCode(got[0].Err, CodeTransportTimeout) {
		t.Fatalf("transport failure hook %+v", got)
	}
}
