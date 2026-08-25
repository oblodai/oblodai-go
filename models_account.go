package oblodai

// BalanceEntry is one asset's balance.
type BalanceEntry struct {
	Currency string `json:"currency"`
	// Balance is the available (spendable) balance.
	Balance Money `json:"balance"`
}

// BalanceAccounts groups the balances by account. Merchant is the business balance the API acts
// on.
type BalanceAccounts struct {
	Merchant []BalanceEntry `json:"merchant"`
}

// Balance is the body of /v1/balance.
type Balance struct {
	Balance BalanceAccounts `json:"balance"`
}

// ReferralWeek is the last seven days of referral activity.
type ReferralWeek struct {
	ReferredCount   int              `json:"referred_count"`
	EarningsByAsset map[string]Money `json:"earnings_by_asset"`
}

// ReferralInfo is the body of /v1/referral/info.
type ReferralInfo struct {
	Code string `json:"code"`
	Link string `json:"link"`
	// TierBPS are the referral tiers, in basis points.
	TierBPS         []int            `json:"tier_bps"`
	ReferredCount   int              `json:"referred_count"`
	EarningsByAsset map[string]Money `json:"earnings_by_asset"`
	Week            ReferralWeek     `json:"week"`
}

// VRCSStatus is the body of /v1/vrcs — volatility risk control, which auto-converts volatile
// deposits to USDT.
type VRCSStatus struct {
	Enabled bool `json:"enabled"`
}

// Wallet is a static (permanent) deposit wallet — /v1/wallet.
type Wallet struct {
	UUID     string  `json:"uuid"`
	Address  string  `json:"address"`
	Network  Network `json:"network"`
	Currency string  `json:"currency"`
	OrderID  string  `json:"order_id"`
	// URL is the hosted page showing the address and its QR.
	URL         string `json:"url"`
	DocumentURL string `json:"document_url"`
	// DestinationTag is the XRP destination tag, and Memo the TON or Stellar memo, when the network
	// needs one.
	DestinationTag  string `json:"destination_tag,omitempty"`
	Memo            string `json:"memo,omitempty"`
	AddressXAddress string `json:"address_xaddress,omitempty"`
	AddressMuxed    string `json:"address_muxed,omitempty"`
}

// WalletBlocked is the body of /v1/wallet/block.
type WalletBlocked struct {
	UUID    string `json:"uuid"`
	Address string `json:"address"`
	Blocked bool   `json:"blocked"`
}

// WalletQR is /v1/wallet/qr — the address QR as a data URI.
type WalletQR struct {
	Image string `json:"image"`
}

// AutoWithdrawRule is one entry of /v1/auto-withdraw/set, /list and /delete: sweep Currency on
// Network to Address once MinAmount is available.
type AutoWithdrawRule struct {
	Currency  string  `json:"currency"`
	Network   Network `json:"network"`
	Address   string  `json:"address"`
	MinAmount Money   `json:"min_amount"`
}

// APIAllowlist is the body of the /v1/api-allowlist routes. Items are CIDRs.
type APIAllowlist struct {
	Enabled bool     `json:"enabled"`
	Items   []string `json:"items"`
}

// DiscountRule is one entry of /v1/payment/discount/set and /list.
type DiscountRule struct {
	Currency string  `json:"currency"`
	Network  Network `json:"network"`
	// DiscountPercent is a discount for the payer when positive, a markup when negative.
	DiscountPercent float64 `json:"discount_percent"`
}

// AccuracyConfig is the body of /v1/payment/accuracy/get and /set: how much underpayment still
// counts as paid.
type AccuracyConfig struct {
	Enabled         bool    `json:"enabled"`
	AccuracyPercent float64 `json:"accuracy_percent"`
}

// AutoRefundConfig is the body of /v1/payment/autorefund/get and /set.
type AutoRefundConfig struct {
	Overpay  bool `json:"overpay"`
	Underpay bool `json:"underpay"`
	// Configured comes back from get only: whether the merchant ever set it.
	Configured *bool `json:"configured,omitempty"`
}

// AcceptedMethod is one entry of /v1/payment/accepted/list: a currency on a network the merchant
// accepts.
type AcceptedMethod struct {
	Currency  string  `json:"currency"`
	Network   Network `json:"network"`
	Available bool    `json:"available"`
	// Reason says why it is unavailable, when it is.
	Reason string `json:"reason,omitempty"`
}

// SplitRule is one entry of /v1/split/rule and /v1/split/rule/list. POST /v1/split/rule answers
// with RuleID and Percent only.
type SplitRule struct {
	RuleID string `json:"rule_id"`
	// Percent is the share of every payment, as a decimal string.
	Percent string  `json:"percent"`
	Active  *bool   `json:"active,omitempty"`
	Address string  `json:"address,omitempty"`
	Network Network `json:"network,omitempty"`
	// MerchantID is set on on-platform partner rules, which are reversible on refund.
	MerchantID string `json:"merchant_id,omitempty"`
	Note       string `json:"note,omitempty"`
	Reversible *bool  `json:"reversible,omitempty"`
}

// SplitConfig is the body of /v1/split/config/get and /set.
type SplitConfig struct {
	RefundHoldSeconds int `json:"refund_hold_seconds"`
}

// SplitOptIn is the body of /v1/split/recipient/optin and /optin/get.
type SplitOptIn struct {
	Enabled bool `json:"enabled"`
}

// DocumentPeriod is the range a document job covers, as plain dates.
type DocumentPeriod struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// DocumentFile is the finished document of a job.
type DocumentFile struct {
	// DownloadURL is a signed link; Documents.JobFile fetches the same bytes through the client.
	DownloadURL string    `json:"download_url"`
	ExpiresAt   Timestamp `json:"expires_at"`
	Rows        int       `json:"rows"`
	SizeBytes   int64     `json:"size_bytes"`
}

// DocumentJob is the body of /v1/documents/jobs and /v1/documents/jobs/info.
type DocumentJob struct {
	JobID  string `json:"job_id"`
	Kind   string `json:"kind"`
	Format string `json:"format"`
	Lang   string `json:"lang"`
	// Status is queued, processing, done or failed.
	Status string         `json:"status"`
	Period DocumentPeriod `json:"period"`
	// ReadyWithin is a human hint while queued (for example "15s").
	ReadyWithin string `json:"ready_within,omitempty"`
	// File is set once the job is done.
	File *DocumentFile `json:"file,omitempty"`
	// Error says why the job failed; it is null while nothing went wrong.
	Error     *string   `json:"error,omitempty"`
	CreatedAt Timestamp `json:"created_at"`
	UpdatedAt Timestamp `json:"updated_at"`
}
