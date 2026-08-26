package oblodai

// WebhookEndpoint is the body of POST /v1/webhooks.
type WebhookEndpoint struct {
	EndpointID string `json:"endpoint_id"`
	URL        string `json:"url"`
	// Secret is shown once, at first registration and at rotation. It is absent when only the URL
	// was changed.
	Secret string `json:"secret,omitempty"`
}

// WebhookSecretRotated is the body of POST /v1/webhooks/rotate-secret.
type WebhookSecretRotated struct {
	EndpointID string `json:"endpoint_id"`
	URL        string `json:"url"`
	Secret     string `json:"secret"`
	// PreviousSecretValidUntil is how long deliveries also carry X-Webhook-Signature-Prev, signed
	// with the old secret.
	PreviousSecretValidUntil Timestamp `json:"previous_secret_valid_until"`
}

// WebhookDelivery is one item of /v1/webhooks/deliveries and of GET /v1/sandbox/webhooks, which
// adds Payload and drops Sequence.
type WebhookDelivery struct {
	ID        string         `json:"id"`
	URL       string         `json:"url"`
	EventType EventType      `json:"event_type"`
	Status    DeliveryStatus `json:"status"`
	Attempts  int            `json:"attempts"`
	LastError string         `json:"last_error"`
	Sequence  *int64         `json:"sequence,omitempty"`
	CreatedAt Timestamp      `json:"created_at"`
	UpdatedAt Timestamp      `json:"updated_at"`
	// Payload is the delivered event body; the sandbox listing carries it.
	Payload map[string]any `json:"payload,omitempty"`
}

// WebhookTestResult is the body of the /v1/test-webhook routes and of
// /v1/payment/testing-webhook.
type WebhookTestResult struct {
	OK     bool `json:"ok"`
	Signed bool `json:"signed"`
	// StatusCode is absent when the receiver could not be reached; Error then says why.
	StatusCode *int   `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
	// URL and DurationMs come back from /v1/payment/testing-webhook only.
	URL        string `json:"url,omitempty"`
	DurationMs *int64 `json:"duration_ms,omitempty"`
}

// WebhookEvent is a verified webhook body. Type-switch on it to reach the concrete event:
// *PaymentEvent, *PayoutEvent or *WalletEvent.
type WebhookEvent interface {
	// Kind reports which body this is: payment, payout or wallet.
	Kind() WebhookKind
	// ID is the uuid of the object the event is about.
	ID() string
	// Seq is the global, increasing sequence number of the event; a lower one arriving later is stale.
	Seq() int64
	// Final reports whether the object reached a state nothing follows.
	Final() bool
	// IsTest reports whether this is a rehearsal delivery (Webhooks.Test, sandbox): signed like a
	// live one, but no money moved.
	IsTest() bool
}

// PaymentEvent is an invoice.<status> delivery: an invoice changed state.
type PaymentEvent struct {
	Type WebhookKind `json:"type"`
	UUID string      `json:"uuid"`
	// OrderID is the merchant reference; null when the object has none.
	OrderID  *string       `json:"order_id"`
	Status   PaymentStatus `json:"status"`
	IsFinal  bool          `json:"is_final"`
	Amount   Money         `json:"amount"`
	Currency string        `json:"currency"`
	Network  Network       `json:"network"`
	// PayerAmount is the amount due in PayerCurrency.
	PayerAmount   Money  `json:"payer_amount"`
	PayerCurrency string `json:"payer_currency"`
	// PaymentAmount is what actually landed on the address, in PayerCurrency.
	PaymentAmount            Money  `json:"payment_amount"`
	PayerAddress             string `json:"payer_address"`
	PayerAddressIsRefundable bool   `json:"payer_address_is_refundable"`
	AdditionalData           string `json:"additional_data"`
	TxID                     string `json:"txid"`
	// EventAt is when the state change was committed; order events by it, or by Sequence.
	EventAt Timestamp `json:"event_at"`
	// Sequence is global and increasing (gaps are normal).
	Sequence int64 `json:"sequence"`
	// Test is true ONLY on a rehearsal delivery (Webhooks.Test, sandbox). Such a body is signed
	// like a live one, so a handler must check this flag (or the X-Webhook-Test header) and never
	// act on a test event as if money moved.
	Test bool `json:"test,omitempty"`
}

// PayoutEvent is a payout.<status> delivery: a payout (or refund) changed state. The body is the
// payout itself.
type PayoutEvent struct {
	Type WebhookKind `json:"type"`
	UUID string      `json:"uuid"`
	// OrderID is the merchant reference; null on refund payouts.
	OrderID  *string      `json:"order_id"`
	Status   PayoutStatus `json:"status"`
	IsFinal  bool         `json:"is_final"`
	Amount   Money        `json:"amount"`
	Currency string       `json:"currency"`
	Network  Network      `json:"network"`
	Address  string       `json:"address"`
	Memo     string       `json:"memo"`
	// PayerAmount is the total debited from the balance.
	PayerAmount Money           `json:"payer_amount"`
	Commission  Money           `json:"commission"`
	FeeBearer   FeeBearerResult `json:"fee_bearer"`
	// Source is the balance the payout was funded from.
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
	// EventAt is when the state change was committed; order events by it, or by Sequence.
	EventAt Timestamp `json:"event_at"`
	// Sequence is global and increasing (gaps are normal).
	Sequence int64 `json:"sequence"`
	// Test is true ONLY on a rehearsal delivery (Webhooks.Test, sandbox). Such a body is signed
	// like a live one, so a handler must check this flag (or the X-Webhook-Test header) and never
	// act on a test event as if money moved.
	Test bool `json:"test,omitempty"`
}

// WalletEvent is a wallet.paid delivery: a deposit landed on a static wallet.
type WalletEvent struct {
	Type WebhookKind `json:"type"`
	UUID string      `json:"uuid"`
	// OrderID is the reference the wallet was created with.
	OrderID *string `json:"order_id"`
	// Status is always "paid" on this body.
	Status        string  `json:"status"`
	IsFinal       bool    `json:"is_final"`
	Address       string  `json:"address"`
	Currency      string  `json:"currency"`
	Network       Network `json:"network"`
	PayerCurrency string  `json:"payer_currency"`
	// PaymentAmount is what landed on the address, in PayerCurrency.
	PaymentAmount Money  `json:"payment_amount"`
	TxID          string `json:"txid"`
	// EventAt is when the deposit was credited; order events by it, or by Sequence.
	EventAt Timestamp `json:"event_at"`
	// Sequence is global and increasing (gaps are normal).
	Sequence int64 `json:"sequence"`
	// Test is true ONLY on a rehearsal delivery (Webhooks.Test, sandbox). Such a body is signed
	// like a live one, so a handler must check this flag (or the X-Webhook-Test header) and never
	// act on a test event as if money moved.
	Test bool `json:"test,omitempty"`
}

// Kind reports which body this is.
func (e *PaymentEvent) Kind() WebhookKind { return e.Type }

// ID is the uuid of the invoice the event is about.
func (e *PaymentEvent) ID() string { return e.UUID }

// Seq is the event's global sequence number.
func (e *PaymentEvent) Seq() int64 { return e.Sequence }

// Final reports whether the invoice reached a state nothing follows.
func (e *PaymentEvent) Final() bool { return e.IsFinal }

// IsTest reports whether this delivery is a rehearsal (Webhooks.Test, sandbox) rather than a
// real invoice: never act on it as if money moved.
func (e *PaymentEvent) IsTest() bool { return e.Test }

// Kind reports which body this is.
func (e *PayoutEvent) Kind() WebhookKind { return e.Type }

// ID is the uuid of the payout the event is about.
func (e *PayoutEvent) ID() string { return e.UUID }

// Seq is the event's global sequence number.
func (e *PayoutEvent) Seq() int64 { return e.Sequence }

// Final reports whether the payout reached a state nothing follows.
func (e *PayoutEvent) Final() bool { return e.IsFinal }

// IsTest reports whether this delivery is a rehearsal (Webhooks.Test, sandbox) rather than a
// real payout: never act on it as if money moved.
func (e *PayoutEvent) IsTest() bool { return e.Test }

// Kind reports which body this is.
func (e *WalletEvent) Kind() WebhookKind { return e.Type }

// ID is the uuid of the wallet the event is about.
func (e *WalletEvent) ID() string { return e.UUID }

// Seq is the event's global sequence number.
func (e *WalletEvent) Seq() int64 { return e.Sequence }

// Final reports whether the deposit reached a state nothing follows.
func (e *WalletEvent) Final() bool { return e.IsFinal }

// IsTest reports whether this delivery is a rehearsal (Webhooks.Test, sandbox) rather than a
// real deposit: never act on it as if money moved.
func (e *WalletEvent) IsTest() bool { return e.Test }
