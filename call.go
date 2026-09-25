package oblodai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// The seam between the generated services and the runtime. A generated method describes its
// request as a Call and hands it to doJSON, doPaged or doFile with the caller's options; those
// run it through the Requester (the client's transport) and decode the answer.

// Call is one request as a generated method describes it.
type Call struct {
	// Route is the operation, from Routes.
	Route RouteSpec
	// PathParams fills the {name} segments of Route.Path.
	PathParams map[string]string
	// Query is the query string (GET routes).
	Query url.Values
	// Body is the JSON body (POST routes); nil goes out as {}.
	Body any
	// IdempotencyKeyInBody: the route's body has its own idempotency_key field, so the
	// WithIdempotencyKey option fills that field and no HeaderIdempotencyKey is sent (the route
	// is not deduplicated by the header).
	IdempotencyKeyInBody bool
}

// Requester sends a Call: sign, send, retry, classify. The client's transport is the only
// implementation; the generated services hold it.
type Requester interface {
	request(ctx context.Context, call Call, o callOptions) (*rawResponse, *Error)
}

// doJSON performs an envelope route and decodes its result into T.
func doJSON[T any](ctx context.Context, r Requester, call Call, opts []RequestOption) (*T, error) {
	raw, err := r.request(ctx, call, applyRequestOptions(opts))
	if err != nil {
		return nil, err
	}
	out, decodeErr := decodeResult[T](call.Route, raw)
	if decodeErr != nil {
		return nil, decodeErr
	}
	return out, nil
}

// doFile performs a bare route and returns its bytes.
func doFile(ctx context.Context, r Requester, call Call, opts []RequestOption) (*FileResult, error) {
	raw, err := r.request(ctx, call, applyRequestOptions(opts))
	if err != nil {
		return nil, err
	}
	contentType := raw.contentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &FileResult{Bytes: raw.body, ContentType: contentType, Filename: filenameFrom(raw.header.Get("Content-Disposition"))}, nil
}

// doPaged builds the lazy list a paged route returns; nothing is requested until it is read.
func doPaged[T any](ctx context.Context, r Requester, call Call, opts []RequestOption) *List[T] {
	return newList[T](ctx, r, call, applyRequestOptions(opts))
}

// decodeResult reads the success envelope of an answer into T.
func decodeResult[T any](route RouteSpec, raw *rawResponse) (*T, *Error) {
	result, err := decodeEnvelope(raw.status, raw.body, decodeContext{})
	if err != nil {
		return nil, err
	}
	// The core replays a cached response by HeaderIdempotencyKey; when the original was too large to
	// cache it answers {ok, idempotent_replay: true, detail} instead of the object — surface that
	// rather than handing back a half-empty struct.
	var replay struct {
		IdempotentReplay bool   `json:"idempotent_replay"`
		Detail           string `json:"detail"`
	}
	if json.Unmarshal(result, &replay) == nil && replay.IdempotentReplay {
		return nil, newContractError(fmt.Sprintf(
			"%s: the request was already processed but its response was too large to replay — fetch the result by order_id or reference (%s)",
			route.OperationID, replay.Detail), raw.status, result)
	}
	out := new(T)
	if err := json.Unmarshal(result, out); err != nil {
		return nil, newContractError(fmt.Sprintf("%s: the result does not match the documented shape: %v", route.OperationID, err), raw.status, result)
	}
	return out, nil
}
