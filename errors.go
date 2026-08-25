package oblodai

import (
	"encoding/json"
	"errors"
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
	// KindPermission is HTTP 403: the key is valid but may not do this (wrong key kind, feature off).
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

	CodeTransportTimeout  = "transport.timeout"
	CodeTransportNetwork  = "transport.network"
	CodeTransportAborted  = "transport.aborted"
	CodeTransportDeadline = "transport.deadline"

	CodeWebhookBadSignature   = "webhook.bad_signature"
	CodeWebhookStaleTimestamp = "webhook.stale_timestamp"
	CodeWebhookMissingHeader  = "webhook.missing_header"

	// CodeIdempotencyKeyReused is the core's 409 for a key replayed with a different body.
	CodeIdempotencyKeyReused = "idempotency.key_reused"
	// CodeWrongKeyKind is the core's 403 for a payout route signed with the payment key.
	CodeWrongKeyKind = "merchant.wrong_key_kind"
	// CodeBadSignature and CodeBadTimestamp are the two 401s that can mean clock skew.
	CodeBadSignature = "merchant.bad_signature"
	CodeBadTimestamp = "auth.bad_timestamp"
)

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

	// raw is the response body. It is deliberately unexported and dropped from MarshalJSON so a
	// structured log of the error cannot leak a secret the body carried.
	raw []byte
	// cause is the underlying error, reachable with errors.Unwrap.
	cause error
}

// Error implements the error interface.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("oblodai: ")
	b.WriteString(e.Code)
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.HTTPStatus != 0 {
		fmt.Fprintf(&b, " (HTTP %d", e.HTTPStatus)
		if e.RequestID != "" {
			fmt.Fprintf(&b, ", request %s", e.RequestID)
		}
		b.WriteString(")")
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
	}{e.Kind, e.Code, e.Message, e.HTTPStatus, e.Retryable, e.RetryAfter, e.RequestID, e.Field, e.Synthetic})
}

// AsError recovers the *Error from an error chain.
func AsError(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

// IsKind reports whether err is an *Error of that kind. KindConflict also matches an idempotency
// conflict, which is a conflict with a specific code.
func IsKind(err error, kind Kind) bool {
	e, ok := AsError(err)
	if !ok {
		return false
	}
	if kind == KindConflict && e.Kind == KindIdempotencyConflict {
		return true
	}
	return e.Kind == kind
}

// IsCode reports whether err is an *Error carrying that code.
func IsCode(err error, code string) bool {
	e, ok := AsError(err)
	return ok && e.Code == code
}

// IsRetryable reports whether the core (or the transport classification) considers the failure
// worth repeating. The client has already retried what it could; this is for your own outer loop.
func IsRetryable(err error) bool {
	e, ok := AsError(err)
	return ok && e.Retryable
}

// IsValidation reports an HTTP 400.
func IsValidation(err error) bool { return IsKind(err, KindValidation) }

// IsAuthentication reports an HTTP 401.
func IsAuthentication(err error) bool { return IsKind(err, KindAuthentication) }

// IsPermission reports an HTTP 403.
func IsPermission(err error) bool { return IsKind(err, KindPermission) }

// IsNotFound reports an HTTP 404.
func IsNotFound(err error) bool { return IsKind(err, KindNotFound) }

// IsConflict reports an HTTP 409, including an idempotency conflict.
func IsConflict(err error) bool { return IsKind(err, KindConflict) }

// IsIdempotencyConflict reports the 409 idempotency.key_reused specifically: the same key was
// replayed with a different body.
func IsIdempotencyConflict(err error) bool { return IsKind(err, KindIdempotencyConflict) }

// IsRateLimit reports an HTTP 429.
func IsRateLimit(err error) bool { return IsKind(err, KindRateLimit) }

// IsUnavailable reports an HTTP 503.
func IsUnavailable(err error) bool { return IsKind(err, KindUnavailable) }

// IsInternal reports a 5xx other than 503.
func IsInternal(err error) bool { return IsKind(err, KindInternal) }

// IsTransport reports a failure with no HTTP response: DNS, TCP, TLS, timeout, cancellation.
func IsTransport(err error) bool { return IsKind(err, KindTransport) }

// IsConfig reports a refusal raised before anything was sent.
func IsConfig(err error) bool { return IsKind(err, KindConfig) }

// IsContract reports a response that could not be read as the documented envelope.
func IsContract(err error) bool { return IsKind(err, KindContract) }

// IsSignature reports a failed webhook verification.
func IsSignature(err error) bool { return IsKind(err, KindSignature) }

// newConfigError is raised before any request leaves the process.
func newConfigError(code, message, field string) *Error {
	return &Error{Kind: KindConfig, Code: code, Message: message, Field: field}
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
