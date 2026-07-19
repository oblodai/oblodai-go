package oblodai

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

// ──────────────────── Песочница (v1.2.0) ────────────────────

// SimulateDeposit: путь/метод/тело; пустые необязательные поля вырезаются; result разбирается.
func TestSandboxSimulateDeposit(t *testing.T) {
	var gotBody []byte
	var gotMethod, gotPath, gotSig string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotMethod, gotPath = r.Method, r.URL.Path
		gotSig = r.Header.Get("X-Signature")
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"invoice_id": "e0a1", "txid": "sbx-tx-1", "amount": "48.5", "confirmations": 0,
		}})
	}, nil)
	defer srv.Close()

	dep, err := c.Sandbox.SimulateDeposit(context.Background(), SandboxDepositParams{InvoiceID: "e0a1"})
	if err != nil || dep.InvoiceID != "e0a1" || dep.TxID != "sbx-tx-1" || dep.Amount != "48.5" || dep.Confirmations != 0 {
		t.Fatalf("SimulateDeposit: %v %+v", err, dep)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/sandbox/deposit" {
		t.Fatalf("expected POST /v1/sandbox/deposit, got %s %s", gotMethod, gotPath)
	}
	if gotSig == "" {
		t.Fatal("sandbox deposit must be signed")
	}
	var body map[string]any
	_ = json.Unmarshal(gotBody, &body)
	if body["invoice_id"] != "e0a1" {
		t.Fatalf("unexpected body: %s", gotBody)
	}
	for _, k := range []string{"amount", "confirmations", "txid"} {
		if _, ok := body[k]; ok {
			t.Fatalf("empty optional field %q must be omitted: %s", k, gotBody)
		}
	}
}

// SimulateDeposit с недоплатой и неглубокими подтверждениями: все поля уходят в тело; повтор того же
// txid с бОльшим confirmations углубляет депозит.
func TestSandboxSimulateDepositUnderpaidAndDeepen(t *testing.T) {
	var bodies [][]byte
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, b)
		var req map[string]any
		_ = json.Unmarshal(b, &req)
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"invoice_id": req["invoice_id"], "txid": "tx-fixed", "amount": "10", "confirmations": req["confirmations"],
		}})
	}, nil)
	defer srv.Close()

	ctx := context.Background()
	dep, err := c.Sandbox.SimulateDeposit(ctx, SandboxDepositParams{
		InvoiceID: "e0a1", Amount: "10", Confirmations: 1, TxID: "tx-fixed",
	})
	if err != nil || dep.Confirmations != 1 {
		t.Fatalf("shallow deposit: %v %+v", err, dep)
	}
	dep, err = c.Sandbox.SimulateDeposit(ctx, SandboxDepositParams{
		InvoiceID: "e0a1", Amount: "10", Confirmations: 6, TxID: "tx-fixed",
	})
	if err != nil || dep.Confirmations != 6 {
		t.Fatalf("deepened deposit: %v %+v", err, dep)
	}
	var first, second map[string]any
	_ = json.Unmarshal(bodies[0], &first)
	_ = json.Unmarshal(bodies[1], &second)
	if first["amount"] != "10" || first["confirmations"] != float64(1) || first["txid"] != "tx-fixed" {
		t.Fatalf("unexpected first body: %s", bodies[0])
	}
	if second["txid"] != "tx-fixed" || second["confirmations"] != float64(6) {
		t.Fatalf("unexpected second body: %s", bodies[1])
	}
}

// Faucet: путь/тело; явный ключ идемпотентности уходит ПОЛЕМ ТЕЛА, а не заголовком.
func TestSandboxFaucet(t *testing.T) {
	var bodies [][]byte
	var idemHeaders []string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, b)
		idemHeaders = append(idemHeaders, r.Header.Get("Idempotency-Key"))
		if r.URL.Path != "/v1/sandbox/faucet" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"asset": "USDT", "amount": "1000", "journal_id": "j-77",
		}})
	}, nil)
	defer srv.Close()

	ctx := context.Background()
	res, err := c.Sandbox.Faucet(ctx, "USDT", "1000")
	if err != nil || res.Asset != "USDT" || res.Amount != "1000" || res.JournalID != "j-77" {
		t.Fatalf("Faucet: %v %+v", err, res)
	}
	if _, err := c.Sandbox.FaucetWithKey(ctx, "USDT", "1000", "seed-1"); err != nil {
		t.Fatalf("FaucetWithKey: %v", err)
	}

	var plain, keyed map[string]any
	_ = json.Unmarshal(bodies[0], &plain)
	_ = json.Unmarshal(bodies[1], &keyed)
	if plain["asset"] != "USDT" || plain["amount"] != "1000" {
		t.Fatalf("unexpected faucet body: %s", bodies[0])
	}
	if _, ok := plain["idempotency_key"]; ok {
		t.Fatalf("empty idempotency_key must be omitted: %s", bodies[0])
	}
	if keyed["idempotency_key"] != "seed-1" {
		t.Fatalf("explicit key must go in the BODY: %s", bodies[1])
	}
	for i, h := range idemHeaders {
		if h != "" {
			t.Fatalf("faucet call %d must not send Idempotency-Key header, got %q", i, h)
		}
	}
}

// Reset: пустое тело {}, разбор счётчиков.
func TestSandboxReset(t *testing.T) {
	var gotBody []byte
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		if r.URL.Path != "/v1/sandbox/reset" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"invoices_cancelled": 3, "balances_zeroed": 2,
		}})
	}, nil)
	defer srv.Close()

	res, err := c.Sandbox.Reset(context.Background())
	if err != nil || res.InvoicesCancelled != 3 || res.BalancesZeroed != 2 {
		t.Fatalf("Reset: %v %+v", err, res)
	}
	if string(gotBody) != "{}" {
		t.Fatalf("Reset must send empty JSON object body, got %q", gotBody)
	}
}

// ListWebhooks: единственный подписанный GET — подпись по "{ts}\nGET\n{path}\n" с ПУСТЫМ телом;
// разбор deliveries с payload байт-в-байт.
func TestSandboxListWebhooksSignedGET(t *testing.T) {
	var gotMethod, gotTS, gotSig, gotPublicID string
	var gotBody []byte
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotTS = r.Header.Get("X-Timestamp")
		gotSig = r.Header.Get("X-Signature")
		gotPublicID = r.Header.Get("X-Public-Id")
		gotBody, _ = io.ReadAll(r.Body)
		if r.URL.Path != "/v1/sandbox/webhooks" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"deliveries": []any{map[string]any{
				"id": "d-1", "event_type": "payment", "url": "https://shop.example/hook",
				"status": "failed", "attempts": 3, "last_error": "connection refused",
				"payload":    map[string]any{"uuid": "p1", "status": "paid"},
				"created_at": "2026-07-19T10:00:00Z", "updated_at": "2026-07-19T10:05:00Z",
			}},
		}})
	}, nil)
	defer srv.Close()

	deliveries, err := c.Sandbox.ListWebhooks(context.Background())
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("ListWebhooks: %v %+v", err, deliveries)
	}
	d := deliveries[0]
	if d.ID != "d-1" || d.EventType != "payment" || d.Status != "failed" || d.Attempts != 3 || d.LastError != "connection refused" {
		t.Fatalf("unexpected delivery: %+v", d)
	}
	var payload map[string]any
	if err := json.Unmarshal(d.Payload, &payload); err != nil || payload["uuid"] != "p1" || payload["status"] != "paid" {
		t.Fatalf("payload not verbatim: %s", d.Payload)
	}

	if gotMethod != http.MethodGet {
		t.Fatalf("ListWebhooks must be GET, got %s", gotMethod)
	}
	if len(gotBody) != 0 {
		t.Fatalf("GET must have empty body, got %q", gotBody)
	}
	if gotPublicID != "pub_1" || gotTS == "" {
		t.Fatalf("signed GET must carry X-Public-Id/X-Timestamp, got %q/%q", gotPublicID, gotTS)
	}
	// Подпись — по канонической строке с ПУСТЫМ телом: ts\nGET\npath\n<пусто>.
	mac := hmac.New(sha256.New, []byte("sec_1"))
	mac.Write([]byte(gotTS + "\nGET\n/v1/sandbox/webhooks\n"))
	if want := hex.EncodeToString(mac.Sum(nil)); gotSig != want {
		t.Fatalf("signed GET signature mismatch:\n got  %s\n want %s", gotSig, want)
	}
}

// ReplayWebhook: путь/тело/разбор.
func TestSandboxReplayWebhook(t *testing.T) {
	var gotBody []byte
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		if r.URL.Path != "/v1/sandbox/webhooks/replay" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"delivery_id": "d-1", "requeued": true,
		}})
	}, nil)
	defer srv.Close()

	res, err := c.Sandbox.ReplayWebhook(context.Background(), "d-1")
	if err != nil || res.DeliveryID != "d-1" || !res.Requeued {
		t.Fatalf("ReplayWebhook: %v %+v", err, res)
	}
	var body map[string]any
	_ = json.Unmarshal(gotBody, &body)
	if body["delivery_id"] != "d-1" {
		t.Fatalf("unexpected body: %s", gotBody)
	}
}

// Живой ключ на sandbox-эндпоинте: 403 sandbox.live_key — терминальная *APIError (без повторов).
func TestSandboxLiveKeyForbidden(t *testing.T) {
	var calls int
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
			"code": "sandbox.live_key", "message": "sandbox endpoints require a test key",
		}})
	}, nil)
	defer srv.Close()

	_, err := c.Sandbox.Faucet(context.Background(), "USDT", "10")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "sandbox.live_key" || apiErr.Status != 403 {
		t.Fatalf("expected 403 sandbox.live_key, got %v", err)
	}
	if apiErr.IsRetriable() || calls != 1 {
		t.Fatalf("sandbox.live_key must be terminal (1 attempt), calls=%d", calls)
	}
}

// ──────────────────── Внутренние переводы пользователям (v1.2.0) ────────────────────

// TransferToUser: путь/метод/тело; денежный эндпоинт — подписан и идёт с заголовком
// Idempotency-Key; result разбирается в типизированный TransferToUserResult.
func TestAccountTransferToUser(t *testing.T) {
	var gotBody []byte
	var gotMethod, gotPath, gotSig, gotIdem string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotMethod, gotPath = r.Method, r.URL.Path
		gotSig = r.Header.Get("X-Signature")
		gotIdem = r.Header.Get("Idempotency-Key")
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"currency": "USDT", "amount": "25", "to_user_id": "5c3a2c1e-9d0b-4f6a-8f3d-2b1a0c9e8d7f",
			"recipient_balance": "125",
		}})
	}, nil)
	defer srv.Close()

	res, err := c.Account.TransferToUser(context.Background(), Params{
		"to_user_id": "5c3a2c1e-9d0b-4f6a-8f3d-2b1a0c9e8d7f", "amount": "25", "currency": "USDT",
		"order_id": "payroll-7",
	})
	if err != nil || res.Currency != "USDT" || res.Amount != "25" ||
		res.ToUserID != "5c3a2c1e-9d0b-4f6a-8f3d-2b1a0c9e8d7f" || res.RecipientBalance != "125" {
		t.Fatalf("TransferToUser: %v %+v", err, res)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/transfer/to-user" {
		t.Fatalf("expected POST /v1/transfer/to-user, got %s %s", gotMethod, gotPath)
	}
	if gotSig == "" {
		t.Fatal("transfer/to-user must be signed")
	}
	if gotIdem == "" {
		t.Fatal("transfer/to-user must send Idempotency-Key header (money endpoint)")
	}
	var body map[string]any
	_ = json.Unmarshal(gotBody, &body)
	if body["to_user_id"] != "5c3a2c1e-9d0b-4f6a-8f3d-2b1a0c9e8d7f" ||
		body["amount"] != "25" || body["currency"] != "USDT" || body["order_id"] != "payroll-7" {
		t.Fatalf("unexpected body: %s", gotBody)
	}
	if _, ok := body["idempotency_key"]; ok {
		t.Fatalf("idempotency_key must not leak into the body: %s", gotBody)
	}
}

// TransferBatch: путь/тело {"transfers":[...],"on_error":...}; заголовок Idempotency-Key; разбор
// BatchSubmission. Пустой onError не кладёт on_error в тело.
func TestAccountTransferBatch(t *testing.T) {
	var bodies [][]byte
	var idemHeaders []string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, b)
		idemHeaders = append(idemHeaders, r.Header.Get("Idempotency-Key"))
		if r.Method != http.MethodPost || r.URL.Path != "/v1/transfer/batch" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"batch_id": "b-42", "kind": "transfers", "count": 2, "status": "pending",
		}})
	}, nil)
	defer srv.Close()

	ctx := context.Background()
	sub, err := c.Account.TransferBatch(ctx, []Params{
		{"to_user_id": "u-1", "amount": "10", "currency": "USDT", "order_id": "s-1"},
		{"to_user_id": "u-2", "amount": "20", "currency": "USDT", "order_id": "s-2"},
	}, "continue")
	if err != nil || sub.BatchID != "b-42" || sub.Kind != "transfers" || sub.Count != 2 || sub.Status != "pending" {
		t.Fatalf("TransferBatch: %v %+v", err, sub)
	}
	if _, err := c.Account.TransferBatch(ctx, []Params{{"to_user_id": "u-3", "amount": "5", "currency": "USDT"}}, ""); err != nil {
		t.Fatalf("TransferBatch (без onError): %v", err)
	}

	var first, second map[string]any
	_ = json.Unmarshal(bodies[0], &first)
	_ = json.Unmarshal(bodies[1], &second)
	transfers, ok := first["transfers"].([]any)
	if !ok || len(transfers) != 2 || first["on_error"] != "continue" {
		t.Fatalf("unexpected first body: %s", bodies[0])
	}
	item := transfers[0].(map[string]any)
	if item["to_user_id"] != "u-1" || item["amount"] != "10" || item["order_id"] != "s-1" {
		t.Fatalf("unexpected first transfer item: %s", bodies[0])
	}
	if _, ok := second["on_error"]; ok {
		t.Fatalf("empty onError must be omitted: %s", bodies[1])
	}
	for i, h := range idemHeaders {
		if h == "" {
			t.Fatalf("transfer/batch call %d must send Idempotency-Key header", i)
		}
	}
}

// ──────────────────── Публичная страница оплаты /v1/pay (v1.2.0) ────────────────────

// PublicGet: GET /v1/pay/{id} — ПУБЛИЧНЫЙ: без X-Signature/X-Public-Id/X-Timestamp; разбор
// Payment-полей и списка accepted для инвойса в статусе выбора.
func TestPaymentsPublicGetUnsigned(t *testing.T) {
	var gotMethod, gotPath string
	var gotHeaders http.Header
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotHeaders = r.Header.Clone()
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"uuid": "inv-1", "amount": "10.00", "currency": "USD", "payment_status": "select",
			"url": "https://pay.oblodai.com/inv-1",
			"accepted": []any{
				map[string]any{"currency": "USDT", "network": "tron"},
				map[string]any{"currency": "ETH", "network": "ethereum"},
			},
		}})
	}, nil)
	defer srv.Close()

	p, err := c.Payments.PublicGet(context.Background(), "inv-1")
	if err != nil || p.UUID != "inv-1" || p.PaymentStatus != "select" || len(p.Accepted) != 2 {
		t.Fatalf("PublicGet: %v %+v", err, p)
	}
	if p.Accepted[0].Currency != "USDT" || p.Accepted[0].Network != "tron" {
		t.Fatalf("unexpected accepted: %+v", p.Accepted)
	}
	if gotMethod != http.MethodGet || gotPath != "/v1/pay/inv-1" {
		t.Fatalf("expected GET /v1/pay/inv-1, got %s %s", gotMethod, gotPath)
	}
	for _, h := range []string{"X-Signature", "X-Public-Id", "X-Timestamp"} {
		if v := gotHeaders.Get(h); v != "" {
			t.Fatalf("public GET /v1/pay/{id} must be UNSIGNED: header %s = %q", h, v)
		}
	}
}

// PublicSelect: POST /v1/pay/{id}/select — ПУБЛИЧНЫЙ (без подписи); тело {"currency","network"};
// ответ — финализированный инвойс (обычный Payment).
func TestPaymentsPublicSelectUnsigned(t *testing.T) {
	var gotBody []byte
	var gotMethod, gotPath string
	var gotHeaders http.Header
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotMethod, gotPath = r.Method, r.URL.Path
		gotHeaders = r.Header.Clone()
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"uuid": "inv-1", "payment_status": "check", "payer_currency": "USDT", "network": "tron",
			"address": "TXyz", "payer_amount": "10.02",
		}})
	}, nil)
	defer srv.Close()

	p, err := c.Payments.PublicSelect(context.Background(), "inv-1", "USDT", "tron")
	if err != nil || p.UUID != "inv-1" || p.PaymentStatus != "check" || p.Address != "TXyz" ||
		p.PayerCurrency != "USDT" || p.Network != "tron" {
		t.Fatalf("PublicSelect: %v %+v", err, p)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/pay/inv-1/select" {
		t.Fatalf("expected POST /v1/pay/inv-1/select, got %s %s", gotMethod, gotPath)
	}
	for _, h := range []string{"X-Signature", "X-Public-Id", "X-Timestamp"} {
		if v := gotHeaders.Get(h); v != "" {
			t.Fatalf("public POST /v1/pay/{id}/select must be UNSIGNED: header %s = %q", h, v)
		}
	}
	var body map[string]any
	_ = json.Unmarshal(gotBody, &body)
	if body["currency"] != "USDT" || body["network"] != "tron" || len(body) != 2 {
		t.Fatalf("unexpected body: %s", gotBody)
	}
}

func TestIsTestKey(t *testing.T) {
	cases := []struct {
		publicID string
		want     bool
	}{
		{"test_abc123", true},
		{"test_", true},
		{"oblodai_abc123", false},
		{"live_test_abc", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsTestKey(tc.publicID); got != tc.want {
			t.Fatalf("IsTestKey(%q) = %v, want %v", tc.publicID, got, tc.want)
		}
	}
}
