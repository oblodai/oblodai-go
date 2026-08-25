package oblodai

// BatchElement is one element of a batch listing: /v1/payout/mass, /v1/payout/link/batch and the
// items of /v1/batch/info. Either Result or the failure fields are set, never both.
type BatchElement[T any] struct {
	// Idx is the element's position in the array that was submitted.
	Idx int `json:"idx"`
	// OK reports whether this element was accepted.
	OK bool `json:"ok"`
	// OrderID echoes the element's merchant reference, when it carried one.
	OrderID string `json:"order_id,omitempty"`
	// Result is the object the element produced; set when OK.
	Result *T `json:"result,omitempty"`
	// Message is the human-readable failure reason.
	Message string `json:"message,omitempty"`
	// ErrorCode is the stable failure code, the same family.reason string an *Error carries.
	ErrorCode string `json:"error_code,omitempty"`
	// HTTPStatus is the status the element would have got as a standalone call.
	HTTPStatus int `json:"http_status,omitempty"`
}

// FeeInfo is how a fee was settled on a priced result. FeeType is the pricing mode
// (percent, fixed and so on).
type FeeInfo struct {
	Commission Money           `json:"commission"`
	FeeBearer  FeeBearerResult `json:"fee_bearer"`
	FeeType    string          `json:"fee_type"`
}

// BatchKind is the kind of an asynchronous batch: payment, payout, refund, transfer or
// payout_link.
type BatchKind string

// BatchStatus is where an asynchronous batch stands in its lifecycle: queued, processing, done or
// stopped.
type BatchStatus string

// OkResult is the body of routes that only acknowledge: /v1/payment/resend,
// /v1/payment/accepted/set and /v1/split/rule/delete.
type OkResult struct {
	OK bool `json:"ok"`
}
