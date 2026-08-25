package oblodai

import "context"

// PayoutsService sends funds to external addresses. Every route here wants the payout key.
type PayoutsService struct{ c *Client }

// Create sends a payout and, for API keys, approves it immediately (POST /v1/payout). It is
// idempotent by OrderID and by the Idempotency-Key this client attaches.
//
// Errors worth handling: payout.insufficient_funds (retryable — the balance may arrive),
// payout.funds_maturing (retryable — deposits are still in their reorg window),
// payout.bad_address, payout.memo_required.
func (s *PayoutsService) Create(ctx context.Context, params PayoutParams, opts ...RequestOption) (*Payout, error) {
	return post[Payout](ctx, s.c, "POST /v1/payout", params, opts)
}

// Validate is a dry run of Create (POST /v1/payout/validate): every check, nothing reserved and
// nothing sent. It raises the same errors Create would.
func (s *PayoutsService) Validate(ctx context.Context, params PayoutValidateParams, opts ...RequestOption) (*PayoutValidation, error) {
	return post[PayoutValidation](ctx, s.c, "POST /v1/payout/validate", params, opts)
}

// Calculate prices a payout without creating anything (POST /v1/payout/calculate).
func (s *PayoutsService) Calculate(ctx context.Context, params PayoutCalculateParams, opts ...RequestOption) (*PayoutCalculation, error) {
	return post[PayoutCalculation](ctx, s.c, "POST /v1/payout/calculate", params, opts)
}

// Info fetches a payout by UUID or by your OrderID (POST /v1/payout/info). Refunds are payouts
// too — they carry IsRefund and RefundFor.
func (s *PayoutsService) Info(ctx context.Context, params PayoutInfoParams, opts ...RequestOption) (*Payout, error) {
	return post[Payout](ctx, s.c, "POST /v1/payout/info", params, opts)
}

// Get is an alias of Info.
func (s *PayoutsService) Get(ctx context.Context, params PayoutInfoParams, opts ...RequestOption) (*Payout, error) {
	return s.Info(ctx, params, opts...)
}

// Cancel stops a payout that has not been broadcast yet — pending, approved or awaiting_cosign
// (POST /v1/payout/cancel). Afterwards the core answers 409 payout.not_pending.
func (s *PayoutsService) Cancel(ctx context.Context, uuid string, opts ...RequestOption) (*Payout, error) {
	return post[Payout](ctx, s.c, "POST /v1/payout/cancel", PayoutCancelParams{UUID: uuid}, opts)
}

// Approve releases a payout that is waiting for manual approval (POST /v1/payout/approve).
func (s *PayoutsService) Approve(ctx context.Context, uuid string, opts ...RequestOption) (*Payout, error) {
	return post[Payout](ctx, s.c, "POST /v1/payout/approve", PayoutApproveParams{UUID: uuid}, opts)
}

// History lists payouts, newest first (POST /v1/payout/history). Kind "refund" lists refunds only.
func (s *PayoutsService) History(ctx context.Context, params PayoutHistoryParams, opts ...RequestOption) *List[Payout] {
	return listOf[Payout](ctx, s.c, "POST /v1/payout/history", params, opts)
}

// List is an alias of History.
func (s *PayoutsService) List(ctx context.Context, params PayoutHistoryParams, opts ...RequestOption) *List[Payout] {
	return s.History(ctx, params, opts...)
}

// Mass sends up to 100 payouts in ONE synchronous call (POST /v1/payout/mass): each element
// reports its own outcome in the response, aligned with the request by Idx.
func (s *PayoutsService) Mass(ctx context.Context, params PayoutMassParams, opts ...RequestOption) ([]BatchElement[Payout], error) {
	return items[BatchElement[Payout]](ctx, s.c, "POST /v1/payout/mass", params, opts)
}

// Batch queues up to 5000 payouts asynchronously (POST /v1/payout/batch) and returns a ticket to
// poll with Batches.Info. OrderID is required on every element.
func (s *PayoutsService) Batch(ctx context.Context, params PayoutBatchParams, opts ...RequestOption) (*BatchSubmitted, error) {
	return post[BatchSubmitted](ctx, s.c, "POST /v1/payout/batch", params, opts)
}

// Services lists the currencies and networks available for payouts (POST /v1/payout/services).
func (s *PayoutsService) Services(ctx context.Context, params PayoutServicesParams, opts ...RequestOption) *List[ServiceMethod] {
	return listOf[ServiceMethod](ctx, s.c, "POST /v1/payout/services", params, opts)
}

// GetFeeConfig reads who bears the network fee on payouts by default
// (POST /v1/payout/fee-config/get).
func (s *PayoutsService) GetFeeConfig(ctx context.Context, opts ...RequestOption) (*PayoutFeeConfig, error) {
	return post[PayoutFeeConfig](ctx, s.c, "POST /v1/payout/fee-config/get", nil, opts)
}

// SetFeeConfig sets who bears the network fee on payouts (POST /v1/payout/fee-config/set).
func (s *PayoutsService) SetFeeConfig(ctx context.Context, params PayoutFeeConfigSetParams, opts ...RequestOption) (*PayoutFeeConfig, error) {
	return post[PayoutFeeConfig](ctx, s.c, "POST /v1/payout/fee-config/set", params, opts)
}

// GetRefundFeeConfig reads who bears the fee on refunds
// (POST /v1/payout/refund-fee-config/get).
func (s *PayoutsService) GetRefundFeeConfig(ctx context.Context, opts ...RequestOption) (*RefundFeeConfig, error) {
	return post[RefundFeeConfig](ctx, s.c, "POST /v1/payout/refund-fee-config/get", nil, opts)
}

// SetRefundFeeConfig sets who bears the fee on refunds (POST /v1/payout/refund-fee-config/set).
func (s *PayoutsService) SetRefundFeeConfig(ctx context.Context, params PayoutRefundFeeConfigSetParams, opts ...RequestOption) (*RefundFeeConfig, error) {
	return post[RefundFeeConfig](ctx, s.c, "POST /v1/payout/refund-fee-config/set", params, opts)
}

// RefundsService issues refunds — payouts in the invoice's own asset — and settles underpayments.
// It wants the payout key.
type RefundsService struct{ c *Client }

// Create refunds a paid invoice, fully or partially (POST /v1/payment/refund). The result is the
// refund payout; follow it with Payouts.Info.
func (s *RefundsService) Create(ctx context.Context, params PaymentRefundParams, opts ...RequestOption) (*Payout, error) {
	return post[Payout](ctx, s.c, "POST /v1/payment/refund", params, opts)
}

// Resolve settles an underpaid (wrong_amount) invoice (POST /v1/payment/resolve): accept the
// short payment as full settlement, or send it back.
func (s *RefundsService) Resolve(ctx context.Context, params PaymentResolveParams, opts ...RequestOption) (*Resolution, error) {
	return post[Resolution](ctx, s.c, "POST /v1/payment/resolve", params, opts)
}

// Batch queues up to 5000 refunds asynchronously (POST /v1/refund/batch); poll Batches.Info.
func (s *RefundsService) Batch(ctx context.Context, params RefundBatchParams, opts ...RequestOption) (*BatchSubmitted, error) {
	return post[BatchSubmitted](ctx, s.c, "POST /v1/refund/batch", params, opts)
}

// Resolution is the answer to POST /v1/payment/resolve, in both of its shapes.
//
// With action "accept" the underpayment is kept as full settlement and PaymentUUID, Currency and
// AmountKept describe it. With action "refund" the money goes back and the embedded Payout is the
// refund that was created. Resolution says which happened: "accepted" or "refunded".
type Resolution struct {
	// Resolution is "accepted" or "refunded".
	Resolution string `json:"resolution"`
	// PaymentUUID is the invoice that was settled; set when the underpayment was accepted.
	PaymentUUID string `json:"payment_uuid,omitempty"`
	// AmountKept is what the merchant kept; set when the underpayment was accepted.
	AmountKept Money `json:"amount_kept,omitempty"`
	// Payout is the refund that was created; set when the underpayment was refunded.
	Payout
}
