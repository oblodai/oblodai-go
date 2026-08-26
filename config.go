package oblodai

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client configuration. Every option has an environment fallback so a deployment can move keys and
// endpoints out of the code:
//
//	OBLODAI_PUBLIC_ID / OBLODAI_SECRET  the merchant's API key pair
//	OBLODAI_BASE_URL                    API origin
//	OBLODAI_ADMIN_TOKEN                 admin token of a self-hosted gateway
//	OBLODAI_LOG                         debug | info | warn | error
//	OBLODAI_ALLOW_INSECURE=1            permit a plain http:// base URL

// Option configures a Client. Options are applied in order; later ones win.
type Option func(*config)

type config struct {
	baseURL       string
	publicID      string
	secret        string
	httpClient    *http.Client
	timeout       time.Duration
	budget        time.Duration
	retry         RetryOptions
	logger        Logger
	headers       map[string]string
	adminToken    string
	allowInsecure bool
	now           func() time.Time
	random        func() float64
}

// WithCredentials sets the merchant's API key pair. One key signs every route the gateway gates:
// payments, payouts, settings, documents.
func WithCredentials(publicID, secret string) Option {
	return func(c *config) { c.publicID, c.secret = publicID, secret }
}

// WithBaseURL overrides the API origin. A path prefix is kept (https://gw.corp/oblodai).
func WithBaseURL(baseURL string) Option {
	return func(c *config) { c.baseURL = baseURL }
}

// WithHTTPClient supplies the http.Client to send with — use it for a proxy, a custom transport,
// mutual TLS or a recording stub in tests. Its Timeout field is ignored in favour of the
// per-attempt timeout, and redirects are always disabled: a signed request must not be replayed
// against another origin.
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) { c.httpClient = client }
}

// WithTimeout sets the per-attempt timeout. Default 30 s.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithCallBudget sets the overall budget for one call including retries and pauses. Default 90 s.
func WithCallBudget(d time.Duration) Option {
	return func(c *config) { c.budget = d }
}

// WithRetry replaces the retry policy. RetryOptions{MaxRetries: 0} disables retries.
func WithRetry(retry RetryOptions) Option {
	return func(c *config) { c.retry = retry }
}

// WithLogger installs a structured logger for the client's diagnostics.
func WithLogger(logger Logger) Option {
	return func(c *config) { c.logger = logger }
}

// WithHeader adds a header to every request. Headers the client signs or owns are ignored,
// compared case-insensitively: X-Public-Id, X-Signature, X-Timestamp, Idempotency-Key,
// X-Admin-Token (sent by the client on onboarding routes only), Accept, User-Agent, Content-Type,
// Content-Length and Host — ReservedHeaders lists them. A name or value carrying a line break or
// a non-ASCII byte is refused with sdk.bad_header on the first call that would send it.
func WithHeader(name, value string) Option {
	return func(c *config) {
		if c.headers == nil {
			c.headers = map[string]string{}
		}
		c.headers[name] = value
	}
}

// WithAdminToken supplies the admin token of a self-hosted gateway. Only the merchant
// provisioning routes send it.
func WithAdminToken(token string) Option {
	return func(c *config) { c.adminToken = token }
}

// WithInsecureBaseURL permits a plain http:// base URL for a host that is not loopback. Loopback
// origins are allowed without it; anything else on http would put a signed secret on the wire.
func WithInsecureBaseURL(allow bool) Option {
	return func(c *config) { c.allowInsecure = allow }
}

// withClock and withRandom keep the client deterministic in tests without exposing knobs that
// have no meaning in production.
func withClock(now func() time.Time) Option   { return func(c *config) { c.now = now } }
func withRandom(random func() float64) Option { return func(c *config) { c.random = random } }

// resolve merges the options with the environment and validates what can be validated up front.
func resolve(opts []Option) (*config, *Error) {
	c := &config{retry: DefaultRetry(), now: time.Now, random: defaultRandom}
	for _, opt := range opts {
		opt(c)
	}

	c.baseURL = strings.TrimRight(firstNonEmpty(c.baseURL, os.Getenv("OBLODAI_BASE_URL"), DefaultBaseURL), "/")
	if err := checkBaseURL(c.baseURL, c.allowInsecure || os.Getenv("OBLODAI_ALLOW_INSECURE") == "1"); err != nil {
		return nil, err
	}

	c.publicID = firstNonEmpty(c.publicID, os.Getenv("OBLODAI_PUBLIC_ID"))
	c.secret = firstNonEmpty(c.secret, os.Getenv("OBLODAI_SECRET"))
	if (c.publicID == "") != (c.secret == "") {
		return nil, newConfigError(CodeBadConfig,
			"the public id and the secret must be provided together (or set both OBLODAI_PUBLIC_ID and OBLODAI_SECRET)", "")
	}
	c.adminToken = firstNonEmpty(c.adminToken, os.Getenv("OBLODAI_ADMIN_TOKEN"))

	if c.logger == nil {
		if level := strings.ToLower(os.Getenv("OBLODAI_LOG")); level != "" {
			if _, ok := logOrder[LogLevel(level)]; ok {
				c.logger = NewTextLogger(LogLevel(level), nil)
			}
		}
	}
	if c.logger == nil {
		c.logger = nopLogger{}
	} else {
		// Redaction happens here, once, so no logger — the built-in one or a caller's — ever sees
		// a secret-looking field value.
		c.logger = redactingLogger{inner: c.logger}
	}
	if c.timeout <= 0 {
		c.timeout = 30 * time.Second
	}
	if c.budget <= 0 {
		c.budget = 90 * time.Second
	}
	c.retry = c.retry.withDefaults()
	if c.httpClient == nil {
		c.httpClient = &http.Client{}
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.random == nil {
		c.random = defaultRandom
	}
	return c, nil
}

// checkBaseURL refuses an origin that would carry a signed secret in clear text.
func checkBaseURL(baseURL string, allowInsecure bool) *Error {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return newConfigError(CodeBadConfig, "the base URL is not a valid URL: "+baseURL, "baseURL")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && (allowInsecure || isLoopback(parsed.Hostname())) {
		return nil
	}
	return newConfigError(CodeBadConfig,
		"the base URL must use https (got "+parsed.Scheme+"://"+parsed.Host+"); pass WithInsecureBaseURL(true) for a local core", "baseURL")
}

func isLoopback(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
