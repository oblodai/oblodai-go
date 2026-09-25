// Package conformance reads the shared behaviour suite every Oblodai SDK runs — the backend's
// tools/sdkgen/conformance — for this SDK's tests. It is not part of the SDK's API.
//
// The suite is found at $SDKGEN_CONFORMANCE, else at tools/sdkgen/conformance of the backend
// checkout ($OBLODAI_BACKEND, else ../oblodai-backend next to this repository). Without one the
// tests skip, loudly; when either variable is set a missing suite fails them instead. Signing
// vectors are not in the scenario files: a suite names the backend's openapi.json and a pointer
// into its x-oblodai-signing, and the vectors are read from there.
package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Suite is one scenario file.
type Suite struct {
	Description string     `json:"description"`
	Source      *Source    `json:"source"`
	Checks      []Check    `json:"checks"`
	Scenarios   []Scenario `json:"scenarios"`
	// HeaderNames says where in the spec the header names are and the role of each by position;
	// the suite itself names no header (see Names).
	HeaderNames *HeaderNames `json:"header_names"`
	// Fields (webhook_delivery): header role → the snake_case field of the delivery info that must
	// carry the value of that role's header; "" — the header is consumed by the signature check.
	Fields map[string]string `json:"fields"`
	// Webhooks (forward_compat): delivery bodies the SDK's parse must read — with their raw type,
	// known or not as expected.
	Webhooks []WebhookParse `json:"webhooks"`
}

// WebhookParse is one delivery body (a JSON object) and what parsing it gives.
type WebhookParse struct {
	Name   string          `json:"name"`
	Body   json.RawMessage `json:"body"`
	Expect struct {
		Known bool   `json:"known"`
		Type  string `json:"type"`
	} `json:"expect"`
}

// HeaderNames points at a list of header names in the spec and gives the role of each position.
type HeaderNames struct {
	Pointer string   `json:"pointer"`
	Roles   []string `json:"roles"`
}

// Source points at the vectors: the spec (relative to the suite directory) and a JSON pointer.
type Source struct {
	Spec    string `json:"spec"`
	Pointer string `json:"pointer"`
}

// Check runs over every vector of the source.
type Check struct {
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Mutate    string          `json:"mutate"`
	NowFromTS json.RawMessage `json:"now_from_ts"`
	Expect    string          `json:"expect"`
	// Key (webhook_delivery): which secret verifies — "current" (secret) or "previous".
	Key string `json:"key"`
	// PublicID (request_headers): the public key id the request is signed with.
	PublicID string `json:"public_id"`
}

// Scenario is one call on scripted responses.
type Scenario struct {
	Name      string     `json:"name"`
	Call      Call       `json:"call"`
	Responses []Response `json:"responses"`
	Expect    Expect     `json:"expect"`
}

// Call names the operation and its body arguments by JSON name.
type Call struct {
	Operation string          `json:"operation"`
	Args      json.RawMessage `json:"args"`
}

// Response is one scripted answer; TransportError "timeout" is a timeout instead of an answer.
type Response struct {
	Status         int               `json:"status"`
	Headers        map[string]string `json:"headers"`
	JSON           json.RawMessage   `json:"json"`
	TransportError string            `json:"transport_error"`
}

// Expect is what the SDK must have done.
type Expect struct {
	Requests           int            `json:"requests"`
	SameIdempotencyKey bool           `json:"same_idempotency_key"`
	IdempotencyKey     string         `json:"idempotency_key"`
	DelaysMS           []float64      `json:"delays_ms"`
	ErrorCode          string         `json:"error_code"`
	ResultField        map[string]any `json:"result_field"`
	RequestBodyField   map[string]any `json:"request_body_field"`
}

// backendRoot is the backend checkout: $OBLODAI_BACKEND, else ../oblodai-backend next to this repository.
func backendRoot() string {
	if backend := os.Getenv("OBLODAI_BACKEND"); backend != "" {
		return backend
	}
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "oblodai-backend")
}

// Signing is x-oblodai-signing of the backend's openapi.json, decoded; the test is skipped when
// there is no backend checkout to read it from.
func Signing(t testing.TB) map[string]any {
	t.Helper()
	path := filepath.Join(backendRoot(), "services", "core", "api", "openapi.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.Getenv("OBLODAI_BACKEND") != "" {
			t.Fatalf("backend spec: %v", err)
		}
		t.Skipf("backend spec not found at %s; set OBLODAI_BACKEND", path)
	}
	var spec struct {
		Signing map[string]any `json:"x-oblodai-signing"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if spec.Signing == nil {
		t.Fatalf("%s: no x-oblodai-signing", path)
	}
	return spec.Signing
}

// Dir is the suite directory; the test is skipped when there is none to find.
func Dir(t testing.TB) string {
	t.Helper()
	explicit := os.Getenv("SDKGEN_CONFORMANCE")
	dir := explicit
	if dir == "" {
		dir = filepath.Join(backendRoot(), "tools", "sdkgen", "conformance")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		if explicit != "" || os.Getenv("OBLODAI_BACKEND") != "" {
			t.Fatalf("conformance suite not found at %s", dir)
		}
		t.Skipf("conformance suite not found at %s; set OBLODAI_BACKEND or SDKGEN_CONFORMANCE", dir)
	}
	return dir
}

// Load reads one suite by name ("retry").
func Load(t testing.TB, name string) Suite {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(Dir(t), name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite Suite
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatalf("%s.json: %v", name, err)
	}
	return suite
}

// Vectors reads the vectors a suite points at, and the spec's x-oblodai-signing skew_seconds.
func Vectors[V any](t testing.TB, suite Suite) (vectors []V, skewSeconds int64) {
	t.Helper()
	if suite.Source == nil {
		t.Fatal("the suite names no source of vectors")
	}
	var signing struct {
		SkewSeconds int64 `json:"skew_seconds"`
	}
	if err := json.Unmarshal(lookup(t, suite, "/x-oblodai-signing"), &signing); err != nil || signing.SkewSeconds <= 0 {
		t.Fatalf("x-oblodai-signing.skew_seconds: %v", err)
	}
	if err := json.Unmarshal(lookup(t, suite, suite.Source.Pointer), &vectors); err != nil {
		t.Fatalf("vectors at %s: %v", suite.Source.Pointer, err)
	}
	if len(vectors) == 0 {
		t.Fatalf("no vectors at %s", suite.Source.Pointer)
	}
	return vectors, signing.SkewSeconds
}

// Names maps each header role of the suite to the header name the spec gives it — the names a
// check compares with, never the SDK's own constants: a rename in the contract that has not
// reached the SDK then fails the suite.
func Names(t testing.TB, suite Suite) map[string]string {
	t.Helper()
	if suite.HeaderNames == nil || suite.Source == nil {
		t.Fatal("the suite names no header_names or no source spec")
	}
	var names []string
	if err := json.Unmarshal(lookup(t, suite, suite.HeaderNames.Pointer), &names); err != nil {
		t.Fatalf("header names at %s: %v", suite.HeaderNames.Pointer, err)
	}
	if len(names) != len(suite.HeaderNames.Roles) {
		t.Fatalf("header names %v at %s, roles %v", names, suite.HeaderNames.Pointer, suite.HeaderNames.Roles)
	}
	out := map[string]string{}
	for i, role := range suite.HeaderNames.Roles {
		if names[i] == "" {
			t.Fatalf("empty header name for role %s", role)
		}
		out[role] = names[i]
	}
	return out
}

// lookup resolves a JSON pointer in the suite's spec.
func lookup(t testing.TB, suite Suite, pointer string) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(Dir(t), suite.Source.Spec))
	if err != nil {
		t.Fatal(err)
	}
	var cur json.RawMessage = data
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		part = strings.NewReplacer("~1", "/", "~0", "~").Replace(part)
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(cur, &obj); err != nil {
			t.Fatalf("pointer %s: %v", pointer, err)
		}
		next, ok := obj[part]
		if !ok {
			t.Fatalf("pointer %s: no %q", pointer, part)
		}
		cur = next
	}
	return cur
}

// Offset turns a check's now_from_ts (0, "skew", "skew+1") into seconds.
func Offset(t testing.TB, raw json.RawMessage, skew int64) int64 {
	t.Helper()
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("now_from_ts %s", raw)
	}
	switch s {
	case "skew":
		return skew
	case "skew+1":
		return skew + 1
	}
	t.Fatalf("now_from_ts %q", s)
	return 0
}
