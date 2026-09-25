# Changelog

Notable changes to this package. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this package follows
[Semantic Versioning](https://semver.org/).

## [2.0.0] — 2026-09-25

The API surface is generated from the gateway's OpenAPI contract by the backend's `tools/sdkgen`,
the same generator every Oblodai SDK uses. Breaking: see [MIGRATION-2.0.md](MIGRATION-2.0.md).

### Added

- Per-call `WithMaxRetries`, `WithExtraHeaders`, `WithRequestID` and `WithRawResponse`; an
  `X-Request-ID` on every call, the same on every attempt.
- `Client.WithOptions` and `WithHooks` (`OnRequest`/`OnResponse` per attempt).
- `List.Items()` and `List.ByPage()` iterators.
- Waiters for long-running operations: `BatchJob`, `DocumentJob`, `JobFor[T]` with `Wait` and
  `Download`, driven by the `LRO` and `Polls` tables generated from the contract's `x-sdk-poll`
  (`Poll` carries the status field and the terminal statuses of each poll).
- Status classes generated from the contract's `x-status-classes`: `PaymentStatusFinalValues`,
  `PaymentStatus.IsFinal()`, `IsSuccess()` and the same for every classified status;
  `IsPaymentFinal`, `IsPaymentPaid`, `IsPayoutFinal`, `IsPayoutSucceeded` read them.
- `webhooks.KnownKinds`, `webhooks.EventKinds`, `webhooks.IDFields` (kind → the body field
  holding the object's id, read by `Event.ID()`) and the kinds' typed bodies (`webhooks.Bodies`,
  embedded in `Event`) generated from the contract's webhooks. `webhooks.Parse` requires only
  `type` of a kind this release does not know, and `Event.ID()` is `""` for it.
- A JSON number with a fraction in a free-form request member (a model's `Extra`) is
  `sdk.float_amount` before anything is sent, except in the numeric fields the contract declares
  are not money.
- An idempotency key given twice to the sandbox faucet — in its `idempotency_key` field and with
  `WithIdempotencyKey` — is refused before anything is sent.
- The backend's shared conformance suite (signing and webhook vectors from `x-oblodai-signing`,
  retry, money and forward-compatibility scenarios), a generated-code drift check, a sweep that
  calls every generated method, and README and examples executed in the tests; `make ci`.

### Changed

- Module path `github.com/oblodai/oblodai-go/v2`; Go ≥ 1.25.
- Services, methods, request and response models, enumerations and `Routes` (keyed by
  `operationId`, `RouteSpec`) are `zz_generated_*.go`: 16 services, 120 operations, named after the
  contract's `operationId` and pinned in `names.lock`. Methods take the parameters as a pointer to
  the generated request model; optional fields are pointers (`oblodai.Ptr`).
- Amounts are `oblodai.Decimal`, a string type: a float does not fit, and a JSON number in an amount
  is `sdk.float_amount`. The money helpers take a `Decimal` or a plain string.
- `Error.Error()` reads `[code] message (request_id=…)`.
- Models print their set fields with secrets redacted; `json.Marshal` of a model is faithful.
- `webhooks` returns `*webhooks.Event` with the typed body of its kind (the generated webhook
  models, conversion events included) and `Delivery.EventID` (`X-Webhook-Event-Id`).
- `List.All` is `List.Collect`.
- The request-signing protocol is generated from the contract's `x-oblodai-signing`
  (`zz_generated_signing.go`, root and `webhooks`): the header names (`HeaderPublicID`,
  `HeaderSignature`, `HeaderTimestamp`, `HeaderIdempotencyKey`, `webhooks.HeaderTimestamp` and the
  other delivery headers, and `webhooks.HeaderTest`, the rehearsal header, from
  `webhook.test_header`), the canonical strings `SignRequest`/`SignWebhook` sign, and the limits:
  the new `SkewSeconds`, `MaxBody` and `SignatureAlgorithm`, and `MaxIdempotencyKeyLength`.
  `SignatureSkewSeconds` stays, as an alias of `SkewSeconds`; the other names and all values are
  unchanged. `webhooks.DefaultTolerance` is the contract's skew window (5 minutes today) and
  `webhooks.MaxBodySize` its body limit (`MaxBody`): the contract has no webhook body limit, so the
  read cap of `VerifyRequest` follows `x-oblodai-signing.max_body`, the core's limit on signed
  request bodies — the rule for every SDK that reads a delivery from a stream. The conformance suite checks the request a
  signed call sends — method, path and query, body and the headers under the contract's names.

### Removed

- Hand-written resources and models, `internal/codegen`, the `contract/` snapshot and the
  `Contract*` constants; `Merchants.Create` (not part of the merchant API contract).

## [1.3.0] — 2026-08-26

A rewrite generated from the gateway's own contract snapshot. See MIGRATION-1.3.md.

### Added

- Every merchant route the core declares (107): cancel/validate, batches, documents, fee configs,
  split opt-in, secret rotation, the payer-facing checkout and claim endpoints, merchant
  provisioning (`client.Merchants`, `WithAdminToken` / `OBLODAI_ADMIN_TOKEN` on a self-hosted
  gateway).
- `*List[T]` lists (`Page`, `Pager`, `All`) that request nothing until consumed,
  `retryable`-driven retries, automatic idempotency keys, clock-skew correction, a per-attempt
  timeout and a per-call budget.
- `github.com/oblodai/oblodai-go/webhooks`: rotation-aware `Verify`, `VerifyRequest`,
  `VerifyDelivery`, `Parse`, `IsStale`, `IsTestEvent`, `IsKnownEvent`, and `Delivery.IsTest` for
  rehearsal deliveries. No client and no API key needed.
- `contract/` snapshot plus `internal/codegen` (`go generate ./...`, `go run ./internal/codegen
  -check` as a drift gate), contract tests against the golden bodies and 43 real signed webhook
  deliveries, and a live journey behind `OBLODAI_LIVE_URL`.
- `AGENTS.md`: the whole surface in one page, for coding agents.
- `WithRequestHeader(name, value)`: a header for one call. `ReservedHeaders()` lists the names the
  client owns.
- `*oblodai.UnknownEvent`: a webhook type a newer core added arrives with its raw `type` instead of
  being refused; narrow with `IsKnownEvent`.
- `Error.LastCode`: on `transport.deadline`, the code of the failure that was in force when the
  client gave up. `HTTPStatus`, `RetryAfter` and `RequestID` are copied onto it too, and the last
  error stays reachable through `errors.Unwrap`.
- `MaxRetryAfterSeconds` (86400) and `MaxAmountLength` (64).

### Changed

- **One API key.** A merchant's single key signs every gated route, payouts included: the payout
  credential pair and the payout-key option are gone (`WithPayoutCredentials`, `WithPayoutKey()`,
  `OBLODAI_PAYOUT_PUBLIC_ID`, `OBLODAI_PAYOUT_SECRET`), along with the key-kind selection in the
  transport and the payout-key retry on `Batches.Info`. The route table's `Auth` is now `public`,
  `key` or `onboard`, and codegen refuses any other value. Onboarding returns `api_key` only, so
  `MerchantOnboarded` and `SandboxStore` no longer carry `PaymentKey`/`PayoutKey`, and `APIKeyPair`
  no longer carries `Kind`. `merchant.wrong_key_kind` stays documented once, as a legacy
  split-key error.
- Every call takes a `context.Context`; options are functional
  (`oblodai.New(oblodai.WithCredentials(…))`); one error type `*oblodai.Error` with `errors.As` and
  the `Is*` predicates; amounts stay decimal strings.
- Retry safety comes from the contract's own `safe` flag per route — the core's hand-classified
  statement that a route is read-only. The SDK no longer infers it from the path, and codegen fails
  if a contract snapshot does not declare it.
- Secrets are redacted in both paths a Go program prints through: `WebhookEndpoint.Secret`,
  `WebhookSecretRotated.Secret`, `APIKeyPair.Secret`, `PayoutLink.ClaimToken`,
  `PayoutLink.ClaimURL` (it embeds the token) and `PayoutLink.Passcode` render as `[redacted]` in
  `fmt` (`%v`, `%+v`, `%#v`) and in
  `json.Marshal`; the fields themselves keep the real value. A `Client` never prints its keys.
- Log field redaction happens in the client, before the value reaches any logger — including one
  installed with `WithLogger`.
- `NewIdempotencyKey` returns `(string, error)` instead of panicking when the platform CSPRNG is
  unavailable.
- `Documents.Download` omits `exp` when it is unset instead of sending `exp=0`.
- Webhook verification checks the MAC before the timestamp, so the freshness window cannot answer
  questions to a caller who cannot sign. An empty secret or a negative tolerance is a `ConfigError`
  raised before any crypto runs; `SkipTimestampCheck` (not a zero tolerance) disables freshness.
- Webhook signature headers are trimmed and accepted in either hex case; a `0x` prefix is refused.
- `Resolution` moved to `models_payments.go`; the duplicate `ResolutionAccepted` is gone —
  `Resolution` already covers both shapes of `POST /v1/payment/resolve`.
- `Money` helpers refuse anything that is not `-?digits[.digits]` (≤ 64 characters) with a
  `ConfigError` carrying `sdk.bad_amount`; a trailing dot (`"5."`) is no longer read as `"5"`.

### Fixed

- Requests are signed with the five-field recipe (`ts\nMETHOD\nrequest_uri\nidempotency_key\nbody`)
  over path plus query. 1.x signed four fields and got 401 on every call against the current core.
- Models, statuses, pagination and parameter names match the current API vocabulary, field for
  field, checked against response bodies recorded from a live core.
- Path parameters are escaped exactly once: `"a b"` reached the core as `a%2520b`.
- `*List[T]` memoizes its first page under a `sync.Once`: two goroutines calling `Page` raced over
  the memo (`go test -race` reproduces it).
- The error envelope is decoded field by field. A `code` that is not a non-empty string makes the
  answer synthetic (the `request_id` is kept); a non-string `message` falls back to `HTTP <status>`;
  a non-boolean `retryable` falls back to the status; `retry_after` accepts an integer, a float or a
  numeric string and is clamped to `[0, 86400]` — negative and implausible values can no longer
  become a wait. The `Retry-After` header is clamped the same way, and an HTTP-date centuries away
  no longer overflows into a negative wait.
- Response bodies are read under a size cap (8 MiB for JSON routes, 64 MiB for document routes) and
  report `sdk.response_too_large` instead of buffering whatever arrives.
- A caller header cannot claim `User-Agent`, `Accept` or `X-Admin-Token` (the admin token is sent by
  the client, on onboarding routes only); a header name or value with a line break or a non-ASCII
  byte is refused with `sdk.bad_header` before anything is sent.
- An injected `http.Client` whose transport follows a redirect itself is detected: the answer would
  come from an origin the request was not signed for.
- An idempotency key passed to a list method is refused with `sdk.idempotency_unsupported` instead
  of being dropped silently.
- A clock correction is compared against, and reverted to, the offset the failing request was signed
  with, so concurrent calls cannot undo each other's correction; the retry that follows a correction
  respects the call budget.
- A verified webhook whose body cannot be read reports `webhook.bad_payload` in the contract family
  — answer 5xx to it — instead of `webhook.bad_signature`, which receivers answer 4xx to.
- `IsStale` returns false for an event that carries no sequence rather than treating it as old.
- Generated documentation is English only: non-ASCII example strings from the core's own docs are no
  longer copied into it.

### Safety rules the transport enforces

An undeduplicated write is never re-sent after a transport failure or an envelope-less proxy answer;
an idempotency key is refused on routes the core does not deduplicate; a clock correction that does
not help is reverted; caller headers cannot overwrite signed ones; a path parameter that would
rewrite the URL is rejected; redirects are reported, never followed.

## [1.2.0] — 2026-07-19

### Added

- **Developer sandbox (`client.Sandbox`).** Business endpoints behave the same for test keys
  (`test_…`); only the key changes. Five test-only helpers were added (a live key gets `403
  sandbox.live_key`): `SimulateDeposit` (exact, under- or overpayment, shallow confirmations,
  idempotent by `TxID`), `Faucet`/`FaucetWithKey` (cap 1000000 per call), `Reset`, `ListWebhooks`
  (last ≤50 deliveries with the raw payload), `ReplayWebhook`.
- **`oblodai.IsTestKey(publicID)`** — whether a public id is a test key (`test_` prefix).
- **Internal transfers to platform users.** `Account.TransferToUser` — a fee-free move from the
  merchant balance to another platform user's personal wallet (`to_user_id` is the user's UUID, not
  a username).
- **Batched internal transfers.** `Account.TransferBatch` — up to 5000 transfers in one request
  (`on_error: continue|stop`); progress through `Batches.Info`.
- **Public payment page** (own checkout, no keys in the browser): `Payments.PublicGet` and
  `Payments.PublicSelect` (unsigned).
- **Signed GET**: the HTTP layer signs GET requests with an empty body.
- **Idempotency keys on money calls that lacked them.** `PayoutLinks.Create`,
  `PayoutLinks.CreateBatch` and `Wallets.BlockedAddressRefund` reserve funds but were sent without
  `Idempotency-Key`, so a retry after a lost response could fund a second link or repeat a payout.
  The key is now fixed before the retry loop; the gateway honours it on `/v1/payout/link` and
  `/v1/payout/link/batch` (a replay returns the same link and the same `claim_token`). `Reference`
  remains the second, durable layer.
- **Documented idempotency-layer codes** on payout links: `idempotency.key_reused` (400),
  `idempotency.bad_key` (400), `idempotency.in_progress` (409), `idempotency.unavailable` (503,
  fail-closed — the SDK retries it). A duplicate `Reference` became
  `payoutlink.duplicate_reference` (409 instead of 500): terminal, no longer retried in vain.
- **Payout-link batches:** a partially failed batch replays as it was (re-send failed elements with
  a NEW key), and an answer larger than 256 KB is not cached — set a per-item `Reference`.
- **Typed status constants** (`statuses.go`) for payments and payouts, both with `IsFinal()`. Purely
  additive: model fields stayed `string`.
- **A "Statuses" section in the README** with both tables and their terminal states.
- **`client.PaymentLinks`** — the canonical resource name across all Oblodai SDKs. `client.Links`
  remains a documented alias for the same object.
- **`oblodai.DefaultWebhookMaxAgeSeconds` (300) and `oblodai.DisableWebhookMaxAge` (-1).**

### Fixed

- **SECURITY: webhook replay protection turned itself off.** A zero `VerifyOptions.MaxAgeSeconds`
  (that is, an unset field) meant "do not check freshness", and `Now` lives in the same struct, so
  any `&VerifyOptions{Now: t}` accepted a captured webhook of any age. Zero now means the default
  (300 s); disabling requires the explicit `DisableWebhookMaxAge` sentinel.
- **SECURITY: a non-`https` base URL was accepted silently**, putting the signature, the public id
  and the body in clear text. The scheme must be `https`, except for loopback hosts
  (`localhost`, `127.0.0.0/8`, `[::1]`), which keep working for local stands.
- **BLOCKER: the README's webhook example passed the wrong secret.** Webhooks are signed with the
  endpoint secret from `Webhooks.Register(...).Secret`, not the API key secret; the parameter is now
  called `endpointSecret` everywhere.
- **Documented that webhook registration is an upsert** of the project's single endpoint: calling it
  with another URL returns the same `endpoint_id`, redirects deliveries and keeps the secret. There
  is no fan-out to several addresses.
- **Clarified `wrong_amount_waiting` vs `wrong_amount`.** While an underpayment is still open the
  invoice can still become `paid`, and `Resolve` answers 409 `resolution.not_underpaid`; the
  underpayment can only be settled once the invoice closes as `wrong_amount`.
- **Noted that `url` and `claim_url` are empty on a local stand**: the gateway builds all three
  links from `GATEWAY_PUBLIC_BASE_URL`, which local stands usually do not set.
- **Clarified `Sandbox.Reset`:** it cancels invoices in `check` and `select` only. An invoice whose
  deposit is already visible (`confirm_check`, `wrong_amount_waiting`) is deliberately left alone.
  Balances are zeroed either way.
- **Removed the false claim that the idempotency header is ignored on payout links** (seven places).
  Both payout-link routes are wrapped in idempotency and the header works; `Reference` is the
  second, durable layer rather than the only protection.
- **Removed the false claim that `Wallets.BlockedAddressRefund` has no deduplication**, and the
  harmful advice to disable retries for it. It is idempotent by state — a deterministic
  `refund-wallet:<wallet_id>` reference under a per-wallet advisory lock — and stronger than the
  header: a repeat returns the same payout, a concurrent repeat waits for the result instead of
  answering 409. A repeat with a different address returns the first payout to the first address.
- **Documented `Payouts.Approve` idempotency**: it is a state transition, accepted only from
  `pending`; otherwise `payout.not_pending` (409), which reads as "already approved".
- **Removed the false claim about "auto-maturing in about 10 minutes."** A sandbox deposit with too
  few confirmations never matures on its own — repeat `SimulateDeposit` with the same `TxID` and a
  higher `Confirmations`. The ~10 minutes belong to the payout maturity hold
  (`payout.funds_maturing`, `GATEWAY_SANDBOX_MATURITY_MINUTES`), which does not touch invoices.

## [1.1.0] — 2026-07-15

### Breaking

- **Idempotency moved to the `Idempotency-Key` header** (UUID v4, generated once before the retry
  loop, not covered by the signature). The SDK no longer writes a generated `order_id` into the
  body: set `order_id` yourself if you relied on it. A caller key travels as
  `params["idempotency_key"]`, which is moved into the header.
- **`Retry: nil` now means the default retry policy** (up to 4 attempts, 500 ms → 30 s backoff,
  `Retry-After` honoured), as in the other Oblodai SDKs. Disable retries explicitly with
  `Retry: oblodai.NoRetry()`.

### Added

- **Batches (up to 5000 elements per request):** `Payments.CreateBatch`, `Payments.RefundBatch`,
  `Payouts.CreateBatch` (`on_error: continue|stop`) and `client.Batches.Info`.
- **Payment links:** `client.Links` — `Create`, `List`, `Info`, `Toggle`, plus unsigned `PublicGet`
  and `Checkout`.
- **Split payments:** `client.Splits` — `CreateRule`, `SplitToAddress`, `SplitToMerchant`,
  `ListRules`, `DeleteRule`, `GetConfig`/`SetConfig`.
- **Payout links (crypto cheques):** `client.PayoutLinks` — `Create`, `CreateBatch` (≤500), `List`,
  `Info`, `Cancel`, plus unsigned `ClaimInfo`, `Claim`, `ClaimWithMemo`. Set `ExpiresInHours`
  explicitly: zero is clamped to the one-hour minimum (range 1–720).
- **Invoice by e-mail:** `Payments.SendEmail`.
- **Settling an underpayment:** `Payments.Resolve` — `accept` (keep the partial payment, which also
  silences the auto-refund) or `refund`.

## [1.0.2] — 2026-07-12

### Fixed

- **The caller's parameter map is no longer mutated.** `Payments.Create` and
  `Account.TransferToPersonal` write the generated `order_id` into a shallow copy. Reusing one
  `oblodai.Params` across two calls leaked the first call's `order_id` into the second, and the
  backend collapsed both operations into one.
- **Normalized the "order_id is missing" check:** it counts as set only when it is a non-empty
  string after trimming.
- **`Retry-After` is clamped to `[0, 5 min]`.** A huge value could overflow `time.Duration` into a
  negative delay and retry in a busy loop; the value is clamped before the multiplication.

## [1.0.1] — 2026-07-12

### Fixed

- **Retry safety (money).** `Payments.Create` and `Account.TransferToPersonal` fill in a stable
  idempotency key (`order_id = "idem-…"`) before the retry loop, so every attempt sends the same
  one. Payouts still require an explicit `order_id`.
- **`Retry-After` is no longer clamped to `MaxDelay`** — the server's header is honoured as sent
  (with an absolute ceiling of 5 minutes).
- **`payout.funds_maturing` is no longer treated as retryable**: wait for the funds to mature and
  repeat the call yourself.

## [1.0.0] — 2026-07-12

### Added

- First release of the official Go SDK for the Oblodai payment gateway.
- Accepting payments, payouts and mass payouts, static wallets, refunds, webhooks, public reference
  data (exchange rates, the currency and network catalogue).
- HMAC-SHA256 request signing and webhook signature verification (constant-time comparison, replay
  protection).
- `oblodai.NewFromEnv()` — `OBLODAI_PUBLIC_ID` / `OBLODAI_SECRET` / `OBLODAI_BASE_URL`.
- Automatic retries with exponential backoff, honouring `Retry-After` on 429.
