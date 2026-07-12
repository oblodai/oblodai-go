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

// VerifyOptions — параметры проверки вебхука.
type VerifyOptions struct {
	// MaxAgeSeconds — окно свежести для replay-защиты. 0 отключает проверку. По умолчанию (если
	// структура не передана) применяется 300 секунд.
	MaxAgeSeconds int
	// Now — текущее время (для тестов). Нулевое значение = time.Now().
	Now time.Time
}

// VerifyWebhook проверяет подпись и свежесть вебхука. Возвращает nil при успехе, иначе *SignatureError.
//
// ВАЖНО: rawBody должен быть СЫРЫМ телом запроса — тем же, что пришло по сети. Не передавайте
// пересериализованный JSON: подпись считается по байтам.
//
// Пробные тела ("is_test": true) НЕ подписаны — их этой функцией проверять не нужно.
func VerifyWebhook(secret string, rawBody []byte, headers WebhookHeaders, opts *VerifyOptions) error {
	if headers.Timestamp == "" || headers.Signature == "" {
		webhookLogf(slog.LevelWarn, "oblodai: webhook verification failed", "reason", "missing timestamp or signature")
		return &SignatureError{Message: "отсутствует timestamp или signature вебхука"}
	}

	expected := ComputeWebhookSignature(secret, headers.Timestamp, rawBody)
	if !hmac.Equal([]byte(expected), []byte(headers.Signature)) {
		webhookLogf(slog.LevelWarn, "oblodai: webhook verification failed", "reason", "signature mismatch")
		return &SignatureError{Message: "подпись вебхука не совпадает"}
	}

	maxAge := 300
	var now time.Time
	if opts != nil {
		maxAge = opts.MaxAgeSeconds
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
func ConstructEvent(secret string, rawBody []byte, headers WebhookHeaders, opts *VerifyOptions, out any) error {
	if err := VerifyWebhook(secret, rawBody, headers, opts); err != nil {
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
