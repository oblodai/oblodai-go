package oblodai

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ──────────────────── Идемпотентность: заголовок вместо авто-order_id ────────────────────

// Без order_id: SDK НЕ подставляет его в тело (ломающее изменение v1.1.0), но шлёт валидный
// UUID v4 в заголовке Idempotency-Key. Заголовок не входит в подпись.
func TestCreateSendsIdempotencyHeaderNoAutoOrderID(t *testing.T) {
	var gotBody []byte
	var gotHeader, gotTS, gotSig string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotHeader = r.Header.Get("Idempotency-Key")
		gotTS = r.Header.Get("X-Timestamp")
		gotSig = r.Header.Get("X-Signature")
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"uuid": "p1", "amount": "10.00", "currency": "USD", "payment_status": "check", "address": "T1",
		}})
	}, nil)
	defer srv.Close()

	if _, err := c.Payments.Create(context.Background(), Params{"amount": "10", "currency": "USD"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if oid := orderIDFromBody(t, gotBody); oid != "" {
		t.Fatalf("SDK must not inject order_id anymore, got %q (body=%s)", oid, gotBody)
	}
	if !uuidRe.MatchString(gotHeader) {
		t.Fatalf("Idempotency-Key must be a UUID v4, got %q", gotHeader)
	}
	// Подпись считается БЕЗ заголовка: ts\nPOST\npath\nbody.
	mac := hmac.New(sha256.New, []byte("sec_1"))
	mac.Write([]byte(gotTS + "\nPOST\n/v1/payment\n" + string(gotBody)))
	if want := hex.EncodeToString(mac.Sum(nil)); gotSig != want {
		t.Fatalf("signature must not include Idempotency-Key header:\n got  %s\n want %s", gotSig, want)
	}
}

// Ключ генерируется ОДИН РАЗ до цикла повторов: все ретраи шлют один и тот же заголовок.
func TestIdempotencyKeyStableAcrossRetries(t *testing.T) {
	var mu sync.Mutex
	var keys []string
	var calls int32
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		mu.Unlock()
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(503)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "x.unavailable", "message": "later"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"uuid": "p1", "amount": "10.00", "currency": "USD", "payment_status": "check", "address": "T1",
		}})
	}, &RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond})
	defer srv.Close()

	if _, err := c.Payments.Create(context.Background(), Params{"amount": "10", "currency": "USD"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(keys) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(keys))
	}
	if keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("Idempotency-Key differs across retries: %q vs %q", keys[0], keys[1])
	}
}

// Два отдельных вызова (даже с одной картой) получают РАЗНЫЕ ключи; карта не мутируется.
func TestIdempotencyKeyDistinctPerCall(t *testing.T) {
	var mu sync.Mutex
	var keys []string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"uuid": "p1", "amount": "10.00", "currency": "USD", "payment_status": "check", "address": "T1",
		}})
	}, nil)
	defer srv.Close()

	m := Params{"amount": "10", "currency": "USD"}
	for i := 0; i < 2; i++ {
		if _, err := c.Payments.Create(context.Background(), m); err != nil {
			t.Fatalf("Create #%d: %v", i+1, err)
		}
	}
	if len(m) != 2 {
		t.Fatalf("caller map mutated: %+v", m)
	}
	mu.Lock()
	defer mu.Unlock()
	if keys[0] == keys[1] {
		t.Fatalf("two calls must get distinct Idempotency-Key, both %q", keys[0])
	}
}

// Явный ключ: params["idempotency_key"] уходит заголовком, вырезается из тела, карта не мутируется.
func TestExplicitIdempotencyKeyGoesToHeader(t *testing.T) {
	var gotBody []byte
	var gotHeader string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotHeader = r.Header.Get("Idempotency-Key")
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"uuid": "p1"}})
	}, nil)
	defer srv.Close()

	m := Params{"amount": "10", "currency": "USD", "idempotency_key": "my-key-42"}
	if _, err := c.Payments.Create(context.Background(), m); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotHeader != "my-key-42" {
		t.Fatalf("explicit key not in header: %q", gotHeader)
	}
	var body map[string]any
	_ = json.Unmarshal(gotBody, &body)
	if _, ok := body["idempotency_key"]; ok {
		t.Fatalf("idempotency_key must be stripped from body: %s", gotBody)
	}
	if _, ok := m["idempotency_key"]; !ok {
		t.Fatalf("caller map mutated: %+v", m)
	}
}

// TransferToPersonal: больше нет авто-order_id, есть заголовок.
func TestTransferToPersonalIdempotencyHeader(t *testing.T) {
	var gotBody []byte
	var gotHeader string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotHeader = r.Header.Get("Idempotency-Key")
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"ok": true}})
	}, nil)
	defer srv.Close()

	if _, err := c.Account.TransferToPersonal(context.Background(), Params{"amount": "5", "currency": "USDT"}); err != nil {
		t.Fatalf("TransferToPersonal: %v", err)
	}
	if oid := orderIDFromBody(t, gotBody); oid != "" {
		t.Fatalf("SDK must not inject order_id, got %q", oid)
	}
	if !uuidRe.MatchString(gotHeader) {
		t.Fatalf("expected UUID Idempotency-Key, got %q", gotHeader)
	}
}

// Эндпоинты БЕЗ withIdempotency на шлюзе не должны получать заголовок (payout-link — дедуп по
// reference; info — read-only).
func TestNonIdempotentEndpointsNoHeader(t *testing.T) {
	var mu sync.Mutex
	headers := map[string]string{}
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		headers[r.URL.Path] = r.Header.Get("Idempotency-Key")
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"link_id": "l1", "status": "funded"}})
	}, nil)
	defer srv.Close()

	ctx := context.Background()
	if _, err := c.PayoutLinks.Create(ctx, PayoutLinkParams{Currency: "BTC", Network: "bitcoin", Amount: "0.005", ExpiresInHours: 24}); err != nil {
		t.Fatalf("PayoutLinks.Create: %v", err)
	}
	if _, err := c.Payments.Info(ctx, "u1", ""); err != nil {
		t.Fatalf("Payments.Info: %v", err)
	}
	for path, h := range headers {
		if h != "" {
			t.Fatalf("endpoint %s must not send Idempotency-Key, got %q", path, h)
		}
	}
}

// ──────────────────── Повторы: nil = DefaultRetry, NoRetry отключает ────────────────────

func TestNilRetryMeansDefaultRetry(t *testing.T) {
	c, err := New(Config{PublicID: "p", Secret: "s"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.retry == nil || c.retry.MaxAttempts != DefaultRetry().MaxAttempts {
		t.Fatalf("nil Retry must mean DefaultRetry, got %+v", c.retry)
	}
}

func TestNoRetryDisablesRetries(t *testing.T) {
	var calls int32
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(503)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "x.unavailable", "message": "later"}})
	}, NoRetry())
	defer srv.Close()

	if _, err := c.Account.Balance(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("NoRetry must make exactly 1 attempt, got %d", n)
	}
}

// ──────────────────── Батчи ────────────────────

func TestPaymentsCreateBatchAndInfo(t *testing.T) {
	var mu sync.Mutex
	bodies := map[string][]byte{}
	idemKeys := map[string]string{}
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies[r.URL.Path] = b
		idemKeys[r.URL.Path] = r.Header.Get("Idempotency-Key")
		mu.Unlock()
		switch r.URL.Path {
		case "/v1/payment/batch":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
				"batch_id": "b-1", "kind": "payments", "count": 2, "status": "pending",
			}})
		case "/v1/batch/info":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
				"batch_id": "b-1", "kind": "payments", "status": "completed", "on_error": "continue",
				"total": 2, "succeeded": 1, "failed": 1,
				"items": []any{
					map[string]any{"idx": 0, "status": "succeeded", "order_id": "a-1", "result": map[string]any{"uuid": "p1"}},
					map[string]any{"idx": 1, "status": "failed", "order_id": "a-2", "error": "payment.unknown_currency"},
				},
			}})
		}
	}, nil)
	defer srv.Close()

	sub, err := c.Payments.CreateBatch(context.Background(), []Params{
		{"amount": "10", "currency": "USD", "order_id": "a-1"},
		{"amount": "20", "currency": "EUR", "order_id": "a-2"},
	}, "continue")
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if sub.BatchID != "b-1" || sub.Count != 2 || sub.Status != "pending" {
		t.Fatalf("unexpected submission: %+v", sub)
	}
	var reqBody map[string]any
	_ = json.Unmarshal(bodies["/v1/payment/batch"], &reqBody)
	if reqBody["on_error"] != "continue" || len(reqBody["payments"].([]any)) != 2 {
		t.Fatalf("unexpected batch request body: %s", bodies["/v1/payment/batch"])
	}
	if !uuidRe.MatchString(idemKeys["/v1/payment/batch"]) {
		t.Fatalf("batch submit must send Idempotency-Key, got %q", idemKeys["/v1/payment/batch"])
	}

	info, err := c.Batches.Info(context.Background(), sub.BatchID, 100, 0)
	if err != nil {
		t.Fatalf("Batches.Info: %v", err)
	}
	if info.Status != "completed" || info.Succeeded != 1 || info.Failed != 1 || len(info.Items) != 2 {
		t.Fatalf("unexpected info: %+v", info)
	}
	if idemKeys["/v1/batch/info"] != "" {
		t.Fatalf("batch/info is read-only and must not send Idempotency-Key")
	}
	var itemResult map[string]any
	if err := json.Unmarshal(info.Items[0].Result, &itemResult); err != nil || itemResult["uuid"] != "p1" {
		t.Fatalf("item result not verbatim: %s", info.Items[0].Result)
	}
	if info.Items[1].Error != "payment.unknown_currency" {
		t.Fatalf("unexpected item error: %+v", info.Items[1])
	}
	var infoBody map[string]any
	_ = json.Unmarshal(bodies["/v1/batch/info"], &infoBody)
	if infoBody["batch_id"] != "b-1" || infoBody["limit"] != float64(100) || infoBody["offset"] != float64(0) {
		t.Fatalf("unexpected info body: %s", bodies["/v1/batch/info"])
	}
}

func TestRefundAndPayoutBatchesPathsAndHeader(t *testing.T) {
	var mu sync.Mutex
	got := map[string]string{} // path → Idempotency-Key
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got[r.URL.Path] = r.Header.Get("Idempotency-Key")
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"batch_id": "b-2", "kind": "x", "count": 1, "status": "pending",
		}})
	}, nil)
	defer srv.Close()

	if _, err := c.Payments.RefundBatch(context.Background(), []Params{{"uuid": "u1", "reference": "r1"}}, ""); err != nil {
		t.Fatalf("RefundBatch: %v", err)
	}
	if _, err := c.Payouts.CreateBatch(context.Background(), []Params{{"order_id": "o1"}}, "stop"); err != nil {
		t.Fatalf("Payouts.CreateBatch: %v", err)
	}
	for _, path := range []string{"/v1/refund/batch", "/v1/payout/batch"} {
		key, ok := got[path]
		if !ok {
			t.Fatalf("endpoint %s was not called; called: %v", path, got)
		}
		if !uuidRe.MatchString(key) {
			t.Fatalf("%s must send Idempotency-Key, got %q", path, key)
		}
	}
}

// ──────────────────── Платёжные ссылки ────────────────────

func TestLinksManagementAndPublic(t *testing.T) {
	var mu sync.Mutex
	sigs := map[string]string{}
	methods := map[string]string{}
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sigs[r.URL.Path] = r.Header.Get("X-Signature")
		methods[r.URL.Path] = r.Method
		mu.Unlock()
		switch r.URL.Path {
		case "/v1/payment/link":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"link_id": "l1", "url": "https://pay.example/link/l1"}})
		case "/v1/payment/link/list":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"items": []any{
				map[string]any{"link_id": "l1", "amount_mode": "open", "currency": "USD", "active": true},
			}}})
		case "/v1/payment/link/info":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
				"link_id": "l1", "amount_mode": "open", "currency": "USD", "active": true,
				"payments": []any{map[string]any{"uuid": "p1", "status": "paid", "amount": "5", "currency": "USDT"}},
			}})
		case "/v1/payment/link/toggle":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"link_id": "l1", "active": false}})
		case "/v1/link/l1":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"link_id": "l1", "amount_mode": "open", "currency": "USD", "active": true}})
		case "/v1/link/l1/checkout":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"uuid": "p2", "url": "https://pay.example/p2"}})
		}
	}, nil)
	defer srv.Close()

	ctx := context.Background()
	created, err := c.Links.Create(ctx, LinkParams{AmountMode: "open", Currency: "USD"})
	if err != nil || created.LinkID != "l1" {
		t.Fatalf("Links.Create: %v %+v", err, created)
	}
	items, err := c.Links.List(ctx, 50, 0)
	if err != nil || len(items) != 1 || !items[0].Active {
		t.Fatalf("Links.List: %v %+v", err, items)
	}
	info, err := c.Links.Info(ctx, "l1")
	if err != nil || info.LinkID != "l1" || len(info.Payments) != 1 || info.Payments[0].UUID != "p1" {
		t.Fatalf("Links.Info: %v %+v", err, info)
	}
	tg, err := c.Links.Toggle(ctx, "l1", false)
	if err != nil || tg.Active {
		t.Fatalf("Links.Toggle: %v %+v", err, tg)
	}
	pub, err := c.Links.PublicGet(ctx, "l1")
	if err != nil || pub.LinkID != "l1" {
		t.Fatalf("Links.PublicGet: %v %+v", err, pub)
	}
	pay, err := c.Links.Checkout(ctx, "l1", Params{"amount": "5", "currency": "USDT", "network": "tron"})
	if err != nil || pay.UUID != "p2" {
		t.Fatalf("Links.Checkout: %v %+v", err, pay)
	}

	// Management — подписан; публичные — нет.
	for _, p := range []string{"/v1/payment/link", "/v1/payment/link/list", "/v1/payment/link/info", "/v1/payment/link/toggle"} {
		if sigs[p] == "" {
			t.Fatalf("management endpoint %s must be signed", p)
		}
	}
	for _, p := range []string{"/v1/link/l1", "/v1/link/l1/checkout"} {
		if sigs[p] != "" {
			t.Fatalf("public endpoint %s must be unsigned", p)
		}
	}
	if methods["/v1/link/l1"] != http.MethodGet {
		t.Fatalf("PublicGet must be GET, got %s", methods["/v1/link/l1"])
	}
}

// ──────────────────── Сплиты ────────────────────

func TestSplits(t *testing.T) {
	var mu sync.Mutex
	bodies := map[string][]byte{}
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies[r.URL.Path] = b
		mu.Unlock()
		switch r.URL.Path {
		case "/v1/split/rule":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"rule_id": "r1", "percent": 10.0}})
		case "/v1/split/rule/list":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"items": []any{
				map[string]any{"rule_id": "r1", "percent": 10.0, "active": true, "address": "T1", "network": "tron", "reversible": false},
				map[string]any{"rule_id": "r2", "percent": 5.0, "active": true, "merchant_id": "m2", "reversible": true},
			}}})
		case "/v1/split/rule/delete":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"deleted": true}})
		case "/v1/split/config/get", "/v1/split/config/set":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"refund_hold_hours": 24}})
		}
	}, nil)
	defer srv.Close()

	ctx := context.Background()
	rule, err := c.Splits.SplitToAddress(ctx, "T1", "tron", 10.0, "партнёр А")
	if err != nil || rule.RuleID != "r1" || rule.Percent != 10.0 {
		t.Fatalf("SplitToAddress: %v %+v", err, rule)
	}
	var ruleBody map[string]any
	_ = json.Unmarshal(bodies["/v1/split/rule"], &ruleBody)
	if ruleBody["address"] != "T1" || ruleBody["network"] != "tron" || ruleBody["percent"] != 10.0 || ruleBody["note"] != "партнёр А" {
		t.Fatalf("unexpected rule body: %s", bodies["/v1/split/rule"])
	}
	if _, err := c.Splits.SplitToMerchant(ctx, "m2", 5.0, ""); err != nil {
		t.Fatalf("SplitToMerchant: %v", err)
	}
	rules, err := c.Splits.ListRules(ctx)
	if err != nil || len(rules) != 2 || rules[0].Reversible || !rules[1].Reversible {
		t.Fatalf("ListRules: %v %+v", err, rules)
	}
	deleted, err := c.Splits.DeleteRule(ctx, "r1")
	if err != nil || !deleted {
		t.Fatalf("DeleteRule: %v %v", err, deleted)
	}
	cfg, err := c.Splits.SetConfig(ctx, 24)
	if err != nil || cfg.RefundHoldHours != 24 {
		t.Fatalf("SetConfig: %v %+v", err, cfg)
	}
	if _, err := c.Splits.GetConfig(ctx); err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
}

// ──────────────────── Send-email и resolve ────────────────────

func TestSendEmail(t *testing.T) {
	var gotBody []byte
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payment/send-email" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotBody, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"sent": true, "email": "buyer@example.com", "uuid": "p1",
		}})
	}, nil)
	defer srv.Close()

	res, err := c.Payments.SendEmail(context.Background(), "p1", "", "buyer@example.com")
	if err != nil || !res.Sent || res.Email != "buyer@example.com" {
		t.Fatalf("SendEmail: %v %+v", err, res)
	}
	var body map[string]any
	_ = json.Unmarshal(gotBody, &body)
	if body["uuid"] != "p1" || body["email"] != "buyer@example.com" {
		t.Fatalf("unexpected body: %s", gotBody)
	}
}

func TestResolveAcceptAndRefund(t *testing.T) {
	var mu sync.Mutex
	var lastBody []byte
	var lastKey string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		lastBody = b
		lastKey = r.Header.Get("Idempotency-Key")
		mu.Unlock()
		var req map[string]any
		_ = json.Unmarshal(b, &req)
		if req["action"] == "accept" {
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
				"payment_uuid": "e0a1", "order_id": "ord-1001", "resolution": "accepted",
				"amount_kept": "48.5", "currency": "USDT",
			}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"payment_uuid": "e0a1", "order_id": "ord-1001", "resolution": "refunded",
			"uuid": "f2b3", "amount": "48.5", "currency": "USDT", "address": "0xPayer",
			"status": "check", "is_final": false,
		}})
	}, nil)
	defer srv.Close()

	ctx := context.Background()
	acc, err := c.Payments.Resolve(ctx, "e0a1", "", "accept", nil)
	if err != nil || acc.Resolution != "accepted" || acc.AmountKept != "48.5" {
		t.Fatalf("Resolve accept: %v %+v", err, acc)
	}
	if !uuidRe.MatchString(lastKey) {
		t.Fatalf("resolve must send Idempotency-Key, got %q", lastKey)
	}

	ref, err := c.Payments.Resolve(ctx, "", "ord-1001", "refund", Params{"reference": "rf-1"})
	if err != nil || ref.Resolution != "refunded" || ref.UUID != "f2b3" || ref.Status != "check" || ref.IsFinal {
		t.Fatalf("Resolve refund: %v %+v", err, ref)
	}
	var body map[string]any
	_ = json.Unmarshal(lastBody, &body)
	if body["order_id"] != "ord-1001" || body["action"] != "refund" || body["reference"] != "rf-1" {
		t.Fatalf("unexpected resolve body: %s", lastBody)
	}
	if _, hasUUID := body["uuid"]; hasUUID {
		t.Fatalf("empty uuid must be omitted: %s", lastBody)
	}
}

// ──────────────────── Payout-ссылки (крипто-чеки) ────────────────────

func TestPayoutLinksLifecycle(t *testing.T) {
	var mu sync.Mutex
	sigs := map[string]string{}
	bodies := map[string][]byte{}
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		sigs[r.URL.Path] = r.Header.Get("X-Signature")
		bodies[r.URL.Path] = b
		mu.Unlock()
		switch r.URL.Path {
		case "/v1/payout/link":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
				"link_id": "7f0c", "status": "funded", "amount": "0.005", "currency": "BTC",
				"network": "bitcoin", "expires_at": "2026-08-14T17:00:00Z", "created_at": "2026-07-15T17:00:00Z",
				"reference": "bonus-42", "claim_token": "Xk3v", "claim_url": "https://pay.oblodai.com/claim/Xk3v",
			}})
		case "/v1/payout/link/batch":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
				"created": 1, "total": 2, "results": []any{
					map[string]any{"ok": true, "link": map[string]any{"link_id": "l1", "status": "funded", "batch_id": "b9", "claim_token": "t1"}},
					map[string]any{"ok": false, "error": "payoutlink.insufficient_funds", "message": "available balance is less than the link amount"},
				},
			}})
		case "/v1/payout/link/list":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"links": []any{
				map[string]any{"link_id": "7f0c", "status": "funded"},
			}}})
		case "/v1/payout/link/info":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
				"link_id": "7f0c", "status": "claimed", "payout_id": "a1b2", "claim_address": "bc1q",
			}})
		case "/v1/payout/link/cancel":
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"link_id": "7f0c", "status": "cancelled"}})
		}
	}, nil)
	defer srv.Close()

	ctx := context.Background()
	link, err := c.PayoutLinks.Create(ctx, PayoutLinkParams{
		Currency: "BTC", Network: "bitcoin", Amount: "0.005", Reference: "bonus-42", ExpiresInHours: 720,
	})
	if err != nil || link.Status != PayoutLinkStatusFunded || link.ClaimToken != "Xk3v" || link.ClaimURL == "" {
		t.Fatalf("Create: %v %+v", err, link)
	}
	var createBody map[string]any
	_ = json.Unmarshal(bodies["/v1/payout/link"], &createBody)
	if createBody["expires_in_hours"] != float64(720) || createBody["reference"] != "bonus-42" {
		t.Fatalf("unexpected create body: %s", bodies["/v1/payout/link"])
	}
	if _, ok := createBody["title"]; ok {
		t.Fatalf("empty optional fields must be omitted: %s", bodies["/v1/payout/link"])
	}

	batch, err := c.PayoutLinks.CreateBatch(ctx, []PayoutLinkParams{
		{Currency: "BTC", Network: "bitcoin", Amount: "0.001", ExpiresInHours: 24},
		{Currency: "BTC", Network: "bitcoin", Amount: "9999", ExpiresInHours: 24},
	})
	if err != nil || batch.Created != 1 || batch.Total != 2 {
		t.Fatalf("CreateBatch: %v %+v", err, batch)
	}
	if !batch.Results[0].OK || batch.Results[0].Link.BatchID != "b9" {
		t.Fatalf("unexpected batch item 0: %+v", batch.Results[0])
	}
	if batch.Results[1].OK || batch.Results[1].Error != "payoutlink.insufficient_funds" {
		t.Fatalf("unexpected batch item 1: %+v", batch.Results[1])
	}

	links, err := c.PayoutLinks.List(ctx, 50, 0)
	if err != nil || len(links) != 1 {
		t.Fatalf("List: %v %+v", err, links)
	}
	info, err := c.PayoutLinks.Info(ctx, "7f0c")
	if err != nil || info.Status != PayoutLinkStatusClaimed || info.PayoutID != "a1b2" || info.ClaimAddress != "bc1q" {
		t.Fatalf("Info: %v %+v", err, info)
	}
	cancelled, err := c.PayoutLinks.Cancel(ctx, "7f0c")
	if err != nil || cancelled.Status != PayoutLinkStatusCancelled {
		t.Fatalf("Cancel: %v %+v", err, cancelled)
	}

	for path, sig := range sigs {
		if sig == "" {
			t.Fatalf("management endpoint %s must be signed", path)
		}
	}
}

// Публичные claim-эндпоинты уходят БЕЗ подписи и без Idempotency-Key.
func TestPayoutLinkClaimUnsigned(t *testing.T) {
	var mu sync.Mutex
	type reqMeta struct {
		method, sig, publicID, idem string
		body                        []byte
	}
	reqs := map[string]reqMeta{}
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs[r.Method] = reqMeta{r.Method, r.Header.Get("X-Signature"), r.Header.Get("X-Public-Id"), r.Header.Get("Idempotency-Key"), b}
		mu.Unlock()
		if r.URL.Path != "/v1/claim/Xk3vTok" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
				"status": "funded", "amount": "0.005", "currency": "BTC", "network": "bitcoin",
				"expires_at": "2026-08-14T17:00:00Z", "claimable": true,
			}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"status": "claimed", "payout_id": "a1b2", "amount": "0.005", "currency": "BTC",
			"network": "bitcoin", "address": "bc1q",
		}})
	}, nil)
	defer srv.Close()

	ctx := context.Background()
	ci, err := c.PayoutLinks.ClaimInfo(ctx, "Xk3vTok")
	if err != nil || !ci.Claimable || ci.Status != PayoutLinkStatusFunded {
		t.Fatalf("ClaimInfo: %v %+v", err, ci)
	}
	claim, err := c.PayoutLinks.Claim(ctx, "Xk3vTok", "bc1q")
	if err != nil || claim.Status != PayoutLinkStatusClaimed || claim.PayoutID != "a1b2" || claim.Address != "bc1q" {
		t.Fatalf("Claim: %v %+v", err, claim)
	}

	for method, m := range reqs {
		if m.sig != "" || m.publicID != "" {
			t.Fatalf("claim %s must be unsigned, got sig=%q public_id=%q", method, m.sig, m.publicID)
		}
		if m.idem != "" {
			t.Fatalf("claim %s must not send Idempotency-Key", method)
		}
	}
	var claimBody map[string]any
	_ = json.Unmarshal(reqs[http.MethodPost].body, &claimBody)
	if claimBody["address"] != "bc1q" {
		t.Fatalf("unexpected claim body: %s", reqs[http.MethodPost].body)
	}
	if _, ok := claimBody["memo"]; ok {
		t.Fatalf("empty memo must be omitted: %s", reqs[http.MethodPost].body)
	}

	// memo уходит в тело при ClaimWithMemo
	if _, err := c.PayoutLinks.ClaimWithMemo(ctx, "Xk3vTok", "EQabc", "12345"); err != nil {
		t.Fatalf("ClaimWithMemo: %v", err)
	}
	_ = json.Unmarshal(reqs[http.MethodPost].body, &claimBody)
	if claimBody["memo"] != "12345" || claimBody["address"] != "EQabc" {
		t.Fatalf("memo not sent: %s", reqs[http.MethodPost].body)
	}
}
