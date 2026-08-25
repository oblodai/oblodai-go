package main

import (
	"fmt"
	"sort"
	"strings"
)

// Vocabularies the core exports, and the Go type each becomes. Every value is a typed string
// constant: the compiler autocompletes the vocabulary while a value newer than this snapshot
// still converts (the underlying type is string).
var enumTypes = []struct{ key, name, doc string }{
	{"payment_status", "PaymentStatus", "is the lifecycle state of an invoice: select -> created -> confirm_check -> paid | paid_over | wrong_amount | expired | cancelled."},
	{"payout_status", "PayoutStatus", "is the lifecycle state of a payout: pending -> approved -> awaiting_cosign -> broadcasting -> sent -> confirmed | failed | cancelled."},
	{"payout_link_status", "PayoutLinkStatus", "is the state of a payout link (cheque)."},
	{"delivery_status", "DeliveryStatus", "is the state of a webhook delivery."},
	{"network", "Network", "is a blockchain network the platform settles on."},
	{"fee_bearer", "FeeBearer", "is who pays the network fee on a payout or payout link."},
	{"fee_bearer_result", "FeeBearerResult", "is who actually bore the network fee, as reported back."},
	{"batch_on_error", "BatchOnError", "is how an asynchronous batch reacts to a failing element."},
	{"webhook_kind", "WebhookKind", "is the kind of event a test webhook delivers."},
	{"error_kind", "ErrorKind", "is the core's own classification of an error envelope."},
}

// Vocabularies the core does not export as enums yet; pinned here from its handlers.
var localEnums = []struct {
	name, doc string
	values    []string
}{
	{"AmountMode", "is how a payment link prices its invoices.", []string{"fixed", "open", "range"}},
}

func emitEnums(c *Contract) []byte {
	var b strings.Builder
	b.WriteString(header(c.CoreCommit))

	for _, e := range enumTypes {
		values, ok := c.Enums[e.key]
		if !ok {
			must(fmt.Errorf("enum %q missing from contract.json", e.key))
		}
		emitEnum(&b, e.name, e.doc, values)
	}
	for _, e := range localEnums {
		emitEnum(&b, e.name, e.doc, e.values)
	}

	b.WriteString(comment("", "EventType", "is a webhook event name: invoice.<status>, payout.<status> or wallet.paid."))
	b.WriteString("type EventType string\n\nconst (\n")
	for _, v := range c.EventTypes {
		fmt.Fprintf(&b, "\tEventType%s EventType = %q\n", goName(v), v)
	}
	b.WriteString(")\n\n")
	b.WriteString("// EventTypes lists every webhook event type the core can deliver.\nvar EventTypes = []EventType{\n")
	for _, v := range c.EventTypes {
		fmt.Fprintf(&b, "\tEventType%s,\n", goName(v))
	}
	b.WriteString("}\n\n")

	b.WriteString("// ErrorCodes is every error code the core source can emit, as family.reason. The code is the\n")
	b.WriteString("// stable discriminator of a failure; the HTTP status only groups codes into kinds.\nvar ErrorCodes = []string{\n")
	codes := append([]string(nil), c.ErrorCodes...)
	sort.Strings(codes)
	for i := 0; i < len(codes); i += 4 {
		end := min(i+4, len(codes))
		quoted := make([]string, 0, 4)
		for _, code := range codes[i:end] {
			quoted = append(quoted, fmt.Sprintf("%q", code))
		}
		fmt.Fprintf(&b, "\t%s,\n", strings.Join(quoted, ", "))
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

func emitEnum(b *strings.Builder, name, doc string, values []string) {
	b.WriteString(comment("", name, doc))
	fmt.Fprintf(b, "type %s string\n\nconst (\n", name)
	for _, v := range values {
		fmt.Fprintf(b, "\t%s%s %s = %q\n", name, goName(v), name, v)
	}
	b.WriteString(")\n\n")
	plural := name + "s"
	if strings.HasSuffix(name, "s") {
		plural = name + "es"
	}
	fmt.Fprintf(b, "// %s lists every value of %s this contract snapshot declares.\nvar %s = []%s{\n", plural, name, plural, name)
	for _, v := range values {
		fmt.Fprintf(b, "\t%s%s,\n", name, goName(v))
	}
	b.WriteString("}\n\n")
}
