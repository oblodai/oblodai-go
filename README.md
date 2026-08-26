# Oblodai Go SDK

Official Go client for the [Oblodai](https://oblodai.com) crypto payment gateway: invoices, payouts,
refunds, payout links, static wallets, webhooks, documents — the whole merchant API, generated from
the gateway's own contract snapshot and verified against golden responses recorded from a live core.

- Go ≥ 1.22, standard library only: **zero third-party dependencies**, runtime and tests alike.
- Every route the gateway exposes has a method here; request bodies and vocabularies are generated.
- Retries driven by the API's own `retryable` flag, automatic idempotency keys, clock-skew correction.
- `oblodai-go/webhooks`: signature verification that needs no client and no API key.

```bash
go get github.com/oblodai/oblodai-go@v1.3.0
```

## Start in the sandbox

Get your keys in the Oblodai dashboard. A **sandbox key** (`test_…`) drives a chainless copy of the
gateway — fake balance from a faucet, simulated deposits, real webhooks — so integrate against it first.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/oblodai/oblodai-go"
)

func main() {
	client, err := oblodai.New() // OBLODAI_PUBLIC_ID / OBLODAI_SECRET from the environment
	if err != nil {
		log.Fatal(err)
	}
	invoice, err := client.Payments.Create(context.Background(), oblodai.PaymentParams{
		Amount:      "25",                // amounts are decimal strings, never floats
		Currency:    "USDT",              // what you price in — a fiat (USD, EUR, …) or a crypto asset
		Network:     oblodai.NetworkTron, // omit to let the payer choose the network on the pay page
		OrderID:     "order-1001",        // your reference; idempotent per order_id
		URLCallback: "https://shop.example/oblodai/webhook",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(invoice.URL, invoice.Address, invoice.Status) // "created"
}
```

Prices in fiat: `Amount: "25", Currency: "USD", ToCurrency: "USDT"` — `Currency` is what you charge,
`ToCurrency` the asset the payer sends. Runnable programs live in [`examples/`](examples).

### Two keys

The gateway issues a **payment key** (`pk_…`) and a **payout key** (`wk_…`). Sandbox keys are both at
once; live keys are separate, and money-out routes need the payout one: `Payouts`, `Refunds`,
`PayoutLinks`, `Transfers`, `Splits`, `Wallets.RefundBlockedDeposit`, auto-withdraw, the IP
allow-list, `Webhooks.RotateSecret`, `Sandbox.Faucet`/`Reset`. Pass both pairs and the client picks
the right one per call:

```go
client, err := oblodai.New(
	oblodai.WithCredentials(publicID, secret),
	oblodai.WithPayoutCredentials(payoutPublicID, payoutSecret),
)
// or OBLODAI_PUBLIC_ID / OBLODAI_SECRET / OBLODAI_PAYOUT_PUBLIC_ID / OBLODAI_PAYOUT_SECRET
```

A call with the wrong kind is a 403 `merchant.wrong_key_kind`.

## Resources

| Service                     | Methods                                                                                                                                                                                       |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Payments`                  | Create · Info/Get · Cancel · History/List · Batch · QR · Services · SendEmail · Resend · PublicView · Select · PublicQR                                                                        |
| `Refunds`                   | Create · Resolve · Batch                                                                                                                                                                       |
| `Payouts`                   | Create · Validate · Calculate · Info/Get · Cancel · Approve · History/List · Mass · Batch · Services · Get/SetFeeConfig · Get/SetRefundFeeConfig                                               |
| `PayoutLinks`               | Create · Info/Get · List · Cancel · Batch · Cheque · ClaimPreview · Claim                                                                                                                      |
| `PaymentLinks`              | Create · Info/Get · List · Toggle · PublicView · Checkout                                                                                                                                      |
| `Batches` / `Transfers`     | Info · ToPersonal · ToUser · Batch                                                                                                                                                             |
| `Wallets`                   | Create · QR · Block · RefundBlockedDeposit                                                                                                                                                     |
| `Webhooks`                  | Register · RotateSecret · Deliveries · Test                                                                                                                                                    |
| `Documents`                 | Statement · Ledger · BalanceCertificate · FeeSchedule · SplitReport · BatchReport · LinkReport · WalletStatement · ReferralsReport · CreateJob · JobInfo · JobFile · Download                   |
| `Splits`                    | CreateRule · ListRules · DeleteRule · Get/SetConfig · Get/SetOptIn                                                                                                                             |
| `Settings`                  | SetDiscount · ListDiscounts · Get/SetAccuracy · Get/SetAutoRefund · ListAccepted · SetAccepted · Get/SetPaymentFeeConfig · List/Set/DeleteAutoWithdraw · List/Add/Remove/EnableAPIAllowlist     |
| `Account` / `Catalog`       | Balance · Referral · VRCS/SetVRCS · Currencies · ExchangeRates                                                                                                                                 |
| `Sandbox`                   | Faucet · Deposit · Webhooks · Replay · Reset                                                                                                                                                   |
| `Merchants`                 | Create · CreateSandbox (provisioning; `WithAdminToken` on a self-hosted gateway)                                                                                                               |

Every method takes a `context.Context` first and optional `RequestOption`s last:
`WithIdempotencyKey`, `WithRequestTimeout`, `WithRequestBudget`, `WithPayoutKey`. Lookups carry both
identifiers, so `PaymentInfoParams{UUID: id}` and `PaymentInfoParams{OrderID: "order-1001"}` both work.

### Lists

A list method returns a `*List[T]` that has requested nothing yet. `Page()` fetches the first page,
`Pager()` walks every page one request at a time, `All(max)` collects them.

```go
page, err := client.Payments.History(ctx, oblodai.PaymentHistoryParams{Limit: &fifty}).Page()
// page.Items, page.Paginate.{Total, PerPage, Offset, HasPages}

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

### Errors

Every failure is an `*oblodai.Error` carrying the API's error envelope: `Code`
(`payout.insufficient_funds`), `HTTPStatus`, `Retryable`, `RetryAfter`, `RequestID`, `Field`,
`Synthetic`. Recover it with `errors.As`, or use the predicates: `IsValidation` (400),
`IsAuthentication` (401), `IsPermission` (403), `IsNotFound` (404), `IsConflict` /
`IsIdempotencyConflict` (409), `IsRateLimit` (429), `IsUnavailable` (503), `IsInternal`,
`IsTransport` (no response), `IsConfig` (refused before sending), `IsContract`, `IsSignature`.
Quote `RequestID` to support.

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

Marshalling an `*Error` to JSON keeps the identity (code, message, status, request id) and drops the
raw body, so a structured log cannot leak what the body carried. `Error.Body()` still returns it for
debugging.

### Retries and idempotency

- Create-type routes get an `Idempotency-Key` automatically — one per logical call, reused on every
  retry — so a timeout can never produce a second payout. Pass `WithIdempotencyKey` to make retries
  safe across process restarts; on routes the gateway does not deduplicate the client refuses a key
  (`sdk.idempotency_unsupported`) rather than let you believe a re-send is safe.
- An error is retried only when the API says `retryable`. Answers with no API envelope (a proxy
  502/503) and transport failures are retried only on read routes and keyed writes. `Retry-After` is
  honoured over the computed backoff.
- `WithRetry(oblodai.RetryOptions{MaxRetries, BaseDelay, MaxDelay, MaxRetryAfter})`,
  `WithTimeout` per attempt, `WithCallBudget` per call (attempts plus pauses). Cancelling the
  context aborts everything, including a retry pause.

### Webhooks

```go
import "github.com/oblodai/oblodai-go/webhooks"

func handler(w http.ResponseWriter, r *http.Request) {
	delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: os.Getenv("OBLODAI_WEBHOOK_SECRET")})
	if err != nil {
		http.Error(w, "bad signature", http.StatusBadRequest)
		return
	}
	if delivery.IsTest { // a rehearsal delivery: signed like a live one, but no money moved
		w.WriteHeader(http.StatusOK)
		return
	}
	switch event := delivery.Event.(type) {
	case *oblodai.PaymentEvent:
		if event.Status == oblodai.PaymentStatusPaid {
			markOrderPaid(event.OrderID, delivery.ID)
		}
	case *oblodai.PayoutEvent:
	case *oblodai.WalletEvent:
	}
	w.WriteHeader(http.StatusOK)
}
```

Verification always runs over the **raw** bytes. Rehearsal deliveries (`Webhooks.Test`, sandbox) are
signed exactly like live ones and carry `test: true` in the body (and `X-Webhook-Test: true`): check
`delivery.IsTest` (or `webhooks.IsTestEvent(event)`) and never act on one as if money moved — no
order shipped, no balance credited. `delivery.ID` (`X-Webhook-Id`) is stable across
retries — use it to deduplicate; `event.Seq()` orders events (`webhooks.IsStale`). After
`Webhooks.RotateSecret` keep the old secret in `Options.PreviousSecret` for at least 26 hours.

### Money helpers

`AddAmounts`, `SubtractAmounts`, `CompareAmounts`, `AmountsEqual`, `IsZeroAmount` — exact decimal
arithmetic on the string amounts the API uses. Never parse a `Money` into a float: USDT has 6
decimals, BTC 8 and ETH 18, and binary floating point holds none of them exactly.

### Self-hosted or local gateway

`WithBaseURL("http://127.0.0.1:8095")` works out of the box; any other plain-http host needs
`WithInsecureBaseURL(true)` (or `OBLODAI_ALLOW_INSECURE=1`). A path prefix in the base URL is kept,
so `https://gw.corp/oblodai` reaches `https://gw.corp/oblodai/v1/payment` — and the signature covers
the prefixed path.

## The contract snapshot

`contract/` is exported by the gateway's own test suite: the route registry, request DTO schemas with
English field docs, every vocabulary and error code, signing vectors, golden response bodies recorded
from a live core, and real signed webhook deliveries. `contract_routes.go`, `contract_enums.go`,
`contract_requests.go` and `contract_version.go` are generated from it and are never edited by hand.

```bash
go generate ./...                  # regenerate after refreshing contract/
go run ./internal/codegen -check   # CI gate: fails when the generated files drift
go test ./...                      # unit + contract tiers (hermetic)
OBLODAI_LIVE_URL=http://127.0.0.1:8095 go test -run TestLive ./...   # against a real core
```

The contract tier is a completeness gate, not a sample: every one of the 107 routes must have a
method wired to the right path, auth gate and idempotency behaviour, and every recorded response body
must decode into a model whose fields match the wire key for key.

## Development

```bash
gofmt -l .                         # formatting gate
go vet ./... && staticcheck ./...  # static analysis
go test ./...                      # everything
```

License: MIT.
