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
  `WithAdminToken`, `WithInsecureBaseURL`. The environment still fills in what you leave out.

## Names

- Request bodies are generated per route: `PaymentParams`, `PayoutParams`, `PayoutLinkParams`,
  `PaymentInfoParams`, … Their fields carry the gateway's own documentation.
- Vocabularies are typed string constants: `oblodai.NetworkTron`, `oblodai.PaymentStatusPaid`,
  `oblodai.PayoutStatusConfirmed`, `oblodai.FeeBearerMerchant`.
- Lookups carry both identifiers: `PaymentInfoParams{UUID: id}` or `PaymentInfoParams{OrderID: ref}`.

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
