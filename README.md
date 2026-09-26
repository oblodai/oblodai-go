<div align="center">

<a href="https://oblodai.com">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/oblodai/.github/main/brand/logo-white.svg">
    <img src="https://raw.githubusercontent.com/oblodai/.github/main/brand/logo-black.svg" alt="oblodai" height="52">
  </picture>
</a>

<h3>Official Go SDK for the <a href="https://oblodai.com">oblodai</a> payment gateway</h3>

Payments, payouts, payment links, splits, static wallets, webhooks — one API key.

<a href="https://pkg.go.dev/github.com/oblodai/oblodai-go/v2"><img src="https://pkg.go.dev/badge/github.com/oblodai/oblodai-go/v2.svg" alt="Go Reference"></a>
<img src="https://img.shields.io/github/go-mod/go-version/oblodai/oblodai-go?style=flat-square" alt="Go version">
<a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-000000?style=flat-square" alt="License: MIT"></a>

[Documentation](https://docs.oblodai.com) · [Dashboard](https://my.oblodai.com) · [Read in Russian →](README.ru.md)

</div>

---

The official Go SDK for the **Oblodai** payment gateway: accepting payments, payouts, bulk
operations (batches), payment links, payout links (crypto cheques), splits, static wallets,
transfers, documents, webhooks. Request signing, typed models, typed errors, idempotency, retries,
pagination and waiters for long-running jobs — out of the box. Go ≥ 1.25 and the standard library
only: **zero third-party dependencies**.

The whole API surface — every service, method, request and response model and enumeration — is
generated from the gateway's OpenAPI contract (`zz_generated_*.go`, never edited by hand), so every
operation the gateway exposes has a method here, named the same way in all eight Oblodai SDKs. The
hand-written runtime around it signs, retries, paginates and verifies.

> **Base URL.** Defaults to `https://api.oblodai.com`. Override it with `WithBaseURL`. The scheme
> must be `https://`; plain `http://` is accepted only for loopback (`http://127.0.0.1:8095`) or
> with `WithInsecureBaseURL(true)` (or `OBLODAI_ALLOW_INSECURE=1`).

## Installation

```bash
go get github.com/oblodai/oblodai-go/v2@v2.0.0
```

Go ≥ 1.25. The module is `github.com/oblodai/oblodai-go/v2`; webhook verification lives in the
standalone sub-package `github.com/oblodai/oblodai-go/v2/webhooks`, which needs no client and no API
key. Coming from 1.x? Read [MIGRATION-2.0.md](MIGRATION-2.0.md).

## Where to get keys

A merchant has **one API key**, issued in the [dashboard](https://my.oblodai.com) → **API keys**: a
public id `oblodai_<hex>` and a secret `oblodai_live_<hex>`. It signs every route the gateway gates
— payments and payouts alike. There is nothing to choose per call:

```go
client, err := oblodai.New(oblodai.WithCredentials(publicID, secret))
```

The environment fallback is `OBLODAI_PUBLIC_ID` / `OBLODAI_SECRET`. A sandbox pair (public id
`test_oblodai_<hex>`, secret `oblodai_test_<hex>`) drives a chainless copy of the gateway. Sandbox
store onboarding (`Sandbox.OnboardStore`) is unsigned; a self-hosted gateway gates it with an
**onboarding admin token** (`WithAdminToken`, or `OBLODAI_ADMIN_TOKEN`).

## Quick start

Every method takes a `context.Context` first, its parameters as a model second, and optional call
options last. Optional fields of a model are pointers — `oblodai.Ptr(v)` makes one. Create an
invoice:

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

To price in fiat, set `Currency: "USD", ToCurrency: oblodai.Ptr("USDT")` — `Currency` is what you
charge, `ToCurrency` the asset the payer sends. Send money out with the same key:

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

Runnable programs live in [`examples/`](examples): `accept-payment`, `payout`, `webhook-receiver` —
each is executed by its own test against a fake gateway.

## Sandbox / testing

A sandbox key drives a chainless copy of the gateway: fake balance from a faucet, simulated
deposits, real webhooks. The business endpoints behave exactly as they do live.

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

- `Sandbox.Faucet` credits test money. `WithIdempotencyKey` fills its `idempotency_key` body field,
  so a retry does not top up twice.
- `Sandbox.SimulateDeposit` pays an invoice: no `Amount` pays exactly what is due, anything else is
  an under- or overpayment, and fewer `Confirmations` than required exercises pending → confirmed.
- `Sandbox.ListWebhooks` lists deliveries with their payloads; `Sandbox.ReplayWebhook` re-sends one.
- `Webhooks.SendTestPayment` (and `…Payout`, `…Wallet`, `…Conversion`) rehearses a delivery against
  any receiver: signed like a real event, with `test: true` in the body. Never act on one.
- `Sandbox.Reset` cancels the store's open invoices and zeroes its balances.

## Method overview

The whole merchant surface, `client.<Service>.<Method>` (the table is generated from the contract):

<!-- sdkgen:methods -->
17 resources, 124 methods.

| Resource | Methods |
| --- | --- |
| `Payments` | `Create` · `GetInfo` · `GetQR` · `ListHistory` · `ListServices` · `Cancel` · `SendEmail` · `SetCheckoutConfig` · `GetCheckoutConfig` · `GetAmlLinks` · `Resolve` |
| `PaymentLinks` | `Create` · `List` · `Get` · `Toggle` |
| `Refunds` | `Payment` · `Calculate` · `BlockedWallet` |
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

Method names follow the contract's `operationId` without the resource name, in Go style; the
table of names is pinned in [`names.lock`](names.lock): the generator adds the names a newer
contract brings, and a name that disappears fails it as a breaking change. Every method's doc comment lists the error codes it can answer with
(`go doc oblodai.PayoutsService.Create`). Document routes return `*FileResult{Bytes, ContentType,
Filename}`.

### Lists

A paged list method returns a `*List[T]` that has requested nothing yet. `Items()` ranges over every
item, `ByPage()` over every page (one request each), `Page()` fetches the first page, `Pager()`
walks with an explicit cursor, `Collect(max)` gathers the items.

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

### Long-running jobs

Batches and document exports are accepted at once and finish later. `client.BatchJob(id)` and
`client.DocumentJob(id)` return a `*Job` whose `Wait` polls until the status is terminal (a
`failed` job is returned, not raised) and whose `Download` fetches an export's file. Which
operations are long-running is the `LRO` table, generated from the contract; `JobFor[T]` follows
any of them.

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

### Statuses and money

- Payment: `select → created → confirm_check → paid | paid_over | wrong_amount | expired | cancelled`.
  `IsPaymentPaid` is true for `paid`/`paid_over`; `wrong_amount` (underpaid) waits for
  `Payments.Resolve`; `IsPaymentFinal` covers the rest.
- Payout: `pending → approved → awaiting_cosign → broadcasting → sent → confirmed | failed | cancelled`.
- An enumeration is a string type: a value this release does not know is kept as received
  (`status.IsKnown()` tells), and a model keeps fields it does not know in `Extra`.
- Amounts are `oblodai.Decimal` — a string type, so a float does not fit, and a JSON number in an
  amount is refused with `sdk.float_amount`. `AddAmounts`, `SubtractAmounts`, `CompareAmounts`,
  `AmountsEqual`, `IsZeroAmount` do exact decimal arithmetic on a `Decimal` or a plain string.

## Webhooks

`Webhooks.Register` sets the endpoint and returns the signing secret — shown once. Verification
needs no client and no API key:

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

Verification runs over the **raw** bytes; `VerifyRequest` reads at most 1 MiB. Checks run in this
order: headers, HMAC (the current secret, then `Options.PreviousSecret`), freshness, body. Answer
4xx **only** when verification failed; a delivery that verified but cannot be read is
`webhook.bad_payload` — answer 5xx, the event is real and the core will retry it. `delivery.Event`
carries the typed body of its kind (`Payment`, `Payout`, `Wallet`, `Conversion` — the generated
webhook models); a kind a newer core added arrives with its `Type` and `Raw` body and
`IsKnown() == false`. `delivery.EventID` (`X-Webhook-Event-Id`) is stable for one state — the key to
deduplicate on; `webhooks.IsStale(event, lastSequence)` drops an out-of-order event. After
`Webhooks.RotateSecret` keep the old secret in `Options.PreviousSecret` for at least 26 hours.

## Errors

Every failure is an `*oblodai.Error`: recover it with `errors.As` (or `oblodai.AsError`) and branch
on `Code` — a stable `family.reason` string. It prints as `[code] message (request_id=…)`.

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

Fields: `Kind`, `Code`, `Message`, `HTTPStatus`, `Retryable` (authoritative — the client has
already retried what it should), `RetryAfter` (seconds), `RequestID` (the core's, else the call's
`X-Request-ID` — quote it to support), `Field` (on 400s), `Synthetic` (the answer came from a proxy),
`LastCode` (on `transport.deadline`: what the API last said). Predicates: `IsValidation`,
`IsAuthentication`, `IsPermission`, `IsNotFound`, `IsConflict`, `IsIdempotencyConflict`,
`IsRateLimit`, `IsUnavailable`, `IsInternal`, `IsTransport`, `IsConfig`, `IsContract`,
`IsSignature`, `IsWebhookPayload`, `IsCode`, `IsRetryable`. The core's codes are the generated
`ErrorCode` enumeration; the client adds `sdk.*` (`sdk.float_amount`, `sdk.idempotency_unsupported`,
`sdk.wait_timeout`, …), `transport.timeout|network|aborted|deadline` and `webhook.*`.

## Retries, idempotency and timeouts

- **Safe to repeat** comes from the contract: a read (`x-retry-safe`) is retried freely; a write
  only when the core deduplicates it by `Idempotency-Key` and a key was sent.
- An error is retried only when the core says `retryable`; answers with no envelope (a proxy 502)
  and transport failures only when repeating is safe. `Retry-After` wins over the backoff.
- **Idempotency keys** are generated on deduplicated routes — one per call, reused on every retry.
  Pass `WithIdempotencyKey` to survive a process restart; on routes the core does not deduplicate
  the client refuses a key with `sdk.idempotency_unsupported` rather than pretend.
- **Clock skew** is corrected from the API's `Date` header after a signature failure.
- **Redirects are never followed**; responses are capped at 8 MiB (64 MiB for documents).

Per call — `WithIdempotencyKey`, `WithRequestTimeout` (per attempt), `WithRequestBudget`,
`WithMaxRetries`, `WithExtraHeaders`/`WithRequestHeader`, `WithRequestID` (`X-Request-ID`, the same
on every attempt; generated when omitted) and `WithRawResponse` (status, headers, body and request
id of the answer). `client.WithOptions(...)` is a copy of the client with other options, and
`WithHooks` observes every attempt:

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

## Configuration

| Option                          | What it does                                                                     |
| ------------------------------- | -------------------------------------------------------------------------------- |
| `WithCredentials(id, secret)`   | the merchant's API key pair — it signs every gated route                          |
| `WithBaseURL(url)`              | the API origin; a path prefix is kept                                             |
| `WithInsecureBaseURL(true)`     | permit plain `http://` for a non-loopback host                                    |
| `WithAdminToken(token)`         | onboarding admin token of a self-hosted gateway (onboarding routes only)          |
| `WithHTTPClient(client)`        | your own `*http.Client`: proxy, custom transport, mutual TLS, a recording stub    |
| `WithTimeout(d)`                | per-attempt timeout (default 30 s)                                                |
| `WithCallBudget(d)`             | budget for one call including retries and pauses (default 90 s)                   |
| `WithRetry(opts)`               | retry policy; `RetryOptions{MaxRetries: 0}` disables retries                      |
| `WithHooks(hooks)`              | `OnRequest`/`OnResponse` on every attempt                                         |
| `WithLogger(logger)`            | structured logger for the client's diagnostics                                    |
| `WithHeader(name, value)`       | a header on every request (reserved names are ignored)                            |

| Environment variable       | Meaning                                                          |
| -------------------------- | ---------------------------------------------------------------- |
| `OBLODAI_PUBLIC_ID`        | API key public id                                                |
| `OBLODAI_SECRET`           | API key secret                                                   |
| `OBLODAI_ADMIN_TOKEN`      | onboarding admin token of a self-hosted gateway                  |
| `OBLODAI_BASE_URL`         | API origin (default `https://api.oblodai.com`)                   |
| `OBLODAI_LOG`              | `debug` \| `info` \| `warn` \| `error` — enables the text logger |
| `OBLODAI_ALLOW_INSECURE`   | `1` permits a plain `http://` base URL                           |

Explicit options win over the environment. **Secrets never print**: a model prints (`%v`, `%+v`,
`%#v`) the fields that are set with every secret-looking one — a webhook secret, a claim token or
URL, a passcode — as `[redacted]`; JSON stays faithful. A `Client` never prints its keys, and log
fields that look secret are redacted before any logger sees them.

## Development

```bash
make ci   # format, vet, lint, build, tests with -race, conformance, generated-code drift, packaging
```

`make ci` runs the shared conformance suite and the drift check against the backend checkout in
`OBLODAI_BACKEND` (default `../oblodai-backend`): the generated files must be exactly what
`tools/sdkgen` makes of `services/core/api/openapi.json`. Never edit `zz_generated_*.go` by hand —
regenerate with `make sdk` in the backend. `OBLODAI_LIVE_URL=http://127.0.0.1:8095 go test -run
TestLive ./...` runs the live tier against a real core. [AGENTS.md](AGENTS.md) is the same surface
in one page for coding agents; [CHANGELOG.md](CHANGELOG.md) says what changed.

## License

MIT — see [LICENSE](LICENSE).
