// Package webhooks verifies Oblodai webhook deliveries. It needs no client and no API key — only
// the endpoint secret — so a receiver can live in its own service:
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//		delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: os.Getenv("OBLODAI_WEBHOOK_SECRET")})
//		if err != nil {
//			http.Error(w, "bad signature", http.StatusBadRequest)
//			return
//		}
//		switch event := delivery.Event.(type) {
//		case *oblodai.PaymentEvent:
//			…
//		}
//		w.WriteHeader(http.StatusOK)
//	}
//
// Deliveries are signed as:
//
//	X-Webhook-Timestamp:       unix seconds
//	X-Webhook-Signature:       hex(HMAC-SHA256(secret, "<ts>." + rawBody))
//	X-Webhook-Signature-Prev:  the same with the previous secret — only during a rotation overlap
//	X-Webhook-Event:           invoice.<status> | payout.<status> | wallet.paid
//	X-Webhook-Id:              stable per delivery (identical across retries) — your dedup key
//	X-Webhook-Event-Time:      unix seconds when the state change committed (order events by it)
//
// Always verify over the RAW request bytes: a re-serialized parse will not match the signature.
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

	"github.com/oblodai/oblodai-go"
)

// Delivery headers.
const (
	HeaderTimestamp     = "X-Webhook-Timestamp"
	HeaderSignature     = "X-Webhook-Signature"
	HeaderSignaturePrev = "X-Webhook-Signature-Prev"
	HeaderEvent         = "X-Webhook-Event"
	HeaderID            = "X-Webhook-Id"
	HeaderEventTime     = "X-Webhook-Event-Time"
)

// DefaultTolerance is how far a delivery's timestamp may be from now before it is refused.
const DefaultTolerance = 5 * time.Minute

// MaxBodySize bounds what VerifyRequest reads from a request body: a webhook is a small JSON
// document, and an unbounded read is a denial-of-service invitation.
const MaxBodySize = 1 << 20

// Options configures verification.
type Options struct {
	// Secret is the endpoint secret from Webhooks.Register or Webhooks.RotateSecret. Required.
	Secret string
	// PreviousSecret is the outgoing secret during a rotation. Deliveries queued before the
	// rotation stay signed with it for their whole retry life (about 26 hours), so keep it at
	// least that long after rotating.
	PreviousSecret string
	// Tolerance is the accepted clock difference; zero means DefaultTolerance.
	Tolerance time.Duration
	// SkipTimestampCheck accepts any timestamp. Only for replaying captured deliveries in tests:
	// in production it removes the replay protection the timestamp provides.
	SkipTimestampCheck bool
	// Now is the clock, for tests. Zero value means time.Now.
	Now func() time.Time
}

// Delivery is a verified delivery: the event plus the advisory headers worth keeping.
type Delivery struct {
	// Event is the parsed body; type-switch on *oblodai.PaymentEvent, *oblodai.PayoutEvent or
	// *oblodai.WalletEvent.
	Event oblodai.WebhookEvent
	// ID is X-Webhook-Id, stable across retries of the same delivery — use it as your dedup key.
	ID string
	// EventType is X-Webhook-Event: invoice.<status>, payout.<status> or wallet.paid.
	EventType oblodai.EventType
	// EventTime is X-Webhook-Event-Time: when the state change committed. Zero when absent.
	EventTime time.Time
	// SentAt is X-Webhook-Timestamp: when this delivery attempt was sent.
	SentAt time.Time
	// Raw is the exact body that was verified.
	Raw []byte
}

// Verify checks the signature and freshness of a delivery and returns the parsed event. It never
// returns an unverified body. Every failure is an *oblodai.Error of kind signature.
func Verify(rawBody []byte, headers http.Header, opts Options) (oblodai.WebhookEvent, error) {
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
func VerifyDelivery(rawBody []byte, headers http.Header, opts Options) (*Delivery, error) {
	timestampHeader := headers.Get(HeaderTimestamp)
	signature := headers.Get(HeaderSignature)
	if timestampHeader == "" || signature == "" {
		return nil, signatureError(oblodai.CodeWebhookMissingHeader,
			fmt.Sprintf("the delivery is missing %s or %s", HeaderTimestamp, HeaderSignature))
	}
	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return nil, signatureError(oblodai.CodeWebhookBadSignature, "the timestamp header is not an integer")
	}

	if !opts.SkipTimestampCheck {
		tolerance := opts.Tolerance
		if tolerance <= 0 {
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

	if opts.Secret == "" {
		return nil, signatureError(oblodai.CodeWebhookBadSignature, "no secret was supplied to verify against")
	}
	// A merchant who has not swapped the stored secret yet verifies the Prev header with it; one
	// who already swapped but kept the old copy verifies the main header with the new secret.
	// Both hold, so try every combination the rotation can produce.
	previous := headers.Get(HeaderSignaturePrev)
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
		if subtle.ConstantTimeCompare([]byte(strings.ToLower(candidate[0])), []byte(want)) == 1 {
			matched = true
		}
	}
	if !matched {
		return nil, signatureError(oblodai.CodeWebhookBadSignature, "the signature does not match the body")
	}

	event, err := Parse(rawBody)
	if err != nil {
		return nil, err
	}
	delivery := &Delivery{
		Event:     event,
		ID:        headers.Get(HeaderID),
		EventType: oblodai.EventType(headers.Get(HeaderEvent)),
		SentAt:    time.Unix(ts, 0).UTC(),
		Raw:       rawBody,
	}
	if eventTime, err := strconv.ParseInt(headers.Get(HeaderEventTime), 10, 64); err == nil {
		delivery.EventTime = time.Unix(eventTime, 0).UTC()
	}
	return delivery, nil
}

// Parse decodes a delivery body into the event it describes. Use it only on bytes Verify has
// already accepted — an unverified body is attacker-controlled input.
func Parse(rawBody []byte) (oblodai.WebhookEvent, error) {
	var head struct {
		Type oblodai.WebhookKind `json:"type"`
		UUID string              `json:"uuid"`
	}
	if err := json.Unmarshal(rawBody, &head); err != nil {
		return nil, signatureError(oblodai.CodeWebhookBadSignature, "the body is not JSON")
	}
	if head.Type == "" || head.UUID == "" {
		return nil, signatureError(oblodai.CodeWebhookBadSignature,
			"the body lacks the type and uuid fields every event carries")
	}
	var event oblodai.WebhookEvent
	switch head.Type {
	case oblodai.WebhookKindPayment:
		event = &oblodai.PaymentEvent{}
	case oblodai.WebhookKindPayout:
		event = &oblodai.PayoutEvent{}
	case oblodai.WebhookKindWallet:
		event = &oblodai.WalletEvent{}
	default:
		return nil, signatureError(oblodai.CodeWebhookBadSignature,
			fmt.Sprintf("unknown event type %q", string(head.Type)))
	}
	if err := json.Unmarshal(rawBody, event); err != nil {
		return nil, signatureError(oblodai.CodeWebhookBadSignature, "the body does not match the "+string(head.Type)+" event shape")
	}
	return event, nil
}

// IsStale reports whether an event is not newer than the last sequence you processed for that
// object. Deliveries can arrive out of order (a retried "paid" after a "refund"), so keep the last
// sequence per object and skip anything IsStale flags.
func IsStale(event oblodai.WebhookEvent, lastProcessedSequence int64) bool {
	return event != nil && event.Seq() <= lastProcessedSequence
}

// signatureError builds the *oblodai.Error every failure here reports.
func signatureError(code, message string) error {
	return oblodai.NewSignatureError(code, message)
}
