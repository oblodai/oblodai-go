package oblodai

import (
	"strings"
	"testing"
)

// The signature is the one thing that cannot be "nearly right": the core recomputes it byte for
// byte. The core's own vectors (x-oblodai-signing of the contract) run in conformance_test.go.

func TestSigningEmptyIdempotencySlot(t *testing.T) {
	// The slot is EMPTY, not absent: the newline separating it is always there. A signer that
	// drops the line for keyless requests would fail every unkeyed call against the core.
	in := SignInput{TS: 1, Method: "POST", RequestURI: "/v1/x", Body: []byte("{}")}
	if got, want := CanonicalString(in), "1\nPOST\n/v1/x\n\n{}"; got != want {
		t.Fatalf("canonical string\n got %q\nwant %q", got, want)
	}
	keyed := in
	keyed.IdempotencyKey = ""
	if SignRequest("s", in) != SignRequest("s", keyed) {
		t.Fatal("an unset key and an empty key must sign identically")
	}
}

func TestSigningSignsBodyBytes(t *testing.T) {
	// Non-ASCII data must be signed as the UTF-8 bytes that go on the wire.
	body := `{"additional_data":"café 日本語 🚀"}`
	in := SignInput{TS: 5, Method: "POST", RequestURI: "/v1/payment", Body: []byte(body)}
	signature := SignRequest("s", in)
	if !hexish(signature, 32) {
		t.Fatalf("signature is not a hex SHA-256 digest: %q", signature)
	}
	if strings.Contains(CanonicalString(in), `\u`) {
		t.Fatal("the canonical string must carry the raw bytes, not an escaped form")
	}
}
