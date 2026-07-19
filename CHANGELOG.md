# Changelog

Значимые изменения этого пакета. Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
версии — [SemVer](https://semver.org/lang/ru/).

## [1.2.0] — 2026-07-19

### Добавлено
- **Песочница разработчика (`client.Sandbox`).** Бизнес-эндпоинты для тестовых ключей
  (`test_…` / `oblodai_test_…`) не меняются — меняется только ключ. Новое — пять test-only
  помощников (в проде их нет; живой ключ получает `403 sandbox.live_key`):
  - `Sandbox.SimulateDeposit(ctx, SandboxDepositParams)` — симуляция он-чейн депозита в инвойс
    (`POST /v1/sandbox/deposit`): точная оплата / недоплата / переплата (`Amount`), неглубокие
    подтверждения и углубление повтором того же `TxID` (`Confirmations`), идемпотентность по `TxID`.
  - `Sandbox.Faucet(ctx, asset, amount)` / `FaucetWithKey(…, key)` — «кран» тестового баланса
    (`POST /v1/sandbox/faucet`, потолок 1000000 за вызов; ключ идемпотентности — полем тела).
  - `Sandbox.Reset(ctx)` — отмена открытых инвойсов и обнуление балансов (`POST /v1/sandbox/reset`).
  - `Sandbox.ListWebhooks(ctx)` — последние ≤50 доставок вебхуков с сырым `Payload`
    (`GET /v1/sandbox/webhooks`, тип `SandboxDelivery`).
  - `Sandbox.ReplayWebhook(ctx, deliveryID)` — повторная постановка доставки в очередь
    (`POST /v1/sandbox/webhooks/replay`).
- **`oblodai.IsTestKey(publicID)`** — проверка, что public_id тестовый (префикс `test_`).
- **Подписанный GET.** HTTP-слой умеет подписывать GET-запросы с пустым телом — та же каноническая
  строка `{ts}\nGET\n{path}\n{пустое тело}` (нужно для `GET /v1/sandbox/webhooks`; поведение
  остальных эндпоинтов не изменилось).

## [1.1.0] — 2026-07-15

### ЛОМАЮЩИЕ ИЗМЕНЕНИЯ
- **Идемпотентность: заголовок `Idempotency-Key` вместо авто-`order_id`.** Создающие вызовы
  (`Payments.Create` / `Refund` / `Resolve` / `CreateBatch` / `RefundBatch`, `Payouts.Create` /
  `CreateMass` / `CreateBatch`, `Account.TransferToPersonal`) шлют заголовок `Idempotency-Key`
  (UUID v4, генерируется ОДИН раз до цикла повторов — все ретраи с одним ключом; в подпись запроса
  заголовок не входит). SDK **больше не подставляет** сгенерированный `order_id` в тело: `order_id`
  уходит как есть (в v1.0.x при пустом `order_id` вставлялся `idem-…`). Если вы полагались на
  авто-`order_id` в ответе — задавайте его явно. Свой ключ идемпотентности можно передать полем
  `params["idempotency_key"]` — оно уйдёт в заголовок и будет вырезано из тела.
- **`Retry: nil` теперь означает дефолтные повторы** (`DefaultRetry()`: до 4 попыток, backoff
  500 мс → 30 с, учёт `Retry-After`), как и в остальных SDK Oblodai. В v1.0.x `nil` означал
  «повторов нет». Отключить повторы — явно: `Retry: oblodai.NoRetry()` (новая функция).

### Добавлено
- **Массовые операции (батчи, до 5000 элементов одним запросом):** `Payments.CreateBatch`,
  `Payments.RefundBatch`, `Payouts.CreateBatch` (постановка, режим `on_error: continue|stop`) и
  `client.Batches.Info(batchID, limit, offset)` — прогресс и результат по каждому элементу
  (типы `BatchSubmission`, `BatchInfo`, `BatchItem`).
- **Платёжные ссылки:** `client.Links` — `Create` (типизированный `LinkParams`), `List`, `Info`,
  `Toggle` + публичные (без подписи) `PublicGet` и `Checkout`.
- **Сплит-платежи:** `client.Splits` — `CreateRule`, удобные `SplitToAddress` / `SplitToMerchant`,
  `ListRules`, `DeleteRule`, `GetConfig` / `SetConfig(refundHoldHours)`.
- **Payout-ссылки (крипто-чеки):** `client.PayoutLinks` — `Create`, `CreateBatch` (до 500), `List`,
  `Info`, `Cancel` + публичные (без подписи, без ключей) `ClaimInfo(token)`, `Claim(token, address)`
  и `ClaimWithMemo`. Тип `PayoutLink` со статусами `funded` / `claiming` / `claimed` / `expired` /
  `cancelled` (константы `PayoutLinkStatus*`). Заголовок `Idempotency-Key` на payout-link-эндпоинты
  НЕ шлётся (они не обёрнуты в идемпотентность на шлюзе) — защита от дублей: per-link `Reference`.
  Задавайте `ExpiresInHours` явно: при 0 бэкенд клампит срок к минимуму — 1 час (диапазон 1–720).
- **Счёт на e-mail:** `Payments.SendEmail(ctx, uuid, orderID, email)` — письмо покупателю с кнопкой
  «Оплатить» (тип `SendEmailResult`).
- **Судьба недоплаты:** `Payments.Resolve(ctx, uuid, orderID, action, opts)` — `accept` (оставить
  частичную оплату, глушит авто-возврат) или `refund` (вернуть плательщику; opts: `address`,
  `network`, `reference`). Тип `Resolution`.

## [1.0.2] — 2026-07-12

### Исправлено
- **Карта параметров вызывающего больше не мутируется.** `Payments.Create` и
  `Account.TransferToPersonal` подставляют авто-`order_id` в поверхностную КОПИЮ переданной карты,
  а не в неё саму. Раньше повторное использование одной `oblodai.Params` в двух вызовах протекало
  `order_id` из первого вызова во второй, и бэкенд схлопывал две операции в одну по дедупу. Теперь
  каждый вызов получает собственный ключ идемпотентности; исходная карта остаётся неизменной.
- **Нормализация проверки «order_id отсутствует».** `order_id` считается заданным только если это
  непустая строка после обрезки пробелов. Отсутствие, `nil`, `""`, `"   "` и не-строковые значения
  теперь одинаково приводят к вставке сгенерированного ключа.
- **`Retry-After` зажимается в диапазон `[0, 5 мин]`.** Огромное значение секунд больше не может
  переполнить `time.Duration` и дать отрицательную задержку (из-за которой повтор срабатывал бы
  мгновенно, в busy-loop). Значение зажимается к потолку до умножения; эффективная задержка всегда
  неотрицательна.

## [1.0.1] — 2026-07-12

### Исправлено
- **Безопасность повторов (деньги).** `Payments.Create` и `Account.TransferToPersonal` теперь
  автоматически подставляют стабильный ключ идемпотентности (`order_id = "idem-…"`), если он не
  задан. Ключ вставляется до цикла повторов, поэтому все попытки шлют один и тот же `order_id`, и
  бэкенд дедуплицирует повтор неидемпотентного POST (без риска двойного платежа/перевода). Выплаты
  по-прежнему требуют явного `order_id`.
- **`Retry-After` больше не зажимается к `MaxDelay`.** Серверный заголовок уважается как есть (напр.
  `Retry-After: 60` ждёт ~60с, а не 30с), с абсолютным потолком в 5 минут.
- **`payout.funds_maturing` больше не считается повторяемой** — это терминальная ошибка
  (`IsRetriable() == false`); дождитесь зрелости средств и повторите вручную.

## [1.0.0] — 2026-07-12

### Добавлено
- Первый релиз официального Go SDK для платёжного шлюза Oblodai.
- Приём платежей, выплаты и массовые выплаты, статические кошельки, возвраты, вебхуки,
  публичные справочники (курсы валют, каталог монет и сетей).
- Подпись запросов HMAC-SHA256 и проверка подписи вебхуков (сравнение в постоянном времени,
  защита от replay).
- Конструктор из переменных окружения `oblodai.NewFromEnv()` — `OBLODAI_PUBLIC_ID` / `OBLODAI_SECRET` /
  `OBLODAI_BASE_URL`.
- Автоматические повторы с экспоненциальным backoff и учётом заголовка `Retry-After` на 429.
