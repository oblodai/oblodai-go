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
	"strings"
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

// ─────────────────────────────── Статусы ───────────────────────────────

// Константы статусов обязаны совпадать со строками из JSON шлюза — иначе типизация только вредит.
func TestStatusConstantsMatchWire(t *testing.T) {
	var p Payment
	if err := json.Unmarshal([]byte(`{"payment_status":"wrong_amount_waiting"}`), &p); err != nil {
		t.Fatal(err)
	}
	// Поле модели — string (типы полей НЕ ломались), словарь — отдельные именованные константы.
	if p.PaymentStatus != string(PaymentStatusWrongAmountWaiting) {
		t.Fatalf("payment_status: %q", p.PaymentStatus)
	}
	// Ключевая пара: ждём доплату — НЕ терминально; счёт закрылся недоплаченным — терминально.
	if PaymentStatus(p.PaymentStatus).IsFinal() {
		t.Fatal("wrong_amount_waiting не терминален: возможна доплата")
	}
	if !PaymentStatusWrongAmount.IsFinal() {
		t.Fatal("wrong_amount терминален")
	}

	var out Payout
	if err := json.Unmarshal([]byte(`{"status":"process"}`), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != string(PayoutStatusProcess) || PayoutStatus(out.Status).IsFinal() {
		t.Fatalf("status: %q", out.Status)
	}
	for _, s := range []PayoutStatus{PayoutStatusPaid, PayoutStatusFail, PayoutStatusCancel} {
		if !s.IsFinal() {
			t.Fatalf("%q терминален", s)
		}
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

// Регрессия на блокер в доках: секрет API-ключа НЕ проверяет вебхуки. Подпись считается секретом
// ЭНДПОИНТА (Webhooks.Register(...).Secret), и подстановка Config.Secret обязана отвергнуть вебхук,
// а не «почти сработать».
func TestVerifyWebhookRejectsAPIKeySecret(t *testing.T) {
	endpointSecret := "wh_endpoint_secret" // то, что вернул Register
	apiKeySecret := "oblodai_live_apikey"  // Config.Secret — НЕ подходит для вебхуков

	body := []byte(`{"type":"payment","status":"paid"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := ComputeWebhookSignature(endpointSecret, ts, body)

	if err := VerifyWebhook(endpointSecret, body, WebhookHeaders{ts, sig}, nil); err != nil {
		t.Fatalf("секрет эндпоинта должен проходить: %v", err)
	}

	err := VerifyWebhook(apiKeySecret, body, WebhookHeaders{ts, sig}, nil)
	var sigErr *SignatureError
	if !errors.As(err, &sigErr) {
		t.Fatalf("секрет API-ключа обязан быть отвергнут, получили: %v", err)
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

	// с ЯВНО отключённой проверкой свежести — проходит
	if err := VerifyWebhook(secret, body, WebhookHeaders{old, sig}, &VerifyOptions{MaxAgeSeconds: DisableWebhookMaxAge}); err != nil {
		t.Fatalf("should pass with MaxAgeSeconds=DisableWebhookMaxAge: %v", err)
	}
}

// Регрессия на дыру в replay-защите: нулевое значение MaxAgeSeconds (структура передана ради Now
// или любого другого поля) обязано означать ДЕФОЛТ 300, а не «проверка выключена».
func TestVerifyWebhookMaxAgeZeroValueMeansDefault(t *testing.T) {
	secret := "wh"
	body := []byte(`{"status":"paid"}`)
	now := time.Unix(1_700_000_000, 0)

	sign := func(ageSeconds int64) WebhookHeaders {
		ts := strconv.FormatInt(now.Unix()-ageSeconds, 10)
		return WebhookHeaders{ts, ComputeWebhookSignature(secret, ts, body)}
	}
	isRejected := func(err error) bool {
		var sigErr *SignatureError
		return errors.As(err, &sigErr)
	}

	// 1. Не задано (только Now) → дефолтные 300 секунд, окно РАБОТАЕТ.
	if err := VerifyWebhook(secret, body, sign(3600), &VerifyOptions{Now: now}); !isRejected(err) {
		t.Fatalf("MaxAgeSeconds не задан → должен действовать дефолт %d с; получили: %v",
			DefaultWebhookMaxAgeSeconds, err)
	}
	if err := VerifyWebhook(secret, body, sign(10), &VerifyOptions{Now: now}); err != nil {
		t.Fatalf("свежий вебхук внутри дефолтного окна отвергнут: %v", err)
	}
	// Граница дефолта: 299 с внутри окна, 301 с — уже нет.
	if err := VerifyWebhook(secret, body, sign(DefaultWebhookMaxAgeSeconds-1), &VerifyOptions{Now: now}); err != nil {
		t.Fatalf("299 с должно проходить при дефолтном окне: %v", err)
	}
	if err := VerifyWebhook(secret, body, sign(DefaultWebhookMaxAgeSeconds+1), &VerifyOptions{Now: now}); !isRejected(err) {
		t.Fatalf("301 с должно отвергаться при дефолтном окне, получили: %v", err)
	}

	// 2. Сентинел -1 → окно ВЫКЛЮЧЕНО, сколь угодно старый подписанный вебхук проходит.
	if err := VerifyWebhook(secret, body, sign(10*365*24*3600),
		&VerifyOptions{Now: now, MaxAgeSeconds: DisableWebhookMaxAge}); err != nil {
		t.Fatalf("DisableWebhookMaxAge обязан отключать окно: %v", err)
	}

	// 3. Положительное значение → ровно оно, а не дефолт.
	opts := &VerifyOptions{Now: now, MaxAgeSeconds: 60}
	if err := VerifyWebhook(secret, body, sign(59), opts); err != nil {
		t.Fatalf("59 с должно проходить при окне 60 с: %v", err)
	}
	if err := VerifyWebhook(secret, body, sign(61), opts); !isRejected(err) {
		t.Fatalf("61 с должно отвергаться при окне 60 с, получили: %v", err)
	}
	// …и заданное окно шире дефолта тоже уважается.
	if err := VerifyWebhook(secret, body, sign(600), &VerifyOptions{Now: now, MaxAgeSeconds: 900}); err != nil {
		t.Fatalf("600 с должно проходить при окне 900 с: %v", err)
	}

	// 4. И без opts вовсе окно остаётся включённым.
	old := strconv.FormatInt(time.Now().Unix()-3600, 10)
	if err := VerifyWebhook(secret, body, WebhookHeaders{old, ComputeWebhookSignature(secret, old, body)}, nil); !isRejected(err) {
		t.Fatalf("opts == nil → дефолтное окно, получили: %v", err)
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
	}, NoRetry()) // повторы выключены явно (nil теперь означает DefaultRetry)
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

// ──────────────────── Идемпотентность v1.1.0 (Idempotency-Key) ────────────────────

// orderIDFromBody достаёт order_id из JSON-тела запроса ("" — поля нет).
func orderIDFromBody(t *testing.T, body []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, body)
	}
	s, _ := m["order_id"].(string)
	return s
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

// (d) Отрицательный/огромный Retry-After не даёт отрицательной длительности и не паникует;
// результат всегда зажат в [0, maxRetryAfter].
func TestParseRetryAfterClampedNonNegative(t *testing.T) {
	cases := []string{"", "abc", "-1", "-999999", "0", "60", "301", "99999999999999999", "9223372036854775807"}
	for _, h := range cases {
		d := parseRetryAfter(h)
		if d < 0 {
			t.Fatalf("Retry-After %q → negative duration %v (immediate busy-retry risk)", h, d)
		}
		if d > maxRetryAfter {
			t.Fatalf("Retry-After %q → %v exceeds cap %v", h, d, maxRetryAfter)
		}
	}
}

// (d, продолжение) Огромный Retry-After ведёт к ОГРАНИЧЕННОЙ (не мгновенной) задержке: с коротким
// контекстом повтор не крутится в busy-loop, а корректно упирается в дедлайн контекста.
func TestHugeRetryAfterDoesNotBusyRetry(t *testing.T) {
	var calls int32
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "99999999999999999") // огромное значение
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]any{"state": 1, "message": "rate limit exceeded"})
	}, &RetryConfig{MaxAttempts: 5, InitialDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.Account.Balance(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error (context deadline during bounded retry wait)")
	}
	// Если задержка была отрицательной/мгновенной, повторы бы прокрутились быстро и сделали
	// MaxAttempts вызовов. Ограниченная задержка (зажата к maxRetryAfter) + короткий контекст →
	// ровно 1 вызов, затем упор в дедлайн.
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected exactly 1 call (bounded wait, then ctx deadline), got %d — busy-retry?", n)
	}
	// Санити: не зависли надолго (context деконтит быстро).
	if elapsed > 2*time.Second {
		t.Fatalf("retry wait not bounded by context: %v", elapsed)
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

// ───────────────────────── Базовый URL: только https (+ loopback) ─────────────────────────

// Подпись запроса (X-Signature) не должна уезжать по открытому каналу: не-https базовый URL
// отвергается на этапе создания клиента. Единственное исключение — локальная петля, на которой
// живут локальные стенды шлюза (в том числе http://localhost:8095).
func TestNewRejectsInsecureBaseURL(t *testing.T) {
	insecure := []string{
		"http://api.oblodai.com",
		"http://api.oblodai.com:8095",
		"http://198.51.100.7:8095",  // внешний IP — не петля
		"http://localhost.evil.com", // хост лишь НАЧИНАЕТСЯ на localhost
		"http://notlocalhost",       // и не заканчивается на .localhost
		"ftp://api.oblodai.com",     // чужая схема
		"api.oblodai.com",           // без схемы
	}
	for _, base := range insecure {
		if _, err := New(Config{PublicID: "p", Secret: "s", BaseURL: base}); err == nil {
			t.Fatalf("BaseURL %q обязан быть отвергнут", base)
		}
	}

	secure := []string{
		"https://api.oblodai.com",
		"https://api.oblodai.com/",
		"https://localhost:8095",
		"http://localhost:8095", // наш локальный стенд
		"http://localhost",
		"http://127.0.0.1:8095",
		"http://127.1.2.3:8095", // вся 127.0.0.0/8 — петля
		"http://[::1]:8095",
		"http://api.localhost:8095", // *.localhost резолвится в петлю
	}
	for _, base := range secure {
		if _, err := New(Config{PublicID: "p", Secret: "s", BaseURL: base}); err != nil {
			t.Fatalf("BaseURL %q обязан приниматься: %v", base, err)
		}
	}

	// Пустой BaseURL → дефолт https://api.oblodai.com.
	c, err := New(Config{PublicID: "p", Secret: "s"})
	if err != nil {
		t.Fatalf("дефолтный BaseURL: %v", err)
	}
	if c.baseURL != defaultBaseURL {
		t.Fatalf("baseURL: %q", c.baseURL)
	}

	// Текст ошибки обязан объяснять причину, а не просто «invalid url».
	_, err = New(Config{PublicID: "p", Secret: "s", BaseURL: "http://api.oblodai.com"})
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("ошибка должна упоминать https, получили: %v", err)
	}
}

// ───────────────────────── Имена ресурсов ─────────────────────────

// PaymentLinks — каноническое имя, Links — задокументированный алиас на ТОТ ЖЕ объект.
func TestPaymentLinksAlias(t *testing.T) {
	c, err := New(Config{PublicID: "p", Secret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if c.PaymentLinks == nil || c.Links == nil {
		t.Fatal("оба имени ресурса должны быть заполнены")
	}
	if c.PaymentLinks != c.Links {
		t.Fatal("Links обязан быть алиасом PaymentLinks (тот же объект), а не второй копией")
	}
}

// ───────────────────────── Совместимость типов полей ─────────────────────────

// Регрессия: типы экспортированных полей статусов должны оставаться string — иначе минорный
// релиз ломает чужой код вида strings.ToUpper(p.Status) или map[string]T[p.PaymentStatus].
// Именованные типы PaymentStatus/PayoutStatus остаются АДДИТИВНЫМ словарём констант.
func TestStatusFieldsStayString(t *testing.T) {
	var (
		_ string = Payment{}.PaymentStatus
		_ string = Payout{}.Status
		_ string = MassPayoutItem{}.Status
		_ string = Resolution{}.Status
	)
	// Типовой пользовательский код, который ломался бы при смене типа поля:
	p := Payment{PaymentStatus: "paid"}
	if strings.ToUpper(p.PaymentStatus) != "PAID" {
		t.Fatal("поле статуса обязано работать как обычная string")
	}
	handlers := map[string]int{"paid": 1}
	if handlers[p.PaymentStatus] != 1 {
		t.Fatal("статус обязан индексировать map[string]T без приведения")
	}
	// А словарь констант при этом доступен и полезен.
	if !PaymentStatus(p.PaymentStatus).IsFinal() {
		t.Fatal("paid терминален")
	}
}
