package oblodai

import (
	"testing"
	"time"
)

func TestConfigReadsCredentialsAndBaseURLFromTheEnvironment(t *testing.T) {
	t.Setenv("OBLODAI_PUBLIC_ID", "pk")
	t.Setenv("OBLODAI_SECRET", "s")
	t.Setenv("OBLODAI_PAYOUT_PUBLIC_ID", "wk")
	t.Setenv("OBLODAI_PAYOUT_SECRET", "ws")
	t.Setenv("OBLODAI_BASE_URL", "https://x.test/")
	t.Setenv("OBLODAI_ADMIN_TOKEN", "adm")

	client, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if client.BaseURL() != "https://x.test" {
		t.Fatalf("BaseURL = %q", client.BaseURL())
	}
	if client.transport.creds.publicID != "pk" || client.transport.creds.secret != "s" {
		t.Fatalf("credentials = %+v", client.transport.creds)
	}
	if client.transport.payoutCreds.publicID != "wk" {
		t.Fatalf("payout credentials = %+v", client.transport.payoutCreds)
	}
	if client.transport.adminToken != "adm" {
		t.Fatalf("admin token = %q", client.transport.adminToken)
	}
}

func TestConfigOptionsWinOverTheEnvironment(t *testing.T) {
	t.Setenv("OBLODAI_PUBLIC_ID", "from-env")
	t.Setenv("OBLODAI_SECRET", "env-secret")
	client, err := New(WithCredentials("explicit", "explicit-secret"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if client.transport.creds.publicID != "explicit" {
		t.Fatalf("public id = %q", client.transport.creds.publicID)
	}
}

func TestConfigRefusesPlainHTTPExceptOnLoopback(t *testing.T) {
	if _, err := New(WithBaseURL("http://api.oblodai.com")); !IsCode(err, CodeBadConfig) {
		t.Fatalf("plain http must be refused, got %v", err)
	}
	for _, local := range []string{"http://localhost:8095", "http://127.0.0.1:8095", "http://[::1]:8095"} {
		if _, err := New(WithBaseURL(local)); err != nil {
			t.Fatalf("loopback %s must be allowed: %v", local, err)
		}
	}
	if _, err := New(WithBaseURL("http://10.0.0.1"), WithInsecureBaseURL(true)); err != nil {
		t.Fatalf("an explicit opt-in must be honoured: %v", err)
	}
	if _, err := New(WithBaseURL("not a url")); !IsCode(err, CodeBadConfig) {
		t.Fatalf("a malformed base URL must be refused, got %v", err)
	}
}

func TestConfigRefusesHalfAKeyPair(t *testing.T) {
	if _, err := New(WithCredentials("pk", "")); !IsCode(err, CodeBadConfig) {
		t.Fatalf("half a key pair must be refused, got %v", err)
	}
	if _, err := New(WithPayoutCredentials("", "s")); !IsCode(err, CodeBadConfig) {
		t.Fatalf("half a payout key pair must be refused, got %v", err)
	}
}

func TestConfigDefaults(t *testing.T) {
	client, err := New(withClock(time.Now))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if client.BaseURL() != DefaultBaseURL {
		t.Fatalf("BaseURL = %q, want %q", client.BaseURL(), DefaultBaseURL)
	}
	if client.transport.timeout != 30*time.Second || client.transport.budget != 90*time.Second {
		t.Fatalf("timeouts = %s / %s", client.transport.timeout, client.transport.budget)
	}
	if client.transport.retry.MaxRetries != 2 {
		t.Fatalf("MaxRetries = %d", client.transport.retry.MaxRetries)
	}
	// A partially specified policy keeps the documented defaults for what it does not set.
	custom, err := New(WithRetry(RetryOptions{MaxRetries: 5}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if custom.transport.retry.BaseDelay != DefaultRetry().BaseDelay {
		t.Fatalf("BaseDelay = %s", custom.transport.retry.BaseDelay)
	}
}

func TestMoneyHelpersWorkAtArbitraryPrecision(t *testing.T) {
	sum, err := AddAmounts("0.1", "0.2")
	if err != nil || sum != "0.3" {
		t.Fatalf("0.1 + 0.2 = %q (%v) — a float would have said 0.30000000000000004", sum, err)
	}
	if sum, _ := AddAmounts("10.000000", "0.5"); sum != "10.500000" {
		t.Fatalf("scale must follow the widest operand, got %q", sum)
	}
	if diff, _ := SubtractAmounts("1", "1.000001"); diff != "-0.000001" {
		t.Fatalf("1 - 1.000001 = %q", diff)
	}
	if cmp, _ := CompareAmounts("25", "25.000000"); cmp != 0 {
		t.Fatalf("25 and 25.000000 must compare equal, got %d", cmp)
	}
	if cmp, _ := CompareAmounts("0.000000000000000001", "0"); cmp != 1 {
		t.Fatalf("18 decimals must not be rounded away, got %d", cmp)
	}
	if !IsZeroAmount("0.000000") || IsZeroAmount("0.000001") {
		t.Fatal("IsZeroAmount is wrong")
	}
	if !AmountsEqual("1.5", "1.50") || AmountsEqual("1.5", "1.51") {
		t.Fatal("AmountsEqual is wrong")
	}
	if _, err := AddAmounts("1", "not-a-number"); err == nil {
		t.Fatal("a malformed amount must be an error, not a silent zero")
	}
}

func TestStatusHelpersFollowTheCoreVocabulary(t *testing.T) {
	if !IsPaymentPaid(PaymentStatusPaidOver) || IsPaymentPaid(PaymentStatusWrongAmount) {
		t.Fatal("wrong_amount is an underpayment awaiting a decision, not a payment")
	}
	if !IsPaymentUnderpaid(PaymentStatusWrongAmount) {
		t.Fatal("wrong_amount must be reported as underpaid")
	}
	if IsPaymentFinal(PaymentStatusConfirmCheck) || !IsPaymentFinal(PaymentStatusExpired) {
		t.Fatal("payment finality is wrong")
	}
	if IsPayoutFinal(PayoutStatusSent) || !IsPayoutFinal(PayoutStatusConfirmed) {
		t.Fatal("a sent payout is not final yet; a confirmed one is")
	}
	if !IsPayoutSucceeded(PayoutStatusConfirmed) || IsPayoutSucceeded(PayoutStatusFailed) {
		t.Fatal("payout success is wrong")
	}
}

func TestIdempotencyKeysAreUUIDsAndValidated(t *testing.T) {
	key, err := NewIdempotencyKey()
	if err != nil {
		t.Fatalf("NewIdempotencyKey: %v", err)
	}
	if len(key) != 36 || key[14] != '4' {
		t.Fatalf("NewIdempotencyKey = %q, want a v4 UUID", key)
	}
	other, err := NewIdempotencyKey()
	if err != nil {
		t.Fatalf("NewIdempotencyKey: %v", err)
	}
	if key == other {
		t.Fatal("keys must be unique")
	}
	if err := checkIdempotencyKey(""); err == nil {
		t.Fatal("an empty key must be refused")
	}
	if err := checkIdempotencyKey("has space"); err == nil {
		t.Fatal("a key with a space must be refused: it would sign differently on each side")
	}
	long := make([]byte, MaxIdempotencyKeyLength+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := checkIdempotencyKey(string(long)); err == nil {
		t.Fatal("an over-long key must be refused")
	}
}

func TestLoggerRedactsSecrets(t *testing.T) {
	if got := redact("secret", "s3cr3t"); got != "[redacted]" {
		t.Fatalf("a secret reached the log: %v", got)
	}
	if got := redact("x-signature", "abcd"); got != "[redacted]" {
		t.Fatalf("a signature reached the log: %v", got)
	}
	if got := redact("route", "POST /v1/payment"); got != "POST /v1/payment" {
		t.Fatalf("an ordinary field must survive: %v", got)
	}
}
