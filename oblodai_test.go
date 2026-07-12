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
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─────────────────────────────── Подпись ───────────────────────────────

func TestSignRequest(t *testing.T) {
	secret := "test_secret"
	body := `{"amount":"25.00"}`
	ts, sig := signRequest(secret, "POST", "/v1/payment", body, "1700000000")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("1700000000\nPOST\n/v1/payment\n" + body))
	expected := hex.EncodeToString(mac.Sum(nil))

	if ts != "1700000000" {
		t.Fatalf("timestamp: got %s", ts)
	}
	if sig != expected {
		t.Fatalf("signature mismatch:\n got  %s\n want %s", sig, expected)
	}
}

func TestSignRequestAutoTimestamp(t *testing.T) {
	before := time.Now().Unix()
	ts, _ := signRequest("s", "POST", "/v1/balance", "{}", "")
	after := time.Now().Unix()
	v, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || v < before || v > after {
		t.Fatalf("auto timestamp out of range: %s", ts)
	}
}

// ─────────────────────────────── Вебхуки ───────────────────────────────

func TestVerifyWebhook(t *testing.T) {
	secret := "wh_secret"
	body := []byte(`{"type":"payment","status":"paid"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := ComputeWebhookSignature(secret, ts, body)

	if err := VerifyWebhook(secret, body, WebhookHeaders{ts, sig}, nil); err != nil {
		t.Fatalf("valid webhook rejected: %v", err)
	}
}

func TestVerifyWebhookBadSignature(t *testing.T) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	err := VerifyWebhook("wh", []byte(`{}`), WebhookHeaders{ts, "deadbeef"}, nil)
	var sigErr *SignatureError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected SignatureError, got %v", err)
	}
}

func TestVerifyWebhookReplay(t *testing.T) {
	secret := "wh"
	body := []byte(`{"status":"paid"}`)
	old := strconv.FormatInt(time.Now().Unix()-3600, 10)
	sig := ComputeWebhookSignature(secret, old, body)

	err := VerifyWebhook(secret, body, WebhookHeaders{old, sig}, &VerifyOptions{MaxAgeSeconds: 300})
	var sigErr *SignatureError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected replay rejection, got %v", err)
	}

	// с отключённой проверкой свежести — проходит
	if err := VerifyWebhook(secret, body, WebhookHeaders{old, sig}, &VerifyOptions{MaxAgeSeconds: 0}); err != nil {
		t.Fatalf("should pass with MaxAgeSeconds=0: %v", err)
	}
}

func TestConstructEvent(t *testing.T) {
	secret := "wh"
	body := []byte(`{"type":"payment","status":"paid","uuid":"abc"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := ComputeWebhookSignature(secret, ts, body)

	var event map[string]any
	if err := ConstructEvent(secret, body, WebhookHeaders{ts, sig}, nil, &event); err != nil {
		t.Fatalf("construct failed: %v", err)
	}
	if event["uuid"] != "abc" || event["status"] != "paid" {
		t.Fatalf("unexpected event: %v", event)
	}
}

// ─────────────────────────────── Клиент ───────────────────────────────

func newTestClient(t *testing.T, handler http.HandlerFunc, retry *RetryConfig) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := New(Config{PublicID: "pub_1", Secret: "sec_1", BaseURL: srv.URL, Retry: retry})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, srv
}

func TestClientSignsAndUnwraps(t *testing.T) {
	var gotHeaders http.Header
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		json.NewEncoder(w).Encode(map[string]any{
			"state": 0,
			"result": map[string]any{
				"uuid": "p1", "order_id": "o1", "amount": "10.00",
				"currency": "USD", "payment_status": "check", "address": "T123",
			},
		})
	}, nil)
	defer srv.Close()

	p, err := c.Payments.Create(context.Background(), Params{"amount": "10", "currency": "USD", "order_id": "o1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.UUID != "p1" || p.Address != "T123" {
		t.Fatalf("unexpected payment: %+v", p)
	}
	if gotHeaders.Get("X-Public-Id") != "pub_1" {
		t.Fatalf("missing X-Public-Id")
	}
	if len(gotHeaders.Get("X-Signature")) != 64 {
		t.Fatalf("bad signature header: %q", gotHeaders.Get("X-Signature"))
	}
}

func TestClientAPIError(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "payout.insufficient_funds", "message": "no"},
		})
	}, nil)
	defer srv.Close()

	_, err := c.Payouts.Create(context.Background(), Params{"amount": "5", "currency": "USDT", "address": "T", "order_id": "x"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.Code != "payout.insufficient_funds" || apiErr.Status != 409 {
		t.Fatalf("unexpected: %+v", apiErr)
	}
	if apiErr.IsRetriable() {
		t.Fatalf("insufficient_funds should not be retriable")
	}
}

func TestClientRetries503(t *testing.T) {
	var calls int32
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(503)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "x.unavailable", "message": "later"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"balance": map[string]any{"merchant": []any{}}}})
	}, &RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond})
	defer srv.Close()

	bal, err := c.Account.Balance(context.Background())
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal == nil || len(bal.Merchant) != 0 {
		t.Fatalf("unexpected balance: %+v", bal)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestClientNoRetry400(t *testing.T) {
	var calls int32
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "request.bad_json", "message": "bad"}})
	}, &RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond})
	defer srv.Close()

	_, err := c.Account.Balance(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("400 should not retry, got %d calls", calls)
	}
}

func TestClientPublicRateNoSignature(t *testing.T) {
	var gotHeaders http.Header
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": []any{
			map[string]any{"from": "ETH", "to": "USDT", "course": "3450"},
		}})
	}, nil)
	defer srv.Close()

	rates, err := c.Rates.List(context.Background(), "ETH")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rates) != 1 || rates[0].Course != "3450" || rates[0].From != "ETH" {
		t.Fatalf("unexpected rates: %+v", rates)
	}
	if gotHeaders.Get("X-Signature") != "" {
		t.Fatalf("public endpoint should not be signed")
	}
}

func TestClientWebhookRegisterNoEnvelope(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"endpoint_id": "e1", "url": "https://x", "secret": "s1"})
	}, nil)
	defer srv.Close()

	reg, err := c.Webhooks.Register(context.Background(), "https://x")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if reg.Secret != "s1" || reg.EndpointID != "e1" {
		t.Fatalf("unexpected registration: %+v", reg)
	}
}

func TestClientMassPayoutPartial(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"items": []any{
			map[string]any{"uuid": "u1", "order_id": "p-1", "status": "process", "success": true},
			map[string]any{"order_id": "p-2", "success": false, "message": "insufficient"},
		}}})
	}, nil)
	defer srv.Close()

	res, err := c.Payouts.CreateMass(context.Background(), []Params{
		{"amount": "25", "currency": "USDT", "network": "tron", "address": "T1", "order_id": "p-1"},
		{"amount": "10", "currency": "USDT", "network": "tron", "address": "T2", "order_id": "p-2"},
	}, "")
	if err != nil {
		t.Fatalf("CreateMass: %v", err)
	}
	if len(res.Items) != 2 || !res.Items[0].Success || res.Items[1].Success {
		t.Fatalf("unexpected items: %+v", res.Items)
	}
	if res.Items[1].Message != "insufficient" {
		t.Fatalf("expected message: %+v", res.Items[1])
	}
}

func TestMissingConfig(t *testing.T) {
	if _, err := New(Config{Secret: "s"}); err == nil {
		t.Fatal("expected error for missing PublicID")
	}
	if _, err := New(Config{PublicID: "p"}); err == nil {
		t.Fatal("expected error for missing Secret")
	}
}

func TestNewFromEnv(t *testing.T) {
	t.Setenv("OBLODAI_PUBLIC_ID", "pub_env")
	t.Setenv("OBLODAI_SECRET", "sec_env")
	t.Setenv("OBLODAI_BASE_URL", "https://env.example")

	c, err := NewFromEnv(Config{})
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if c.publicID != "pub_env" || c.secret != "sec_env" || c.baseURL != "https://env.example" {
		t.Fatalf("unexpected client creds: %s %s %s", c.publicID, c.secret, c.baseURL)
	}

	t.Setenv("OBLODAI_PUBLIC_ID", "")
	if _, err := NewFromEnv(Config{}); err == nil {
		t.Fatal("expected error when OBLODAI_PUBLIC_ID is empty")
	}
}

func TestCurrenciesPublicGET(t *testing.T) {
	var method string
	var gotSig string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		gotSig = r.Header.Get("X-Signature")
		json.NewEncoder(w).Encode(map[string]any{"currencies": []any{
			map[string]any{"symbol": "USDT", "decimals": 6, "networks": []any{
				map[string]any{"network": "tron", "kind": "token", "min_confirmations": 20,
					"available": true, "deposit_available": true, "payout_available": true},
			}},
		}})
	}, nil)
	defer srv.Close()

	cur, err := c.Rates.Currencies(context.Background())
	if err != nil {
		t.Fatalf("Currencies: %v", err)
	}
	if method != http.MethodGet {
		t.Fatalf("expected GET, got %s", method)
	}
	if gotSig != "" {
		t.Fatalf("public GET must be unsigned, got sig %q", gotSig)
	}
	if len(cur) != 1 || cur[0].Symbol != "USDT" || cur[0].Networks[0].Network != "tron" {
		t.Fatalf("unexpected currencies: %+v", cur)
	}
}

func TestListDiscounts(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payment/discount/list" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": []any{
			map[string]any{"currency": "USDT", "network": "tron", "discount_percent": 3},
		}})
	}, nil)
	defer srv.Close()

	list, err := c.Payments.ListDiscounts(context.Background())
	if err != nil {
		t.Fatalf("ListDiscounts: %v", err)
	}
	if len(list) != 1 || list[0]["currency"] != "USDT" {
		t.Fatalf("unexpected discounts: %+v", list)
	}
}

func Test429SurfacesMessageAndRetryAfter(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]any{"state": 1, "message": "rate limit exceeded"})
	}, nil) // повторы выключены
	defer srv.Close()

	_, err := c.Account.Balance(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.Code != "http.429" || apiErr.Message != "rate limit exceeded" {
		t.Fatalf("unexpected 429 error: %+v", apiErr)
	}
	if apiErr.RetryAfter != 60*time.Second {
		t.Fatalf("expected RetryAfter=60s, got %v", apiErr.RetryAfter)
	}
}

// ──────────────────── Авто-идемпотентность (money-safety) ────────────────────

// orderIDFromBody достаёт order_id из JSON-тела запроса.
func orderIDFromBody(t *testing.T, body []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, body)
	}
	s, _ := m["order_id"].(string)
	return s
}

func TestPaymentsCreateInjectsOrderID(t *testing.T) {
	var gotBody []byte
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"uuid": "p1", "amount": "10.00", "currency": "USD", "payment_status": "check", "address": "T1",
		}})
	}, nil)
	defer srv.Close()

	// order_id НЕ задан — SDK обязан подставить непустой ключ идемпотентности.
	if _, err := c.Payments.Create(context.Background(), Params{"amount": "10", "currency": "USD"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if oid := orderIDFromBody(t, gotBody); oid == "" {
		t.Fatalf("expected auto-injected order_id, body=%s", gotBody)
	}
}

func TestPaymentsCreateSameOrderIDAcrossRetries(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	var calls int32
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
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
	if len(bodies) != 2 {
		t.Fatalf("expected 2 attempts (503 then ok), got %d", len(bodies))
	}
	oid0 := orderIDFromBody(t, bodies[0])
	oid1 := orderIDFromBody(t, bodies[1])
	if oid0 == "" {
		t.Fatalf("first attempt missing order_id")
	}
	if oid0 != oid1 {
		t.Fatalf("order_id differs across retries: %q vs %q", oid0, oid1)
	}
}

func TestTransferToPersonalInjectsOrderID(t *testing.T) {
	var gotBody []byte
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"ok": true}})
	}, nil)
	defer srv.Close()

	if _, err := c.Account.TransferToPersonal(context.Background(), Params{"amount": "5", "currency": "USDT"}); err != nil {
		t.Fatalf("TransferToPersonal: %v", err)
	}
	if oid := orderIDFromBody(t, gotBody); oid == "" {
		t.Fatalf("expected auto-injected order_id, body=%s", gotBody)
	}
}

func TestPaymentsCreateKeepsCallerOrderID(t *testing.T) {
	var gotBody []byte
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{
			"uuid": "p1", "amount": "10.00", "currency": "USD", "payment_status": "check", "address": "T1",
		}})
	}, nil)
	defer srv.Close()

	if _, err := c.Payments.Create(context.Background(), Params{"amount": "10", "currency": "USD", "order_id": "mine-1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if oid := orderIDFromBody(t, gotBody); oid != "mine-1" {
		t.Fatalf("caller order_id overwritten: %q", oid)
	}
}

func TestFundsMaturingNotRetriable(t *testing.T) {
	e := &APIError{Code: "payout.funds_maturing", Status: 409}
	if e.IsRetriable() {
		t.Fatal("payout.funds_maturing must be terminal (not retriable)")
	}
}

func Test429HonorsRetryAfter(t *testing.T) {
	var calls int32
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]any{"state": 1, "message": "rate limit exceeded"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"state": 0, "result": map[string]any{"balance": map[string]any{"merchant": []any{}}}})
	}, &RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond})
	defer srv.Close()

	if _, err := c.Account.Balance(context.Background()); err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls (429 then ok), got %d", calls)
	}
}
