// Package oblodai is the official Go client for the Oblodai crypto payment gateway.
//
// One client per API key; it is safe to share across goroutines:
//
//	client, err := oblodai.New(oblodai.WithCredentials("oblodai_…", "oblodai_live_…"))
//	invoice, err := client.Payments.Create(ctx, &oblodai.PaymentRequest{
//		Amount: "25", Currency: "USDT", OrderID: oblodai.Ptr("order-1"),
//	})
//
// The API surface — every resource service, method, request and response model, enumeration and
// route — is generated from the gateway's OpenAPI contract into the zz_generated_*.go files; the
// rest of the package is the hand-written runtime: signing, retries, errors, pagination, waiters
// for long-running jobs. Webhook verification lives in the standalone sub-package
// github.com/oblodai/oblodai-go/v2/webhooks and needs no client and no API key.
//
// Three rules that matter more than the rest:
//
//   - Amounts are Decimal strings ("25", "10.000000"), never floats: a float64 does not fit the
//     type, and a JSON number in an amount is refused with sdk.float_amount. Use AddAmounts and
//     CompareAmounts instead of parsing them.
//   - Errors are always *Error: check Code (a stable family.reason string) and Retryable, not the
//     message. The client has already retried whatever was safe to retry.
//   - One API key signs everything: payments, payouts, settings, documents. Only merchant
//     provisioning is different — it takes an admin token (WithAdminToken).
package oblodai

// Version is the SDK release, sent in User-Agent.
const Version = "2.0.0"

// DefaultBaseURL is the production API origin.
const DefaultBaseURL = "https://api.oblodai.com"

// Credentials a route's gate expects (RouteSpec.Auth).
const (
	// AuthPublic routes are unsigned: payer-facing pages and the currency catalog.
	AuthPublic = "public"
	// AuthKey routes are signed with the merchant's API key — every route that touches merchant
	// money or configuration.
	AuthKey = "key"
	// AuthOnboard routes are unsigned merchant provisioning; a self-hosted gateway gates them with
	// an admin token.
	AuthOnboard = "onboard"
)

// ListPaged is RouteSpec.ListKind of a route that returns {items, paginate}.
const ListPaged = "paged"

// RouteSpec is one operation of the API, as the generated Routes table lists it.
type RouteSpec struct {
	// OperationID is the operation's OpenAPI operationId, the key of Routes.
	OperationID string
	// Method is GET or POST.
	Method string
	// Path is the path template; {name} segments are filled from path parameters.
	Path string
	// Auth is the credential the route's gate expects: AuthKey, AuthPublic or AuthOnboard.
	Auth string
	// Idempotent reports whether the core deduplicates the route by HeaderIdempotencyKey. The client
	// generates a key for such routes and reuses it across retries.
	Idempotent bool
	// Safe reports a route without side effects: repeating it cannot duplicate anything.
	Safe bool
	// Bare routes answer outside the JSON envelope (PDF and CSV documents).
	Bare bool
	// ListKind is ListPaged for a paged list, else empty.
	ListKind string
	// BodyIdempotencyKey: the route's request body carries its own idempotency_key field, so
	// WithIdempotencyKey fills that field and no HeaderIdempotencyKey is sent.
	BodyIdempotencyKey bool
}

// Key is the route's request line, "POST /v1/payment".
func (r RouteSpec) Key() string { return r.Method + " " + r.Path }

// Page is one page of a paged list.
type Page[T any] struct {
	Items    []T        `json:"items"`
	Paginate Pagination `json:"paginate"`
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

// Ptr returns a pointer to v — for the optional fields of request models:
//
//	&oblodai.PaymentRequest{Amount: "25", Currency: "USDT", OrderID: oblodai.Ptr("order-1")}
func Ptr[T any](v T) *T { return &v }
