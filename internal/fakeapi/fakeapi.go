// Package fakeapi is a stand-in gateway for the tests that execute the examples and the README
// code: it answers any route of oblodai.Routes with a minimal valid body — an empty page for a
// paged list, a PDF for a document, the scripted or an empty result otherwise — and records what
// it was sent. It is not part of the SDK's API.
package fakeapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/oblodai/oblodai-go/v2"
)

// Request is one request the fake gateway received.
type Request struct {
	OperationID string
	Method      string
	Path        string
	Query       string
	Header      http.Header
	Body        map[string]any
}

// Server is the fake gateway.
type Server struct {
	*httptest.Server
	t       testing.TB
	results map[string][]any

	mu       sync.Mutex
	requests []Request
}

// New starts a fake gateway. results scripts the result of an operation by operationId; a slice
// value is answered one element per call, the last one repeating.
func New(t testing.TB, results map[string]any) *Server {
	t.Helper()
	s := &Server{t: t, results: map[string][]any{}}
	for op, result := range results {
		if _, ok := oblodai.Routes[op]; !ok {
			t.Fatalf("fakeapi: no operation %s", op)
		}
		if list, ok := result.([]any); ok {
			s.results[op] = list
		} else {
			s.results[op] = []any{result}
		}
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Env points oblodai.New() at the fake gateway through the environment, with a test key.
func (s *Server) Env(t testing.TB) {
	t.Setenv("OBLODAI_BASE_URL", s.URL)
	t.Setenv("OBLODAI_PUBLIC_ID", "oblodai_test_pk")
	t.Setenv("OBLODAI_SECRET", "oblodai_test_sk")
}

// Client is a client of the fake gateway.
func (s *Server) Client(t testing.TB, opts ...oblodai.Option) *oblodai.Client {
	t.Helper()
	all := append([]oblodai.Option{oblodai.WithBaseURL(s.URL), oblodai.WithCredentials("oblodai_test_pk", "oblodai_test_sk")}, opts...)
	client, err := oblodai.New(all...)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// Requests is every request received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// Operations lists the operationIds received, in order.
func (s *Server) Operations() []string {
	var out []string
	for _, r := range s.Requests() {
		out = append(out, r.OperationID)
	}
	return out
}

var placeholder = regexp.MustCompile(`\\\{[^}]*\\\}`)

// route finds the operation a request line belongs to; a literal segment beats a placeholder
// (/v1/documents/jobs/file is not /v1/documents/{kind}/{id}).
func route(method, path string) (string, oblodai.RouteSpec, bool) {
	best, found := "", false
	for op, r := range oblodai.Routes {
		pattern := "^" + placeholder.ReplaceAllString(regexp.QuoteMeta(r.Path), "[^/]+") + "$"
		if r.Method != method || !regexp.MustCompile(pattern).MatchString(path) {
			continue
		}
		if !found || strings.Count(r.Path, "{") < strings.Count(oblodai.Routes[best].Path, "{") {
			best, found = op, true
		}
	}
	return best, oblodai.Routes[best], found
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	op, spec, found := route(r.Method, r.URL.Path)
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	s.mu.Lock()
	s.requests = append(s.requests, Request{
		OperationID: op, Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: body,
	})
	var result any = map[string]any{}
	if queue := s.results[op]; len(queue) > 0 {
		result = queue[0]
		if len(queue) > 1 {
			s.results[op] = queue[1:]
		}
	}
	s.mu.Unlock()

	if !found {
		s.t.Errorf("fakeapi: the code called a route the API does not have: %s %s", r.Method, r.URL.Path)
		http.Error(w, `{"error":{"code":"test.no_route","retryable":false}}`, http.StatusNotFound)
		return
	}
	if spec.Bare {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7 fake"))
		return
	}
	if spec.ListKind == oblodai.ListPaged && isEmptyObject(result) {
		result = map[string]any{
			"items":    []any{},
			"paginate": map[string]any{"total": 0, "per_page": 25, "offset": 0, "has_pages": false},
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(oblodai.HeaderRequestID, strings.TrimSpace(r.Header.Get(oblodai.HeaderRequestID)))
	_ = json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": result})
}

func isEmptyObject(v any) bool {
	m, ok := v.(map[string]any)
	return ok && len(m) == 0
}
