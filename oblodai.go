// Package oblodai is the official Go client for the Oblodai crypto payment gateway.
//
// One client per API key; it is safe to share across goroutines:
//
//	client, err := oblodai.New(oblodai.WithCredentials("oblodai_…", "oblodai_live_…"))
//	invoice, err := client.Payments.Create(ctx, oblodai.PaymentParams{
//		Amount: "25", Currency: "USDT", Network: oblodai.NetworkTron, OrderID: "order-1",
//	})
//
// Everything the client knows about the API — routes, vocabularies, request bodies — is generated
// from the contract snapshot in contract/contract.json, which the core exports from its own
// conformance table. Webhook verification lives in the standalone sub-package
// github.com/oblodai/oblodai-go/webhooks and needs no client and no API key.
//
// Three rules that matter more than the rest:
//
//   - Amounts are decimal strings ("25", "10.000000"), never floats. Use AddAmounts and
//     CompareAmounts instead of parsing them.
//   - Errors are always *Error: check Code (a stable family.reason string) and Retryable, not the
//     message. The client has already retried whatever was safe to retry.
//   - One API key signs everything: payments, payouts, settings, documents. Only merchant
//     provisioning is different — it takes an admin token (WithAdminToken).
package oblodai

//go:generate go run ./internal/codegen

// Version is the SDK release, sent in User-Agent.
const Version = "1.3.0"

// DefaultBaseURL is the production API origin.
const DefaultBaseURL = "https://api.oblodai.com"

// Money is a decimal amount rendered at the asset's own scale ("10.000000" for USDT). It is a
// string on purpose: USDT has 6 decimals, BTC 8 and ETH 18, and float64 cannot hold them exactly.
type Money = string

// Timestamp is an RFC 3339 instant in UTC ("2026-08-25T20:58:55Z").
type Timestamp = string

// Auth is the credential a route's gate expects. It mirrors the core's conformance table.
type Auth string

const (
	// AuthPublic routes are unsigned: payer-facing pages and the currency catalog.
	AuthPublic Auth = "public"
	// AuthKey routes are signed with the merchant's API key — every route that touches merchant
	// money or configuration, payments and payouts alike.
	AuthKey Auth = "key"
	// AuthOnboard routes are unsigned merchant provisioning; a self-hosted gateway gates them with
	// an admin token.
	AuthOnboard Auth = "onboard"
)

// ListKind tells how a route paginates.
type ListKind string

const (
	// ListNone is a route that returns a single object.
	ListNone ListKind = ""
	// ListPaged returns {items, paginate}.
	ListPaged ListKind = "paged"
	// ListPlain returns {items} without a paginate block: the core caps it by catalog size.
	ListPlain ListKind = "plain"
)

// Route is one endpoint of the core's merchant surface.
type Route struct {
	// Method is GET or POST.
	Method string
	// Path is the path template; {name} segments are filled from path parameters.
	Path string
	// Auth is the credential the route's gate expects.
	Auth Auth
	// Idempotent reports whether the core deduplicates the route by Idempotency-Key. The client
	// generates a key for such routes and reuses it across retries.
	Idempotent bool
	// Safe reports a read-only route: repeating it cannot duplicate a side effect.
	Safe bool
	// Bare routes answer outside the JSON envelope (PDF and CSV documents).
	Bare bool
	// List is the pagination shape of the result, if any.
	List ListKind
}

// Key is the route's registry key, "POST /v1/payment".
func (r Route) Key() string { return r.Method + " " + r.Path }

// Paginate is the pagination block of a list result.
type Paginate struct {
	// Total is how many items match the filter across all pages.
	Total int `json:"total"`
	// PerPage is the page size the core applied.
	PerPage int `json:"per_page"`
	// Offset is the offset this page starts at.
	Offset int `json:"offset"`
	// HasPages reports whether another page follows.
	HasPages bool `json:"has_pages"`
}

// Page is one page of a paged list route.
type Page[T any] struct {
	Items    []T      `json:"items"`
	Paginate Paginate `json:"paginate"`
}

// FileResult is the body of a bare route: a generated PDF or CSV document.
type FileResult struct {
	// Bytes is the document itself.
	Bytes []byte
	// ContentType is the response media type ("application/pdf").
	ContentType string
	// Filename comes from Content-Disposition when the core sets one.
	Filename string
}
