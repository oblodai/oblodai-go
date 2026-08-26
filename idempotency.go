package oblodai

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// Idempotency keys. On create-type routes the core caches the first response per key for the
// merchant and replays it on retries; a different body under the same key is a 409
// idempotency.key_reused. The client generates a key once per logical call and reuses it on every
// retry, so a timeout never turns into a double payout.

// MaxIdempotencyKeyLength is the longest key the core accepts.
const MaxIdempotencyKeyLength = 255

// NewIdempotencyKey returns a random RFC 4122 v4 UUID from the platform CSPRNG. Generate one
// yourself and pass it with WithIdempotencyKey when a retry has to survive a process restart.
//
// The error is the platform CSPRNG failing, which no supported platform does; a key that cannot
// be trusted to be unique is never returned, because reusing one is how a retry becomes a second
// payout. Callers who want the key or nothing can treat the error as fatal.
func NewIdempotencyKey() (string, error) {
	key, err := newIdempotencyKey()
	if err != nil {
		return "", err
	}
	return key, nil
}

// newIdempotencyKey is NewIdempotencyKey with this package's concrete error type.
func newIdempotencyKey() (string, *Error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", newConfigError(CodeBadIdempotencyKey,
			"a random idempotency key could not be generated: crypto/rand is unavailable: "+err.Error(), "idempotencyKey")
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:]), nil
}

// checkIdempotencyKey validates a caller-supplied key before it is signed and sent.
func checkIdempotencyKey(key string) *Error {
	if key == "" {
		return newConfigError(CodeBadIdempotencyKey, "the idempotency key must not be empty", "idempotencyKey")
	}
	if len(key) > MaxIdempotencyKeyLength {
		return newConfigError(CodeBadIdempotencyKey,
			fmt.Sprintf("the idempotency key is too long (max %d bytes)", MaxIdempotencyKeyLength), "idempotencyKey")
	}
	// Header values must be visible ASCII: the key is signed verbatim, so a stray control
	// character or surrounding whitespace would silently change the MAC on one side only.
	for i := 0; i < len(key); i++ {
		if key[i] < 0x21 || key[i] > 0x7e {
			return newConfigError(CodeBadIdempotencyKey,
				"the idempotency key must be printable ASCII without spaces", "idempotencyKey")
		}
	}
	return nil
}
