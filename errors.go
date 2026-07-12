package oblodai

import (
	"fmt"
	"time"
)

// APIError — ошибка, вернувшаяся от API (конверт "error").
//
// Все ошибки API приходят в конверте {"error": {"code", "message"}}, где Code — машиночитаемый
// идентификатор вида <домен>.<причина> (например "payout.insufficient_funds"). Ветвитесь по Code,
// а не по тексту Message.
type APIError struct {
	Code    string // машиночитаемый код вида <домен>.<причина>
	Message string // человекочитаемое пояснение
	Status  int    // HTTP-статус ответа
	Raw     []byte // сырое тело ответа (для отладки)
	// RetryAfter — рекомендованная сервером пауза перед повтором (из заголовка Retry-After,
	// например на 429 шлюз отдаёт Retry-After: 60). 0 — заголовка не было.
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("oblodai: API error %s (HTTP %d): %s", e.Code, e.Status, e.Message)
}

// IsRetriable сообщает, временная ли ошибка (стоит ли повторять с backoff).
func (e *APIError) IsRetriable() bool {
	if e.Status >= 500 {
		return true
	}
	if e.Status == 429 {
		return true
	}
	// payout.funds_maturing — терминальная: средства ещё дозревают, автоповтор с backoff почти
	// наверняка получит ту же ошибку. Дождитесь зрелости и повторите вручную.
	return false
}

// ConnectionError — сетевая ошибка (соединение не удалось или таймаут).
// Как правило, безопасно повторить с backoff.
type ConnectionError struct {
	Message string
	Cause   error
}

func (e *ConnectionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("oblodai: %s: %v", e.Message, e.Cause)
	}
	return "oblodai: " + e.Message
}

func (e *ConnectionError) Unwrap() error { return e.Cause }

// IsRetriable всегда true для сетевых ошибок.
func (e *ConnectionError) IsRetriable() bool { return true }

// SignatureError — ошибка проверки подписи вебхука (VerifyWebhook).
type SignatureError struct {
	Message string
}

func (e *SignatureError) Error() string {
	return "oblodai: signature error: " + e.Message
}
