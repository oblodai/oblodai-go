package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Request schemas for routes whose core DTO is not declared in docsapi (kept in one place so a
// future undocumented route has somewhere to go; remove an entry once the core documents it).
var requestOverrides = map[string]*Schema{
	"POST /v1/merchants": {
		Type:     "object",
		Required: []string{"email"},
		Properties: map[string]*Schema{
			"email": {Type: "string", Example: "owner@shop.example"},
			"name":  {Type: "string", Example: "Acme"},
		},
	},
}

// Fields the handler requires although the shared DTO marks them optional (batch items reuse the
// single-create DTO, where the core backfills the key from the Idempotency-Key header).
var requiredOverrides = map[string][]string{
	"POST /v1/payment/batch":     {"payments.order_id"},
	"POST /v1/payout/batch":      {"payouts.order_id"},
	"POST /v1/refund/batch":      {"refunds.reference"},
	"POST /v1/transfer/batch":    {"transfers.order_id", "transfers.amount", "transfers.currency"},
	"POST /v1/payout/link/batch": {"items.reference"},
	"POST /v1/transfer/to-user":  {"amount", "currency"},
	"POST /v1/claim/{token}":     {"address"},
}

// Fields drawn from a generated vocabulary get its typed string type.
var fieldEnums = map[string]string{
	"network":        "Network",
	"pinned_network": "Network",
	"on_error":       "BatchOnError",
	"fee_bearer":     "FeeBearer",
	"amount_mode":    "AmountMode",
}

var routeFieldEnums = map[string]string{
	"POST /v1/payment/history#status":         "PaymentStatus",
	"POST /v1/payout/history#status":          "PayoutStatus",
	"POST /v1/test-webhook/payment#status":    "PaymentStatus",
	"POST /v1/test-webhook/payout#status":     "PayoutStatus",
	"POST /v1/payment/testing-webhook#status": "PaymentStatus",
}

// deferred is a nested struct type discovered while emitting a request body.
type deferred struct {
	name   string
	schema *Schema
	route  string
	prefix string
}

func emitRequests(routes []*Route, desc *Descriptions, coreCommit string) []byte {
	var b strings.Builder
	b.WriteString(header(coreCommit))
	b.WriteString("// Request bodies, one struct per route the core documents a DTO for. Field names, required\n")
	b.WriteString("// flags, examples and documentation all come from the contract snapshot: an optional field is\n")
	b.WriteString("// omitted from the JSON when empty, a required one is always sent.\n\n")

	for _, r := range routes {
		schema := r.RequestSchema
		if schema == nil {
			schema = requestOverrides[r.key]
		}
		if schema == nil || len(schema.Properties) == 0 {
			continue
		}
		queue := []deferred{{name: r.requestTypeName, schema: schema, route: r.key, prefix: ""}}
		for len(queue) > 0 {
			d := queue[0]
			queue = queue[1:]
			doc := fmt.Sprintf("is the request body of %s.", r.key)
			if d.prefix != "" {
				doc = fmt.Sprintf("is an element of %s in %s.", strings.TrimSuffix(d.prefix, "."), r.key)
			}
			b.WriteString(comment("", d.name, doc))
			fmt.Fprintf(&b, "type %s struct {\n", d.name)
			queue = append(queue, emitFields(&b, d, desc)...)
			b.WriteString("}\n\n")
		}
	}
	return []byte(b.String())
}

// emitFields writes one struct body and returns the nested struct types it referenced.
func emitFields(b *strings.Builder, d deferred, desc *Descriptions) []deferred {
	required := map[string]bool{}
	for _, name := range d.schema.Required {
		required[name] = true
	}
	for _, path := range requiredOverrides[d.route] {
		if strings.HasPrefix(path, d.prefix) {
			rest := strings.TrimPrefix(path, d.prefix)
			if !strings.Contains(rest, ".") {
				required[rest] = true
			}
		}
	}

	names := make([]string, 0, len(d.schema.Properties))
	for name := range d.schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	var nested []deferred
	for i, wire := range names {
		prop := d.schema.Properties[wire]
		field := goName(wire)
		typ, sub := goType(prop, d, wire, field)
		nested = append(nested, sub...)
		opt := !required[wire]
		if opt {
			typ = optionalType(typ)
		}
		tag := wire
		if opt {
			tag += ",omitempty"
		}
		if doc := fieldDoc(desc, d, wire, prop, required[wire]); doc != "" {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(comment("\t", "", doc))
		}
		fmt.Fprintf(b, "\t%s %s `json:%q`\n", field, typ, tag)
	}
	return nested
}

// goType maps one schema node to a Go type, queueing nested structs for later emission.
func goType(s *Schema, d deferred, wire, field string) (string, []deferred) {
	switch s.Type {
	case "string":
		if enum, ok := routeFieldEnums[d.route+"#"+wire]; ok {
			return enum, nil
		}
		if enum, ok := fieldEnums[wire]; ok {
			return enum, nil
		}
		if isMoneyField(wire) {
			return "Money", nil
		}
		return "string", nil
	case "integer":
		return "int", nil
	case "number":
		return "float64", nil
	case "boolean":
		return "bool", nil
	case "array":
		if s.Items == nil {
			return "[]any", nil
		}
		name := d.name + singular(field) + "Item"
		if s.Items.Type == "object" && len(s.Items.Properties) > 0 {
			return "[]" + name, []deferred{{name: name, schema: s.Items, route: d.route, prefix: d.prefix + wire + "."}}
		}
		inner, sub := goType(s.Items, d, wire, field)
		return "[]" + inner, sub
	case "object":
		if len(s.Properties) > 0 {
			name := d.name + field
			return name, []deferred{{name: name, schema: s, route: d.route, prefix: d.prefix + wire + "."}}
		}
		if s.AdditionalProperties != nil {
			inner, sub := goType(s.AdditionalProperties, d, wire, field)
			return "map[string]" + inner, sub
		}
		return "map[string]any", nil
	default:
		return "any", nil
	}
}

// optionalType wraps scalars whose zero value is meaningful on the wire (false, 0) in a pointer,
// so "not set" and "set to the zero value" stay distinguishable. Strings, slices and maps carry
// that distinction already through omitempty.
func optionalType(typ string) string {
	switch typ {
	case "bool", "int", "float64":
		return "*" + typ
	default:
		return typ
	}
}

// isASCII reports whether a string is plain ASCII, the only thing generated documentation carries.
func isASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] > 0x7e || text[i] < 0x09 {
			return false
		}
	}
	return true
}

func isMoneyField(wire string) bool {
	return wire == "amount" || wire == "amount_fixed" || strings.HasSuffix(wire, "_amount")
}

// fieldDoc composes the English documentation of one field: the contract description, whether it
// is required, and the example the core documents.
func fieldDoc(desc *Descriptions, d deferred, wire string, s *Schema, required bool) string {
	text := strings.TrimSpace(desc.Request[d.route][d.prefix+wire])
	parts := []string{}
	if text != "" {
		parts = append(parts, text)
	}
	if required {
		parts = append(parts, "Required.")
	}
	if s.Example != nil {
		if example, err := json.Marshal(s.Example); err == nil && isASCII(string(example)) {
			// The core's examples are written for its own docs and some are Russian. Generated Go
			// documentation is English only, so a non-ASCII example is dropped rather than copied:
			// the field description above it already says what the value means.
			parts = append(parts, fmt.Sprintf("Example: %s.", example))
		}
	}
	return strings.Join(parts, " ")
}
