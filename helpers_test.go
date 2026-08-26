package oblodai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// A scripted API stands in for the core: it answers a queued list of responses in order and
// records every request it saw, so a test can assert on the exact bytes and headers the client
// put on the wire. It is a real httptest server, so the whole stack — signing, encoding,
// redirects, timeouts — runs for real.

// step is one scripted answer.
type step struct {
	status  int
	body    any               // string (sent verbatim) or any value (encoded as JSON)
	headers map[string]string // extra response headers
	delay   time.Duration     // wait before answering, to trip the per-attempt timeout
	// abort closes the connection instead of answering, which the client sees as a network error.
	abort bool
}

// recorded is one recorded request.
type recorded struct {
	method string
	path   string
	// rawPath is the path exactly as it arrived on the wire, still percent-encoded.
	rawPath  string
	rawQuery string
	header   http.Header
	body     string
}

// fakeAPI is a scripted core.
type fakeAPI struct {
	t      *testing.T
	server *httptest.Server

	mu    sync.Mutex
	steps []step
	calls []recorded
}

// ok scripts a success envelope carrying result.
func ok(result any) step {
	return step{status: 200, body: map[string]any{"state": 0, "result": result}}
}

// apiError scripts an error envelope.
func apiError(status int, detail map[string]any, headers ...map[string]string) step {
	s := step{status: status, body: map[string]any{"error": detail}}
	if len(headers) > 0 {
		s.headers = headers[0]
	}
	return s
}

// emptyPage is the answer a list route gives when nothing matches.
func emptyPage() step {
	return ok(map[string]any{
		"items":    []any{},
		"paginate": map[string]any{"total": 0, "per_page": 1, "offset": 0, "has_pages": false},
		"enabled":  true,
	})
}

// pageOf scripts one page of a paged list.
func pageOf(items []any, offset, total, perPage int) step {
	return ok(map[string]any{
		"items": items,
		"paginate": map[string]any{
			"total": total, "per_page": perPage, "offset": offset,
			"has_pages": offset+len(items) < total,
		},
	})
}

func newFakeAPI(t *testing.T, steps ...step) *fakeAPI {
	t.Helper()
	api := &fakeAPI{t: t, steps: steps}
	api.server = httptest.NewServer(http.HandlerFunc(api.serve))
	t.Cleanup(api.server.Close)
	return api
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, recorded{
		method: r.Method, path: r.URL.Path, rawPath: r.URL.EscapedPath(), rawQuery: r.URL.RawQuery,
		header: r.Header.Clone(), body: string(body),
	})
	var next step
	if len(f.steps) == 0 {
		f.mu.Unlock()
		http.Error(w, `{"error":{"code":"test.no_scripted_response","retryable":false}}`, http.StatusTeapot)
		return
	}
	next, f.steps = f.steps[0], f.steps[1:]
	f.mu.Unlock()

	if next.delay > 0 {
		time.Sleep(next.delay)
	}
	if next.abort {
		panic(http.ErrAbortHandler) // closes the connection: the client sees a network failure
	}
	payload := []byte("{}")
	contentType := "application/json"
	switch typed := next.body.(type) {
	case nil:
	case string:
		payload = []byte(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			f.t.Fatalf("fakeAPI: cannot encode the scripted body: %v", err)
		}
		payload = encoded
	}
	w.Header().Set("Content-Type", contentType)
	for k, v := range next.headers {
		w.Header().Set(k, v)
	}
	status := next.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

// client builds a client pointed at the scripted API, with retry pauses short enough that a test
// does not have to wait for them.
func (f *fakeAPI) client(opts ...Option) *Client {
	f.t.Helper()
	base := []Option{
		WithBaseURL(f.server.URL),
		WithCredentials("pk_test_1", "secret-1"),
		WithRetry(RetryOptions{MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond, MaxRetryAfter: 30 * time.Second}),
		withRandom(func() float64 { return 0 }),
	}
	client, err := New(append(base, opts...)...)
	if err != nil {
		f.t.Fatalf("New: %v", err)
	}
	return client
}

// requests returns every request the scripted API has received.
func (f *fakeAPI) requests() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.calls...)
}

func (f *fakeAPI) count() int { return len(f.requests()) }

// last is the most recent request.
func (f *fakeAPI) last() recorded {
	f.t.Helper()
	calls := f.requests()
	if len(calls) == 0 {
		f.t.Fatal("no request was made")
	}
	return calls[len(calls)-1]
}

// at is the n-th request (zero-based).
func (f *fakeAPI) at(n int) recorded {
	f.t.Helper()
	calls := f.requests()
	if n >= len(calls) {
		f.t.Fatalf("request %d was never made (saw %d)", n, len(calls))
	}
	return calls[n]
}

// jsonBody decodes a recorded request body.
func (c recorded) jsonBody(t *testing.T) map[string]any {
	t.Helper()
	if c.body == "" {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(c.body), &out); err != nil {
		t.Fatalf("request body is not JSON: %v (%s)", err, c.body)
	}
	return out
}

// mustError fails unless err is an *Error, and returns it.
func mustError(t *testing.T, err error) *Error {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	apiErr, okErr := AsError(err)
	if !okErr {
		t.Fatalf("expected *oblodai.Error, got %T: %v", err, err)
	}
	return apiErr
}

// requireCode fails unless err carries that code.
func requireCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	apiErr := mustError(t, err)
	if apiErr.Code != code {
		t.Fatalf("expected code %q, got %q (%v)", code, apiErr.Code, err)
	}
	return apiErr
}

// hexish reports whether s looks like a lower-case hex digest of n bytes.
func hexish(s string, n int) bool {
	if len(s) != n*2 {
		return false
	}
	return strings.Trim(s, "0123456789abcdef") == ""
}
