package oblodai

import (
	"context"
	"net/url"
)

// ─────────────────────────────── Payment links ───────────────────────────────

// LinkParams — параметры создания платёжной ссылки (Links.Create).
type LinkParams struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	AmountMode  string `json:"amount_mode"` // "fixed" | "open" | "range"
	Currency    string `json:"currency"`    // валюта ЦЕНЫ (фиат или монета)
	AmountFixed string `json:"amount_fixed,omitempty"`
	AmountMin   string `json:"amount_min,omitempty"`
	AmountMax   string `json:"amount_max,omitempty"`
	// PinnedCurrency/PinnedNetwork закрепляют валюту и сеть расчёта; пустые — выберет плательщик.
	PinnedCurrency string `json:"pinned_currency,omitempty"`
	PinnedNetwork  string `json:"pinned_network,omitempty"`
	// ExpiresIn — срок жизни ссылки в СЕКУНДАХ; 0 — бессрочно.
	ExpiresIn int64 `json:"expires_in,omitempty"`
}

// PaymentLinkCreated — результат создания платёжной ссылки.
type PaymentLinkCreated struct {
	LinkID string `json:"link_id"`
	URL    string `json:"url"` // публичная страница ссылки
}

// PaymentLink — платёжная ссылка (linkView).
type PaymentLink struct {
	LinkID         string `json:"link_id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	AmountMode     string `json:"amount_mode"`
	Currency       string `json:"currency"`
	Active         bool   `json:"active"`
	URL            string `json:"url"`
	CreatedAt      string `json:"created_at"`
	AmountFixed    string `json:"amount_fixed,omitempty"`
	AmountMin      string `json:"amount_min,omitempty"`
	AmountMax      string `json:"amount_max,omitempty"`
	PinnedCurrency string `json:"pinned_currency,omitempty"`
	PinnedNetwork  string `json:"pinned_network,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
}

// LinkPayment — платёж, созданный по ссылке (в ответе Links.Info).
type LinkPayment struct {
	UUID      string `json:"uuid"`
	Status    string `json:"status"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	CreatedAt string `json:"created_at"`
	OrderID   string `json:"order_id,omitempty"`
}

// PaymentLinkInfo — ссылка вместе с платежами по ней.
type PaymentLinkInfo struct {
	PaymentLink
	Payments []LinkPayment `json:"payments"`
}

// PaymentLinkToggle — результат Links.Toggle.
type PaymentLinkToggle struct {
	LinkID string `json:"link_id"`
	Active bool   `json:"active"`
}

// LinksResource — переиспользуемые платёжные ссылки: по одной ссылке платят много людей, каждый
// платёж — отдельный инвойс. Management-методы (Create/List/Info/Toggle) подписываются платёжным
// ключом; PublicGet/Checkout — публичные (без подписи), их зовёт страница оплаты.
type LinksResource struct{ c *Client }

// Create создаёт платёжную ссылку. POST /v1/payment/link
//
// Дедуп-заголовок Idempotency-Key на этот эндпоинт НЕ шлётся (создание ссылки не двигает деньги).
func (r *LinksResource) Create(ctx context.Context, params LinkParams) (*PaymentLinkCreated, error) {
	var out PaymentLinkCreated
	return &out, r.c.request(ctx, "/v1/payment/link", params, &out)
}

// List возвращает страницу ссылок мерчанта (created_at DESC). POST /v1/payment/link/list
func (r *LinksResource) List(ctx context.Context, limit, offset int) ([]PaymentLink, error) {
	var out struct {
		Items []PaymentLink `json:"items"`
	}
	if err := r.c.request(ctx, "/v1/payment/link/list", Params{"limit": limit, "offset": offset}, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// Info возвращает ссылку и платежи по ней. POST /v1/payment/link/info
func (r *LinksResource) Info(ctx context.Context, linkID string) (*PaymentLinkInfo, error) {
	var out PaymentLinkInfo
	return &out, r.c.request(ctx, "/v1/payment/link/info", Params{"link_id": linkID}, &out)
}

// Toggle включает/выключает ссылку. POST /v1/payment/link/toggle
func (r *LinksResource) Toggle(ctx context.Context, linkID string, active bool) (*PaymentLinkToggle, error) {
	var out PaymentLinkToggle
	return &out, r.c.request(ctx, "/v1/payment/link/toggle", Params{"link_id": linkID, "active": active}, &out)
}

// PublicGet возвращает публичные детали ссылки (для страницы оплаты). GET /v1/link/{id} — БЕЗ подписи.
func (r *LinksResource) PublicGet(ctx context.Context, linkID string) (*PaymentLink, error) {
	var out PaymentLink
	return &out, r.c.requestPublicGET(ctx, "/v1/link/"+url.PathEscape(linkID), &out)
}

// Checkout создаёт инвойс по публичной ссылке. POST /v1/link/{id}/checkout — БЕЗ подписи.
//
// params: "amount", "currency", "network", "payer_email" (закреплённые в ссылке валюта/сеть
// побеждают). Лимит шлюза: 30 инвойсов/мин на ссылку (paylink.rate_limited). Ответ — обычный
// объект платежа (uuid + url страницы оплаты).
func (r *LinksResource) Checkout(ctx context.Context, linkID string, params Params) (*Payment, error) {
	var out Payment
	return &out, r.c.requestPublic(ctx, "/v1/link/"+url.PathEscape(linkID)+"/checkout", params, &out)
}
