package oblodai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// newIdempotencyKey генерирует стабильный ключ идемпотентности вида "idem-<32 hex>"
// из 16 случайных байт (crypto/rand). Без внешних зависимостей.
func newIdempotencyKey() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // rand.Read из crypto/rand не возвращает частичного чтения
	return "idem-" + hex.EncodeToString(b[:])
}

// hasOrderID сообщает, задан ли в params «настоящий» order_id: значение должно быть строкой,
// непустой после обрезки пробелов. Отсутствие, nil, "", "   " и любое не-строковое значение
// считаются отсутствием (и приведут к вставке сгенерированного ключа).
func hasOrderID(params Params) bool {
	v, ok := params["order_id"]
	if !ok {
		return false
	}
	s, isStr := v.(string)
	if !isStr {
		return false
	}
	return strings.TrimSpace(s) != ""
}

// withOrderID возвращает ПОВЕРХНОСТНУЮ КОПИЮ params с гарантированным непустым order_id.
// Исходную карту вызывающего НЕ мутируем: повторное использование одной карты в двух вызовах
// Create/TransferToPersonal иначе протекло бы order_id из первого вызова во второй, и бэкенд
// схлопнул бы две операции в одну по дедупу. Копию делаем ОДИН раз, до цикла повторов, чтобы все
// попытки слали ОДИН И ТОТ ЖЕ order_id и бэкенд дедуплицировал повтор неидемпотентного POST.
func withOrderID(params Params) Params {
	out := make(Params, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	if !hasOrderID(out) {
		out["order_id"] = newIdempotencyKey()
	}
	return out
}

// lookup собирает тело запроса по uuid или order_id (нужен хотя бы один).
func lookup(uuid, orderID string) Params {
	p := Params{}
	if uuid != "" {
		p["uuid"] = uuid
	}
	if orderID != "" {
		p["order_id"] = orderID
	}
	return p
}

// ─────────────────────────────── Payments ───────────────────────────────

// PaymentsResource — методы приёма платежей.
type PaymentsResource struct{ c *Client }

// Create создаёт платёжный счёт (инвойс). POST /v1/payment
func (r *PaymentsResource) Create(ctx context.Context, params Params) (*Payment, error) {
	// Авто-ключ идемпотентности: без непустого order_id повтор неидемпотентного POST создал бы
	// второй платёж. Вставляем стабильный order_id в КОПИЮ до цикла повторов (карту вызывающего не
	// мутируем) — бэкенд дедуплицирует по нему.
	body := withOrderID(params)
	var out Payment
	return &out, r.c.request(ctx, "/v1/payment", body, &out)
}

// Info возвращает информацию о счёте по uuid или order_id. POST /v1/payment/info
func (r *PaymentsResource) Info(ctx context.Context, uuid, orderID string) (*Payment, error) {
	var out Payment
	return &out, r.c.request(ctx, "/v1/payment/info", lookup(uuid, orderID), &out)
}

// History возвращает список платежей. POST /v1/payment/history
func (r *PaymentsResource) History(ctx context.Context, params Params) (*PaymentList, error) {
	var out PaymentList
	return &out, r.c.request(ctx, "/v1/payment/history", params, &out)
}

// Services возвращает доступные методы приёма. POST /v1/payment/services
func (r *PaymentsResource) Services(ctx context.Context) ([]ServiceMethod, error) {
	var out []ServiceMethod
	return out, r.c.request(ctx, "/v1/payment/services", Params{}, &out)
}

// QR возвращает QR-код депозит-адреса счёта. POST /v1/payment/qr
func (r *PaymentsResource) QR(ctx context.Context, uuid, orderID string) (map[string]string, error) {
	var out map[string]string
	return out, r.c.request(ctx, "/v1/payment/qr", lookup(uuid, orderID), &out)
}

// Resend переотправляет текущий вебхук платежа. POST /v1/payment/resend
func (r *PaymentsResource) Resend(ctx context.Context, uuid, orderID string) error {
	return r.c.request(ctx, "/v1/payment/resend", lookup(uuid, orderID), nil)
}

// Refund выполняет возврат средств платежа. POST /v1/payment/refund
func (r *PaymentsResource) Refund(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payment/refund", params, &out)
}

// ListAccepted возвращает набор принимаемых валют. POST /v1/payment/accepted/list
func (r *PaymentsResource) ListAccepted(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payment/accepted/list", Params{}, &out)
}

// SetAccepted заменяет набор принимаемых валют. POST /v1/payment/accepted/set
func (r *PaymentsResource) SetAccepted(ctx context.Context, accepted []AcceptedMethod) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payment/accepted/set", Params{"accepted": accepted}, &out)
}

// GetAccuracy читает допуск недоплаты. POST /v1/payment/accuracy/get
func (r *PaymentsResource) GetAccuracy(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payment/accuracy/get", Params{}, &out)
}

// SetAccuracy задаёт допуск недоплаты. POST /v1/payment/accuracy/set
func (r *PaymentsResource) SetAccuracy(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payment/accuracy/set", params, &out)
}

// GetAutorefund читает настройки автовозврата. POST /v1/payment/autorefund/get
func (r *PaymentsResource) GetAutorefund(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payment/autorefund/get", Params{}, &out)
}

// SetAutorefund задаёт настройки автовозврата. POST /v1/payment/autorefund/set
func (r *PaymentsResource) SetAutorefund(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payment/autorefund/set", params, &out)
}

// SetDiscount задаёт скидку/наценку. POST /v1/payment/discount/set
func (r *PaymentsResource) SetDiscount(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payment/discount/set", params, &out)
}

// ListDiscounts возвращает настроенные скидки/наценки. POST /v1/payment/discount/list
func (r *PaymentsResource) ListDiscounts(ctx context.Context) ([]map[string]any, error) {
	var out []map[string]any
	return out, r.c.request(ctx, "/v1/payment/discount/list", map[string]any{}, &out)
}

// ─────────────────────────────── Payouts ───────────────────────────────

// PayoutsResource — методы выплат и возвратов.
type PayoutsResource struct{ c *Client }

// Create создаёт выплату на внешний адрес. POST /v1/payout
func (r *PayoutsResource) Create(ctx context.Context, params Params) (*Payout, error) {
	var out Payout
	return &out, r.c.request(ctx, "/v1/payout", params, &out)
}

// CreateMass выполняет массовую выплату (до 100). POST /v1/payout/mass
func (r *PayoutsResource) CreateMass(ctx context.Context, payouts []Params, source string) (*MassPayoutResult, error) {
	body := Params{"payouts": payouts}
	if source != "" {
		body["source"] = source
	}
	var out MassPayoutResult
	return &out, r.c.request(ctx, "/v1/payout/mass", body, &out)
}

// Info возвращает информацию о выплате. POST /v1/payout/info
func (r *PayoutsResource) Info(ctx context.Context, uuid, orderID string) (*Payout, error) {
	var out Payout
	return &out, r.c.request(ctx, "/v1/payout/info", lookup(uuid, orderID), &out)
}

// History возвращает историю выплат. POST /v1/payout/history
func (r *PayoutsResource) History(ctx context.Context, params Params) (*PayoutList, error) {
	var out PayoutList
	return &out, r.c.request(ctx, "/v1/payout/history", params, &out)
}

// Services возвращает доступные методы выплат. POST /v1/payout/services
func (r *PayoutsResource) Services(ctx context.Context) ([]ServiceMethod, error) {
	var out []ServiceMethod
	return out, r.c.request(ctx, "/v1/payout/services", Params{}, &out)
}

// Calculate выполняет предрасчёт выплаты. POST /v1/payout/calculate
func (r *PayoutsResource) Calculate(ctx context.Context, params Params) (*PayoutCalculation, error) {
	var out PayoutCalculation
	return &out, r.c.request(ctx, "/v1/payout/calculate", params, &out)
}

// Approve подтверждает выплату в статусе pending. POST /v1/payout/approve
func (r *PayoutsResource) Approve(ctx context.Context, uuid string) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payout/approve", Params{"uuid": uuid}, &out)
}

// Refund выполняет возврат средств платежа (движок выплат). POST /v1/payment/refund
func (r *PayoutsResource) Refund(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payment/refund", params, &out)
}

// GetFeeConfig читает, кто платит сетевую комиссию выплаты. POST /v1/payout/fee-config/get
func (r *PayoutsResource) GetFeeConfig(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payout/fee-config/get", Params{}, &out)
}

// SetFeeConfig задаёт, кто платит сетевую комиссию выплаты. POST /v1/payout/fee-config/set
func (r *PayoutsResource) SetFeeConfig(ctx context.Context, feeOnRecipient bool) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payout/fee-config/set", Params{"fee_on_recipient": feeOnRecipient}, &out)
}

// GetRefundFeeConfig читает, кто несёт нашу комиссию при возврате. POST /v1/payout/refund-fee-config/get
func (r *PayoutsResource) GetRefundFeeConfig(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payout/refund-fee-config/get", Params{}, &out)
}

// SetRefundFeeConfig задаёт, кто несёт нашу комиссию при возврате. POST /v1/payout/refund-fee-config/set
func (r *PayoutsResource) SetRefundFeeConfig(ctx context.Context, feeOnCustomer bool) (map[string]any, error) {
	var out map[string]any
	return out, r.c.request(ctx, "/v1/payout/refund-fee-config/set", Params{"fee_on_customer": feeOnCustomer}, &out)
}
