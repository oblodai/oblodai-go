package oblodai

import "context"

// AccountService reads balances and account-level facts.
type AccountService struct{ c *Client }

// Balance reads the available balance per currency (POST /v1/balance). "Available" excludes what
// is still maturing in a reorg window and what is reserved by open payout links.
func (s *AccountService) Balance(ctx context.Context, opts ...RequestOption) (*Balance, error) {
	return post[Balance](ctx, s.c, "POST /v1/balance", nil, opts)
}

// Referral reads the referral code, link and earnings (POST /v1/referral/info).
func (s *AccountService) Referral(ctx context.Context, opts ...RequestOption) (*ReferralInfo, error) {
	return post[ReferralInfo](ctx, s.c, "POST /v1/referral/info", nil, opts)
}

// VRCS reads volatility risk control — whether volatile deposits are auto-converted to USDT
// (POST /v1/vrcs).
func (s *AccountService) VRCS(ctx context.Context, opts ...RequestOption) (*VRCSStatus, error) {
	return post[VRCSStatus](ctx, s.c, "POST /v1/vrcs", nil, opts)
}

// SetVRCS turns volatility risk control on or off (POST /v1/vrcs).
func (s *AccountService) SetVRCS(ctx context.Context, enabled bool, opts ...RequestOption) (*VRCSStatus, error) {
	return post[VRCSStatus](ctx, s.c, "POST /v1/vrcs", VRCSParams{Enabled: &enabled}, opts)
}

// CatalogService is public reference data. No credentials needed.
type CatalogService struct{ c *Client }

// Currencies lists every asset, its networks and their live availability (GET /v1/currencies).
func (s *CatalogService) Currencies(ctx context.Context, opts ...RequestOption) (*Currencies, error) {
	return get[Currencies](ctx, s.c, "GET /v1/currencies", nil, nil, opts)
}

// ExchangeRates lists current rates, optionally filtered by CurrencyFrom and CurrencyTo
// (POST /v1/exchange-rate/list).
func (s *CatalogService) ExchangeRates(ctx context.Context, params ExchangeRateListParams, opts ...RequestOption) *List[ExchangeRate] {
	return listOf[ExchangeRate](ctx, s.c, "POST /v1/exchange-rate/list", params, opts)
}
