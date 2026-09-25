package oblodai_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oblodai/oblodai-go/v2"
	"github.com/oblodai/oblodai-go/v2/internal/conformance"
	"github.com/oblodai/oblodai-go/v2/webhooks"
)

// The signing protocol has one source: x-oblodai-signing of the contract, generated into
// zz_generated_signing.go (and webhooks/zz_generated_signing.go). The runtime reads the header
// names, the canonical strings and the limits from there, so a header the core renames reaches this
// SDK by regeneration alone.

func strs(t *testing.T, v any) []string {
	t.Helper()
	list, ok := v.([]any)
	if !ok {
		t.Fatalf("not a list: %v", v)
	}
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = x.(string)
	}
	return out
}

func TestGeneratedSigningIsTheContracts(t *testing.T) {
	s := conformance.Signing(t)
	hook := s["webhook"].(map[string]any)
	want := map[string]string{}
	for i, name := range strs(t, s["headers"]) {
		want[[]string{"HeaderPublicID", "HeaderSignature", "HeaderTimestamp", "HeaderIdempotencyKey"}[i]] = name
	}
	for i, name := range strs(t, hook["headers"]) {
		want["webhooks."+[]string{"HeaderTimestamp", "HeaderSignature", "HeaderSignaturePrev", "HeaderEvent",
			"HeaderID", "HeaderEventID", "HeaderEventTime"}[i]] = name
	}
	want["webhooks.HeaderTest"] = hook["test_header"].(string)
	got := map[string]string{
		"HeaderPublicID": oblodai.HeaderPublicID, "HeaderSignature": oblodai.HeaderSignature,
		"HeaderTimestamp": oblodai.HeaderTimestamp, "HeaderIdempotencyKey": oblodai.HeaderIdempotencyKey,
		"webhooks.HeaderTimestamp": webhooks.HeaderTimestamp, "webhooks.HeaderSignature": webhooks.HeaderSignature,
		"webhooks.HeaderSignaturePrev": webhooks.HeaderSignaturePrev, "webhooks.HeaderEvent": webhooks.HeaderEvent,
		"webhooks.HeaderID": webhooks.HeaderID, "webhooks.HeaderEventID": webhooks.HeaderEventID,
		"webhooks.HeaderEventTime": webhooks.HeaderEventTime, "webhooks.HeaderTest": webhooks.HeaderTest,
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %q, the contract says %q", name, got[name], w)
		}
	}
	for _, c := range []struct {
		name      string
		got, want any
	}{
		{"SkewSeconds", float64(oblodai.SkewSeconds), s["skew_seconds"]},
		{"MaxBody", float64(oblodai.MaxBody), s["max_body"]},
		{"MaxIdempotencyKeyLength", float64(oblodai.MaxIdempotencyKeyLength), s["max_idempotency_key_length"]},
		{"SignatureAlgorithm", oblodai.SignatureAlgorithm, s["algorithm"]},
		{"SignatureSkewSeconds (1.x name)", float64(oblodai.SignatureSkewSeconds), s["skew_seconds"]},
		{"webhooks.MaxBodySize", float64(webhooks.MaxBodySize), s["max_body"]},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v, the contract says %v", c.name, c.got, c.want)
		}
	}
}

// No header name of the signing protocol — request, webhook delivery or rehearsal — is spelled in a
// hand-written source file: every one comes from the generated code.
func TestNoSigningHeaderOutsideGenerated(t *testing.T) {
	s := conformance.Signing(t)
	hook := s["webhook"].(map[string]any)
	var names []string
	for _, n := range append(append(strs(t, s["headers"]), strs(t, hook["headers"])...), hook["test_header"].(string)) {
		names = append(names, strings.ToLower(n))
	}
	var offenders []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		base := d.Name()
		if d.IsDir() {
			if path != "." && strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "zz_generated_") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := strings.ToLower(string(data))
		for _, n := range names {
			if strings.Contains(text, n) {
				offenders = append(offenders, path+": "+n)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("signing header names spelled outside the generated code:\n%s", strings.Join(offenders, "\n"))
	}
}
