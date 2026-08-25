package oblodai

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Request signing — the exact recipe the core verifies (crypto.SignRequest):
//
//	canonical = ts "\n" METHOD "\n" requestURI "\n" idempotencyKey "\n" body
//	signature = hex(HMAC-SHA256(secret, canonical))
//
//   - ts is unix seconds; the core accepts +/-300 s of skew.
//   - requestURI is path plus raw query ("/v1/x?limit=1"), never the origin.
//   - The idempotency slot is the EMPTY STRING when no Idempotency-Key header is sent — empty, not
//     absent: the separator newline is always there.
//   - body is the byte-exact request body; GETs sign an empty body.
//
// The idempotency key is inside the signature because it decides whether a request is a duplicate:
// outside it, a captured signed payout could be replayed with the header stripped and would be
// executed twice.
//
// These functions are pure: no clock, no I/O. The vectors in contract/contract.json come from the
// core's own test suite and are checked in signing_test.go.

// Headers the core reads on a signed request.
const (
	HeaderPublicID       = "X-Public-Id"
	HeaderSignature      = "X-Signature"
	HeaderTimestamp      = "X-Timestamp"
	HeaderIdempotencyKey = "Idempotency-Key"
	// HeaderAdminToken gates merchant provisioning on a self-hosted gateway.
	HeaderAdminToken = "X-Admin-Token"
)

// SignatureSkewSeconds is how far the core lets a request timestamp drift from its own clock.
const SignatureSkewSeconds = 300

// SignInput is everything the request signature binds.
type SignInput struct {
	// TS is the unix timestamp in seconds, as sent in X-Timestamp.
	TS int64
	// Method is the upper-case HTTP method.
	Method string
	// RequestURI is the path plus raw query string, exactly as the request line carries it.
	RequestURI string
	// IdempotencyKey is the value of the Idempotency-Key header, empty when none is sent.
	IdempotencyKey string
	// Body is the request body bytes (already-serialized JSON, or empty for GET).
	Body []byte
}

// CanonicalString renders the string the signature is computed over. Useful when debugging a
// signature mismatch: compare it with the core's log line, byte for byte.
func CanonicalString(in SignInput) string {
	return strconv.FormatInt(in.TS, 10) + "\n" + in.Method + "\n" + in.RequestURI + "\n" + in.IdempotencyKey + "\n" + string(in.Body)
}

// SignRequest computes the hex HMAC-SHA256 request signature for the X-Signature header.
func SignRequest(secret string, in SignInput) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(in.TS, 10)))
	mac.Write([]byte("\n"))
	mac.Write([]byte(in.Method))
	mac.Write([]byte("\n"))
	mac.Write([]byte(in.RequestURI))
	mac.Write([]byte("\n"))
	mac.Write([]byte(in.IdempotencyKey))
	mac.Write([]byte("\n"))
	mac.Write(in.Body)
	return hex.EncodeToString(mac.Sum(nil))
}

// SignWebhook computes a webhook delivery signature — webhook.Sign on the core side:
//
//	signature = hex(HMAC-SHA256(secret, "<unix ts>." + payload))
//
// The payload is signed verbatim, so verifiers must use the raw request bytes, never a re-encoded
// parse of them.
func SignWebhook(secret string, ts int64, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
