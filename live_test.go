package oblodai

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// Live tests run against a REAL core, not a stub: set OBLODAI_LIVE_URL (for example
// http://127.0.0.1:8095) and they onboard a merchant, take a sandbox key and walk the money path.
// Without that variable they skip, so `go test ./...` stays hermetic.
//
//	OBLODAI_LIVE_URL=http://127.0.0.1:8095 go test -run TestLive ./...
//
// Set OBLODAI_LIVE_HOOK_URL to a reachable receiver to exercise webhook delivery as well.

const liveAddress = "TQrY8bkbpXKPt2LZbU8jqfnpFbUSF15sbx"

func liveURL(t *testing.T) string {
	t.Helper()
	base := os.Getenv("OBLODAI_LIVE_URL")
	if base == "" {
		t.Skip("OBLODAI_LIVE_URL is not set: skipping the live journey")
	}
	return base
}

func liveHookURL() string {
	if hook := os.Getenv("OBLODAI_LIVE_HOOK_URL"); hook != "" {
		return hook
	}
	return "http://127.0.0.1:8096/hook"
}

// onboardSandbox provisions a merchant and returns a client holding its sandbox key. Onboarding is
// unsigned, so the same client does both steps.
func onboardSandbox(t *testing.T, base string) (*Client, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	anonymous, err := New(WithBaseURL(base), WithInsecureBaseURL(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	merchant, err := anonymous.Merchants.Create(ctx, MerchantsParams{
		Email: fmt.Sprintf("sdk-go-%d@example.com", time.Now().UnixNano()),
		Name:  "SDK live",
	})
	if err != nil {
		t.Fatalf("Merchants.Create: %v", err)
	}
	store, err := anonymous.Merchants.CreateSandbox(ctx, merchant.MerchantID)
	if err != nil {
		t.Fatalf("Merchants.CreateSandbox: %v", err)
	}
	client, err := New(
		WithBaseURL(base),
		WithInsecureBaseURL(true),
		WithCredentials(store.APIKey.PublicID, store.APIKey.Secret),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client, store.MerchantID
}

func TestLiveSandboxJourney(t *testing.T) {
	base := liveURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	client, _ := onboardSandbox(t, base)

	t.Run("public catalog needs no credentials", func(t *testing.T) {
		anonymous, err := New(WithBaseURL(base), WithInsecureBaseURL(true))
		if err != nil {
			t.Fatal(err)
		}
		catalog, err := anonymous.Catalog.Currencies(ctx)
		if err != nil {
			t.Fatalf("Catalog.Currencies: %v", err)
		}
		if len(catalog.Currencies) == 0 {
			t.Fatal("the catalog is empty")
		}
	})

	orderID := fmt.Sprintf("sdk-go-live-%d", time.Now().UnixNano())
	invoice, err := client.Payments.Create(ctx, PaymentParams{
		Amount: "25", Currency: "USDT", Network: NetworkTron, OrderID: orderID,
	})
	if err != nil {
		t.Fatalf("Payments.Create: %v", err)
	}
	if invoice.Status != PaymentStatusCreated {
		t.Fatalf("a fresh invoice is %q, want created", invoice.Status)
	}

	t.Run("reads the invoice back by order_id, by uuid and in the history", func(t *testing.T) {
		byOrder, err := client.Payments.Info(ctx, PaymentInfoParams{OrderID: invoice.OrderID})
		if err != nil {
			t.Fatalf("Payments.Info by order_id: %v", err)
		}
		if byOrder.UUID != invoice.UUID {
			t.Fatalf("order_id lookup returned %s", byOrder.UUID)
		}
		limit := 5
		history, err := client.Payments.History(ctx, PaymentHistoryParams{Limit: &limit}).Page()
		if err != nil {
			t.Fatalf("Payments.History: %v", err)
		}
		found := false
		for _, payment := range history.Items {
			found = found || payment.UUID == invoice.UUID
		}
		if !found {
			t.Fatal("the invoice is missing from its own history")
		}
		// A signed GET with a query: the signature must cover path AND query.
		if _, err := client.Sandbox.Webhooks(ctx, SandboxWebhooksParams{Limit: &limit}).Page(); err != nil {
			t.Fatalf("Sandbox.Webhooks: %v", err)
		}
	})

	t.Run("replays an idempotent create and refuses a reused key", func(t *testing.T) {
		key := fmt.Sprintf("sdk-go-idem-%d", time.Now().UnixNano())
		params := PaymentParams{Amount: "1", Currency: "USDT", Network: NetworkTron, OrderID: key + "-o"}
		first, err := client.Payments.Create(ctx, params, WithIdempotencyKey(key))
		if err != nil {
			t.Fatalf("first create: %v", err)
		}
		second, err := client.Payments.Create(ctx, params, WithIdempotencyKey(key))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if second.UUID != first.UUID {
			t.Fatalf("the replay created a second invoice: %s != %s", second.UUID, first.UUID)
		}
		different := params
		different.Amount = "2"
		different.OrderID = key + "-o2"
		_, err = client.Payments.Create(ctx, different, WithIdempotencyKey(key))
		conflict := requireCode(t, err, CodeIdempotencyKeyReused)
		if conflict.HTTPStatus != 409 || !IsIdempotencyConflict(err) {
			t.Fatalf("unexpected conflict: %+v", conflict)
		}
	})

	t.Run("simulates a deposit and pays a payout out of the proceeds", func(t *testing.T) {
		confirmations := 20
		if _, err := client.Sandbox.Deposit(ctx, SandboxDepositParams{
			InvoiceID: invoice.UUID, Amount: "25", Confirmations: &confirmations,
			TxID: fmt.Sprintf("sdk-go-tx-%d", time.Now().UnixNano()),
		}); err != nil {
			t.Fatalf("Sandbox.Deposit: %v", err)
		}
		paid, err := client.Payments.Info(ctx, PaymentInfoParams{UUID: invoice.UUID})
		if err != nil {
			t.Fatalf("Payments.Info: %v", err)
		}
		if !IsPaymentPaid(paid.Status) {
			t.Fatalf("the invoice is %q after a full deposit", paid.Status)
		}

		if _, err := client.Sandbox.Faucet(ctx, SandboxFaucetParams{Asset: "USDT", Amount: "100"}); err != nil {
			t.Fatalf("Sandbox.Faucet: %v", err)
		}
		balance, err := client.Account.Balance(ctx)
		if err != nil {
			t.Fatalf("Account.Balance: %v", err)
		}
		if len(balance.Balance.Merchant) == 0 {
			t.Fatal("the balance is empty after a faucet")
		}

		if _, err := client.Payouts.Calculate(ctx, PayoutCalculateParams{
			Amount: "10", Currency: "USDT", Network: NetworkTron,
		}); err != nil {
			t.Fatalf("Payouts.Calculate: %v", err)
		}
		validation, err := client.Payouts.Validate(ctx, PayoutValidateParams{
			Amount: "10", Currency: "USDT", Network: NetworkTron, Address: liveAddress,
		})
		if err != nil {
			t.Fatalf("Payouts.Validate: %v", err)
		}
		if !validation.Valid {
			t.Fatalf("the dry run refused a payout the balance covers: %+v", validation)
		}
		payout, err := client.Payouts.Create(ctx, PayoutParams{
			Amount: "10", Currency: "USDT", Network: NetworkTron, Address: liveAddress,
			OrderID: fmt.Sprintf("sdk-go-po-%d", time.Now().UnixNano()),
		})
		if err != nil {
			t.Fatalf("Payouts.Create: %v", err)
		}
		readBack, err := client.Payouts.Info(ctx, PayoutInfoParams{UUID: payout.UUID})
		if err != nil {
			t.Fatalf("Payouts.Info: %v", err)
		}
		if readBack.OrderID == nil || payout.OrderID == nil || *readBack.OrderID != *payout.OrderID {
			t.Fatalf("the payout came back with a different order_id: %v", readBack.OrderID)
		}
	})

	t.Run("classifies a domain refusal with the core's own retryable flag", func(t *testing.T) {
		_, err := client.Payouts.Create(ctx, PayoutParams{
			Amount: "999999", Currency: "USDT", Network: NetworkTron, Address: liveAddress,
			OrderID: fmt.Sprintf("sdk-go-big-%d", time.Now().UnixNano()),
		})
		refusal := requireCode(t, err, "payout.insufficient_funds")
		if refusal.HTTPStatus != 409 {
			t.Fatalf("status = %d, want 409", refusal.HTTPStatus)
		}
		if refusal.RequestID == "" {
			t.Fatal("a refusal must carry a request id to quote to support")
		}
	})

	t.Run("registers an endpoint and delivers a signed test webhook", func(t *testing.T) {
		endpoint, err := client.Webhooks.Register(ctx, liveHookURL())
		if err != nil {
			t.Fatalf("Webhooks.Register: %v", err)
		}
		if endpoint.Secret == "" {
			t.Fatal("registration must return the signing secret once")
		}
		if os.Getenv("OBLODAI_LIVE_HOOK_URL") == "" {
			t.Skip("no reachable receiver: delivery itself is covered by the recorded samples")
		}
		result, err := client.Webhooks.Test(ctx, WebhookKindPayment, WebhookTestParams{
			URLCallback: liveHookURL(), Currency: "USDT", Network: NetworkTron, Status: "paid",
		})
		if err != nil {
			t.Fatalf("Webhooks.Test: %v", err)
		}
		if !result.OK {
			t.Fatalf("the delivery was refused: %+v", result)
		}
	})
}
