// Command codegen writes the generated half of the SDK from the contract snapshot the core
// exports: contract/contract.json (routes, enums, error codes, signing vectors) and
// contract/descriptions.en.json (English field documentation).
//
// Generated files:
//
//	contract_routes.go    every merchant-facing route with its auth gate, idempotency and list kind
//	contract_enums.go     status/network/fee vocabularies, event types, error codes
//	contract_requests.go  a request struct per route the core documents a DTO for
//	contract_version.go   the contract commit, export time and hash
//
// Run it with `go generate ./...`; `go run ./internal/codegen -check` fails when the committed
// files differ from what the current contract.json produces.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Route is one row of the core's conformance table.
type Route struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Auth       string `json:"auth"`
	Idempotent bool   `json:"idempotent"`
	// Safe is the core's own read-only classification: a pointer so a contract that predates the
	// field fails codegen instead of silently generating "not safe to repeat" for every route.
	Safe            *bool           `json:"safe"`
	Bare            bool            `json:"bare"`
	List            string          `json:"list"`
	RequestSchema   *Schema         `json:"request_schema"`
	ResponseSchema  json.RawMessage `json:"response_schema"`
	baseName, key   string
	requestTypeName string
}

// Schema is the slice of JSON Schema the core exports for request DTOs.
type Schema struct {
	Type                 string             `json:"type"`
	Description          string             `json:"description"`
	Example              any                `json:"example"`
	Properties           map[string]*Schema `json:"properties"`
	Required             []string           `json:"required"`
	Items                *Schema            `json:"items"`
	AdditionalProperties *Schema            `json:"additionalProperties"`
}

// Contract is contract.json as far as codegen cares.
type Contract struct {
	CoreCommit string              `json:"core_commit"`
	ExportedAt string              `json:"exported_at"`
	Routes     []*Route            `json:"routes"`
	Enums      map[string][]string `json:"enums"`
	ErrorCodes []string            `json:"error_codes"`
	EventTypes []string            `json:"event_types"`
}

// Descriptions is the English documentation snapshot: route -> dotted field path -> text.
type Descriptions struct {
	Request  map[string]map[string]string `json:"request"`
	Response map[string]map[string]string `json:"response"`
}

// Routes outside the merchant surface: health probes, docs, internal endpoints.
var skipPath = regexp.MustCompile(`^/(healthz|readyz|docs|openapi\.json|internal)`)

func main() {
	check := flag.Bool("check", false, "fail when the committed generated files are out of date")
	flag.Parse()

	root := moduleRoot()
	raw, err := os.ReadFile(filepath.Join(root, "contract", "contract.json"))
	must(err)
	var contract Contract
	must(json.Unmarshal(raw, &contract))

	var desc Descriptions
	descRaw, err := os.ReadFile(filepath.Join(root, "contract", "descriptions.en.json"))
	must(err)
	must(json.Unmarshal(descRaw, &desc))

	routes := prepare(contract.Routes)
	files := map[string][]byte{
		"contract_routes.go":   emitRoutes(routes, contract.CoreCommit),
		"contract_enums.go":    emitEnums(&contract),
		"contract_requests.go": emitRequests(routes, &desc, contract.CoreCommit),
		"contract_version.go":  emitVersion(&contract, sha256.Sum256(raw)),
	}

	drifted := []string{}
	for name, body := range files {
		formatted, err := format.Source(body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "codegen: %s does not parse: %v\n", name, err)
			os.Exit(1)
		}
		path := filepath.Join(root, name)
		if *check {
			old, err := os.ReadFile(path)
			if err != nil || string(old) != string(formatted) {
				drifted = append(drifted, name)
			}
			continue
		}
		must(os.WriteFile(path, formatted, 0o644))
	}
	if *check {
		sort.Strings(drifted)
		if len(drifted) > 0 {
			fmt.Fprintf(os.Stderr, "contract drift: %s differ from contract/contract.json — run `go generate ./...` and commit\n",
				strings.Join(drifted, ", "))
			os.Exit(1)
		}
		fmt.Printf("check-drift: %d generated files are in sync with contract %s\n", len(files), contract.CoreCommit[:12])
		return
	}
	fmt.Printf("codegen: %d routes, %d error codes, contract %s\n", len(routes), len(contract.ErrorCodes), contract.CoreCommit[:12])
}

// prepare filters the merchant surface, sorts it the way the reference SDK keys it, and assigns
// each route its Go name stem (unique: a GET and a POST on one path differ by method prefix).
func prepare(all []*Route) []*Route {
	out := make([]*Route, 0, len(all))
	for _, r := range all {
		if skipPath.MatchString(r.Path) {
			continue
		}
		r.key = r.Method + " " + r.Path
		if r.Safe == nil {
			// Retry safety is never inferred here. The core hand-classifies every route; a snapshot
			// without the flag would make this SDK guess which writes are safe to re-send.
			fmt.Fprintf(os.Stderr, "codegen: route %s has no \"safe\" field — re-export contract/contract.json from a core that declares it\n", r.key)
			os.Exit(1)
		}
		r.baseName = routeBaseName(r.Path)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Method < out[j].Method
		}
		return out[i].Path < out[j].Path
	})
	seen := map[string]int{}
	for _, r := range out {
		seen[r.baseName]++
	}
	for _, r := range out {
		if seen[r.baseName] > 1 {
			r.baseName = goName(strings.ToLower(r.Method)) + r.baseName
		}
		r.requestTypeName = r.baseName + "Params"
	}
	return out
}

func header(coreCommit string) string {
	return fmt.Sprintf("// Code generated by internal/codegen from contract/contract.json (core %s). DO NOT EDIT.\n\npackage oblodai\n\n", coreCommit[:12])
}

func emitRoutes(routes []*Route, coreCommit string) []byte {
	var b strings.Builder
	b.WriteString(header(coreCommit))
	b.WriteString("// Routes is every merchant-facing route the core declares, keyed exactly as its conformance\n")
	b.WriteString("// table keys them (\"POST /v1/payment\"). A route the core does not declare cannot be called.\n")
	b.WriteString("var Routes = map[string]Route{\n")
	for _, r := range routes {
		list := "ListNone"
		switch r.List {
		case "paged":
			list = "ListPaged"
		case "plain":
			list = "ListPlain"
		}
		fmt.Fprintf(&b, "\t%q: {Method: %q, Path: %q, Auth: Auth%s, Idempotent: %t, Safe: %t, Bare: %t, List: %s},\n",
			r.key, r.Method, r.Path, goName(r.Auth), r.Idempotent, *r.Safe, r.Bare, list)
	}
	b.WriteString("}\n\n")
	b.WriteString("// RouteKeys lists every key of Routes in a stable order.\nvar RouteKeys = []string{\n")
	for _, r := range routes {
		fmt.Fprintf(&b, "\t%q,\n", r.key)
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

func emitVersion(c *Contract, sum [32]byte) []byte {
	var b strings.Builder
	b.WriteString(header(c.CoreCommit))
	b.WriteString("const (\n")
	b.WriteString("\t// ContractCoreCommit is the core revision the contract snapshot was exported from.\n")
	fmt.Fprintf(&b, "\tContractCoreCommit = %q\n", c.CoreCommit)
	b.WriteString("\t// ContractExportedAt is when that snapshot was taken.\n")
	fmt.Fprintf(&b, "\tContractExportedAt = %q\n", c.ExportedAt)
	b.WriteString("\t// ContractHash is the SHA-256 of contract/contract.json; it identifies the wire contract.\n")
	fmt.Fprintf(&b, "\tContractHash = %q\n", hex.EncodeToString(sum[:]))
	b.WriteString(")\n")
	return []byte(b.String())
}

func moduleRoot() string {
	dir, err := os.Getwd()
	must(err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("codegen: go.mod not found above the working directory")
		}
		dir = parent
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "codegen:", err)
		os.Exit(1)
	}
}
