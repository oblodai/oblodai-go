package oblodai

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oblodai/oblodai-go/v2/internal/conformance"
)

// The shared conformance suite every Oblodai SDK runs (backend tools/sdkgen/conformance): signing
// vectors of the core, and calls of generated methods on scripted responses. The webhook suite
// runs in the webhooks package. Retry pauses are recorded instead of slept.
//
// In Go an amount is a Decimal string type, so a float cannot reach a method at all: a scenario's
// JSON arguments are decoded into the method's typed params, and a JSON number in an amount fails
// that decoding with sdk.float_amount — before anything is sent.

type requestVector struct {
	Name           string `json:"name"`
	Secret         string `json:"secret"`
	TS             int64  `json:"ts"`
	Method         string `json:"method"`
	RequestURI     string `json:"request_uri"`
	IdempotencyKey string `json:"idempotency_key"`
	Body           string `json:"body"`
	Canonical      string `json:"canonical"`
	Signature      string `json:"signature"`
}

func TestConformanceSigning(t *testing.T) {
	suite := conformance.Load(t, "signing")
	vectors, _ := conformance.Vectors[requestVector](t, suite)
	names := conformance.Names(t, suite)
	for _, check := range suite.Checks {
		for _, v := range vectors {
			t.Run(check.Name+"/"+v.Name, func(t *testing.T) {
				in := SignInput{TS: v.TS, Method: v.Method, RequestURI: v.RequestURI, IdempotencyKey: v.IdempotencyKey, Body: []byte(v.Body)}
				switch check.Kind {
				case "request_canonical":
					if got := CanonicalString(in); got != v.Canonical {
						t.Fatalf("canonical\n got %q\nwant %q", got, v.Canonical)
					}
				case "request_signature":
					if got := SignRequest(v.Secret, in); got != v.Signature {
						t.Fatalf("signature\n got %s\nwant %s", got, v.Signature)
					}
				case "request_headers":
					checkRequestHeaders(t, v, check.PublicID, names)
				default:
					t.Fatalf("unknown check kind %q", check.Kind)
				}
			})
		}
	}
}

// checkRequestHeaders sends the vector's request through the transport every generated method uses
// — the vector's keys, the clock at its ts — and compares what went on the wire with the vector
// under the header names of the spec (by role), not under this SDK's constants.
func checkRequestHeaders(t *testing.T, v requestVector, publicID string, names map[string]string) {
	if publicID == "" {
		t.Fatal("request_headers names no public_id")
	}
	uri, err := url.Parse(v.RequestURI)
	if err != nil {
		t.Fatal(err)
	}
	query, err := url.ParseQuery(uri.RawQuery)
	if err != nil {
		t.Fatal(err)
	}
	s := &script{t: t, responses: []conformance.Response{{Status: 200, JSON: json.RawMessage(`{}`)}}}
	client, err := New(
		WithBaseURL("https://api.test"),
		WithCredentials(publicID, v.Secret),
		WithHTTPClient(&http.Client{Transport: s}),
		withClock(func() time.Time { return time.Unix(v.TS, 0) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	call := Call{
		Route: RouteSpec{OperationID: "conformance", Method: v.Method, Path: uri.Path, Auth: AuthKey, Idempotent: v.IdempotencyKey != ""},
		Query: query,
	}
	if v.Method != http.MethodGet {
		call.Body = json.RawMessage(v.Body)
	}
	if _, callErr := client.transport.request(context.Background(), call, callOptions{idempotencyKey: v.IdempotencyKey}); callErr != nil {
		t.Fatalf("request: %v", callErr)
	}
	if len(s.requests) != 1 {
		t.Fatalf("%d requests, want 1", len(s.requests))
	}
	req, body := s.requests[0], string(s.bodies[0])
	if got := req.URL.RequestURI(); req.Method != v.Method || got != v.RequestURI || body != v.Body {
		t.Fatalf("sent %s %s %q, the vector is %s %s %q", req.Method, got, body, v.Method, v.RequestURI, v.Body)
	}
	want := map[string]string{
		"public_id":       publicID,
		"signature":       v.Signature,
		"timestamp":       strconv.FormatInt(v.TS, 10),
		"idempotency_key": v.IdempotencyKey,
	}
	if len(names) != len(want) {
		t.Fatalf("header roles %v, want %v", names, want)
	}
	for role, name := range names {
		value, ok := want[role]
		if !ok {
			t.Fatalf("unknown header role %q", role)
		}
		got := req.Header.Values(name) // http.Header: the name compares case-insensitively
		if value == "" {
			if len(got) != 0 {
				t.Errorf("%s (%s) = %q, want no such header", name, role, got)
			}
			continue
		}
		if len(got) != 1 || got[0] != value {
			t.Errorf("%s (%s) = %q, want %q", name, role, got, value)
		}
	}
}

func TestConformanceCalls(t *testing.T) {
	operations := generatedOperations(t)
	for _, name := range []string{"retry", "money", "forward_compat"} {
		for _, scenario := range conformance.Load(t, name).Scenarios {
			t.Run(name+"/"+scenario.Name, func(t *testing.T) {
				runScenario(t, operations, scenario)
			})
		}
	}
}

// generatedOperations maps operationId to (service field of Resources, method) — read from the
// generated source, the way a reader finds Routes["op"] in a method body.
func generatedOperations(t *testing.T) map[string][2]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "zz_generated_resources.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{} // service type -> field of Resources
	resources := reflect.TypeFor[Resources]()
	for i := 0; i < resources.NumField(); i++ {
		fields[resources.Field(i).Type.Elem().Name()] = resources.Field(i).Name
	}
	out := map[string][2]string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		service := fields[star.X.(*ast.Ident).Name]
		ast.Inspect(fn, func(n ast.Node) bool {
			index, ok := n.(*ast.IndexExpr)
			if !ok {
				return true
			}
			if id, ok := index.X.(*ast.Ident); ok && id.Name == "Routes" {
				if lit, ok := index.Index.(*ast.BasicLit); ok {
					op, _ := strconv.Unquote(lit.Value)
					out[op] = [2]string{service, fn.Name.Name}
				}
			}
			return true
		})
	}
	if len(out) != len(Routes) {
		t.Fatalf("found %d generated operations, Routes has %d", len(out), len(Routes))
	}
	return out
}

// script replays a scenario's responses and records what the client sent.
type script struct {
	t         *testing.T
	mu        sync.Mutex
	responses []conformance.Response
	requests  []*http.Request
	bodies    [][]byte
}

func (s *script) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil { // a GET carries none
		body, _ = io.ReadAll(req.Body)
	}
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.bodies = append(s.bodies, body)
	if len(s.responses) == 0 {
		s.mu.Unlock()
		s.t.Errorf("unscripted request %s %s", req.Method, req.URL)
		return nil, io.ErrUnexpectedEOF
	}
	next := s.responses[0]
	s.responses = s.responses[1:]
	s.mu.Unlock()

	if next.TransportError == "timeout" {
		<-req.Context().Done() // the attempt's own timeout fires
		return nil, req.Context().Err()
	}
	header := http.Header{}
	for k, v := range next.Headers {
		header.Set(k, v)
	}
	payload := []byte("<html>proxy</html>")
	header.Set("Content-Type", "text/html")
	if len(next.JSON) > 0 {
		payload = next.JSON
		header.Set("Content-Type", "application/json")
	}
	return &http.Response{
		StatusCode: next.Status, Header: header, Body: io.NopCloser(bytes.NewReader(payload)),
		ContentLength: int64(len(payload)), Request: req,
	}, nil
}

func runScenario(t *testing.T, operations map[string][2]string, scenario conformance.Scenario) {
	target, ok := operations[scenario.Call.Operation]
	if !ok {
		t.Fatalf("no generated method calls %s", scenario.Call.Operation)
	}
	s := &script{t: t, responses: scenario.Responses}
	var delays []float64
	client, err := New(
		WithBaseURL("https://api.test"),
		WithCredentials("oblodai_test_pk", "oblodai_test_sk"),
		WithHTTPClient(&http.Client{Transport: s}),
		WithTimeout(50*time.Millisecond),
		withSleep(func(_ context.Context, d time.Duration) error {
			delays = append(delays, float64(d.Milliseconds()))
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	method := reflect.ValueOf(client.Resources).FieldByName(target[0]).MethodByName(target[1])
	result, callErr := callWithArgs(method, scenario.Call.Args)

	expect := scenario.Expect
	if len(s.requests) != expect.Requests {
		t.Fatalf("%d requests, want %d (error: %v)", len(s.requests), expect.Requests, callErr)
	}
	var keys []string
	for _, r := range s.requests {
		keys = append(keys, r.Header.Get(HeaderIdempotencyKey))
	}
	switch expect.IdempotencyKey {
	case "absent":
		for _, k := range keys {
			if k != "" {
				t.Fatalf("idempotency keys %q, want none", keys)
			}
		}
	case "present":
		for _, k := range keys {
			if k == "" {
				t.Fatalf("idempotency keys %q, want one on every request", keys)
			}
		}
	}
	if expect.SameIdempotencyKey {
		for _, k := range keys {
			if k == "" || k != keys[0] {
				t.Fatalf("idempotency keys %q, want one key on every attempt", keys)
			}
		}
	}
	if expect.DelaysMS != nil && !reflect.DeepEqual(delays, expect.DelaysMS) {
		t.Fatalf("pauses %v ms, want %v", delays, expect.DelaysMS)
	}
	for name, want := range expect.RequestBodyField {
		var body map[string]any
		if err := json.Unmarshal(s.bodies[len(s.bodies)-1], &body); err != nil || !reflect.DeepEqual(body[name], want) {
			t.Fatalf("request body %s = %v, want %v (%v)", name, body[name], want, err)
		}
	}
	if expect.ErrorCode != "" {
		apiErr, ok := AsError(callErr)
		if !ok || apiErr.Code != expect.ErrorCode {
			t.Fatalf("error %v, want %s", callErr, expect.ErrorCode)
		}
		return
	}
	if callErr != nil {
		t.Fatalf("call failed: %v", callErr)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for name, want := range expect.ResultField {
		if !reflect.DeepEqual(fields[name], want) {
			t.Fatalf("result %s = %v, want %v", name, fields[name], want)
		}
	}
}

// callWithArgs calls a generated method: the context, then the scenario's arguments decoded into
// the method's params struct (nil when there are none), no call options. A decoding failure is
// the call's error — nothing was sent.
func callWithArgs(method reflect.Value, args json.RawMessage) (any, error) {
	fn := method.Type()
	in := []reflect.Value{reflect.ValueOf(context.Background())}
	for i := 1; i < fn.NumIn(); i++ {
		param := fn.In(i)
		if fn.IsVariadic() && i == fn.NumIn()-1 {
			break
		}
		if param.Kind() != reflect.Pointer || param.Elem().Kind() != reflect.Struct {
			panic("conformance: unsupported parameter " + param.String())
		}
		value := reflect.New(param.Elem())
		if len(args) > 0 && strings.TrimSpace(string(args)) != "{}" {
			if err := json.Unmarshal(args, value.Interface()); err != nil {
				return nil, err
			}
		}
		in = append(in, value)
	}
	out := method.Call(in)
	var err error
	if e := out[1].Interface(); e != nil {
		err = e.(error)
	}
	return out[0].Interface(), err
}
