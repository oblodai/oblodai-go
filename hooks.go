package oblodai

import (
	"net/http"
	"strings"
	"time"
)

// Hooks observe every attempt the client makes — for metrics, tracing and structured logs:
//
//	client, err := oblodai.New(oblodai.WithHooks(oblodai.Hooks{
//		OnResponse: func(r oblodai.ResponseInfo) {
//			metrics.Observe(r.Request.OperationID, r.StatusCode, r.Elapsed)
//		},
//	}))
//
// They run synchronously on the calling goroutine, once per attempt (a retried call reports each
// attempt), so keep them cheap. The headers they see have the signature and the admin token
// redacted.
type Hooks struct {
	// OnRequest runs just before an attempt is sent.
	OnRequest func(RequestInfo)
	// OnResponse runs when an attempt ended: with a response, or with StatusCode 0 and Err set.
	OnResponse func(ResponseInfo)
}

// RequestInfo describes one attempt about to be sent.
type RequestInfo struct {
	// OperationID is the route's OpenAPI operationId.
	OperationID string
	Method      string
	URL         string
	// Header is the request headers as sent, with the signature and the admin token redacted.
	Header http.Header
	// Attempt is 1 for the first attempt, 2 for the first retry, and so on.
	Attempt int
	// RequestID is the call's X-Request-ID; the same on every attempt.
	RequestID string
}

// ResponseInfo describes how one attempt ended.
type ResponseInfo struct {
	Request RequestInfo
	// StatusCode is the HTTP status, or 0 when the attempt produced no response.
	StatusCode int
	Header     http.Header
	// Elapsed is the time from sending the attempt to this point.
	Elapsed time.Duration
	// Err is the error the attempt ended with (an error status or a transport failure), else nil.
	Err error
}

// hookHeaders copies the headers of an outgoing request for a hook, with secrets redacted.
func hookHeaders(headers map[string]string) http.Header {
	out := http.Header{}
	for name, value := range headers {
		switch strings.ToLower(name) {
		case strings.ToLower(HeaderSignature), strings.ToLower(HeaderAdminToken):
			value = redactedPlaceholder
		}
		out.Set(name, value)
	}
	return out
}
