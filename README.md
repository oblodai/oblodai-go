<div align="center">

<a href="https://oblodai.com">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/oblodai/.github/main/brand/logo-white.svg">
    <img src="https://raw.githubusercontent.com/oblodai/.github/main/brand/logo-black.svg" alt="oblodai" height="52">
  </picture>
</a>

<h3>Official Go SDK for the <a href="https://oblodai.com">oblodai</a> payment gateway</h3>

Payments, payouts, payment links, splits, static wallets, webhooks — one API key.

<a href="https://pkg.go.dev/github.com/oblodai/oblodai-go"><img src="https://pkg.go.dev/badge/github.com/oblodai/oblodai-go.svg" alt="Go Reference"></a>
<a href="https://github.com/oblodai/oblodai-go/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/oblodai/oblodai-go/ci.yml?branch=main&style=flat-square&label=CI" alt="CI"></a>
<img src="https://img.shields.io/github/go-mod/go-version/oblodai/oblodai-go?style=flat-square" alt="Go version">
<a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-000000?style=flat-square" alt="License: MIT"></a>

[Documentation](https://docs.oblodai.com) · [Dashboard](https://my.oblodai.com) · [Читать по-русски →](README.ru.md)

</div>

---

The official Go SDK for the **Oblodai** payment gateway: accepting payments, payouts, bulk
operations (batches), payment links, payout links (crypto cheques), splits, static wallets,
transfers, webhooks. Request signing, response parsing, typed errors, idempotency and retries — out
of the box. Go ≥ 1.22 and the standard library only: **zero third-party dependencies**, at runtime
and in the tests alike; every route the gateway exposes has a method here, generated from the
gateway's own contract snapshot and verified against golden responses recorded from a live core.

> **Base URL.** Defaults to `https://api.oblodai.com`. Override `WithBaseURL` and supply your own
> keys at initialisation if needed. The scheme must be `https://`; plain `http://` is accepted only
> for loopback (`http://127.0.0.1:8095`) or with the explicit allow-insecure option
> (`WithInsecureBaseURL(true)`, or `OBLODAI_ALLOW_INSECURE=1`).

## Installation

```bash
go get github.com/oblodai/oblodai-go@v1.3.0
```

Go ≥ 1.22. The module is `github.com/oblodai/oblodai-go`; webhook verification lives in the
standalone sub-package `github.com/oblodai/oblodai-go/webhooks`, which needs no client and no API
key. Nothing else is pulled in: the SDK and its test suite import the standard library only.

## Where to get keys

A merchant has **one API key**, issued in the [dashboard](https://my.oblodai.com) → **API keys**: a
public id `oblodai_<hex>` and a secret `oblodai_live_<hex>`. It signs every route the gateway gates
— invoices, payment links, wallets, settings and documents on one side, `Payouts.*`, `Refunds.*`,
`PayoutLinks.*`, `Transfers.*`, `Splits.*` and the auto-withdraw rules on the other. There is
nothing to choose per call:

```go
client, err := oblodai.New(oblodai.WithCredentials(publicID, secret))
```

The environment fallback is `OBLODAI_PUBLIC_ID` / `OBLODAI_SECRET`. The sandbox pair — public id
`test_oblodai_<hex>`, secret `oblodai_test_<hex>` — comes from the sandbox onboarding
(`Merchants.CreateSandbox`) and drives a chainless copy of the gateway. Merchant provisioning
(`Merchants.Create`, `Merchants.CreateSandbox`) is unsigned — a self-hosted gateway gates it with an
**onboarding admin token** (`WithAdminToken`, or `OBLODAI_ADMIN_TOKEN`), which is the only other
credential this SDK knows.

> Merchants who still hold an old **split pair** (`oblodai_pk_<hex>` for payments,
> `oblodai_wk_<hex>` for payouts) get a 403 `merchant.wrong_key_kind` when the wrong half signs a
> call. Ask the dashboard for the single `oblodai_<hex>` key and the error goes away for good.

## Quick start

Every method takes a `context.Context` first and optional `RequestOption`s last. Create an invoice:

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

To price in fiat, set `Amount: "25", Currency: "USD", ToCurrency: "USDT"` — `Currency` is what you
charge, `ToCurrency` the asset the payer sends. Omit `Network` and the payer chooses it on the pay
page. Send money out with the same key:

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

Runnable programs live in [`examples/`](examples): `accept-payment`, `payout`, `webhook-receiver`.

## Sandbox / testing

A sandbox key drives a chainless copy of the gateway: fake balance from a faucet, simulated
deposits, real webhooks. The business endpoints behave exactly as they do live — only the key
changes, and a live key on a sandbox route is refused.

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

- `Sandbox.Faucet` credits test money, capped at 1000000 per call. Give it an `IdempotencyKey`
  when a retry must not top up twice.
- `Sandbox.Deposit` pays an invoice: no `Amount` pays exactly what is due, anything else produces an
  under- or overpayment, and `Confirmations` fewer than required exercises the pending → confirmed
  transition. Repeating a `TxID` adds confirmations instead of paying twice.
- `Sandbox.Webhooks` lists the deliveries with their payloads — what your receiver would have been
  sent — and `Sandbox.Replay(deliveryID)` re-sends a terminal one.
- `Webhooks.Test(kind, params)` rehearses a delivery against any receiver, sandbox or live: it is
  signed exactly like a real event and carries `test: true` in the signed body (and
  `X-Webhook-Test: true`). Check `delivery.IsTest` and never act on one as if money moved.
- `Sandbox.Reset` cancels the store's open invoices and zeroes its balances.

## Method overview

16 namespaces, 107 routes — the whole merchant surface.

| Namespace      | Methods                                                                                                                                                                                   | Routes |
| -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| `Payments`     | Create · Info/Get · Cancel · History/List · Batch · QR · Services · SendEmail · Resend · PublicView · Select · PublicQR                                                                    | 12     |
| `Refunds`      | Create · Resolve · Batch                                                                                                                                                                   | 3      |
| `Payouts`      | Create · Validate · Calculate · Info/Get · Cancel · Approve · History/List · Mass · Batch · Services · Get/SetFeeConfig · Get/SetRefundFeeConfig                                           | 14     |
| `PayoutLinks`  | Create · Info/Get · List · Cancel · Batch · Cheque · ClaimPreview · Claim                                                                                                                  | 8      |
| `PaymentLinks` | Create · Info/Get · List · Toggle · PublicView · Checkout                                                                                                                                  | 6      |
| `Transfers`    | ToPersonal · ToUser · Batch                                                                                                                                                                | 3      |
| `Batches`      | Info (asynchronous batch progress)                                                                                                                                                         | 2      |
| `Wallets`      | Create · QR · Block · RefundBlockedDeposit                                                                                                                                                 | 4      |
| `Webhooks`     | Register · RotateSecret · Deliveries · Test                                                                                                                                                | 5      |
| `Documents`    | Statement · Ledger · BalanceCertificate · FeeSchedule · SplitReport · BatchReport · LinkReport · WalletStatement · ReferralsReport · CreateJob · JobInfo · JobFile · Download               | 13     |
| `Splits`       | CreateRule · ListRules · DeleteRule · Get/SetConfig · Get/SetOptIn                                                                                                                          | 7      |
| `Settings`     | SetDiscount · ListDiscounts · Get/SetAccuracy · Get/SetAutoRefund · ListAccepted · SetAccepted · Get/SetPaymentFeeConfig · List/Set/DeleteAutoWithdraw · List/Add/Remove/EnableAPIAllowlist | 17     |
| `Account`      | Balance · Referral · VRCS/SetVRCS                                                                                                                                                          | 4      |
| `Catalog`      | Currencies · ExchangeRates                                                                                                                                                                 | 2      |
| `Sandbox`      | Faucet · Deposit · Webhooks · Replay · Reset                                                                                                                                               | 5      |
| `Merchants`    | Create · CreateSandbox (provisioning; `WithAdminToken` on a self-hosted gateway)                                                                                                           | 2      |

Lookups carry both identifiers, so `PaymentInfoParams{UUID: id}` and
`PaymentInfoParams{OrderID: "order-1001"}` both work. Ids are plain strings
(`PayoutLinks.Cancel(ctx, linkID)`). Synchronous bulk calls (`Payouts.Mass` ≤ 100,
`PayoutLinks.Batch` ≤ 500) answer per element with `BatchElement{Idx, OK, Result, Message,
ErrorCode}`; asynchronous ones (`Payments.Batch`, `Payouts.Batch`, `Refunds.Batch`,
`Transfers.Batch`, ≤ 5000) are polled through `Batches.Info`. Document routes answer outside the
JSON envelope and return `*FileResult{Bytes, ContentType, Filename}`. The money-moving methods list
the error codes worth branching on in their own doc comments (`go doc oblodai.PayoutsService.Create`).

### Lists

A list method returns a `*List[T]` that has requested nothing yet. `Page()` fetches the first page,
`Pager()` walks every page one request at a time, `All(max)` collects them. The first page is
memoised, so several goroutines may call `Page()` on one list and share the single request; a
`Pager` is single-consumer state and belongs to one goroutine.

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

### Statuses

- Payment: `select → created → confirm_check → paid | paid_over | wrong_amount | expired | cancelled`.
  `IsPaymentPaid(status)` is true for `paid`/`paid_over`; `wrong_amount` (underpaid) waits for
  `Refunds.Resolve` with action `accept` or `refund`; `IsPaymentFinal` covers the rest.
- Payout: `pending → approved → awaiting_cosign → broadcasting → sent → confirmed | failed | cancelled`.

Prefer webhooks for state changes; poll `Info` only as a fallback.

### Money helpers

`AddAmounts`, `SubtractAmounts`, `CompareAmounts`, `AmountsEqual`, `IsZeroAmount` — exact decimal
arithmetic on the string amounts the API uses. Never parse a `Money` into a float: USDT has 6
decimals, BTC 8 and ETH 18, and binary floating point holds none of them exactly. `Money` is an
alias of `string`, so Go will let you write `a < b`: do not — `"9" < "10"` is true as text and false
as money. Anything that is not `-?digits[.digits]` (at most 64 characters, no trailing dot, no
exponent) is refused with `sdk.bad_amount`.

## Webhooks

`Webhooks.Register(ctx, url)` sets (or replaces) the endpoint and returns the signing secret — shown
once, so store it where the receiver can read it. Verification needs no client and no API key:

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

Verification always runs over the **raw** bytes; `VerifyRequest` reads at most `MaxBodySize`
(1 MiB) from the request. Checks run in this order: headers, HMAC (the current secret, then
`Options.PreviousSecret`), freshness, body. An empty `Secret` or a negative `Tolerance` is a
`ConfigError`; `Tolerance` zero means the 5-minute default (in Go the zero value is "unset", so
freshness is never disabled by accident), and `SkipTimestampCheck: true` is how it is switched off
deliberately.

The receiver's status-code rule: answer 401 (or any 4xx) **only** when verification failed — a
forged or stale delivery. A delivery that verified but cannot be read is `webhook.bad_payload`, a
*contract* error rather than a signature one: answer 5xx, because the event is real and the core
will retry it. An event type a newer core added arrives as `*oblodai.UnknownEvent` carrying its raw
type instead of an error — narrow with `webhooks.IsKnownEvent` before switching on the concrete
types.

Rehearsal deliveries (`Webhooks.Test`, sandbox) are signed exactly like live ones and carry
`test: true` in the body (and `X-Webhook-Test: true`): check `delivery.IsTest` (or
`webhooks.IsTestEvent(event)`) and never act on one as if money moved — no order shipped, no balance
credited. `delivery.ID` (`X-Webhook-Id`) is stable across retries — use it to deduplicate;
`event.Seq()` orders events, and `webhooks.IsStale(event, lastSequence)` drops an out-of-order one
(it is false for an event without a sequence). After `Webhooks.RotateSecret` keep the old secret in
`Options.PreviousSecret` for at least 26 hours: deliveries queued before the rotation stay signed
with it for their whole retry life.

## Errors

Every failure is an `*oblodai.Error` carrying the API's error envelope. Recover it with
`errors.As(err, &apiErr)` (or `oblodai.AsError`) and branch on `Code` — a stable `family.reason`
string — never on the message.

| Kind                      | HTTP           | When                                                              |
| ------------------------- | -------------- | ----------------------------------------------------------------- |
| `KindValidation`          | 400            | malformed request or a business rule; `Field` names the culprit    |
| `KindAuthentication`      | 401            | bad signature, unknown key, clock skew, IP not allow-listed        |
| `KindPermission`          | 403            | the key is valid but not allowed here (a feature is off)           |
| `KindNotFound`            | 404            | no such object for this merchant                                   |
| `KindConflict`            | 409            | a state conflict                                                   |
| `KindIdempotencyConflict` | 409            | `idempotency.key_reused`: same key, different body                 |
| `KindRateLimit`           | 429            | `RetryAfter` is set                                                |
| `KindUnavailable`         | 503            | an upstream dependency is down; safe to retry after a pause        |
| `KindInternal`            | other 5xx      | the core failed                                                    |
| `KindAPI`                 | anything else  | an error status with an envelope                                   |
| `KindTransport`           | —              | no response at all: DNS, TCP, TLS, timeout, cancellation           |
| `KindConfig`              | —              | refused before sending: bad options, missing credentials           |
| `KindContract`            | —              | the answer could not be read as the documented envelope            |
| `KindSignature`           | —              | webhook verification failed                                        |

Fields: `Code`, `Message`, `HTTPStatus`, `Retryable` (authoritative — the client has already retried
what it should), `RetryAfter` (seconds), `RequestID` (quote it to support), `Field` (on 400s),
`Synthetic` (the answer came from a proxy, not the API), `LastCode` (on `transport.deadline`: what
the API last said), `Kind`. Predicates cover the same ground: `IsValidation`, `IsAuthentication`,
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

The catalogue is `oblodai.ErrorCodes` — all 469 codes the core can answer with, shipped in the
contract snapshot. Codes worth handling first: `payout.insufficient_funds` and
`payout.funds_maturing` (both retryable), `idempotency.key_reused`, `invoice.not_payable`,
`payment.not_found`, `merchant.bad_signature`, `request.rate_limited`.
The client raises its own families on top: `sdk.missing_credentials`, `sdk.bad_config`,
`sdk.bad_idempotency_key`, `sdk.idempotency_unsupported`, `sdk.bad_envelope`, `sdk.bad_path_param`,
`sdk.bad_amount`, `sdk.bad_header`, `sdk.response_too_large`,
`transport.timeout|network|aborted|deadline`,
`webhook.bad_signature|stale_timestamp|missing_header|bad_payload`.

`RetryAfter` is reported in seconds, clamped to `[0, MaxRetryAfterSeconds]` (a day) whether it came
from the envelope or from the `Retry-After` header — what the retry loop actually sleeps stays
bounded by `RetryOptions.MaxRetryAfter`. When a call runs out of budget the error is
`transport.deadline` and carries what the API last said: `LastCode`, `HTTPStatus`, `RetryAfter`,
`RequestID`, with the last error itself reachable through `errors.Unwrap`.

Marshalling an `*Error` to JSON keeps the identity (code, message, status, request id) and drops the
raw body, so a structured log cannot leak what the body carried. `Error.Body()` still returns it for
debugging.

## Retries, idempotency and timeouts

- **Safe to repeat** is not guessed: `Routes[key].Safe` is the core's own read-only classification,
  shipped in the contract snapshot, and codegen fails on a snapshot that omits it.
- An error is retried only when the API says `retryable`. Answers with no API envelope (a proxy
  502/503) and transport failures are retried only on read routes and keyed writes. `Retry-After` is
  honoured over the computed backoff.
- **Idempotency keys** are attached automatically on create-type routes — one per logical call,
  reused on every retry — so a timeout can never produce a second payout. Pass `WithIdempotencyKey`
  to make retries safe across process restarts; on routes the gateway does not deduplicate (list
  methods included) the client refuses a key with `sdk.idempotency_unsupported` rather than let you
  believe a re-send is safe.
- **Per call:** `WithIdempotencyKey`, `WithRequestTimeout`, `WithRequestBudget`,
  `WithRequestHeader`. **Per client:** `WithTimeout` (per attempt, 30 s), `WithCallBudget` (attempts
  plus pauses, 90 s), `WithRetry(oblodai.RetryOptions{MaxRetries, BaseDelay, MaxDelay,
  MaxRetryAfter})`. Cancelling the context aborts everything, including a retry pause.
- **Clock skew** is corrected from the API's `Date` header after a 401 that looks like skew, and the
  correction is reverted when it does not help; `Client.ClockOffset()` reports it.
- **Redirects are never followed**: a signed request must not be replayed against another origin, so
  a redirect is reported as an error.
- **Body size caps**: 8 MiB on JSON routes, 64 MiB on document routes — an answer larger than that is
  `sdk.response_too_large`, a contract error, rather than an out-of-memory. Webhook verification
  reads at most 1 MiB (`webhooks.MaxBodySize`).
- **Reserved headers** win over `WithHeader`/`WithRequestHeader`, compared case-insensitively:
  `ReservedHeaders()` is `X-Public-Id`, `X-Signature`, `X-Timestamp`, `Idempotency-Key`,
  `X-Admin-Token`, `Accept`, `User-Agent`, `Content-Type`, `Content-Length`, `Host`. A header
  carrying a line break or a non-ASCII byte is refused with `sdk.bad_header` before anything is sent.

## Configuration

| Option                          | What it does                                                                     |
| ------------------------------- | -------------------------------------------------------------------------------- |
| `WithCredentials(id, secret)`   | the merchant's API key pair — it signs every gated route                          |
| `WithBaseURL(url)`              | the API origin; a path prefix is kept                                             |
| `WithInsecureBaseURL(true)`     | permit plain `http://` for a non-loopback host                                    |
| `WithAdminToken(token)`         | onboarding admin token of a self-hosted gateway (provisioning routes only)        |
| `WithHTTPClient(client)`        | your own `*http.Client`: proxy, custom transport, mutual TLS, a recording stub    |
| `WithTimeout(d)`                | per-attempt timeout (default 30 s)                                                |
| `WithCallBudget(d)`             | budget for one call including retries and pauses (default 90 s)                   |
| `WithRetry(opts)`               | retry policy; `RetryOptions{MaxRetries: 0}` disables retries                      |
| `WithLogger(logger)`            | structured logger for the client's diagnostics                                    |
| `WithHeader(name, value)`       | a header on every request (reserved names are ignored)                            |

| Environment variable       | Meaning                                                       |
| -------------------------- | -------------------------------------------------------------- |
| `OBLODAI_PUBLIC_ID`        | API key public id                                              |
| `OBLODAI_SECRET`           | API key secret                                                 |
| `OBLODAI_ADMIN_TOKEN`      | onboarding admin token of a self-hosted gateway                |
| `OBLODAI_BASE_URL`         | API origin (default `https://api.oblodai.com`)                 |
| `OBLODAI_LOG`              | `debug` \| `info` \| `warn` \| `error` — enables the text logger |
| `OBLODAI_ALLOW_INSECURE`   | `1` permits a plain `http://` base URL                         |

Explicit options win over the environment. Half a key pair (an id without its secret, or the other
way round) is refused at `New` with `sdk.bad_config`; missing credentials surface later, on the first
call that needs them.

**Secrets never print.** `WebhookEndpoint.Secret`, `WebhookSecretRotated.Secret`, `APIKeyPair.Secret`,
`PayoutLink.ClaimToken`, `PayoutLink.ClaimURL` (it embeds the token) and `PayoutLink.Passcode` read
normally as fields and render as `[redacted]` in `fmt` (`%v`, `%+v`, `%#v`) and in `json.Marshal` —
store them by reading the field, not by serialising the struct. A `Client` never prints its keys
either. Log fields whose name looks like a secret are redacted inside the client, before the value
reaches any logger, including one installed with `WithLogger`.

**Self-hosted or local gateway.** `WithBaseURL("http://127.0.0.1:8095")` works out of the box; any
other plain-http host needs `WithInsecureBaseURL(true)` (or `OBLODAI_ALLOW_INSECURE=1`). A path
prefix in the base URL is kept, so `https://gw.corp/oblodai` reaches
`https://gw.corp/oblodai/v1/payment` — and the signature covers the prefixed path.

## The contract snapshot

`contract/` is exported by the gateway's own test suite: the route registry (107 routes, each with
the core's own `safe` flag), request DTO schemas, every vocabulary and all 469 error codes, signing
vectors, golden response bodies recorded from a live core, and 43 real signed webhook deliveries.
Only `contract/descriptions.en.json` (the English field docs) is repo-local; everything else is
replaced wholesale on a refresh. `contract_routes.go`, `contract_enums.go`, `contract_requests.go`
and `contract_version.go` are generated from it and are never edited by hand.
`ContractCoreCommit`, `ContractExportedAt` and `ContractHash` identify the snapshot in use.

```bash
go generate ./...                  # regenerate after refreshing contract/
go run ./internal/codegen -check   # drift gate: fails when the generated files are stale
go test ./...                      # unit + contract tiers (hermetic)
```

The contract tier is a completeness gate, not a sample: every one of the 107 routes must have a
method wired to the right path, auth gate and idempotency behaviour, and every recorded response
body must decode into a model whose fields match the wire key for key.

## Development

```bash
git clone https://github.com/oblodai/oblodai-go && cd oblodai-go
gofmt -l .                         # formatting gate: must print nothing
go vet ./... && staticcheck ./...  # static analysis
go test -race ./...                # everything, hermetic
OBLODAI_LIVE_URL=http://127.0.0.1:8095 go test -run TestLive ./...   # against a real core
```

Source files stay under ~400 lines, and tests live next to what they test. Read
[AGENTS.md](AGENTS.md) for the same surface in one page, written for coding agents;
[CHANGELOG.md](CHANGELOG.md) for what changed; [MIGRATION-1.3.md](MIGRATION-1.3.md) for the move
from 1.2.

## License

MIT — see [LICENSE](LICENSE).
