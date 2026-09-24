package webhooks_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oblodai/oblodai-go/v2"
	"github.com/oblodai/oblodai-go/v2/webhooks"
)

// testdata/deliveries.json holds deliveries the core's own dispatcher sent, recorded byte for byte
// and signed with the endpoint secret in force at that moment. If verification passes here, it
// passes in production.

type recorded struct {
	Headers map[string]string `json:"headers"`
	Raw     string            `json:"raw"`
}

func loadDeliveries(t *testing.T) (string, []recorded) {
	t.Helper()
	data, err := os.ReadFile("testdata/deliveries.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Secret     string     `json:"secret"`
		Deliveries []recorded `json:"deliveries"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if file.Secret == "" || len(file.Deliveries) == 0 {
		t.Fatal("testdata/deliveries.json carries no secret or no deliveries")
	}
	return file.Secret, file.Deliveries
}

func headersOf(sample recorded) http.Header {
	header := http.Header{}
	for name, value := range sample.Headers {
		header.Set(name, value)
	}
	return header
}

func TestVerifyRealDeliveries(t *testing.T) {
	secret, samples := loadDeliveries(t)
	for i, sample := range samples {
		eventName := sample.Headers[webhooks.HeaderEvent]
		t.Run(strconv.Itoa(i)+" "+eventName, func(t *testing.T) {
			ts, err := strconv.ParseInt(sample.Headers[webhooks.HeaderTimestamp], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			at := func() time.Time { return time.Unix(ts, 0) }
			raw := []byte(sample.Raw)

			delivery, err := webhooks.VerifyDelivery(raw, headersOf(sample), webhooks.Options{Secret: secret, Now: at})
			if err != nil {
				t.Fatalf("a real delivery failed verification: %v", err)
			}
			if delivery.ID != sample.Headers[webhooks.HeaderID] {
				t.Errorf("delivery id = %q", delivery.ID)
			}
			if string(delivery.EventType) != eventName {
				t.Errorf("event type = %q, want %q", delivery.EventType, eventName)
			}
			var body struct {
				UUID string `json:"uuid"`
				Type string `json:"type"`
				Test bool   `json:"test"`
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
			if delivery.Event.ID() != body.UUID || delivery.Event.Type != body.Type || !delivery.Event.IsKnown() {
				t.Errorf("event = %s/%s, want %s/%s", delivery.Event.Type, delivery.Event.ID(), body.Type, body.UUID)
			}
			// A rehearsal delivery (Webhooks.Test, sandbox) is signed like a live one, carries
			// test: true in the signed body and the X-Webhook-Test header, and has no place in the
			// live sequence — the live ones number from one upwards.
			wantTest := body.Test
			if delivery.IsTest != wantTest || delivery.Event.IsTest() != wantTest {
				t.Errorf("IsTest = %v / %v, want %v", delivery.IsTest, delivery.Event.IsTest(), wantTest)
			}
			if wantTest != (sample.Headers[webhooks.HeaderTest] == "true") {
				t.Errorf("the body says test=%v but the %s header says %q", wantTest,
					webhooks.HeaderTest, sample.Headers[webhooks.HeaderTest])
			}
			if wantTest {
				if delivery.Event.Sequence() != 0 {
					t.Errorf("a rehearsal delivery carries sequence %d", delivery.Event.Sequence())
				}
			} else if delivery.Event.Sequence() <= 0 {
				t.Errorf("sequence = %d", delivery.Event.Sequence())
			}

			// The same bytes under any other secret must fail.
			if _, err := webhooks.Verify(raw, headersOf(sample), webhooks.Options{
				Secret: "some-other-secret", PreviousSecret: "another", Now: at,
			}); !oblodai.IsSignature(err) {
				t.Fatalf("a wrong secret was accepted: %v", err)
			}
			// So must a body that differs by one byte.
			tampered := append(bytes.TrimSuffix(raw, []byte("}")), []byte(`,"x":1}`)...)
			if _, err := webhooks.Verify(tampered, headersOf(sample), webhooks.Options{Secret: secret, Now: at}); err == nil {
				t.Fatal("a tampered body was accepted")
			}
		})
	}
}

// signed builds a delivery signed with secret.
func signed(t *testing.T, secret string, ts int64, body string, extra map[string]string) http.Header {
	t.Helper()
	header := http.Header{}
	header.Set(webhooks.HeaderTimestamp, strconv.FormatInt(ts, 10))
	header.Set(webhooks.HeaderSignature, oblodai.SignWebhook(secret, ts, []byte(body)))
	for name, value := range extra {
		header.Set(name, value)
	}
	return header
}

const sampleBody = `{"type":"payment","uuid":"u1","order_id":"o","status":"paid","is_final":true,` +
	`"sequence":7,"event_at":"2026-01-01T00:00:00Z","txid":""}`

func TestVerifyRules(t *testing.T) {
	const ts = int64(1_755_600_000)
	at := func() time.Time { return time.Unix(ts, 0) }

	t.Run("accepts a valid signature with case-insensitive headers", func(t *testing.T) {
		header := signed(t, "whsec", ts, sampleBody, nil)
		lowercase := http.Header{}
		for name, values := range header {
			lowercase[name] = values // http.Header canonicalizes on Set/Get, which is the point
		}
		event, err := webhooks.Verify([]byte(sampleBody), lowercase, webhooks.Options{Secret: "whsec", Now: at})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if event.Type != webhooks.KindPayment || !event.IsFinal() {
			t.Fatalf("unexpected event: %+v", event)
		}
		if event.Payment == nil || event.Payment.Status != oblodai.PaymentStatusPaid || event.Payout != nil {
			t.Fatalf("expected a payment event with status paid, got %+v", event)
		}
	})

	t.Run("rejects a wrong secret, a tampered body and a missing header", func(t *testing.T) {
		header := signed(t, "whsec", ts, sampleBody, nil)
		if _, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{Secret: "other", Now: at}); !oblodai.IsCode(err, oblodai.CodeWebhookBadSignature) {
			t.Fatalf("wrong secret: %v", err)
		}
		tampered := []byte(`{"type":"payment","uuid":"u1","status":"paid_over","sequence":7}`)
		if _, err := webhooks.Verify(tampered, header, webhooks.Options{Secret: "whsec", Now: at}); !oblodai.IsCode(err, oblodai.CodeWebhookBadSignature) {
			t.Fatalf("tampered body: %v", err)
		}
		bare := http.Header{}
		bare.Set(webhooks.HeaderSignature, "aa")
		if _, err := webhooks.Verify([]byte(sampleBody), bare, webhooks.Options{Secret: "whsec"}); !oblodai.IsCode(err, oblodai.CodeWebhookMissingHeader) {
			t.Fatalf("missing header: %v", err)
		}
	})

	t.Run("rejects stale deliveries unless the check is disabled", func(t *testing.T) {
		header := signed(t, "whsec", ts, sampleBody, nil)
		late := func() time.Time { return time.Unix(ts+600, 0) }
		if _, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{Secret: "whsec", Now: late}); !oblodai.IsCode(err, oblodai.CodeWebhookStaleTimestamp) {
			t.Fatalf("stale delivery: %v", err)
		}
		event, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{
			Secret: "whsec", Now: late, SkipTimestampCheck: true,
		})
		if err != nil || event.ID() != "u1" {
			t.Fatalf("with the check disabled: %v", err)
		}
		// A wider tolerance accepts it too.
		if _, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{
			Secret: "whsec", Now: late, Tolerance: 20 * time.Minute,
		}); err != nil {
			t.Fatalf("with a wider tolerance: %v", err)
		}
	})

	t.Run("verifies through a secret rotation", func(t *testing.T) {
		header := signed(t, "new", ts, sampleBody, map[string]string{
			webhooks.HeaderSignaturePrev: oblodai.SignWebhook("old", ts, []byte(sampleBody)),
		})
		// Not swapped yet: the stored secret is the old one, which signs the Prev header.
		if _, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{Secret: "old", Now: at}); err != nil {
			t.Fatalf("before the swap: %v", err)
		}
		// Already swapped: the stored secret is the new one.
		if _, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{Secret: "new", Now: at}); err != nil {
			t.Fatalf("after the swap: %v", err)
		}
		// Keeping the outgoing secret explicitly works too.
		if _, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{
			Secret: "unrelated", PreviousSecret: "old", Now: at,
		}); err != nil {
			t.Fatalf("with PreviousSecret: %v", err)
		}
	})

	t.Run("verifies an http.Request end to end", func(t *testing.T) {
		header := signed(t, "whsec", ts, sampleBody, map[string]string{
			webhooks.HeaderID:        "d-1",
			webhooks.HeaderEventID:   "e-1",
			webhooks.HeaderEvent:     "invoice.paid",
			webhooks.HeaderEventTime: strconv.FormatInt(ts, 10),
		})
		request := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewBufferString(sampleBody))
		request.Header = header
		delivery, err := webhooks.VerifyRequest(request, webhooks.Options{Secret: "whsec", Now: at})
		if err != nil {
			t.Fatalf("VerifyRequest: %v", err)
		}
		if delivery.ID != "d-1" || delivery.EventID != "e-1" || delivery.EventType != oblodai.WebhookEventNameInvoicePaid {
			t.Fatalf("unexpected delivery: %+v", delivery)
		}
		if !delivery.EventTime.Equal(time.Unix(ts, 0).UTC()) || !delivery.SentAt.Equal(time.Unix(ts, 0).UTC()) {
			t.Fatalf("times = %s / %s", delivery.EventTime, delivery.SentAt)
		}
		if string(delivery.Raw) != sampleBody {
			t.Fatal("Raw must be the exact bytes that were verified")
		}
	})

	t.Run("parses the event union and detects stale sequences", func(t *testing.T) {
		event, err := webhooks.Parse([]byte(sampleBody))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if !webhooks.IsStale(event, 7) || webhooks.IsStale(event, 6) {
			t.Fatal("sequence 7 is stale against 7 and fresh against 6")
		}
		if _, err := webhooks.Parse([]byte(`{"uuid":"x"}`)); err == nil {
			t.Fatal("a body without a type must be refused")
		}
		if _, err := webhooks.Parse([]byte(`not json`)); err == nil {
			t.Fatal("a non-JSON body must be refused")
		}
	})
}

// An event type this release does not know is a newer core, not an attack: it must reach the
// receiver with its raw type, and it must not be mistaken for a known shape.
func TestUnknownEventTypeIsReturnedNotRefused(t *testing.T) {
	event, err := webhooks.Parse([]byte(`{"type":"alien","uuid":"x","sequence":9,"is_final":true,"test":true}`))
	if err != nil {
		t.Fatalf("an unknown event type must not be refused: %v", err)
	}
	// Which field identifies an unknown kind's object is not guessed: ID() is "" for it.
	if event.Type != "alien" || event.ID() != "" || event.Sequence() != 9 || !event.IsFinal() {
		t.Fatalf("the raw event was not preserved: %+v", event)
	}
	if event.IsKnown() || event.Payment != nil {
		t.Fatal("IsKnown must be false for an unmodelled type")
	}
	if !event.IsTest() {
		t.Fatal("the test flag must work on an unmodelled type")
	}
	if !webhooks.IsStale(event, 20) || webhooks.IsStale(event, 8) {
		t.Fatal("IsStale must work on an unmodelled type")
	}
	if len(event.Raw) == 0 {
		t.Fatal("the raw body must be kept")
	}
}

// A kind newer than this release is not refused for lacking uuid and id: only the contract's
// kinds have a known id field (IDFields).
func TestUnknownKindNeedsNoUUIDOrID(t *testing.T) {
	event, err := webhooks.Parse([]byte(`{"type":"refund","refund_id":"r1","sequence":3}`))
	if err != nil {
		t.Fatalf("an unknown kind without uuid/id must parse: %v", err)
	}
	if event.Type != "refund" || event.IsKnown() || event.ID() != "" || event.Sequence() != 3 {
		t.Fatalf("event = %+v", event)
	}
}

// A known kind must carry the id field IDFields names for it — the other spelling does not do.
func TestKnownKindNeedsItsIDField(t *testing.T) {
	for kind, field := range webhooks.IDFields {
		other := map[string]string{"uuid": "id", "id": "uuid"}[field]
		body := `{"type":"` + kind + `","` + other + `":"x","sequence":1}`
		if _, err := webhooks.Parse([]byte(body)); !oblodai.IsCode(err, oblodai.CodeWebhookBadPayload) {
			t.Errorf("%s: want webhook.bad_payload, got %v", body, err)
		}
		body = `{"type":"` + kind + `","` + field + `":null,"sequence":1}`
		if _, err := webhooks.Parse([]byte(body)); !oblodai.IsCode(err, oblodai.CodeWebhookBadPayload) {
			t.Errorf("%s: want webhook.bad_payload, got %v", body, err)
		}
	}
	if len(webhooks.IDFields) != len(webhooks.KnownKinds) {
		t.Fatalf("IDFields %v, KnownKinds %v", webhooks.IDFields, webhooks.KnownKinds)
	}
}

// A conversion event names its object by id, not uuid.
func TestConversionEventIsModelled(t *testing.T) {
	event, err := webhooks.Parse([]byte(`{"type":"conversion","id":"c1","status":"completed","sent":"10","sequence":2}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if event.Conversion == nil || event.ID() != "c1" || event.Conversion.Sent != "10" || !event.IsKnown() {
		t.Fatalf("conversion = %+v", event)
	}
}

// A body that verified but cannot be read is webhook.bad_payload in the contract family: a
// receiver that answers 401 to signature failures must not answer 401 to an authentic event.
func TestAuthenticButUnreadableBodyIsAPayloadError(t *testing.T) {
	const ts = int64(1_755_600_000)
	at := func() time.Time { return time.Unix(ts, 0) }
	body := `{"type":"payment","uuid":"u1","sequence":"seven"}`
	header := signed(t, "whsec", ts, body, nil)
	_, err := webhooks.Verify([]byte(body), header, webhooks.Options{Secret: "whsec", Now: at})
	if !oblodai.IsCode(err, oblodai.CodeWebhookBadPayload) {
		t.Fatalf("want webhook.bad_payload, got %v", err)
	}
	if oblodai.IsSignature(err) {
		t.Fatal("an authentic delivery must never report a signature failure")
	}
	if !oblodai.IsContract(err) || !oblodai.IsWebhookPayload(err) {
		t.Fatalf("webhook.bad_payload must be in the contract family: %v", err)
	}
}

// The MAC is checked before freshness: a forged delivery with a stale timestamp must report the
// signature failure, never the timestamp — otherwise the tolerance window answers questions to
// callers who cannot sign.
func TestSignatureIsCheckedBeforeFreshness(t *testing.T) {
	const ts = int64(1_755_600_000)
	late := func() time.Time { return time.Unix(ts+86_400, 0) }
	header := signed(t, "whsec", ts, sampleBody, nil)
	header.Set(webhooks.HeaderSignature, "00")
	_, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{Secret: "whsec", Now: late})
	if !oblodai.IsCode(err, oblodai.CodeWebhookBadSignature) {
		t.Fatalf("a forged stale delivery must fail on the signature, got %v", err)
	}
}

func TestVerificationConfigurationIsRefusedBeforeAnyCrypto(t *testing.T) {
	const ts = int64(1_755_600_000)
	at := func() time.Time { return time.Unix(ts, 0) }
	header := signed(t, "whsec", ts, sampleBody, nil)

	_, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{Secret: "", Now: at})
	if !oblodai.IsConfig(err) {
		t.Fatalf("an empty secret must be a ConfigError, got %v", err)
	}
	_, err = webhooks.Verify([]byte(sampleBody), header, webhooks.Options{Secret: "whsec", Tolerance: -time.Second, Now: at})
	if !oblodai.IsConfig(err) {
		t.Fatalf("a negative tolerance must be a ConfigError, got %v", err)
	}
}

func TestSignatureHeaderSpellings(t *testing.T) {
	const ts = int64(1_755_600_000)
	at := func() time.Time { return time.Unix(ts, 0) }
	digest := oblodai.SignWebhook("whsec", ts, []byte(sampleBody))

	for name, value := range map[string]string{
		"upper case": strings.ToUpper(digest),
		"padded":     "  " + digest + "\t",
	} {
		header := signed(t, "whsec", ts, sampleBody, nil)
		header.Set(webhooks.HeaderSignature, value)
		if _, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{Secret: "whsec", Now: at}); err != nil {
			t.Fatalf("%s signature: %v", name, err)
		}
	}

	header := signed(t, "whsec", ts, sampleBody, nil)
	header.Set(webhooks.HeaderSignature, "0x"+digest)
	if _, err := webhooks.Verify([]byte(sampleBody), header, webhooks.Options{Secret: "whsec", Now: at}); !oblodai.IsCode(err, oblodai.CodeWebhookBadSignature) {
		t.Fatalf("a 0x-prefixed signature must be refused, got %v", err)
	}
}

// A body with no sequence orders nothing: dropping it as "stale" would lose a real state change.
func TestEventWithoutASequenceIsNeverStale(t *testing.T) {
	event, err := webhooks.Parse([]byte(`{"type":"payment","uuid":"u1","status":"paid"}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if webhooks.IsStale(event, 0) || webhooks.IsStale(event, 99) {
		t.Fatal("an event without a sequence must never be reported as stale")
	}
}

func TestEveryEventKindDecodesToItsOwnType(t *testing.T) {
	_, samples := loadDeliveries(t)
	seen := map[string]bool{}
	for _, sample := range samples {
		event, err := webhooks.Parse([]byte(sample.Raw))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		seen[event.Type] = true
		typed := map[string]bool{
			webhooks.KindPayment: event.Payment != nil,
			webhooks.KindPayout:  event.Payout != nil,
			webhooks.KindWallet:  event.Wallet != nil,
		}
		for kind, set := range typed {
			if set != (kind == event.Type) {
				t.Fatalf("a %s event set the %s body: %v", event.Type, kind, set)
			}
		}
	}
	if len(seen) < 2 {
		t.Fatalf("the recorded deliveries cover only %v", seen)
	}
}

// The kinds and their bodies are generated from the contract's webhooks: every event name maps to
// a kind this release models, and every kind parses into its typed body.
func TestEveryEventOfTheContractIsAKnownKind(t *testing.T) {
	if len(webhooks.EventKinds) == 0 {
		t.Fatal("EventKinds is empty")
	}
	for name, kind := range webhooks.EventKinds {
		if !slices.Contains(webhooks.KnownKinds, kind) {
			t.Errorf("event %s: kind %q is not in KnownKinds %v", name, kind, webhooks.KnownKinds)
		}
	}
	for _, kind := range webhooks.KnownKinds {
		event, err := webhooks.Parse([]byte(`{"type":"` + kind + `","uuid":"u1","id":"i1"}`))
		if err != nil || !event.IsKnown() {
			t.Errorf("kind %s: %v %+v", kind, err, event)
		}
	}
	for _, kind := range []string{webhooks.KindPayment, webhooks.KindPayout, webhooks.KindWallet, webhooks.KindConversion} {
		if !slices.Contains(webhooks.KnownKinds, kind) {
			t.Errorf("KnownKinds %v lacks %s", webhooks.KnownKinds, kind)
		}
	}
}
