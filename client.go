package oblodai

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://api.oblodai.com"
	defaultTimeout = 30 * time.Second

	// maxRetryAfter — абсолютный потолок для серверного Retry-After. Заголовок уважается как есть
	// (даже выше RetryConfig.MaxDelay), но не дольше 5 минут — иначе один ответ мог бы усыпить
	// вызывающего надолго.
	maxRetryAfter = 300 * time.Second

	// Переменные окружения для NewFromEnv.
	envPublicID = "OBLODAI_PUBLIC_ID"
	envSecret   = "OBLODAI_SECRET"
	envBaseURL  = "OBLODAI_BASE_URL" // необязательная — переопределяет базовый URL

	// envLog — необязательная переменная для opt-in логирования, если Config.Logger не задан.
	// Значения: debug/info/warn/error. Любое иное (или пусто) — логирование выключено.
	envLog = "OBLODAI_LOG"
)

// envLogger разбирает переменную окружения OBLODAI_LOG один раз и возвращает text-логгер slog
// в stderr на заданном уровне, либо nil если переменная не задана/некорректна.
var (
	envLoggerOnce sync.Once
	envLoggerVal  *slog.Logger
)

func envLogger() *slog.Logger {
	envLoggerOnce.Do(func() {
		level, ok := parseLogLevel(os.Getenv(envLog))
		if !ok {
			return
		}
		envLoggerVal = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	})
	return envLoggerVal
}

// parseLogLevel сопоставляет строку уровню slog. ok=false — строка не распознана.
func parseLogLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return 0, false
	}
}

// RetryConfig — настройки повторов с экспоненциальным backoff.
type RetryConfig struct {
	MaxAttempts  int           // максимум попыток (включая первую)
	InitialDelay time.Duration // начальная задержка
	MaxDelay     time.Duration // потолок задержки
}

// DefaultRetry возвращает разумные настройки повторов по умолчанию.
// С v1.1.0 передавать его не обязательно: nil в Config.Retry означает те же дефолтные повторы.
func DefaultRetry() *RetryConfig {
	return &RetryConfig{MaxAttempts: 4, InitialDelay: 500 * time.Millisecond, MaxDelay: 30 * time.Second}
}

// NoRetry отключает автоматические повторы (ровно одна попытка на вызов).
//
// С v1.1.0 повторы включены по умолчанию (Retry: nil == DefaultRetry()), как и в остальных SDK
// Oblodai. Отключайте их осознанно: Config{Retry: oblodai.NoRetry()}.
func NoRetry() *RetryConfig {
	return &RetryConfig{MaxAttempts: 1}
}

// Config — конфигурация клиента.
type Config struct {
	PublicID   string        // public_id (несекретный идентификатор ключа) — обязателен
	Secret     string        // secret для подписи запросов — обязателен
	BaseURL    string        // базовый URL API (по умолчанию https://api.oblodai.com)
	Timeout    time.Duration // таймаут запроса (по умолчанию 30с)
	Retry      *RetryConfig  // настройки повторов; nil = DefaultRetry(); отключить — NoRetry()
	HTTPClient *http.Client  // кастомный HTTP-клиент (по умолчанию свой)
	// Logger — необязательный slog-логгер. nil (по умолчанию) отключает логирование. Если nil, но
	// задана переменная окружения OBLODAI_LOG (debug/info/warn/error), клиент создаёт text-логгер в
	// stderr на этом уровне. Логи никогда не содержат секрет, подпись или тело запроса/ответа.
	Logger *slog.Logger
}

// Client — клиент Oblodai API. Создаётся через New. Ресурсы доступны как поля:
// Payments, Payouts, Batches, Links, Splits, PayoutLinks, Wallets, Account, Webhooks, Settings,
// Rates, Sandbox.
type Client struct {
	publicID string
	secret   string
	baseURL  string
	retry    *RetryConfig
	hc       *http.Client
	logger   *slog.Logger

	Payments    *PaymentsResource
	Payouts     *PayoutsResource
	Batches     *BatchesResource
	Links       *LinksResource
	Splits      *SplitsResource
	PayoutLinks *PayoutLinksResource
	Wallets     *WalletsResource
	Account     *AccountResource
	Webhooks    *WebhooksResource
	Settings    *SettingsResource
	Rates       *RatesResource
	Sandbox     *SandboxResource
}

// New создаёт клиента. Возвращает ошибку, если не заданы обязательные поля конфигурации.
func New(cfg Config) (*Client, error) {
	if cfg.PublicID == "" {
		return nil, errors.New("oblodai: Config.PublicID обязателен")
	}
	if cfg.Secret == "" {
		return nil, errors.New("oblodai: Config.Secret обязателен")
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	hc := cfg.HTTPClient
	if hc == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = defaultTimeout
		}
		hc = &http.Client{Timeout: timeout}
	}

	// С v1.1.0 nil означает дефолтные повторы (как во всех SDK Oblodai). Отключить — NoRetry().
	retry := cfg.Retry
	if retry == nil {
		retry = DefaultRetry()
	}

	// Логгер: явный из конфига имеет приоритет; иначе — env-based opt-in (OBLODAI_LOG), разбираемый
	// один раз. nil остаётся nil (логирование выключено).
	logger := cfg.Logger
	if logger == nil {
		logger = envLogger()
	}

	c := &Client{
		publicID: cfg.PublicID,
		secret:   cfg.Secret,
		baseURL:  baseURL,
		retry:    retry,
		hc:       hc,
		logger:   logger,
	}
	c.Payments = &PaymentsResource{c}
	c.Payouts = &PayoutsResource{c}
	c.Batches = &BatchesResource{c}
	c.Links = &LinksResource{c}
	c.Splits = &SplitsResource{c}
	c.PayoutLinks = &PayoutLinksResource{c}
	c.Wallets = &WalletsResource{c}
	c.Account = &AccountResource{c}
	c.Webhooks = &WebhooksResource{c}
	c.Settings = &SettingsResource{c}
	c.Rates = &RatesResource{c}
	c.Sandbox = &SandboxResource{c}
	return c, nil
}

// NewFromEnv создаёт клиента из переменных окружения: OBLODAI_PUBLIC_ID и OBLODAI_SECRET
// (обязательны), OBLODAI_BASE_URL (необязательна). Поля переданного cfg перекрывают окружение
// (кроме PublicID/Secret, которые всегда берутся из окружения). Возвращает ошибку, если обязательная
// переменная не задана.
//
//	client, err := oblodai.NewFromEnv(oblodai.Config{})
func NewFromEnv(cfg Config) (*Client, error) {
	publicID := os.Getenv(envPublicID)
	if publicID == "" {
		return nil, errors.New("oblodai: переменная окружения " + envPublicID + " не задана")
	}
	secret := os.Getenv(envSecret)
	if secret == "" {
		return nil, errors.New("oblodai: переменная окружения " + envSecret + " не задана")
	}
	cfg.PublicID = publicID
	cfg.Secret = secret
	if cfg.BaseURL == "" {
		cfg.BaseURL = os.Getenv(envBaseURL) // пусто → New подставит дефолт
	}
	return New(cfg)
}

// logf пишет структурированную запись в логгер клиента. No-op, если логгер не задан.
// НИКОГДА не передавайте сюда секрет, подпись или тело запроса/ответа — только метаданные
// (method/path/status/attempt/delay/ms/code).
func (c *Client) logf(level slog.Level, msg string, args ...any) {
	if c.logger == nil {
		return
	}
	c.logger.Log(context.Background(), level, msg, args...)
}

// retryReason классифицирует ошибку для лога повтора: "429 rate limit" / "5xx" / "network".
func retryReason(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.Status == 429 {
			return "429 rate limit"
		}
		return "5xx"
	}
	return "network"
}

// newIdempotencyKey генерирует UUID v4 (RFC 4122) из crypto/rand — ключ идемпотентности для
// заголовка Idempotency-Key. Без внешних зависимостей.
func newIdempotencyKey() string {
	var b [16]byte
	_, _ = cryptorand.Read(b[:]) // crypto/rand.Read не возвращает частичного чтения
	b[6] = (b[6] & 0x0f) | 0x40  // версия 4
	b[8] = (b[8] & 0x3f) | 0x80  // вариант RFC 4122
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// request выполняет подписанный POST-запрос и разбирает result из конверта в out.
func (c *Client) request(ctx context.Context, path string, payload any, out any) error {
	return c.execute(ctx, http.MethodPost, path, payload, true, "", out)
}

// requestIdem выполняет подписанный POST-запрос НА СОЗДАЮЩИЙ эндпоинт (обёрнутый бэкендом в
// withIdempotency) с заголовком Idempotency-Key.
//
// Ключ генерируется ОДИН РАЗ, до цикла повторов, — все внутренние ретраи шлют один и тот же
// заголовок, и шлюз дедуплицирует повтор неидемпотентного POST (без риска двойного платежа или
// выплаты). Заголовок НЕ входит в подпись запроса (подписываются только timestamp/метод/путь/тело).
//
// Свой ключ можно передать полем body["idempotency_key"]: оно вырезается из тела (в копии — карта
// вызывающего не мутируется) и уходит заголовком.
func (c *Client) requestIdem(ctx context.Context, path string, body Params, out any) error {
	key := ""
	if v, ok := body["idempotency_key"]; ok {
		if s, isStr := v.(string); isStr && strings.TrimSpace(s) != "" {
			key = strings.TrimSpace(s)
		}
		// Служебное поле не должно уйти в тело — вырезаем его из ПОВЕРХНОСТНОЙ КОПИИ,
		// исходную карту вызывающего не трогаем.
		clean := make(Params, len(body))
		for k, val := range body {
			if k != "idempotency_key" {
				clean[k] = val
			}
		}
		body = clean
	}
	return c.requestIdemKey(ctx, path, body, key, out)
}

// requestIdemKey — как requestIdem, но тело произвольного типа (структура, срез), а ключ
// передаётся ОТДЕЛЬНЫМ аргументом, а не служебным полем тела. Нужен там, где тело — типизированная
// структура (напр. PayoutLinkParams) и вырезать из неё поле нельзя.
//
// Пустой key — сгенерировать UUID v4. Как и в requestIdem, ключ фиксируется ДО цикла повторов:
// все внутренние ретраи одного вызова шлют один и тот же заголовок.
func (c *Client) requestIdemKey(ctx context.Context, path string, body any, key string, out any) error {
	key = strings.TrimSpace(key)
	if key == "" {
		key = newIdempotencyKey()
	}
	return c.execute(ctx, http.MethodPost, path, body, true, key, out)
}

// requestPublic выполняет запрос БЕЗ подписи (публичные эндпоинты).
func (c *Client) requestPublic(ctx context.Context, path string, payload any, out any) error {
	return c.execute(ctx, http.MethodPost, path, payload, false, "", out)
}

// requestPublicGET выполняет публичный GET-запрос без подписи (напр. GET /v1/currencies).
func (c *Client) requestPublicGET(ctx context.Context, path string, out any) error {
	return c.execute(ctx, http.MethodGet, path, nil, false, "", out)
}

// requestSignedGET выполняет ПОДПИСАННЫЙ GET-запрос (напр. GET /v1/sandbox/webhooks).
// Каноническая строка подписи та же, что у POST — "{ts}\nGET\n{path}\n{body}" — с пустым телом.
func (c *Client) requestSignedGET(ctx context.Context, path string, out any) error {
	return c.execute(ctx, http.MethodGet, path, nil, true, "", out)
}

func (c *Client) execute(ctx context.Context, method, path string, payload any, signed bool, idemKey string, out any) error {
	attempts := 1
	if c.retry != nil && c.retry.MaxAttempts > 1 {
		attempts = c.retry.MaxAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		c.logf(slog.LevelDebug, "oblodai: request", "method", method, "path", path, "attempt", attempt, "attempts", attempts)
		result, err := c.once(ctx, method, path, payload, signed, idemKey)
		if err == nil {
			if out != nil && len(result) > 0 {
				if jsonErr := json.Unmarshal(result, out); jsonErr != nil {
					return fmt.Errorf("oblodai: не удалось разобрать ответ: %w", jsonErr)
				}
			}
			return nil
		}
		lastErr = err
		if !isRetriable(err) || attempt == attempts {
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				c.logf(slog.LevelWarn, "oblodai: request failed", "status", apiErr.Status, "code", apiErr.Code, "method", method, "path", path)
			}
			return err
		}
		// Уважаем Retry-After от сервера (напр. 429), иначе — собственный backoff.
		delay := c.backoff(attempt)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
			// Уважаем серверный Retry-After как есть (не зажимаем к MaxDelay), но зажимаем в
			// диапазон [0, maxRetryAfter]: верхняя граница — против чрезмерного ожидания, нижняя —
			// против отрицательной длительности (например, при переполнении time.Duration), из-за
			// которой time.After сработал бы мгновенно и дал busy-retry.
			delay = apiErr.RetryAfter
			if delay > maxRetryAfter {
				delay = maxRetryAfter
			}
			if delay < 0 {
				delay = 0
			}
		}
		c.logf(slog.LevelWarn, "oblodai: retrying", "method", method, "path", path, "delay_ms", delay.Milliseconds(), "reason", retryReason(err), "next_attempt", attempt+1)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

// once делает один HTTP-запрос и возвращает сырой result (или ошибку).
func (c *Client) once(ctx context.Context, method, path string, payload any, signed bool, idemKey string) ([]byte, error) {
	var bodyBytes []byte
	if method != http.MethodGet {
		if payload == nil {
			payload = map[string]any{}
		}
		var err error
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("oblodai: не удалось сериализовать тело: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, &ConnectionError{Message: "не удалось создать запрос", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")
	if idemKey != "" {
		// Ключ идемпотентности стабилен между повторами (сгенерирован до цикла в requestIdem)
		// и НЕ входит в подпись — подписываются только timestamp/метод/путь/тело.
		req.Header.Set("Idempotency-Key", idemKey)
	}

	if signed {
		// Для GET bodyBytes == nil → string(nil) == "" — подписывается пустое тело.
		ts, sig := signRequest(c.secret, method, path, string(bodyBytes), "")
		req.Header.Set("X-Public-Id", c.publicID)
		req.Header.Set("X-Timestamp", ts)
		req.Header.Set("X-Signature", sig)
	}

	start := time.Now()
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, &ConnectionError{Message: "сетевая ошибка при запросе " + path, Cause: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ConnectionError{Message: "не удалось прочитать ответ", Cause: err}
	}
	c.logf(slog.LevelDebug, "oblodai: response", "status", resp.StatusCode, "method", method, "path", path, "ms", time.Since(start).Milliseconds())

	return parseResponse(resp.StatusCode, respBody, parseRetryAfter(resp.Header.Get("Retry-After")))
}

// parseRetryAfter разбирает заголовок Retry-After в длительность. Поддерживает форму «секунды»
// (как отдаёт шлюз на 429: Retry-After: 60). HTTP-date форму не используем. 0 — заголовка нет.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	secs, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil || secs < 0 {
		return 0
	}
	// Зажимаем к потолку ДО умножения: огромное значение секунд иначе переполнило бы time.Duration
	// (int64 наносекунд) и могло дать отрицательную длительность → time.After сработал бы мгновенно
	// (busy-retry). Результат всегда в диапазоне [0, maxRetryAfter].
	if secs > int(maxRetryAfter/time.Second) {
		return maxRetryAfter
	}
	return time.Duration(secs) * time.Second
}

// parseResponse разбирает ответ: возвращает result из конверта или *APIError.
func parseResponse(status int, body []byte, retryAfter time.Duration) ([]byte, error) {
	// Пытаемся разобрать как объект с полями error/result.
	var envelope map[string]json.RawMessage
	if len(body) > 0 {
		if err := json.Unmarshal(body, &envelope); err != nil {
			// Не JSON-объект (может быть массив или мусор).
			if status >= 200 && status < 300 {
				return body, nil // вернём как есть — вызывающий разберёт (напр. массив)
			}
			return nil, &APIError{Code: "response.not_json", Message: fmt.Sprintf("ответ не является JSON-объектом (HTTP %d)", status), Status: status, Raw: body}
		}
	}

	// Конверт ошибки.
	if errRaw, ok := envelope["error"]; ok {
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(errRaw, &e)
		if e.Code == "" {
			e.Code = "unknown"
		}
		return nil, &APIError{Code: e.Code, Message: e.Message, Status: status, Raw: body, RetryAfter: retryAfter}
	}

	// Не-2xx без конверта ошибки. Сюда попадает и 429 (тело {"state":1,"message":"rate limit exceeded"}
	// без ключа "error") — достаём message из тела и учитываем Retry-After.
	if status < 200 || status >= 300 {
		msg := fmt.Sprintf("HTTP %d", status)
		if raw, ok := envelope["message"]; ok {
			var m string
			if json.Unmarshal(raw, &m) == nil && m != "" {
				msg = m
			}
		}
		return nil, &APIError{Code: "http." + strconv.Itoa(status), Message: msg, Status: status, Raw: body, RetryAfter: retryAfter}
	}

	// Успешный конверт { state: 0, result: ... }.
	if result, ok := envelope["result"]; ok {
		return result, nil
	}

	// Ответ без конверта (например, POST /v1/webhooks) — возвращаем всё тело.
	return body, nil
}

func isRetriable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsRetriable()
	}
	var connErr *ConnectionError
	if errors.As(err, &connErr) {
		return true
	}
	return false
}

func (c *Client) backoff(attempt int) time.Duration {
	r := c.retry
	base := math.Min(float64(r.InitialDelay)*math.Pow(2, float64(attempt-1)), float64(r.MaxDelay))
	jitter := rand.Float64() * float64(r.InitialDelay) / 2
	return time.Duration(base + jitter)
}
