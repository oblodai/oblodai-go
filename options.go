package oblodai

import (
	"net/http"
	"time"
)

// Per-call options. Every generated method takes them last:
//
//	payout, err := client.Payouts.Create(ctx, params,
//		oblodai.WithIdempotencyKey("payout-1"),
//		oblodai.WithRequestTimeout(10*time.Second),
//		oblodai.WithRequestID("checkout-42"))
//
// An option left out falls back to the client's setting (see Option for those).

// RequestOption tunes one call.
type RequestOption func(*callOptions)

// callOptions is the per-call state a RequestOption may change.
type callOptions struct {
	idempotencyKey string
	timeout        time.Duration
	budget         time.Duration
	maxRetries     *int
	headers        map[string]string
	requestID      string
	raw            **RawResponse
}

// WithIdempotencyKey supplies your own idempotency key, so a retry survives a process restart.
// Routes the core deduplicates generate one automatically when you do not. Routes it does not
// deduplicate reject a key with sdk.idempotency_unsupported rather than pretend a re-send would be
// safe — except the few whose request body carries its own idempotency_key field (the sandbox
// faucet), where the key goes into that field and no header is sent.
func WithIdempotencyKey(key string) RequestOption {
	return func(o *callOptions) { o.idempotencyKey = key }
}

// WithRequestTimeout overrides the per-attempt timeout for one call (the client's WithTimeout).
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(o *callOptions) { o.timeout = d }
}

// WithRequestBudget overrides the overall budget (attempts plus retry pauses) for one call.
func WithRequestBudget(d time.Duration) RequestOption {
	return func(o *callOptions) { o.budget = d }
}

// WithMaxRetries overrides how many times this call may be repeated after the first attempt; 0
// sends it once. What is safe to repeat is still decided by the route (RetryOptions).
func WithMaxRetries(n int) RequestOption {
	return func(o *callOptions) {
		if n < 0 {
			n = 0
		}
		o.maxRetries = &n
	}
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

// WithExtraHeaders adds several headers to one call, as WithRequestHeader does for one.
func WithExtraHeaders(headers map[string]string) RequestOption {
	return func(o *callOptions) {
		for name, value := range headers {
			WithRequestHeader(name, value)(o)
		}
	}
}

// WithRequestID sets the X-Request-ID the call is sent with — the same on every attempt — so your
// logs and ours can be joined. Without it the client uses an X-Request-ID header you added, or
// generates a random one.
func WithRequestID(id string) RequestOption {
	return func(o *callOptions) { o.requestID = id }
}

// WithRawResponse stores the call's last HTTP response in *dst: status, headers, body and request
// id. It is set whenever a response arrived — on success and on an error status alike — and left
// alone when none did (a timeout). For a list it is the response of the last page fetched.
//
//	var raw *oblodai.RawResponse
//	invoice, err := client.Payments.Create(ctx, params, oblodai.WithRawResponse(&raw))
//	log.Println(raw.StatusCode, raw.RequestID)
func WithRawResponse(dst **RawResponse) RequestOption {
	return func(o *callOptions) { o.raw = dst }
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

// RawResponse is the HTTP side of one call.
type RawResponse struct {
	// StatusCode is the HTTP status.
	StatusCode int
	// Header holds the response headers.
	Header http.Header
	// Body is the response body as received.
	Body []byte
	// RequestID is the response's X-Request-ID, else the one the call was sent with.
	RequestID string
}
