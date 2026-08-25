package oblodai

import "context"

// WebhooksService registers the merchant's endpoint and inspects deliveries. Verifying an
// incoming delivery needs no client at all — use github.com/oblodai/oblodai-go/webhooks.
type WebhooksService struct{ c *Client }

// Register sets (or replaces) the merchant's webhook endpoint (POST /v1/webhooks) and returns the
// signing secret. The secret is shown once: store it where your receiver can read it.
func (s *WebhooksService) Register(ctx context.Context, url string, opts ...RequestOption) (*WebhookEndpoint, error) {
	return post[WebhookEndpoint](ctx, s.c, "POST /v1/webhooks", WebhooksParams{URL: url}, opts)
}

// RotateSecret issues a new signing secret (POST /v1/webhooks/rotate-secret). The old one keeps
// verifying until PreviousSecretValidUntil — keep both in your receiver until then. Payout key.
func (s *WebhooksService) RotateSecret(ctx context.Context, opts ...RequestOption) (*WebhookSecretRotated, error) {
	return post[WebhookSecretRotated](ctx, s.c, "POST /v1/webhooks/rotate-secret", nil, opts)
}

// Deliveries lists the delivery log, newest first (POST /v1/webhooks/deliveries).
func (s *WebhooksService) Deliveries(ctx context.Context, params WebhooksDeliveriesParams, opts ...RequestOption) *List[WebhookDelivery] {
	return listOf[WebhookDelivery](ctx, s.c, "POST /v1/webhooks/deliveries", params, opts)
}

// WebhookTestParams is the body of a test delivery. Status is validated against the vocabulary of
// the event kind: a payment status for WebhookKindPayment, a payout status for
// WebhookKindPayout, "paid" for WebhookKindWallet.
type WebhookTestParams struct {
	// URLCallback is where to send the test body. Required.
	URLCallback string `json:"url_callback"`
	// Currency put into the test event body.
	Currency string `json:"currency,omitempty"`
	// Network put into the test event body.
	Network Network `json:"network,omitempty"`
	// OrderID put into the test event body.
	OrderID string `json:"order_id,omitempty"`
	// Status put into the test event body. Defaults to paid (confirmed for a payout).
	Status string `json:"status,omitempty"`
}

// Test delivers a sample event of that kind to URLCallback, signed exactly like a real one
// (POST /v1/test-webhook/{payment|payout|wallet}) — the way to exercise a receiver end to end.
// It wants the payout key for WebhookKindPayout.
func (s *WebhooksService) Test(ctx context.Context, kind WebhookKind, params WebhookTestParams, opts ...RequestOption) (*WebhookTestResult, error) {
	switch kind {
	case WebhookKindPayment, WebhookKindPayout, WebhookKindWallet:
	default:
		return nil, newConfigError(CodeBadConfig,
			"the webhook kind must be payment, payout or wallet (got "+string(kind)+")", "kind")
	}
	return post[WebhookTestResult](ctx, s.c, "POST /v1/test-webhook/"+string(kind), params, opts)
}

// TestLegacy is the older rehearsal door, for payment events only
// (POST /v1/payment/testing-webhook).
//
// Deprecated: use Test with WebhookKindPayment.
func (s *WebhooksService) TestLegacy(ctx context.Context, params PaymentTestingWebhookParams, opts ...RequestOption) (*WebhookTestResult, error) {
	return post[WebhookTestResult](ctx, s.c, "POST /v1/payment/testing-webhook", params, opts)
}

// SandboxService is the developer sandbox, open to test_ keys only: fake money, simulated
// deposits and a webhook inspector.
type SandboxService struct{ c *Client }

// Faucet credits test funds (POST /v1/sandbox/faucet). Payout key.
func (s *SandboxService) Faucet(ctx context.Context, params SandboxFaucetParams, opts ...RequestOption) (*FaucetResult, error) {
	return post[FaucetResult](ctx, s.c, "POST /v1/sandbox/faucet", params, opts)
}

// Deposit simulates an on-chain deposit to an invoice (POST /v1/sandbox/deposit). Repeating the
// same txid adds confirmations instead of paying twice.
func (s *SandboxService) Deposit(ctx context.Context, params SandboxDepositParams, opts ...RequestOption) (*SandboxDeposit, error) {
	return post[SandboxDeposit](ctx, s.c, "POST /v1/sandbox/deposit", params, opts)
}

// SandboxWebhooksParams pages the sandbox webhook inspector.
type SandboxWebhooksParams struct {
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}

// Webhooks lists sandbox deliveries together with their payloads (GET /v1/sandbox/webhooks) —
// what your receiver would have been sent.
func (s *SandboxService) Webhooks(ctx context.Context, params SandboxWebhooksParams, opts ...RequestOption) *List[WebhookDelivery] {
	return listOf[WebhookDelivery](ctx, s.c, "GET /v1/sandbox/webhooks", params, opts)
}

// Replay re-sends a terminal (delivered or dead) delivery
// (POST /v1/sandbox/webhooks/replay).
func (s *SandboxService) Replay(ctx context.Context, deliveryID string, opts ...RequestOption) (*SandboxReplay, error) {
	return post[SandboxReplay](ctx, s.c, "POST /v1/sandbox/webhooks/replay",
		SandboxWebhooksReplayParams{DeliveryID: deliveryID}, opts)
}

// Reset cancels the store's open invoices and zeroes its balances (POST /v1/sandbox/reset).
// Payout key.
func (s *SandboxService) Reset(ctx context.Context, opts ...RequestOption) (*SandboxReset, error) {
	return post[SandboxReset](ctx, s.c, "POST /v1/sandbox/reset", nil, opts)
}

// MerchantsService provisions merchants — for platforms that onboard merchants themselves. These
// routes are not HMAC-signed; a self-hosted gateway gates them with its admin token
// (WithAdminToken).
type MerchantsService struct{ c *Client }

// Create provisions a merchant and mints its payment and payout keys (POST /v1/merchants). The
// secrets are shown once.
func (s *MerchantsService) Create(ctx context.Context, params MerchantsParams, opts ...RequestOption) (*MerchantOnboarded, error) {
	return post[MerchantOnboarded](ctx, s.c, "POST /v1/merchants", params, opts)
}

// CreateSandbox returns the merchant's dev store and its test_ key
// (POST /v1/merchants/{id}/sandbox). The call is idempotent: Created is false when the store
// already existed.
func (s *MerchantsService) CreateSandbox(ctx context.Context, merchantID string, opts ...RequestOption) (*SandboxStore, error) {
	return postPath[SandboxStore](ctx, s.c, "POST /v1/merchants/{id}/sandbox",
		map[string]string{"id": merchantID}, nil, opts)
}
