# Oblodai Go SDK

Официальный Go SDK для платёжного шлюза **Oblodai**: приём платежей, выплаты, массовые операции
(батчи), платёжные ссылки, payout-ссылки (крипто-чеки), сплиты, статические кошельки, вебхуки.
Без внешних зависимостей — только стандартная библиотека. Автоподпись запросов, разбор ответов,
типизированные ошибки и автоматические повторы.

> **Базовый URL.** По умолчанию — `https://api.oblodai.com`. При необходимости переопределите `BaseURL` и свои ключи при инициализации.

## Установка

```bash
go get github.com/oblodai/oblodai-go
```

Требуется Go 1.22.2+.

## Учётные данные

Храните ключи в переменных окружения (см. `.env.example`):

```bash
export OBLODAI_PUBLIC_ID=oblodai_...
export OBLODAI_SECRET=oblodai_live_...
# необязательно: export OBLODAI_BASE_URL=https://api.oblodai.com
```

```go
// читает OBLODAI_PUBLIC_ID / OBLODAI_SECRET / OBLODAI_BASE_URL; поля Config перекрывают окружение
client, err := oblodai.NewFromEnv(oblodai.Config{})
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
		// Retry: nil — дефолтные повторы; oblodai.NoRetry() — отключить
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

## Идемпотентность (изменилось в v1.1.0)

Защита от дублей при повторах — заголовок **`Idempotency-Key`**: на создающих вызовах
(`Payments.Create` / `Refund` / `Resolve` / `*Batch`, `Payouts.Create` / `CreateMass` / `CreateBatch`,
`Account.TransferToPersonal` / `TransferToUser` / `TransferBatch`) SDK генерирует UUID **один раз до
цикла повторов**, поэтому все
внутренние ретраи шлют один и тот же ключ, и шлюз дедуплицирует повтор. Заголовок не входит в
подпись запроса.

- **`order_id` уходит как есть.** SDK его больше **не подставляет и не переписывает** (ломающее
  изменение: в v1.0.x пустой `order_id` заменялся сгенерированным). `order_id` — ваш
  бизнес-идентификатор для поиска через `Payments.Info`; для выплат он обязателен всегда.
- **Свой ключ идемпотентности**: передайте `params["idempotency_key"]` — он уйдёт в заголовок
  (и будет вырезан из тела).
- **Payout-ссылки** (`PayoutLinks.Create` / `CreateBatch`) тоже резервируют деньги и тоже
  автоматически ретраятся, поэтому SDK шлёт на них `Idempotency-Key` — и **шлюз его уважает**:
  `/v1/payout/link` и `/v1/payout/link/batch` обёрнуты в идемпотентность. Повтор с тем же ключом
  реплеит первый ответ (та же ссылка, тот же `claim_token`) с заголовком
  `Idempotent-Replayed: true`, а баланс дебетуется **ровно один раз**. Без заголовка поведение
  прежнее: два одинаковых вызова создадут две ссылки. Второй, durable слой — per-link
  `Reference` (уникальный индекс на `(merchant_id, reference)`): работает даже без заголовка,
  дубль даёт `payoutlink.duplicate_reference` (409).
- **`Wallets.BlockedAddressRefund`** намеренно **не** обёрнут в идемпотентность на шлюзе — и не
  нуждается: он идемпотентен по состоянию, причём сильнее заголовка. Шлюз строит
  детерминированную ссылку `refund-wallet:<wallet_id>`, берёт per-wallet advisory-lock и внутри
  лока возвращает уже существующую выплату, если она есть. Повтор — с заголовком или без —
  возвращает **ту же** выплату; конкурентные повторы дожидаются результата, а не получают 409.
  Оговорка: адрес не входит в ссылку, поэтому повтор с **другим** адресом вернёт первую выплату
  на **первый** адрес.
- **`Payouts.Approve`** — переход состояния, а не создание: заголовок не нужен. Шлюз принимает
  только `pending` и иначе отвечает `payout.not_pending` (409). Читайте этот 409 как **«уже
  одобрено»**, а не как провал, и уточняйте статус через `Payouts.Info`.
- **Свой ключ на этих вызовах**: `PayoutLinkParams.IdempotencyKey`,
  `PayoutLinks.CreateBatchWithKey`, `Wallets.BlockedAddressRefundWithKey`.

### Коды ответа слоя идемпотентности

Эндпоинты, обёрнутые в идемпотентность на шлюзе (включая `/v1/payout/link` и
`/v1/payout/link/batch`), могут вернуть:

| Код | HTTP | Смысл | Ретраит ли SDK |
| --- | --- | --- | --- |
| `idempotency.key_reused` | 400 | тот же ключ с **другим** телом | нет (терминальная) |
| `idempotency.bad_key` | 400 | кривой ключ / длиннее 255 символов | нет (терминальная) |
| `idempotency.in_progress` | 409 | параллельный повтор, пока первый ещё выполняется | нет — повторите сами чуть позже с **тем же** ключом |
| `idempotency.unavailable` | 503 | стор идемпотентности недоступен (fail-closed by design) | **да**, автоматически |
| `payoutlink.duplicate_reference` | 409 | дубль `Reference` (раньше было 500) | нет (терминальная) |

Классификация в SDK (`APIError.IsRetriable`) уже соответствует: ретраятся только 5xx и 429,
а 4xx — никогда. Переход `duplicate_reference` с 500 на 409 поэтому важен: 500 SDK ретраил
впустую, 409 отдаётся вам сразу.

**Батчи.** Частично упавший батч реплеится **как есть**: упавшие элементы не повторяются под тем
же ключом — шлите их **новым** ключом. И ответ батча больше **256 КБ шлюз не кэширует**, тогда
повтор выполнится заново — поэтому на батчах проставляйте **per-item `Reference`** как второй
слой защиты.

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

## Повторы (retry) — включены по умолчанию (изменилось в v1.1.0)

Временные ошибки (`5xx`, `429`, сетевые сбои) повторяются автоматически с экспоненциальным backoff
и джиттером (до 4 попыток, старт 500 мс, потолок 30 с). Ошибки запроса (`4xx`, кроме 429) не
повторяются. Заголовок `Retry-After` уважается как есть (до потолка в 5 минут).

`Retry: nil` теперь означает **дефолтные повторы** — как и в остальных SDK Oblodai (в v1.0.x `nil`
означал «повторов нет»). Отключайте осознанно:

```go
client, _ := oblodai.New(oblodai.Config{
	PublicID: "...", Secret: "...",
	Retry: oblodai.NoRetry(), // отключить повторы
	// либо свои настройки:
	// Retry: &oblodai.RetryConfig{MaxAttempts: 4, InitialDelay: 500 * time.Millisecond, MaxDelay: 30 * time.Second},
})
```

> **Важно про таймаут.** Таймаут не означает, что операция не прошла. Все внутренние ретраи
> создающих вызовов идут с одним и тем же `Idempotency-Key`, поэтому на эндпоинтах, обёрнутых в
> идемпотентность на шлюзе, дубля не будет — если операция уже создана, шлюз вернёт её же.
>
> Это относится и к `PayoutLinks.Create` / `CreateBatch` — они обёрнуты в идемпотентность на
> шлюзе, поэтому отключать для них повторы не нужно (это была бы деградация). У
> `Wallets.BlockedAddressRefund` заголовка на шлюзе нет, но он идемпотентен по состоянию через
> детерминированную ссылку под advisory-lock: повтор возвращает ту же выплату. Подробнее — раздел
> «Идемпотентность».

## Массовые операции (v1.1.0)

До 5000 платежей / возвратов / выплат одним подписанным запросом — одна отметка rate-limit.
Обработка фоновая: постановка возвращает `batch_id`, результаты — через `Batches.Info`.

```go
sub, err := client.Payments.CreateBatch(ctx, []oblodai.Params{
	{"amount": "10", "currency": "USD", "order_id": "a-1", "to_currency": "USDT", "network": "tron"},
	{"amount": "20", "currency": "EUR", "order_id": "a-2", "to_currency": "USDT", "network": "tron"},
}, "continue") // "continue" (по умолчанию) или "stop"

info, err := client.Batches.Info(ctx, sub.BatchID, 100, 0) // прогресс и результат по каждому элементу
```

`order_id` обязателен на каждом элементе платежей/выплат; для возвратов — `reference` +
`uuid`/`order_id` инвойса.

## Платёжные ссылки, сплиты, счёт на e-mail (v1.1.0)

```go
// Платёжная ссылка: платят многие, каждый платёж — свой инвойс. Принимает деньги без вашего бэкенда.
link, err := client.Links.Create(ctx, oblodai.LinkParams{AmountMode: "open", Currency: "USD"})

// Сплит: доля каждого входящего платежа автоматически уходит партнёру.
rule, err := client.Splits.SplitToAddress(ctx, "T...", "tron", 10.0, "партнёр А")

// Счёт на e-mail (письмо с кнопкой «Оплатить»).
_, err = client.Payments.SendEmail(ctx, payment.UUID, "" /* orderID */, "buyer@example.com")

// Судьба недоплаченного платежа: принять частичную оплату или вернуть плательщику.
res, err := client.Payments.Resolve(ctx, payment.UUID, "", "accept", nil)
```

## Payout-ссылки — крипто-чеки (v1.1.0)

Зарезервируйте средства, **не зная кошелька получателя**: получатель откроет `ClaimURL`, введёт свой
адрес — и из резерва породится обычная выплата.

```go
link, err := client.PayoutLinks.Create(ctx, oblodai.PayoutLinkParams{
	Currency: "USDT", Network: "tron", Amount: "25",
	Reference:      "bonus-42", // ЗАДАВАЙТЕ ВСЕГДА: второй, durable слой защиты от дублей — дубль
	                            // reference даёт 409 payoutlink.duplicate_reference. Работает и без
	                            // заголовка, и на батчах, чей ответ слишком велик для кэша (>256 КБ)
	ExpiresInHours: 168,        // ЗАДАВАЙТЕ ЯВНО: при 0 бэкенд клампит к минимуму — 1 час
	Email:          "user@example.com", // необязательно: письмо со ссылкой claim
})
// link.ClaimToken / link.ClaimURL возвращаются ТОЛЬКО здесь — сохраните сразу.

// Публичные методы для своей страницы claim (без ключей):
info, err := client.PayoutLinks.ClaimInfo(ctx, token)          // GET /v1/claim/{token}
claim, err := client.PayoutLinks.Claim(ctx, token, "T-адрес")  // POST /v1/claim/{token}
```

Статусы ссылки: `funded` → `claiming` → `claimed`; либо `expired` / `cancelled`
(константы `oblodai.PayoutLinkStatus*`). До 500 ссылок за раз — `PayoutLinks.CreateBatch`.

## Песочница и тестирование (v1.2.0)

У шлюза есть песочница разработчика. **Бизнес-эндпоинты и код интеграции одинаковы** для теста и
прода — меняется только ключ: тестовый `public_id` начинается с `test_…`, тестовый секрет — с
`oblodai_test_…`. Переключение тест ↔ прод = замена пары ключей, ни строчки кода.

Новое — пять test-only помощников (`client.Sandbox`), которых в проде нет: они заменяют
«покупатель оплатил в сети». **Только для тестового кода** — не зовите их из продовой интеграции:
живой ключ получает на них HTTP 403 с кодом `sandbox.live_key`. Проверить ключ можно хелпером
`oblodai.IsTestKey(publicID)`.

```go
client, _ := oblodai.New(oblodai.Config{
	PublicID: "test_...",         // тестовый ключ — всё остальное как в проде
	Secret:   "oblodai_test_...",
})

// 1. Создаём инвойс — ровно тем же кодом, что и в проде.
payment, _ := client.Payments.Create(ctx, oblodai.Params{
	"amount": "10", "currency": "USD", "order_id": "order-1",
	"to_currency": "USDT", "network": "tron",
})

// 2. «Оплачиваем» его: в проде это делает покупатель он-чейн, в песочнице — вы.
dep, _ := client.Sandbox.SimulateDeposit(ctx, oblodai.SandboxDepositParams{
	InvoiceID: payment.UUID, // Amount пустой = оплатить ровно сумму инвойса
})
_ = dep.TxID // повторите тот же TxID с бОльшим Confirmations, чтобы углубить депозит

// 3. Дожидаемся paid как обычно — вебхуком или поллингом.
info, _ := client.Payments.Info(ctx, payment.UUID, "")

// 4. Баланс для выплат «чеканится» краном (потолок 1000000 за вызов)…
_, _ = client.Sandbox.Faucet(ctx, "USDT", "1000")

// 5. …и выплата снова обычным продовым кодом.
_, _ = client.Payouts.Create(ctx, oblodai.Params{
	"amount": "25", "currency": "USDT", "network": "tron",
	"address": "T...", "order_id": "payout-1",
})
```

Остальные помощники: `Sandbox.Reset` (отменить открытые инвойсы, обнулить балансы),
`Sandbox.ListWebhooks` (последние ≤50 доставок вебхуков, новые первыми, с сырым `Payload`) и
`Sandbox.ReplayWebhook(deliveryID)` (повторно поставить доставку в очередь) — удобно отлаживать
свой обработчик вебхуков.

**Нюансы:**

- **Недоплата/переплата** — передайте `Amount` меньше/больше суммы инвойса; недоплату дальше
  решает `Payments.Resolve` (`accept`/`refund`), как в проде.
- **Неглубокие подтверждения сами НЕ дозревают.** У симулированного депозита нет цепочки: его никто
  не переэмитит, курсор для него не двигается — инвойс с `confirmations` меньше требуемых висит в
  `confirm_check` сколь угодно долго. Довести его до `paid` можно ровно одним способом: повторить
  `SimulateDeposit` с **тем же `TxID`** и бОльшим `Confirmations` (повтор того же `TxID`
  идемпотентен — сумма не удваивается).
- **~10 минут — это про другое.** Десятиминутное ожидание относится к maturity-**холду на выплате**
  (ошибка `payout.funds_maturing`): в песочнице холд снимает по возрасту фоновый джоб, по умолчанию
  через 10 минут (`GATEWAY_SANDBOX_MATURITY_MINUTES`). Это доступность средств к выводу, а не
  подтверждения инвойса — на статус инвойса джоб не влияет.
- **UTXO-сети (Bitcoin и т.п.)** ведут себя как в проде: **нет** авто-возврата переплаты и **нет**
  адреса плательщика (`payer_address`).
- `Sandbox.ListWebhooks` — единственный подписанный `GET` в API: подпись по той же канонической
  строке `{ts}\nGET\n{path}\n` с пустым телом (SDK делает это сам).

## Внутренние переводы пользователям платформы (v1.2.0)

Перевод **без комиссии** с баланса мерчанта на личный кошелёк другого пользователя платформы —
например, выплата исполнителю, у которого есть аккаунт Oblodai. `to_user_id` — **UUID пользователя
на платформе, НЕ username** (username резолвится в id на стороне кабинета). Требует PAYOUT-ключ;
дедуп — та же лестница, что у остальных денежных вызовов: заголовок `Idempotency-Key` (SDK шлёт
сам) → `order_id` → подпись.

```go
res, err := client.Account.TransferToUser(ctx, oblodai.Params{
	"to_user_id": "5c3a2c1e-9d0b-4f6a-8f3d-2b1a0c9e8d7f", // UUID получателя, не username
	"amount":     "25",
	"currency":   "USDT",
	"order_id":   "payroll-7", // необязателен
})
// res.RecipientBalance — новый баланс получателя

// «Зарплатная» пачка (до 5000 переводов одним запросом; обработка фоновая):
sub, err := client.Account.TransferBatch(ctx, []oblodai.Params{
	{"to_user_id": "…", "amount": "10", "currency": "USDT", "order_id": "s-1"},
	{"to_user_id": "…", "amount": "20", "currency": "USDT", "order_id": "s-2"},
}, "continue") // "continue" (по умолчанию) или "stop"
info, err := client.Batches.Info(ctx, sub.BatchID, 100, 0) // прогресс и результат по каждой строке
```

## Публичная страница оплаты — свой checkout (v1.2.0)

Публичные методы (`GET /v1/pay/{id}` и `POST /v1/pay/{id}/select`, **без подписи и без ключей**) —
для СВОЕЙ страницы оплаты вместо hosted-страницы шлюза: браузер рендерит и поллит инвойс без
секрета мерчанта.

```go
// Публичное состояние инвойса: адрес, сумма, QR, статус, срок. Мерчант-приватные поля
// (additional_data, payer_email, payer_address) бэкенд вырезает.
pub, err := client.Payments.PublicGet(ctx, payment.UUID)

// Для валюто-агностичного инвойса (payment_status "select") pub.Accepted — методы на выбор;
// выбор фиксирует курс, выделяет депозит-адрес и возвращает финализированный инвойс.
p, err := client.Payments.PublicSelect(ctx, payment.UUID, "USDT", "tron")
```

## Обзор методов

```go
// Платежи
client.Payments.Create(ctx, params)
client.Payments.CreateBatch(ctx, payments, onError)   // v1.1.0
client.Payments.RefundBatch(ctx, refunds, onError)    // v1.1.0
client.Payments.SendEmail(ctx, uuid, orderID, email)  // v1.1.0
client.Payments.Resolve(ctx, uuid, orderID, action, opts) // v1.1.0
client.Payments.Info(ctx, uuid, orderID)
client.Payments.History(ctx, params)
client.Payments.Services(ctx)
client.Payments.QR(ctx, uuid, orderID)
client.Payments.Resend(ctx, uuid, orderID)
client.Payments.Refund(ctx, params)
client.Payments.SetAccepted(ctx, methods) / ListAccepted(ctx)
client.Payments.SetAccuracy(ctx, params) / GetAccuracy(ctx)
client.Payments.SetAutorefund(ctx, params) / GetAutorefund(ctx)
client.Payments.SetDiscount(ctx, params) / ListDiscounts(ctx)
client.Payments.PublicGet(ctx, uuid)                      // v1.2.0, публичный, без подписи
client.Payments.PublicSelect(ctx, uuid, currency, network) // v1.2.0, публичный, без подписи

// Выплаты
client.Payouts.Create(ctx, params)
client.Payouts.CreateMass(ctx, payouts, source)
client.Payouts.CreateBatch(ctx, payouts, onError)     // v1.1.0
client.Payouts.Info(ctx, uuid, orderID)
client.Payouts.History(ctx, params)
client.Payouts.Services(ctx)
client.Payouts.Calculate(ctx, params)
client.Payouts.Approve(ctx, uuid)
client.Payouts.Refund(ctx, params)
client.Payouts.GetFeeConfig(ctx) / SetFeeConfig(ctx, bool)
client.Payouts.GetRefundFeeConfig(ctx) / SetRefundFeeConfig(ctx, bool)

// Пачки (v1.1.0)
client.Batches.Info(ctx, batchID, limit, offset)

// Платёжные ссылки (v1.1.0)
client.Links.Create(ctx, linkParams)
client.Links.List(ctx, limit, offset)
client.Links.Info(ctx, linkID)
client.Links.Toggle(ctx, linkID, active)
client.Links.PublicGet(ctx, linkID)        // публичный, без подписи
client.Links.Checkout(ctx, linkID, params) // публичный, без подписи

// Сплиты (v1.1.0)
client.Splits.CreateRule(ctx, params)
client.Splits.SplitToAddress(ctx, address, network, percent, note)
client.Splits.SplitToMerchant(ctx, merchantID, percent, note)
client.Splits.ListRules(ctx) / DeleteRule(ctx, ruleID)
client.Splits.GetConfig(ctx) / SetConfig(ctx, refundHoldHours)

// Payout-ссылки — крипто-чеки (v1.1.0)
client.PayoutLinks.Create(ctx, params)
client.PayoutLinks.CreateBatch(ctx, links) // до 500
client.PayoutLinks.List(ctx, limit, offset)
client.PayoutLinks.Info(ctx, linkID)
client.PayoutLinks.Cancel(ctx, linkID)
client.PayoutLinks.ClaimInfo(ctx, token)                 // публичный, без подписи
client.PayoutLinks.Claim(ctx, token, address)            // публичный, без подписи
client.PayoutLinks.ClaimWithMemo(ctx, token, address, memo)

// Кошельки
client.Wallets.Create(ctx, params)
client.Wallets.Block(ctx, address, forceBlock)
client.Wallets.BlockedAddressRefund(ctx, uuid, address)
client.Wallets.QR(ctx, address)

// Аккаунт
client.Account.Balance(ctx)
client.Account.Referral(ctx)
client.Account.TransferToPersonal(ctx, params)
client.Account.TransferToUser(ctx, params)             // v1.2.0, to_user_id — UUID, не username
client.Account.TransferBatch(ctx, transfers, onError)  // v1.2.0, до 5000, результат — Batches.Info
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
client.Rates.Currencies(ctx)

// Песочница — ТОЛЬКО тестовые ключи (v1.2.0)
client.Sandbox.SimulateDeposit(ctx, params)
client.Sandbox.Faucet(ctx, asset, amount) / FaucetWithKey(ctx, asset, amount, key)
client.Sandbox.Reset(ctx)
client.Sandbox.ListWebhooks(ctx)           // подписанный GET
client.Sandbox.ReplayWebhook(ctx, deliveryID)
oblodai.IsTestKey(publicID)                // "test_…" → true
```

## Логи и отладка

Включаются переменной окружения (`export OBLODAI_LOG=debug`; уровни `debug`/`info`/`warn`/`error`)
или своим логгером: `Config{Logger: slog-логгер}`. Секреты, подпись и тела запросов в лог не
попадают никогда — только метод, путь, статус, номер попытки и задержка ретрая.

## Замечания

- **Суммы — строки** в единицах валюты (`"25.00"`), не числа. Так сохраняется точность.
- **`order_id` — ваш бизнес-идентификатор**, обязателен для выплат; ключом идемпотентности он
  больше не является (см. раздел «Идемпотентность»).
- **Секрет — только на сервере.** SDK серверный; не встраивайте ключ в клиентские приложения.

## Лицензия

MIT
