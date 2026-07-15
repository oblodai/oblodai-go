package oblodai

import "context"

// ─────────────────────────────── Splits ───────────────────────────────

// SplitRuleCreated — результат создания правила сплита.
type SplitRuleCreated struct {
	RuleID  string  `json:"rule_id"`
	Percent float64 `json:"percent"`
}

// SplitRule — правило сплита. Либо Address+Network (внешний адрес, Reversible=false — возврат долю
// не отзовёт), либо MerchantID (аккаунт на платформе, Reversible=true).
type SplitRule struct {
	RuleID     string  `json:"rule_id"`
	Percent    float64 `json:"percent"`
	Active     bool    `json:"active"`
	Note       string  `json:"note,omitempty"`
	Address    string  `json:"address,omitempty"`
	Network    string  `json:"network,omitempty"`
	MerchantID string  `json:"merchant_id,omitempty"`
	Reversible bool    `json:"reversible"`
}

// SplitConfig — настройки сплитов: окно удержания исходящей маршрутизации после оплаты.
type SplitConfig struct {
	RefundHoldHours int `json:"refund_hold_hours"`
}

// SplitsResource — сплит-платежи: доля каждого входящего платежа автоматически уходит партнёру.
// Все методы требуют PAYOUT-ключ. Заголовок Idempotency-Key на эти эндпоинты не шлётся.
type SplitsResource struct{ c *Client }

// CreateRule создаёт правило сплита из свободного набора полей. POST /v1/split/rule
//
// Обязателен либо {"address","network"} (внешний адрес), либо {"merchant_id"} (партнёр на
// платформе) — ровно одно из двух; плюс "percent" (шаг 0.01, сумма активных правил ≤ 100) и
// опциональная "note". Удобные обёртки: SplitToAddress / SplitToMerchant.
func (r *SplitsResource) CreateRule(ctx context.Context, params Params) (*SplitRuleCreated, error) {
	var out SplitRuleCreated
	return &out, r.c.request(ctx, "/v1/split/rule", params, &out)
}

// SplitToAddress создаёт правило «percent% каждого платежа — на внешний адрес» (необратимо:
// возврат покупателю долю не отзовёт). POST /v1/split/rule
func (r *SplitsResource) SplitToAddress(ctx context.Context, address, network string, percent float64, note string) (*SplitRuleCreated, error) {
	body := Params{"address": address, "network": network, "percent": percent}
	if note != "" {
		body["note"] = note
	}
	return r.CreateRule(ctx, body)
}

// SplitToMerchant создаёт правило «percent% каждого платежа — аккаунту на платформе» (обратимо:
// возврат отзовёт долю). POST /v1/split/rule
func (r *SplitsResource) SplitToMerchant(ctx context.Context, merchantID string, percent float64, note string) (*SplitRuleCreated, error) {
	body := Params{"merchant_id": merchantID, "percent": percent}
	if note != "" {
		body["note"] = note
	}
	return r.CreateRule(ctx, body)
}

// ListRules возвращает правила сплита. POST /v1/split/rule/list
func (r *SplitsResource) ListRules(ctx context.Context) ([]SplitRule, error) {
	var out struct {
		Items []SplitRule `json:"items"`
	}
	if err := r.c.request(ctx, "/v1/split/rule/list", Params{}, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// DeleteRule удаляет правило сплита. POST /v1/split/rule/delete
func (r *SplitsResource) DeleteRule(ctx context.Context, ruleID string) (bool, error) {
	var out struct {
		Deleted bool `json:"deleted"`
	}
	if err := r.c.request(ctx, "/v1/split/rule/delete", Params{"rule_id": ruleID}, &out); err != nil {
		return false, err
	}
	return out.Deleted, nil
}

// GetConfig читает настройки сплитов. POST /v1/split/config/get
func (r *SplitsResource) GetConfig(ctx context.Context) (*SplitConfig, error) {
	var out SplitConfig
	return &out, r.c.request(ctx, "/v1/split/config/get", Params{}, &out)
}

// SetConfig задаёт окно удержания (в часах) исходящей маршрутизации после оплаты — в этом окне
// возврат покупателю сам уменьшает или отменяет отчисление партнёру. POST /v1/split/config/set
func (r *SplitsResource) SetConfig(ctx context.Context, refundHoldHours int) (*SplitConfig, error) {
	var out SplitConfig
	return &out, r.c.request(ctx, "/v1/split/config/set", Params{"refund_hold_hours": refundHoldHours}, &out)
}
