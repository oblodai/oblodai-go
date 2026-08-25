package oblodai

import (
	"math/rand"
	"time"
)

// Retry policy. Two questions decide every retry:
//
//  1. Can it succeed? — the core's Retryable flag (authoritative when the core wrote the
//     envelope), or a transient status for answers that carry no envelope.
//  2. Is repeating safe? — only for read-only routes and for writes the core deduplicates by
//     Idempotency-Key. A write the core does not deduplicate is never re-sent once it MAY have
//     reached the core: a transport error or a proxy 503 after the request left the socket could
//     mean the payout already happened.
//
// An envelope error on an unsafe write is still retried when Retryable — the core answered, so it
// did not perform the operation (429, 503, frozen and maturing all fail before any effect).
// Retry-After always wins over the computed backoff; otherwise exponential backoff with jitter.

// RetryOptions tunes the retry loop. The zero value is not usable; start from DefaultRetry.
type RetryOptions struct {
	// MaxRetries is how many times a call may be repeated after the first attempt. Default 2;
	// zero disables retries.
	MaxRetries int
	// BaseDelay is the delay before the first retry. Default 250 ms.
	BaseDelay time.Duration
	// MaxDelay bounds a computed (non Retry-After) delay. Default 4 s.
	MaxDelay time.Duration
	// MaxRetryAfter bounds a server-provided Retry-After. Default 30 s.
	MaxRetryAfter time.Duration
}

// DefaultRetry is the policy a client uses unless WithRetry overrides it.
func DefaultRetry() RetryOptions {
	return RetryOptions{
		MaxRetries:    2,
		BaseDelay:     250 * time.Millisecond,
		MaxDelay:      4 * time.Second,
		MaxRetryAfter: 30 * time.Second,
	}
}

// withDefaults fills unset fields so a partially specified policy still behaves.
func (o RetryOptions) withDefaults() RetryOptions {
	d := DefaultRetry()
	if o.MaxRetries < 0 {
		o.MaxRetries = 0
	}
	if o.BaseDelay <= 0 {
		o.BaseDelay = d.BaseDelay
	}
	if o.MaxDelay <= 0 {
		o.MaxDelay = d.MaxDelay
	}
	if o.MaxRetryAfter <= 0 {
		o.MaxRetryAfter = d.MaxRetryAfter
	}
	return o
}

// shouldRetry decides whether attempt number `attempt` (0 for the first retry decision) may be
// followed by another one. safeToRepeat is route.Safe or (route.Idempotent and a key was sent).
func shouldRetry(err *Error, attempt int, safeToRepeat bool, opts RetryOptions) bool {
	if err == nil || attempt >= opts.MaxRetries || !err.Retryable {
		return false
	}
	if err.Kind == KindTransport {
		return safeToRepeat
	}
	// No core envelope: something in front of the core answered; the core may have done the work.
	if err.Synthetic {
		return safeToRepeat
	}
	return true
}

// retryDelay is how long to wait before the next attempt. random is injectable for tests.
func retryDelay(err *Error, attempt int, opts RetryOptions, random func() float64) time.Duration {
	if err != nil && err.RetryAfter != nil && *err.RetryAfter > 0 {
		wait := time.Duration(*err.RetryAfter) * time.Second
		if wait > opts.MaxRetryAfter {
			wait = opts.MaxRetryAfter
		}
		return wait
	}
	// A Retry-After of 0 means "immediately", which in practice means "as soon as the backoff
	// lets you": falling through here keeps a rate-limited burst from hammering the core.
	exp := opts.BaseDelay << attempt
	if exp > opts.MaxDelay || exp <= 0 {
		exp = opts.MaxDelay
	}
	if random == nil {
		random = rand.Float64
	}
	// Full jitter with a floor, so a burst of retries never lands in the same instant.
	jittered := time.Duration(random() * float64(exp))
	if floor := exp / 4; jittered < floor {
		jittered = floor
	}
	return jittered
}
