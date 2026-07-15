package oblodai

// Модели объектов API. Суммы — строки в единицах валюты ("25.00"), не числа, — чтобы не терять
// точность. Неописанные поля JSON игнорируются при разборе.

// ─────────────────────────────── Платежи ───────────────────────────────

// Payment — объект платежа (инвойса).
type Payment struct {
	UUID                  string `json:"uuid"`
	OrderID               string `json:"order_id"`
	Amount                string `json:"amount"`
	PaymentAmount         string `json:"payment_amount"`
	AmountPaid            string `json:"amount_paid"`
	AmountRemaining       string `json:"amount_remaining"`
	PayerAmount           string `json:"payer_amount"`
	PayerCurrency         string `json:"payer_currency"`
	Currency              string `json:"currency"`
	Network               string `json:"network"`
	Address               string `json:"address"`
	AddressQRCode         string `json:"address_qr_code"`
	PaymentStatus         string `json:"payment_status"`
	IsMulti               bool   `json:"is_multi"`
	URL                   string `json:"url"`
	ExpiredAt             int64  `json:"expired_at"`
	IsFinal               bool   `json:"is_final"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
	AdditionalData        string `json:"additional_data"`
	PayerEmail            string `json:"payer_email"`
	URLReturn             string `json:"url_return"`
	URLSuccess            string `json:"url_success"`
	RateExpiresAt         int64  `json:"rate_expires_at"`
	Confirmations         int    `json:"confirmations"`
	RequiredConfirmations int    `json:"required_confirmations"`
	TxID                  string `json:"txid"`
	// v1.1.0:
	PayerAddress string           `json:"payer_address,omitempty"` // адрес, с которого пришли деньги
	RefundStatus string           `json:"refund_status,omitempty"` // "none" | "partial" | "full"
	Refunds      []map[string]any `json:"refunds,omitempty"`       // выполненные возвраты по платежу
}

// Paginate — пагинация в списковых ответах.
type Paginate struct {
	Count   int `json:"count"`
	PerPage int `json:"per_page"`
	Offset  int `json:"offset"`
}

// PaymentList — страница списка платежей.
type PaymentList struct {
	Items    []Payment `json:"items"`
	Paginate Paginate  `json:"paginate"`
}

// Limit — лимиты метода.
type Limit struct {
	MinAmount string `json:"min_amount"`
	MaxAmount string `json:"max_amount"`
}

// Commission — комиссия метода.
type Commission struct {
	FeeAmount string `json:"fee_amount"`
	Percent   string `json:"percent"`
}

// ServiceMethod — доступный метод приёма/выплат.
type ServiceMethod struct {
	Network     string     `json:"network"`
	Currency    string     `json:"currency"`
	IsAvailable bool       `json:"is_available"`
	Limit       Limit      `json:"limit"`
	Commission  Commission `json:"commission"`
}

// ─────────────────────────────── Кошельки ───────────────────────────────

// Wallet — статический кошелёк.
type Wallet struct {
	UUID     string `json:"uuid"`
	Address  string `json:"address"`
	Network  string `json:"network"`
	Currency string `json:"currency"`
	OrderID  string `json:"order_id"`
	URL      string `json:"url"`
}

// MerchantBalance — баланс по одной валюте.
type MerchantBalance struct {
	Currency string `json:"currency"`
	Balance  string `json:"balance"`
}

// Balance — доступные балансы мерчанта.
type Balance struct {
	Merchant []MerchantBalance `json:"merchant"`
}

// ReferralInfo — реферальная статистика.
type ReferralInfo struct {
	Code            string            `json:"code"`
	Link            string            `json:"link"`
	TierBps         []int             `json:"tier_bps"`
	ReferredCount   int               `json:"referred_count"`
	EarningsByAsset map[string]string `json:"earnings_by_asset"`
}

// ─────────────────────────────── Выплаты ───────────────────────────────

// PayoutConvert — данные конвертации (при from_currency).
type PayoutConvert struct {
	FromCurrency string `json:"from_currency"`
	ToCurrency   string `json:"to_currency"`
	FromAmount   string `json:"from_amount"`
	Rate         string `json:"rate"`
}

// Payout — объект выплаты.
type Payout struct {
	UUID             string         `json:"uuid"`
	OrderID          string         `json:"order_id"`
	Amount           string         `json:"amount"`
	Currency         string         `json:"currency"`
	Network          string         `json:"network"`
	Address          string         `json:"address"`
	TxID             string         `json:"txid"`
	Status           string         `json:"status"`
	IsFinal          bool           `json:"is_final"`
	ApprovalRequired bool           `json:"approval_required"`
	Source           string         `json:"source"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at"`
	Convert          *PayoutConvert `json:"convert,omitempty"`
}

// MassPayoutItem — элемент результата массовой выплаты.
type MassPayoutItem struct {
	OrderID          string `json:"order_id"`
	Success          bool   `json:"success"`
	UUID             string `json:"uuid,omitempty"`
	Status           string `json:"status,omitempty"`
	IsFinal          bool   `json:"is_final,omitempty"`
	ApprovalRequired bool   `json:"approval_required,omitempty"`
	Message          string `json:"message,omitempty"`
}

// MassPayoutResult — результат массовой выплаты.
type MassPayoutResult struct {
	Items []MassPayoutItem `json:"items"`
}

// PayoutList — страница истории выплат.
type PayoutList struct {
	Items    []Payout `json:"items"`
	Paginate Paginate `json:"paginate"`
}

// PayoutCalculation — предрасчёт выплаты.
type PayoutCalculation struct {
	Amount         string `json:"amount"`
	Currency       string `json:"currency"`
	Network        string `json:"network"`
	Commission     string `json:"commission"`
	MerchantAmount string `json:"merchant_amount"`
	ToAmount       string `json:"to_amount"`
}

// ─────────────────────────────── Курсы ───────────────────────────────

// ExchangeRate — котировка валюты к USDT.
type ExchangeRate struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Course string `json:"course"`
}

// CurrencyNetwork — сеть в публичном каталоге GET /v1/currencies.
type CurrencyNetwork struct {
	Network          string `json:"network"`
	Kind             string `json:"kind"`     // "native" | "token"
	Contract         string `json:"contract"` // адрес контракта токена (для "token")
	MinConfirmations int    `json:"min_confirmations"`
	Available        bool   `json:"available"` // синоним DepositAvailable
	DepositAvailable bool   `json:"deposit_available"`
	PayoutAvailable  bool   `json:"payout_available"`
}

// Currency — актив в публичном каталоге GET /v1/currencies.
type Currency struct {
	Symbol   string            `json:"symbol"`
	Decimals int               `json:"decimals"`
	Networks []CurrencyNetwork `json:"networks"`
}

// ─────────────────────────────── Вебхуки/настройки ───────────────────────────────

// WebhookRegistration — результат регистрации endpoint вебхуков.
type WebhookRegistration struct {
	EndpointID string `json:"endpoint_id"`
	URL        string `json:"url"`
	Secret     string `json:"secret"`
}

// Delivery — запись журнала доставок.
type Delivery struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	EventType string `json:"event_type"`
	Status    string `json:"status"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AcceptedMethod — принимаемая пара валюта+сеть.
type AcceptedMethod struct {
	Currency string `json:"currency"`
	Network  string `json:"network"`
}

// AutoWithdrawRule — правило автовывода.
type AutoWithdrawRule struct {
	Currency string `json:"currency"`
	Network  string `json:"network"`
	Address  string `json:"address"`
	MinMinor string `json:"min_minor"`
}

// SendEmailResult — результат Payments.SendEmail.
type SendEmailResult struct {
	Sent  bool   `json:"sent"`
	Email string `json:"email"`
	UUID  string `json:"uuid"`
}

// Resolution — результат Payments.Resolve (решение по недоплаченному платежу).
// При action=accept заполнены AmountKept/Currency; при action=refund — UUID (рефанд-выплата),
// Amount, Address, Status, IsFinal.
type Resolution struct {
	PaymentUUID string `json:"payment_uuid"`
	OrderID     string `json:"order_id,omitempty"`
	Resolution  string `json:"resolution"` // "accepted" | "refunded"
	// accept:
	AmountKept string `json:"amount_kept,omitempty"`
	// refund:
	UUID    string `json:"uuid,omitempty"` // uuid рефанд-выплаты
	Amount  string `json:"amount,omitempty"`
	Address string `json:"address,omitempty"`
	Status  string `json:"status,omitempty"` // словарь статусов выплат: check/process/paid/fail/cancel
	IsFinal bool   `json:"is_final,omitempty"`
	// общие:
	Currency string `json:"currency,omitempty"`
}

// Params — универсальная карта параметров запроса для методов со свободным набором полей.
type Params map[string]any
