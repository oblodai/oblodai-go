package oblodai

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// What the client puts on the wire, and how it reacts to what comes back. Every test here is a
// rule the reference SDK guarantees, exercised through a real HTTP round trip.

func TestGETIsSignedOverPathAndQueryWithNoBody(t *testing.T) {
	api := newFakeAPI(t, emptyPage())
	client := api.client()
	limit, offset := int64(10), int64(0)
	if _, err := client.Sandbox.ListWebhooks(context.Background(), &SandboxListWebhooksParams{Limit: &limit, Offset: &offset}).Page(); err != nil {
		t.Fatalf("Sandbox.ListWebhooks: %v", err)
	}
	req := api.last()
	if req.method != "GET" || req.path != "/v1/sandbox/webhooks" {
		t.Fatalf("unexpected request line: %s %s", req.method, req.path)
	}
	if req.rawQuery != "limit=10&offset=0" {
		t.Fatalf("query = %q, want limit=10&offset=0", req.rawQuery)
	}
	if req.body != "" {
		t.Fatalf("a GET must not carry a body, got %q", req.body)
	}
	if got := req.header.Get(HeaderPublicID); got != "pk_test_1" {
		t.Fatalf("X-Public-Id = %q", got)
	}
	if got := req.header.Get(HeaderSignature); !hexish(got, 32) {
		t.Fatalf("X-Signature = %q, want a hex SHA-256 digest", got)
	}
	// The signature must cover path AND query, exactly as the request line carries them.
	ts, err := strconv.ParseInt(req.header.Get(HeaderTimestamp), 10, 64)
	if err != nil {
		t.Fatalf("X-Timestamp = %q", req.header.Get(HeaderTimestamp))
	}
	want := SignRequest("secret-1", SignInput{TS: ts, Method: "GET", RequestURI: "/v1/sandbox/webhooks?limit=10&offset=0"})
	if got := req.header.Get(HeaderSignature); got != want {
		t.Fatalf("signature does not cover path+query:\n got %s\nwant %s", got, want)
	}
}

func TestIdempotencyKeyIsGeneratedOnceAndReusedAcrossRetries(t *testing.T) {
	api := newFakeAPI(t,
		apiError(503, map[string]any{"code": "db.unavailable", "message": "down", "retryable": true}),
		ok(map[string]any{"uuid": "u"}),
	)
	client := api.client()
	if _, err := client.Payments.Create(context.Background(), &PaymentRequest{Amount: "1", Currency: "USDT"}); err != nil {
		t.Fatalf("Payments.Create: %v", err)
	}
	if api.count() != 2 {
		t.Fatalf("expected 2 attempts, saw %d", api.count())
	}
	key := api.at(0).header.Get(HeaderIdempotencyKey)
	if len(key) != 36 {
		t.Fatalf("Idempotency-Key = %q, want a UUID", key)
	}
	if got := api.at(1).header.Get(HeaderIdempotencyKey); got != key {
		t.Fatalf("the retry used a different key: %q != %q — a lost response could become a double spend", got, key)
	}
	if !hexish(api.at(1).header.Get(HeaderSignature), 32) {
		t.Fatal("the retry must be re-signed")
	}
}

func TestCallerIdempotencyKeyAndReadRoutes(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"uuid": "u"}), ok(map[string]any{"uuid": "u"}))
	client := api.client()
	ctx := context.Background()
	if _, err := client.Payouts.Create(ctx,
		&PayoutRequest{Amount: "1", Currency: "USDT", Address: "T", OrderID: "o"},
		WithIdempotencyKey("my-key-1")); err != nil {
		t.Fatalf("Payouts.Create: %v", err)
	}
	if _, err := client.Payments.GetInfo(ctx, &LookupRequest{UUID: Ptr("u")}); err != nil {
		t.Fatalf("Payments.GetInfo: %v", err)
	}
	if got := api.at(0).header.Get(HeaderIdempotencyKey); got != "my-key-1" {
		t.Fatalf("caller key was not used: %q", got)
	}
	if got := api.at(1).header.Get(HeaderIdempotencyKey); got != "" {
		t.Fatalf("a read route must not carry an idempotency key, got %q", got)
	}
}

func TestNonRetryableErrorIsNotRetriedEvenOn5xx(t *testing.T) {
	api := newFakeAPI(t, apiError(500, map[string]any{"code": "internal", "retryable": false}))
	_, err := api.client().Account.GetBalance(context.Background())
	apiErr := requireCode(t, err, "internal")
	if apiErr.HTTPStatus != 500 || apiErr.Retryable {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
	if api.count() != 1 {
		t.Fatalf("expected 1 attempt, saw %d", api.count())
	}
}

func TestRetryableErrorIsRetriedUntilTheBudgetRunsOut(t *testing.T) {
	rateLimited := map[string]any{"code": "request.rate_limited", "retryable": true, "retry_after": 0}
	api := newFakeAPI(t,
		apiError(429, rateLimited, map[string]string{"Retry-After": "0"}),
		apiError(429, rateLimited),
		apiError(429, rateLimited),
	)
	_, err := api.client().Account.GetBalance(context.Background())
	apiErr := requireCode(t, err, "request.rate_limited")
	if apiErr.Kind != KindRateLimit || !IsRateLimit(err) {
		t.Fatalf("expected a rate-limit error, got %+v", apiErr)
	}
	if apiErr.RetryAfter == nil || *apiErr.RetryAfter != 0 {
		t.Fatalf("RetryAfter = %v, want 0", apiErr.RetryAfter)
	}
	if api.count() != 3 { // the first attempt plus MaxRetries
		t.Fatalf("expected 3 attempts, saw %d", api.count())
	}
}

func TestTransportFailureIsRetriedOnlyWhenSafeToRepeat(t *testing.T) {
	ctx := context.Background()

	read := newFakeAPI(t, step{abort: true}, ok(map[string]any{"balance": map[string]any{"merchant": []any{}}}))
	if _, err := read.client().Account.GetBalance(ctx); err != nil {
		t.Fatalf("a read route must be retried after a network failure: %v", err)
	}
	if read.count() != 2 {
		t.Fatalf("read route: expected 2 attempts, saw %d", read.count())
	}

	// A write the core does not deduplicate may have reached it: re-sending could double it.
	write := newFakeAPI(t, step{abort: true}, ok(map[string]any{}))
	_, err := write.client().Settings.SetAccuracy(ctx, &SetAccuracyRequest{Enabled: true})
	apiErr := mustError(t, err)
	if apiErr.Kind != KindTransport || apiErr.Code != CodeTransportNetwork {
		t.Fatalf("expected a network transport error, got %+v", apiErr)
	}
	if write.count() != 1 {
		t.Fatalf("unsafe write: expected 1 attempt, saw %d", write.count())
	}

	// A keyed write is deduplicated by the core, so repeating it is safe.
	keyed := newFakeAPI(t, step{abort: true}, ok(map[string]any{"uuid": "u"}))
	if _, err := keyed.client().Payments.Create(ctx, &PaymentRequest{Amount: "1", Currency: "USDT"}); err != nil {
		t.Fatalf("a keyed write must be retried: %v", err)
	}
	if keyed.count() != 2 {
		t.Fatalf("keyed write: expected 2 attempts, saw %d", keyed.count())
	}
}

func TestErrorEnvelopeIsClassified(t *testing.T) {
	api := newFakeAPI(t,
		apiError(400, map[string]any{
			"code": "payment.below_minimum", "message": "too small",
			"field": "amount", "retryable": false, "request_id": "rq-1",
		}),
		apiError(401, map[string]any{"code": "merchant.bad_signature", "message": "bad", "retryable": false}),
		apiError(409, map[string]any{"code": "idempotency.key_reused", "message": "reused", "retryable": false}),
		apiError(403, map[string]any{"code": "merchant.key_mode_mismatch", "retryable": false}),
		apiError(404, map[string]any{"code": "payment.not_found", "retryable": false}),
	)
	client := api.client(WithRetry(RetryOptions{MaxRetries: 0}))
	ctx := context.Background()

	_, err := client.Payments.Create(ctx, &PaymentRequest{Amount: "0", Currency: "USDT"})
	validation := requireCode(t, err, "payment.below_minimum")
	if !IsValidation(err) || validation.Field != "amount" || validation.RequestID != "rq-1" || validation.Family() != "payment" {
		t.Fatalf("unexpected validation error: %+v", validation)
	}

	if _, err := client.Account.GetBalance(ctx); !IsAuthentication(err) {
		t.Fatalf("expected an authentication error, got %v", err)
	}
	_, err = client.Payments.Create(ctx, &PaymentRequest{Amount: "1", Currency: "USDT"})
	if !IsIdempotencyConflict(err) || !IsConflict(err) {
		t.Fatalf("expected an idempotency conflict (which is also a conflict), got %v", err)
	}
	if _, err := client.Account.GetBalance(ctx); !IsPermission(err) || !IsCode(err, "merchant.key_mode_mismatch") {
		t.Fatalf("expected a permission error, got %v", err)
	}
	if _, err := client.Account.GetBalance(ctx); !IsNotFound(err) {
		t.Fatalf("expected a not-found error, got %v", err)
	}
}

func TestClockSkewIsCorrectedOnceFromTheServerDate(t *testing.T) {
	serverNow := time.Now().Add(time.Hour)
	api := newFakeAPI(t,
		apiError(401, map[string]any{"code": "merchant.bad_signature", "retryable": false},
			map[string]string{"Date": serverNow.UTC().Format(http.TimeFormat)}),
		ok(map[string]any{"balance": map[string]any{"merchant": []any{}}}),
	)
	client := api.client(WithRetry(RetryOptions{MaxRetries: 0}))
	if _, err := client.Account.GetBalance(context.Background()); err != nil {
		t.Fatalf("Account.GetBalance: %v", err)
	}
	if api.count() != 2 {
		t.Fatalf("expected the call to be re-signed once, saw %d attempts", api.count())
	}
	ts, err := strconv.ParseInt(api.at(1).header.Get(HeaderTimestamp), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if drift := ts - serverNow.Unix(); drift > 5 || drift < -5 {
		t.Fatalf("the re-signed timestamp is %d s from the server clock", drift)
	}
	if client.ClockOffset() < 55*time.Minute {
		t.Fatalf("the offset should have been kept, got %s", client.ClockOffset())
	}
}

func TestPerAttemptTimeout(t *testing.T) {
	api := newFakeAPI(t, step{delay: 300 * time.Millisecond, body: map[string]any{"state": 0, "result": map[string]any{}}})
	client := api.client(WithTimeout(20*time.Millisecond), WithRetry(RetryOptions{MaxRetries: 0}))
	_, err := client.Account.GetBalance(context.Background())
	apiErr := requireCode(t, err, CodeTransportTimeout)
	if !IsTransport(err) || !apiErr.Retryable {
		t.Fatalf("a timeout is a retryable transport error, got %+v", apiErr)
	}
}

// One API key signs every gated route: the money-out side is not a separate credential.
func TestTheOneAPIKeySignsPayoutsAndPaymentsAlike(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"uuid": "p"}), ok(map[string]any{"uuid": "i"}))
	client := api.client()
	ctx := context.Background()
	if _, err := client.Payouts.Create(ctx, &PayoutRequest{Amount: "1", Currency: "USDT", Address: "T", OrderID: "o"}); err != nil {
		t.Fatalf("Payouts.Create: %v", err)
	}
	if _, err := client.Payments.Create(ctx, &PaymentRequest{Amount: "1", Currency: "USDT"}); err != nil {
		t.Fatalf("Payments.Create: %v", err)
	}
	for i, label := range []string{"a payout route", "a payment route"} {
		if got := api.at(i).header.Get(HeaderPublicID); got != "pk_test_1" {
			t.Fatalf("%s must use the merchant's API key, got %q", label, got)
		}
		if !hexish(api.at(i).header.Get(HeaderSignature), 32) {
			t.Fatalf("%s is not signed", label)
		}
	}
}

func TestCredentialsAreOnlyRequiredWhereTheRouteNeedsThem(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"currencies": []any{}, "pricing_currencies": []any{}}))
	client, err := New(WithBaseURL(api.server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := client.Checkout.ListCurrencies(ctx); err != nil {
		t.Fatalf("a public route must work without credentials: %v", err)
	}
	_, err = client.Account.GetBalance(ctx)
	if !IsConfig(err) || !IsCode(err, CodeMissingCredentials) {
		t.Fatalf("expected sdk.missing_credentials, got %v", err)
	}
	if api.count() != 1 {
		t.Fatal("the unsigned call must not have left the process")
	}
}

func TestOnboardRoutesCarryTheAdminTokenAndNoSignature(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"merchant_id": "m1"}))
	client := api.client(WithAdminToken("adm"))
	if _, err := client.Sandbox.OnboardStore(context.Background(), "m1"); err != nil {
		t.Fatalf("Sandbox.OnboardStore: %v", err)
	}
	req := api.last()
	if got := req.header.Get(HeaderAdminToken); got != "adm" {
		t.Fatalf("X-Admin-Token = %q", got)
	}
	if got := req.header.Get(HeaderSignature); got != "" {
		t.Fatalf("an onboarding route is unsigned, got X-Signature = %q", got)
	}
}

func TestErrorSerializationKeepsTheMessageAndDropsTheBody(t *testing.T) {
	api := newFakeAPI(t, apiError(400, map[string]any{
		"code": "payment.below_minimum", "message": "too small", "retryable": false,
		"secret_echo": "must never be logged",
	}))
	_, err := api.client().Payments.Create(context.Background(), &PaymentRequest{Amount: "0", Currency: "USDT"})
	apiErr := requireCode(t, err, "payment.below_minimum")

	encoded, marshalErr := json.Marshal(apiErr)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["message"] != "too small" || fields["code"] != "payment.below_minimum" {
		t.Fatalf("serialized error lost its identity: %s", encoded)
	}
	for _, forbidden := range []string{"raw", "body", "secret_echo"} {
		if _, present := fields[forbidden]; present {
			t.Fatalf("serialized error carries %q: %s", forbidden, encoded)
		}
	}
	if len(apiErr.Body()) == 0 {
		t.Fatal("the raw body should still be reachable through Body() for debugging")
	}
}
