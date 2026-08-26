package oblodai

import "context"

// PaymentsService covers invoices: create, look up, cancel, list, plus the payer-facing checkout
// endpoints. Signed with the API key; the payer-facing calls need no credentials at all.
type PaymentsService struct{ c *Client }

// Create opens an invoice (POST /v1/payment). It is idempotent twice over: by your OrderID, and
// by the Idempotency-Key this client generates and reuses across retries.
//
// Errors worth branching on: payment.bad_amount, payment.below_minimum,
// payment.minimum_unavailable (the rate feed is down — retryable), payment.unsupported_network,
// payment.network_required (a multi-network asset with no Network), request.unknown_currency,
// idempotency.key_reused (the same key with a different body).
func (s *PaymentsService) Create(ctx context.Context, params PaymentParams, opts ...RequestOption) (*Payment, error) {
	return post[Payment](ctx, s.c, "POST /v1/payment", params, opts)
}

// Info fetches an invoice by UUID or by your OrderID (POST /v1/payment/info). The result includes
// Refunds and RefundStatus, which the other invoice routes leave out.
func (s *PaymentsService) Info(ctx context.Context, params PaymentInfoParams, opts ...RequestOption) (*Payment, error) {
	return post[Payment](ctx, s.c, "POST /v1/payment/info", params, opts)
}

// Get is an alias of Info.
func (s *PaymentsService) Get(ctx context.Context, params PaymentInfoParams, opts ...RequestOption) (*Payment, error) {
	return s.Info(ctx, params, opts...)
}

// Cancel cancels an unpaid invoice (POST /v1/payment/cancel). Once a deposit has been seen the
// core refuses with 409 invoice.not_payable.
func (s *PaymentsService) Cancel(ctx context.Context, params PaymentCancelParams, opts ...RequestOption) (*Payment, error) {
	return post[Payment](ctx, s.c, "POST /v1/payment/cancel", params, opts)
}

// History lists invoices, newest first (POST /v1/payment/history).
func (s *PaymentsService) History(ctx context.Context, params PaymentHistoryParams, opts ...RequestOption) *List[Payment] {
	return listOf[Payment](ctx, s.c, "POST /v1/payment/history", params, opts)
}

// List is an alias of History.
func (s *PaymentsService) List(ctx context.Context, params PaymentHistoryParams, opts ...RequestOption) *List[Payment] {
	return s.History(ctx, params, opts...)
}

// Batch creates up to 5000 invoices asynchronously (POST /v1/payment/batch). Track it with
// Batches.Info.
//
// Errors worth branching on: payment.bad_amount, payment.below_minimum,
// request.unknown_currency, request.missing_field (an item without OrderID),
// payout.batch_too_large, idempotency.key_reused.
func (s *PaymentsService) Batch(ctx context.Context, params PaymentBatchParams, opts ...RequestOption) (*BatchSubmitted, error) {
	return post[BatchSubmitted](ctx, s.c, "POST /v1/payment/batch", params, opts)
}

// QR renders the invoice's payment URI as a QR image (POST /v1/payment/qr).
func (s *PaymentsService) QR(ctx context.Context, params PaymentQRParams, opts ...RequestOption) (*QRCode, error) {
	return post[QRCode](ctx, s.c, "POST /v1/payment/qr", params, opts)
}

// Services lists the currency and network pairs deposits are accepted in, with limits and fees
// (POST /v1/payment/services).
func (s *PaymentsService) Services(ctx context.Context, params PaymentServicesParams, opts ...RequestOption) *List[ServiceMethod] {
	return listOf[ServiceMethod](ctx, s.c, "POST /v1/payment/services", params, opts)
}

// SendEmail emails the invoice receipt (POST /v1/payment/send-email). Without an explicit address
// it goes to the invoice's payer_email.
func (s *PaymentsService) SendEmail(ctx context.Context, params PaymentSendEmailParams, opts ...RequestOption) (*EmailSent, error) {
	return post[EmailSent](ctx, s.c, "POST /v1/payment/send-email", params, opts)
}

// Resend re-delivers the invoice's last webhook (POST /v1/payment/resend).
func (s *PaymentsService) Resend(ctx context.Context, params PaymentResendParams, opts ...RequestOption) (*OkResult, error) {
	return post[OkResult](ctx, s.c, "POST /v1/payment/resend", params, opts)
}

// PublicView returns the invoice as the payer sees it (GET /v1/pay/{id}). No credentials needed:
// use it to build your own checkout page.
func (s *PaymentsService) PublicView(ctx context.Context, uuid string, opts ...RequestOption) (*PublicPayment, error) {
	return get[PublicPayment](ctx, s.c, "GET /v1/pay/{id}", nil, map[string]string{"id": uuid}, opts)
}

// Select picks the asset and network on a multi-currency invoice (POST /v1/pay/{id}/select). No
// credentials needed.
func (s *PaymentsService) Select(ctx context.Context, uuid string, params PaySelectParams, opts ...RequestOption) (*PublicPayment, error) {
	return postPath[PublicPayment](ctx, s.c, "POST /v1/pay/{id}/select", map[string]string{"id": uuid}, params, opts)
}

// PublicQR renders the payer page's QR (GET /v1/pay/{id}/qr). No credentials needed.
func (s *PaymentsService) PublicQR(ctx context.Context, uuid string, opts ...RequestOption) (*QRCode, error) {
	return get[QRCode](ctx, s.c, "GET /v1/pay/{id}/qr", nil, map[string]string{"id": uuid}, opts)
}
