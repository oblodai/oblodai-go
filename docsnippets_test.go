package oblodai_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/oblodai/oblodai-go"
	"github.com/oblodai/oblodai-go/webhooks"
)

// Every Go snippet in README.md, AGENTS.md and MIGRATION-1.3.md, compiled. Documentation that does
// not compile is documentation that sends a reader down a path this SDK does not have.

func snippetQuickstart() {
	client, err := oblodai.New()
	if err != nil {
		panic(err)
	}
	invoice, err := client.Payments.Create(context.Background(), oblodai.PaymentParams{
		Amount:      "25",
		Currency:    "USDT",
		Network:     oblodai.NetworkTron,
		OrderID:     "order-1001",
		URLCallback: "https://shop.example/oblodai/webhook",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(invoice.URL, invoice.Address, invoice.Status)
	_, _ = client.Payments.Create(context.Background(),
		oblodai.PaymentParams{Amount: "25", Currency: "USD", ToCurrency: "USDT"})
}

func snippetCredentials(publicID, secret, payoutPublicID, payoutSecret string) {
	_, _ = oblodai.New(
		oblodai.WithCredentials(publicID, secret),
		oblodai.WithPayoutCredentials(payoutPublicID, payoutSecret),
	)
}

func snippetLists(ctx context.Context, client *oblodai.Client) error {
	fifty := 50
	page, err := client.Payments.History(ctx, oblodai.PaymentHistoryParams{Limit: &fifty}).Page()
	if err != nil {
		return err
	}
	fmt.Println(page.Items, page.Paginate.Total, page.Paginate.PerPage, page.Paginate.Offset, page.Paginate.HasPages)

	pager := client.Payouts.History(ctx, oblodai.PayoutHistoryParams{Status: oblodai.PayoutStatusConfirmed}).Pager()
	for pager.Next() {
		fmt.Println(pager.Item().UUID)
	}
	if err := pager.Err(); err != nil {
		return err
	}
	_, err = client.Payouts.History(ctx, oblodai.PayoutHistoryParams{Kind: "refund"}).All(1000)
	return err
}

func snippetErrors(ctx context.Context, client *oblodai.Client, params oblodai.PayoutParams) error {
	payout, err := client.Payouts.Create(ctx, params)
	if err != nil {
		var apiErr *oblodai.Error
		if !errors.As(err, &apiErr) {
			return err
		}
		switch apiErr.Code {
		case "payout.insufficient_funds", "payout.funds_maturing":
			return scheduleRetry(apiErr.RetryAfter)
		default:
			return err
		}
	}
	_ = payout
	return nil
}

func scheduleRetry(*int) error { return nil }

func snippetWebhookHandler(w http.ResponseWriter, r *http.Request) {
	delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: os.Getenv("OBLODAI_WEBHOOK_SECRET")})
	if err != nil {
		http.Error(w, "bad signature", http.StatusBadRequest)
		return
	}
	if delivery.IsTest {
		w.WriteHeader(http.StatusOK)
		return
	}
	switch event := delivery.Event.(type) {
	case *oblodai.PaymentEvent:
		if event.Status == oblodai.PaymentStatusPaid {
			markOrderPaid(event.OrderID, delivery.ID)
		}
	case *oblodai.PayoutEvent:
	case *oblodai.WalletEvent:
	}
	w.WriteHeader(http.StatusOK)
}

func markOrderPaid(*string, string) {}

func snippetAgentsWebhook(r *http.Request, endpointSecret string) {
	delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: endpointSecret})
	_, _ = delivery, err
}

func snippetMigration(ctx context.Context, id, secret string) {
	client, err := oblodai.New(oblodai.WithCredentials(id, secret))
	if err != nil {
		return
	}
	_, _ = client.Payments.Create(ctx, oblodai.PaymentParams{})
	_, _ = client.Payments.Info(ctx, oblodai.PaymentInfoParams{UUID: "u"})
	_, _ = client.Payments.Info(ctx, oblodai.PaymentInfoParams{OrderID: "ref"})
	_, _ = client.Merchants.Create(ctx, oblodai.MerchantsParams{Email: "a@b.c", Name: "Acme"})
	_, _ = client.Merchants.CreateSandbox(ctx, "m1")
	_, _ = oblodai.New(oblodai.WithBaseURL("http://127.0.0.1:8095"), oblodai.WithInsecureBaseURL(true))
	_ = oblodai.NetworkTron
	_ = oblodai.PaymentStatusPaid
	_ = oblodai.PayoutStatusConfirmed
	_ = oblodai.FeeBearerMerchant
	_ = oblodai.ReservedHeaders()
	_ = oblodai.IsKnownEvent(&oblodai.PaymentEvent{})
}

func TestDocumentationSnippetsCompile(t *testing.T) {
	// Referencing them is enough: the compiler has checked every line above.
	_ = snippetQuickstart
	_ = snippetCredentials
	_ = snippetLists
	_ = snippetErrors
	_ = snippetWebhookHandler
	_ = snippetAgentsWebhook
	_ = snippetMigration
}
