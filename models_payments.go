package oblodai

// PaymentTx is one on-chain deposit attributed to an invoice.
type PaymentTx struct {
	TxID      string    `json:"txid"`
	Amount    Money     `json:"amount"`
	Network   Network   `json:"network"`
	Height    int64     `json:"height"`
	CreatedAt Timestamp `json:"created_at"`
}

// PaymentRefund is a refund issued against an invoice: a payout in disguise, with the full detail
// available from Payouts.Info.
type PaymentRefund struct {
	UUID      string       `json:"uuid"`
	Address   string       `json:"address"`
	Amount    Money        `json:"amount"`
	Status    PayoutStatus `json:"status"`
	IsFinal   bool         `json:"is_final"`
	TxID      string       `json:"txid"`
	CreatedAt Timestamp    `json:"created_at"`
}

// Payment is the invoice as /v1/payment, /v1/payment/info, /v1/payment/history and
// /v1/payment/cancel render it. Refunds and RefundStatus are present on info only.
type Payment struct {
	UUID    string        `json:"uuid"`
	OrderID string        `json:"order_id"`
	Status  PaymentStatus `json:"status"`
	IsFinal bool          `json:"is_final"`
	// Amount is the priced amount, in Currency.
	Amount   Money  `json:"amount"`
	Currency string `json:"currency"`
	// Network is the settlement network; empty until the payer selects one on a multi-network
	// invoice.
	Network Network `json:"network"`
	// PayerAmount is the amount due in the payer asset (PayerCurrency).
	PayerAmount     Money  `json:"payer_amount"`
	PayerCurrency   string `json:"payer_currency"`
	AmountPaid      Money  `json:"amount_paid"`
	AmountRemaining Money  `json:"amount_remaining"`
	Address         string `json:"address"`
	// DestinationTag is the XRP destination tag, and Memo the Stellar or TON memo, when the network
	// needs one.
	DestinationTag  string `json:"destination_tag"`
	Memo            string `json:"memo"`
	AddressXAddress string `json:"address_xaddress"`
	AddressMuxed    string `json:"address_muxed"`
	// AddressQRCode is a data:image/png;base64,… QR of the payment URI.
	AddressQRCode string `json:"address_qr_code"`
	IsMulti       bool   `json:"is_multi"`
	// URL is the hosted pay page.
	URL           string    `json:"url"`
	URLReturn     string    `json:"url_return"`
	URLSuccess    string    `json:"url_success"`
	ExpiredAt     Timestamp `json:"expired_at"`
	RateExpiresAt Timestamp `json:"rate_expires_at"`
	ExchangeRate  Money     `json:"exchange_rate"`
	// Confirmations is how many the settlement transaction has, out of RequiredConfirmations.
	Confirmations         int         `json:"confirmations"`
	RequiredConfirmations int         `json:"required_confirmations"`
	TxID                  string      `json:"txid"`
	TxList                []PaymentTx `json:"tx_list"`
	// PaidAt is null while the invoice is unpaid.
	PaidAt                   *Timestamp `json:"paid_at"`
	PayerAddress             string     `json:"payer_address"`
	PayerAddressIsRefundable bool       `json:"payer_address_is_refundable"`
	PayerEmail               string     `json:"payer_email"`
	AdditionalData           string     `json:"additional_data"`
	Commission               Money      `json:"commission"`
	MerchantAmount           Money      `json:"merchant_amount"`
	DocumentURL              string     `json:"document_url"`
	IsTest                   bool       `json:"is_test"`
	CreatedAt                Timestamp  `json:"created_at"`
	UpdatedAt                Timestamp  `json:"updated_at"`
	// Refunds and RefundStatus arrive on /v1/payment/info only.
	Refunds      []PaymentRefund `json:"refunds,omitempty"`
	RefundStatus string          `json:"refund_status,omitempty"`
}

// PublicPayment is the payer-facing view of an invoice — GET /v1/pay/{id}, /v1/pay/{id}/select and
// /v1/link/{id}/checkout — with no merchant-only fields.
type PublicPayment struct {
	UUID                  string        `json:"uuid"`
	OrderID               string        `json:"order_id"`
	Status                PaymentStatus `json:"status"`
	IsFinal               bool          `json:"is_final"`
	Amount                Money         `json:"amount"`
	Currency              string        `json:"currency"`
	Network               Network       `json:"network"`
	PayerAmount           Money         `json:"payer_amount"`
	PayerCurrency         string        `json:"payer_currency"`
	AmountPaid            Money         `json:"amount_paid"`
	AmountRemaining       Money         `json:"amount_remaining"`
	Address               string        `json:"address"`
	DestinationTag        string        `json:"destination_tag"`
	Memo                  string        `json:"memo"`
	AddressXAddress       string        `json:"address_xaddress"`
	AddressMuxed          string        `json:"address_muxed"`
	AddressQRCode         string        `json:"address_qr_code"`
	IsMulti               bool          `json:"is_multi"`
	URL                   string        `json:"url"`
	URLReturn             string        `json:"url_return"`
	URLSuccess            string        `json:"url_success"`
	ExpiredAt             Timestamp     `json:"expired_at"`
	RateExpiresAt         Timestamp     `json:"rate_expires_at"`
	Confirmations         int           `json:"confirmations"`
	RequiredConfirmations int           `json:"required_confirmations"`
	TxID                  string        `json:"txid"`
	CreatedAt             Timestamp     `json:"created_at"`
	UpdatedAt             Timestamp     `json:"updated_at"`
}

// QRCode is the body of /v1/payment/qr and GET /v1/pay/{id}/qr. Every field is empty while the
// invoice has no real address: sandbox invoices (their address is a synthetic sandbox: one) and
// select invoices still awaiting a network.
type QRCode struct {
	// Image is a data:image/png;base64,… PNG.
	Image string `json:"image"`
	// Payload is what the QR encodes: a payment URI when IsURI, else the bare address.
	Payload string `json:"payload"`
	IsURI   bool   `json:"is_uri"`
	Address string `json:"address"`
}

// ResolutionAccepted is /v1/payment/resolve with action "accept": the underpayment was kept as
// full settlement. With action "refund" the body is the refund payout instead.
type ResolutionAccepted struct {
	// Resolution is always "accepted" on this body.
	Resolution  string `json:"resolution"`
	PaymentUUID string `json:"payment_uuid"`
	OrderID     string `json:"order_id"`
	Currency    string `json:"currency"`
	AmountKept  Money  `json:"amount_kept"`
}

// EmailSent is the body of /v1/payment/send-email.
type EmailSent struct {
	OK    bool   `json:"ok"`
	Email string `json:"email"`
	UUID  string `json:"uuid"`
}

// ServiceLimit is the per-method amount range. Both bounds are null when the asset cannot be
// priced right now.
type ServiceLimit struct {
	// Currency is the asset the bounds are quoted in, when the core reports one.
	Currency  string `json:"currency,omitempty"`
	MinAmount *Money `json:"min_amount"`
	MaxAmount *Money `json:"max_amount"`
}

// ServiceCommission is the fee charged on a method. FeeAmount and Percent are null when the fee
// cannot be quoted.
type ServiceCommission struct {
	Currency  string  `json:"currency"`
	FeeAmount *Money  `json:"fee_amount"`
	Percent   *string `json:"percent"`
	FeeType   string  `json:"fee_type"`
}

// ServiceMethod is one item of /v1/payment/services and /v1/payout/services: a currency on a
// network, with its limits and fee.
type ServiceMethod struct {
	Currency    string            `json:"currency"`
	Network     Network           `json:"network"`
	IsAvailable bool              `json:"is_available"`
	Limit       ServiceLimit      `json:"limit"`
	Commission  ServiceCommission `json:"commission"`
}

// BatchSubmitted acknowledges /v1/payment/batch, /v1/refund/batch, /v1/payout/batch and
// /v1/transfer/batch. Poll BatchID with Batches.Info for the outcome.
type BatchSubmitted struct {
	BatchID string      `json:"batch_id"`
	Kind    BatchKind   `json:"kind"`
	Status  BatchStatus `json:"status"`
	// Count is how many elements were queued.
	Count int `json:"count"`
}

// BatchInfoItem is one element of a batch as /v1/batch/info reports it, with the per-element
// status the worker recorded.
type BatchInfoItem struct {
	Idx int `json:"idx"`
	// OK is absent while the element has not been attempted yet.
	OK      *bool  `json:"ok,omitempty"`
	OrderID string `json:"order_id,omitempty"`
	Status  string `json:"status"`
	// Result is the raw object the element produced; its shape follows the batch kind.
	Result     map[string]any `json:"result,omitempty"`
	Message    string         `json:"message,omitempty"`
	ErrorCode  string         `json:"error_code,omitempty"`
	HTTPStatus int            `json:"http_status,omitempty"`
}

// BatchInfo is the body of /v1/batch/info: an asynchronous batch and every element in it.
type BatchInfo struct {
	BatchID   string          `json:"batch_id"`
	Kind      BatchKind       `json:"kind"`
	Status    BatchStatus     `json:"status"`
	OnError   BatchOnError    `json:"on_error"`
	Total     int             `json:"total"`
	Succeeded int             `json:"succeeded"`
	Failed    int             `json:"failed"`
	Items     []BatchInfoItem `json:"items"`
	CreatedAt Timestamp       `json:"created_at"`
	UpdatedAt Timestamp       `json:"updated_at"`
}
