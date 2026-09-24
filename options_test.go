package oblodai

import (
	"context"
	"testing"
	"time"
)

// The five things one call may override — idempotency key, timeout, retries, headers, request
// id — and the X-Request-ID every call carries.

func TestEveryCallCarriesOneRequestIDOnEveryAttempt(t *testing.T) {
	api := newFakeAPI(t,
		apiError(503, map[string]any{"code": "db.unavailable", "retryable": true}),
		ok(map[string]any{"balance": map[string]any{}}),
		ok(map[string]any{"balance": map[string]any{}}),
	)
	client := api.client()
	if _, err := client.Account.GetBalance(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, retry := api.at(0).header.Get(HeaderRequestID), api.at(1).header.Get(HeaderRequestID)
	if len(first) != 36 || first != retry {
		t.Fatalf("X-Request-ID = %q then %q: one generated id for every attempt of a call", first, retry)
	}
	if _, err := client.Account.GetBalance(context.Background()); err != nil {
		t.Fatal(err)
	}
	if next := api.at(2).header.Get(HeaderRequestID); next == first || next == "" {
		t.Fatalf("the next call reused the id %q", next)
	}
}

func TestWithRequestIDIsSentAndNamesTheError(t *testing.T) {
	api := newFakeAPI(t,
		ok(map[string]any{"balance": map[string]any{}}),
		apiError(400, map[string]any{"code": "payment.bad_amount", "message": "bad", "retryable": false}),
	)
	client := api.client()
	ctx := context.Background()
	if _, err := client.Account.GetBalance(ctx, WithRequestID("checkout-42")); err != nil {
		t.Fatal(err)
	}
	if got := api.at(0).header.Get(HeaderRequestID); got != "checkout-42" {
		t.Fatalf("X-Request-ID = %q", got)
	}
	_, err := client.Payments.Create(ctx, &PaymentRequest{Amount: "1", Currency: "USDT"}, WithRequestID("order-7"))
	if e := requireCode(t, err, "payment.bad_amount"); e.RequestID != "order-7" {
		t.Fatalf("an error without the core's request id names the call's: %q", e.RequestID)
	}
	// A caller's own X-Request-ID header is adopted as the call's id.
	api2 := newFakeAPI(t, ok(map[string]any{"balance": map[string]any{}}))
	if _, err := api2.client(WithHeader("x-request-id", "from-header")).Account.GetBalance(ctx); err != nil {
		t.Fatal(err)
	}
	if got := api2.last().header.Values(HeaderRequestID); len(got) != 1 || got[0] != "from-header" {
		t.Fatalf("X-Request-ID = %v", got)
	}
	// An id that cannot be sent verbatim is refused before the network.
	_, err = client.Account.GetBalance(ctx, WithRequestID("a\r\nb"))
	_ = requireCode(t, err, CodeBadHeader)
}

func TestWithMaxRetriesOverridesThePolicyForOneCall(t *testing.T) {
	down := map[string]any{"code": "db.unavailable", "retryable": true}
	api := newFakeAPI(t, apiError(503, down), apiError(503, down), apiError(503, down))
	_, err := api.client().Account.GetBalance(context.Background(), WithMaxRetries(0))
	_ = requireCode(t, err, "db.unavailable")
	if api.count() != 1 {
		t.Fatalf("WithMaxRetries(0) made %d attempts", api.count())
	}

	more := newFakeAPI(t, apiError(503, down), apiError(503, down), apiError(503, down), ok(map[string]any{}))
	if _, err := more.client().Account.GetBalance(context.Background(), WithMaxRetries(3)); err != nil {
		t.Fatalf("WithMaxRetries(3): %v", err)
	}
	if more.count() != 4 {
		t.Fatalf("WithMaxRetries(3) made %d attempts", more.count())
	}
}

func TestWithRequestTimeoutBoundsOneAttempt(t *testing.T) {
	api := newFakeAPI(t, step{delay: 300 * time.Millisecond, body: map[string]any{"state": 0, "result": map[string]any{}}})
	started := time.Now()
	_, err := api.client().Account.GetBalance(context.Background(), WithRequestTimeout(20*time.Millisecond), WithMaxRetries(0))
	_ = requireCode(t, err, CodeTransportTimeout)
	if time.Since(started) > 250*time.Millisecond {
		t.Fatal("the per-call timeout was not applied")
	}
}

func TestWithExtraHeadersAddsHeadersToOneCall(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{}), ok(map[string]any{}))
	client := api.client()
	ctx := context.Background()
	if _, err := client.Account.GetBalance(ctx, WithExtraHeaders(map[string]string{"X-Trace": "t1", "X-Signature": "zz"})); err != nil {
		t.Fatal(err)
	}
	if got := api.at(0).header.Get("X-Trace"); got != "t1" {
		t.Fatalf("X-Trace = %q", got)
	}
	if !hexish(api.at(0).header.Get(HeaderSignature), 32) {
		t.Fatal("a caller header must not replace the signature")
	}
	if _, err := client.Account.GetBalance(ctx); err != nil {
		t.Fatal(err)
	}
	if got := api.at(1).header.Get("X-Trace"); got != "" {
		t.Fatalf("a per-call header leaked into the next call: %q", got)
	}
}

// Ruling 10: a route whose body has its own idempotency_key field takes the option there, and no
// Idempotency-Key header is sent (the route is not deduplicated by the header).
func TestIdempotencyKeyGoesIntoTheBodyFieldOfTheFaucet(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{}), ok(map[string]any{}))
	client := api.client()
	ctx := context.Background()
	if _, err := client.Sandbox.Faucet(ctx, &FaucetRequest{Amount: "100", Asset: "USDT"}, WithIdempotencyKey("tap-1")); err != nil {
		t.Fatalf("Faucet: %v", err)
	}
	req := api.at(0)
	if body := req.jsonBody(t); body["idempotency_key"] != "tap-1" || body["amount"] != "100" {
		t.Fatalf("body = %v", body)
	}
	if got := req.header.Get(HeaderIdempotencyKey); got != "" {
		t.Fatalf("Idempotency-Key header %q on a route that takes the key in its body", got)
	}
	// Without the option the params' own field goes as set.
	if _, err := client.Sandbox.Faucet(ctx, &FaucetRequest{Amount: "1", Asset: "USDT", IdempotencyKey: Ptr("own")}); err != nil {
		t.Fatal(err)
	}
	if body := api.at(1).jsonBody(t); body["idempotency_key"] != "own" {
		t.Fatalf("body = %v", body)
	}
}
