package oblodai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Every failure this package reports is an *Error, mirroring the core's error envelope:
//
//	{"error": {"code", "message", "field"?, "retryable", "retry_after"?, "request_id"?}}
//
// Retryable is authoritative when the core wrote the envelope: it is the core's own classification
// of the failure, and the client has already retried what it was safe to retry. A response without
// an envelope (a proxy 502, an HTML 503) is Synthetic — the core never saw or never answered the
// request — and is retried only when repeating is safe.
//
// The discriminator you branch on is always Code (a stable family.reason string); Kind groups
// codes by HTTP status for the ergonomic Is* predicates.

// Kind classifies an error for the Is* predicates.
type Kind string

const (
	// KindValidation is HTTP 400: the request is malformed or violates a business rule. Field
	// names the offending request field when the core knows it.
	KindValidation Kind = "validation"
	// KindAuthentication is HTTP 401: bad signature, unknown key, clock skew, IP not allow-listed.
	KindAuthentication Kind = "authentication"
	// KindPermission is HTTP 403: the key is valid but may not do this (a feature is off, an
	// object belongs to someone else).
	KindPermission Kind = "permission"
	// KindNotFound is HTTP 404: no such object for this merchant.
	KindNotFound Kind = "not_found"
	// KindConflict is HTTP 409: a state conflict.
	KindConflict Kind = "conflict"
	// KindIdempotencyConflict is HTTP 409 idempotency.key_reused: the same Idempotency-Key was used
	// with a different request body.
	KindIdempotencyConflict Kind = "idempotency_conflict"
	// KindRateLimit is HTTP 429; RetryAfter is set.
	KindRateLimit Kind = "rate_limit"
	// KindUnavailable is HTTP 503: an upstream dependency is down; safe to retry after a pause.
	KindUnavailable Kind = "unavailable"
	// KindInternal is a 5xx other than 503.
	KindInternal Kind = "internal"
	// KindAPI is any other error status the API answered with.
	KindAPI Kind = "api"
	// KindTransport means no HTTP response was produced: DNS, TCP, TLS, timeout, cancellation.
	KindTransport Kind = "transport"
	// KindConfig means the client refused before sending: bad options, missing credentials,
	// unusable arguments.
	KindConfig Kind = "config"
	// KindContract means the response could not be read as the documented envelope.
	KindContract Kind = "contract"
	// KindSignature means webhook verification failed.
	KindSignature Kind = "signature"
)

// Codes the SDK itself raises before or instead of a request.
const (
	CodeMissingCredentials     = "sdk.missing_credentials"
	CodeBadConfig              = "sdk.bad_config"
	CodeBadIdempotencyKey      = "sdk.bad_idempotency_key"
	CodeIdempotencyUnsupported = "sdk.idempotency_unsupported"
	CodeBadEnvelope            = "sdk.bad_envelope"
	CodeBadPathParam           = "sdk.bad_path_param"
	CodeBadAmount              = "sdk.bad_amount"
	CodeFloatAmount            = "sdk.float_amount"
	CodeWaitTimeout            = "sdk.wait_timeout"
	CodeBadHeader              = "sdk.bad_header"
	CodeResponseTooLarge       = "sdk.response_too_large"

	CodeTransportTimeout  = "transport.timeout"
	CodeTransportNetwork  = "transport.network"
	CodeTransportAborted  = "transport.aborted"
	CodeTransportDeadline = "transport.deadline"

	CodeWebhookBadSignature   = "webhook.bad_signature"
	CodeWebhookStaleTimestamp = "webhook.stale_timestamp"
	CodeWebhookMissingHeader  = "webhook.missing_header"
	// CodeWebhookBadPayload marks a delivery whose signature is genuine but whose body cannot be
	// read. It is a contract failure, not a signature failure: a receiver that answers 401 to
	// forged deliveries must not answer 401 to an authentic event it failed to parse.
	CodeWebhookBadPayload = "webhook.bad_payload"

	// CodeIdempotencyKeyReused is the core's 409 for a key replayed with a different body. The
	// value is the generated ErrorCodeIdempotencyKeyReused, so a code renamed in the contract
	// fails the build instead of silently unmapping IdempotencyConflictError.
	CodeIdempotencyKeyReused = string(ErrorCodeIdempotencyKeyReused)
	// CodeWrongKeyKind is a legacy 403: it can only reach a merchant who still holds an old split
	// key pair (oblodai_pk_… / oblodai_wk_…). One API key signs everything, so a current merchant
	// never sees it — the constant is kept so an old integration can still branch on it.
	CodeWrongKeyKind = "merchant.wrong_key_kind"
	// CodeBadSignature and CodeBadTimestamp are the two 401s that can mean clock skew. Both are
	// the generated ErrorCode values, so a rename in the contract fails the build instead of
	// silently switching the clock resync off.
	CodeBadSignature = string(ErrorCodeMerchantBadSignature)
	CodeBadTimestamp = string(ErrorCodeAuthBadTimestamp)
)

// MaxRetryAfterSeconds bounds what the client reports as RetryAfter, whether the hint came from
// the error envelope or from a Retry-After header: a day. It is a plausibility bound on the
// reported value only — the retry loop still sleeps at most RetryOptions.MaxRetryAfter.
const MaxRetryAfterSeconds = 86400

// clampRetryAfter folds any retry hint into [0, MaxRetryAfterSeconds]. Arithmetic happens in
// int64 so a far-future HTTP-date cannot overflow into a negative wait.
func clampRetryAfter(seconds int64) int {
	if seconds < 0 {
		seconds = 0
	}
	if seconds > MaxRetryAfterSeconds {
		seconds = MaxRetryAfterSeconds
	}
	return int(seconds)
}

// Error is the one error type this package returns. Recover it with errors.As:
//
//	var apiErr *oblodai.Error
//	if errors.As(err, &apiErr) && apiErr.Code == "payout.insufficient_funds" { … }
type Error struct {
	// Kind groups the failure for the Is* predicates.
	Kind Kind
	// Code is the stable machine code, family.reason (for example payout.insufficient_funds).
	Code string
	// Message is the human-readable explanation; never branch on it.
	Message string
	// HTTPStatus is the response status, or 0 when no response was received.
	HTTPStatus int
	// Retryable reports whether repeating the identical request can succeed later.
	Retryable bool
	// RetryAfter is how many seconds to wait before retrying, when the core or a Retry-After
	// header provided a hint. Nil when there was none; 0 means "immediately".
	RetryAfter *int
	// RequestID is the server-side request id — quote it when contacting support.
	RequestID string
	// Field is the request field a validation error refers to.
	Field string
	// Synthetic reports that no core envelope was present: the answer came from something in front
	// of the core, so the core may or may not have performed the operation.
	Synthetic bool
	// LastCode is the code of the failure that was in force when the client gave up. It is set on
	// transport.deadline, where Code describes the client's own decision to stop and LastCode
	// (together with HTTPStatus, RetryAfter and RequestID, copied from the same error) describes
	// what the API last said. Unwrap returns that error itself.
	LastCode string

	// raw is the response body. It is deliberately unexported and dropped from MarshalJSON so a
	// structured log of the error cannot leak a secret the body carried.
	raw []byte
	// cause is the underlying error, reachable with errors.Unwrap.
	cause error
}

// Error reads as "[code] message (request_id=…)" — the code to branch on, the explanation, and the
// id to quote to support; the suffix only when there is an id.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("[" + e.Code + "]")
	if e.Message != "" {
		b.WriteString(" " + e.Message)
	}
	if e.RequestID != "" {
		b.WriteString(" (request_id=" + e.RequestID + ")")
	}
	return b.String()
}

// Unwrap returns the underlying cause, if any.
func (e *Error) Unwrap() error { return e.cause }

// Family is the first half of the code: "payout" in "payout.insufficient_funds".
func (e *Error) Family() string {
	if i := strings.IndexByte(e.Code, '.'); i >= 0 {
		return e.Code[:i]
	}
	return e.Code
}

// Body returns a copy of the raw response body the error came with, for debugging. It is not part
// of the error's own serialization on purpose.
func (e *Error) Body() []byte {
	if e.raw == nil {
		return nil
	}
	out := make([]byte, len(e.raw))
	copy(out, e.raw)
	return out
}

// MarshalJSON keeps the fields a structured logger wants and drops the raw body.
func (e *Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Kind       Kind   `json:"kind"`
		Code       string `json:"code"`
		Message    string `json:"message"`
		HTTPStatus int    `json:"httpStatus"`
		Retryable  bool   `json:"retryable"`
		RetryAfter *int   `json:"retryAfter,omitempty"`
		RequestID  string `json:"requestId,omitempty"`
		Field      string `json:"field,omitempty"`
		Synthetic  bool   `json:"synthetic"`
		LastCode   string `json:"lastCode,omitempty"`
	}{e.Kind, e.Code, e.Message, e.HTTPStatus, e.Retryable, e.RetryAfter, e.RequestID, e.Field, e.Synthetic, e.LastCode})
}

// newConfigError is raised before any request leaves the process.
func newConfigError(code, message, field string) *Error {
	return &Error{Kind: KindConfig, Code: code, Message: message, Field: field}
}

// NewConfigError builds the error this package raises when a caller's configuration cannot be
// used. It is exported so the webhooks sub-package can refuse an unusable secret or tolerance
// with exactly the error type — and the kind — the client documents.
func NewConfigError(code, message, field string) *Error {
	return newConfigError(code, message, field)
}

// newTransportError describes a request that never produced an HTTP response.
func newTransportError(code, message string, cause error) *Error {
	return &Error{
		Kind:      KindTransport,
		Code:      code,
		Message:   message,
		Retryable: code == CodeTransportTimeout || code == CodeTransportNetwork,
		cause:     cause,
	}
}

// newDeadlineError reports that the call budget ran out before another attempt could be made. It
// carries the identity of the last failure — code (as LastCode), HTTP status, Retry-After and
// request id — so a caller that inspects the returned error still sees what the API said, and
// keeps it as the cause so errors.Unwrap and errors.Is reach it.
func newDeadlineError(message string, last *Error) *Error {
	e := &Error{Kind: KindTransport, Code: CodeTransportDeadline, Message: message, cause: last}
	if last != nil {
		e.LastCode = last.Code
		e.HTTPStatus = last.HTTPStatus
		e.RetryAfter = last.RetryAfter
		e.RequestID = last.RequestID
		e.Field = last.Field
		e.Synthetic = last.Synthetic
	}
	return e
}

// newResponseTooLargeError describes an answer the client refused to buffer.
func newResponseTooLargeError(httpStatus int, limit int64) *Error {
	return &Error{
		Kind:       KindContract,
		Code:       CodeResponseTooLarge,
		Message:    fmt.Sprintf("the response is larger than the %d-byte limit this client reads; it was not buffered", limit),
		HTTPStatus: httpStatus,
	}
}

// NewWebhookPayloadError builds the error an authentic delivery with an unreadable body reports.
// It is a contract error, not a signature error, so a receiver can answer 5xx (and have the core
// retry the delivery) instead of 401.
func NewWebhookPayloadError(message string) *Error {
	return &Error{Kind: KindContract, Code: CodeWebhookBadPayload, Message: message}
}

// newContractError describes a response that is not the documented envelope.
func newContractError(message string, httpStatus int, raw []byte) *Error {
	return &Error{Kind: KindContract, Code: CodeBadEnvelope, Message: message, HTTPStatus: httpStatus, raw: raw}
}

// NewSignatureError builds the error a failed webhook verification reports. It is exported so the
// webhooks sub-package can produce exactly the error type this package documents.
func NewSignatureError(code, message string) *Error {
	return &Error{Kind: KindSignature, Code: code, Message: message}
}

// errorDetail is the error envelope the core writes.
type errorDetail struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Field      string `json:"field"`
	Retryable  *bool  `json:"retryable"`
	RetryAfter *int   `json:"retry_after"`
	RequestID  string `json:"request_id"`
}

// Statuses a response without an envelope may carry transiently (load balancer, proxy, timeouts).
var transientStatuses = map[int]bool{408: true, 425: true, 429: true, 500: true, 502: true, 503: true, 504: true}

// apiErrorFrom builds the error for an error status from its envelope (or a synthesized one).
func apiErrorFrom(httpStatus int, detail errorDetail, raw []byte, synthetic bool, retryAfterHeader *int) *Error {
	code := detail.Code
	if code == "" {
		code = "internal"
	}
	message := detail.Message
	if message == "" {
		message = fmt.Sprintf("request failed with HTTP %d (%s)", httpStatus, code)
	}
	retryable := false
	switch {
	case synthetic:
		retryable = transientStatuses[httpStatus]
	case detail.Retryable != nil:
		retryable = *detail.Retryable
	default:
		retryable = httpStatus == 429 || httpStatus == 503
	}
	retryAfter := detail.RetryAfter
	if retryAfter == nil {
		retryAfter = retryAfterHeader
	}
	if retryAfter != nil {
		clamped := clampRetryAfter(int64(*retryAfter))
		retryAfter = &clamped
	}
	return &Error{
		Kind:       kindFor(httpStatus, code),
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Retryable:  retryable,
		RetryAfter: retryAfter,
		RequestID:  detail.RequestID,
		Field:      detail.Field,
		Synthetic:  synthetic,
		raw:        raw,
	}
}

func kindFor(httpStatus int, code string) Kind {
	if code == CodeIdempotencyKeyReused {
		return KindIdempotencyConflict
	}
	switch httpStatus {
	case 400:
		return KindValidation
	case 401:
		return KindAuthentication
	case 403:
		return KindPermission
	case 404:
		return KindNotFound
	case 409:
		return KindConflict
	case 429:
		return KindRateLimit
	case 503:
		return KindUnavailable
	}
	if httpStatus >= 500 {
		return KindInternal
	}
	return KindAPI
}
