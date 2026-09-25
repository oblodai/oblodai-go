package oblodai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Invoking an operation by its operationId — for tools that are driven by the contract rather than
// by typed models (the oblodai CLI, generic proxies). The request goes through the same transport
// as a typed method: signing, idempotency, retries, error classification.

// Error codes of Invoke, InvokeList and InvokeFile.
const (
	// CodeUnknownOperation: no route in this SDK release carries that operationId.
	CodeUnknownOperation = "sdk.unknown_operation"
	// CodeWrongInvoke: the operationId names a route of a different kind (bare, paged, or plain
	// envelope) than the Invoke* method called.
	CodeWrongInvoke = "sdk.wrong_invoke"
)

// invokeSuffix names the right Invoke* method for a route kind, for the CodeWrongInvoke message.
var invokeSuffix = map[string]string{"json": "", "list": "List", "file": "File"}

// InvokeInput is the request of an Invoke, InvokeList or InvokeFile call.
type InvokeInput struct {
	// PathParams fills the {name} segments of the route's path.
	PathParams map[string]string
	// Query is the query string (GET routes).
	Query url.Values
	// Body is the JSON body (POST routes); nil goes out as {}.
	Body json.RawMessage
}

// route looks up operationID and checks it is the kind the caller means to invoke, before anything
// reaches the network.
func (c *Client) route(operationID string, want string) (RouteSpec, *Error) {
	r, ok := Routes[operationID]
	if !ok {
		return r, newConfigError(CodeUnknownOperation, fmt.Sprintf("no operation %q in this SDK release", operationID), "operationID")
	}
	kind := "json"
	switch {
	case r.Bare:
		kind = "file"
	case r.ListKind == ListPaged:
		kind = "list"
	}
	if kind != want {
		return r, newConfigError(CodeWrongInvoke, fmt.Sprintf(
			"%s is a %s operation; call Invoke%s", operationID, kind, invokeSuffix[kind]), "operationID")
	}
	return r, nil
}

// call turns the input into the Call a generated method would have built.
func (in InvokeInput) call(r RouteSpec) Call {
	var body any
	if in.Body != nil {
		body = in.Body
	}
	return Call{Route: r, PathParams: in.PathParams, Query: in.Query, Body: body, IdempotencyKeyInBody: r.BodyIdempotencyKey}
}

// Invoke performs an envelope operation (neither paged nor bare) and returns its result as raw
// JSON.
func (c *Client) Invoke(ctx context.Context, operationID string, in InvokeInput, opts ...RequestOption) (json.RawMessage, error) {
	r, err := c.route(operationID, "json")
	if err != nil {
		return nil, err
	}
	out, doErr := doJSON[json.RawMessage](ctx, c.transport, in.call(r), opts)
	if doErr != nil {
		return nil, doErr
	}
	return *out, nil
}

// InvokeList performs a paged operation lazily; limit/offset in Query (GET) or Body (POST) set the
// first page, exactly as a generated list method's params would.
func (c *Client) InvokeList(ctx context.Context, operationID string, in InvokeInput, opts ...RequestOption) *List[json.RawMessage] {
	r, err := c.route(operationID, "list")
	if err != nil {
		return failedList[json.RawMessage](ctx, err)
	}
	return doPaged[json.RawMessage](ctx, c.transport, in.call(r), opts)
}

// InvokeFile performs a bare operation (a generated PDF or CSV document).
func (c *Client) InvokeFile(ctx context.Context, operationID string, in InvokeInput, opts ...RequestOption) (*FileResult, error) {
	r, err := c.route(operationID, "file")
	if err != nil {
		return nil, err
	}
	return doFile(ctx, c.transport, in.call(r), opts)
}
