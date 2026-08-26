package oblodai_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/oblodai/oblodai-go"
	"github.com/oblodai/oblodai-go/webhooks"
)

// Every Go snippet in README.md, README.ru.md, AGENTS.md and MIGRATION-1.3.md, compiled.
// Documentation that does not compile is documentation that sends a reader down a path this SDK
// does not have.
//
// The snippets printed in the two READMEs are not written twice: the regions between a
// "// snippet:<name>" and an "// endsnippet" comment below are the source of truth, and
// TestReadmeSnippetsMatchCompiledCode fails when a README drifts from them.

func snippetCredentials(publicID, secret string) *oblodai.Client {
	// snippet:credentials
	client, err := oblodai.New(oblodai.WithCredentials(publicID, secret))
	// endsnippet
	if err != nil {
		log.Fatal(err)
	}
	return client
}

func snippetQuickstartPayment(ctx context.Context) {
	// snippet:quickstart-payment
	client, err := oblodai.New() // OBLODAI_PUBLIC_ID / OBLODAI_SECRET from the environment
	if err != nil {
		log.Fatal(err)
	}
	invoice, err := client.Payments.Create(ctx, oblodai.PaymentParams{
		Amount:      "25",                // amounts are decimal strings, never floats
		Currency:    "USDT",              // what you price in: a fiat (USD, EUR, …) or a crypto asset
		Network:     oblodai.NetworkTron, // omit to let the payer choose the network on the pay page
		OrderID:     "order-1001",        // your reference; the invoice is idempotent per order_id
		URLCallback: "https://shop.example/oblodai/webhook",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(invoice.URL, invoice.Address, invoice.Status) // "created"
	// endsnippet

	// Pricing in fiat, mentioned in prose next to the snippet.
	_, _ = client.Payments.Create(ctx,
		oblodai.PaymentParams{Amount: "25", Currency: "USD", ToCurrency: "USDT"})
}

func snippetQuickstartPayout(ctx context.Context, client *oblodai.Client) {
	// snippet:quickstart-payout
	payout, err := client.Payouts.Create(ctx, oblodai.PayoutParams{
		Address:  "TQn9Y2khEsLJW1ChVWFMSMeRDow5KNbBav",
		Amount:   "10",
		Currency: "USDT",
		Network:  oblodai.NetworkTron,
		OrderID:  "payout-1", // your reference; the payout is idempotent per order_id
	}, oblodai.WithIdempotencyKey("payout-1"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(payout.UUID, payout.Status) // "pending" → … → "confirmed"
	// endsnippet
}

func snippetSandbox(ctx context.Context, testPublicID, testSecret string) {
	// snippet:sandbox
	sandbox, err := oblodai.New(oblodai.WithCredentials(testPublicID, testSecret)) // a test_oblodai_… pair
	if err != nil {
		log.Fatal(err)
	}
	if _, err := sandbox.Sandbox.Faucet(ctx, oblodai.SandboxFaucetParams{Asset: "USDT", Amount: "1000"}); err != nil {
		log.Fatal(err)
	}
	invoice, err := sandbox.Payments.Create(ctx, oblodai.PaymentParams{
		Amount: "25", Currency: "USDT", Network: oblodai.NetworkTron, OrderID: "sandbox-1",
	})
	if err != nil {
		log.Fatal(err)
	}
	// No amount pays exactly what is due; repeating a txid adds confirmations instead of paying twice.
	deposit, err := sandbox.Sandbox.Deposit(ctx, oblodai.SandboxDepositParams{InvoiceID: invoice.UUID})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(deposit.TxID, deposit.Confirmations)
	// endsnippet

	// The rehearsal delivery and the wipe, mentioned in prose next to the snippet.
	_, _ = sandbox.Webhooks.Test(ctx, oblodai.WebhookKindPayment, oblodai.WebhookTestParams{
		URLCallback: "https://shop.example/oblodai/webhook",
	})
	_, _ = sandbox.Sandbox.Reset(ctx)
}

func snippetLists(ctx context.Context, client *oblodai.Client) error {
	fifty := 50
	// snippet:lists
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
	refunds, err := client.Payouts.History(ctx, oblodai.PayoutHistoryParams{Kind: "refund"}).All(1000)
	// endsnippet
	_ = refunds
	return err
}

func snippetWebhookHandler(w http.ResponseWriter, r *http.Request) {
	// snippet:webhook-receiver
	delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: os.Getenv("OBLODAI_WEBHOOK_SECRET")})
	if err != nil {
		http.Error(w, "bad signature", http.StatusBadRequest) // 401/4xx only for a signature failure
		return
	}
	if delivery.IsTest { // a rehearsal delivery: signed like a live one, but no money moved
		w.WriteHeader(http.StatusOK)
		return
	}
	switch event := delivery.Event.(type) {
	case *oblodai.PaymentEvent:
		if event.Status == oblodai.PaymentStatusPaid {
			markOrderPaid(event.OrderID, delivery.ID) // delivery.ID is stable across retries
		}
	case *oblodai.PayoutEvent:
	case *oblodai.WalletEvent:
	}
	w.WriteHeader(http.StatusOK)
	// endsnippet
}

func markOrderPaid(*string, string) {}

func snippetErrors(ctx context.Context, client *oblodai.Client, params oblodai.PayoutParams) error {
	// snippet:errors
	payout, err := client.Payouts.Create(ctx, params)
	if err != nil {
		var apiErr *oblodai.Error
		if !errors.As(err, &apiErr) {
			return err
		}
		switch apiErr.Code {
		case "payout.insufficient_funds", "payout.funds_maturing":
			return scheduleRetry(apiErr.RetryAfter) // retryable — the balance may still arrive
		default:
			return err // the client already retried whatever was safe to retry
		}
	}
	// endsnippet
	_ = payout
	return nil
}

func scheduleRetry(*int) error { return nil }

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
	_ = snippetQuickstartPayment
	_ = snippetQuickstartPayout
	_ = snippetCredentials
	_ = snippetSandbox
	_ = snippetLists
	_ = snippetWebhookHandler
	_ = snippetErrors
	_ = snippetAgentsWebhook
	_ = snippetMigration
}

// The two READMEs must show the code this file compiles — and show the same code as each other.

const (
	englishReadme = "README.md"
	russianReadme = "README.ru.md"
	snippetSource = "docsnippets_test.go"
	markerStart   = "// snippet:"
	markerEnd     = "// endsnippet"
)

type codeBlock struct {
	lang string
	body string
	line int
}

func TestReadmeSnippetsMatchCompiledCode(t *testing.T) {
	snippets := compiledSnippets(t)
	if len(snippets) == 0 {
		t.Fatal("no snippet markers found in " + snippetSource)
	}
	for _, path := range []string{englishReadme, russianReadme} {
		var goBlocks []codeBlock
		for _, block := range fencedBlocks(t, path) {
			if block.lang == "go" {
				goBlocks = append(goBlocks, block)
			}
		}
		if len(goBlocks) != len(snippets) {
			t.Fatalf("%s: %d Go blocks, but %s marks %d snippets", path, len(goBlocks), snippetSource, len(snippets))
		}
		for i, block := range goBlocks {
			want := snippets[i]
			if got := stripImports(block.body); got != want.body {
				t.Errorf("%s:%d: the block does not match snippet %q in %s\n--- README ---\n%s\n--- compiled ---\n%s",
					path, block.line, want.name, snippetSource, got, want.body)
			}
		}
	}
}

func TestRussianReadmeMirrorsEnglish(t *testing.T) {
	english := fencedBlocks(t, englishReadme)
	russian := fencedBlocks(t, russianReadme)
	if len(english) != len(russian) {
		t.Fatalf("%s has %d code blocks, %s has %d", englishReadme, len(english), russianReadme, len(russian))
	}
	for i := range english {
		if english[i].lang != russian[i].lang || english[i].body != russian[i].body {
			t.Errorf("code block %d differs between the READMEs (a translation must not translate code)\n--- %s:%d ---\n%s\n--- %s:%d ---\n%s",
				i+1, englishReadme, english[i].line, english[i].body, russianReadme, russian[i].line, russian[i].body)
		}
	}
	if got, want := headings(t, russianReadme), headings(t, englishReadme); len(got) != len(want) {
		t.Errorf("%s has %d H2 sections, %s has %d: the translation must carry every section",
			russianReadme, len(got), englishReadme, len(want))
	}
}

// compiledSnippets reads the marked regions of this file, dedented by the one tab a function body
// adds, in source order.
func compiledSnippets(t *testing.T) []struct{ name, body string } {
	t.Helper()
	var out []struct{ name, body string }
	var current []string
	name := ""
	inside := false
	for _, line := range strings.Split(readFile(t, snippetSource), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, markerStart):
			inside, name, current = true, strings.TrimPrefix(trimmed, markerStart), nil
		case strings.HasPrefix(trimmed, markerEnd) && inside:
			out = append(out, struct{ name, body string }{name, strings.Join(current, "\n")})
			inside = false
		case inside:
			current = append(current, strings.TrimPrefix(line, "\t"))
		}
	}
	if inside {
		t.Fatalf("snippet %q is never closed with %q", name, markerEnd)
	}
	return out
}

// fencedBlocks returns the fenced code blocks of a Markdown file, in order.
func fencedBlocks(t *testing.T, path string) []codeBlock {
	t.Helper()
	var out []codeBlock
	var current []string
	lang := ""
	start := 0
	inside := false
	for i, line := range strings.Split(readFile(t, path), "\n") {
		if !strings.HasPrefix(line, "```") {
			if inside {
				current = append(current, line)
			}
			continue
		}
		if inside {
			out = append(out, codeBlock{lang: lang, body: strings.Join(current, "\n"), line: start})
			inside, current = false, nil
			continue
		}
		inside, lang, start = true, strings.TrimSpace(strings.TrimPrefix(line, "```")), i+1
	}
	if inside {
		t.Fatalf("%s: a fenced code block is never closed", path)
	}
	return out
}

// stripImports drops the import line a README block carries for the reader's benefit; the compiled
// snippet cannot hold one inside a function body.
func stripImports(body string) string {
	lines := strings.Split(body, "\n")
	for len(lines) > 0 && (strings.HasPrefix(lines[0], "import ") || strings.TrimSpace(lines[0]) == "") {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}

func headings(t *testing.T, path string) []string {
	t.Helper()
	var out []string
	inside := false
	for _, line := range strings.Split(readFile(t, path), "\n") {
		if strings.HasPrefix(line, "```") {
			inside = !inside
			continue
		}
		if !inside && strings.HasPrefix(line, "## ") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "## ")))
		}
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}
