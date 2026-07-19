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
