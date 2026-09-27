package oblodai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// R3: a claim token in the path and a signed link's sig/exp in the query are bearer secrets. They
// must not reach an error's text, its JSON, its unwrapped cause, or a hook's URL.
func TestClaimTokenNeverReachesErrorsOrHooks(t *testing.T) {
	const token = "CLAIMSECRET_abc123"
	var mu sync.Mutex
	var urls []string
	client, err := New(WithBaseURL("http://127.0.0.1:1"), WithInsecureBaseURL(true),
		WithRetry(RetryOptions{MaxRetries: 0}), WithTimeout(2*time.Second),
		WithHooks(Hooks{OnRequest: func(r RequestInfo) { mu.Lock(); urls = append(urls, r.URL); mu.Unlock() }}))
	if err != nil {
		t.Fatal(err)
	}
	_, callErr := client.PayoutLinks.GetPayoutClaim(context.Background(), token)
	if callErr == nil {
		t.Fatal("a closed port must fail")
	}
	encoded, _ := json.Marshal(callErr)
	rendered := []string{callErr.Error(), string(encoded), fmt.Sprintf("%+v", callErr)}
	for cause := errors.Unwrap(callErr); cause != nil; cause = errors.Unwrap(cause) {
		rendered = append(rendered, cause.Error())
	}
	mu.Lock()
	rendered = append(rendered, urls...)
	mu.Unlock()
	for _, text := range rendered {
		if strings.Contains(text, token) {
			t.Fatalf("the claim token leaked: %s", text)
		}
	}
	if len(urls) != 1 || !strings.Contains(urls[0], "/v1/claim/[redacted]") {
		t.Fatalf("hook URLs = %q, want the route with the token redacted", urls)
	}
	if !strings.Contains(callErr.Error(), "/v1/claim/[redacted]") {
		t.Fatalf("the error should still name the (redacted) URL: %v", callErr)
	}
}

func TestSignedLinkQueryAndSecretHeadersAreRedactedInHooks(t *testing.T) {
	api := newFakeAPI(t, step{status: 200, body: "%PDF", headers: map[string]string{"Content-Type": "application/pdf"}})
	var req RequestInfo
	var res ResponseInfo
	client := api.client(
		WithHeader("Authorization", "Bearer caller-secret"),
		WithHeader("X-Api-Key", "caller-key"),
		WithHeader("X-Claim-Passcode", "4321"),
		WithHooks(Hooks{OnRequest: func(r RequestInfo) { req = r }, OnResponse: func(r ResponseInfo) { res = r }}),
	)
	if _, err := client.Documents.GetSigned(context.Background(), "receipt", "p1",
		&GetSignedDocumentParams{Exp: 1790000000, Sig: "SIGSECRET", Lang: Ptr("en")}); err != nil {
		t.Fatalf("Documents.GetSigned: %v", err)
	}
	if strings.Contains(req.URL, "SIGSECRET") || strings.Contains(req.URL, "1790000000") || !strings.Contains(req.URL, "lang=en") {
		t.Fatalf("hook URL = %s", req.URL)
	}
	if got := api.last().rawQuery; !strings.Contains(got, "sig=SIGSECRET") {
		t.Fatalf("the wire must still carry the real signature, got %q", got)
	}
	for _, name := range []string{"Authorization", "X-Api-Key", "X-Claim-Passcode"} {
		if v := req.Header.Get(name); v != redactedPlaceholder {
			t.Errorf("hook header %s = %q, want %s", name, v, redactedPlaceholder)
		}
	}
	if res.Request.URL != req.URL {
		t.Fatalf("the response hook must see the same redacted URL")
	}
}

func TestBadSecretPathParamIsNotEchoed(t *testing.T) {
	api := newFakeAPI(t)
	_, err := api.client().PayoutLinks.GetPayoutClaim(context.Background(), "abc/SECRETPART")
	if !IsCode(err, CodeBadPathParam) || strings.Contains(err.Error(), "SECRETPART") {
		t.Fatalf("err = %v", err)
	}
}

func TestModelsRedactDeviceCodeAndSignedLinks(t *testing.T) {
	auth := CLIDeviceAuthorization{DeviceCode: "DEVCODE-secret", UserCode: "ABCD-EFGH"}
	payout := PayoutView{DocumentURL: "https://api.oblodai.com/v1/documents/payout/p1?exp=1790000000&sig=SIGSECRET&lang=en"}
	for _, text := range []string{fmt.Sprint(auth), fmt.Sprintf("%#v", auth), fmt.Sprint(payout), fmt.Sprintf("%+v", payout)} {
		if strings.Contains(text, "DEVCODE-secret") || strings.Contains(text, "SIGSECRET") || strings.Contains(text, "1790000000") {
			t.Fatalf("a secret leaked: %s", text)
		}
	}
	if !strings.Contains(fmt.Sprint(auth), "ABCD-EFGH") || !strings.Contains(fmt.Sprint(payout), "lang=en") {
		t.Fatalf("non-secret values must stay: %s / %s", auth, payout)
	}
	fields := redactFields(LogFields{"device_code": "DEVCODE-secret", "Cookie": "a=b"})
	if fields["device_code"] != redactedPlaceholder || fields["Cookie"] != redactedPlaceholder {
		t.Fatalf("log fields = %v", fields)
	}
}
