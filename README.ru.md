<div align="center">

<a href="https://oblodai.com">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/oblodai/.github/main/brand/logo-white.svg">
    <img src="https://raw.githubusercontent.com/oblodai/.github/main/brand/logo-black.svg" alt="oblodai" height="52">
  </picture>
</a>

<h3>Официальный Go SDK для платёжного шлюза <a href="https://oblodai.com">oblodai</a></h3>

Платежи, выплаты, платёжные ссылки, сплиты, статические кошельки, вебхуки — по одному API-ключу.

<a href="https://pkg.go.dev/github.com/oblodai/oblodai-go"><img src="https://pkg.go.dev/badge/github.com/oblodai/oblodai-go.svg" alt="Go Reference"></a>
<a href="https://github.com/oblodai/oblodai-go/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/oblodai/oblodai-go/ci.yml?branch=main&style=flat-square&label=CI" alt="CI"></a>
<img src="https://img.shields.io/github/go-mod/go-version/oblodai/oblodai-go?style=flat-square" alt="Go version">
<a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-000000?style=flat-square" alt="License: MIT"></a>

[Documentation](https://docs.oblodai.com) · [Dashboard](https://my.oblodai.com) · [Read in English →](README.md)

</div>

---

Официальный Go SDK для платёжного шлюза **Oblodai**: приём платежей, выплаты, массовые операции
(батчи), платёжные ссылки, выплатные ссылки (крипточеки), сплиты, статические кошельки, переводы,
вебхуки. Подпись запросов, разбор ответов, типизированные ошибки, идемпотентность и ретраи — из
коробки. Go ≥ 1.22 и только стандартная библиотека: **ноль сторонних зависимостей** — и в рантайме,
и в тестах; каждому маршруту шлюза здесь соответствует метод, сгенерированный из снимка контракта
самого шлюза и проверенный на эталонных ответах, записанных с живого ядра.

> **Base URL.** По умолчанию `https://api.oblodai.com`. При необходимости переопределите его через
> `WithBaseURL` и передайте свои ключи при инициализации. Схема должна быть `https://`; обычный
> `http://` принимается только для loopback (`http://127.0.0.1:8095`) или с явным разрешением
> небезопасного адреса (`WithInsecureBaseURL(true)` либо `OBLODAI_ALLOW_INSECURE=1`).

## Установка

```bash
go get github.com/oblodai/oblodai-go@v1.3.0
```

Требуется Go ≥ 1.22. Модуль — `github.com/oblodai/oblodai-go`; проверка вебхуков живёт в отдельном
подпакете `github.com/oblodai/oblodai-go/webhooks`, которому не нужны ни клиент, ни API-ключ.
Больше ничего не тянется: и SDK, и его тесты импортируют только стандартную библиотеку.

## Где взять ключи

Ключи выпускаются в [личном кабинете](https://my.oblodai.com) → **API keys**. Боевая пара — это
public id `oblodai_<hex>` и секрет `oblodai_live_<hex>`: один унифицированный API-ключ, открывающий
и платёжную, и выплатную сторону. У давних мерчантов два вида могут быть разведены по-старому:
`oblodai_pk_<hex>` (платёжный) и `oblodai_wk_<hex>` (выплатной):

- **платёжным ключом** подписываются счета, платёжные ссылки, кошельки, справочник, настройки и
  документы;
- **выплатным ключом** — всё, что выводит деньги: `Payouts.*`, `Refunds.*` (включая `Resolve`),
  `PayoutLinks.*`, `Transfers.*`, `Splits.*`, `Wallets.RefundBlockedDeposit`,
  `Settings.*AutoWithdraw`, `Settings.*APIAllowlist`, `Webhooks.RotateSecret`,
  `Webhooks.Test(WebhookKindPayout, …)`, `Sandbox.Faucet`, `Sandbox.Reset`.

Пара песочницы — это public id `test_oblodai_<hex>` и секрет `oblodai_test_<hex>`; она работает с
бесцепочечной копией шлюза и служит **обоими** видами ключа сразу, так что интеграции в песочнице
хватает одной пары. Если боевых пар у вас две, передайте обе — клиент сам выберет нужную для
каждого вызова:

```go
client, err := oblodai.New(
	oblodai.WithCredentials(publicID, secret),
	oblodai.WithPayoutCredentials(payoutPublicID, payoutSecret),
)
```

Запасной вариант через окружение — `OBLODAI_PUBLIC_ID` / `OBLODAI_SECRET` и
`OBLODAI_PAYOUT_PUBLIC_ID` / `OBLODAI_PAYOUT_SECRET`. Вызов не тем видом ключа даёт 403
`merchant.wrong_key_kind`; на маршруте, который принимает любой вид, `WithPayoutKey()` выбирает для
этого вызова выплатной. Заведение мерчантов (`Merchants.Create`, `Merchants.CreateSandbox`) идёт без
подписи — self-hosted шлюз закрывает эти маршруты **админ-токеном онбординга** (`WithAdminToken` или
`OBLODAI_ADMIN_TOKEN`).

## Быстрый старт

Каждый метод принимает `context.Context` первым аргументом и необязательные `RequestOption`
последними. Создаём счёт:

```go
client, err := oblodai.New() // OBLODAI_PUBLIC_ID / OBLODAI_SECRET from the environment
if err != nil {
	log.Fatal(err)
}
invoice, err := client.Payments.Create(ctx, oblodai.PaymentParams{
	Amount:      "25",                // amounts are decimal strings, never floats
	Currency:    "USDT",              // what you price in: a fiat (USD, EUR, …) or a crypto asset
	Network:     oblodai.NetworkTron, // omit to let the payer choose the network on the pay page
	OrderID:     "order-1001",        // your reference; the invoice is idempotent per order_id
	URLCallback: "https://shop.example/oblodai/webhook",
})
if err != nil {
	log.Fatal(err)
}
fmt.Println(invoice.URL, invoice.Address, invoice.Status) // "created"
```

Чтобы выставить цену в фиате, укажите `Amount: "25", Currency: "USD", ToCurrency: "USDT"`:
`Currency` — то, в чём вы выставляете счёт, `ToCurrency` — актив, который отправляет плательщик.
Если не задавать `Network`, плательщик выберет сеть на платёжной странице. Вывод денег идёт по
выплатному ключу:

```go
payout, err := client.Payouts.Create(ctx, oblodai.PayoutParams{
	Address:  "TQn9Y2khEsLJW1ChVWFMSMeRDow5KNbBav",
	Amount:   "10",
	Currency: "USDT",
	Network:  oblodai.NetworkTron,
	OrderID:  "payout-1", // your reference; the payout is idempotent per order_id
}, oblodai.WithIdempotencyKey("payout-1"))
if err != nil {
	log.Fatal(err)
}
fmt.Println(payout.UUID, payout.Status) // "pending" → … → "confirmed"
```

Готовые к запуску программы лежат в [`examples/`](examples): `accept-payment`, `payout`,
`webhook-receiver`.

## Песочница и тестирование

Ключ песочницы работает с бесцепочечной копией шлюза: фейковый баланс из крана, смоделированные
депозиты, настоящие вебхуки. Бизнес-эндпоинты ведут себя ровно так же, как на бою, — меняется
только ключ, а боевой ключ на маршруте песочницы отклоняется.

```go
sandbox, err := oblodai.New(oblodai.WithCredentials(testPublicID, testSecret)) // a test_oblodai_… pair
if err != nil {
	log.Fatal(err)
}
if _, err := sandbox.Sandbox.Faucet(ctx, oblodai.SandboxFaucetParams{Asset: "USDT", Amount: "1000"}); err != nil {
	log.Fatal(err)
}
invoice, err := sandbox.Payments.Create(ctx, oblodai.PaymentParams{
	Amount: "25", Currency: "USDT", Network: oblodai.NetworkTron, OrderID: "sandbox-1",
})
if err != nil {
	log.Fatal(err)
}
// No amount pays exactly what is due; repeating a txid adds confirmations instead of paying twice.
deposit, err := sandbox.Sandbox.Deposit(ctx, oblodai.SandboxDepositParams{InvoiceID: invoice.UUID})
if err != nil {
	log.Fatal(err)
}
fmt.Println(deposit.TxID, deposit.Confirmations)
```

- `Sandbox.Faucet` начисляет тестовые деньги, не больше 1000000 за вызов (выплатной ключ). Передайте
  `IdempotencyKey`, если повтор не должен пополнить баланс дважды.
- `Sandbox.Deposit` оплачивает счёт: без `Amount` платит ровно столько, сколько нужно, любое другое
  значение даёт недо- или переплату, а `Confirmations` меньше требуемого проверяет переход
  pending → confirmed. Повтор того же `TxID` добавляет подтверждения, а не платит второй раз.
- `Sandbox.Webhooks` показывает доставки вместе с телами — то, что получил бы ваш обработчик, — а
  `Sandbox.Replay(deliveryID)` переотправляет доставку в терминальном состоянии.
- `Webhooks.Test(kind, params)` присылает репетиционную доставку на любой обработчик, в песочнице
  или на бою: она подписана точно так же, как настоящая, и несёт `test: true` в подписанном теле
  (а также заголовок `X-Webhook-Test: true`). Проверяйте `delivery.IsTest` и никогда не считайте
  такую доставку движением денег.
- `Sandbox.Reset` отменяет открытые счета магазина и обнуляет его балансы (выплатной ключ).

## Обзор методов

16 неймспейсов, 107 маршрутов — вся мерчантская поверхность.

| Неймспейс      | Методы                                                                                                                                                                                    | Маршрутов |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------- |
| `Payments`     | Create · Info/Get · Cancel · History/List · Batch · QR · Services · SendEmail · Resend · PublicView · Select · PublicQR                                                                    | 12        |
| `Refunds`      | Create · Resolve · Batch                                                                                                                                                                   | 3         |
| `Payouts`      | Create · Validate · Calculate · Info/Get · Cancel · Approve · History/List · Mass · Batch · Services · Get/SetFeeConfig · Get/SetRefundFeeConfig                                           | 14        |
| `PayoutLinks`  | Create · Info/Get · List · Cancel · Batch · Cheque · ClaimPreview · Claim                                                                                                                  | 8         |
| `PaymentLinks` | Create · Info/Get · List · Toggle · PublicView · Checkout                                                                                                                                  | 6         |
| `Transfers`    | ToPersonal · ToUser · Batch                                                                                                                                                                | 3         |
| `Batches`      | Info (прогресс асинхронных батчей)                                                                                                                                                         | 2         |
| `Wallets`      | Create · QR · Block · RefundBlockedDeposit                                                                                                                                                 | 4         |
| `Webhooks`     | Register · RotateSecret · Deliveries · Test                                                                                                                                                | 5         |
| `Documents`    | Statement · Ledger · BalanceCertificate · FeeSchedule · SplitReport · BatchReport · LinkReport · WalletStatement · ReferralsReport · CreateJob · JobInfo · JobFile · Download               | 13        |
| `Splits`       | CreateRule · ListRules · DeleteRule · Get/SetConfig · Get/SetOptIn                                                                                                                          | 7         |
| `Settings`     | SetDiscount · ListDiscounts · Get/SetAccuracy · Get/SetAutoRefund · ListAccepted · SetAccepted · Get/SetPaymentFeeConfig · List/Set/DeleteAutoWithdraw · List/Add/Remove/EnableAPIAllowlist | 17        |
| `Account`      | Balance · Referral · VRCS/SetVRCS                                                                                                                                                          | 4         |
| `Catalog`      | Currencies · ExchangeRates                                                                                                                                                                 | 2         |
| `Sandbox`      | Faucet · Deposit · Webhooks · Replay · Reset                                                                                                                                               | 5         |
| `Merchants`    | Create · CreateSandbox (заведение мерчантов; на self-hosted шлюзе — `WithAdminToken`)                                                                                                      | 2         |

Методы поиска принимают оба идентификатора, поэтому работают и `PaymentInfoParams{UUID: id}`, и
`PaymentInfoParams{OrderID: "order-1001"}`. Идентификаторы — обычные строки
(`PayoutLinks.Cancel(ctx, linkID)`). Синхронные массовые вызовы (`Payouts.Mass` ≤ 100,
`PayoutLinks.Batch` ≤ 500) отвечают поэлементно через `BatchElement{Idx, OK, Result, Message,
ErrorCode}`; асинхронные (`Payments.Batch`, `Payouts.Batch`, `Refunds.Batch`, `Transfers.Batch`,
≤ 5000) опрашиваются через `Batches.Info`. Маршруты документов отвечают вне JSON-конверта и
возвращают `*FileResult{Bytes, ContentType, Filename}`. Методы, двигающие деньги, перечисляют коды
ошибок, которые стоит разбирать, в собственных doc-комментариях
(`go doc oblodai.PayoutsService.Create`).

### Списки

Метод-список возвращает `*List[T]`, который пока ничего не запросил. `Page()` забирает первую
страницу, `Pager()` обходит все страницы по одному запросу за раз, `All(max)` собирает их. Первая
страница мемоизирована, поэтому несколько горутин могут вызвать `Page()` на одном списке и разделить
единственный запрос; `Pager` — состояние на одного потребителя и принадлежит одной горутине.

```go
page, err := client.Payments.History(ctx, oblodai.PaymentHistoryParams{Limit: &fifty}).Page()
if err != nil {
	return err
}
fmt.Println(page.Items, page.Paginate.Total, page.Paginate.PerPage, page.Paginate.Offset, page.Paginate.HasPages)

pager := client.Payouts.History(ctx, oblodai.PayoutHistoryParams{Status: oblodai.PayoutStatusConfirmed}).Pager()
for pager.Next() {
	fmt.Println(pager.Item().UUID)
}
if err := pager.Err(); err != nil {
	return err
}
refunds, err := client.Payouts.History(ctx, oblodai.PayoutHistoryParams{Kind: "refund"}).All(1000)
```

### Статусы

- Платёж: `select → created → confirm_check → paid | paid_over | wrong_amount | expired | cancelled`.
  `IsPaymentPaid(status)` истинно для `paid`/`paid_over`; `wrong_amount` (недоплата) ждёт
  `Refunds.Resolve` с действием `accept` или `refund`; `IsPaymentFinal` покрывает остальные.
- Выплата: `pending → approved → awaiting_cosign → broadcasting → sent → confirmed | failed | cancelled`.

Об изменениях состояния лучше узнавать из вебхуков; опрос `Info` — только запасной вариант.

### Работа с суммами

`AddAmounts`, `SubtractAmounts`, `CompareAmounts`, `AmountsEqual`, `IsZeroAmount` — точная десятичная
арифметика над строковыми суммами, которыми оперирует API. Никогда не разбирайте `Money` во float:
у USDT 6 знаков после запятой, у BTC 8, у ETH 18, и двоичная плавающая точка не хранит их точно.
`Money` — алиас `string`, поэтому Go позволит написать `a < b`: не надо — `"9" < "10"` истинно как
текст и ложно как деньги. Всё, что не `-?digits[.digits]` (не длиннее 64 символов, без точки в конце
и без экспоненты), отклоняется с `sdk.bad_amount`.

## Вебхуки

`Webhooks.Register(ctx, url)` задаёт (или заменяет) эндпоинт и возвращает секрет подписи — он
показывается один раз, так что сохраните его туда, откуда его прочитает обработчик. Для проверки не
нужны ни клиент, ни API-ключ:

```go
import "github.com/oblodai/oblodai-go/webhooks"

delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: os.Getenv("OBLODAI_WEBHOOK_SECRET")})
if err != nil {
	http.Error(w, "bad signature", http.StatusBadRequest) // 401/4xx only for a signature failure
	return
}
if delivery.IsTest { // a rehearsal delivery: signed like a live one, but no money moved
	w.WriteHeader(http.StatusOK)
	return
}
switch event := delivery.Event.(type) {
case *oblodai.PaymentEvent:
	if event.Status == oblodai.PaymentStatusPaid {
		markOrderPaid(event.OrderID, delivery.ID) // delivery.ID is stable across retries
	}
case *oblodai.PayoutEvent:
case *oblodai.WalletEvent:
}
w.WriteHeader(http.StatusOK)
```

Проверка всегда идёт по **сырым** байтам; `VerifyRequest` читает из запроса не больше `MaxBodySize`
(1 МиБ). Порядок проверок: заголовки, HMAC (текущий секрет, затем `Options.PreviousSecret`),
свежесть, тело. Пустой `Secret` или отрицательный `Tolerance` — это `ConfigError`; `Tolerance`,
равный нулю, означает 5-минутное значение по умолчанию (в Go нулевое значение — это «не задано»,
поэтому проверка свежести не отключается случайно), а осознанно её выключает
`SkipTimestampCheck: true`.

Правило по кодам ответа обработчика: отвечайте 401 (или любым 4xx) **только** когда проверка не
прошла — подделка или устаревшая доставка. Доставка, которая проверку прошла, но тело которой
прочитать не удалось, — это `webhook.bad_payload`, ошибка *контракта*, а не подписи: отвечайте 5xx,
потому что событие настоящее и ядро повторит доставку. Тип события, добавленный более новым ядром,
приходит как `*oblodai.UnknownEvent` с сырым типом, а не ошибкой, — сузьте тип через
`webhooks.IsKnownEvent`, прежде чем разбирать конкретные события.

Репетиционные доставки (`Webhooks.Test`, песочница) подписаны ровно как боевые и несут `test: true`
в теле (и `X-Webhook-Test: true`): проверяйте `delivery.IsTest` (или `webhooks.IsTestEvent(event)`)
и никогда не действуйте по ним так, будто деньги двигались, — ничего не отгружать, ничего не
зачислять. `delivery.ID` (`X-Webhook-Id`) не меняется при повторах — используйте его для
дедупликации; `event.Seq()` упорядочивает события, а `webhooks.IsStale(event, lastSequence)`
отбрасывает пришедшее не по порядку (для события без последовательности он ложен). После
`Webhooks.RotateSecret` держите старый секрет в `Options.PreviousSecret` минимум 26 часов: доставки,
поставленные в очередь до ротации, остаются подписанными им весь срок своих повторов.

## Ошибки

Любая неудача — это `*oblodai.Error` с конвертом ошибки от API. Достаньте его через
`errors.As(err, &apiErr)` (или `oblodai.AsError`) и разбирайте `Code` — стабильную строку
`family.reason`, — но никогда не сообщение.

| Kind                      | HTTP           | Когда                                                             |
| ------------------------- | -------------- | ----------------------------------------------------------------- |
| `KindValidation`          | 400            | некорректный запрос или бизнес-правило; `Field` называет поле      |
| `KindAuthentication`      | 401            | плохая подпись, неизвестный ключ, расхождение часов, IP не в белом списке |
| `KindPermission`          | 403            | ключ валиден, но здесь нельзя (не тот вид ключа, фича выключена)   |
| `KindNotFound`            | 404            | у этого мерчанта такого объекта нет                                |
| `KindConflict`            | 409            | конфликт состояния                                                 |
| `KindIdempotencyConflict` | 409            | `idempotency.key_reused`: тот же ключ, другое тело                 |
| `KindRateLimit`           | 429            | выставлен `RetryAfter`                                             |
| `KindUnavailable`         | 503            | лежит зависимость; безопасно повторить после паузы                 |
| `KindInternal`            | другие 5xx     | ядро упало                                                         |
| `KindAPI`                 | всё остальное  | статус ошибки с конвертом                                          |
| `KindTransport`           | —              | ответа не было вовсе: DNS, TCP, TLS, таймаут, отмена               |
| `KindConfig`              | —              | отказ до отправки: плохие опции, нет учётных данных                |
| `KindContract`            | —              | ответ не читается как документированный конверт                    |
| `KindSignature`           | —              | проверка вебхука не прошла                                         |

Поля: `Code`, `Message`, `HTTPStatus`, `Retryable` (авторитетно — клиент уже повторил то, что
следовало), `RetryAfter` (в секундах), `RequestID` (называйте его поддержке), `Field` (на 400),
`Synthetic` (ответ пришёл от прокси, а не от API), `LastCode` (на `transport.deadline` — что API
сказал последним), `Kind`. Предикаты покрывают то же самое: `IsValidation`, `IsAuthentication`,
`IsPermission`, `IsNotFound`, `IsConflict`, `IsIdempotencyConflict`, `IsRateLimit`, `IsUnavailable`,
`IsInternal`, `IsTransport`, `IsConfig`, `IsContract`, `IsSignature`, `IsWebhookPayload`, `IsCode`,
`IsKind`, `IsRetryable`.

```go
payout, err := client.Payouts.Create(ctx, params)
if err != nil {
	var apiErr *oblodai.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Code {
	case "payout.insufficient_funds", "payout.funds_maturing":
		return scheduleRetry(apiErr.RetryAfter) // retryable — the balance may still arrive
	default:
		return err // the client already retried whatever was safe to retry
	}
}
```

Полный каталог — `oblodai.ErrorCodes`: все 471 код, которыми может ответить ядро, поставляются в
снимке контракта. Коды, которые стоит обработать в первую очередь: `payout.insufficient_funds` и
`payout.funds_maturing` (оба retryable), `idempotency.key_reused`, `invoice.not_payable`,
`payment.not_found`, `merchant.wrong_key_kind`, `merchant.bad_signature`, `request.rate_limited`.
Сверх них клиент поднимает собственные семейства: `sdk.missing_credentials`, `sdk.bad_config`,
`sdk.bad_idempotency_key`, `sdk.idempotency_unsupported`, `sdk.bad_envelope`, `sdk.bad_path_param`,
`sdk.bad_amount`, `sdk.bad_header`, `sdk.response_too_large`,
`transport.timeout|network|aborted|deadline`,
`webhook.bad_signature|stale_timestamp|missing_header|bad_payload`.

`RetryAfter` отдаётся в секундах и ограничен диапазоном `[0, MaxRetryAfterSeconds]` (сутки) — и
когда пришёл из конверта, и когда из заголовка `Retry-After`; то, сколько реально спит цикл ретраев,
ограничено `RetryOptions.MaxRetryAfter`. Когда вызов исчерпал бюджет, ошибка — `transport.deadline`,
и она несёт то, что API сказал последним: `LastCode`, `HTTPStatus`, `RetryAfter`, `RequestID`, а до
самой последней ошибки можно добраться через `errors.Unwrap`.

Сериализация `*Error` в JSON сохраняет идентичность (код, сообщение, статус, request id) и выбрасывает
сырое тело, поэтому структурированный лог не утечёт то, что в теле было. `Error.Body()` по-прежнему
возвращает его для отладки.

## Ретраи, идемпотентность и таймауты

- **Безопасность повтора** не угадывается: `Routes[key].Safe` — собственная классификация ядра
  (read-only), поставляемая в снимке контракта, и кодоген падает на снимке, где её нет.
- Ошибка повторяется, только если API сказал `retryable`. Ответы без конверта API (502/503 от
  прокси) и транспортные сбои повторяются только на читающих маршрутах и на записи с ключом
  идемпотентности. `Retry-After` важнее вычисленного бэкоффа.
- **Ключи идемпотентности** проставляются автоматически на создающих маршрутах — один на логический
  вызов, переиспользуемый на каждом повторе, — так что таймаут не может породить вторую выплату.
  Передайте `WithIdempotencyKey`, чтобы повторы были безопасны и после перезапуска процесса; на
  маршрутах, которые шлюз не дедуплицирует (включая списки), клиент отклоняет ключ с
  `sdk.idempotency_unsupported`, вместо того чтобы позволить вам считать повтор безопасным.
- **На вызов:** `WithIdempotencyKey`, `WithRequestTimeout`, `WithRequestBudget`,
  `WithRequestHeader`, `WithPayoutKey`. **На клиент:** `WithTimeout` (на попытку, 30 с),
  `WithCallBudget` (попытки вместе с паузами, 90 с),
  `WithRetry(oblodai.RetryOptions{MaxRetries, BaseDelay, MaxDelay, MaxRetryAfter})`. Отмена контекста
  прекращает всё, включая паузу между повторами.
- **Расхождение часов** корректируется по заголовку `Date` от API после 401, похожего на перекос, и
  коррекция откатывается, если не помогла; `Client.ClockOffset()` показывает её.
- **Редиректы никогда не выполняются**: подписанный запрос не должен повторяться на другой origin,
  поэтому редирект сообщается ошибкой.
- **Ограничения на размер тела**: 8 МиБ на JSON-маршрутах, 64 МиБ на маршрутах документов — ответ
  больше этого даёт `sdk.response_too_large`, ошибку контракта, а не нехватку памяти. Проверка
  вебхука читает не больше 1 МиБ (`webhooks.MaxBodySize`).
- **Зарезервированные заголовки** выигрывают у `WithHeader`/`WithRequestHeader` при
  регистронезависимом сравнении: `ReservedHeaders()` — это `X-Public-Id`, `X-Signature`,
  `X-Timestamp`, `Idempotency-Key`, `X-Admin-Token`, `Accept`, `User-Agent`, `Content-Type`,
  `Content-Length`, `Host`. Заголовок с переводом строки или не-ASCII байтом отклоняется с
  `sdk.bad_header` до того, как что-либо будет отправлено.

## Конфигурация

| Опция                           | Что делает                                                                       |
| ------------------------------- | -------------------------------------------------------------------------------- |
| `WithCredentials(id, secret)`   | платёжная пара ключей (используется и для выплат, если выплатной пары нет)        |
| `WithPayoutCredentials(id, s)`  | отдельная выплатная пара ключей                                                   |
| `WithBaseURL(url)`              | origin API; префикс пути сохраняется                                              |
| `WithInsecureBaseURL(true)`     | разрешить обычный `http://` для не-loopback хоста                                 |
| `WithAdminToken(token)`         | админ-токен онбординга self-hosted шлюза (только маршруты заведения мерчантов)    |
| `WithHTTPClient(client)`        | свой `*http.Client`: прокси, кастомный транспорт, mutual TLS, стаб в тестах       |
| `WithTimeout(d)`                | таймаут на попытку (по умолчанию 30 с)                                            |
| `WithCallBudget(d)`             | бюджет одного вызова вместе с повторами и паузами (по умолчанию 90 с)             |
| `WithRetry(opts)`               | политика ретраев; `RetryOptions{MaxRetries: 0}` выключает их                      |
| `WithLogger(logger)`            | структурированный логгер для диагностики клиента                                  |
| `WithHeader(name, value)`       | заголовок на каждый запрос (зарезервированные имена игнорируются)                 |

| Переменная окружения       | Значение                                                       |
| -------------------------- | -------------------------------------------------------------- |
| `OBLODAI_PUBLIC_ID`        | public id платёжного ключа                                     |
| `OBLODAI_SECRET`           | секрет платёжного ключа                                        |
| `OBLODAI_PAYOUT_PUBLIC_ID` | public id выплатного ключа                                     |
| `OBLODAI_PAYOUT_SECRET`    | секрет выплатного ключа                                        |
| `OBLODAI_ADMIN_TOKEN`      | админ-токен онбординга self-hosted шлюза                       |
| `OBLODAI_BASE_URL`         | origin API (по умолчанию `https://api.oblodai.com`)            |
| `OBLODAI_LOG`              | `debug` \| `info` \| `warn` \| `error` — включает текстовый логгер |
| `OBLODAI_ALLOW_INSECURE`   | `1` разрешает обычный `http://` в базовом URL                  |

Явные опции важнее окружения. Половина пары ключей (id без секрета или наоборот) отклоняется прямо
в `New` с `sdk.bad_config`; отсутствие учётных данных всплывёт позже — на первом вызове, которому
они нужны.

**Секреты не печатаются.** `WebhookEndpoint.Secret`, `WebhookSecretRotated.Secret`,
`APIKeyPair.Secret`, `PayoutLink.ClaimToken`, `PayoutLink.ClaimURL` (в нём зашит токен) и
`PayoutLink.Passcode` читаются как обычные поля, но выводятся как `[redacted]` в `fmt` (`%v`, `%+v`,
`%#v`) и в `json.Marshal` — сохраняйте их, читая поле, а не сериализуя структуру. `Client` тоже
никогда не печатает свои ключи. Поля лога, чьё имя похоже на секрет, вычищаются внутри клиента, до
того как значение дойдёт до любого логгера, включая установленный через `WithLogger`.

**Self-hosted или локальный шлюз.** `WithBaseURL("http://127.0.0.1:8095")` работает сразу; любому
другому http-хосту нужен `WithInsecureBaseURL(true)` (или `OBLODAI_ALLOW_INSECURE=1`). Префикс пути
в базовом URL сохраняется, поэтому `https://gw.corp/oblodai` ведёт на
`https://gw.corp/oblodai/v1/payment` — и подпись покрывает путь вместе с префиксом.

## Снимок контракта

`contract/` экспортируется собственным тестовым набором шлюза: реестр маршрутов (107 маршрутов, у
каждого — собственный флаг `safe` от ядра), схемы DTO запросов, все словари и все 471 код ошибок,
векторы подписи, эталонные тела ответов, записанные с живого ядра, и 43 настоящие подписанные
доставки вебхуков. Локальным для репозитория остаётся только `contract/descriptions.en.json`
(английские описания полей); всё остальное при обновлении заменяется целиком. `contract_routes.go`,
`contract_enums.go`, `contract_requests.go` и `contract_version.go` сгенерированы из него и никогда
не правятся руками. `ContractCoreCommit`, `ContractExportedAt` и `ContractHash` идентифицируют
используемый снимок.

```bash
go generate ./...                  # regenerate after refreshing contract/
go run ./internal/codegen -check   # drift gate: fails when the generated files are stale
go test ./...                      # unit + contract tiers (hermetic)
```

Контрактный слой — это гейт полноты, а не выборка: у каждого из 107 маршрутов должен быть метод,
привязанный к нужному пути, гейту авторизации и поведению идемпотентности, а каждое записанное тело
ответа должно разбираться в модель, поля которой совпадают с проводным форматом ключ в ключ.

## Разработка

```bash
git clone https://github.com/oblodai/oblodai-go && cd oblodai-go
gofmt -l .                         # formatting gate: must print nothing
go vet ./... && staticcheck ./...  # static analysis
go test -race ./...                # everything, hermetic
OBLODAI_LIVE_URL=http://127.0.0.1:8095 go test -run TestLive ./...   # against a real core
```

Файлы исходников держатся в пределах ~400 строк, а тесты живут рядом с тем, что проверяют. См.
[AGENTS.md](AGENTS.md) — та же поверхность на одной странице, написанная для кодовых агентов;
[CHANGELOG.md](CHANGELOG.md) — что менялось; [MIGRATION-1.3.md](MIGRATION-1.3.md) — переход с 1.2.

## License

MIT — см. [LICENSE](LICENSE).
