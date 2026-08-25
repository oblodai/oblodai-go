package main

import (
	"regexp"
	"strings"
)

// Go naming for wire identifiers. Wire names are snake_case; Go wants CamelCase with the usual
// initialisms upper-cased, and the mapping has to be deterministic so the generated files are
// stable across runs (the drift gate compares bytes).

var initialisms = map[string]string{
	"api":   "API",
	"bps":   "BPS",
	"cidr":  "CIDR",
	"csv":   "CSV",
	"http":  "HTTP",
	"id":    "ID",
	"ip":    "IP",
	"json":  "JSON",
	"pdf":   "PDF",
	"qr":    "QR",
	"ttl":   "TTL",
	"txid":  "TxID",
	"uri":   "URI",
	"url":   "URL",
	"uuid":  "UUID",
	"vrcs":  "VRCS",
	"xaddr": "XAddr",
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// goName turns a wire identifier ("payer_address_is_refundable") into an exported Go name.
func goName(wire string) string {
	var b strings.Builder
	for _, word := range nonAlnum.Split(wire, -1) {
		if word == "" {
			continue
		}
		lower := strings.ToLower(word)
		if up, ok := initialisms[lower]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(lower[:1]))
		b.WriteString(lower[1:])
	}
	return b.String()
}

// routeBaseName is the Go name stem of a route: its path without the version prefix and without
// path parameters ("/v1/payment/link/batch" -> "PaymentLinkBatch").
func routeBaseName(path string) string {
	var b strings.Builder
	for _, seg := range strings.Split(path, "/") {
		if seg == "" || seg == "v1" || strings.HasPrefix(seg, "{") {
			continue
		}
		b.WriteString(goName(seg))
	}
	return b.String()
}

// singular trims a trailing plural "s" so an array field yields a readable element type name
// ("payments" -> "Payment"). Words that are not simple plurals are left alone.
func singular(name string) string {
	switch {
	case strings.HasSuffix(name, "ies"):
		return strings.TrimSuffix(name, "ies") + "y"
	case strings.HasSuffix(name, "ss"), !strings.HasSuffix(name, "s"):
		return name
	default:
		return strings.TrimSuffix(name, "s")
	}
}

// comment renders a doc comment block, wrapped, indented and prefixed with the Go name so it
// reads like ordinary Go documentation.
func comment(indent, name, text string) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if text == "" {
		return ""
	}
	if name != "" {
		text = name + " " + text
	}
	const width = 100
	var out strings.Builder
	line := indent + "//"
	for _, word := range strings.Fields(text) {
		if len(line)+1+len(word) > width && line != indent+"//" {
			out.WriteString(line + "\n")
			line = indent + "//"
		}
		line += " " + word
	}
	out.WriteString(line + "\n")
	return out.String()
}
