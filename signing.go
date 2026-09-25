package oblodai

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Request signing — the exact recipe the core verifies (crypto.SignRequest), generated from the
// contract's x-oblodai-signing into zz_generated_signing.go: the header names (HeaderPublicID,
// HeaderSignature, HeaderTimestamp, HeaderIdempotencyKey), the canonical string (canonicalRequest)
// and the limits (SkewSeconds, MaxBody, MaxIdempotencyKeyLength). Today the canonical string is
//
//	ts "\n" METHOD "\n" requestURI "\n" idempotencyKey "\n" body
//	signature = hex(HMAC-SHA256(secret, canonical))
//
//   - ts is unix seconds; the core accepts +/-SkewSeconds of skew.
//   - requestURI is path plus raw query ("/v1/x?limit=1"), never the origin.
//   - The idempotency slot is the EMPTY STRING when no idempotency key header is sent — empty,
//     not absent: the separator is always there.
//   - body is the byte-exact request body; GETs sign an empty body.
//
// The idempotency key is inside the signature because it decides whether a request is a duplicate:
// outside it, a captured signed payout could be replayed with the header stripped and would be
// executed twice.
//
// These functions are pure: no clock, no I/O. The core's vectors (x-oblodai-signing of the
// contract) are checked by the shared conformance suite in conformance_test.go.

// SignatureSkewSeconds is the 1.x name of SkewSeconds (x-oblodai-signing.skew_seconds): how far the
// core lets a request timestamp drift from its own clock.
const SignatureSkewSeconds = SkewSeconds

// HeaderAdminToken gates merchant provisioning on a self-hosted gateway. It is not part of request
// signing and not in the contract's x-oblodai-signing.
const HeaderAdminToken = "X-Admin-Token"

// SignInput is everything the request signature binds.
type SignInput struct {
	// TS is the unix timestamp in seconds, as sent in HeaderTimestamp.
	TS int64
	// Method is the upper-case HTTP method.
	Method string
	// RequestURI is the path plus raw query string, exactly as the request line carries it.
	RequestURI string
	// IdempotencyKey is the value of HeaderIdempotencyKey, empty when none is sent.
	IdempotencyKey string
	// Body is the request body bytes (already-serialized JSON, or empty for GET).
	Body []byte
}

// CanonicalString renders the string the signature is computed over. Useful when debugging a
// signature mismatch: compare it with the core's log line, byte for byte.
func CanonicalString(in SignInput) string {
	return string(canonical(in))
}

func canonical(in SignInput) []byte {
	return canonicalRequest(strconv.FormatInt(in.TS, 10), in.Method, in.RequestURI, in.IdempotencyKey, in.Body)
}

// SignRequest computes the hex HMAC-SHA256 request signature for the HeaderSignature header.
func SignRequest(secret string, in SignInput) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(canonical(in))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignWebhook computes a webhook delivery signature — webhook.Sign on the core side — over the
// generated canonicalWebhook; today
//
//	signature = hex(HMAC-SHA256(secret, "<unix ts>." + payload))
//
// The payload is signed verbatim, so verifiers must use the raw request bytes, never a re-encoded
// parse of them.
func SignWebhook(secret string, ts int64, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(canonicalWebhook(strconv.FormatInt(ts, 10), payload))
	return hex.EncodeToString(mac.Sum(nil))
}
