package oblodai

// Payout is the payout as /v1/payout, /v1/payout/info, /v1/payout/history, /v1/payout/cancel,
// mass and batch elements and refunds render it. Error and ErrorCode appear on info for failed
// payouts, WalletUUID on refunds of blocked static-wallet deposits.
type Payout struct {
	UUID string `json:"uuid"`
	// OrderID is the merchant reference; null for refunds, which are keyed by Reference and
	// RefundFor instead.
	OrderID  *string      `json:"order_id"`
	Status   PayoutStatus `json:"status"`
	IsFinal  bool         `json:"is_final"`
	Amount   Money        `json:"amount"`
	Currency string       `json:"currency"`
	Network  Network      `json:"network"`
	Address  string       `json:"address"`
	Memo     string       `json:"memo"`
	// PayerAmount is the total debited from the balance: the amount plus the commission when the
	// merchant bears the fee.
	PayerAmount Money           `json:"payer_amount"`
	Commission  Money           `json:"commission"`
	FeeBearer   FeeBearerResult `json:"fee_bearer"`
	// Source is the balance the payout was funded from (business or personal).
	Source           string `json:"source"`
	ApprovalRequired bool   `json:"approval_required"`
	IsRefund         bool   `json:"is_refund"`
	// RefundFor is the invoice being refunded, on refunds.
	RefundFor      *string   `json:"refund_for"`
	PaymentOrderID *string   `json:"payment_order_id"`
	TxID           string    `json:"txid"`
	DocumentURL    string    `json:"document_url"`
	CreatedAt      Timestamp `json:"created_at"`
	UpdatedAt      Timestamp `json:"updated_at"`
	// Error and ErrorCode carry why a failed payout failed; both are null while nothing went wrong.
	Error     *string `json:"error,omitempty"`
	ErrorCode *string `json:"error_code,omitempty"`
	// WalletUUID is set on refunds of deposits that landed on a blocked static wallet.
	WalletUUID string `json:"wallet_uuid,omitempty"`
}

// PayoutCalculation is the body of /v1/payout/calculate. The amounts are null when the asset
// cannot be priced right now.
type PayoutCalculation struct {
	Amount      *Money          `json:"amount"`
	Currency    string          `json:"currency"`
	Network     Network         `json:"network"`
	Commission  *Money          `json:"commission"`
	PayerAmount *Money          `json:"payer_amount"`
	FeeBearer   FeeBearerResult `json:"fee_bearer"`
	FeeType     string          `json:"fee_type"`
}

// PayoutValidation is the body of /v1/payout/validate, the dry run: it raises the same errors the
// create call would.
type PayoutValidation struct {
	Valid       bool            `json:"valid"`
	Amount      Money           `json:"amount"`
	Currency    string          `json:"currency"`
	Network     Network         `json:"network"`
	Commission  Money           `json:"commission"`
	PayerAmount Money           `json:"payer_amount"`
	FeeBearer   FeeBearerResult `json:"fee_bearer"`
	// FundedBy is which balance would fund it (business or personal), when the core reports it.
	FundedBy string `json:"funded_by,omitempty"`
	// MaturityNote is non-empty when part of the balance is still maturing inside the reorg window.
	MaturityNote string `json:"maturity_note"`
}

// TransferToPersonal is the body of /v1/transfer/to-personal: business to the owner's personal
// balance.
type TransferToPersonal struct {
	UUID     string `json:"uuid"`
	Currency string `json:"currency"`
	Amount   Money  `json:"amount"`
	// Direction is always "to_personal" on this body.
	Direction string `json:"direction"`
	// PersonalBalance is the personal balance after the transfer.
	PersonalBalance Money  `json:"personal_balance"`
	DocumentURL     string `json:"document_url"`
}

// TransferToUser is the body of /v1/transfer/to-user: business to another user's personal balance.
type TransferToUser struct {
	UUID        string `json:"uuid"`
	Currency    string `json:"currency"`
	Amount      Money  `json:"amount"`
	ToUserID    string `json:"to_user_id"`
	DocumentURL string `json:"document_url"`
}

// PayoutFeeConfig is the body of /v1/payout/fee-config/get and /set.
type PayoutFeeConfig struct {
	FeeOnRecipient bool `json:"fee_on_recipient"`
	// Configured comes back from get only: whether the merchant ever set it.
	Configured *bool `json:"configured,omitempty"`
}

// RefundFeeConfig is the body of /v1/payout/refund-fee-config/get and /set.
type RefundFeeConfig struct {
	FeeOnCustomer bool `json:"fee_on_customer"`
	// Configured comes back from get only: whether the merchant ever set it.
	Configured *bool `json:"configured,omitempty"`
}

// PaymentFeeConfig is the body of /v1/payment/fee-config/get and /set.
type PaymentFeeConfig struct {
	PayerPaysPercent float64 `json:"payer_pays_percent"`
	// Enabled comes back from get only.
	Enabled *bool `json:"enabled,omitempty"`
}
