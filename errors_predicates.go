package oblodai

import "errors"

// The ergonomic half of the error surface: recovering an *Error from a chain and asking what kind
// of failure it is. Branch on Code when the exact reason matters; these predicates are for the
// coarse decisions (retry the call, ask for credentials, answer the webhook sender).

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

// IsWebhookPayload reports an authentic delivery whose body could not be read (webhook.bad_payload).
// Answer 5xx to it — the signature was valid, so the event is real and the core will retry it.
func IsWebhookPayload(err error) bool { return IsCode(err, CodeWebhookBadPayload) }
