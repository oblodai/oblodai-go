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
<a href="https://github.com/oblodai/oblodai-go"><img src="https://img.shields.io/github/go-mod/go-version/oblodai/oblodai-go?style=flat-square" alt="Go version"></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-000000?style=flat-square" alt="License: MIT"></a>

[Documentation](https://docs.oblodai.com) · [Dashboard](https://my.oblodai.com) · [Читать по-русски →](README.ru.md)

</div>

---

The official Go SDK for the **Oblodai** payment gateway: accepting payments, payouts, bulk operations
(batches), payment links, payout links (crypto cheques), splits, static wallets, webhooks.
No external dependencies — standard library only. Automatic request signing, response parsing,
typed errors, and automatic retries.

> **Base URL.** Defaults to `https://api.oblodai.com`. Override `BaseURL` and your keys at initialization if needed. The scheme must be `https://` — the only exception is a local loopback environment (`http://localhost:8095`).

## Installation

```bash
go get github.com/oblodai/oblodai-go
```

Requires Go 1.22.2+.

## Where to get your keys

Keys are issued in the **Oblodai dashboard** (<https://oblodai.com>) — in the API keys section. A key
consists of a pair:

- **`public_id`** — the non-secret key identifier, sent in a header with every request;
- **`secret`** — the secret the SDK signs requests with. **Shown only once, at the moment the key is
  created** — save it to your secret store right away. A lost secret cannot be
  recovered: a new key is issued instead.

For development, use a **test key**: its `public_id` starts with `test_…`, and its secret with
`oblodai_test_…`. A test key works against the sandbox, where nothing costs real money — start
there (see the "Sandbox and testing" section). A live key differs only by the secret prefix
(`oblodai_live_…`); your integration code does not change when you switch.

The secret is access to money: keep it on the server, in environment variables or a secret store,
and never put it in browser code, a mobile app, or git.

## Credentials

Keep your keys in environment variables (see `.env.example`):

```bash
export OBLODAI_PUBLIC_ID=test_...
export OBLODAI_SECRET=oblodai_test_...
# optional: export OBLODAI_BASE_URL=https://api.oblodai.com
```

```go
// reads OBLODAI_PUBLIC_ID / OBLODAI_SECRET / OBLODAI_BASE_URL; Config fields override the environment
client, err := oblodai.NewFromEnv(oblodai.Config{})
```

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	oblodai "github.com/oblodai/oblodai-go"
)

func main() {
	// or explicitly (equivalent to NewFromEnv above):
	client, err := oblodai.New(oblodai.Config{
		PublicID: "test_...",         // test key — start with the sandbox
		Secret:   "oblodai_test_...", // the live key goes right here too, no code changes
		BaseURL:  "https://api.oblodai.com", // optional; the scheme must be https
		// Retry: nil — default retries; oblodai.NoRetry() — disable
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

	fmt.Println(payment.Address)       // address to pay to
	fmt.Println(payment.PaymentStatus) // "check" — no payment seen yet (see the "Statuses" section)
	fmt.Println(payment.URL)           // hosted payment page; empty on a local environment — see below
}
```

Every method takes a `context.Context` as its first argument — you can set timeouts and cancellation.

**The exact same code works with a live key — only the key pair changes, not a single line of code.**
So it makes sense to walk through the whole scenario in the sandbox first (next section) and plug in
live keys once the integration already works.

**`BaseURL` must be `https://`.** Over http, the request signature (`X-Signature`) and headers would
go out in plaintext, so `New` returns an error. The only exception is a local loopback environment
(`http://localhost:8095`, `http://127.0.0.1:…`, `http://[::1]:…`).

## Sandbox and testing (v1.2.0)

The gateway has a developer sandbox. **The business endpoints and integration code are identical** in
test and production — only the key changes: a test `public_id` starts with `test_…`, a test secret with
`oblodai_test_…`. Switching test ↔ production = swapping the key pair, not a single line of code.

What's new is five test-only helpers (`client.Sandbox`) that do not exist in production: they stand in
for "the buyer paid on-chain". **For test code only** — do not call them from a production integration:
a live key gets HTTP 403 with code `sandbox.live_key` on them. You can check a key with the
`oblodai.IsTestKey(publicID)` helper.

```go
client, _ := oblodai.New(oblodai.Config{
	PublicID: "test_...",         // test key — everything else is just like production
	Secret:   "oblodai_test_...",
})

// 1. Create an invoice — with exactly the same code as in production.
payment, _ := client.Payments.Create(ctx, oblodai.Params{
	"amount": "10", "currency": "USD", "order_id": "order-1",
	"to_currency": "USDT", "network": "tron",
})

// 2. "Pay" it: in production the buyer does this on-chain, in the sandbox — you do.
dep, _ := client.Sandbox.SimulateDeposit(ctx, oblodai.SandboxDepositParams{
	InvoiceID: payment.UUID, // empty Amount = pay exactly the invoice amount
})
_ = dep.TxID // repeat the same TxID with a higher Confirmations to deepen the deposit

// 3. Wait for paid as usual — via webhook or polling.
info, _ := client.Payments.Info(ctx, payment.UUID, "")

// 4. Balance for payouts is "minted" by the faucet (cap 1000000 per call)…
_, _ = client.Sandbox.Faucet(ctx, "USDT", "1000")

// 5. …and the payout is regular production code again.
_, _ = client.Payouts.Create(ctx, oblodai.Params{
	"amount": "25", "currency": "USDT", "network": "tron",
	"address": "T...", "order_id": "payout-1",
})
```

The remaining helpers: `Sandbox.Reset` (zero out balances and cancel invoices that were never paid —
see the caveat below), `Sandbox.ListWebhooks` (the latest ≤50 webhook deliveries, newest first, with the raw `Payload`) and
`Sandbox.ReplayWebhook(deliveryID)` (re-enqueue a delivery) — handy for debugging
your webhook handler.

**Fine points:**

- **Underpayment/overpayment** — pass an `Amount` smaller/larger than the invoice amount. Underpayment
  is then resolved by `Payments.Resolve` (`accept`/`refund`), just like in production — but **only after
  the invoice closes**: while it is in `wrong_amount_waiting`, `Resolve` returns 409
  `resolution.not_underpaid` (see the "Statuses" section).
- **`Sandbox.Reset` is not a "clean slate".** It zeroes out balances (with a compensating entry: the
  ledger is append-only, nothing is deleted) and cancels invoices **only** in the `check` and `select`
  statuses. An invoice whose deposit is **already visible** (`confirm_check`, `wrong_amount_waiting`) is
  deliberately **left alone**: cancelling it would let that deposit confirm into a cancelled invoice.
  The same rule applies in production, and the sandbox does not bypass it. Such invoices will sit out
  their term (or drive them to a terminal status yourself) — the balance is still zeroed out regardless.
  The experiment history stays in the journal and remains readable after the reset.
- **Shallow confirmations do NOT mature on their own.** A simulated deposit has no chain: nobody
  re-emits it, no cursor advances for it — an invoice with fewer `confirmations` than required hangs in
  `confirm_check` indefinitely. There is exactly one way to drive it to `paid`: repeat
  `SimulateDeposit` with the **same `TxID`** and a higher `Confirmations` (repeating the same `TxID`
  is idempotent — the amount is not doubled).
- **The ~10 minutes are about something else.** The ten-minute wait refers to the maturity **hold on a
  payout** (error `payout.funds_maturing`): in the sandbox the hold is lifted by age by a background
  job, after 10 minutes by default (`GATEWAY_SANDBOX_MATURITY_MINUTES`). This is about funds becoming
  available for withdrawal, not invoice confirmations — the job does not affect invoice status.
- **UTXO networks (Bitcoin and the like)** behave as in production: **no** automatic overpayment refund
  and **no** payer address (`payer_address`).
- `Sandbox.ListWebhooks` is the only signed `GET` in the API: signed over the same canonical
  string `{ts}\nGET\n{path}\n` with an empty body (the SDK does this itself).

## Idempotency (changed in v1.1.0)

Protection against duplicates on retries is the **`Idempotency-Key`** header: on creating calls
(`Payments.Create` / `Refund` / `Resolve` / `*Batch`, `Payouts.Create` / `CreateMass` / `CreateBatch`,
`Account.TransferToPersonal` / `TransferToUser` / `TransferBatch`) the SDK generates a UUID **once,
before the retry loop**, so all
internal retries send the same key and the gateway deduplicates the repeat. The header is not part of
the request signature.

- **`order_id` is sent as is.** The SDK no longer **injects or rewrites it** (breaking
  change: in v1.0.x an empty `order_id` was replaced with a generated one). `order_id` is your
  business identifier for lookups via `Payments.Info`; for payouts it is always required.
- **Your own idempotency key**: pass `params["idempotency_key"]` — it goes into the header
  (and is stripped from the body).
- **Payout links** (`PayoutLinks.Create` / `CreateBatch`) also reserve funds and are also
  retried automatically, so the SDK sends an `Idempotency-Key` on them — and **the gateway honors
  it**: `/v1/payout/link` and `/v1/payout/link/batch` are wrapped in idempotency. A repeat with the
  same key replays the first response (same link, same `claim_token`) with the
  `Idempotent-Replayed: true` header, and the balance is debited **exactly once**. Without the header
  the old behavior applies: two identical calls create two links. The second, durable layer is the
  per-link `Reference` (a unique index on `(merchant_id, reference)`): it works even without the
  header, and a duplicate yields `payoutlink.duplicate_reference` (409).
- **`Wallets.BlockedAddressRefund`** is deliberately **not** wrapped in idempotency on the gateway —
  and does not need it: it is idempotent by state, more strongly than the header. The gateway builds a
  deterministic reference `refund-wallet:<wallet_id>`, takes a per-wallet advisory lock, and inside
  the lock returns the already-existing payout if there is one. A repeat — with the header or without —
  returns **the same** payout; concurrent repeats wait for the result rather than getting a 409.
  Caveat: the address is not part of the reference, so a repeat with a **different** address returns
  the first payout to the **first** address.
- **`Payouts.Approve`** is a state transition, not a creation: no header needed. The gateway accepts
  only `pending` and otherwise replies `payout.not_pending` (409). Read that 409 as **"already
  approved"**, not as a failure, and check the status via `Payouts.Info`.
- **Your own key on these calls**: `PayoutLinkParams.IdempotencyKey`,
  `PayoutLinks.CreateBatchWithKey`, `Wallets.BlockedAddressRefundWithKey`.

### Idempotency-layer response codes

Endpoints wrapped in idempotency on the gateway (including `/v1/payout/link` and
`/v1/payout/link/batch`) may return:

| Code | HTTP | Meaning | Does the SDK retry it |
| --- | --- | --- | --- |
| `idempotency.key_reused` | 400 | the same key with a **different** body | no (terminal) |
| `idempotency.bad_key` | 400 | malformed key / longer than 255 characters | no (terminal) |
| `idempotency.in_progress` | 409 | a concurrent repeat while the first is still running | no — retry yourself a bit later with the **same** key |
| `idempotency.unavailable` | 503 | the idempotency store is unavailable (fail-closed by design) | **yes**, automatically |
| `payoutlink.duplicate_reference` | 409 | duplicate `Reference` (used to be a 500) | no (terminal) |

The SDK's classification (`APIError.IsRetriable`) already matches: only 5xx and 429 are retried,
and 4xx never are. That is why the `duplicate_reference` move from 500 to 409 matters: the SDK
retried the 500 in vain, while the 409 reaches you immediately.

**Batches.** A partially failed batch is replayed **as is**: the failed items are not retried under the
same key — send them with a **new** key. And the gateway does **not cache** a batch response over
256 KB, in which case a repeat executes anew — so on batches set a **per-item `Reference`** as the
second layer of protection.

## Webhook verification

A webhook signature is different from a request signature — the SDK handles both. For incoming
webhooks, take the **raw body** and the `X-Webhook-Timestamp` / `X-Webhook-Signature` headers.

> **⚠ The webhook secret is NOT `Config.Secret`.** Webhooks are signed with a **separate endpoint
> secret**, returned by `client.Webhooks.Register(ctx, url).Secret`. The API key secret
> (`Config.Secret`, `OBLODAI_SECRET`) signs your **outgoing** requests and has nothing to do with
> webhooks. Put it into `ConstructEvent` and you will reject **100%** of webhooks — the signature will
> never match. Save the endpoint secret at registration time (e.g. in `OBLODAI_WEBHOOK_SECRET`):
> the gateway returns it again only through the same `Register` call.

```go
reg, err := client.Webhooks.Register(ctx, "https://example.com/hooks/oblodai")
// reg.Secret — THIS and only this is what you pass to ConstructEvent/VerifyWebhook
```

> **⚠ `Register` is an upsert of the project's SINGLE endpoint, not "add another one".**
> Calling it again with a **different** URL does not create a second endpoint — it **redirects
> deliveries**: the same `EndpointID` comes back, and the old URL silently stops receiving anything.
> Fan-out to multiple addresses is not supported by the gateway — route events to consumers yourself,
> behind a single URL. **The secret is preserved on a URL change** and the previous one is returned:
> deliveries are signed with the secret in effect at enqueue time, and a new secret would orphan
> everything already sitting in the queue (401 → retries → dead-letter, i.e. lost
> `paid`/`payout` events).

```go
func handleWebhook(endpointSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body) // the RAW body — do not re-serialize

		// Probe bodies (is_test) are not signed
		var probe struct{ IsTest bool `json:"is_test"` }
		json.Unmarshal(raw, &probe)
		if probe.IsTest {
			w.WriteHeader(200)
			return
		}

		var event map[string]any
		// endpointSecret = Webhooks.Register(...).Secret, NOT your API key's Config.Secret
		err := oblodai.ConstructEvent(endpointSecret, raw, oblodai.WebhookHeadersFromRequest(r), nil, &event)
		if err != nil {
			w.WriteHeader(403) // *SignatureError
			return
		}

		if event["type"] == "payment" && event["status"] == "paid" {
			// mark the order event["order_id"] as paid (idempotent by uuid + status)
		}
		w.WriteHeader(200) // 2xx only after successful processing
	}
}
```

### Replay protection (freshness window)

`ConstructEvent` / `VerifyWebhook` check webhook freshness within a **300-second** window. The window
is **always** in effect unless you explicitly disabled it:

| `VerifyOptions` | Window |
| --- | --- |
| `nil` | 300 s (`DefaultWebhookMaxAgeSeconds`) |
| `&VerifyOptions{Now: t}` — `MaxAgeSeconds` field not set | 300 s |
| `&VerifyOptions{MaxAgeSeconds: 900}` | 900 s |
| `&VerifyOptions{MaxAgeSeconds: oblodai.DisableWebhookMaxAge}` (`-1`) | disabled |

**A zero `MaxAgeSeconds` means the default, not "disabled"** (fixed in v1.2.0):
otherwise `&VerifyOptions{Now: t}` — the only way to inject a clock in tests — would silently strip
replay protection in production. The window can be disabled only with the
`oblodai.DisableWebhookMaxAge` sentinel, and doing so should be a deliberate choice: without the
window, a once-intercepted webhook with a valid signature can be replayed at any moment.

## Statuses

Status fields in the models are plain `string`s (`Payment.PaymentStatus`, `Payout.Status`, etc.), as
before: your code around them changes nothing. Alongside them lives a **dictionary of named
constants** — `oblodai.PaymentStatus` and `oblodai.PayoutStatus` (file `statuses.go`), so you don't
type the strings by hand:

```go
if p.PaymentStatus == string(oblodai.PaymentStatusPaid) { /* paid */ }

// terminality when all you have is the string (gateway responses also carry a ready-made IsFinal field):
if oblodai.PaymentStatus(p.PaymentStatus).IsFinal() { /* no more transitions */ }
```

The constant values are exactly the strings that arrive in JSON, so comparing against a literal
(`p.PaymentStatus == "paid"`) keeps working too.

### Payment statuses (`Payment.PaymentStatus`)

| Value | Constant | Terminal | Meaning |
| --- | --- | :---: | --- |
| `check` | `PaymentStatusCheck` | no | invoice created, no payment seen yet |
| `confirm_check` | `PaymentStatusConfirmCheck` | no | payment seen, waiting for network confirmations |
| `wrong_amount_waiting` | `PaymentStatusWrongAmountWaiting` | **no** | a **partial** payment was seen, waiting for the remainder |
| `wrong_amount` | `PaymentStatusWrongAmount` | yes | the invoice **closed** underpaid |
| `paid` | `PaymentStatusPaid` | yes | paid in full |
| `paid_over` | `PaymentStatusPaidOver` | yes | overpaid |
| `cancel` | `PaymentStatusCancel` | yes | expired or cancelled |
| `select` | `PaymentStatusSelect` | no | currency-agnostic invoice: the buyer has not picked a currency yet |

**`wrong_amount_waiting` vs `wrong_amount` — two different moments, not synonyms.**

- `wrong_amount_waiting` — an underpayment **in progress**: less money arrived than needed, but the
  invoice is still alive, and a late top-up can still bring it to `paid`. There is nothing to resolve
  here, and the gateway forbids it: `Payments.Resolve` replies **409 `resolution.not_underpaid`**.
  Do not treat that 409 as an integration bug and do not retry it — just wait for the invoice to close.
- `wrong_amount` — the invoice **closed** underpaid; no top-up is coming. **Now**
  `Payments.Resolve` works: `accept` (keep the partial payment, suppresses the auto-refund) or
  `refund` (return it to the payer).

Ship goods on `paid` / `paid_over` (and on `wrong_amount` + `accept`, if that is your decision);
`confirm_check` and `wrong_amount_waiting` are "not money yet".

`paid_over` is an overpayment: the excess goes to auto-refund if it is enabled
(`Payments.SetAutorefund`) and the network supports it (UTXO networks have no auto-refund).

### Payout statuses (`Payout.Status`)

| Value | Constant | Terminal | Meaning |
| --- | --- | :---: | --- |
| `check` | `PayoutStatusCheck` | no | created, awaiting approval (`Payouts.Approve`, see `ApprovalRequired`) |
| `process` | `PayoutStatusProcess` | no | approved and on its way to / already in the network |
| `paid` | `PayoutStatusPaid` | yes | confirmed by the network |
| `fail` | `PayoutStatusFail` | yes | failed |
| `cancel` | `PayoutStatusCancel` | yes | cancelled |

Payout **link** statuses are a separate dictionary: `oblodai.PayoutLinkStatus*` (see the
"Payout links" section).

## Links on a local environment (`url`, `claim_url`)

The gateway **builds all public links from its public base URL**
(`GATEWAY_PUBLIC_BASE_URL`). There are three such fields:

| Field | Where from | What is built |
| --- | --- | --- |
| `Payment.URL` | `Payments.Create` / `Info` | `{base}/pay/{uuid}` — hosted payment page |
| `PayoutLink.ClaimURL` | `PayoutLinks.Create` | `{base}/claim/{token}` — cheque claim page |
| `PaymentLinkCreated.URL` / `PaymentLink.URL` | `PaymentLinks.Create` / `List` / `Info` | `{base}/link/{link_id}` — payment link page |

On a local environment this variable is usually unset — and all three fields arrive as an **empty
string**. This is not an SDK bug and not a gateway bug: in production the gateway simply does not
start without `GATEWAY_PUBLIC_BASE_URL`, so the fields are always populated there. But code you debug
locally should not rely on them — build the link yourself from the identifier: `payment.UUID`,
`link.ClaimToken` (the token always arrives, and
`PayoutLinks.ClaimInfo` / `Claim` work on exactly that) or `link.LinkID`
(`PaymentLinks.PublicGet` / `Checkout` work on it).

## Error handling

All API errors are `*APIError` with a machine-readable `Code`. Use `errors.As`.

```go
_, err := client.Payouts.Create(ctx, oblodai.Params{
	"amount": "25", "currency": "USDT", "network": "tron",
	"address": "T...", "order_id": "payout-1",
})

var apiErr *oblodai.APIError
if errors.As(err, &apiErr) {
	switch apiErr.Code {
	case "payout.insufficient_funds":
		// not enough funds
	case "payout.funds_maturing":
		// funds are still maturing — terminal (IsRetriable() == false); wait for maturity and retry
	}
	log.Printf("%s (HTTP %d): %s", apiErr.Code, apiErr.Status, apiErr.Message)
}
```

### Error types

| Type | When |
|---|---|
| `*APIError` | The API returned an `error` envelope. Carries `Code`, `Status`, `IsRetriable()`. |
| `*ConnectionError` | Network unreachable or timeout. |
| `*SignatureError` | Webhook signature verification failed. |

## Retries — enabled by default (changed in v1.1.0)

Transient errors (`5xx`, `429`, network failures) are retried automatically with exponential backoff
and jitter (up to 4 attempts, 500 ms initial, 30 s cap). Request errors (`4xx` except 429) are not
retried. The `Retry-After` header is honored as is (up to a 5-minute cap).

`Retry: nil` now means **default retries** — same as the other Oblodai SDKs (in v1.0.x `nil`
meant "no retries"). Disable deliberately:

```go
client, _ := oblodai.New(oblodai.Config{
	PublicID: "...", Secret: "...",
	Retry: oblodai.NoRetry(), // disable retries
	// or your own settings:
	// Retry: &oblodai.RetryConfig{MaxAttempts: 4, InitialDelay: 500 * time.Millisecond, MaxDelay: 30 * time.Second},
})
```

> **Important note on timeouts.** A timeout does not mean the operation did not go through. All
> internal retries of creating calls go out with the same `Idempotency-Key`, so on endpoints wrapped
> in idempotency on the gateway there will be no duplicate — if the operation was already created,
> the gateway returns that same one.
>
> This also applies to `PayoutLinks.Create` / `CreateBatch` — they are wrapped in idempotency on the
> gateway, so there is no need to disable retries for them (that would be a degradation).
> `Wallets.BlockedAddressRefund` has no header on the gateway, but it is idempotent by state via a
> deterministic reference under an advisory lock: a repeat returns the same payout. See the
> "Idempotency" section for details.

## Bulk operations (v1.1.0)

Up to 5000 payments / refunds / payouts in a single signed request — a single rate-limit tick.
Processing is in the background: submission returns a `batch_id`, results come via `Batches.Info`.

```go
sub, err := client.Payments.CreateBatch(ctx, []oblodai.Params{
	{"amount": "10", "currency": "USD", "order_id": "a-1", "to_currency": "USDT", "network": "tron"},
	{"amount": "20", "currency": "EUR", "order_id": "a-2", "to_currency": "USDT", "network": "tron"},
}, "continue") // "continue" (default) or "stop"

info, err := client.Batches.Info(ctx, sub.BatchID, 100, 0) // progress and per-item results
```

`order_id` is required on every payment/payout item; for refunds — `reference` plus the invoice's
`uuid`/`order_id`.

## Payment links, splits, invoice by e-mail (v1.1.0)

The payment links resource is available under **two names**: `client.PaymentLinks` — the canonical
one, matching `payment_links` / `paymentLinks` in the other Oblodai SDKs (port code between languages
without renames), and `client.Links` — a documented alias for **the very same object**
(`client.Links == client.PaymentLinks`). Both stay forever; pick either.

```go
// Payment link: many people pay, each payment is its own invoice. Takes money without your backend.
link, err := client.PaymentLinks.Create(ctx, oblodai.LinkParams{AmountMode: "open", Currency: "USD"})
// the same via the alias: client.Links.Create(...)

// Split: a share of every incoming payment automatically goes to a partner.
rule, err := client.Splits.SplitToAddress(ctx, "T...", "tron", 10.0, "partner A")

// Invoice by e-mail (an email with a "Pay" button).
_, err = client.Payments.SendEmail(ctx, payment.UUID, "" /* orderID */, "buyer@example.com")

// The fate of an underpaid payment: accept the partial payment or refund the payer.
res, err := client.Payments.Resolve(ctx, payment.UUID, "", "accept", nil)
```

## Payout links — crypto cheques (v1.1.0)

Reserve funds **without knowing the recipient's wallet**: the recipient opens the `ClaimURL`, enters
their address — and a regular payout is spawned from the reserve.

```go
link, err := client.PayoutLinks.Create(ctx, oblodai.PayoutLinkParams{
	Currency: "USDT", Network: "tron", Amount: "25",
	Reference:      "bonus-42", // ALWAYS SET THIS: the second, durable layer of duplicate protection — a
	                            // duplicate reference yields 409 payoutlink.duplicate_reference. Works even
	                            // without the header, and on batches whose response is too big for the cache (>256 KB)
	ExpiresInHours: 168,        // SET EXPLICITLY: at 0 the backend clamps to the minimum — 1 hour
	Email:          "user@example.com", // optional: an email with the claim link
})
// link.ClaimToken / link.ClaimURL are returned ONLY here — save them immediately.

// Public methods for your own claim page (no keys):
info, err := client.PayoutLinks.ClaimInfo(ctx, token)          // GET /v1/claim/{token}
claim, err := client.PayoutLinks.Claim(ctx, token, "T-address")  // POST /v1/claim/{token}
```

Link statuses: `funded` → `claiming` → `claimed`; or `expired` / `cancelled`
(the `oblodai.PayoutLinkStatus*` constants are a **separate** dictionary — do not confuse them with
the `PayoutStatus*` payout statuses). Up to 500 links at a time — `PayoutLinks.CreateBatch`.

The gateway builds `ClaimURL` from its public base URL — on a local environment without
`GATEWAY_PUBLIC_BASE_URL` it arrives empty; build the link from `ClaimToken` (see the "Links
on a local environment" section).

## Internal transfers to platform users (v1.2.0)

A **fee-free** transfer from the merchant balance to another platform user's personal wallet —
for example, paying a contractor who has an Oblodai account. `to_user_id` is the **user's UUID
on the platform, NOT a username** (usernames are resolved to ids on the dashboard side). Requires a
PAYOUT key; dedup is the same ladder as the other money calls: the `Idempotency-Key` header (the SDK
sends it itself) → `order_id` → signature.

```go
res, err := client.Account.TransferToUser(ctx, oblodai.Params{
	"to_user_id": "5c3a2c1e-9d0b-4f6a-8f3d-2b1a0c9e8d7f", // recipient's UUID, not a username
	"amount":     "25",
	"currency":   "USDT",
	"order_id":   "payroll-7", // optional
})
// res.RecipientBalance — the recipient's new balance

// A "payroll" batch (up to 5000 transfers in one request; processed in the background):
sub, err := client.Account.TransferBatch(ctx, []oblodai.Params{
	{"to_user_id": "…", "amount": "10", "currency": "USDT", "order_id": "s-1"},
	{"to_user_id": "…", "amount": "20", "currency": "USDT", "order_id": "s-2"},
}, "continue") // "continue" (default) or "stop"
info, err := client.Batches.Info(ctx, sub.BatchID, 100, 0) // progress and per-row results
```

## Public payment page — your own checkout (v1.2.0)

Public methods (`GET /v1/pay/{id}` and `POST /v1/pay/{id}/select`, **no signature and no keys**) —
for YOUR OWN payment page instead of the gateway's hosted page: the browser renders and polls the
invoice without the merchant secret.

```go
// The invoice's public state: address, amount, QR, status, expiry. Merchant-private fields
// (additional_data, payer_email, payer_address) are stripped by the backend.
pub, err := client.Payments.PublicGet(ctx, payment.UUID)

// For a currency-agnostic invoice (payment_status "select") pub.Accepted lists the methods to choose from;
// the choice locks the rate, allocates a deposit address, and returns the finalized invoice.
p, err := client.Payments.PublicSelect(ctx, payment.UUID, "USDT", "tron")
```

## Method overview

```go
// Payments
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
client.Payments.PublicGet(ctx, uuid)                      // v1.2.0, public, unsigned
client.Payments.PublicSelect(ctx, uuid, currency, network) // v1.2.0, public, unsigned

// Payouts
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

// Batches (v1.1.0)
client.Batches.Info(ctx, batchID, limit, offset)

// Payment links (v1.1.0). client.PaymentLinks is the canonical name, client.Links is an alias for the same object
client.PaymentLinks.Create(ctx, linkParams)
client.PaymentLinks.List(ctx, limit, offset)
client.PaymentLinks.Info(ctx, linkID)
client.PaymentLinks.Toggle(ctx, linkID, active)
client.PaymentLinks.PublicGet(ctx, linkID)        // public, unsigned
client.PaymentLinks.Checkout(ctx, linkID, params) // public, unsigned

// Splits (v1.1.0)
client.Splits.CreateRule(ctx, params)
client.Splits.SplitToAddress(ctx, address, network, percent, note)
client.Splits.SplitToMerchant(ctx, merchantID, percent, note)
client.Splits.ListRules(ctx) / DeleteRule(ctx, ruleID)
client.Splits.GetConfig(ctx) / SetConfig(ctx, refundHoldHours)

// Payout links — crypto cheques (v1.1.0)
client.PayoutLinks.Create(ctx, params)
client.PayoutLinks.CreateBatch(ctx, links) // up to 500
client.PayoutLinks.List(ctx, limit, offset)
client.PayoutLinks.Info(ctx, linkID)
client.PayoutLinks.Cancel(ctx, linkID)
client.PayoutLinks.ClaimInfo(ctx, token)                 // public, unsigned
client.PayoutLinks.Claim(ctx, token, address)            // public, unsigned
client.PayoutLinks.ClaimWithMemo(ctx, token, address, memo)

// Wallets
client.Wallets.Create(ctx, params)
client.Wallets.Block(ctx, address, forceBlock)
client.Wallets.BlockedAddressRefund(ctx, uuid, address)
client.Wallets.QR(ctx, address)

// Account
client.Account.Balance(ctx)
client.Account.Referral(ctx)
client.Account.TransferToPersonal(ctx, params)
client.Account.TransferToUser(ctx, params)             // v1.2.0, to_user_id is a UUID, not a username
client.Account.TransferBatch(ctx, transfers, onError)  // v1.2.0, up to 5000, results via Batches.Info
client.Account.VRCS(ctx, enabled)

// Webhooks
client.Webhooks.Register(ctx, url) // UPSERT of the project's single endpoint; .Secret is a SEPARATE
                                   // webhook signing secret, not Config.Secret
client.Webhooks.Deliveries(ctx)
client.Webhooks.TestPayment(ctx, params)

// Settings
client.Settings.ListAutoWithdraw(ctx) / SetAutoWithdraw(ctx, params) / DeleteAutoWithdraw(ctx, currency)
client.Settings.ListAllowlist(ctx) / AddAllowlist(ctx, cidr) / RemoveAllowlist(ctx, cidr) / EnableAllowlist(ctx, bool)

// Rates (public, no key)
client.Rates.List(ctx, "ETH")
client.Rates.Currencies(ctx)

// Sandbox — test keys ONLY (v1.2.0)
client.Sandbox.SimulateDeposit(ctx, params)
client.Sandbox.Faucet(ctx, asset, amount) / FaucetWithKey(ctx, asset, amount, key)
client.Sandbox.Reset(ctx)
client.Sandbox.ListWebhooks(ctx)           // signed GET
client.Sandbox.ReplayWebhook(ctx, deliveryID)
oblodai.IsTestKey(publicID)                // "test_…" → true
```

## Logging and debugging

Enabled via an environment variable (`export OBLODAI_LOG=debug`; levels `debug`/`info`/`warn`/`error`)
or your own logger: `Config{Logger: a slog logger}`. Secrets, signatures, and request bodies never
end up in the log — only the method, path, status, attempt number, and retry delay.

## Notes

- **Amounts are strings** in currency units (`"25.00"`), not numbers. That preserves precision.
- **`order_id` is your business identifier**, required for payouts; it is no longer an
  idempotency key (see the "Idempotency" section).
- **The secret stays on the server.** The SDK is server-side; do not embed the key in client apps.
- **There are two secrets.** `Config.Secret` signs outgoing requests; the secret from
  `Webhooks.Register(...).Secret` verifies incoming webhooks. They are not interchangeable.

## License

MIT
