package oblodai

import "context"

// SettingsService is the merchant-level configuration the API exposes. The payer-facing settings
// take the payment key; the auto-withdraw and IP allow-list routes take the payout key.
type SettingsService struct{ c *Client }

// SetDiscount sets a payer-facing discount or markup per currency and network
// (POST /v1/payment/discount/set). A negative percent is a markup.
func (s *SettingsService) SetDiscount(ctx context.Context, params PaymentDiscountSetParams, opts ...RequestOption) (*DiscountRule, error) {
	return post[DiscountRule](ctx, s.c, "POST /v1/payment/discount/set", params, opts)
}

// ListDiscounts lists the discount rules (POST /v1/payment/discount/list).
func (s *SettingsService) ListDiscounts(ctx context.Context, params PaymentDiscountListParams, opts ...RequestOption) *List[DiscountRule] {
	return listOf[DiscountRule](ctx, s.c, "POST /v1/payment/discount/list", params, opts)
}

// GetAccuracy reads the under/overpayment tolerance (POST /v1/payment/accuracy/get).
func (s *SettingsService) GetAccuracy(ctx context.Context, opts ...RequestOption) (*AccuracyConfig, error) {
	return post[AccuracyConfig](ctx, s.c, "POST /v1/payment/accuracy/get", nil, opts)
}

// SetAccuracy sets the under/overpayment tolerance (POST /v1/payment/accuracy/set).
func (s *SettingsService) SetAccuracy(ctx context.Context, params PaymentAccuracySetParams, opts ...RequestOption) (*AccuracyConfig, error) {
	return post[AccuracyConfig](ctx, s.c, "POST /v1/payment/accuracy/set", params, opts)
}

// GetAutoRefund reads whether over- and underpayments are refunded automatically
// (POST /v1/payment/autorefund/get).
func (s *SettingsService) GetAutoRefund(ctx context.Context, opts ...RequestOption) (*AutoRefundConfig, error) {
	return post[AutoRefundConfig](ctx, s.c, "POST /v1/payment/autorefund/get", nil, opts)
}

// SetAutoRefund turns automatic refunds of over- and underpayments on or off
// (POST /v1/payment/autorefund/set).
func (s *SettingsService) SetAutoRefund(ctx context.Context, params PaymentAutorefundSetParams, opts ...RequestOption) (*AutoRefundConfig, error) {
	return post[AutoRefundConfig](ctx, s.c, "POST /v1/payment/autorefund/set", params, opts)
}

// ListAccepted lists which currency and network pairs invoices may be paid in
// (POST /v1/payment/accepted/list).
func (s *SettingsService) ListAccepted(ctx context.Context, params PaymentAcceptedListParams, opts ...RequestOption) *List[AcceptedMethod] {
	return listOf[AcceptedMethod](ctx, s.c, "POST /v1/payment/accepted/list", params, opts)
}

// SetAccepted replaces the accepted currency and network pairs
// (POST /v1/payment/accepted/set). An empty list means "everything in the catalog".
func (s *SettingsService) SetAccepted(ctx context.Context, params PaymentAcceptedSetParams, opts ...RequestOption) (*OkResult, error) {
	return post[OkResult](ctx, s.c, "POST /v1/payment/accepted/set", params, opts)
}

// GetPaymentFeeConfig reads what share of the network fee the payer is charged
// (POST /v1/payment/fee-config/get).
func (s *SettingsService) GetPaymentFeeConfig(ctx context.Context, opts ...RequestOption) (*PaymentFeeConfig, error) {
	return post[PaymentFeeConfig](ctx, s.c, "POST /v1/payment/fee-config/get", nil, opts)
}

// SetPaymentFeeConfig sets what share of the network fee the payer is charged
// (POST /v1/payment/fee-config/set).
func (s *SettingsService) SetPaymentFeeConfig(ctx context.Context, params PaymentFeeConfigSetParams, opts ...RequestOption) (*PaymentFeeConfig, error) {
	return post[PaymentFeeConfig](ctx, s.c, "POST /v1/payment/fee-config/set", params, opts)
}

// ListAutoWithdraw lists the automatic withdrawal rules (POST /v1/auto-withdraw/list). Payout key.
func (s *SettingsService) ListAutoWithdraw(ctx context.Context, opts ...RequestOption) ([]AutoWithdrawRule, error) {
	return items[AutoWithdrawRule](ctx, s.c, "POST /v1/auto-withdraw/list", nil, opts)
}

// SetAutoWithdraw sweeps a currency to an address once the balance passes MinAmount
// (POST /v1/auto-withdraw/set) and returns the full rule set. Payout key.
func (s *SettingsService) SetAutoWithdraw(ctx context.Context, params AutoWithdrawSetParams, opts ...RequestOption) ([]AutoWithdrawRule, error) {
	return items[AutoWithdrawRule](ctx, s.c, "POST /v1/auto-withdraw/set", params, opts)
}

// DeleteAutoWithdraw drops the rule for a currency (POST /v1/auto-withdraw/delete) and returns
// what is left. Payout key.
func (s *SettingsService) DeleteAutoWithdraw(ctx context.Context, currency string, opts ...RequestOption) ([]AutoWithdrawRule, error) {
	return items[AutoWithdrawRule](ctx, s.c, "POST /v1/auto-withdraw/delete",
		AutoWithdrawDeleteParams{Currency: currency}, opts)
}

// ListAPIAllowlist lists the source IPs allowed to use the API keys
// (POST /v1/api-allowlist/list). Payout key.
func (s *SettingsService) ListAPIAllowlist(ctx context.Context, opts ...RequestOption) (*APIAllowlist, error) {
	return post[APIAllowlist](ctx, s.c, "POST /v1/api-allowlist/list", nil, opts)
}

// AddAPIAllowlist adds an IP or CIDR to the allow-list (POST /v1/api-allowlist/add). Payout key.
func (s *SettingsService) AddAPIAllowlist(ctx context.Context, cidr string, opts ...RequestOption) (*APIAllowlist, error) {
	return post[APIAllowlist](ctx, s.c, "POST /v1/api-allowlist/add", APIAllowlistAddParams{CIDR: cidr}, opts)
}

// RemoveAPIAllowlist drops an entry from the allow-list (POST /v1/api-allowlist/remove).
// Payout key.
func (s *SettingsService) RemoveAPIAllowlist(ctx context.Context, cidr string, opts ...RequestOption) (*APIAllowlist, error) {
	return post[APIAllowlist](ctx, s.c, "POST /v1/api-allowlist/remove", APIAllowlistRemoveParams{CIDR: cidr}, opts)
}

// EnableAPIAllowlist switches enforcement on or off, keeping the list
// (POST /v1/api-allowlist/enable). Payout key.
func (s *SettingsService) EnableAPIAllowlist(ctx context.Context, enabled bool, opts ...RequestOption) (*APIAllowlist, error) {
	return post[APIAllowlist](ctx, s.c, "POST /v1/api-allowlist/enable",
		APIAllowlistEnableParams{Enabled: enabled}, opts)
}

// SplitsService forwards a share of every payment to a partner. It wants the payout key.
type SplitsService struct{ c *Client }

// CreateRule adds a split rule (POST /v1/split/rule) — to an external address (Address plus
// Network) or to a platform merchant (MerchantID, which makes the share reversible on refund).
func (s *SplitsService) CreateRule(ctx context.Context, params SplitRuleParams, opts ...RequestOption) (*SplitRule, error) {
	return post[SplitRule](ctx, s.c, "POST /v1/split/rule", params, opts)
}

// ListRules lists the split rules (POST /v1/split/rule/list).
func (s *SplitsService) ListRules(ctx context.Context, params SplitRuleListParams, opts ...RequestOption) *List[SplitRule] {
	return listOf[SplitRule](ctx, s.c, "POST /v1/split/rule/list", params, opts)
}

// DeleteRule removes a split rule (POST /v1/split/rule/delete).
func (s *SplitsService) DeleteRule(ctx context.Context, ruleID string, opts ...RequestOption) (*OkResult, error) {
	return post[OkResult](ctx, s.c, "POST /v1/split/rule/delete", SplitRuleDeleteParams{RuleID: ruleID}, opts)
}

// GetConfig reads how long split shares are held back for refunds
// (POST /v1/split/config/get).
func (s *SplitsService) GetConfig(ctx context.Context, opts ...RequestOption) (*SplitConfig, error) {
	return post[SplitConfig](ctx, s.c, "POST /v1/split/config/get", nil, opts)
}

// SetConfig sets how long split shares are held back for refunds
// (POST /v1/split/config/set).
func (s *SplitsService) SetConfig(ctx context.Context, params SplitConfigSetParams, opts ...RequestOption) (*SplitConfig, error) {
	return post[SplitConfig](ctx, s.c, "POST /v1/split/config/set", params, opts)
}

// GetOptIn reads whether this merchant accepts being a split recipient
// (POST /v1/split/recipient/optin/get).
func (s *SplitsService) GetOptIn(ctx context.Context, opts ...RequestOption) (*SplitOptIn, error) {
	return post[SplitOptIn](ctx, s.c, "POST /v1/split/recipient/optin/get", nil, opts)
}

// SetOptIn accepts or refuses being a split recipient (POST /v1/split/recipient/optin).
func (s *SplitsService) SetOptIn(ctx context.Context, enabled bool, opts ...RequestOption) (*SplitOptIn, error) {
	return post[SplitOptIn](ctx, s.c, "POST /v1/split/recipient/optin",
		SplitRecipientOptinParams{Enabled: enabled}, opts)
}
