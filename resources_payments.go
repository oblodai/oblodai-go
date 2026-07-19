package oblodai

import (
	"context"
	"net/url"
)

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
//
// Защита от дублей при повторах — заголовок Idempotency-Key: SDK генерирует UUID один раз до цикла
// повторов, все ретраи шлют один и тот же ключ. order_id уходит КАК ЕСТЬ — SDK его не подставляет
// и не переписывает (ломающее изменение v1.1.0: раньше при пустом order_id SDK вставлял свой).
// Свой ключ идемпотентности можно передать полем params["idempotency_key"] — оно уйдёт в заголовок,
// не в тело.
func (r *PaymentsResource) Create(ctx context.Context, params Params) (*Payment, error) {
	var out Payment
	return &out, r.c.requestIdem(ctx, "/v1/payment", params, &out)
}

// CreateBatch ставит в обработку пачку платежей (до 5000) одним подписанным запросом.
// POST /v1/payment/batch
//
// Каждый элемент — тело обычного Payments.Create; order_id ОБЯЗАТЕЛЕН на каждом элементе
// (batch.order_id_required) и уникален внутри пачки. onError: "continue" (по умолчанию) или "stop".
// Обработка фоновая: результаты по каждому элементу — через Batches.Info(batchID, ...).
// Запрос идёт с заголовком Idempotency-Key (стабилен между повторами).
func (r *PaymentsResource) CreateBatch(ctx context.Context, payments []Params, onError string) (*BatchSubmission, error) {
	body := Params{"payments": payments}
	if onError != "" {
		body["on_error"] = onError
	}
	var out BatchSubmission
	return &out, r.c.requestIdem(ctx, "/v1/payment/batch", body, &out)
}

// RefundBatch ставит в обработку пачку возвратов (до 5000). POST /v1/refund/batch
//
// Каждый элемент — тело обычного Payments.Refund; обязательны reference (batch.reference_required)
// и uuid/order_id инвойса (batch.invoice_required); дедуп внутри пачки — по (инвойс, reference).
// onError: "continue" (по умолчанию) или "stop". Требует PAYOUT-ключ. Запрос идёт с заголовком
// Idempotency-Key (стабилен между повторами).
func (r *PaymentsResource) RefundBatch(ctx context.Context, refunds []Params, onError string) (*BatchSubmission, error) {
	body := Params{"refunds": refunds}
	if onError != "" {
		body["on_error"] = onError
	}
	var out BatchSubmission
	return &out, r.c.requestIdem(ctx, "/v1/refund/batch", body, &out)
}

// SendEmail отправляет покупателю письмо-счёт с кнопкой «Оплатить». POST /v1/payment/send-email
//
// Платёж ищется по uuid или orderID (нужен хотя бы один). email — получатель; пустая строка —
// использовать payer_email платежа (если и он пуст — email.no_recipient). Лимит шлюза: 10 писем/час
// на адрес получателя (email.rate_limited).
func (r *PaymentsResource) SendEmail(ctx context.Context, uuid, orderID, email string) (*SendEmailResult, error) {
	body := lookup(uuid, orderID)
	if email != "" {
		body["email"] = email
	}
	var out SendEmailResult
	return &out, r.c.request(ctx, "/v1/payment/send-email", body, &out)
}

// Resolve решает судьбу НЕДОПЛАЧЕННОГО платежа (статус wrong_amount). POST /v1/payment/resolve
//
// action: "accept" — оставить частичную оплату себе (глушит авто-возврат) или "refund" — вернуть
// плательщику. Платёж ищется по uuid или orderID (нужен хотя бы один). Для refund в opts можно
// передать "address" (по умолчанию — адрес плательщика), "network" (по умолчанию — сеть инвойса) и
// "reference" (per-refund ключ дедупликации); opts может быть nil. Требует PAYOUT-ключ.
// Запрос идёт с заголовком Idempotency-Key (стабилен между повторами).
func (r *PaymentsResource) Resolve(ctx context.Context, uuid, orderID, action string, opts Params) (*Resolution, error) {
	body := make(Params, len(opts)+3)
	for k, v := range opts {
		body[k] = v
	}
	if uuid != "" {
		body["uuid"] = uuid
	}
	if orderID != "" {
		body["order_id"] = orderID
	}
	body["action"] = action
	var out Resolution
	return &out, r.c.requestIdem(ctx, "/v1/payment/resolve", body, &out)
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
//
// Запрос идёт с заголовком Idempotency-Key (стабилен между повторами); свой ключ — в
// params["idempotency_key"].
func (r *PaymentsResource) Refund(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.requestIdem(ctx, "/v1/payment/refund", params, &out)
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

// PublicPayment — публичное состояние инвойса для своей страницы оплаты (Payments.PublicGet).
// Это Payment без мерчант-приватных полей (additional_data, payer_email, payer_address — бэкенд их
// вырезает). Для валюто-агностичного инвойса, ждущего выбора метода, заполнен Accepted — пары
// валюта+сеть, из которых плательщик может выбрать (Payments.PublicSelect).
type PublicPayment struct {
	Payment
	Accepted []AcceptedMethod `json:"accepted,omitempty"`
}

// PublicGet возвращает публичное состояние инвойса — для СВОЕЙ страницы оплаты: рендер и поллинг
// статуса без секрета мерчанта в браузере. GET /v1/pay/{id} — ПУБЛИЧНЫЙ, запрос уходит БЕЗ подписи.
func (r *PaymentsResource) PublicGet(ctx context.Context, uuid string) (*PublicPayment, error) {
	var out PublicPayment
	return &out, r.c.requestPublicGET(ctx, "/v1/pay/"+url.PathEscape(uuid), &out)
}

// PublicSelect выбирает валюту и сеть оплаты валюто-агностичного инвойса: фиксирует курс, выделяет
// депозит-адрес и возвращает финализированный инвойс. POST /v1/pay/{id}/select — ПУБЛИЧНЫЙ, без
// подписи. Пара (currency, network) должна входить в принимаемый набор мерчанта (Accepted из
// PublicGet); повторный выбор даёт pay.not_selectable.
func (r *PaymentsResource) PublicSelect(ctx context.Context, uuid, currency, network string) (*Payment, error) {
	var out Payment
	return &out, r.c.requestPublic(ctx, "/v1/pay/"+url.PathEscape(uuid)+"/select",
		Params{"currency": currency, "network": network}, &out)
}

// ─────────────────────────────── Payouts ───────────────────────────────

// PayoutsResource — методы выплат и возвратов.
type PayoutsResource struct{ c *Client }

// Create создаёт выплату на внешний адрес. POST /v1/payout
//
// order_id обязателен (его задаёте вы — payout.order_id_required). Запрос идёт с заголовком
// Idempotency-Key (стабилен между повторами); свой ключ — в params["idempotency_key"].
func (r *PayoutsResource) Create(ctx context.Context, params Params) (*Payout, error) {
	var out Payout
	return &out, r.c.requestIdem(ctx, "/v1/payout", params, &out)
}

// CreateMass выполняет массовую выплату (до 100). POST /v1/payout/mass
//
// Запрос идёт с заголовком Idempotency-Key (стабилен между повторами).
func (r *PayoutsResource) CreateMass(ctx context.Context, payouts []Params, source string) (*MassPayoutResult, error) {
	body := Params{"payouts": payouts}
	if source != "" {
		body["source"] = source
	}
	var out MassPayoutResult
	return &out, r.c.requestIdem(ctx, "/v1/payout/mass", body, &out)
}

// CreateBatch ставит в обработку пачку выплат (до 5000) одним подписанным запросом.
// POST /v1/payout/batch
//
// Каждый элемент — тело обычного Payouts.Create; order_id ОБЯЗАТЕЛЕН на каждом элементе и уникален
// внутри пачки. onError: "continue" (по умолчанию) или "stop". Обработка фоновая: результаты —
// через Batches.Info(batchID, ...). Требует PAYOUT-ключ. Запрос идёт с заголовком Idempotency-Key.
func (r *PayoutsResource) CreateBatch(ctx context.Context, payouts []Params, onError string) (*BatchSubmission, error) {
	body := Params{"payouts": payouts}
	if onError != "" {
		body["on_error"] = onError
	}
	var out BatchSubmission
	return &out, r.c.requestIdem(ctx, "/v1/payout/batch", body, &out)
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
//
// Запрос идёт с заголовком Idempotency-Key (стабилен между повторами); свой ключ — в
// params["idempotency_key"].
func (r *PayoutsResource) Refund(ctx context.Context, params Params) (map[string]any, error) {
	var out map[string]any
	return out, r.c.requestIdem(ctx, "/v1/payment/refund", params, &out)
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
