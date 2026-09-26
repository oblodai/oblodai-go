<div align="center">

<a href="https://oblodai.com">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/oblodai/.github/main/brand/logo-white.svg">
    <img src="https://raw.githubusercontent.com/oblodai/.github/main/brand/logo-black.svg" alt="oblodai" height="52">
  </picture>
</a>

<h3>Официальный Go SDK платёжного шлюза <a href="https://oblodai.com">oblodai</a></h3>

Приём платежей, выплаты, платёжные ссылки, сплиты, статические кошельки, вебхуки — один API-ключ.

<a href="https://pkg.go.dev/github.com/oblodai/oblodai-go/v2"><img src="https://pkg.go.dev/badge/github.com/oblodai/oblodai-go/v2.svg" alt="Go Reference"></a>
<img src="https://img.shields.io/github/go-mod/go-version/oblodai/oblodai-go?style=flat-square" alt="Go version">
<a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-000000?style=flat-square" alt="License: MIT"></a>

[Документация](https://docs.oblodai.com) · [Кабинет](https://my.oblodai.com) · [Read in English →](README.md)

</div>

---

Официальный Go SDK платёжного шлюза **Oblodai**: приём платежей, выплаты, массовые операции
(пакеты), платёжные ссылки, ссылки на выплату (крипточеки), сплиты, статические кошельки, переводы,
документы, вебхуки. Подпись запросов, типизированные модели и ошибки, идемпотентность, повторы,
постраничность и ожидание долгих операций — из коробки. Go ≥ 1.25 и только стандартная библиотека:
**ни одной сторонней зависимости**.

Вся поверхность API — сервисы, методы, модели запросов и ответов, перечисления — сгенерирована из
OpenAPI-контракта шлюза (`zz_generated_*.go`, руками не правятся), поэтому у каждой операции шлюза
здесь есть метод, названный так же, как во всех восьми SDK Oblodai. Рукописный runtime вокруг них
подписывает, повторяет, листает страницы и проверяет подписи.

> **Базовый URL.** По умолчанию `https://api.oblodai.com`, меняется `WithBaseURL`. Схема —
> `https://`; обычный `http://` принимается только для loopback (`http://127.0.0.1:8095`) или с
> `WithInsecureBaseURL(true)` (либо `OBLODAI_ALLOW_INSECURE=1`).

## Установка

```bash
go get github.com/oblodai/oblodai-go/v2@v2.0.0
```

Go ≥ 1.25. Модуль — `github.com/oblodai/oblodai-go/v2`; проверка вебхуков живёт в отдельном
подпакете `github.com/oblodai/oblodai-go/v2/webhooks`, которому не нужны ни клиент, ни API-ключ.
Переходите с 1.x — читайте [MIGRATION-2.0.md](MIGRATION-2.0.md).

## Где взять ключи

У мерчанта **один API-ключ**, он выдаётся в [кабинете](https://my.oblodai.com) → **API-ключи**:
публичный id `oblodai_<hex>` и секрет `oblodai_live_<hex>`. Он подписывает все закрытые маршруты —
и платежи, и выплаты. Выбирать на каждый вызов нечего:

```go
client, err := oblodai.New(oblodai.WithCredentials(publicID, secret))
```

Из окружения берутся `OBLODAI_PUBLIC_ID` / `OBLODAI_SECRET`. Пара песочницы (публичный id
`test_oblodai_<hex>`, секрет `oblodai_test_<hex>`) работает с копией шлюза без блокчейна.
Подключение магазина песочницы (`Sandbox.OnboardStore`) не подписывается; свой шлюз закрывает его
**токеном администратора** (`WithAdminToken` или `OBLODAI_ADMIN_TOKEN`).

## Быстрый старт

Каждый метод принимает первым `context.Context`, вторым — параметры моделью, последними —
необязательные опции вызова. Необязательные поля модели — указатели, их делает `oblodai.Ptr(v)`.
Создать счёт:

```go
client, err := oblodai.New() // OBLODAI_PUBLIC_ID / OBLODAI_SECRET from the environment
if err != nil {
	log.Fatal(err)
}
invoice, err := client.Payments.Create(ctx, &oblodai.PaymentRequest{
	Amount:      "25",                      // amounts are decimal strings, never floats
	Currency:    "USDT",                    // what you price in: a fiat (USD, EUR, …) or a crypto asset
	Network:     oblodai.Ptr("tron"),       // omit to let the payer choose on the pay page
	OrderID:     oblodai.Ptr("order-1001"), // your reference
	URLCallback: oblodai.Ptr("https://shop.example/oblodai/webhook"),
})
if err != nil {
	log.Fatal(err)
}
fmt.Println(invoice.URL, invoice.Address, invoice.Status) // "created"
```

Цена в фиате — `Currency: "USD", ToCurrency: oblodai.Ptr("USDT")`: `Currency` — в чём выставлен
счёт, `ToCurrency` — актив, который пришлёт плательщик. Выплата — тем же ключом:

```go
payout, err := client.Payouts.Create(ctx, &oblodai.PayoutRequest{
	Address:  "TQn9Y2khEsLJW1ChVWFMSMeRDow5KNbBav",
	Amount:   "10",
	Currency: "USDT",
	Network:  oblodai.Ptr("tron"),
	OrderID:  "payout-1", // your reference; the payout is idempotent per order_id
}, oblodai.WithIdempotencyKey("payout-1"))
if err != nil {
	log.Fatal(err)
}
fmt.Println(payout.UUID, payout.Status) // "pending" → … → "confirmed"
```

Готовые программы — в [`examples/`](examples): `accept-payment`, `payout`, `webhook-receiver`; каждую
исполняет её тест против поддельного шлюза.

## Песочница / тестирование

Ключ песочницы работает с копией шлюза без блокчейна: тестовый баланс из крана, имитация
поступлений, настоящие вебхуки. Бизнес-эндпоинты ведут себя так же, как в бою.

```go
sandbox, err := oblodai.New(oblodai.WithCredentials(testPublicID, testSecret)) // a test_oblodai_… pair
if err != nil {
	log.Fatal(err)
}
if _, err := sandbox.Sandbox.Faucet(ctx, &oblodai.FaucetRequest{Asset: "USDT", Amount: "1000"}); err != nil {
	log.Fatal(err)
}
invoice, err := sandbox.Payments.Create(ctx, &oblodai.PaymentRequest{
	Amount: "25", Currency: "USDT", Network: oblodai.Ptr("tron"), OrderID: oblodai.Ptr("sandbox-1"),
})
if err != nil {
	log.Fatal(err)
}
// No amount pays exactly what is due; repeating a txid adds confirmations instead of paying twice.
deposit, err := sandbox.Sandbox.SimulateDeposit(ctx, &oblodai.SimulateDepositRequest{InvoiceID: invoice.UUID})
if err != nil {
	log.Fatal(err)
}
fmt.Println(deposit.Txid, deposit.Confirmations)
```

- `Sandbox.Faucet` начисляет тестовые деньги. `WithIdempotencyKey` заполняет поле тела
  `idempotency_key` — повтор не пополнит дважды.
- `Sandbox.SimulateDeposit` оплачивает счёт: без `Amount` — ровно столько, сколько нужно, иначе
  недоплата или переплата; `Confirmations` меньше нужного проверяет переход pending → confirmed.
- `Sandbox.ListWebhooks` показывает доставки с телами; `Sandbox.ReplayWebhook` отправляет повторно.
- `Webhooks.SendTestPayment` (и `…Payout`, `…Wallet`, `…Conversion`) — репетиция доставки на любой
  приёмник: подписана как настоящая, в теле `test: true`. Действовать по ней нельзя.
- `Sandbox.Reset` отменяет открытые счета магазина и обнуляет балансы.

## Обзор методов

Вся поверхность мерчанта, `client.<Сервис>.<Метод>` (таблица генерируется из контракта):

<!-- sdkgen:methods -->
17 ресурсов, 123 метода.

| Ресурс | Методы |
| --- | --- |
| `Payments` | `Create` · `GetInfo` · `GetQR` · `ListHistory` · `ListServices` · `Cancel` · `SendEmail` · `SetCheckoutConfig` · `GetCheckoutConfig` · `GetAmlLinks` · `Resolve` |
| `PaymentLinks` | `Create` · `List` · `Get` · `Toggle` |
| `Refunds` | `Payment` · `BlockedWallet` |
| `Payouts` | `Create` · `CreateMass` · `GetInfo` · `ListHistory` · `Calculate` · `Validate` · `Cancel` · `Approve` · `ListServices` · `TransferToPersonal` · `TransferToUser` · `CreateTransferBatch` |
| `PayoutLinks` | `Create` · `CreateBatch` · `List` · `Get` · `Cancel` · `GetPayoutClaim` · `ClaimPayout` |
| `Batches` | `CreatePayment` · `CreateRefund` · `CreatePayout` · `GetInfo` |
| `Splits` | `CreateRule` · `ListRules` · `DeleteRule` · `SetConfig` · `GetConfig` · `SetRecipientOptIn` · `GetRecipientOptIn` |
| `Wallets` | `Create` · `Block` · `GetQR` |
| `Account` | `GetBalance` · `GetSummary` · `ListExchangeRates` |
| `Webhooks` | `ResendPayment` · `Register` · `ListDeliveries` · `RequeueDelivery` · `SendLegacyTest` · `SendTestPayment` · `SendTestWallet` · `SendTestPayout` · `SendTestConversion` · `RotateSecret` · `SetActive` |
| `Settings` | `SetAccuracy` · `GetAccuracy` · `SetAutoRefund` · `GetAutoRefund` · `SetDiscount` · `ListDiscounts` · `ListAPILog` · `GetAutoConvert` · `SetAutoConvert` · `SetAcceptedCurrencies` · `ListAcceptedCurrencies` · `SetPayoutFeeConfig` · `GetPayoutFeeConfig` · `SetRefundFeeConfig` · `GetRefundFeeConfig` · `SetPaymentFeeConfig` · `GetPaymentFeeConfig` · `SetAutoWithdrawRule` · `ListAutoWithdrawRules` · `DeleteAutoWithdrawRule` · `ConfigureVrcs` |
| `APIAllowlist` | `List` · `AddEntry` · `RemoveEntry` · `SetEnabled` |
| `Referrals` | `GetInfo` |
| `Documents` | `GetSigned` · `GetBalance` · `GetFees` · `GetLedger` · `GetSplit` · `GetPayoutLinkCheque` · `GetStatement` · `GetBatch` · `GetPaymentLink` · `GetWalletStatement` · `GetReferrals` · `CreateJob` · `GetJob` · `DownloadJobFile` |
| `Checkout` | `GetSourceOfFundsForm` · `SubmitSourceOfFunds` · `GetPublicPaymentLink` · `PaymentLink` · `ListCurrencies` · `Get` · `SelectMethod` · `StartOnramp` · `GetOnramp` · `GetQR` |
| `Sandbox` | `OnboardStore` · `Faucet` · `SimulateDeposit` · `Reset` · `ListWebhooks` · `ReplayWebhook` |
| `CLILogin` | `Start` · `Poll` · `Logout` |
<!-- /sdkgen:methods -->

Имя метода — `operationId` контракта без имени ресурса, в стиле Go; список имён зафиксирован в
[`names.lock`](names.lock): новые имена генератор дописывает сам, а пропажа имени роняет его как
ломающее изменение. В комментарии
каждого метода перечислены коды ошибок, которыми он может ответить
(`go doc oblodai.PayoutsService.Create`). Маршруты документов возвращают
`*FileResult{Bytes, ContentType, Filename}`.

### Списки

Постраничный метод возвращает `*List[T]`, который ещё ничего не запросил. `Items()` проходит по
всем элементам, `ByPage()` — по страницам (запрос на страницу), `Page()` берёт первую страницу,
`Pager()` идёт явным курсором, `Collect(max)` собирает элементы.

```go
for payment, err := range client.Payments.ListHistory(ctx, &oblodai.PaymentHistoryRequest{Status: oblodai.Ptr("paid")}).Items() {
	if err != nil {
		return err
	}
	fmt.Println(payment.UUID, payment.Amount)
}
for page, err := range client.Payouts.ListHistory(ctx, &oblodai.HistoryRequest{Limit: oblodai.Ptr[int64](100)}).ByPage() {
	if err != nil {
		return err
	}
	fmt.Println(len(page.Items), page.Paginate.Total, page.Paginate.HasPages)
}
```

### Долгие операции

Пакеты и выгрузки документов принимаются сразу, а доделываются позже. `client.BatchJob(id)` и
`client.DocumentJob(id)` возвращают `*Job`: `Wait` опрашивает до конечного статуса (задача со
статусом `failed` возвращается, а не бросается), `Download` скачивает файл выгрузки. Какие
операции долгие — таблица `LRO`, сгенерированная из контракта; `JobFor[T]` следит за любой из них.

```go
accepted, err := client.Batches.CreatePayout(ctx, &oblodai.PayoutBatchRequest{Payouts: payouts})
if err != nil {
	return err
}
info, err := client.BatchJob(accepted.BatchID).Wait(ctx) // polls until completed or stopped
if err != nil {
	return err
}
fmt.Println(info.Status, info.Succeeded, info.Failed)

export, err := client.Documents.CreateJob(ctx, &oblodai.DocumentJobRequest{Kind: oblodai.DocumentJobKindLedger})
if err != nil {
	return err
}
job := client.DocumentJob(export.JobID)
view, err := job.Wait(ctx, oblodai.WithWaitTimeout(10*time.Minute))
if err != nil {
	return err
}
if view.Status == oblodai.DocumentJobStatusDone {
	file, err := job.Download(ctx)
	if err != nil {
		return err
	}
	fmt.Println(file.ContentType, len(file.Bytes))
}
```

### Статусы и деньги

- Платёж: `select → created → confirm_check → paid | paid_over | wrong_amount | expired | cancelled`.
  `IsPaymentPaid` — для `paid`/`paid_over`; `wrong_amount` (недоплата) ждёт `Payments.Resolve`;
  остальное покрывает `IsPaymentFinal`.
- Выплата: `pending → approved → awaiting_cosign → broadcasting → sent → confirmed | failed | cancelled`.
- Перечисление — строковый тип: незнакомое значение сохраняется как пришло (`status.IsKnown()`
  скажет), а незнакомые поля модели — в `Extra`.
- Суммы — `oblodai.Decimal`, строковый тип: float туда не помещается, а JSON-число в сумме
  отвергается с `sdk.float_amount`. `AddAmounts`, `SubtractAmounts`, `CompareAmounts`,
  `AmountsEqual`, `IsZeroAmount` считают точно — над `Decimal` и обычной строкой.

## Вебхуки

`Webhooks.Register` задаёт эндпоинт и возвращает секрет подписи — он показывается один раз. Для
проверки не нужны ни клиент, ни API-ключ:

```go
import "github.com/oblodai/oblodai-go/v2/webhooks"

delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: os.Getenv("OBLODAI_WEBHOOK_SECRET")})
if err != nil {
	http.Error(w, "bad signature", http.StatusBadRequest) // 4xx only for a failed verification
	return
}
if delivery.IsTest { // a rehearsal delivery: signed like a live one, but no money moved
	w.WriteHeader(http.StatusOK)
	return
}
if payment := delivery.Event.Payment; payment != nil && oblodai.IsPaymentPaid(payment.Status) {
	markOrderPaid(payment.OrderID, delivery.EventID) // EventID is stable for one state
}
w.WriteHeader(http.StatusOK)
```

Проверка идёт по **сырым** байтам; `VerifyRequest` читает не больше 1 МиБ. Порядок: заголовки,
HMAC (текущий секрет, затем `Options.PreviousSecret`), свежесть, тело. Отвечайте 4xx **только**
при провале проверки; доставка, которая прошла проверку, но не читается, — `webhook.bad_payload`:
отвечайте 5xx, событие настоящее, и ядро его повторит. `delivery.Event` несёт типизированное тело
своего вида (`Payment`, `Payout`, `Wallet`, `Conversion` — сгенерированные модели вебхуков); вид,
который добавило более новое ядро, приходит с `Type` и сырым телом `Raw` и `IsKnown() == false`.
`delivery.EventID` (`X-Webhook-Event-Id`) постоянен для одного состояния — по нему и дедуплицируйте;
`webhooks.IsStale(event, lastSequence)` отбрасывает событие не по порядку. После
`Webhooks.RotateSecret` держите старый секрет в `Options.PreviousSecret` не меньше 26 часов.

## Ошибки

Любой сбой — `*oblodai.Error`: достаётся `errors.As` (или `oblodai.AsError`), ветвиться — по `Code`,
стабильной строке `семейство.причина`. Печатается как `[код] текст (request_id=…)`.

```go
payout, err := client.Payouts.Create(ctx, params)
if err != nil {
	var apiErr *oblodai.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	log.Println(apiErr) // [payout.insufficient_funds] not enough USDT (request_id=…)
	switch apiErr.Code {
	case "payout.insufficient_funds", "payout.funds_maturing":
		return scheduleRetry(apiErr.RetryAfter) // retryable — the balance may still arrive
	default:
		return err // the client already retried whatever was safe to retry
	}
}
fmt.Println(payout.UUID)
```

Поля: `Kind`, `Code`, `Message`, `HTTPStatus`, `Retryable` (решающий — клиент уже повторил что
можно), `RetryAfter` (секунды), `RequestID` (ядра, иначе `X-Request-ID` вызова — назовите его
поддержке), `Field` (на 400), `Synthetic` (ответил прокси), `LastCode` (на `transport.deadline`: что
API сказал последним). Предикаты: `IsValidation`, `IsAuthentication`, `IsPermission`, `IsNotFound`,
`IsConflict`, `IsIdempotencyConflict`, `IsRateLimit`, `IsUnavailable`, `IsInternal`, `IsTransport`,
`IsConfig`, `IsContract`, `IsSignature`, `IsWebhookPayload`, `IsCode`, `IsRetryable`. Коды ядра —
сгенерированное перечисление `ErrorCode`; клиент добавляет `sdk.*` (`sdk.float_amount`,
`sdk.idempotency_unsupported`, `sdk.wait_timeout`, …), `transport.timeout|network|aborted|deadline`
и `webhook.*`.

## Повторы, идемпотентность и таймауты

- **Безопасность повтора** берётся из контракта: чтение (`x-retry-safe`) повторяется свободно,
  запись — только если ядро дедуплицирует её по `Idempotency-Key` и ключ отправлен.
- Ошибка повторяется, только если ядро пометило её `retryable`; ответы без конверта (502 прокси) и
  сбои сети — только когда повтор безопасен. `Retry-After` важнее расчётной паузы.
- **Ключи идемпотентности** создаются на дедуплицируемых маршрутах — один на вызов, тот же на всех
  повторах. `WithIdempotencyKey` переживает перезапуск процесса; на маршрутах без дедупликации
  клиент отвергает ключ с `sdk.idempotency_unsupported`, а не делает вид.
- **Расхождение часов** поправляется по заголовку `Date` после отказа подписи.
- **Редиректы не выполняются**; ответы ограничены 8 МиБ (64 МиБ для документов).

На вызов — `WithIdempotencyKey`, `WithRequestTimeout` (на попытку), `WithRequestBudget`,
`WithMaxRetries`, `WithExtraHeaders`/`WithRequestHeader`, `WithRequestID` (`X-Request-ID`, один на
все попытки; без опции создаётся) и `WithRawResponse` (статус, заголовки, тело и request id ответа).
`client.WithOptions(...)` — копия клиента с другими опциями, `WithHooks` видит каждую попытку:

```go
var raw *oblodai.RawResponse
balance, err := client.Account.GetBalance(ctx,
	oblodai.WithRequestTimeout(5*time.Second), // per attempt
	oblodai.WithMaxRetries(0),                 // this call only
	oblodai.WithRequestID("checkout-42"),      // X-Request-ID: joins your logs with ours
	oblodai.WithExtraHeaders(map[string]string{"X-Trace": "t-1"}),
	oblodai.WithRawResponse(&raw),
)
if err != nil {
	return nil, err
}
fmt.Println(balance, raw.StatusCode, raw.RequestID)

reports, err := client.WithOptions(oblodai.WithTimeout(2*time.Minute), oblodai.WithHooks(oblodai.Hooks{
	OnResponse: func(r oblodai.ResponseInfo) {
		log.Println(r.Request.OperationID, r.StatusCode, r.Elapsed)
	},
}))
```

## Настройка

| Опция                           | Что делает                                                                       |
| ------------------------------- | -------------------------------------------------------------------------------- |
| `WithCredentials(id, secret)`   | пара API-ключа мерчанта — подписывает все закрытые маршруты                       |
| `WithBaseURL(url)`              | адрес API; префикс пути сохраняется                                               |
| `WithInsecureBaseURL(true)`     | разрешить обычный `http://` не для loopback                                       |
| `WithAdminToken(token)`         | токен администратора своего шлюза (только маршруты подключения)                   |
| `WithHTTPClient(client)`        | свой `*http.Client`: прокси, транспорт, mTLS, записывающая заглушка               |
| `WithTimeout(d)`                | таймаут попытки (по умолчанию 30 с)                                               |
| `WithCallBudget(d)`             | бюджет вызова вместе с повторами и паузами (по умолчанию 90 с)                    |
| `WithRetry(opts)`               | политика повторов; `RetryOptions{MaxRetries: 0}` их выключает                     |
| `WithHooks(hooks)`              | `OnRequest`/`OnResponse` на каждую попытку                                        |
| `WithLogger(logger)`            | структурный логгер диагностики клиента                                            |
| `WithHeader(name, value)`       | заголовок в каждом запросе (зарезервированные имена игнорируются)                 |

| Переменная окружения       | Значение                                                         |
| -------------------------- | ---------------------------------------------------------------- |
| `OBLODAI_PUBLIC_ID`        | публичный id API-ключа                                           |
| `OBLODAI_SECRET`           | секрет API-ключа                                                 |
| `OBLODAI_ADMIN_TOKEN`      | токен администратора своего шлюза                                |
| `OBLODAI_BASE_URL`         | адрес API (по умолчанию `https://api.oblodai.com`)               |
| `OBLODAI_LOG`              | `debug` \| `info` \| `warn` \| `error` — включает текстовый логгер |
| `OBLODAI_ALLOW_INSECURE`   | `1` разрешает обычный `http://`                                  |

Явные опции важнее окружения. **Секреты не печатаются**: модель печатает (`%v`, `%+v`, `%#v`)
заданные поля, а всё похожее на секрет — секрет вебхука, токен или ссылку чека, пароль — как
`[redacted]`; JSON остаётся точным. `Client` не печатает ключи, а поля логов, похожие на секреты,
скрываются до того, как их увидит логгер.

## Разработка

```bash
make ci   # format, vet, lint, build, tests with -race, conformance, generated-code drift, packaging
```

`make ci` прогоняет общий набор сценариев conformance и проверку дрейфа против checkout бэкенда в
`OBLODAI_BACKEND` (по умолчанию `../oblodai-backend`): сгенерированные файлы должны быть ровно тем,
что `tools/sdkgen` делает из `services/core/api/openapi.json`. `zz_generated_*.go` руками не
правятся — перегенерируйте `make sdk` в бэкенде. `OBLODAI_LIVE_URL=http://127.0.0.1:8095 go test
-run TestLive ./...` — живой прогон против настоящего ядра. [AGENTS.md](AGENTS.md) — та же
поверхность на одной странице для агентов; [CHANGELOG.md](CHANGELOG.md) — что изменилось.

## Лицензия

MIT — см. [LICENSE](LICENSE).
