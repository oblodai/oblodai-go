package oblodai

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Response envelopes, as httpx/apiutil on the core write them:
//
//	success : {"state": 0, "result": <payload>}
//	list    : result = {"items": [...], "paginate": {total, per_page, offset, has_pages}}
//	error   : {"error": {code, message, field?, retryable, retry_after?, request_id?}}
//
// Every non-bare route uses these; bare routes (PDF and CSV documents) bypass this file.

type envelope struct {
	State  *int            `json:"state"`
	Result json.RawMessage `json:"result"`
	// Error stays raw: whether the key is present decides synthetic-or-not, and each of its fields
	// is read on its own so one field of the wrong type cannot discard the whole envelope.
	Error json.RawMessage `json:"error"`
}

// decodeContext carries the response headers that change how a body is read.
type decodeContext struct {
	retryAfter string
	location   string
	now        time.Time
}

// decodeEnvelope interprets a response body. It returns the raw result on success, or the *Error
// the body describes. body is kept as evidence on non-JSON failures.
func decodeEnvelope(httpStatus int, body []byte, ctx decodeContext) (json.RawMessage, *Error) {
	retryAfterHeader := parseRetryAfter(ctx.retryAfter, ctx.now)

	if httpStatus >= 300 && httpStatus < 400 {
		where := ""
		if ctx.location != "" {
			where = " to " + ctx.location
		}
		detail := errorDetail{
			Code:    "internal",
			Message: fmt.Sprintf("unexpected redirect (HTTP %d)%s; check the base URL", httpStatus, where),
		}
		return nil, apiErrorFrom(httpStatus, detail, body, true, retryAfterHeader)
	}

	var env envelope
	if len(body) > 0 {
		if err := json.Unmarshal(body, &env); err != nil {
			if httpStatus >= 400 {
				detail := errorDetail{Code: "internal", Message: noEnvelope(httpStatus, body)}
				return nil, apiErrorFrom(httpStatus, detail, body, true, retryAfterHeader)
			}
			return nil, newContractError("expected a JSON envelope, got "+excerpt(body), httpStatus, body)
		}
	}

	if env.Error != nil {
		detail, usable := decodeErrorDetail(env.Error, httpStatus)
		if usable {
			return nil, apiErrorFrom(httpStatus, detail, body, false, retryAfterHeader)
		}
		// The key is there but carries no usable code: something answered in the core's shape
		// without being the core. Keep the request id if it was a string, and report it as the
		// synthetic error the HTTP status describes.
		detail.Code = "internal"
		detail.Message = noEnvelope(httpStatus, body)
		detail.Retryable, detail.RetryAfter, detail.Field = nil, nil, ""
		return nil, apiErrorFrom(httpStatus, detail, body, true, retryAfterHeader)
	}
	if httpStatus >= 400 {
		detail := errorDetail{Code: "internal", Message: noEnvelope(httpStatus, body)}
		return nil, apiErrorFrom(httpStatus, detail, body, true, retryAfterHeader)
	}
	if env.State != nil && *env.State == 0 && env.Result != nil {
		return env.Result, nil
	}
	return nil, newContractError("response is not a {state:0,result} envelope: "+excerpt(body), httpStatus, body)
}

// decodeErrorDetail reads the error object one field at a time. A field of the wrong type is
// dropped, never fatal: an error answer must still reach the caller as the failure it describes.
// The second result is false when there is no usable code, which makes the answer synthetic.
func decodeErrorDetail(raw json.RawMessage, httpStatus int) (errorDetail, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return errorDetail{}, false
	}
	detail := errorDetail{
		Code:      stringField(fields["code"]),
		Message:   stringField(fields["message"]),
		Field:     stringField(fields["field"]),
		RequestID: stringField(fields["request_id"]),
	}
	// Only a literal true/false is the core speaking; anything else leaves the decision to the
	// status, where a wrong guess is at worst a missed retry rather than a repeated payout.
	var retryable bool
	if err := json.Unmarshal(fields["retryable"], &retryable); len(fields["retryable"]) > 0 && err == nil {
		detail.Retryable = &retryable
	}
	if seconds, ok := retryAfterSeconds(fields["retry_after"]); ok {
		detail.RetryAfter = &seconds
	}
	if detail.Code == "" {
		return detail, false
	}
	if detail.Message == "" {
		detail.Message = fmt.Sprintf("HTTP %d", httpStatus)
	}
	return detail, true
}

// stringField returns a JSON string field, or "" when it is absent or not a string.
func stringField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

// retryAfterSeconds reads retry_after as an integer, a float or a numeric string, clamped to
// [0, MaxRetryAfterSeconds]. Anything else means the core said nothing usable.
func retryAfterSeconds(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return fromFloatSeconds(number)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, false
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0, false
	}
	return fromFloatSeconds(number)
}

// fromFloatSeconds rounds a numeric hint up to whole seconds without letting an infinity or a
// value beyond int64 wrap around.
func fromFloatSeconds(number float64) (int, bool) {
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	if number > MaxRetryAfterSeconds {
		return MaxRetryAfterSeconds, true
	}
	if number < 0 {
		return 0, true
	}
	return clampRetryAfter(int64(math.Ceil(number))), true
}

// parseRetryAfter reads a Retry-After header as delta-seconds or an HTTP date. It returns nil when
// the header is absent or unparsable, and never a negative wait.
func parseRetryAfter(value string, now time.Time) *int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		clamped := clampRetryAfter(seconds)
		return &clamped
	}
	at, err := http1123(value)
	if err != nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	// Unix seconds, not a time.Duration: a date centuries away would overflow a Duration and come
	// back as a negative wait.
	clamped := clampRetryAfter(at.Unix() - now.Unix())
	return &clamped
}

// http1123 parses the date formats an HTTP header may legally carry.
func http1123(value string) (time.Time, error) {
	var lastErr error
	for _, layout := range []string{time.RFC1123, time.RFC1123Z, time.RFC850, time.ANSIC} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

func noEnvelope(status int, body []byte) string {
	return fmt.Sprintf("HTTP %d without an Oblodai error envelope (%s) — the answer came from a proxy or load balancer, not the API",
		status, excerpt(body))
}

// excerpt renders a short, single-line excerpt of a body for an error message.
func excerpt(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if text == "" {
		return "<empty body>"
	}
	if runes := []rune(text); len(runes) > 120 {
		return string(runes[:120]) + "…"
	}
	return text
}
