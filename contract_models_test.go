package oblodai

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/oblodai/oblodai-go/v2/internal/fixtures"
)

// The wire models against the golden bodies the core recorded. Two rules, both one-directional
// failures of the same drift:
//
//   - every key the core sent must exist in the model — otherwise a field silently disappears
//     from a user's program the day the core starts sending it;
//   - every model field that is not `omitempty` must be present on the wire — otherwise the model
//     promises a field the core no longer sends.
//
// Optional (omitempty) fields may be absent: that is what optional means.

// Routes the API guarantees to refuse for API keys, so no success body exists to model.
var notModelled = map[string]bool{
	// API-key payouts approve themselves; approve serves the cabinet's maker-checker flow.
	"POST /v1/payout/approve": true,
}

func TestModelsMatchTheGoldenBodies(t *testing.T) {
	recorded := fixtures.LoadFixtures(t)
	for _, row := range modelRows() {
		name := row.route
		if row.path != "" {
			name += " " + row.path
		}
		t.Run(name, func(t *testing.T) {
			fixture, ok := recorded[row.route]
			if !ok {
				t.Fatalf("no recorded body for %s", row.route)
			}
			if fixture.Status < 200 || fixture.Status >= 300 {
				t.Skipf("%s was recorded as a refusal (HTTP %d) in this environment", row.route, fixture.Status)
			}
			body, err := walk(fixture.Response.Result, row.path)
			if err != nil {
				t.Fatalf("%s: %v", row.route, err)
			}

			// The model must decode the recorded body without losing its footing.
			decoded := reflect.New(reflect.TypeOf(row.model))
			if err := json.Unmarshal(body, decoded.Interface()); err != nil {
				t.Fatalf("%T cannot decode the recorded body: %v", row.model, err)
			}

			wire := map[string]bool{}
			var asObject map[string]json.RawMessage
			if err := json.Unmarshal(body, &asObject); err != nil {
				t.Fatalf("the recorded body at %q is not an object: %v", row.path, err)
			}
			for key := range asObject {
				wire[key] = true
			}
			optional := map[string]bool{}
			for _, key := range row.alsoOptional {
				optional[key] = true
			}

			required, known := modelFields(reflect.TypeOf(row.model))
			for key := range wire {
				if !known[key] {
					t.Errorf("the core sends %q on %s but %T has no field for it", key, row.route, row.model)
				}
			}
			for _, key := range required {
				if !wire[key] && !optional[key] {
					t.Errorf("%T declares %q as always present, but %s did not send it", row.model, key, row.route)
				}
			}
		})
	}
}

func TestEveryRecordedSuccessBodyHasAModel(t *testing.T) {
	covered := map[string]bool{}
	for _, row := range modelRows() {
		covered[row.route] = true
	}
	for route, fixture := range fixtures.LoadFixtures(t) {
		if fixture.Status < 200 || fixture.Status >= 300 || notModelled[route] {
			continue
		}
		if !strings.Contains(fixture.Headers["Content-Type"], "json") {
			continue // a document: bytes, not a model
		}
		if !covered[route] {
			t.Errorf("%s has a recorded success body but no model row", route)
		}
	}
}

// The event models against the deliveries the core's own dispatcher signed, by the same two rules
// as the golden bodies: nothing on the wire may be missing from the model, and nothing the model
// declares as always present may be missing from the wire. `test` is omitempty on purpose — only a
// rehearsal delivery carries it.
func TestWebhookSampleBodiesMatchTheEventModels(t *testing.T) {
	models := map[WebhookKind]any{
		WebhookKindPayment: PaymentEvent{},
		WebhookKindPayout:  PayoutEvent{},
		WebhookKindWallet:  WalletEvent{},
	}
	samples := fixtures.LoadWebhookSamples(t)
	if len(samples) == 0 {
		t.Fatal("no recorded deliveries to check the event models against")
	}
	for i, sample := range samples {
		var wire map[string]json.RawMessage
		if err := json.Unmarshal([]byte(sample.Raw), &wire); err != nil {
			t.Fatalf("delivery %d is not a JSON object: %v", i, err)
		}
		var kind WebhookKind
		decode(t, wire["type"], &kind)
		model, ok := models[kind]
		if !ok {
			t.Fatalf("delivery %d carries the unknown event type %q", i, kind)
		}
		required, known := modelFields(reflect.TypeOf(model))
		for key := range wire {
			if !known[key] {
				t.Errorf("the core sends %q on a %s event but %T has no field for it", key, kind, model)
			}
		}
		for _, key := range required {
			if _, sent := wire[key]; !sent {
				t.Errorf("%T declares %q as always present, but delivery %d did not send it", model, key, i)
			}
		}
	}
}

func TestEnumsCoverWhatTheWireCarries(t *testing.T) {
	recorded := fixtures.LoadFixtures(t)
	vocabulary := func(values any) map[string]bool {
		out := map[string]bool{}
		list := reflect.ValueOf(values)
		for i := 0; i < list.Len(); i++ {
			out[list.Index(i).String()] = true
		}
		return out
	}

	var payments Page[Payment]
	decode(t, recorded["POST /v1/payment/history"].Response.Result, &payments)
	known := vocabulary(PaymentStatuses)
	for _, payment := range payments.Items {
		if !known[string(payment.Status)] {
			t.Errorf("payment status %q is not in the vocabulary", payment.Status)
		}
	}

	var payouts Page[Payout]
	decode(t, recorded["POST /v1/payout/history"].Response.Result, &payouts)
	known = vocabulary(PayoutStatuses)
	for _, payout := range payouts.Items {
		if !known[string(payout.Status)] {
			t.Errorf("payout status %q is not in the vocabulary", payout.Status)
		}
	}

	var links Page[PayoutLink]
	decode(t, recorded["POST /v1/payout/link/list"].Response.Result, &links)
	known = vocabulary(PayoutLinkStatuses)
	for _, link := range links.Items {
		if !known[string(link.Status)] {
			t.Errorf("payout link status %q is not in the vocabulary", link.Status)
		}
	}

	var deliveries Page[WebhookDelivery]
	decode(t, recorded["POST /v1/webhooks/deliveries"].Response.Result, &deliveries)
	known = vocabulary(DeliveryStatuses)
	events := vocabulary(EventTypes)
	for _, delivery := range deliveries.Items {
		if !known[string(delivery.Status)] {
			t.Errorf("delivery status %q is not in the vocabulary", delivery.Status)
		}
		if !events[string(delivery.EventType)] {
			t.Errorf("event type %q is not in the vocabulary", delivery.EventType)
		}
	}
}

func TestRecordedErrorSamplesAreDocumentedCodes(t *testing.T) {
	codes := map[string]bool{}
	for _, code := range ErrorCodes {
		codes[code] = true
	}
	for code, fixture := range fixtures.LoadErrorSamples(t) {
		if !codes[code] {
			t.Errorf("the core emitted %q, which is not in ErrorCodes", code)
		}
		body := fixture.Response.Error
		if body == nil {
			t.Fatalf("%s: the recorded sample carries no error envelope", code)
		}
		var envelopeCode string
		decode(t, body["code"], &envelopeCode)
		if envelopeCode != code {
			t.Errorf("%s: the envelope says %q", code, envelopeCode)
		}
		if _, ok := body["retryable"]; !ok {
			t.Errorf("%s: the envelope has no retryable flag", code)
		}
		if _, ok := body["request_id"]; !ok {
			t.Errorf("%s: the envelope has no request id to quote to support", code)
		}
		// The error decoder must reach the same conclusion from the recorded bytes.
		raw, err := json.Marshal(fixture.Response)
		if err != nil {
			t.Fatal(err)
		}
		_, decoded := decodeEnvelope(fixture.Status, raw, decodeContext{})
		if decoded == nil || decoded.Code != code {
			t.Errorf("%s: the client decoded %v", code, decoded)
		}
		if decoded.Synthetic {
			t.Errorf("%s: an enveloped error must not be classified as synthetic", code)
		}
	}
}

// modelFields returns the json names a struct declares: those that must always be present, and
// the full set. Embedded structs are flattened the way encoding/json flattens them.
func modelFields(t reflect.Type) (required []string, known map[string]bool) {
	known = map[string]bool{}
	var visit func(reflect.Type)
	visit = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("json")
			if field.Anonymous && tag == "" {
				visit(field.Type)
				continue
			}
			parts := strings.Split(tag, ",")
			name := parts[0]
			if name == "" || name == "-" {
				continue
			}
			known[name] = true
			if !slicesContains(parts[1:], "omitempty") {
				required = append(required, name)
			}
		}
	}
	visit(t)
	sort.Strings(required)
	return required, known
}

func slicesContains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// walk follows a dotted path into a JSON document; numeric segments index arrays.
func walk(document json.RawMessage, path string) (json.RawMessage, error) {
	current := document
	if path == "" {
		return current, nil
	}
	for _, segment := range strings.Split(path, ".") {
		if index, err := strconv.Atoi(segment); err == nil {
			var list []json.RawMessage
			if err := json.Unmarshal(current, &list); err != nil {
				return nil, fmt.Errorf("%q: not an array at %q", path, segment)
			}
			if index >= len(list) {
				return nil, fmt.Errorf("%q: the recording has no element %d", path, index)
			}
			current = list[index]
			continue
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(current, &object); err != nil {
			return nil, fmt.Errorf("%q: not an object at %q", path, segment)
		}
		next, ok := object[segment]
		if !ok {
			return nil, fmt.Errorf("%q: the recording has no field %q", path, segment)
		}
		current = next
	}
	return current, nil
}

func decode(t *testing.T, raw json.RawMessage, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("cannot decode %T: %v", into, err)
	}
}
