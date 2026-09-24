package oblodai

import (
	"fmt"
	"net/http"
	"runtime"
	"time"
)

// Client is the Oblodai API client. One instance per API key; it is safe to share across
// goroutines and should be created once and reused, so connections and the learned clock offset
// are shared.
//
// Its resource services — client.Payments, client.Payouts, client.Webhooks and the rest — come
// from the embedded, generated Resources.
type Client struct {
	Resources

	cfg       *config
	transport *transport
}

// New builds a client. With no options it reads OBLODAI_PUBLIC_ID and OBLODAI_SECRET from the
// environment and talks to the production API:
//
//	client, err := oblodai.New()
//	client, err := oblodai.New(oblodai.WithCredentials(publicID, secret))
//
// It fails only on unusable configuration (a malformed base URL, half a key pair); missing
// credentials surface later, on the first call that needs them.
func New(opts ...Option) (*Client, error) {
	cfg, err := resolve(opts)
	if err != nil {
		return nil, err
	}
	return newClient(cfg, newSkewClock(cfg.now)), nil
}

// WithOptions returns a copy of the client with more options applied on top of its own — a
// shorter timeout, another retry policy, extra headers, hooks — for part of a program:
//
//	fast, err := client.WithOptions(oblodai.WithTimeout(5*time.Second), oblodai.WithRetry(oblodai.RetryOptions{}))
//
// The copy shares the learned clock offset; the original is left unchanged.
func (c *Client) WithOptions(opts ...Option) (*Client, error) {
	cfg := c.cfg.clone()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	if err := cfg.finalize(); err != nil {
		return nil, err
	}
	return newClient(cfg, c.transport.clock), nil
}

func newClient(cfg *config, clock *skewClock) *Client {
	httpClient := *cfg.httpClient
	// A signed request must never be replayed against another origin, and a redirect would strip
	// the body on top of that: surface it as an error instead of following it.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	httpClient.Timeout = 0 // the per-attempt context timeout owns this

	var logger Logger = nopLogger{}
	if cfg.logger != nil {
		// Redaction happens here, once, so no logger — the built-in one or a caller's — ever sees
		// a secret-looking field value.
		logger = redactingLogger{inner: cfg.logger}
	}
	t := &transport{
		baseURL:    cfg.baseURL,
		httpClient: &httpClient,
		timeout:    cfg.timeout,
		budget:     cfg.budget,
		retry:      cfg.retry,
		clock:      clock,
		logger:     logger,
		headers:    cfg.headers,
		adminToken: cfg.adminToken,
		random:     cfg.random,
		hooks:      cfg.hooks,
		sleep:      cfg.sleep,
		userAgent:  fmt.Sprintf("oblodai-go/%s (%s)", Version, runtime.Version()),
	}
	if cfg.publicID != "" {
		t.creds = &credentials{publicID: cfg.publicID, secret: cfg.secret}
	}
	return &Client{Resources: newResources(t), cfg: cfg, transport: t}
}

// BaseURL reports the API origin the client talks to.
func (c *Client) BaseURL() string { return c.transport.baseURL }

// ClockOffset reports the correction the client learned from the API's Date header after a
// signature failure. A non-zero value means this host's clock drifts.
func (c *Client) ClockOffset() time.Duration { return c.transport.clock.currentOffset() }
