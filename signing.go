package oblodai

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

// signRequest считает подпись запроса.
//
// Каноническая строка: "{timestamp}\n{METHOD}\n{path}\n{body}", подписывается секретом по HMAC-SHA256,
// результат — hex в нижнем регистре. Тело подписывается ровно теми байтами, что уходят в сеть.
//
// Если ts == "", используется текущее время.
func signRequest(secret, method, path, body, ts string) (timestamp, signature string) {
	if ts == "" {
		ts = strconv.FormatInt(time.Now().Unix(), 10)
	}
	signingString := ts + "\n" + method + "\n" + path + "\n" + body
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingString))
	return ts, hex.EncodeToString(mac.Sum(nil))
}

// ComputeWebhookSignature считает ожидаемую подпись вебхука для заданных timestamp и сырого тела.
//
// ВНИМАНИЕ: подпись вебхука — другой алгоритм, чем подпись запроса. Здесь подписанная строка это
// "{timestamp}." + сырое_тело (точка-разделитель, без метода и пути).
func ComputeWebhookSignature(secret, timestamp string, rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(rawBody)
	return hex.EncodeToString(mac.Sum(nil))
}
