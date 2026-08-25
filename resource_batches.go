package oblodai

import "context"

// BatchesService reports the progress of asynchronous batches: payment, refund, payout, transfer
// and payout-link.
type BatchesService struct{ c *Client }

// Info reads a batch's status, counters and per-row outcomes (POST /v1/batch/info).
//
// The route accepts either key kind, but the core requires the kind that created the batch, so a
// payout batch is transparently retried with the payout key when one is configured.
func (s *BatchesService) Info(ctx context.Context, params BatchInfoParams, opts ...RequestOption) (*BatchInfo, error) {
	result, err := post[BatchInfo](ctx, s.c, "POST /v1/batch/info", params, opts)
	if err != nil && IsCode(err, CodeWrongKeyKind) && s.c.transport.payoutCreds != nil && !applyRequestOptions(opts).preferPayoutKey {
		return post[BatchInfo](ctx, s.c, "POST /v1/batch/info", params, append(append([]RequestOption{}, opts...), WithPayoutKey()))
	}
	return result, err
}

// TransfersService moves money between platform balances: internal, instant and fee-free. It
// wants the payout key.
type TransfersService struct{ c *Client }

// ToPersonal moves funds from the business balance to the owner's personal wallet
// (POST /v1/transfer/to-personal). It needs an owner link on the merchant.
func (s *TransfersService) ToPersonal(ctx context.Context, params TransferToPersonalParams, opts ...RequestOption) (*TransferToPersonal, error) {
	return post[TransferToPersonal](ctx, s.c, "POST /v1/transfer/to-personal", params, opts)
}

// ToUser moves funds from the business balance to another platform user's personal wallet
// (POST /v1/transfer/to-user). Amount and Currency are required.
func (s *TransfersService) ToUser(ctx context.Context, params TransferToUserParams, opts ...RequestOption) (*TransferToUser, error) {
	return post[TransferToUser](ctx, s.c, "POST /v1/transfer/to-user", params, opts)
}

// Batch queues up to 5000 ToUser transfers asynchronously (POST /v1/transfer/batch); poll
// Batches.Info. OrderID is required on every element.
func (s *TransfersService) Batch(ctx context.Context, params TransferBatchParams, opts ...RequestOption) (*BatchSubmitted, error) {
	return post[BatchSubmitted](ctx, s.c, "POST /v1/transfer/batch", params, opts)
}

// WalletsService manages static deposit wallets: one permanent address per customer, whose
// deposits arrive as wallet.paid webhooks.
type WalletsService struct{ c *Client }

// Create allocates a static deposit address (POST /v1/wallet). Idempotent by OrderID.
func (s *WalletsService) Create(ctx context.Context, params WalletParams, opts ...RequestOption) (*Wallet, error) {
	return post[Wallet](ctx, s.c, "POST /v1/wallet", params, opts)
}

// QR renders the address as a QR image (POST /v1/wallet/qr).
func (s *WalletsService) QR(ctx context.Context, address string, opts ...RequestOption) (*WalletQR, error) {
	return post[WalletQR](ctx, s.c, "POST /v1/wallet/qr", WalletQRParams{Address: address}, opts)
}

// Block stops crediting an address (POST /v1/wallet/block). Deposits that land afterwards wait
// for a refund decision.
func (s *WalletsService) Block(ctx context.Context, params WalletBlockParams, opts ...RequestOption) (*WalletBlocked, error) {
	return post[WalletBlocked](ctx, s.c, "POST /v1/wallet/block", params, opts)
}

// RefundBlockedDeposit sends funds that landed on a blocked address back to their sender
// (POST /v1/wallet/blocked-address-refund). It wants the payout key.
func (s *WalletsService) RefundBlockedDeposit(ctx context.Context, params WalletBlockedAddressRefundParams, opts ...RequestOption) (*Payout, error) {
	return post[Payout](ctx, s.c, "POST /v1/wallet/blocked-address-refund", params, opts)
}
