package oblodai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/oblodai/oblodai-go/internal/fixtures"
)

// Every route the core declares has exactly one SDK method, wired to the right method, path, auth
// gate and idempotency behaviour. The coverage ledger itself lives in contract_coverage_test.go:
// a route the core adds fails these tests until a method exists for it.

func TestRouteRegistryIsTheCoresMerchantSurface(t *testing.T) {
	contract := fixtures.LoadContract(t)
	skip := regexp.MustCompile(`^/(healthz|readyz|docs|openapi\.json|internal)`)
	declared := map[string]bool{}
	for _, r := range contract.Routes {
		if !skip.MatchString(r.Path) {
			declared[r.Method+" "+r.Path] = true
		}
	}
	for key := range declared {
		if _, ok := Routes[key]; !ok {
			t.Errorf("the core declares %s but the SDK does not know it", key)
		}
	}
	for key := range Routes {
		if !declared[key] {
			t.Errorf("the SDK declares %s but the core does not", key)
		}
	}
	if len(Routes) != len(RouteKeys) {
		t.Errorf("Routes has %d entries but RouteKeys lists %d", len(Routes), len(RouteKeys))
	}
}

// Every flag of every route must equal what contract.json declares — not just the key set. The
// registry is generated, so a mismatch here means the generator invented something (or a hand
// edit slipped into a generated file), and retry safety in particular must never be inferred.
func TestRouteRegistryMatchesTheContractFieldForField(t *testing.T) {
	contract := fixtures.LoadContract(t)
	skip := regexp.MustCompile(`^/(healthz|readyz|docs|openapi\.json|internal)`)
	checked := 0
	for _, declared := range contract.Routes {
		if skip.MatchString(declared.Path) {
			continue
		}
		key := declared.Method + " " + declared.Path
		spec, ok := Routes[key]
		if !ok {
			t.Errorf("the core declares %s but the SDK does not know it", key)
			continue
		}
		if declared.Safe == nil {
			t.Fatalf("%s carries no \"safe\" field: the contract snapshot must declare retry safety for every route", key)
		}
		for _, mismatch := range routeMismatches(spec, declared) {
			t.Errorf("%s: %s", key, mismatch)
		}
		checked++
	}
	if checked != len(Routes) {
		t.Errorf("checked %d routes, the registry has %d", checked, len(Routes))
	}

	// The comparison has to be able to fail: flip one flag and it must be caught.
	flipped := fixtures.Route{
		Method: "POST", Path: "/v1/payout", Auth: "payout",
		Idempotent: true, Safe: boolPtr(true), Bare: false, List: "",
	}
	if got := routeMismatches(Routes["POST /v1/payout"], flipped); len(got) != 1 {
		t.Fatalf("a flipped safe flag must be reported exactly once, got %v", got)
	}
}

func boolPtr(v bool) *bool { return &v }

// routeMismatches compares one generated route with the contract row it came from.
func routeMismatches(spec Route, declared fixtures.Route) []string {
	var out []string
	report := func(field string, got, want any) {
		out = append(out, fmt.Sprintf("%s = %v, contract says %v", field, got, want))
	}
	if spec.Method != declared.Method {
		report("method", spec.Method, declared.Method)
	}
	if spec.Path != declared.Path {
		report("path", spec.Path, declared.Path)
	}
	if string(spec.Auth) != declared.Auth {
		report("auth", spec.Auth, declared.Auth)
	}
	if spec.Idempotent != declared.Idempotent {
		report("idempotent", spec.Idempotent, declared.Idempotent)
	}
	if declared.Safe != nil && spec.Safe != *declared.Safe {
		report("safe", spec.Safe, *declared.Safe)
	}
	if spec.Bare != declared.Bare {
		report("bare", spec.Bare, declared.Bare)
	}
	if string(spec.List) != declared.List {
		report("list", spec.List, declared.List)
	}
	return out
}

func TestEveryRecordedFixtureBelongsToAKnownRoute(t *testing.T) {
	for _, route := range fixtures.FixtureRoutes(t) {
		if _, ok := Routes[route]; !ok {
			t.Errorf("a response body was recorded for %s, which the SDK does not know", route)
		}
	}
}

func TestEveryRouteHasAMethodWiredToIt(t *testing.T) {
	table := coverage()
	for key := range Routes {
		if _, ok := table[key]; !ok {
			t.Fatalf("%s has no SDK method: add one and list it in coverage()", key)
		}
	}
	for key := range table {
		if _, ok := Routes[key]; !ok {
			t.Fatalf("coverage() lists %s, which is not a route", key)
		}
	}

	for _, key := range RouteKeys {
		spec := Routes[key]
		t.Run(key, func(t *testing.T) {
			body := any(map[string]any{
				"items":    []any{},
				"paginate": map[string]any{"total": 0, "per_page": 1, "offset": 0, "has_pages": false},
				"enabled":  true,
			})
			answer := ok(body)
			if spec.Bare {
				answer = step{status: 200, body: "%PDF", headers: map[string]string{"Content-Type": "application/pdf"}}
			}
			api := newFakeAPI(t, answer)
			client := api.client(
				WithPayoutCredentials("wk_test_1", "secret-2"),
				WithAdminToken("adm"),
			)
			if err := table[key](context.Background(), client); err != nil {
				t.Fatalf("%s: %v", key, err)
			}
			if api.count() != 1 {
				t.Fatalf("%s made %d requests, want exactly 1", key, api.count())
			}
			req := api.last()
			if req.method != spec.Method {
				t.Errorf("method = %s, want %s", req.method, spec.Method)
			}
			pattern := regexp.MustCompile("^" + regexp.MustCompile(`\{[a-z_]+\}`).ReplaceAllString(spec.Path, "[^/]+") + "$")
			if !pattern.MatchString(req.path) {
				t.Errorf("path = %s, want %s", req.path, spec.Path)
			}
			switch spec.Auth {
			case AuthPublic:
				if req.header.Get(HeaderSignature) != "" {
					t.Error("a public route must not be signed")
				}
			case AuthOnboard:
				if req.header.Get(HeaderSignature) != "" {
					t.Error("an onboarding route must not be signed")
				}
				if req.header.Get(HeaderAdminToken) != "adm" {
					t.Error("an onboarding route must carry the admin token")
				}
			default:
				want := "pk_test_1"
				if spec.Auth == AuthPayout {
					want = "wk_test_1"
				}
				if got := req.header.Get(HeaderPublicID); got != want {
					t.Errorf("X-Public-Id = %q, want %q (auth %s)", got, want, spec.Auth)
				}
				if !hexish(req.header.Get(HeaderSignature), 32) {
					t.Errorf("X-Signature = %q", req.header.Get(HeaderSignature))
				}
			}
			idempotencyKey := req.header.Get(HeaderIdempotencyKey)
			if spec.Idempotent && idempotencyKey == "" {
				t.Error("an idempotent route must carry a generated Idempotency-Key")
			}
			if !spec.Idempotent && idempotencyKey != "" {
				t.Errorf("a non-deduplicated route must not carry an Idempotency-Key, got %q", idempotencyKey)
			}
		})
	}
}

func TestRecordedRequestsOnlyUseDocumentedFields(t *testing.T) {
	contract := fixtures.LoadContract(t)
	schemas := map[string]*fixtures.Schema{}
	for _, r := range contract.Routes {
		schemas[r.Method+" "+r.Path] = r.RequestSchema
	}
	for route, fixture := range fixtures.LoadFixtures(t) {
		schema := schemas[route]
		if schema == nil || len(schema.Properties) == 0 || len(fixture.Request) == 0 {
			continue
		}
		sent := map[string]any{}
		if err := json.Unmarshal(fixture.Request, &sent); err != nil {
			continue // the recording did not capture an object body
		}
		for field := range sent {
			if _, documented := schema.Properties[field]; !documented {
				t.Errorf("%s: the recorded journey sent an undocumented field %q", route, field)
			}
		}
	}
}
