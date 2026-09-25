package oblodai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime"
	"net/http"
	"strings"
	"time"
)

// The HTTP engine every generated method goes through. One method, execute, does the whole lifecycle:
// serialize -> sign -> send (with a per-attempt timeout) -> decode envelope -> classify error ->
// retry per policy. Bare routes take the same path and keep their bytes instead of a JSON result.

// transport carries the configuration shared by every call. It is the Requester the generated
// services hold.
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
	hooks      Hooks
	// sleep waits out a retry pause; tests record the pause instead.
	sleep func(ctx context.Context, d time.Duration) error
}

// HeaderRequestID carries the call's id to the core and back (WithRequestID).
const HeaderRequestID = "X-Request-ID"

// Response size caps. A JSON answer is a document the core composed; a bare route streams a
// generated PDF or CSV, which is legitimately larger. Past the cap the client reports a contract
// error instead of buffering whatever a proxy decided to send.
const (
	maxJSONResponseBytes = 8 << 20
	maxBareResponseBytes = 64 << 20
)

// skewCorrectionThreshold is how far the measured server offset must be from the one a request was
// signed with before re-signing is worth an extra round trip (half the core's acceptance window).
const skewCorrectionThreshold = (SkewSeconds / 2) * time.Second

// rawResponse is one HTTP answer, fully read.
type rawResponse struct {
	status      int
	header      http.Header
	body        []byte
	contentType string
}

// Error codes that mean the core rejected the signature because of the timestamp or the MAC.
var signatureFailureCodes = map[string]bool{CodeBadSignature: true, CodeBadTimestamp: true}

// request runs one call: serialize, sign, send, classify, retry per policy. The final response —
// success or error — is stored for WithRawResponse, and every error carries the call's request id
// when the core did not name one.
func (t *transport) request(ctx context.Context, call Call, o callOptions) (*rawResponse, *Error) {
	requestID := o.requestID
	if requestID == "" {
		requestID = headerValue(o.headers, HeaderRequestID)
	}
	if requestID == "" {
		requestID = headerValue(t.headers, HeaderRequestID)
	}
	if requestID == "" {
		generated, err := newIdempotencyKey()
		if err != nil {
			return nil, err
		}
		requestID = generated
	}
	if err := checkHeader(HeaderRequestID, requestID); err != nil {
		return nil, err
	}
	raw, err := t.execute(ctx, call, o, requestID)
	if raw != nil && o.raw != nil {
		id := raw.header.Get(HeaderRequestID)
		if id == "" {
			id = requestID
		}
		*o.raw = &RawResponse{StatusCode: raw.status, Header: raw.header.Clone(), Body: raw.body, RequestID: id}
	}
	if err != nil {
		if err.RequestID == "" {
			err.RequestID = requestID
			if raw != nil && raw.header.Get(HeaderRequestID) != "" {
				err.RequestID = raw.header.Get(HeaderRequestID)
			}
		}
		return nil, err
	}
	return raw, nil
}

// execute is the attempt loop. It returns the last response it read (also next to an error, for
// WithRawResponse) and the error the call ends with, if any.
func (t *transport) execute(ctx context.Context, call Call, o callOptions, requestID string) (*rawResponse, *Error) {
	r := call.Route
	if ctx == nil {
		return nil, newConfigError(CodeBadConfig, "a non-nil context is required", "ctx")
	}
	if r.Method == "" || r.Path == "" {
		return nil, newConfigError(CodeBadConfig, "the call names no route (operation "+r.OperationID+")", "")
	}
	idempotencyKey := o.idempotencyKey
	callBody := call.Body
	if idempotencyKey != "" {
		if err := checkIdempotencyKey(idempotencyKey); err != nil {
			return nil, err
		}
	}
	switch {
	case call.IdempotencyKeyInBody:
		// The route takes its key in the body and ignores the header (Ruling 10).
		if idempotencyKey != "" {
			withKey, err := setBodyField(callBody, "idempotency_key", idempotencyKey)
			if err != nil {
				return nil, err
			}
			callBody = withKey
		}
		idempotencyKey = ""
	case idempotencyKey != "" && !r.Idempotent:
		// The core ignores the header here, so a key would only make this client believe a
		// re-send is deduplicated when it is not — the one belief that turns a lost response
		// into a double spend.
		return nil, newConfigError(CodeIdempotencyUnsupported, fmt.Sprintf(
			"%s %s does not deduplicate by %s; drop WithIdempotencyKey from this call",
			r.Method, r.Path, HeaderIdempotencyKey), "idempotencyKey")
	case idempotencyKey == "" && r.Idempotent:
		generated, keyErr := newIdempotencyKey()
		if keyErr != nil {
			return nil, keyErr
		}
		idempotencyKey = generated
	}
	body, err := serializeBody(callBody, r.Method)
	if err != nil {
		return nil, err
	}
	safeToRepeat := r.Safe || (r.Idempotent && idempotencyKey != "")

	retry := t.retry
	if o.maxRetries != nil {
		retry.MaxRetries = *o.maxRetries
	}
	budget := t.budget
	if o.budget > 0 {
		budget = o.budget
	}
	deadline := time.Now().Add(budget)
	label := r.Method + " " + r.Path

	extra := map[string]string{}
	// A per-call header wins over the same client-level one; both lose to the headers the client
	// owns. X-Request-ID is set from requestID, whichever of them named it.
	for _, source := range []map[string]string{t.headers, o.headers} {
		for k, v := range source {
			if !strings.EqualFold(k, HeaderRequestID) {
				extra[k] = v
			}
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
			pathParams:     call.PathParams,
			query:          call.Query,
			body:           body,
			creds:          t.creds,
			idempotencyKey: idempotencyKey,
			ts:             ts,
			userAgent:      t.userAgent,
			extraHeaders:   extra,
			adminToken:     adminToken,
			requestID:      requestID,
		})
		if err != nil {
			return nil, err
		}
		t.logger.Debug("request", LogFields{"route": label, "attempt": attempt, "idempotencyKey": idempotencyKey, "requestId": requestID})

		info := RequestInfo{
			OperationID: r.OperationID, Method: req.method, URL: req.url, Header: hookHeaders(req.headers),
			Attempt: attempt + 1, RequestID: requestID,
		}
		if t.hooks.OnRequest != nil {
			t.hooks.OnRequest(info)
		}
		started := time.Now()
		raw, sendErr := t.send(ctx, req, o, deadline, limit)
		if sendErr != nil {
			t.report(info, nil, started, sendErr)
			if shouldRetry(sendErr, attempt, safeToRepeat, retry) {
				if pauseErr := t.pause(ctx, sendErr, attempt, deadline, retry); pauseErr != nil {
					return nil, pauseErr
				}
				attempt++
				continue
			}
			return nil, sendErr
		}
		if raw.status >= 200 && raw.status < 300 {
			t.report(info, raw, started, nil)
			return raw, nil
		}

		failure := t.classify(r, raw)
		t.report(info, raw, started, failure)
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
						return raw, newDeadlineError(
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
		if shouldRetry(failure, attempt, safeToRepeat, retry) {
			if pauseErr := t.pause(ctx, failure, attempt, deadline, retry); pauseErr != nil {
				return raw, pauseErr
			}
			attempt++
			continue
		}
		return raw, failure
	}
}

// report hands an attempt's outcome to the response hook.
func (t *transport) report(info RequestInfo, raw *rawResponse, started time.Time, failure *Error) {
	if t.hooks.OnResponse == nil {
		return
	}
	out := ResponseInfo{Request: info, Elapsed: time.Since(started)}
	if raw != nil {
		out.StatusCode, out.Header = raw.status, raw.header.Clone()
	}
	if failure != nil {
		out.Err = failure
	}
	t.hooks.OnResponse(out)
}

// headerValue looks a header up case-insensitively.
func headerValue(headers map[string]string, name string) string {
	for k, v := range headers {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// setBodyField returns body as a JSON object with name set to value.
func setBodyField(body any, name, value string) (map[string]any, *Error) {
	fields, err := toFields(body)
	if err != nil {
		return nil, err
	}
	if current, set := fields[name]; set && current != nil && current != "" {
		// Two keys for one call: which one the caller meant is a guess, and a wrong guess
		// re-credits or refuses a retry. Refused before anything is sent, as in every SDK.
		return nil, newConfigError(CodeBadConfig, fmt.Sprintf(
			"the idempotency key is given twice: in the body field %s and by WithIdempotencyKey; keep one", name), name)
	}
	fields[name] = value
	return fields, nil
}

// classify turns a non-2xx answer into the error it describes.
func (t *transport) classify(r RouteSpec, raw *rawResponse) *Error {
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
func (t *transport) pause(ctx context.Context, err *Error, attempt int, deadline time.Time, retry RetryOptions) *Error {
	wait := retryDelay(err, attempt, retry, t.random)
	if time.Now().Add(wait).After(deadline) {
		return newDeadlineError("a retry would exceed the call budget; last error: "+err.Message, err)
	}
	if wait <= 0 {
		return nil
	}
	sleep := t.sleep
	if sleep == nil {
		sleep = sleepContext
	}
	if sleep(ctx, wait) != nil {
		return newTransportError(CodeTransportAborted, "the call was cancelled during a retry pause", ctx.Err())
	}
	return nil
}

// sleepContext waits d or until ctx ends, whichever is first.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
	defer func() { _ = res.Body.Close() }()
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
