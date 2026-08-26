# Oblodai Go SDK — guide for coding agents

Module `github.com/oblodai/oblodai-go` (1.3.0), Go ≥ 1.22, standard library only — no third-party
dependencies at runtime or in the tests. Everything below is verified against the gateway's contract
snapshot in `contract/contract.json`, from which `contract_routes.go`, `contract_enums.go`,
`contract_requests.go` and `contract_version.go` are generated.

## Non-negotiables

- Amounts are decimal **strings** (`oblodai.Money`, an alias of `string`): `Amount: "25"`, never a
  float. Do not compare them with `<` — `"9" < "10"` is true as text and false as money. Use
  `CompareAmounts`, `AmountsEqual`, `AddAmounts`, `SubtractAmounts`, `IsZeroAmount`; anything that is
  not `-?digits[.digits]` (≤ 64 chars) is a `ConfigError` with `sdk.bad_amount`.
- Every method takes `ctx context.Context` first and `...RequestOption` last:
  `WithIdempotencyKey`, `WithRequestTimeout`, `WithRequestBudget`, `WithRequestHeader`,
  `WithPayoutKey`.
- Two key kinds. The **payout key** is required for `Payouts.*`, `Refunds.*` (including `Resolve`),
  `PayoutLinks.*`, `Transfers.*`, `Splits.*`, `Wallets.RefundBlockedDeposit`, `Settings.*AutoWithdraw`,
  `Settings.*APIAllowlist`, `Webhooks.RotateSecret`, `Webhooks.Test(WebhookKindPayout, …)`,
  `Sandbox.Faucet`, `Sandbox.Reset`. Configure it with `WithPayoutCredentials` (or
  `OBLODAI_PAYOUT_PUBLIC_ID`/`OBLODAI_PAYOUT_SECRET`); the wrong kind is a 403
  `merchant.wrong_key_kind`. On routes that accept either kind, `WithPayoutKey()` picks the payout one.
- List methods return `*List[T]` and request **nothing** until consumed: `Page()` is the first page
  (`{Items, Paginate}`), `Pager()` walks every item one page at a time, `All(max)` collects. The
  first page is memoized and safe to ask for from several goroutines; a `Pager` belongs to one.
- Idempotency keys are generated automatically on create routes and reused across retries. Passing
  `WithIdempotencyKey` to a route the core does not deduplicate — list methods included — is refused
  with `sdk.idempotency_unsupported` instead of being dropped.
- Retry safety is `Routes[key].Safe`, the core's own read-only classification from the contract. The
  SDK never infers it from a path or a verb, and codegen fails on a snapshot that omits it.
- Secrets (`Client`, `WebhookEndpoint.Secret`, `WebhookSecretRotated.Secret`, `APIKeyPair.Secret`,
  `PayoutLink.ClaimToken`/`ClaimURL`/`Passcode`) read normally as fields and render as
  `[redacted]` in `fmt` (`%v`, `%+v`, `%#v`) and in `json.Marshal`. Log fields are redacted before they reach any logger,
  including one installed with `WithLogger`.

## Naming

| intent            | call                                                                                                        |
| ----------------- | ------------------------------------------------------------------------------------------------------------ |
| fetch one         | `.Info(ctx, XInfoParams{UUID: …})` — or `{OrderID: …}` (alias `.Get`)                                       |
| fetch many        | `.History(ctx, params)` on payments/payouts (alias `.List`), `.List(ctx, params)` elsewhere                  |
| create            | `.Create(ctx, params)`; webhooks: `Webhooks.Register(ctx, url)`                                             |
| many, synchronous | `Payouts.Mass` ≤100, `PayoutLinks.Batch` ≤500 — per-element `BatchElement{Idx, OK, Result, Message, ErrorCode}` |
| many, async       | `Payments.Batch`, `Payouts.Batch`, `Refunds.Batch`, `Transfers.Batch` — ≤5000, poll `Batches.Info`           |
| documents         | `Documents.Statement/Ledger/BalanceCertificate/FeeSchedule/…` → `*FileResult{Bytes, ContentType, Filename}`  |
| provisioning      | `Merchants.Create(ctx, MerchantsParams{Email: …})`, `Merchants.CreateSandbox(ctx, merchantID)` — unsigned; `WithAdminToken` on a self-hosted gateway |
| payer-facing      | `Payments.PublicView/Select/PublicQR`, `PaymentLinks.PublicView/Checkout`, `PayoutLinks.ClaimPreview/Claim` — no credentials |

Ids are plain strings (`PayoutLinks.Cancel(ctx, linkID)`): Go has no union types, so where the
other ports accept "the model or its id", pass the field — `link.LinkID`, `payout.UUID`.

## Errors

Every failure is `*oblodai.Error`. Recover it with `errors.As(err, &apiErr)` or `oblodai.AsError`.
Fields: `Code` (`family.reason` — branch on this), `Message`, `HTTPStatus`, `Retryable`
(authoritative: the client already retried what it should), `RetryAfter` (seconds, `[0, 86400]`),
`RequestID` (quote to support), `Field` (on 400s), `Synthetic` (the answer came from a proxy, not the
API), `LastCode` (on `transport.deadline`: what the API last said), `Kind`. `Error.Body()` returns
the raw response body; `json.Marshal(err)` keeps the identity and drops the body.

Predicates: `IsValidation` 400, `IsAuthentication` 401, `IsPermission` 403, `IsNotFound` 404,
`IsConflict`/`IsIdempotencyConflict` 409, `IsRateLimit` 429, `IsUnavailable` 503, `IsInternal` other
5xx, `IsTransport` (no response), `IsConfig` (refused before sending), `IsContract` (undecodable or
oversized answer), `IsSignature` (forged or stale webhook), `IsWebhookPayload` (authentic webhook,
unreadable body), `IsCode`, `IsKind`, `IsRetryable`.

Client-raised codes: `sdk.missing_credentials`, `sdk.bad_config`, `sdk.bad_idempotency_key`,
`sdk.idempotency_unsupported`, `sdk.bad_envelope`, `sdk.bad_path_param`, `sdk.bad_amount`,
`sdk.bad_header`, `sdk.response_too_large`, `transport.timeout|network|aborted|deadline`,
`webhook.bad_signature|stale_timestamp|missing_header|bad_payload`.

Codes worth handling: `payout.insufficient_funds` (retryable), `payout.funds_maturing` (retryable),
`idempotency.key_reused`, `invoice.not_payable`, `payment.not_found`, `merchant.wrong_key_kind`,
`merchant.bad_signature`, `request.rate_limited`. The full list is `oblodai.ErrorCodes` (471); the
money-moving methods name the ones to branch on in their own doc comments.

## Statuses

- Payment: `select → created → confirm_check → paid | paid_over | wrong_amount | expired | cancelled`.
  `IsPaymentPaid` is paid/paid_over; `wrong_amount` waits for `Refunds.Resolve` (`accept` or `refund`);
  `IsPaymentFinal` covers the rest.
- Payout: `pending → approved → awaiting_cosign → broadcasting → sent → confirmed | failed | cancelled`.
- Webhook event types: `invoice.<status>`, `payout.<status>`, `wallet.paid`; the body's `type` is
  `payment | payout | wallet` (`*PaymentEvent`, `*PayoutEvent`, `*WalletEvent`). A type from a newer
  core arrives as `*UnknownEvent` with the raw string, never an error — narrow with `IsKnownEvent`.

## Webhooks

```go
import "github.com/oblodai/oblodai-go/webhooks"

delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: endpointSecret})
```

Order of checks: headers → HMAC (current secret, then `PreviousSecret`) → freshness → body. An empty
`Secret` or a negative `Tolerance` is a `ConfigError`; `Tolerance` zero means the 5-minute default
(Go's zero value is "unset"), and `SkipTimestampCheck: true` is how freshness is disabled. Answer
4xx to `IsSignature`, 5xx to `IsWebhookPayload` (`webhook.bad_payload`: the delivery is authentic, so
the core will retry it).

Verify over the **raw** bytes. `delivery.IsTest` (or `webhooks.IsTestEvent`) marks a rehearsal
delivery (`test: true` in the signed body) — never treat one as money. Deduplicate on `delivery.ID`
(`X-Webhook-Id`); drop out-of-order events with `webhooks.IsStale(event, lastSequence)`, which is
false for an event without a sequence. During a rotation pass `PreviousSecret` for ≥26 h.

## Machine-readable surface

`Routes` and `RouteKeys` (107 routes: Method, Path, Auth, Idempotent, Safe, Bare, List), the
generated `…Params` request bodies, `ErrorCodes` (471), `Networks`, `PaymentStatuses`,
`PayoutStatuses`, `EventTypes`, `ContractCoreCommit`/`ContractExportedAt`/`ContractHash`, and
`contract/` in the repository (schemas, golden response bodies per route, error samples, 43 signed
webhook deliveries).

Environment: `OBLODAI_PUBLIC_ID`, `OBLODAI_SECRET`, `OBLODAI_PAYOUT_PUBLIC_ID`,
`OBLODAI_PAYOUT_SECRET`, `OBLODAI_BASE_URL`, `OBLODAI_ADMIN_TOKEN`, `OBLODAI_ALLOW_INSECURE`,
`OBLODAI_LOG` (`debug|info|warn|error`).

## Working in this repository

```bash
go generate ./...                  # regenerate after refreshing contract/
go run ./internal/codegen -check   # drift gate: fails when the generated files are stale
gofmt -l .                         # must print nothing
go vet ./... && staticcheck ./...
go test -race ./...                # unit + contract tiers, all hermetic
OBLODAI_LIVE_URL=http://127.0.0.1:8095 go test -run TestLive ./...   # needs a real core
```

- `contract_*.go` are generated: never edit them by hand, change `internal/codegen` and regenerate.
- `contract/descriptions.en.json` is repo-local (the English field docs) and is NOT part of the
  core's export; everything else under `contract/` is replaced wholesale on a refresh.
- Source files stay under ~400 lines; tests live next to what they test (`*_test.go` in the same
  package, so they can reach unexported helpers).
- A fix without a test that fails before it is not finished. The contract tier is a completeness
  gate: every route needs a method in `coverage()`, and every recorded body must decode into a model
  whose fields match the wire key for key.
