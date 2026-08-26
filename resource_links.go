package oblodai

import "context"

// PayoutLinksService mints payout links (cheques): funds are reserved now and claimed later by
// whoever holds the token. The recipient-facing calls need no credentials.
type PayoutLinksService struct{ c *Client }

// Create reserves the funds and mints a claim token (POST /v1/payout/link). ClaimToken and
// ClaimURL come back once, on this call only — store them, they cannot be read again (and they
// are redacted when a PayoutLink is printed or serialized). Idempotent by Reference.
//
// Errors worth branching on: payout_link.disabled, payout.insufficient_funds (retryable),
// payout.funds_maturing (retryable), payout.bad_amount, payout.bad_address,
// payout.reference_collision (that Reference already minted a different link).
func (s *PayoutLinksService) Create(ctx context.Context, params PayoutLinkParams, opts ...RequestOption) (*PayoutLink, error) {
	return post[PayoutLink](ctx, s.c, "POST /v1/payout/link", params, opts)
}

// Info fetches a payout link (POST /v1/payout/link/info).
func (s *PayoutLinksService) Info(ctx context.Context, linkID string, opts ...RequestOption) (*PayoutLink, error) {
	return post[PayoutLink](ctx, s.c, "POST /v1/payout/link/info", PayoutLinkInfoParams{LinkID: linkID}, opts)
}

// Get is an alias of Info.
func (s *PayoutLinksService) Get(ctx context.Context, linkID string, opts ...RequestOption) (*PayoutLink, error) {
	return s.Info(ctx, linkID, opts...)
}

// List lists payout links (POST /v1/payout/link/list).
func (s *PayoutLinksService) List(ctx context.Context, params PayoutLinkListParams, opts ...RequestOption) *List[PayoutLink] {
	return listOf[PayoutLink](ctx, s.c, "POST /v1/payout/link/list", params, opts)
}

// Cancel releases the reserved funds of an unclaimed link (POST /v1/payout/link/cancel).
func (s *PayoutLinksService) Cancel(ctx context.Context, linkID string, opts ...RequestOption) (*PayoutLink, error) {
	return post[PayoutLink](ctx, s.c, "POST /v1/payout/link/cancel", PayoutLinkCancelParams{LinkID: linkID}, opts)
}

// Batch creates up to 500 links in one synchronous call (POST /v1/payout/link/batch): each
// element succeeds or fails on its own and the answer is aligned with the request by Idx, so a
// 200 can still contain failures. Reference is required on every item.
//
// Errors worth branching on (call level): payout.batch_too_large (more than 500),
// payout.empty_batch, payout_link.disabled, payout.insufficient_funds (retryable). Per-element
// failures arrive as ErrorCode, with the vocabulary of Create.
func (s *PayoutLinksService) Batch(ctx context.Context, params PayoutLinkBatchParams, opts ...RequestOption) ([]BatchElement[PayoutLink], error) {
	return items[BatchElement[PayoutLink]](ctx, s.c, "POST /v1/payout/link/batch", params, opts)
}

// Cheque renders a printable PDF cheque for a claim token (POST /v1/payout/link/cheque).
func (s *PayoutLinksService) Cheque(ctx context.Context, params PayoutLinkChequeParams, opts ...RequestOption) (*FileResult, error) {
	return file(ctx, s.c, "POST /v1/payout/link/cheque", nil, nil, params, opts)
}

// ClaimPreview shows what the recipient sees before claiming (GET /v1/claim/{token}). No
// credentials needed.
func (s *PayoutLinksService) ClaimPreview(ctx context.Context, token string, opts ...RequestOption) (*ClaimPreview, error) {
	return get[ClaimPreview](ctx, s.c, "GET /v1/claim/{token}", nil, map[string]string{"token": token}, opts)
}

// Claim pays the link out to an address, with the passcode when the link has one
// (POST /v1/claim/{token}). No credentials needed — this is the recipient's call, not yours.
//
// Errors worth branching on: request.not_found (an unknown or spent token), payout.bad_address,
// payout.address_network_mismatch, payout.memo_required, payout.bad_status (already claimed,
// cancelled or expired), request.rate_limited (too many passcode attempts — the link locks after
// ten).
func (s *PayoutLinksService) Claim(ctx context.Context, token string, params PostClaimParams, opts ...RequestOption) (*ClaimResult, error) {
	return postPath[ClaimResult](ctx, s.c, "POST /v1/claim/{token}", map[string]string{"token": token}, params, opts)
}

// PaymentLinksService manages reusable payment links (tip jars, price tags): every checkout
// spawns an invoice. Signed with the API key; the payer-facing calls need no credentials.
type PaymentLinksService struct{ c *Client }

// Create opens a payment link (POST /v1/payment/link).
//
// Errors worth branching on: invoice.bad_price, payment.bad_amount, request.unknown_currency,
// payment.unsupported_network, payment.below_minimum, idempotency.key_reused.
func (s *PaymentLinksService) Create(ctx context.Context, params PaymentLinkParams, opts ...RequestOption) (*PaymentLinkCreated, error) {
	return post[PaymentLinkCreated](ctx, s.c, "POST /v1/payment/link", params, opts)
}

// Info fetches a link together with a page of the invoices it spawned
// (POST /v1/payment/link/info).
func (s *PaymentLinksService) Info(ctx context.Context, params PaymentLinkInfoParams, opts ...RequestOption) (*PaymentLink, error) {
	return post[PaymentLink](ctx, s.c, "POST /v1/payment/link/info", params, opts)
}

// Get is an alias of Info.
func (s *PaymentLinksService) Get(ctx context.Context, params PaymentLinkInfoParams, opts ...RequestOption) (*PaymentLink, error) {
	return s.Info(ctx, params, opts...)
}

// List lists payment links (POST /v1/payment/link/list).
func (s *PaymentLinksService) List(ctx context.Context, params PaymentLinkListParams, opts ...RequestOption) *List[PaymentLink] {
	return listOf[PaymentLink](ctx, s.c, "POST /v1/payment/link/list", params, opts)
}

// Toggle enables or disables a link (POST /v1/payment/link/toggle).
func (s *PaymentLinksService) Toggle(ctx context.Context, linkID string, active bool, opts ...RequestOption) (*PaymentLinkToggled, error) {
	return post[PaymentLinkToggled](ctx, s.c, "POST /v1/payment/link/toggle",
		PaymentLinkToggleParams{LinkID: linkID, Active: active}, opts)
}

// PublicView returns the link as the payer sees it (GET /v1/link/{id}). No credentials needed.
func (s *PaymentLinksService) PublicView(ctx context.Context, linkID string, opts ...RequestOption) (*PublicPaymentLink, error) {
	return get[PublicPaymentLink](ctx, s.c, "GET /v1/link/{id}", nil, map[string]string{"id": linkID}, opts)
}

// Checkout spawns an invoice from the link (POST /v1/link/{id}/checkout). No credentials needed;
// the core rate-caps it per IP.
func (s *PaymentLinksService) Checkout(ctx context.Context, linkID string, params LinkCheckoutParams, opts ...RequestOption) (*PublicPayment, error) {
	return postPath[PublicPayment](ctx, s.c, "POST /v1/link/{id}/checkout", map[string]string{"id": linkID}, params, opts)
}
