package oblodai

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The error envelope is read field by field. A proxy, a WAF or a partially written answer can put
// anything in any of those fields; none of it may cost the caller the error itself, and none of it
// may turn into a wait this client honours.

func TestErrorEnvelopeIsDecodedFieldByField(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		status     int
		code       string
		message    string
		synthetic  bool
		retryable  bool
		retryAfter *int
		requestID  string
	}{
		{
			name: "a complete envelope is taken as written", status: 400,
			body: `{"error":{"code":"invoice.bad_price","message":"too small","field":"amount","retryable":false,"retry_after":7,"request_id":"req-1"}}`,
			code: "invoice.bad_price", message: "too small", retryAfter: intp(7), requestID: "req-1",
		},
		{
			name: "a non-string code makes the answer synthetic but keeps the request id", status: 502,
			body: `{"error":{"code":123,"message":"nope","request_id":"req-2"}}`,
			code: "internal", synthetic: true, retryable: true, requestID: "req-2",
		},
		{
			name: "an empty code makes the answer synthetic", status: 500,
			body: `{"error":{"code":"","message":"nope"}}`,
			code: "internal", synthetic: true, retryable: true,
		},
		{
			name: "a non-string message falls back to the status", status: 500,
			body: `{"error":{"code":"internal.boom","message":{"nested":true}}}`,
			code: "internal.boom", message: "HTTP 500",
		},
		{
			name: "a non-boolean retryable falls back to the status", status: 429,
			body: `{"error":{"code":"request.rate_limited","message":"slow down","retryable":"yes"}}`,
			code: "request.rate_limited", message: "slow down", retryable: true,
		},
		{
			name: "a numeric string retry_after is read", status: 429,
			body: `{"error":{"code":"request.rate_limited","message":"slow down","retryable":true,"retry_after":"12"}}`,
			code: "request.rate_limited", message: "slow down", retryable: true, retryAfter: intp(12),
		},
		{
			name: "a fractional retry_after rounds up", status: 503,
			body: `{"error":{"code":"db.unavailable","message":"later","retryable":true,"retry_after":1.2}}`,
			code: "db.unavailable", message: "later", retryable: true, retryAfter: intp(2),
		},
		{
			name: "a negative retry_after clamps to zero", status: 503,
			body: `{"error":{"code":"db.unavailable","message":"later","retryable":true,"retry_after":-30}}`,
			code: "db.unavailable", message: "later", retryable: true, retryAfter: intp(0),
		},
		{
			name: "an implausible retry_after clamps to a day", status: 503,
			body: `{"error":{"code":"db.unavailable","message":"later","retryable":true,"retry_after":99999999}}`,
			code: "db.unavailable", message: "later", retryable: true, retryAfter: intp(MaxRetryAfterSeconds),
		},
		{
			name: "an unusable retry_after is simply absent", status: 503,
			body: `{"error":{"code":"db.unavailable","message":"later","retryable":true,"retry_after":"soon"}}`,
			code: "db.unavailable", message: "later", retryable: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeEnvelope(tc.status, []byte(tc.body), decodeContext{now: time.Now()})
			if err == nil {
				t.Fatal("expected an error")
			}
			if err.Code != tc.code {
				t.Errorf("code = %q, want %q", err.Code, tc.code)
			}
			if tc.message != "" && err.Message != tc.message {
				t.Errorf("message = %q, want %q", err.Message, tc.message)
			}
			if err.Synthetic != tc.synthetic {
				t.Errorf("synthetic = %v, want %v", err.Synthetic, tc.synthetic)
			}
			if err.Retryable != tc.retryable {
				t.Errorf("retryable = %v, want %v", err.Retryable, tc.retryable)
			}
			if tc.requestID != "" && err.RequestID != tc.requestID {
				t.Errorf("requestId = %q, want %q", err.RequestID, tc.requestID)
			}
			switch {
			case tc.retryAfter == nil && err.RetryAfter != nil:
				t.Errorf("retryAfter = %d, want none", *err.RetryAfter)
			case tc.retryAfter != nil && err.RetryAfter == nil:
				t.Errorf("retryAfter = none, want %d", *tc.retryAfter)
			case tc.retryAfter != nil && *err.RetryAfter != *tc.retryAfter:
				t.Errorf("retryAfter = %d, want %d", *err.RetryAfter, *tc.retryAfter)
			}
		})
	}
}

// details carries the facts a code documents (cli.permission_denied: required_role, role). Only
// string values are kept; a details of the wrong shape is dropped without costing the error.
func TestErrorDetailsAreStringValuesOnly(t *testing.T) {
	cases := map[string]map[string]string{
		`{"required_role":"finance","role":"viewer"}`: {"required_role": "finance", "role": "viewer"},
		`{"role":"viewer","n":3,"x":null}`:            {"role": "viewer"},
		`["finance"]`:                                 nil,
		`"finance"`:                                   nil,
		`{}`:                                          nil,
	}
	for details, want := range cases {
		body := `{"error":{"code":"cli.permission_denied","message":"no","retryable":false,"details":` + details + `}}`
		_, err := decodeEnvelope(403, []byte(body), decodeContext{now: time.Now()})
		if err == nil || err.Code != "cli.permission_denied" || err.Synthetic {
			t.Fatalf("%s: want the core's error, got %+v", details, err)
		}
		if !reflect.DeepEqual(err.Details, want) {
			t.Errorf("%s: details = %v, want %v", details, err.Details, want)
		}
	}
	_, err := decodeEnvelope(403, []byte(`{"error":{"code":"cli.permission_denied","message":"no"}}`), decodeContext{now: time.Now()})
	if err.Details != nil {
		t.Errorf("details without the key = %v, want nil", err.Details)
	}
}

// An envelope whose error object is present but empty is not the core speaking: the caller must
// see a synthetic failure, not an error with an empty code.
func TestErrorObjectWithoutACodeIsSynthetic(t *testing.T) {
	_, err := decodeEnvelope(500, []byte(`{"error":{}}`), decodeContext{now: time.Now()})
	if err == nil || !err.Synthetic || err.Code != "internal" {
		t.Fatalf("want a synthetic internal error, got %+v", err)
	}
	// And an answer with no error key at all is synthetic in the same way.
	_, plain := decodeEnvelope(500, []byte(`{"message":"boom"}`), decodeContext{now: time.Now()})
	if plain == nil || !plain.Synthetic {
		t.Fatalf("want a synthetic error, got %+v", plain)
	}
}

func TestRetryAfterHeaderIsClampedAndNeverNegative(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	cases := map[string]*int{
		"":          nil,
		"soon":      nil,
		"30":        intp(30),
		"-30":       intp(0),
		"999999999": intp(MaxRetryAfterSeconds),
		now.Add(-time.Hour).Format(http.TimeFormat):       intp(0),
		now.Add(90 * time.Second).Format(http.TimeFormat): intp(90),
		now.AddDate(400, 0, 0).Format(http.TimeFormat):    intp(MaxRetryAfterSeconds),
		"Mon, 02 Jan 3006 15:04:05 GMT":                   intp(MaxRetryAfterSeconds),
	}
	for header, want := range cases {
		got := parseRetryAfter(header, now)
		switch {
		case want == nil && got != nil:
			t.Errorf("Retry-After %q = %d, want none", header, *got)
		case want != nil && got == nil:
			t.Errorf("Retry-After %q = none, want %d", header, *want)
		case want != nil && *got != *want:
			t.Errorf("Retry-After %q = %d, want %d", header, *got, *want)
		}
		if got != nil && (*got < 0 || *got > MaxRetryAfterSeconds) {
			t.Errorf("Retry-After %q = %d, outside [0, %d]", header, *got, MaxRetryAfterSeconds)
		}
	}
}

// The reported Retry-After may be up to a day; what the retry loop actually sleeps stays bounded
// by the retry policy, so an hour-long hint cannot wedge a call.
func TestALongRetryAfterIsReportedButNotSlept(t *testing.T) {
	api := newFakeAPI(t,
		apiError(429, map[string]any{"code": "request.rate_limited", "message": "slow", "retryable": true, "retry_after": 3600}),
		ok(map[string]any{"uuid": "p1"}))
	client := api.client(WithRetry(RetryOptions{MaxRetries: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, MaxRetryAfter: 5 * time.Millisecond}))
	started := time.Now()
	if _, err := client.Payments.GetInfo(context.Background(), &LookupRequest{UUID: Ptr("p1")}); err != nil {
		t.Fatalf("Payments.GetInfo: %v", err)
	}
	if waited := time.Since(started); waited > time.Second {
		t.Fatalf("the client slept %s; MaxRetryAfter must bound the wait", waited)
	}

	failing := newFakeAPI(t, apiError(429, map[string]any{
		"code": "request.rate_limited", "message": "slow", "retryable": true, "retry_after": 3600,
	}))
	_, err := failing.client(WithRetry(RetryOptions{MaxRetries: 0})).Payments.GetInfo(context.Background(), &LookupRequest{UUID: Ptr("p1")})
	apiErr := mustError(t, err)
	if apiErr.RetryAfter == nil || *apiErr.RetryAfter != 3600 {
		t.Fatalf("the reported Retry-After must survive: %+v", apiErr.RetryAfter)
	}
}

// A response bigger than the client will buffer is a contract failure, not an out-of-memory.
func TestResponseLargerThanTheCapIsRefused(t *testing.T) {
	huge := `{"state":0,"result":{"uuid":"` + strings.Repeat("a", maxJSONResponseBytes) + `"}}`
	api := newFakeAPI(t, step{status: 200, body: huge})
	_, err := api.client().Payments.GetInfo(context.Background(), &LookupRequest{UUID: Ptr("p1")})
	apiErr := mustError(t, err)
	if !IsContract(err) || apiErr.Code != CodeResponseTooLarge {
		t.Fatalf("want sdk.response_too_large, got %v", err)
	}
}

func intp(v int) *int { return &v }
