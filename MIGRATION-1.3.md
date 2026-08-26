# Migrating to 1.3

1.3 is a rewrite. The 1.x client signed a four-field canonical string and gets `401
merchant.bad_signature` on every call against the current gateway, so there is no gradual path:
swap the client, then fix the compile errors the new names produce.

```bash
go get github.com/oblodai/oblodai-go@v1.3.0
```

## The shape of the client

```go
// 1.x
client := oblodai.NewClient(oblodai.Config{PublicID: id, Secret: secret})
invoice, err := client.Payments.Create(oblodai.CreatePaymentRequest{...})

// 1.3
client, err := oblodai.New(oblodai.WithCredentials(id, secret))
invoice, err := client.Payments.Create(ctx, oblodai.PaymentParams{...})
```

- Every call takes a `context.Context` first and optional `RequestOption`s last.
- Construction returns an error instead of panicking on bad configuration.
- Options are functional: `WithCredentials`, `WithPayoutCredentials`, `WithBaseURL`,
  `WithHTTPClient`, `WithTimeout`, `WithCallBudget`, `WithRetry`, `WithLogger`, `WithHeader`,
  `WithAdminToken`, `WithInsecureBaseURL`. The environment still fills in what you leave out:
  `OBLODAI_PUBLIC_ID`, `OBLODAI_SECRET`, `OBLODAI_PAYOUT_PUBLIC_ID`, `OBLODAI_PAYOUT_SECRET`,
  `OBLODAI_BASE_URL`, `OBLODAI_ADMIN_TOKEN`, `OBLODAI_ALLOW_INSECURE`, `OBLODAI_LOG`.
- Per-call options: `WithIdempotencyKey`, `WithRequestTimeout`, `WithRequestBudget`,
  `WithRequestHeader`, `WithPayoutKey`.

## Names

- Request bodies are generated per route: `PaymentParams`, `PayoutParams`, `PayoutLinkParams`,
  `PaymentInfoParams`, … Their fields carry the gateway's own documentation.
- Vocabularies are typed string constants: `oblodai.NetworkTron`, `oblodai.PaymentStatusPaid`,
  `oblodai.PayoutStatusConfirmed`, `oblodai.FeeBearerMerchant`.
- Lookups carry both identifiers: `PaymentInfoParams{UUID: id}` or `PaymentInfoParams{OrderID: ref}`.

## Merchant provisioning

`client.Merchants.Create(ctx, oblodai.MerchantsParams{Email: …, Name: …})` and
`client.Merchants.CreateSandbox(ctx, merchantID)` provision merchants. They are unsigned; a
self-hosted gateway gates them with an admin token (`WithAdminToken`, or `OBLODAI_ADMIN_TOKEN`),
which the client sends on those two routes only — a caller header named `X-Admin-Token` is ignored.

## Errors

One type, `*oblodai.Error`, recovered with `errors.As` (or the `Is*` predicates). Branch on `Code`,
never on the message. `Retryable` is the gateway's own classification — the client has already
retried whatever was safe to retry.

## Lists

List methods return `*List[T]`: `Page()` for the first page, `Pager()` to walk every page one
request at a time, `All(max)` to collect. Nothing is requested until you consume it.

## Webhooks

Verification moved to `github.com/oblodai/oblodai-go/webhooks` and needs no client:

```go
delivery, err := webhooks.VerifyRequest(r, webhooks.Options{Secret: secret})
switch event := delivery.Event.(type) {
case *oblodai.PaymentEvent:
	…
}
```

Verify over the raw request bytes, deduplicate on `delivery.ID`, and drop out-of-order events with
`webhooks.IsStale`.

- Checks run headers → HMAC → freshness → body, so a forged delivery with a stale timestamp reports
  the signature failure rather than the timestamp.
- `delivery.IsTest` (and `webhooks.IsTestEvent`) marks a rehearsal delivery — `Webhooks.Test` and
  sandbox deliveries are signed exactly like live ones and carry `test: true` in the signed body.
  Never treat one as money.
- A delivery that verified but cannot be read is `webhook.bad_payload` (`oblodai.IsWebhookPayload`),
  a **contract** error rather than a signature error: answer 5xx so the core retries it, and keep
  4xx for `oblodai.IsSignature`.
- An event type this release does not model arrives as `*oblodai.UnknownEvent` with its raw `type`
  instead of an error. Narrow with `webhooks.IsKnownEvent(event)` before switching on the concrete
  type.
- `Options.Secret` must be non-empty and `Options.Tolerance` must not be negative — both are
  `ConfigError`s raised before any crypto. Zero tolerance means the 5-minute default; disable
  freshness with `SkipTimestampCheck`.

## What else changed in 1.3

- **Retry safety comes from the contract.** `Routes[key].Safe` is the core's own read-only
  classification; the SDK no longer infers it from the path, and `go run ./internal/codegen -check`
  fails on a contract snapshot that does not declare it.
- **Secrets do not print.** `WebhookEndpoint.Secret`, `WebhookSecretRotated.Secret`,
  `APIKeyPair.Secret`, `PayoutLink.ClaimToken`, `PayoutLink.ClaimURL` (it embeds the token) and
  `PayoutLink.Passcode` read normally as fields but render as `[redacted]` in `fmt` and in
  `json.Marshal`. If you persisted one of these models by serializing the struct, read the field
  instead. Log fields are redacted inside the client, before
  they reach a logger installed with `WithLogger`.
- **`NewIdempotencyKey` returns `(string, error)`** instead of panicking when the platform CSPRNG is
  unavailable.
- **An idempotency key on a list method is refused** (`sdk.idempotency_unsupported`) rather than
  dropped.
- **`ResolutionAccepted` is gone.** `Resolution` already describes both shapes of
  `POST /v1/payment/resolve`: `Resolution == "accepted"` fills `PaymentUUID`/`AmountKept`,
  `"refunded"` fills the embedded `Payout`.
- **Money helpers refuse malformed amounts** with `sdk.bad_amount`, including a trailing dot: `"5."`
  used to add up as `"5"`.
- **`Documents.Download` omits `exp` when it is zero**, instead of sending `exp=0`.
- **Response bodies are capped** (8 MiB JSON, 64 MiB documents) with `sdk.response_too_large`.
- **`Error.LastCode`** carries what the API last said when a call gives up on
  `transport.deadline`.
