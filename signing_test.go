package oblodai

import (
	"strings"
	"testing"

	"github.com/oblodai/oblodai-go/v2/internal/fixtures"
)

// The signature is the one thing that cannot be "nearly right": the core recomputes it byte for
// byte. These vectors were exported from the core's own test suite.

func TestSigningVectors(t *testing.T) {
	contract := fixtures.LoadContract(t)
	if len(contract.SigningVectors) == 0 {
		t.Fatal("contract.json carries no signing vectors")
	}
	for _, vector := range contract.SigningVectors {
		t.Run(vector.Name, func(t *testing.T) {
			in := SignInput{
				TS:             vector.TS,
				Method:         vector.Method,
				RequestURI:     vector.RequestURI,
				IdempotencyKey: vector.IdempotencyKey,
				Body:           []byte(vector.Body),
			}
			if got := CanonicalString(in); got != vector.Canonical {
				t.Errorf("canonical string\n got %q\nwant %q", got, vector.Canonical)
			}
			if got := SignRequest(vector.Secret, in); got != vector.Signature {
				t.Errorf("signature\n got %s\nwant %s", got, vector.Signature)
			}
		})
	}
}

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

func TestWebhookSigningVectors(t *testing.T) {
	contract := fixtures.LoadContract(t)
	if len(contract.WebhookVectors) == 0 {
		t.Fatal("contract.json carries no webhook vectors")
	}
	for i, vector := range contract.WebhookVectors {
		if got := SignWebhook(vector.Secret, vector.TS, []byte(vector.Payload)); got != vector.Signature {
			t.Errorf("webhook vector %d\n got %s\nwant %s", i, got, vector.Signature)
		}
	}
}
