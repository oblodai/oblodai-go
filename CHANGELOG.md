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
  - `Sandbox.Reset(ctx)` — обнуление балансов и отмена инвойсов, по которым ещё не видели оплату
    (`POST /v1/sandbox/reset`); инвойсы в `confirm_check` / `wrong_amount_waiting` сознательно не
    трогаются — см. «Уточнена формулировка `Sandbox.Reset`» ниже.
  - `Sandbox.ListWebhooks(ctx)` — последние ≤50 доставок вебхуков с сырым `Payload`
    (`GET /v1/sandbox/webhooks`, тип `SandboxDelivery`).
  - `Sandbox.ReplayWebhook(ctx, deliveryID)` — повторная постановка доставки в очередь
    (`POST /v1/sandbox/webhooks/replay`).
- **`oblodai.IsTestKey(publicID)`** — проверка, что public_id тестовый (префикс `test_`).
- **Внутренние переводы пользователям платформы.** `Account.TransferToUser(ctx, params)` —
  перевод без комиссии с баланса мерчанта на личный кошелёк ДРУГОГО пользователя платформы
  (`POST /v1/transfer/to-user`; `to_user_id` — UUID пользователя, НЕ username; PAYOUT-ключ;
  заголовок `Idempotency-Key` как у остальных денежных вызовов, лестница дедупа
  заголовок → `order_id` → подпись). Тип `TransferToUserResult`
  (`currency`/`amount`/`to_user_id`/`recipient_balance`).
- **Пачка внутренних переводов.** `Account.TransferBatch(ctx, transfers, onError)` —
  «зарплатная» постановка до 5000 переводов одним запросом (`POST /v1/transfer/batch`,
  `on_error: continue|stop`); результат `BatchSubmission`, прогресс и построчные результаты —
  через `Batches.Info(batchID, …)`.
- **Публичная страница оплаты (свой checkout, без ключей в браузере).**
  `Payments.PublicGet(ctx, uuid)` — публичное состояние инвойса (`GET /v1/pay/{id}`, БЕЗ подписи;
  тип `PublicPayment` = `Payment` + `Accepted []AcceptedMethod` для инвойса в статусе выбора) и
  `Payments.PublicSelect(ctx, uuid, currency, network)` — выбор валюты и сети валюто-агностичного
  инвойса с фиксацией курса и выдачей адреса (`POST /v1/pay/{id}/select`, БЕЗ подписи; ответ —
  обычный `Payment`).
- **Подписанный GET.** HTTP-слой умеет подписывать GET-запросы с пустым телом — та же каноническая
  строка `{ts}\nGET\n{path}\n{пустое тело}` (нужно для `GET /v1/sandbox/webhooks`; поведение
  остальных эндпоинтов не изменилось).
- **Ключ идемпотентности на денежных вызовах, которые его не слали.** `PayoutLinks.Create`,
  `PayoutLinks.CreateBatch` и `Wallets.BlockedAddressRefund` резервируют средства, но шли без
  `Idempotency-Key` — при автоповторе (сетевая ошибка/5xx, до 4 попыток) потерянный ответ мог
  обернуться второй профинансированной ссылкой или повторной выплатой. Теперь запрос идёт с
  заголовком, зафиксированным ДО цикла повторов. Свой ключ — `PayoutLinkParams.IdempotencyKey`,
  `PayoutLinks.CreateBatchWithKey(ctx, links, key)`,
  `Wallets.BlockedAddressRefundWithKey(ctx, uuid, address, key)`.
  На `/v1/payout/link` и `/v1/payout/link/batch` шлюз заголовок **уважает**: повтор с тем же
  ключом реплеит первый ответ (та же ссылка, тот же `claim_token`, заголовок
  `Idempotent-Replayed: true`), баланс дебетуется ровно один раз. `Reference` остаётся вторым,
  durable слоем защиты.
- **Задокументированы коды слоя идемпотентности** на payout-ссылках: `idempotency.key_reused`
  (400, тот же ключ с другим телом), `idempotency.bad_key` (400), `idempotency.in_progress`
  (409, параллельный повтор), `idempotency.unavailable` (503, fail-closed — SDK ретраит сам).
  Дубль `Reference` теперь `payoutlink.duplicate_reference` (**409** вместо прежнего 500):
  терминальная ошибка, SDK больше не ретраит её впустую. Классификация `APIError.IsRetriable`
  уже корректна — ретраятся только 5xx и 429.
- **Батчи payout-ссылок:** частично упавший батч реплеится как есть (упавшие элементы шлите
  НОВЫМ ключом), а ответ больше 256 КБ шлюз не кэширует — поэтому на батчах проставляйте
  per-item `Reference`.

- **Типизированные константы статусов** (`statuses.go`): `oblodai.PaymentStatus` со значениями
  `PaymentStatusCheck` / `ConfirmCheck` / `WrongAmountWaiting` / `WrongAmount` / `Paid` /
  `PaidOver` / `Cancel` / `Select` и `oblodai.PayoutStatus` со значениями `PayoutStatusCheck` /
  `Process` / `Paid` / `Fail` / `Cancel`. У обоих типов — метод `IsFinal()`.
  Изменение **чисто аддитивное**: типы полей моделей (`Payment.PaymentStatus`, `Payout.Status`,
  `MassPayoutItem.Status`, `Resolution.Status`) остались **`string`**, существующий код
  (`strings.ToUpper(p.Status)`, `map[string]T[p.PaymentStatus]`) продолжает компилироваться
  без единой правки. Значения констант — те же строки из JSON: сравнивайте как
  `p.PaymentStatus == string(oblodai.PaymentStatusPaid)`, терминальность —
  `oblodai.PaymentStatus(p.PaymentStatus).IsFinal()`. Статусы payout-ссылок
  (`PayoutLinkStatus*`) — отдельный словарь и не изменились.
- **Раздел «Статусы» в README** — обе таблицы (платёж, выплата) с пометкой терминальности.
- **`client.PaymentLinks`** — каноническое имя ресурса платёжных ссылок, единое во всех SDK Oblodai
  (`payment_links` / `paymentLinks` / `PaymentLinks`): код переносится между языками без
  переименований. Прежнее `client.Links` остаётся **задокументированным алиасом на тот же самый
  объект** (`client.Links == client.PaymentLinks`) и никуда не денется — ничего переписывать не
  нужно.
- **`oblodai.DefaultWebhookMaxAgeSeconds` (300) и `oblodai.DisableWebhookMaxAge` (-1)** —
  именованные значения окна свежести вебхука (см. исправление replay-защиты ниже).

### Исправлено

- **БЕЗОПАСНОСТЬ: replay-защита вебхуков молча отключалась.** В `VerifyOptions.MaxAgeSeconds`
  нулевое значение (то есть **незаполненное поле**) трактовалось как «окно свежести не проверять».
  А `Now` — единственный способ подставить часы — задаётся той же структурой, поэтому любой
  `&VerifyOptions{Now: t}` (и вообще любая передача опций без явного `MaxAgeSeconds`) снимал
  replay-защиту: сколь угодно старый перехваченный вебхук с валидной подписью принимался как
  свежий. В остальных четырёх SDK Oblodai дефолт 300 срабатывал всегда.
  Теперь **0 (zero value) = дефолт `DefaultWebhookMaxAgeSeconds` (300)**, отключение — только
  явным сентинелом `MaxAgeSeconds: oblodai.DisableWebhookMaxAge` (`-1`).
  ⚠ Если вы **намеренно** отключали окно через `MaxAgeSeconds: 0` — замените на
  `oblodai.DisableWebhookMaxAge`, иначе начнёт действовать окно 300 с.
- **БЕЗОПАСНОСТЬ: не-`https` базовый URL принимался молча.** `New` / `NewFromEnv` брали любой
  `BaseURL`, включая `http://` на внешний хост, — и подпись запроса (`X-Signature`), `public_id` и
  тело уходили открытым текстом, где их можно перехватить и переиграть. Теперь схема обязана быть
  `https://`, иначе создание клиента возвращает ошибку с объяснением причины. **Исключение —
  локальная петля**: `http://localhost:…` (и `*.localhost`), `http://127.0.0.0/8`, `http://[::1]:…`
  принимаются как раньше, локальные стенды (в т.ч. `http://localhost:8095`) не ломаются. Чужие
  схемы (`ftp://`) и строки без схемы тоже отвергаются.
- **БЛОКЕР: в примере проверки вебхуков в README передавался не тот секрет.** Параметр назывался
  `secret`, а во всём остальном README «секрет» — это `Config.Secret` (секрет API-ключа), так что
  интегратор закономерно подставлял его в `ConstructEvent` и **отвергал 100% вебхуков**: вебхуки
  подписываются **отдельным секретом эндпоинта**, который возвращает `Webhooks.Register(...).Secret`.
  Параметр переименован в `endpointSecret` (и в примере, и в сигнатурах `VerifyWebhook`,
  `ConstructEvent`, `ComputeWebhookSignature`), добавлено явное предупреждение в README, в
  доккомментарии этих функций и в поле `WebhookRegistration.Secret`.
- **Задокументирован upsert при регистрации вебхука** (README + доккоммент `Webhooks.Register`).
  Register — это upsert **единственного** эндпоинта на проект (`ON CONFLICT (project_id) DO UPDATE`
  в ядре), а не добавление ещё одного: повторный вызов с ДРУГИМ URL возвращает тот же `endpoint_id`
  и **перенаправляет** доставки, а старый URL молча замолкает. Фан-аута на несколько адресов нет.
  Секрет при этом сохраняется — иначе смена URL осиротила бы уже стоящие в очереди доставки
  (они подписаны секретом на момент постановки) и потеряла события `paid`/`payout`.
- **Разъяснено `wrong_amount_waiting` vs `wrong_amount`** (README + доккоммент `Payments.Resolve`).
  Это два разных момента: `wrong_amount_waiting` — недоплата в процессе, счёт ещё жив и может
  стать `paid` доплатой, и `Resolve` там отвечает **409 `resolution.not_underpaid`** (не баг
  интеграции, ретраить бессмысленно); решать судьбу недоплаты можно только после закрытия счёта, в
  `wrong_amount`.
- **Оговорка про пустые `url` и `claim_url` на локальном стенде** (README + доккомментарии
  `Payment.URL`, `PayoutLink.ClaimURL`, `PaymentLinkCreated.URL` и `PaymentLink.URL`). Шлюз собирает
  все три ссылки из `GATEWAY_PUBLIC_BASE_URL` (`{base}/pay/{uuid}`, `{base}/claim/{token}`,
  `{base}/link/{link_id}`); локально переменной обычно нет — поля приходят пустой строкой. В проде
  шлюз без неё не стартует. Локально стройте ссылку сами из `UUID` / `ClaimToken` / `LinkID`.
- **Уточнена формулировка `Sandbox.Reset`** (README + доккоммент). «Отменяет открытые инвойсы» и
  «чистый лист» вводили в заблуждение: отменяются **только** инвойсы в статусах `check` и `select`.
  Инвойс, по которому депозит уже виден (`confirm_check`, `wrong_amount_waiting`), сознательно не
  трогается — отмена дала бы депозиту подтвердиться в отменённый счёт. Балансы обнуляются в любом
  случае.
- **Убрано ложное утверждение «заголовок на payout-ссылках сегодня игнорируется»** (README,
  доккомменты `PayoutLinksResource`, `PayoutLinkParams.Reference` / `.IdempotencyKey`,
  `PayoutLinks.Create` / `CreateBatch`, CHANGELOG — семь мест). `/v1/payout/link` и
  `/v1/payout/link/batch` **обёрнуты** в идемпотентность на шлюзе, заголовок работает. Вместе с
  ним ушло и утверждение, что `Reference` — «единственная реальная защита»: это второй,
  durable слой, а не единственный.
- **Убрано ложное утверждение, что у `Wallets.BlockedAddressRefund` «дедупа нет вовсе»** и вредный
  совет отключать для него повторы (`NoRetry`). Эндпоинт намеренно не обёрнут в middleware, но
  идемпотентен по состоянию и притом сильнее заголовка: детерминированная ссылка
  `refund-wallet:<wallet_id>` + per-wallet advisory-lock + поиск существующей выплаты внутри
  лока. Повтор возвращает ТУ ЖЕ выплату безусловно, конкурентный повтор дожидается результата
  вместо 409. Оговорка: повтор с другим адресом вернёт первую выплату на первый адрес.
- **Задокументирована идемпотентность `Payouts.Approve`.** Это переход состояния: принимается
  только `pending`, иначе `payout.not_pending` (409). Повторный approve не может одобрить или
  сдвинуть деньги дважды; 409 следует читать как «уже одобрено» и уточнять через `Payouts.Info`.
- **Убрано ложное утверждение про «автодозревание за ~10 минут»** (README, доккоммент
  `Sandbox.SimulateDeposit`, CHANGELOG). Были слиты два разных механизма. Депозит в песочнице с
  недобором подтверждений **сам не дозревает никогда**: цепочки у него нет, никто его не
  переэмитит, курсор не двигается — инвойс висит в `confirm_check`, пока не повторить
  `SimulateDeposit` с тем же `TxID` и бОльшим `Confirmations`. Ожидание ~10 минут относится
  ИСКЛЮЧИТЕЛЬНО к maturity-холду на **выплате** (`payout.funds_maturing`), который снимает по
  возрасту фоновый джоб (`GATEWAY_SANDBOX_MATURITY_MINUTES`, по умолчанию 10 минут) и который на
  статус инвойса не влияет.

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
  в этой версии ещё НЕ шлётся — защита от дублей: per-link `Reference`. (Исправлено в v1.2.0: SDK
  шлёт заголовок, и шлюз его уважает.)
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
