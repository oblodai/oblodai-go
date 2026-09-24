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

// Dir is the suite directory; the test is skipped when there is none to find.
func Dir(t testing.TB) string {
	t.Helper()
	explicit := os.Getenv("SDKGEN_CONFORMANCE")
	dir := explicit
	if dir == "" {
		backend := os.Getenv("OBLODAI_BACKEND")
		if backend == "" {
			_, file, _, _ := runtime.Caller(0)
			backend = filepath.Join(filepath.Dir(file), "..", "..", "..", "oblodai-backend")
		}
		dir = filepath.Join(backend, "tools", "sdkgen", "conformance")
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
	data, err := os.ReadFile(filepath.Join(Dir(t), suite.Source.Spec))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	var signing struct {
		SkewSeconds int64 `json:"skew_seconds"`
	}
	if err := json.Unmarshal(spec["x-oblodai-signing"], &signing); err != nil || signing.SkewSeconds <= 0 {
		t.Fatalf("x-oblodai-signing.skew_seconds: %v", err)
	}
	var cur json.RawMessage = data
	for _, part := range strings.Split(strings.TrimPrefix(suite.Source.Pointer, "/"), "/") {
		part = strings.NewReplacer("~1", "/", "~0", "~").Replace(part)
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(cur, &obj); err != nil {
			t.Fatalf("pointer %s: %v", suite.Source.Pointer, err)
		}
		next, ok := obj[part]
		if !ok {
			t.Fatalf("pointer %s: no %q", suite.Source.Pointer, part)
		}
		cur = next
	}
	if err := json.Unmarshal(cur, &vectors); err != nil {
		t.Fatalf("vectors at %s: %v", suite.Source.Pointer, err)
	}
	if len(vectors) == 0 {
		t.Fatalf("no vectors at %s", suite.Source.Pointer)
	}
	return vectors, signing.SkewSeconds
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
