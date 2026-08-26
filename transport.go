package oblodai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime"
	"net/http"
	"net/url"
	"time"
)

// The HTTP engine every resource goes through. One method, execute, does the whole lifecycle:
// serialize -> sign -> send (with a per-attempt timeout) -> decode envelope -> classify error ->
// retry per policy. Bare routes take the same path and keep their bytes instead of a JSON result.

// transport carries the configuration shared by every call.
type transport struct {
	baseURL    string
	creds      *credentials
	httpClient *http.Client
	timeout    time.Duration
	budget     time.Duration
	retry      RetryOptions
	clock      *skewClock
	logger     Logger
	headers    map[string]string
	adminToken string
	userAgent  string
	random     func() float64
}

// callOptions is the per-call state a RequestOption may change.
type callOptions struct {
	body           any
	query          url.Values
	pathParams     map[string]string
	idempotencyKey string
	timeout        time.Duration
	budget         time.Duration
	headers        map[string]string
}

// Response size caps. A JSON answer is a document the core composed; a bare route streams a
// generated PDF or CSV, which is legitimately larger. Past the cap the client reports a contract
// error instead of buffering whatever a proxy decided to send.
const (
	maxJSONResponseBytes = 8 << 20
	maxBareResponseBytes = 64 << 20
)

// skewCorrectionThreshold is how far the measured server offset must be from the one a request was
// signed with before re-signing is worth an extra round trip (half the core's acceptance window).
const skewCorrectionThreshold = (SignatureSkewSeconds / 2) * time.Second

// rawResponse is one HTTP answer, fully read.
type rawResponse struct {
	status      int
	header      http.Header
	body        []byte
	contentType string
}

// Error codes that mean the core rejected the signature because of the timestamp or the MAC.
var signatureFailureCodes = map[string]bool{CodeBadSignature: true, CodeBadTimestamp: true}

// route looks up a generated route by its registry key. A missing key is a programming error in
// this package, never a runtime condition of a caller's program.
func route(key string) Route {
	r, ok := Routes[key]
	if !ok {
		panic("oblodai: unknown route " + key)
	}
	return r
}

// call performs an envelope route and decodes its result into T.
func call[T any](ctx context.Context, t *transport, key string, o callOptions) (*T, error) {
	r := route(key)
	raw, err := t.execute(ctx, r, o)
	if err != nil {
		return nil, err
	}
	result, decodeErr := decodeEnvelope(raw.status, raw.body, decodeContext{now: time.Now()})
	if decodeErr != nil {
		return nil, decodeErr
	}
	// The core replays a cached response by Idempotency-Key; when the original was too large to
	// cache it answers {ok, idempotent_replay: true, detail} instead of the object — surface that
	// rather than handing back a half-empty struct.
	var replay struct {
		IdempotentReplay bool   `json:"idempotent_replay"`
		Detail           string `json:"detail"`
	}
	if json.Unmarshal(result, &replay) == nil && replay.IdempotentReplay {
		return nil, newContractError(fmt.Sprintf(
			"%s: the request was already processed but its response was too large to replay — fetch the result by order_id or reference (%s)",
			key, replay.Detail), raw.status, result)
	}
	var out T
	if err := json.Unmarshal(result, &out); err != nil {
		return nil, newContractError(fmt.Sprintf("%s: the result does not match the documented shape: %v", key, err), raw.status, result)
	}
	return &out, nil
}

// callFile performs a bare route and returns its bytes.
func callFile(ctx context.Context, t *transport, key string, o callOptions) (*FileResult, error) {
	raw, err := t.execute(ctx, route(key), o)
	if err != nil {
		return nil, err
	}
	contentType := raw.contentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &FileResult{Bytes: raw.body, ContentType: contentType, Filename: filenameFrom(raw.header.Get("Content-Disposition"))}, nil
}

func (t *transport) execute(ctx context.Context, r Route, o callOptions) (*rawResponse, *Error) {
	if ctx == nil {
		return nil, newConfigError(CodeBadConfig, "a non-nil context is required", "ctx")
	}
	body, err := serializeBody(o.body, r.Method)
	if err != nil {
		return nil, err
	}
	idempotencyKey := o.idempotencyKey
	if idempotencyKey != "" {
		if err := checkIdempotencyKey(idempotencyKey); err != nil {
			return nil, err
		}
		if !r.Idempotent {
			// The core ignores the header here, so a key would only make this client believe a
			// re-send is deduplicated when it is not — the one belief that turns a lost response
			// into a double spend.
			return nil, newConfigError(CodeIdempotencyUnsupported, fmt.Sprintf(
				"%s %s does not deduplicate by Idempotency-Key; drop WithIdempotencyKey from this call",
				r.Method, r.Path), "idempotencyKey")
		}
	} else if r.Idempotent {
		generated, keyErr := newIdempotencyKey()
		if keyErr != nil {
			return nil, keyErr
		}
		idempotencyKey = generated
	}
	safeToRepeat := r.Safe || (r.Idempotent && idempotencyKey != "")

	budget := t.budget
	if o.budget > 0 {
		budget = o.budget
	}
	deadline := time.Now().Add(budget)
	label := r.Method + " " + r.Path

	extra := t.headers
	if len(o.headers) > 0 {
		extra = map[string]string{}
		for k, v := range t.headers {
			extra[k] = v
		}
		// A per-call header wins over the same client-level one; both lose to the headers the
		// client owns.
		for k, v := range o.headers {
			extra[k] = v
		}
	}
	// The admin token gates merchant provisioning on a self-hosted gateway and goes nowhere else.
	// It is not merged into the caller's headers: X-Admin-Token is a reserved name, so a caller
	// cannot send one, and the client sends it on onboarding routes only.
	adminToken := ""
	if r.Auth == AuthOnboard {
		adminToken = t.adminToken
	}

	limit := int64(maxJSONResponseBytes)
	if r.Bare {
		limit = maxBareResponseBytes
	}

	attempt := 0
	skewTried := false
	var skewBefore, skewInstalled time.Duration
	for {
		ts, signedOffset := t.clock.stamp()
		req, err := buildRequest(buildInput{
			baseURL:        t.baseURL,
			route:          r,
			pathParams:     o.pathParams,
			query:          o.query,
			body:           body,
			creds:          t.creds,
			idempotencyKey: idempotencyKey,
			ts:             ts,
			userAgent:      t.userAgent,
			extraHeaders:   extra,
			adminToken:     adminToken,
		})
		if err != nil {
			return nil, err
		}
		t.logger.Debug("request", LogFields{"route": label, "attempt": attempt, "idempotencyKey": idempotencyKey})

		raw, sendErr := t.send(ctx, req, o, deadline, limit)
		if sendErr != nil {
			if shouldRetry(sendErr, attempt, safeToRepeat, t.retry) {
				if pauseErr := t.pause(ctx, sendErr, attempt, deadline); pauseErr != nil {
					return nil, pauseErr
				}
				attempt++
				continue
			}
			return nil, sendErr
		}
		if raw.status >= 200 && raw.status < 300 {
			return raw, nil
		}

		failure := t.classify(r, raw)
		t.logger.Debug("response", LogFields{
			"route": label, "status": raw.status, "code": failure.Code, "requestId": failure.RequestID,
		})

		// Clock skew: the core rejected the timestamp or the MAC. Learn its time from the Date
		// header, re-sign once, and keep the offset only if that attempt got past authentication.
		// The comparison is against the offset THIS request was signed with, not against whatever
		// the shared clock holds now: another goroutine may have corrected it in between.
		if raw.status == 401 && signatureFailureCodes[failure.Code] {
			if !skewTried {
				if offset, ok := t.clock.observeServerDate(raw.header.Get("Date")); ok &&
					abs(offset-signedOffset) > skewCorrectionThreshold {
					if time.Now().After(deadline) {
						return nil, newDeadlineError(
							"the call budget ran out before the clock-corrected retry; last error: "+failure.Message, failure)
					}
					t.logger.Warn("clock skew detected; re-signing with server time",
						LogFields{"route": label, "offsetSec": int(offset.Seconds())})
					skewTried = true
					skewBefore, skewInstalled = signedOffset, offset
					t.clock.correct(offset)
					continue
				}
			} else {
				// The corrected timestamp did not help: it was not skew. Put the old offset back,
				// but only if this call's correction is still the one in force — a concurrent call
				// that measured its own offset must keep it.
				t.clock.revert(skewInstalled, skewBefore)
			}
		}
		if shouldRetry(failure, attempt, safeToRepeat, t.retry) {
			if pauseErr := t.pause(ctx, failure, attempt, deadline); pauseErr != nil {
				return nil, pauseErr
			}
			attempt++
			continue
		}
		return nil, failure
	}
}

// classify turns a non-2xx answer into the error it describes.
func (t *transport) classify(r Route, raw *rawResponse) *Error {
	_, err := decodeEnvelope(raw.status, raw.body, decodeContext{
		retryAfter: raw.header.Get("Retry-After"),
		location:   raw.header.Get("Location"),
		now:        time.Now(),
	})
	if err != nil {
		return err
	}
	return newContractError(fmt.Sprintf("%s %s: HTTP %d with a success envelope", r.Method, r.Path, raw.status), raw.status, raw.body)
}

// pause waits before the next attempt, refusing to start one that would outlive the call budget.
func (t *transport) pause(ctx context.Context, err *Error, attempt int, deadline time.Time) *Error {
	wait := retryDelay(err, attempt, t.retry, t.random)
	if time.Now().Add(wait).After(deadline) {
		return newDeadlineError("a retry would exceed the call budget; last error: "+err.Message, err)
	}
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return newTransportError(CodeTransportAborted, "the call was cancelled during a retry pause", ctx.Err())
	}
}

// send performs one HTTP attempt and reads the whole body, under one deadline: the per-attempt
// timeout covers the response read, not only the first byte.
func (t *transport) send(ctx context.Context, req *builtRequest, o callOptions, deadline time.Time, limit int64) (*rawResponse, *Error) {
	timeout := t.timeout
	if o.timeout > 0 {
		timeout = o.timeout
	}
	if remaining := time.Until(deadline); remaining < timeout {
		timeout = max(time.Millisecond, remaining)
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var reader io.Reader
	if req.body != nil {
		reader = bytes.NewReader(req.body)
	}
	httpReq, err := http.NewRequestWithContext(attemptCtx, req.method, req.url, reader)
	if err != nil {
		return nil, newConfigError(CodeBadConfig, "the request could not be built: "+err.Error(), "")
	}
	for k, v := range req.headers {
		httpReq.Header.Set(k, v)
	}
	if req.body != nil {
		httpReq.ContentLength = int64(len(req.body))
	}

	res, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, transportErrorFor(ctx, attemptCtx, timeout, err)
	}
	defer res.Body.Close()
	// A signed request must never be replayed against another origin. The client's own redirect
	// policy refuses to follow one, but an injected http.Client may carry a transport that does:
	// compare the URL the answer came from with the one that was signed.
	if res.Request != nil && res.Request.URL != nil && res.Request.URL.String() != req.url {
		return nil, apiErrorFrom(res.StatusCode, errorDetail{
			Code:    "internal",
			Message: fmt.Sprintf("unexpected redirect to %s; check the base URL", res.Request.URL.Redacted()),
		}, nil, true, nil)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, transportErrorFor(ctx, attemptCtx, timeout, err)
	}
	if int64(len(body)) > limit {
		return nil, newResponseTooLargeError(res.StatusCode, limit)
	}
	return &rawResponse{status: res.StatusCode, header: res.Header, body: body, contentType: res.Header.Get("Content-Type")}, nil
}

// transportErrorFor tells a caller cancellation, a caller deadline, this client's per-attempt
// timeout and a genuine network failure apart — they are retried differently.
func transportErrorFor(parent, attempt context.Context, timeout time.Duration, cause error) *Error {
	switch {
	case errors.Is(parent.Err(), context.Canceled):
		return newTransportError(CodeTransportAborted, "the call was cancelled by the caller", cause)
	case errors.Is(parent.Err(), context.DeadlineExceeded):
		return newTransportError(CodeTransportDeadline, "the caller's context deadline passed before the call finished", cause)
	case errors.Is(attempt.Err(), context.DeadlineExceeded):
		return newTransportError(CodeTransportTimeout, fmt.Sprintf("the request timed out after %s", timeout), cause)
	default:
		return newTransportError(CodeTransportNetwork, "network error: "+cause.Error(), cause)
	}
}

// filenameFrom reads the download name out of a Content-Disposition header. mime.ParseMediaType
// already folds the RFC 5987 filename* form into filename.
func filenameFrom(disposition string) string {
	if disposition == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	return params["filename"]
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// defaultRandom is the jitter source; math/rand's global source is safe for concurrent use.
func defaultRandom() float64 { return rand.Float64() }
