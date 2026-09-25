// Package webhooks verifies Oblodai webhook deliveries. It needs no client and no API key — only
// the endpoint secret — so a receiver can live in its own service:
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//		delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: os.Getenv("OBLODAI_WEBHOOK_SECRET")})
//		if err != nil {
//			http.Error(w, "bad signature", http.StatusBadRequest)
//			return
//		}
//		if delivery.IsTest {
//			w.WriteHeader(http.StatusOK) // a rehearsal: no money moved
//			return
//		}
//		if payment := delivery.Event.Payment; payment != nil {
//			… // payment.Status, payment.Amount, payment.OrderID
//		}
//		w.WriteHeader(http.StatusOK)
//	}
//
// Deliveries are signed as (header names are generated from the contract, zz_generated_signing.go):
//
//	HeaderTimestamp:      unix seconds
//	HeaderSignature:      hex(HMAC-SHA256(secret, "<ts>." + rawBody))
//	HeaderSignaturePrev:  the same with the previous secret — only during a rotation overlap
//	HeaderEvent:          the event name, a key of EventKinds: invoice.paid, payout.sent, …
//	HeaderID:             stable per delivery (identical across retries of THAT delivery)
//	HeaderEventID:        stable per STATE — the same for a resend of a state you handled,
//	                      different as soon as the state differs: the key to deduplicate on
//	HeaderEventTime:      unix seconds when the state change committed (order events by it)
//	HeaderTest:           "true" on a rehearsal delivery (Webhooks.Test, sandbox)
//
// Always verify over the RAW request bytes: a re-serialized parse will not match the signature.
//
// The checks run in this order: headers, HMAC (current secret, then the previous one), freshness,
// body. A failure before the body is an *oblodai.Error of kind signature — answer 4xx. A body
// that verified but cannot be read is webhook.bad_payload, kind contract — answer 5xx, because
// the event is real and the core will retry it. An event type this release does not model is not
// a failure at all: it arrives as an Event with its raw Type and no typed body (Event.IsKnown).
//
// The typed bodies are the generated models of the contract's webhook schemas
// (oblodai.PaymentWebhook, …): which kinds exist and which model each carries is generated from
// the contract (zz_generated_events.go), never listed by hand.
//
// Rehearsal deliveries are signed exactly like live ones and carry test: true in the body (and the
// HeaderTest header). Check Delivery.IsTest — or Event.IsTest — and never act on one as if
// money moved.
package webhooks

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oblodai/oblodai-go/v2"
)

// DefaultTolerance is how far a delivery's timestamp may be from now before it is refused: the
// contract's skew window.
const DefaultTolerance = time.Duration(oblodai.SkewSeconds) * time.Second

// MaxBodySize bounds what VerifyRequest reads from a request body: a webhook is a small JSON
// document, and an unbounded read is a denial-of-service invitation.
//
// The bound is x-oblodai-signing.max_body (oblodai.MaxBody) by a deliberate rule of the SDK family:
// the contract states no webhook body limit of its own, and max_body — the core's limit on signed
// merchant request bodies — is the only body bound it has. A delivery is far smaller than either
// (1 MiB today), so the cap follows max_body if the core changes it rather than being a number of
// this SDK's. Only SDKs that read a delivery from a stream themselves need such a cap; the others
// verify the bytes the caller hands them and expose MaxBody only in their generated code.
const MaxBodySize = oblodai.MaxBody

// Options configures verification.
type Options struct {
	// Secret is the endpoint secret from Webhooks.Register or Webhooks.RotateSecret. Required: an
	// empty one is refused with a ConfigError before any crypto runs, never verified against.
	Secret string
	// PreviousSecret is the outgoing secret during a rotation. Deliveries queued before the
	// rotation stay signed with it for their whole retry life (about 26 hours), so keep it at
	// least that long after rotating.
	PreviousSecret string
	// Tolerance is the accepted clock difference. Zero means DefaultTolerance — in Go the zero
	// value is "unset", so freshness is never silently disabled; use SkipTimestampCheck for that.
	// A negative value is a configuration error.
	Tolerance time.Duration
	// SkipTimestampCheck accepts any timestamp. Only for replaying captured deliveries in tests:
	// in production it removes the replay protection the timestamp provides.
	SkipTimestampCheck bool
	// Now is the clock, for tests. Zero value means time.Now.
	Now func() time.Time
}

// Delivery is a verified delivery: the event plus the advisory headers worth keeping.
type Delivery struct {
	// Event is the parsed body.
	Event *Event
	// ID is HeaderID: stable across retries of the same DELIVERY. It is not enough to
	// deduplicate on — a resend of a state you already handled is a new delivery with a new id.
	ID string
	// EventID is HeaderEventID: the id of the STATE this delivery carries — the same for the
	// original, its retries and every resend of that state, different as soon as the state differs.
	// Keep the ids you have handled and skip repeats. Empty from a core that predates it.
	EventID string
	// EventType is HeaderEvent: the event name (invoice.paid, payout.sent, …; see EventKinds).
	EventType oblodai.WebhookEventName
	// EventTime is HeaderEventTime: when the state change committed. Zero when absent.
	EventTime time.Time
	// SentAt is HeaderTimestamp: when this attempt was signed and sent.
	SentAt time.Time
	// IsTest marks a rehearsal delivery (HeaderTest or test: true in the body): signed like a
	// live one, but no money moved.
	IsTest bool
	// Raw is the exact body that was verified.
	Raw []byte
}

// Verify checks the signature and freshness of a delivery and returns the parsed event. It never
// returns an unverified body. Every failure is an *oblodai.Error of kind signature.
func Verify(rawBody []byte, headers http.Header, opts Options) (*Event, error) {
	delivery, err := VerifyDelivery(rawBody, headers, opts)
	if err != nil {
		return nil, err
	}
	return delivery.Event, nil
}

// VerifyRequest verifies an incoming *http.Request, reading its body (up to MaxBodySize) itself.
// The body is consumed, so call it before anything else touches r.Body; Delivery.Raw keeps the
// exact bytes that were verified.
func VerifyRequest(r *http.Request, opts Options) (*Delivery, error) {
	if r == nil || r.Body == nil {
		return nil, signatureError(oblodai.CodeWebhookMissingHeader, "the request has no body to verify")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodySize+1))
	if err != nil {
		return nil, signatureError(oblodai.CodeWebhookBadSignature, "the request body could not be read: "+err.Error())
	}
	if len(body) > MaxBodySize {
		return nil, signatureError(oblodai.CodeWebhookBadSignature, "the request body is larger than a webhook can be")
	}
	return VerifyDelivery(body, r.Header, opts)
}

// VerifyDelivery is Verify, and also returns the delivery id, event type and times.
//
// The order of the checks is deliberate: headers, then the MAC, then freshness, then the body.
// Checking freshness before the MAC would answer "stale" to an unsigned probe and turn the
// tolerance window into an oracle; parsing the body before the MAC would run a decoder over
// attacker-controlled bytes.
func VerifyDelivery(rawBody []byte, headers http.Header, opts Options) (*Delivery, error) {
	if opts.Secret == "" {
		return nil, oblodai.NewConfigError(oblodai.CodeBadConfig,
			"webhooks: a non-empty endpoint secret is required to verify a delivery", "Secret")
	}
	if opts.Tolerance < 0 {
		return nil, oblodai.NewConfigError(oblodai.CodeBadConfig,
			"webhooks: Tolerance must not be negative; leave it zero for the default window, or set SkipTimestampCheck to accept any timestamp", "Tolerance")
	}

	timestampHeader := strings.TrimSpace(headers.Get(HeaderTimestamp))
	signature, sigErr := normalizeSignature(headers.Get(HeaderSignature))
	if sigErr != nil {
		return nil, sigErr
	}
	if timestampHeader == "" || signature == "" {
		return nil, signatureError(oblodai.CodeWebhookMissingHeader,
			fmt.Sprintf("the delivery is missing %s or %s", HeaderTimestamp, HeaderSignature))
	}
	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return nil, signatureError(oblodai.CodeWebhookBadSignature, "the timestamp header is not an integer")
	}

	// A merchant who has not swapped the stored secret yet verifies the Prev header with it; one
	// who already swapped but kept the old copy verifies the main header with the new secret.
	// Both hold, so try every combination the rotation can produce.
	previous, prevErr := normalizeSignature(headers.Get(HeaderSignaturePrev))
	if prevErr != nil {
		return nil, prevErr
	}
	candidates := [][2]string{{signature, opts.Secret}}
	if previous != "" {
		candidates = append(candidates, [2]string{previous, opts.Secret})
	}
	if opts.PreviousSecret != "" {
		candidates = append(candidates, [2]string{signature, opts.PreviousSecret})
		if previous != "" {
			candidates = append(candidates, [2]string{previous, opts.PreviousSecret})
		}
	}
	matched := false
	for _, candidate := range candidates {
		want := oblodai.SignWebhook(candidate[1], ts, rawBody)
		if subtle.ConstantTimeCompare([]byte(candidate[0]), []byte(want)) == 1 {
			matched = true
		}
	}
	if !matched {
		return nil, signatureError(oblodai.CodeWebhookBadSignature, "the signature does not match the body")
	}

	if !opts.SkipTimestampCheck {
		tolerance := opts.Tolerance
		if tolerance == 0 {
			tolerance = DefaultTolerance
		}
		now := time.Now
		if opts.Now != nil {
			now = opts.Now
		}
		if drift := now().Sub(time.Unix(ts, 0)); drift > tolerance || drift < -tolerance {
			return nil, signatureError(oblodai.CodeWebhookStaleTimestamp, fmt.Sprintf(
				"the delivery timestamp %d is outside the +/-%s window", ts, tolerance))
		}
	}

	event, err := Parse(rawBody)
	if err != nil {
		return nil, err
	}
	delivery := &Delivery{
		Event:     event,
		ID:        headers.Get(HeaderID),
		EventID:   headers.Get(HeaderEventID),
		EventType: oblodai.WebhookEventName(headers.Get(HeaderEvent)),
		SentAt:    time.Unix(ts, 0).UTC(),
		IsTest:    strings.EqualFold(strings.TrimSpace(headers.Get(HeaderTest)), "true") || event.IsTest(),
		Raw:       rawBody,
	}
	if eventTime, err := strconv.ParseInt(strings.TrimSpace(headers.Get(HeaderEventTime)), 10, 64); err == nil {
		delivery.EventTime = time.Unix(eventTime, 0).UTC()
	}
	return delivery, nil
}

// normalizeSignature trims the surrounding whitespace a proxy may add and folds the hex to lower
// case, so an upper-case digest still verifies. A "0x" prefix is refused rather than stripped:
// this is not an Ethereum quantity, and accepting two spellings of one value invites the kind of
// mismatch signature checks exist to prevent.
func normalizeSignature(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		return "", signatureError(oblodai.CodeWebhookBadSignature,
			"the signature header must be bare hex, without a 0x prefix")
	}
	return strings.ToLower(value), nil
}

// Event is a delivery body. Type names the kind; for a kind this release models exactly one of
// the typed bodies (the generated Bodies: Payment, Payout, …) is set, for any other none is — a
// newer core may add a kind, and dropping it would lose a real event, so it still arrives with its
// Raw body. The kinds (KindPayment, …, KnownKinds) come from the contract's webhooks.
type Event struct {
	// Type is the body's type: one of KnownKinds or a newer one.
	Type string
	Bodies
	// Raw is the body as delivered.
	Raw json.RawMessage

	head eventHead
	id   string
}

// eventHead is what the runtime reads of an event of any kind, without its model. The generator
// refuses a contract whose webhook body lacks one of these fields, so no kind parses them as zeros.
type eventHead struct {
	Type     string `json:"type"`
	Sequence int64  `json:"sequence"`
	IsFinal  bool   `json:"is_final"`
	Test     bool   `json:"test"`
}

// ID is the id of the object the event is about: the body field IDFields names for its kind (a
// payment's uuid, a conversion's id, …). Key per-object state on it (the last Sequence). It is
// "" for a kind this release does not know: which field identifies that object is not guessed.
func (e *Event) ID() string { return e.id }

// Sequence orders the events of one object; 0 when the body carries none (a rehearsal).
func (e *Event) Sequence() int64 { return e.head.Sequence }

// IsFinal reports a state after which nothing else happens to the object.
func (e *Event) IsFinal() bool { return e.head.IsFinal }

// IsTest reports a rehearsal event (test: true): never act on one as if money moved.
func (e *Event) IsTest() bool { return e.head.Test }

// IsKnown reports whether the event's kind is one this release models with a typed body.
func (e *Event) IsKnown() bool { return e.known() }

// Parse reads a (previously verified) delivery body. A kind this release does not model is not an
// error, and only its type is required; a body that is not JSON, lacks the type every event
// carries, or is of a known kind without its object's id (IDFields) is webhook.bad_payload.
func Parse(rawBody []byte) (*Event, error) {
	event := &Event{Raw: append(json.RawMessage(nil), rawBody...)}
	if err := json.Unmarshal(rawBody, &event.head); err != nil {
		return nil, payloadError("the body is not a JSON event: " + err.Error())
	}
	event.Type = event.head.Type
	if event.Type == "" {
		return nil, payloadError("the body lacks the type field every event carries")
	}
	if field := IDFields[event.Type]; field != "" {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(rawBody, &fields) // the head above decoded, so this is a JSON object
		if json.Unmarshal(fields[field], &event.id) != nil || event.id == "" {
			return nil, payloadError("the " + event.Type + " body lacks the string " + field + " field")
		}
	}
	target := event.target(event.Type)
	if target == nil {
		return event, nil
	}
	if err := json.Unmarshal(rawBody, target); err != nil {
		return nil, payloadError("the body does not match the " + event.Type + " event shape: " + err.Error())
	}
	return event, nil
}

// IsStale reports whether event is at or behind the last sequence you processed for its object,
// so an out-of-order delivery can be acknowledged and dropped. An event without a sequence is
// never stale: dropping it would lose a real state change.
func IsStale(event *Event, lastProcessedSequence int64) bool {
	if event == nil || event.Sequence() <= 0 {
		return false
	}
	return event.Sequence() <= lastProcessedSequence
}

// signatureError builds the *oblodai.Error a verification failure reports: the delivery is not
// (provably) from Oblodai, so a receiver answers 4xx.
func signatureError(code, message string) error {
	return oblodai.NewSignatureError(code, message)
}

// payloadError builds the *oblodai.Error an authentic delivery with an unreadable body reports.
// It carries webhook.bad_payload in the contract family, never the signature family.
func payloadError(message string) error {
	return oblodai.NewWebhookPayloadError("webhooks: " + message)
}
