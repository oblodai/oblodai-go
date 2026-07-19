package oblodai

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"
)

// webhookLogf логирует событие проверки вебхука через env-based логгер (OBLODAI_LOG). No-op, если
// он не задан. VerifyWebhook — пакетная функция без клиента, поэтому логгер берётся из окружения.
// НИКОГДА не передавайте сюда секрет, подпись или тело — только причину/метаданные.
func webhookLogf(level slog.Level, msg string, args ...any) {
	l := envLogger()
	if l == nil {
		return
	}
	l.Log(context.Background(), level, msg, args...)
}

// WebhookHeaders — заголовки доставки вебхука, нужные для проверки.
type WebhookHeaders struct {
	Timestamp string // X-Webhook-Timestamp
	Signature string // X-Webhook-Signature
}

// DefaultWebhookMaxAgeSeconds — окно свежести вебхука по умолчанию (replay-защита), совпадает с
// остальными SDK Oblodai.
const DefaultWebhookMaxAgeSeconds = 300

// DisableWebhookMaxAge — сентинел для MaxAgeSeconds, ЯВНО отключающий проверку свежести.
// Отключайте только осознанно: без окна старый, но корректно подписанный вебхук можно переиграть.
const DisableWebhookMaxAge = -1

// VerifyOptions — параметры проверки вебхука.
type VerifyOptions struct {
	// MaxAgeSeconds — окно свежести для replay-защиты, в секундах.
	//
	// НУЛЕВОЕ ЗНАЧЕНИЕ (поле не задано) = DefaultWebhookMaxAgeSeconds (300), а НЕ «выключено»:
	// иначе &VerifyOptions{Now: t} — единственный способ подставить часы — молча снимал бы
	// replay-защиту. Отключить окно можно только явным сентинелом MaxAgeSeconds:
	// oblodai.DisableWebhookMaxAge (-1). Любое положительное значение — своё окно.
	MaxAgeSeconds int
	// Now — текущее время (для тестов). Нулевое значение = time.Now().
	Now time.Time
}

// VerifyWebhook проверяет подпись и свежесть вебхука. Возвращает nil при успехе, иначе *SignatureError.
//
// ВАЖНО: endpointSecret — это СЕКРЕТ ЭНДПОИНТА ВЕБХУКОВ, то есть поле Secret из ответа
// Webhooks.Register(ctx, url). Это ОТДЕЛЬНЫЙ секрет, он НЕ равен Config.Secret (секрету
// API-ключа, которым подписываются исходящие запросы). Подставив сюда секрет API-ключа, вы
// отвергнете 100% вебхуков — подпись не сойдётся никогда.
//
// ВАЖНО: rawBody должен быть СЫРЫМ телом запроса — тем же, что пришло по сети. Не передавайте
// пересериализованный JSON: подпись считается по байтам.
//
// Пробные тела ("is_test": true) НЕ подписаны — их этой функцией проверять не нужно.
//
// Replay-защита включена всегда: окно 300 секунд применяется и при opts == nil, и при
// &VerifyOptions{} с незаполненным MaxAgeSeconds. Выключить — только MaxAgeSeconds:
// oblodai.DisableWebhookMaxAge.
func VerifyWebhook(endpointSecret string, rawBody []byte, headers WebhookHeaders, opts *VerifyOptions) error {
	if headers.Timestamp == "" || headers.Signature == "" {
		webhookLogf(slog.LevelWarn, "oblodai: webhook verification failed", "reason", "missing timestamp or signature")
		return &SignatureError{Message: "отсутствует timestamp или signature вебхука"}
	}

	expected := ComputeWebhookSignature(endpointSecret, headers.Timestamp, rawBody)
	if !hmac.Equal([]byte(expected), []byte(headers.Signature)) {
		webhookLogf(slog.LevelWarn, "oblodai: webhook verification failed", "reason", "signature mismatch")
		return &SignatureError{Message: "подпись вебхука не совпадает"}
	}

	// Окно свежести: 0 (незаданное поле) = дефолт, а не «выключено». Выключает только явный
	// сентинел DisableWebhookMaxAge (см. VerifyOptions.MaxAgeSeconds).
	maxAge := DefaultWebhookMaxAgeSeconds
	var now time.Time
	if opts != nil {
		if opts.MaxAgeSeconds != 0 {
			maxAge = opts.MaxAgeSeconds
		}
		now = opts.Now
	}
	if maxAge > 0 {
		ts, err := strconv.ParseInt(headers.Timestamp, 10, 64)
		if err != nil {
			webhookLogf(slog.LevelWarn, "oblodai: webhook verification failed", "reason", "invalid timestamp")
			return &SignatureError{Message: "некорректный timestamp вебхука"}
		}
		if now.IsZero() {
			now = time.Now()
		}
		age := math.Abs(float64(now.Unix() - ts))
		if age > float64(maxAge) {
			webhookLogf(slog.LevelWarn, "oblodai: webhook verification failed", "reason", "too old (replay protection)", "age_s", int64(age), "max_age_s", maxAge)
			return &SignatureError{Message: "вебхук слишком старый (replay-защита)"}
		}
	}

	webhookLogf(slog.LevelDebug, "oblodai: webhook signature ok")
	return nil
}

// ConstructEvent проверяет вебхук и разбирает тело в out (указатель на структуру или map).
// Бросает *SignatureError при неверной подписи.
//
// endpointSecret — секрет ЭНДПОИНТА вебхуков (поле Secret из Webhooks.Register), а НЕ Config.Secret
// вашего API-ключа. См. VerifyWebhook.
func ConstructEvent(endpointSecret string, rawBody []byte, headers WebhookHeaders, opts *VerifyOptions, out any) error {
	if err := VerifyWebhook(endpointSecret, rawBody, headers, opts); err != nil {
		return err
	}
	return json.Unmarshal(rawBody, out)
}

// WebhookHeadersFromRequest извлекает заголовки вебхука из http.Request.
func WebhookHeadersFromRequest(r *http.Request) WebhookHeaders {
	return WebhookHeaders{
		Timestamp: r.Header.Get("X-Webhook-Timestamp"),
		Signature: r.Header.Get("X-Webhook-Signature"),
	}
}
