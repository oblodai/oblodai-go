# Oblodai Go SDK

Официальный Go SDK для платёжного шлюза **Oblodai**: приём платежей, выплаты, статические кошельки,
вебхуки. Без внешних зависимостей — только стандартная библиотека. Автоподпись запросов, разбор
ответов, типизированные ошибки и автоматические повторы.

> **Базовый URL.** По умолчанию — `https://api.oblodai.com`. При необходимости переопределите `BaseURL` и свои ключи при инициализации.

## Установка

```bash
go get github.com/oblodai/oblodai-go
```

Требуется Go 1.21+.

## Учётные данные

Храните ключи в переменных окружения (см. `.env.example`):

```bash
export OBLODAI_PUBLIC_ID=oblodai_...
export OBLODAI_SECRET=oblodai_live_...
# необязательно: export OBLODAI_BASE_URL=https://api.oblodai.com
```

```go
// читает OBLODAI_PUBLIC_ID / OBLODAI_SECRET / OBLODAI_BASE_URL; поля Config перекрывают окружение
client, err := oblodai.NewFromEnv(oblodai.Config{Retry: oblodai.DefaultRetry()})
```

## Быстрый старт

```go
package main

import (
	"context"
	"fmt"
	"log"

	oblodai "github.com/oblodai/oblodai-go"
)

func main() {
	// либо явно (эквивалент NewFromEnv выше):
	client, err := oblodai.New(oblodai.Config{
		PublicID: "oblodai_...",
		Secret:   "oblodai_live_...",
		BaseURL:  "https://api.oblodai.com", // необязательно
		Retry:    oblodai.DefaultRetry(),        // nil — без повторов
	})
	if err != nil {
		log.Fatal(err)
	}

	payment, err := client.Payments.Create(context.Background(), oblodai.Params{
		"amount":      "10",
		"currency":    "USD",
		"order_id":    "order-1",
		"to_currency": "USDT",
		"network":     "tron",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(payment.Address) // адрес для оплаты
	fmt.Println(payment.URL)     // hosted-страница оплаты
}
```

Каждый метод принимает `context.Context` первым аргументом — можно задавать таймауты и отмену.

## Проверка вебхуков

Подпись вебхука отличается от подписи запроса — SDK делает и то, и другое. Для входящих вебхуков
берите **сырое тело** и заголовки `X-Webhook-Timestamp` / `X-Webhook-Signature`.

```go
func handleWebhook(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body) // СЫРОЕ тело — не пересериализовывать

		// Пробные тела (is_test) не подписаны
		var probe struct{ IsTest bool `json:"is_test"` }
		json.Unmarshal(raw, &probe)
		if probe.IsTest {
			w.WriteHeader(200)
			return
		}

		var event map[string]any
		err := oblodai.ConstructEvent(secret, raw, oblodai.WebhookHeadersFromRequest(r), nil, &event)
		if err != nil {
			w.WriteHeader(403) // *SignatureError
			return
		}

		if event["type"] == "payment" && event["status"] == "paid" {
			// пометить заказ event["order_id"] оплаченным (идемпотентно по uuid + status)
		}
		w.WriteHeader(200) // 2xx только после успешной обработки
	}
}
```

`ConstructEvent` / `VerifyWebhook` по умолчанию проверяют свежесть в окне 5 минут. Передайте
`&VerifyOptions{MaxAgeSeconds: 0}`, чтобы отключить replay-защиту.

## Обработка ошибок

Все ошибки API — `*APIError` с машиночитаемым `Code`. Используйте `errors.As`.

```go
_, err := client.Payouts.Create(ctx, oblodai.Params{
	"amount": "25", "currency": "USDT", "network": "tron",
	"address": "T...", "order_id": "payout-1",
})

var apiErr *oblodai.APIError
if errors.As(err, &apiErr) {
	switch apiErr.Code {
	case "payout.insufficient_funds":
		// недостаточно средств
	case "payout.funds_maturing":
		// средства ещё дозревают — терминально (IsRetriable() == false); дождитесь зрелости и повторите
	}
	log.Printf("%s (HTTP %d): %s", apiErr.Code, apiErr.Status, apiErr.Message)
}
```

### Типы ошибок

| Тип | Когда |
|---|---|
| `*APIError` | API вернул конверт `error`. Есть `Code`, `Status`, `IsRetriable()`. |
| `*ConnectionError` | Сеть недоступна или таймаут. |
| `*SignatureError` | Не прошла проверка подписи вебхука. |

## Повторы (retry)

Временные ошибки (`5xx`, `429`, сетевые сбои) повторяются автоматически с экспоненциальным backoff
и джиттером. Ошибки запроса (`4xx`) не повторяются. Заголовок `Retry-After` уважается как есть (до
потолка в 5 минут).

```go
client, _ := oblodai.New(oblodai.Config{
	PublicID: "...", Secret: "...",
	Retry: &oblodai.RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     30 * time.Second,
	},
	// Retry: nil — отключить повторы
})
```

> **Важно про таймаут.** Таймаут не означает, что операция не прошла. Повтор безопасен благодаря
> идемпотентности по `order_id`: если операция уже создана — вернётся она же, дубля не будет.
>
> - **Платежи** (`Payments.Create`) и **перевод на личный кошелёк** (`Account.TransferToPersonal`):
>   если вы не задали `order_id`, SDK автоматически подставит стабильный ключ идемпотентности
>   (`idem-…`) *до* цикла повторов, поэтому все попытки шлют один и тот же `order_id`. Ключ
>   вставляется в копию — переданная вами `Params` не изменяется, и одну карту можно безопасно
>   переиспользовать для нескольких вызовов (каждый получит собственный `order_id`).
> - **Выплаты** (`Payouts.Create` / `CreateMass`) требуют `order_id` — задавайте его сами.

## Обзор методов

```go
// Платежи
client.Payments.Create(ctx, params)
client.Payments.Info(ctx, uuid, orderID)
client.Payments.History(ctx, params)
client.Payments.Services(ctx)
client.Payments.QR(ctx, uuid, orderID)
client.Payments.Resend(ctx, uuid, orderID)
client.Payments.Refund(ctx, params)
client.Payments.SetAccepted(ctx, methods) / ListAccepted(ctx)
client.Payments.SetAccuracy(ctx, params) / GetAccuracy(ctx)
client.Payments.SetAutorefund(ctx, params) / GetAutorefund(ctx)
client.Payments.SetDiscount(ctx, params)

// Выплаты
client.Payouts.Create(ctx, params)
client.Payouts.CreateMass(ctx, payouts, source)
client.Payouts.Info(ctx, uuid, orderID)
client.Payouts.History(ctx, params)
client.Payouts.Services(ctx)
client.Payouts.Calculate(ctx, params)
client.Payouts.Approve(ctx, uuid)
client.Payouts.Refund(ctx, params)
client.Payouts.GetFeeConfig(ctx) / SetFeeConfig(ctx, bool)
client.Payouts.GetRefundFeeConfig(ctx) / SetRefundFeeConfig(ctx, bool)

// Кошельки
client.Wallets.Create(ctx, params)
client.Wallets.Block(ctx, address, forceBlock)
client.Wallets.BlockedAddressRefund(ctx, uuid, address)
client.Wallets.QR(ctx, address)

// Аккаунт
client.Account.Balance(ctx)
client.Account.Referral(ctx)
client.Account.TransferToPersonal(ctx, params)
client.Account.VRCS(ctx, enabled)

// Вебхуки
client.Webhooks.Register(ctx, url)
client.Webhooks.Deliveries(ctx)
client.Webhooks.TestPayment(ctx, params)

// Настройки
client.Settings.ListAutoWithdraw(ctx) / SetAutoWithdraw(ctx, params) / DeleteAutoWithdraw(ctx, currency)
client.Settings.ListAllowlist(ctx) / AddAllowlist(ctx, cidr) / RemoveAllowlist(ctx, cidr) / EnableAllowlist(ctx, bool)

// Курсы (публично, без ключа)
client.Rates.List(ctx, "ETH")
```

## Замечания

- **Суммы — строки** в единицах валюты (`"25.00"`), не числа. Так сохраняется точность.
- **`order_id`/`reference` — ваш ключ идемпотентности.** Обязателен для выплат; для платежей и
  перевода на личный кошелёк SDK подставит его автоматически, если вы не задали.
- **Секрет — только на сервере.** SDK серверный; не встраивайте ключ в клиентские приложения.

## Лицензия

MIT
