package oblodai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// Live tests run against a REAL core, not a stub: set OBLODAI_LIVE_URL (for example
// http://127.0.0.1:8095) and they onboard a merchant, take a sandbox key and walk the money path.
// Without that variable they skip, so `go test ./...` stays hermetic.
//
//	OBLODAI_LIVE_URL=http://127.0.0.1:8095 make live
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

// onboardSandbox provisions a merchant and returns a client holding its sandbox key. Creating the
// merchant is dev-stand provisioning, not part of the merchant API contract, so it is a plain
// HTTP call; minting the sandbox store is Sandbox.OnboardStore.
func onboardSandbox(ctx context.Context, t *testing.T, base string) *Client {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email": fmt.Sprintf("sdk-go-%d@example.com", time.Now().UnixNano()), "name": "SDK live",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/merchants", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/merchants: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var merchant struct {
		Result struct {
			MerchantID string `json:"merchant_id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&merchant); err != nil || merchant.Result.MerchantID == "" {
		t.Fatalf("POST /v1/merchants: HTTP %d, %v", res.StatusCode, err)
	}

	anonymous, err := New(WithBaseURL(base), WithInsecureBaseURL(true))
	if err != nil {
		t.Fatal(err)
	}
	store, err := anonymous.Sandbox.OnboardStore(ctx, merchant.Result.MerchantID)
	if err != nil {
		t.Fatalf("Sandbox.OnboardStore: %v", err)
	}
	client, err := New(WithBaseURL(base), WithInsecureBaseURL(true),
		WithCredentials(store.APIKey.PublicID, store.APIKey.Secret))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestLiveSandboxJourney(t *testing.T) {
	base := liveURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	client := onboardSandbox(ctx, t, base)

	t.Run("public catalog needs no credentials", func(t *testing.T) {
		anonymous, err := New(WithBaseURL(base), WithInsecureBaseURL(true))
		if err != nil {
			t.Fatal(err)
		}
		catalog, err := anonymous.Checkout.ListCurrencies(ctx)
		if err != nil || len(catalog.Currencies) == 0 {
			t.Fatalf("Checkout.ListCurrencies: %v", err)
		}
	})

	orderID := fmt.Sprintf("sdk-go-live-%d", time.Now().UnixNano())
	invoice, err := client.Payments.Create(ctx, &PaymentRequest{
		Amount: "25", Currency: "USDT", Network: Ptr("tron"), OrderID: &orderID,
	})
	if err != nil {
		t.Fatalf("Payments.Create: %v", err)
	}
	if invoice.Status != PaymentStatusCreated {
		t.Fatalf("a fresh invoice is %q, want created", invoice.Status)
	}

	t.Run("reads the invoice back by order_id and in the history", func(t *testing.T) {
		byOrder, err := client.Payments.GetInfo(ctx, &LookupRequest{OrderID: &orderID})
		if err != nil || byOrder.UUID != invoice.UUID {
			t.Fatalf("Payments.GetInfo by order_id: %v", err)
		}
		found := false
		for payment, err := range client.Payments.ListHistory(ctx, &PaymentHistoryRequest{Limit: Ptr[int64](5)}).Items() {
			if err != nil {
				t.Fatalf("Payments.ListHistory: %v", err)
			}
			if payment.UUID == invoice.UUID {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("the invoice is missing from its own history")
		}
		// A signed GET with a query: the signature must cover path AND query.
		if _, err := client.Sandbox.ListWebhooks(ctx, &SandboxListWebhooksParams{Limit: Ptr[int64](5)}).Page(); err != nil {
			t.Fatalf("Sandbox.ListWebhooks: %v", err)
		}
	})

	t.Run("replays an idempotent create and refuses a reused key", func(t *testing.T) {
		key := fmt.Sprintf("sdk-go-idem-%d", time.Now().UnixNano())
		params := &PaymentRequest{Amount: "5", Currency: "USDT", Network: Ptr("tron"), OrderID: Ptr(key + "-o")}
		first, err := client.Payments.Create(ctx, params, WithIdempotencyKey(key))
		if err != nil {
			t.Fatalf("first create: %v", err)
		}
		second, err := client.Payments.Create(ctx, params, WithIdempotencyKey(key))
		if err != nil || second.UUID != first.UUID {
			t.Fatalf("the replay created a second invoice: %v", err)
		}
		different := *params
		different.Amount, different.OrderID = "2", Ptr(key+"-o2")
		_, err = client.Payments.Create(ctx, &different, WithIdempotencyKey(key))
		if conflict := requireCode(t, err, CodeIdempotencyKeyReused); conflict.HTTPStatus != 409 {
			t.Fatalf("unexpected conflict: %+v", conflict)
		}
	})

	t.Run("simulates a deposit and pays a payout out of the proceeds", func(t *testing.T) {
		if _, err := client.Sandbox.SimulateDeposit(ctx, &SimulateDepositRequest{
			InvoiceID: invoice.UUID, Amount: Ptr("25"), Confirmations: Ptr[int64](20),
			Txid: Ptr(fmt.Sprintf("sdk-go-tx-%d", time.Now().UnixNano())),
		}); err != nil {
			t.Fatalf("Sandbox.SimulateDeposit: %v", err)
		}
		paid, err := client.Payments.GetInfo(ctx, &LookupRequest{UUID: &invoice.UUID})
		if err != nil || !IsPaymentPaid(paid.Status) {
			t.Fatalf("after a full deposit: %v %+v", err, paid)
		}
		if _, err := client.Sandbox.Faucet(ctx, &FaucetRequest{Asset: "USDT", Amount: "100"}); err != nil {
			t.Fatalf("Sandbox.Faucet: %v", err)
		}
		balance, err := client.Account.GetBalance(ctx)
		if err != nil || len(balance.Balance.Merchant) == 0 {
			t.Fatalf("Account.GetBalance after a faucet: %v", err)
		}
		validation, err := client.Payouts.Validate(ctx, &PayoutValidateRequest{
			Amount: "10", Currency: "USDT", Network: Ptr("tron"), Address: liveAddress,
		})
		if err != nil || !validation.Valid {
			t.Fatalf("the dry run refused a payout the balance covers: %v %+v", err, validation)
		}
		payout, err := client.Payouts.Create(ctx, &PayoutRequest{
			Amount: "10", Currency: "USDT", Network: Ptr("tron"), Address: liveAddress,
			OrderID: fmt.Sprintf("sdk-go-po-%d", time.Now().UnixNano()),
		})
		if err != nil {
			t.Fatalf("Payouts.Create: %v", err)
		}
		readBack, err := client.Payouts.GetInfo(ctx, &LookupRequest{UUID: &payout.UUID})
		if err != nil || readBack.UUID != payout.UUID {
			t.Fatalf("Payouts.GetInfo: %v", err)
		}
	})

	t.Run("classifies a domain refusal with the core's own retryable flag", func(t *testing.T) {
		_, err := client.Payouts.Create(ctx, &PayoutRequest{
			Amount: "999999", Currency: "USDT", Network: Ptr("tron"), Address: liveAddress,
			OrderID: fmt.Sprintf("sdk-go-big-%d", time.Now().UnixNano()),
		}, WithMaxRetries(0))
		refusal := requireCode(t, err, "payout.insufficient_funds")
		if refusal.RequestID == "" {
			t.Fatal("a refusal must carry a request id to quote to support")
		}
	})

	t.Run("registers an endpoint and delivers a signed test webhook", func(t *testing.T) {
		hook := os.Getenv("OBLODAI_LIVE_HOOK_URL")
		if hook == "" {
			hook = "http://127.0.0.1:8096/hook"
		}
		endpoint, err := client.Webhooks.Register(ctx, &RegisterWebhookRequest{URL: hook})
		if err != nil || endpoint.Secret == nil || *endpoint.Secret == "" {
			t.Fatalf("Webhooks.Register must return the signing secret once: %v", err)
		}
		if os.Getenv("OBLODAI_LIVE_HOOK_URL") == "" {
			t.Skip("no reachable receiver: delivery itself is covered by the recorded deliveries")
		}
		result, err := client.Webhooks.SendTestPayment(ctx, &TestWebhookKindRequest{
			URLCallback: hook, Currency: Ptr("USDT"), Network: Ptr("tron"), Status: Ptr("paid"),
		})
		if err != nil || !result.Ok {
			t.Fatalf("Webhooks.SendTestPayment: %v %+v", err, result)
		}
	})
}
