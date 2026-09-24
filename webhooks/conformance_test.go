package webhooks_test

import (
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
				header.Set(webhooks.HeaderTimestamp, strconv.FormatInt(v.TS, 10))
				header.Set(webhooks.HeaderSignature, signature)
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
