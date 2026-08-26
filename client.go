package oblodai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"time"
)

// Client is the Oblodai API client. One instance per key pair; it is safe to share across
// goroutines and should be created once and reused, so connections and the learned clock offset
// are shared.
type Client struct {
	transport *transport

	// Payments creates and looks up invoices, and serves the payer-facing checkout endpoints.
	Payments *PaymentsService
	// Refunds refunds paid invoices and resolves underpaid ones.
	Refunds *RefundsService
	// Payouts sends funds to external addresses.
	Payouts *PayoutsService
	// PayoutLinks mints claimable cheques backed by reserved funds.
	PayoutLinks *PayoutLinksService
	// PaymentLinks manages reusable payment links.
	PaymentLinks *PaymentLinksService
	// Batches reports the progress of asynchronous batches.
	Batches *BatchesService
	// Transfers moves funds between platform balances.
	Transfers *TransfersService
	// Wallets manages static deposit addresses.
	Wallets *WalletsService
	// Webhooks registers endpoints and inspects deliveries. Verification lives in the
	// github.com/oblodai/oblodai-go/webhooks sub-package.
	Webhooks *WebhooksService
	// Documents downloads generated PDF and CSV documents.
	Documents *DocumentsService
	// Splits forwards a share of every payment to a partner.
	Splits *SplitsService
	// Settings is merchant-level configuration exposed over the API.
	Settings *SettingsService
	// Account reads balances and account-level facts.
	Account *AccountService
	// Catalog is public reference data: currencies, networks and exchange rates.
	Catalog *CatalogService
	// Sandbox is the developer sandbox: fake money, simulated deposits, a webhook inspector.
	Sandbox *SandboxService
	// Merchants provisions merchants on a platform or a self-hosted gateway.
	Merchants *MerchantsService
}

// New builds a client. With no options it reads OBLODAI_PUBLIC_ID and OBLODAI_SECRET from the
// environment and talks to the production API:
//
//	client, err := oblodai.New()
//	client, err := oblodai.New(
//		oblodai.WithCredentials(publicID, secret),
//		oblodai.WithPayoutCredentials(payoutID, payoutSecret),
//	)
//
// It fails only on unusable configuration (a malformed base URL, half a key pair); missing
// credentials surface later, on the first call that needs them.
func New(opts ...Option) (*Client, error) {
	cfg, err := resolve(opts)
	if err != nil {
		return nil, err
	}

	httpClient := *cfg.httpClient
	// A signed request must never be replayed against another origin, and a redirect would strip
	// the body on top of that: surface it as an error instead of following it.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	httpClient.Timeout = 0 // the per-attempt context timeout owns this

	t := &transport{
		baseURL:    cfg.baseURL,
		httpClient: &httpClient,
		timeout:    cfg.timeout,
		budget:     cfg.budget,
		retry:      cfg.retry,
		clock:      newSkewClock(cfg.now),
		logger:     cfg.logger,
		headers:    cfg.headers,
		adminToken: cfg.adminToken,
		random:     cfg.random,
		userAgent: fmt.Sprintf("oblodai-go/%s (contract %s; %s)",
			Version, ContractHash[:12], runtime.Version()),
	}
	if cfg.publicID != "" {
		t.creds = &credentials{publicID: cfg.publicID, secret: cfg.secret}
	}
	if cfg.payoutPublicID != "" {
		t.payoutCreds = &credentials{publicID: cfg.payoutPublicID, secret: cfg.payoutSecret}
	}

	c := &Client{transport: t}
	c.Payments = &PaymentsService{c: c}
	c.Refunds = &RefundsService{c: c}
	c.Payouts = &PayoutsService{c: c}
	c.PayoutLinks = &PayoutLinksService{c: c}
	c.PaymentLinks = &PaymentLinksService{c: c}
	c.Batches = &BatchesService{c: c}
	c.Transfers = &TransfersService{c: c}
	c.Wallets = &WalletsService{c: c}
	c.Webhooks = &WebhooksService{c: c}
	c.Documents = &DocumentsService{c: c}
	c.Splits = &SplitsService{c: c}
	c.Settings = &SettingsService{c: c}
	c.Account = &AccountService{c: c}
	c.Catalog = &CatalogService{c: c}
	c.Sandbox = &SandboxService{c: c}
	c.Merchants = &MerchantsService{c: c}
	return c, nil
}

// BaseURL reports the API origin the client talks to.
func (c *Client) BaseURL() string { return c.transport.baseURL }

// ClockOffset reports the correction the client learned from the API's Date header after a
// signature failure. A non-zero value means this host's clock drifts.
func (c *Client) ClockOffset() time.Duration { return c.transport.clock.currentOffset() }

// RequestOption tunes one call.
type RequestOption func(*callOptions)

// WithIdempotencyKey supplies your own idempotency key, so a retry survives a process restart.
// Create routes generate one automatically when you do not. Routes the core does not deduplicate
// reject a key with sdk.idempotency_unsupported rather than pretend a re-send would be safe.
func WithIdempotencyKey(key string) RequestOption {
	return func(o *callOptions) { o.idempotencyKey = key }
}

// WithRequestTimeout overrides the per-attempt timeout for one call.
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(o *callOptions) { o.timeout = d }
}

// WithRequestBudget overrides the overall budget (attempts plus retry pauses) for one call.
func WithRequestBudget(d time.Duration) RequestOption {
	return func(o *callOptions) { o.budget = d }
}

// WithRequestHeader adds a header to one call. Headers the client owns (ReservedHeaders) are
// ignored, and a name or value with a line break or a non-ASCII byte is refused with
// sdk.bad_header before anything is sent. A per-call header wins over the same client-level one.
func WithRequestHeader(name, value string) RequestOption {
	return func(o *callOptions) {
		if o.headers == nil {
			o.headers = map[string]string{}
		}
		o.headers[name] = value
	}
}

// WithPayoutKey signs a route that accepts either key kind with the payout key — for example
// Batches.Info for a batch created by a payout key.
func WithPayoutKey() RequestOption {
	return func(o *callOptions) { o.preferPayoutKey = true }
}

func applyRequestOptions(opts []RequestOption) callOptions {
	var o callOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// The helpers below are what every resource method is built from. Keeping them here means a
// resource file contains only the route it calls and the shape it sends.

// post calls an envelope route with a JSON body.
func post[T any](ctx context.Context, c *Client, key string, body any, opts []RequestOption) (*T, error) {
	o := applyRequestOptions(opts)
	o.body = body
	return call[T](ctx, c.transport, key, o)
}

// postPath calls an envelope route with a JSON body and path parameters.
func postPath[T any](ctx context.Context, c *Client, key string, pathParams map[string]string, body any, opts []RequestOption) (*T, error) {
	o := applyRequestOptions(opts)
	o.body = body
	o.pathParams = pathParams
	return call[T](ctx, c.transport, key, o)
}

// get calls an envelope route with a query string.
func get[T any](ctx context.Context, c *Client, key string, query url.Values, pathParams map[string]string, opts []RequestOption) (*T, error) {
	o := applyRequestOptions(opts)
	o.query = query
	o.pathParams = pathParams
	return call[T](ctx, c.transport, key, o)
}

// file calls a bare route and returns the document bytes.
func file(ctx context.Context, c *Client, key string, query url.Values, pathParams map[string]string, body any, opts []RequestOption) (*FileResult, error) {
	o := applyRequestOptions(opts)
	o.query = query
	o.pathParams = pathParams
	o.body = body
	return callFile(ctx, c.transport, key, o)
}

// items calls a plain list route (an {items} result with no paginate block) and returns the items.
func items[T any](ctx context.Context, c *Client, key string, body any, opts []RequestOption) ([]T, error) {
	result, err := post[struct {
		Items []T `json:"items"`
	}](ctx, c, key, body, opts)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// listOf builds the lazy handle a paged list route returns.
func listOf[T any](ctx context.Context, c *Client, key string, params any, opts []RequestOption) *List[T] {
	return newList[T](ctx, c.transport, key, params, opts)
}

// query builds a query string from name/value pairs, skipping empty values.
func query(pairs ...string) url.Values {
	values := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			values.Set(pairs[i], pairs[i+1])
		}
	}
	return values
}
