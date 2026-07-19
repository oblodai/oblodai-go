package oblodai

import (
	"context"
	"encoding/json"
	"strings"
)

// ─────────────────────────────── Sandbox ───────────────────────────────

// Песочница разработчика. Тестовые ключи (public_id с префиксом "test_", секрет с префиксом
// "oblodai_test_") работают со ВСЕМИ бизнес-эндпоинтами без изменений — между тестом и продом
// меняется только ключ, интеграционный код одинаков. Методы этого ресурса — пять test-only
// помощников, которых в проде нет: они заменяют «покупатель оплатил в сети». Живой ключ получает
// на них HTTP 403 с кодом sandbox.live_key.

// IsTestKey сообщает, является ли public_id тестовым (ключ песочницы): префикс "test_".
func IsTestKey(publicID string) bool {
	return strings.HasPrefix(publicID, "test_")
}

// SandboxDepositParams — параметры симуляции он-чейн депозита в инвойс.
type SandboxDepositParams struct {
	// InvoiceID — uuid инвойса. Обязателен.
	InvoiceID string
	// Amount — сумма «оплаты». Пусто — оплатить ровно столько, сколько должно; другое значение —
	// недоплата или переплата.
	Amount string
	// Confirmations — число подтверждений. 0 — сразу полностью подтверждён; малое число —
	// депозит «висит» неподтверждённым. Повтор того же TxID с бОльшим числом углубляет депозит.
	Confirmations int
	// TxID — идентификатор транзакции. Пусто — песочница сгенерирует новый; повторите тот же
	// TxID для тестов идемпотентности и углубления подтверждений.
	TxID string
}

// SandboxDeposit — результат симуляции депозита.
type SandboxDeposit struct {
	InvoiceID     string `json:"invoice_id"`
	TxID          string `json:"txid"`
	Amount        string `json:"amount"`
	Confirmations int    `json:"confirmations"`
}

// SandboxFaucetResult — результат «крана» тестового баланса.
type SandboxFaucetResult struct {
	Asset     string `json:"asset"`
	Amount    string `json:"amount"`
	JournalID string `json:"journal_id"`
}

// SandboxResetResult — результат сброса песочницы.
type SandboxResetResult struct {
	InvoicesCancelled int `json:"invoices_cancelled"`
	BalancesZeroed    int `json:"balances_zeroed"`
}

// SandboxDelivery — запись журнала доставок вебхуков песочницы: как Delivery, плюс сырое тело
// события (Payload — байт-в-байт; разберите его сами при необходимости).
type SandboxDelivery struct {
	ID        string          `json:"id"`
	EventType string          `json:"event_type"`
	URL       string          `json:"url"`
	Status    string          `json:"status"`
	Attempts  int             `json:"attempts"`
	LastError string          `json:"last_error"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

// SandboxReplayResult — результат повторной постановки доставки в очередь.
type SandboxReplayResult struct {
	DeliveryID string `json:"delivery_id"`
	Requeued   bool   `json:"requeued"`
}

// SandboxResource — test-only помощники песочницы. ТОЛЬКО ДЛЯ ТЕСТОВОГО КОДА: не вызывайте эти
// методы из продовой интеграции — с живым ключом они всегда отвечают 403 sandbox.live_key.
type SandboxResource struct{ c *Client }

// SimulateDeposit симулирует он-чейн депозит в инвойс. POST /v1/sandbox/deposit
//
// Пустой Amount — оплатить ровно сумму инвойса; Confirmations 0 — сразу подтверждён. Депозит с
// малым числом подтверждений дозревает сам (~10 минут) — либо повторите тот же TxID с бОльшим
// Confirmations, чтобы углубить его немедленно.
func (r *SandboxResource) SimulateDeposit(ctx context.Context, params SandboxDepositParams) (*SandboxDeposit, error) {
	body := Params{"invoice_id": params.InvoiceID}
	if params.Amount != "" {
		body["amount"] = params.Amount
	}
	if params.Confirmations != 0 {
		body["confirmations"] = params.Confirmations
	}
	if params.TxID != "" {
		body["txid"] = params.TxID
	}
	var out SandboxDeposit
	return &out, r.c.request(ctx, "/v1/sandbox/deposit", body, &out)
}

// Faucet начисляет тестовый баланс (потолок 1000000 за вызов). POST /v1/sandbox/faucet
func (r *SandboxResource) Faucet(ctx context.Context, asset, amount string) (*SandboxFaucetResult, error) {
	return r.faucet(ctx, asset, amount, "")
}

// FaucetWithKey — как Faucet, но с ключом идемпотентности. Ключ уходит ПОЛЕМ ТЕЛА
// idempotency_key (эндпоинт песочницы дедуплицирует сам; заголовок Idempotency-Key
// здесь не используется).
func (r *SandboxResource) FaucetWithKey(ctx context.Context, asset, amount, idempotencyKey string) (*SandboxFaucetResult, error) {
	return r.faucet(ctx, asset, amount, idempotencyKey)
}

func (r *SandboxResource) faucet(ctx context.Context, asset, amount, idempotencyKey string) (*SandboxFaucetResult, error) {
	body := Params{"asset": asset, "amount": amount}
	if idempotencyKey != "" {
		body["idempotency_key"] = idempotencyKey
	}
	var out SandboxFaucetResult
	return &out, r.c.request(ctx, "/v1/sandbox/faucet", body, &out)
}

// Reset сбрасывает песочницу: отменяет открытые инвойсы и обнуляет балансы (компенсирующая
// проводка). POST /v1/sandbox/reset
func (r *SandboxResource) Reset(ctx context.Context) (*SandboxResetResult, error) {
	var out SandboxResetResult
	return &out, r.c.request(ctx, "/v1/sandbox/reset", Params{}, &out)
}

// ListWebhooks возвращает последние доставки вебхуков песочницы (до 50, новые первыми).
// GET /v1/sandbox/webhooks
//
// Единственный подписанный GET в API: подпись считается по той же канонической строке
// "{ts}\nGET\n{path}\n" с ПУСТЫМ телом.
func (r *SandboxResource) ListWebhooks(ctx context.Context) ([]SandboxDelivery, error) {
	var wrapper struct {
		Deliveries []SandboxDelivery `json:"deliveries"`
	}
	if err := r.c.requestSignedGET(ctx, "/v1/sandbox/webhooks", &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Deliveries, nil
}

// ReplayWebhook повторно ставит одну доставку в очередь. POST /v1/sandbox/webhooks/replay
func (r *SandboxResource) ReplayWebhook(ctx context.Context, deliveryID string) (*SandboxReplayResult, error) {
	var out SandboxReplayResult
	return &out, r.c.request(ctx, "/v1/sandbox/webhooks/replay", Params{"delivery_id": deliveryID}, &out)
}
