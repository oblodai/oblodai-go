package oblodai

// PayoutLink is the payout link (cheque) as /v1/payout/link, /info, /list, /cancel and batch
// elements render it.
type PayoutLink struct {
	LinkID   string           `json:"link_id"`
	Status   PayoutLinkStatus `json:"status"`
	Amount   Money            `json:"amount"`
	Currency string           `json:"currency"`
	Network  Network          `json:"network"`
	// Commission and PayerAmount are null while the asset cannot be priced.
	Commission        *Money    `json:"commission"`
	PayerAmount       *Money    `json:"payer_amount"`
	FeeBearer         FeeBearer `json:"fee_bearer"`
	FeeType           string    `json:"fee_type"`
	Reference         string    `json:"reference"`
	Title             string    `json:"title"`
	Note              string    `json:"note"`
	PasscodeProtected bool      `json:"passcode_protected"`
	ExpiresAt         Timestamp `json:"expires_at"`
	CreatedAt         Timestamp `json:"created_at"`
	// ClaimToken is the secret the recipient claims with; it comes back on create and batch-create
	// only, and ClaimURL is the page built around it.
	ClaimToken string `json:"claim_token,omitempty"`
	ClaimURL   string `json:"claim_url,omitempty"`
	BatchID    string `json:"batch_id,omitempty"`
	// PayoutID is set once claimed: the payout that paid the recipient.
	PayoutID     string `json:"payout_id,omitempty"`
	ClaimAddress string `json:"claim_address,omitempty"`
	Email        string `json:"email,omitempty"`
	// Passcode is the generated passcode, shown once on create when passcode "auto" was requested.
	Passcode string `json:"passcode,omitempty"`
}

// ClaimPreview is GET /v1/claim/{token} — what the recipient sees before claiming.
type ClaimPreview struct {
	Status      PayoutLinkStatus `json:"status"`
	Claimable   bool             `json:"claimable"`
	Amount      Money            `json:"amount"`
	Currency    string           `json:"currency"`
	Network     Network          `json:"network"`
	Commission  *Money           `json:"commission"`
	PayerAmount *Money           `json:"payer_amount"`
	FeeBearer   FeeBearer        `json:"fee_bearer"`
	FeeType     string           `json:"fee_type"`
	Title       string           `json:"title"`
	Note        string           `json:"note"`
	ExpiresAt   Timestamp        `json:"expires_at"`
}

// ClaimResult is POST /v1/claim/{token} — the payout minted by a claim.
type ClaimResult struct {
	// PayoutID is the payout that pays the recipient; pass it to Payouts.Info.
	PayoutID    string       `json:"payout_id"`
	Status      PayoutStatus `json:"status"`
	Address     string       `json:"address"`
	Amount      Money        `json:"amount"`
	Currency    string       `json:"currency"`
	Network     Network      `json:"network"`
	Commission  *Money       `json:"commission"`
	PayerAmount *Money       `json:"payer_amount"`
	FeeBearer   FeeBearer    `json:"fee_bearer"`
	FeeType     string       `json:"fee_type"`
}

// PaymentLinkPayment is one invoice spawned by a payment link.
type PaymentLinkPayment struct {
	UUID      string        `json:"uuid"`
	OrderID   string        `json:"order_id,omitempty"`
	Amount    Money         `json:"amount"`
	Currency  string        `json:"currency"`
	Status    PaymentStatus `json:"status"`
	CreatedAt Timestamp     `json:"created_at"`
}

// PaymentLink is the payment link as /v1/payment/link/info and /list render it. Which amount
// fields are set follows AmountMode.
type PaymentLink struct {
	LinkID      string     `json:"link_id"`
	URL         string     `json:"url"`
	Active      bool       `json:"active"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	AmountMode  AmountMode `json:"amount_mode"`
	Currency    string     `json:"currency"`
	// AmountFixed is set on fixed links.
	AmountFixed Money `json:"amount_fixed,omitempty"`
	// MinAmount and MaxAmount are set on range links.
	MinAmount      Money     `json:"min_amount,omitempty"`
	MaxAmount      Money     `json:"max_amount,omitempty"`
	PinnedCurrency string    `json:"pinned_currency,omitempty"`
	PinnedNetwork  Network   `json:"pinned_network,omitempty"`
	ExpiresAt      Timestamp `json:"expires_at,omitempty"`
	DocumentURL    string    `json:"document_url"`
	CreatedAt      Timestamp `json:"created_at"`
	// Payments comes back from info only: the invoices this link spawned.
	Payments []PaymentLinkPayment `json:"payments,omitempty"`
}

// PaymentLinkCreated acknowledges POST /v1/payment/link.
type PaymentLinkCreated struct {
	LinkID      string `json:"link_id"`
	URL         string `json:"url"`
	DocumentURL string `json:"document_url"`
}

// PaymentLinkToggled is the body of /v1/payment/link/toggle.
type PaymentLinkToggled struct {
	LinkID string `json:"link_id"`
	Active bool   `json:"active"`
}

// PublicPaymentLink is GET /v1/link/{id} — the payer-facing view of a payment link.
type PublicPaymentLink struct {
	LinkID         string     `json:"link_id"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	AmountMode     AmountMode `json:"amount_mode"`
	Currency       string     `json:"currency"`
	AmountFixed    Money      `json:"amount_fixed,omitempty"`
	MinAmount      Money      `json:"min_amount,omitempty"`
	MaxAmount      Money      `json:"max_amount,omitempty"`
	PinnedCurrency string     `json:"pinned_currency,omitempty"`
	PinnedNetwork  Network    `json:"pinned_network,omitempty"`
}
