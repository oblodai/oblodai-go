package webhooks_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/oblodai/oblodai-go/v2"
	"github.com/oblodai/oblodai-go/v2/internal/conformance"
	"github.com/oblodai/oblodai-go/v2/webhooks"
)

// The webhook part of the shared conformance suite (backend tools/sdkgen/conformance): the core's
// webhook vectors sign and verify here exactly as the core does.

type webhookVector struct {
	Secret    string `json:"secret"`
	TS        int64  `json:"ts"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func TestConformanceWebhooks(t *testing.T) {
	suite := conformance.Load(t, "webhook")
	vectors, skew := conformance.Vectors[webhookVector](t, suite)
	names := conformance.Names(t, suite)
	for _, check := range suite.Checks {
		for i, v := range vectors {
			t.Run(check.Name+"#"+strconv.Itoa(i), func(t *testing.T) {
				if check.Kind == "webhook_signature" {
					if got := oblodai.SignWebhook(v.Secret, v.TS, []byte(v.Payload)); got != v.Signature {
						t.Fatalf("signature\n got %s\nwant %s", got, v.Signature)
					}
					return
				}
				if check.Kind != "webhook_verify" {
					t.Fatalf("unknown check kind %q", check.Kind)
				}
				payload, signature := v.Payload, v.Signature
				switch check.Mutate {
				case "payload":
					payload += " "
				case "signature":
					first := "0"
					if signature[0] == '0' {
						first = "1"
					}
					signature = first + signature[1:]
				}
				header := http.Header{}
				// The spec's names, not this package's constants: a renamed header the SDK does not
				// know fails here.
				header.Set(names["timestamp"], strconv.FormatInt(v.TS, 10))
				header.Set(names["signature"], signature)
				now := v.TS + conformance.Offset(t, check.NowFromTS, skew)
				_, err := webhooks.Verify([]byte(payload), header, webhooks.Options{
					Secret:    v.Secret,
					Tolerance: time.Duration(skew) * time.Second,
					Now:       func() time.Time { return time.Unix(now, 0) },
				})
				if check.Expect == "ok" {
					// The vectors sign bare payloads, not whole events: past the MAC and the
					// freshness window a refusal to parse the body is still a pass.
					if err != nil && !oblodai.IsWebhookPayload(err) {
						t.Fatalf("want accepted, got %v", err)
					}
					return
				}
				if !oblodai.IsCode(err, "webhook."+check.Expect) {
					t.Fatalf("want webhook.%s, got %v", check.Expect, err)
				}
			})
		}
	}
}

type deliveryVector struct {
	Event          string            `json:"event"`
	Kind           string            `json:"kind"`
	Secret         string            `json:"secret"`
	PreviousSecret string            `json:"previous_secret"`
	TS             int64             `json:"ts"`
	Payload        string            `json:"payload"`
	Headers        map[string]string `json:"headers"`
}

// A real delivery of every event of the contract verifies (with the current secret and, as a
// receiver that has not swapped yet, with the previous one), parses into its kind's typed body and
// exposes every delivery header of the spec; with the spec's rehearsal header it is a test, without
// it a live one.
func TestConformanceWebhookDeliveries(t *testing.T) {
	suite := conformance.Load(t, "webhook_delivery")
	vectors, _ := conformance.Vectors[deliveryVector](t, suite)
	names := conformance.Names(t, suite)
	testHeader := conformance.TestHeader(t, suite)
	dedupeField := conformance.DedupeField(t, suite)
	if dedupeField != webhooks.EventIDField || suite.DedupeKey.Fallback != "type:id:sequence" {
		t.Fatalf("dedupe_key %s / %q, the SDK keys on %s / type:id:sequence", dedupeField, suite.DedupeKey.Fallback, webhooks.EventIDField)
	}
	if !suite.FieldsUnverified {
		t.Fatal("fields_unverified: the SDK exposes the header fields only as Delivery.Unverified")
	}
	if len(suite.Fields) != len(names) {
		t.Fatalf("fields %v, header roles %v", suite.Fields, names)
	}
	events := map[string]bool{}
	for _, v := range vectors {
		events[v.Event] = true
	}
	for name := range webhooks.EventKinds {
		if !events[name] {
			t.Errorf("no delivery of %s", name)
		}
	}
	if len(events) != len(webhooks.EventKinds) {
		t.Errorf("%d deliveries, %d known events", len(events), len(webhooks.EventKinds))
	}
	for _, check := range suite.Checks {
		if check.Kind != "webhook_delivery" {
			t.Fatalf("unknown check kind %q", check.Kind)
		}
		for _, v := range vectors {
			t.Run(check.Name+"/"+v.Event+"/"+check.Key, func(t *testing.T) {
				secret := map[string]string{"current": v.Secret, "previous": v.PreviousSecret}[check.Key]
				if secret == "" {
					t.Fatalf("key %q", check.Key)
				}
				header := http.Header{}
				for k, val := range v.Headers {
					header.Set(k, val)
				}
				if check.Test {
					header.Set(testHeader, "true")
				}
				delivery, err := webhooks.VerifyDelivery([]byte(v.Payload), header, webhooks.Options{
					Secret: secret,
					Now:    func() time.Time { return time.Unix(v.TS, 0) },
				})
				if err != nil {
					t.Fatal(err)
				}
				if !delivery.Event.IsKnown() || delivery.Event.Type != v.Kind || webhooks.EventKinds[v.Event] != v.Kind {
					t.Fatalf("event %s parsed as kind %q (known %v), want %s", v.Event, delivery.Event.Type,
						delivery.Event.IsKnown(), v.Kind)
				}
				if delivery.Event.ID() == "" {
					t.Fatal("no object id")
				}
				// The rehearsal header is not signed: it is reported only as the unverified header,
				// and the verified flag follows the signed body — live in every one of these vectors.
				if delivery.Unverified.Test != check.Test {
					t.Fatalf("Unverified.Test = %v, want %v (rehearsal header %s)", delivery.Unverified.Test, check.Test, testHeader)
				}
				if delivery.IsTest || delivery.IsTest != delivery.Event.IsTest() {
					t.Fatalf("IsTest = %v on a live body (signed test = %v)", delivery.IsTest, delivery.Event.IsTest())
				}
				// dedupe_key: the signed body field the spec names, else the suite's fallback.
				var body map[string]any
				if err := json.Unmarshal([]byte(v.Payload), &body); err != nil {
					t.Fatal(err)
				}
				want := delivery.Event.Type + ":" + delivery.Event.ID() + ":" + strconv.FormatInt(delivery.Event.Sequence(), 10)
				if id, ok := body[dedupeField].(string); ok {
					want = id
					if eventIDHeader := v.Headers[names["event_id"]]; id != eventIDHeader {
						t.Fatalf("body %s %q differs from the header %q", dedupeField, id, eventIDHeader)
					}
				}
				if delivery.EventKey != want || delivery.Event.Key() != want {
					t.Fatalf("EventKey = %q, want %q (dedupe_key)", delivery.EventKey, want)
				}
				for role, field := range suite.Fields {
					name, ok := names[role]
					if !ok {
						t.Fatalf("field %q has unknown header role %q", field, role)
					}
					var got string
					switch field {
					case "":
						continue
					case "id":
						got = delivery.Unverified.ID
					case "event_id":
						got = delivery.Unverified.EventID
					case "event_type":
						got = string(delivery.Unverified.EventType)
					case "event_time":
						got = strconv.FormatInt(delivery.Unverified.EventTime.Unix(), 10)
					case "sent_at":
						got = strconv.FormatInt(delivery.SentAt.Unix(), 10)
					default:
						t.Fatalf("the delivery has no field %q for %s", field, name)
					}
					if v.Headers[name] == "" {
						t.Fatalf("the delivery vector has no %s header (role %s)", name, role)
					}
					if got != v.Headers[name] {
						t.Errorf("%s = %q, header %s = %q", field, got, name, v.Headers[name])
					}
				}
			})
		}
	}
}

// forward_compat webhooks: a body parses, keeps its raw type, and is known exactly when expected —
// a kind newer than this release is delivered, not refused.
func TestConformanceWebhookParse(t *testing.T) {
	suite := conformance.Load(t, "forward_compat")
	if len(suite.Webhooks) == 0 {
		t.Fatal("forward_compat.json has no webhook bodies")
	}
	for _, c := range suite.Webhooks {
		t.Run(c.Name, func(t *testing.T) {
			event, err := webhooks.Parse(c.Body)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if event.Type != c.Expect.Type || event.IsKnown() != c.Expect.Known {
				t.Fatalf("type %q known %v, want %q known %v", event.Type, event.IsKnown(), c.Expect.Type, c.Expect.Known)
			}
		})
	}
}
