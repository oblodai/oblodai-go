package oblodai

import (
	"encoding/json"
	"fmt"
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
	Error  *errorDetail    `json:"error"`
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
			return nil, newContractError("expected a JSON envelope, got "+describe(body), httpStatus, body)
		}
	}

	if env.Error != nil {
		return nil, apiErrorFrom(httpStatus, *env.Error, body, false, retryAfterHeader)
	}
	if httpStatus >= 400 {
		detail := errorDetail{Code: "internal", Message: noEnvelope(httpStatus, body)}
		return nil, apiErrorFrom(httpStatus, detail, body, true, retryAfterHeader)
	}
	if env.State != nil && *env.State == 0 && env.Result != nil {
		return env.Result, nil
	}
	return nil, newContractError("response is not a {state:0,result} envelope: "+describe(body), httpStatus, body)
}

// parseRetryAfter reads a Retry-After header as delta-seconds or an HTTP date. It returns nil when
// the header is absent or unparsable, and never a negative wait.
func parseRetryAfter(value string, now time.Time) *int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		return &seconds
	}
	at, err := http1123(value)
	if err != nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	seconds := int((at.Sub(now) + time.Second - 1) / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	return &seconds
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
		status, describe(body))
}

// describe renders a short, single-line excerpt of a body for an error message.
func describe(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if text == "" {
		return "<empty body>"
	}
	if runes := []rune(text); len(runes) > 120 {
		return string(runes[:120]) + "…"
	}
	return text
}
