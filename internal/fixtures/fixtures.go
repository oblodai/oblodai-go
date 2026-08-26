// Package fixtures loads the contract snapshot shipped in contract/ so tests in both the root
// package and the webhooks sub-package can read the same golden data.
package fixtures

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// Dir is the contract directory of this module.
func Dir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("fixtures: cannot locate the contract directory")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "contract")
}

// Fixture is one recorded exchange with a live core.
type Fixture struct {
	Route    string            `json:"route"`
	Status   int               `json:"status"`
	Request  json.RawMessage   `json:"request"`
	Response *Envelope         `json:"response"`
	Headers  map[string]string `json:"headers"`
}

// Envelope is the response envelope of a recorded exchange.
type Envelope struct {
	State  *int                       `json:"state"`
	Result json.RawMessage            `json:"result"`
	Error  map[string]json.RawMessage `json:"error"`
}

// Route is one row of the core's conformance table, as contract.json carries it.
type Route struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Auth       string `json:"auth"`
	Idempotent bool   `json:"idempotent"`
	// Safe is the core's own read-only classification. It is a pointer so a test can tell a
	// contract that declares "safe": false from one that predates the field entirely.
	Safe          *bool           `json:"safe"`
	Bare          bool            `json:"bare"`
	List          string          `json:"list"`
	RequestSchema *Schema         `json:"request_schema"`
	Response      json.RawMessage `json:"response_schema"`
}

// Schema is the request DTO schema of a route.
type Schema struct {
	Type       string             `json:"type"`
	Properties map[string]*Schema `json:"properties"`
	Required   []string           `json:"required"`
	Items      *Schema            `json:"items"`
}

// SigningVector is one request-signature test vector exported from the core's own test suite.
type SigningVector struct {
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

// WebhookVector is one webhook-signature test vector.
type WebhookVector struct {
	Secret    string `json:"secret"`
	TS        int64  `json:"ts"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// Contract is contract.json.
type Contract struct {
	CoreCommit     string              `json:"core_commit"`
	Routes         []Route             `json:"routes"`
	Enums          map[string][]string `json:"enums"`
	ErrorCodes     []string            `json:"error_codes"`
	EventTypes     []string            `json:"event_types"`
	SigningVectors []SigningVector     `json:"signing_vectors"`
	WebhookVectors []WebhookVector     `json:"webhook_vectors"`
}

// Sample is one real signed webhook delivery.
type Sample struct {
	Headers map[string]string          `json:"headers"`
	Body    map[string]json.RawMessage `json:"body"`
	// Raw is the exact byte string the core delivered; verify over this, never over a re-encode.
	Raw string `json:"raw"`
}

// LoadContract reads contract.json.
func LoadContract(t *testing.T) Contract {
	t.Helper()
	var contract Contract
	read(t, filepath.Join(Dir(t), "contract.json"), &contract)
	return contract
}

// LoadFixtures reads every recorded response body, keyed by route.
func LoadFixtures(t *testing.T) map[string]Fixture {
	t.Helper()
	out := map[string]Fixture{}
	dir := filepath.Join(Dir(t), "fixtures")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	for _, entry := range entries {
		var fixture Fixture
		read(t, filepath.Join(dir, entry.Name()), &fixture)
		out[fixture.Route] = fixture
	}
	return out
}

// FixtureRoutes lists the recorded routes in a stable order.
func FixtureRoutes(t *testing.T) []string {
	t.Helper()
	all := LoadFixtures(t)
	routes := make([]string, 0, len(all))
	for route := range all {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	return routes
}

// LoadErrorSamples reads the recorded error envelopes, keyed by error code.
func LoadErrorSamples(t *testing.T) map[string]Fixture {
	t.Helper()
	out := map[string]Fixture{}
	dir := filepath.Join(Dir(t), "errors")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	for _, entry := range entries {
		var fixture Fixture
		read(t, filepath.Join(dir, entry.Name()), &fixture)
		out[entry.Name()[:len(entry.Name())-len(".json")]] = fixture
	}
	return out
}

// LoadWebhookSamples reads the recorded signed deliveries.
func LoadWebhookSamples(t *testing.T) []Sample {
	t.Helper()
	var samples []Sample
	read(t, filepath.Join(Dir(t), "webhook-samples.json"), &samples)
	return samples
}

func read(t *testing.T, path string, into any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("fixtures: %s: %v", path, err)
	}
}
