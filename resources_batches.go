package oblodai

import (
	"context"
	"encoding/json"
)

// ─────────────────────────────── Batches ───────────────────────────────

// BatchSubmission — результат постановки пачки (Payments.CreateBatch / Payments.RefundBatch /
// Payouts.CreateBatch). Обработка фоновая: прогресс и результаты — через Batches.Info.
type BatchSubmission struct {
	BatchID string `json:"batch_id"`
	Kind    string `json:"kind"`   // "payments" | "refunds" | "payouts"
	Count   int    `json:"count"`  // принято элементов
	Status  string `json:"status"` // "pending" на постановке
}

// BatchItem — результат одного элемента пачки. Result — байт-в-байт сохранённый result
// соответствующего единичного эндпоинта (разберите его в *Payment / *Payout и т.п.).
type BatchItem struct {
	Idx     int             `json:"idx"`
	Status  string          `json:"status"`
	OrderID string          `json:"order_id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// BatchInfo — состояние пачки и страница её элементов. Статусы пачки:
// "pending" → "processing" → "completed".
type BatchInfo struct {
	BatchID   string      `json:"batch_id"`
	Kind      string      `json:"kind"`
	Status    string      `json:"status"`
	OnError   string      `json:"on_error"` // "continue" | "stop"
	Total     int         `json:"total"`
	Succeeded int         `json:"succeeded"`
	Failed    int         `json:"failed"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
	Items     []BatchItem `json:"items"`
}

// BatchesResource — состояние массовых операций (пачек).
type BatchesResource struct{ c *Client }

// Info возвращает состояние пачки и страницу результатов её элементов. POST /v1/batch/info
//
// limit ≤0 или >500 бэкенд заменяет на 100. Read-only: без заголовка Idempotency-Key.
func (r *BatchesResource) Info(ctx context.Context, batchID string, limit, offset int) (*BatchInfo, error) {
	body := Params{"batch_id": batchID, "limit": limit, "offset": offset}
	var out BatchInfo
	return &out, r.c.request(ctx, "/v1/batch/info", body, &out)
}
