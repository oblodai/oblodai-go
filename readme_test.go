package oblodai_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oblodai/oblodai-go/v2"
	"github.com/oblodai/oblodai-go/v2/internal/fakeapi"
	"github.com/oblodai/oblodai-go/v2/webhooks"
)

// The documentation's Go code is this file. The regions between a "// snippet:<name>" and an
// "// endsnippet" comment below are compiled here, executed against a fake gateway by
// TestReadmeSnippetsRun, and are the source of truth for every ```go block of README.md,
// README.ru.md (in order), AGENTS.md and MIGRATION-2.0.md (each block must be one of them):
// TestDocsMatchSnippets fails when a document drifts from the code that runs.

func snippetCredentials(publicID, secret string) *oblodai.Client {
	// snippet:credentials
	client, err := oblodai.New(oblodai.WithCredentials(publicID, secret))
	// endsnippet
	if err != nil {
		log.Fatal(err)
	}
	return client
}

func snippetQuickstartPayment(ctx context.Context) *oblodai.Client {
	// snippet:quickstart-payment
	client, err := oblodai.New() // OBLODAI_PUBLIC_ID / OBLODAI_SECRET from the environment
	if err != nil {
		log.Fatal(err)
	}
	invoice, err := client.Payments.Create(ctx, &oblodai.PaymentRequest{
		Amount:      "25",                      // amounts are decimal strings, never floats
		Currency:    "USDT",                    // what you price in: a fiat (USD, EUR, …) or a crypto asset
		Network:     oblodai.Ptr("tron"),       // omit to let the payer choose on the pay page
		OrderID:     oblodai.Ptr("order-1001"), // your reference
		URLCallback: oblodai.Ptr("https://shop.example/oblodai/webhook"),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(invoice.URL, invoice.Address, invoice.Status) // "created"
	// endsnippet
	return client
}

func snippetQuickstartPayout(ctx context.Context, client *oblodai.Client) {
	// snippet:quickstart-payout
	payout, err := client.Payouts.Create(ctx, &oblodai.PayoutRequest{
		Address:  "TQn9Y2khEsLJW1ChVWFMSMeRDow5KNbBav",
		Amount:   "10",
		Currency: "USDT",
		Network:  oblodai.Ptr("tron"),
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
	if _, err := sandbox.Sandbox.Faucet(ctx, &oblodai.FaucetRequest{Asset: "USDT", Amount: "1000"}); err != nil {
		log.Fatal(err)
	}
	invoice, err := sandbox.Payments.Create(ctx, &oblodai.PaymentRequest{
		Amount: "25", Currency: "USDT", Network: oblodai.Ptr("tron"), OrderID: oblodai.Ptr("sandbox-1"),
	})
	if err != nil {
		log.Fatal(err)
	}
	// No amount pays exactly what is due; repeating a txid adds confirmations instead of paying twice.
	deposit, err := sandbox.Sandbox.SimulateDeposit(ctx, &oblodai.SimulateDepositRequest{InvoiceID: invoice.UUID})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(deposit.Txid, deposit.Confirmations)
	// endsnippet
}

func snippetLists(ctx context.Context, client *oblodai.Client) error {
	// snippet:lists
	for payment, err := range client.Payments.ListHistory(ctx, &oblodai.HistoryRequest{Status: oblodai.Ptr("paid")}).Items() {
		if err != nil {
			return err
		}
		fmt.Println(payment.UUID, payment.Amount)
	}
	for page, err := range client.Payouts.ListHistory(ctx, &oblodai.HistoryRequest{Limit: oblodai.Ptr[int64](100)}).ByPage() {
		if err != nil {
			return err
		}
		fmt.Println(len(page.Items), page.Paginate.Total, page.Paginate.HasPages)
	}
	// endsnippet
	return nil
}

func snippetJobs(ctx context.Context, client *oblodai.Client, payouts []oblodai.PayoutRequest) error {
	// snippet:jobs
	accepted, err := client.Batches.CreatePayout(ctx, &oblodai.PayoutBatchRequest{Payouts: payouts})
	if err != nil {
		return err
	}
	info, err := client.BatchJob(accepted.BatchID).Wait(ctx) // polls until completed or stopped
	if err != nil {
		return err
	}
	fmt.Println(info.Status, info.Succeeded, info.Failed)

	export, err := client.Documents.CreateJob(ctx, &oblodai.DocumentJobRequest{Kind: oblodai.DocumentJobKindLedger})
	if err != nil {
		return err
	}
	job := client.DocumentJob(export.JobID)
	view, err := job.Wait(ctx, oblodai.WithWaitTimeout(10*time.Minute))
	if err != nil {
		return err
	}
	if view.Status == oblodai.DocumentJobStatusDone {
		file, err := job.Download(ctx)
		if err != nil {
			return err
		}
		fmt.Println(file.ContentType, len(file.Bytes))
	}
	// endsnippet
	return nil
}

var paidOrders []string

func markOrderPaid(orderID, eventID string) { paidOrders = append(paidOrders, orderID+"/"+eventID) }

func snippetWebhookHandler(w http.ResponseWriter, r *http.Request) {
	// snippet:webhooks
	delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: os.Getenv("OBLODAI_WEBHOOK_SECRET")})
	if err != nil {
		http.Error(w, "bad signature", http.StatusBadRequest) // 4xx only for a failed verification
		return
	}
	if delivery.IsTest { // a rehearsal delivery: signed like a live one, but no money moved
		w.WriteHeader(http.StatusOK)
		return
	}
	if payment := delivery.Event.Payment; payment != nil && oblodai.IsPaymentPaid(payment.Status) {
		markOrderPaid(payment.OrderID, delivery.EventID) // EventID is stable for one state
	}
	w.WriteHeader(http.StatusOK)
	// endsnippet
}

var retries []*int

func scheduleRetry(after *int) error {
	retries = append(retries, after)
	return nil
}

func snippetErrors(ctx context.Context, client *oblodai.Client, params *oblodai.PayoutRequest) error {
	// snippet:errors
	payout, err := client.Payouts.Create(ctx, params)
	if err != nil {
		var apiErr *oblodai.Error
		if !errors.As(err, &apiErr) {
			return err
		}
		log.Println(apiErr) // [payout.insufficient_funds] not enough USDT (request_id=…)
		switch apiErr.Code {
		case "payout.insufficient_funds", "payout.funds_maturing":
			return scheduleRetry(apiErr.RetryAfter) // retryable — the balance may still arrive
		default:
			return err // the client already retried whatever was safe to retry
		}
	}
	fmt.Println(payout.UUID)
	// endsnippet
	return nil
}

func snippetOptions(ctx context.Context, client *oblodai.Client) (*oblodai.Client, error) {
	// snippet:options
	var raw *oblodai.RawResponse
	balance, err := client.Account.GetBalance(ctx,
		oblodai.WithRequestTimeout(5*time.Second), // per attempt
		oblodai.WithMaxRetries(0),                 // this call only
		oblodai.WithRequestID("checkout-42"),      // X-Request-ID: joins your logs with ours
		oblodai.WithExtraHeaders(map[string]string{"X-Trace": "t-1"}),
		oblodai.WithRawResponse(&raw),
	)
	if err != nil {
		return nil, err
	}
	fmt.Println(balance, raw.StatusCode, raw.RequestID)

	reports, err := client.WithOptions(oblodai.WithTimeout(2*time.Minute), oblodai.WithHooks(oblodai.Hooks{
		OnResponse: func(r oblodai.ResponseInfo) {
			log.Println(r.Request.OperationID, r.StatusCode, r.Elapsed)
		},
	}))
	// endsnippet
	return reports, err
}

// Every snippet runs against the fake gateway, the way a reader pastes it.
func TestReadmeSnippetsRun(t *testing.T) {
	api := fakeapi.New(t, map[string]any{
		"createPayment":          map[string]any{"uuid": "u1", "status": "created", "url": "https://pay.example/u1"},
		"createPayout":           []any{map[string]any{"uuid": "p1", "status": "pending"}, fakeapi.Fail{Status: 400, Code: "payout.insufficient_funds", Message: "not enough USDT", Retryable: true}},
		"sandboxSimulateDeposit": map[string]any{"invoice_id": "u1", "txid": "sandboxtx_1", "confirmations": 20},
		"createPayoutBatch":      map[string]any{"batch_id": "b1", "status": "pending"},
		"getBatchInfo":           map[string]any{"batch_id": "b1", "status": "completed", "succeeded": 2},
		"createDocumentJob":      map[string]any{"job_id": "j1", "status": "queued"},
		"getDocumentJob":         map[string]any{"job_id": "j1", "status": "done"},
	})
	api.Env(t)
	ctx := context.Background()

	snippetCredentials("oblodai_pk", "oblodai_live_sk")
	client := snippetQuickstartPayment(ctx)
	snippetQuickstartPayout(ctx, client)
	snippetSandbox(ctx, "test_oblodai_pk", "oblodai_test_sk")
	if err := snippetLists(ctx, client); err != nil {
		t.Fatalf("lists: %v", err)
	}
	if err := snippetJobs(ctx, client, []oblodai.PayoutRequest{{Address: "T", Amount: "1", Currency: "USDT", OrderID: "o"}}); err != nil {
		t.Fatalf("jobs: %v", err)
	}
	if err := snippetErrors(ctx, client, &oblodai.PayoutRequest{Address: "T", Amount: "1", Currency: "USDT", OrderID: "o2"}); err != nil || len(retries) != 1 {
		t.Fatalf("errors: %v, %d retries scheduled", err, len(retries))
	}
	reports, err := snippetOptions(ctx, client)
	if err != nil || reports == nil {
		t.Fatalf("options: %v", err)
	}

	want := []string{
		"createPayment", "createPayout", "sandboxFaucet", "createPayment", "sandboxSimulateDeposit",
		"listPaymentHistory", "listPayoutHistory", "createPayoutBatch", "getBatchInfo",
		"createDocumentJob", "getDocumentJob", "downloadDocumentJobFile",
		"createPayout", "createPayout", "createPayout", // retryable: the client retried twice, one key
		"getBalance",
	}
	if got := api.Operations(); !slices.Equal(got, want) {
		t.Fatalf("the snippets called\n%v\nwant\n%v", got, want)
	}
	if got := api.Requests()[len(want)-1].Header.Get(oblodai.HeaderRequestID); got != "checkout-42" {
		t.Fatalf("X-Request-ID %q", got)
	}

	// The webhook handler, fed a delivery signed with the secret it reads.
	t.Setenv("OBLODAI_WEBHOOK_SECRET", "whsec")
	body := `{"type":"payment","uuid":"u1","order_id":"order-1001","status":"paid","sequence":1}`
	now := time.Now().Unix()
	r := httptest.NewRequest(http.MethodPost, "/hook", strings.NewReader(body))
	r.Header.Set(webhooks.HeaderTimestamp, strconv.FormatInt(now, 10))
	r.Header.Set(webhooks.HeaderSignature, oblodai.SignWebhook("whsec", now, []byte(body)))
	r.Header.Set(webhooks.HeaderEventID, "ev-1")
	w := httptest.NewRecorder()
	snippetWebhookHandler(w, r)
	if w.Code != http.StatusOK || !slices.Equal(paidOrders, []string{"order-1001/ev-1"}) {
		t.Fatalf("webhook handler: %d, paid %v", w.Code, paidOrders)
	}
}

const (
	snippetSource = "readme_test.go"
	markerStart   = "// snippet:"
	markerEnd     = "// endsnippet"
)

type snippet struct{ name, body string }

type codeBlock struct {
	lang string
	body string
	line int
}

func TestDocsMatchSnippets(t *testing.T) {
	snippets := compiledSnippets(t)
	if len(snippets) == 0 {
		t.Fatal("no snippet markers found in " + snippetSource)
	}
	for _, path := range []string{"README.md", "README.ru.md"} {
		blocks := goBlocks(t, path)
		if len(blocks) != len(snippets) {
			t.Fatalf("%s: %d Go blocks, but %s marks %d snippets", path, len(blocks), snippetSource, len(snippets))
		}
		for i, block := range blocks {
			if got := stripImports(block.body); got != snippets[i].body {
				t.Errorf("%s:%d: the block does not match snippet %q\n--- document ---\n%s\n--- compiled ---\n%s",
					path, block.line, snippets[i].name, got, snippets[i].body)
			}
		}
	}
	for _, path := range []string{"AGENTS.md", "MIGRATION-2.0.md"} {
		for _, block := range goBlocks(t, path) {
			found := false
			for _, s := range snippets {
				found = found || stripImports(block.body) == s.body
			}
			if !found {
				t.Errorf("%s:%d: a Go block that is none of the compiled snippets:\n%s", path, block.line, block.body)
			}
		}
	}
}

func TestRussianReadmeMirrorsEnglish(t *testing.T) {
	english, russian := fencedBlocks(t, "README.md"), fencedBlocks(t, "README.ru.md")
	if len(english) != len(russian) {
		t.Fatalf("README.md has %d code blocks, README.ru.md has %d", len(english), len(russian))
	}
	for i := range english {
		if english[i].lang != russian[i].lang || english[i].body != russian[i].body {
			t.Errorf("code block %d differs between the READMEs (a translation must not translate code)\n--- README.md:%d ---\n%s\n--- README.ru.md:%d ---\n%s",
				i+1, english[i].line, english[i].body, russian[i].line, russian[i].body)
		}
	}
	if got, want := headings(t, "README.ru.md"), headings(t, "README.md"); len(got) != len(want) {
		t.Errorf("README.ru.md has %d H2 sections, README.md has %d: the translation must carry every section", len(got), len(want))
	}
}

// compiledSnippets reads the marked regions of this file, dedented by the one tab a function body
// adds, in source order.
func compiledSnippets(t *testing.T) []snippet {
	t.Helper()
	var out []snippet
	var current []string
	name, inside := "", false
	for _, line := range strings.Split(readFile(t, snippetSource), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, markerStart) && !strings.Contains(trimmed, `"`):
			inside, name, current = true, strings.TrimPrefix(trimmed, markerStart), nil
		case strings.HasPrefix(trimmed, markerEnd) && !strings.Contains(trimmed, `"`) && inside:
			out = append(out, snippet{name, strings.Join(current, "\n")})
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

func goBlocks(t *testing.T, path string) []codeBlock {
	var out []codeBlock
	for _, block := range fencedBlocks(t, path) {
		if block.lang == "go" {
			out = append(out, block)
		}
	}
	return out
}

// fencedBlocks returns the fenced code blocks of a Markdown file, in order.
func fencedBlocks(t *testing.T, path string) []codeBlock {
	t.Helper()
	var out []codeBlock
	var current []string
	lang, start, inside := "", 0, false
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

// stripImports drops the import line a document block carries for the reader's benefit; the
// compiled snippet cannot hold one inside a function body.
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
