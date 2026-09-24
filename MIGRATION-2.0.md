# Migrating to oblodai-go 2.0

2.0 is a breaking release. The API surface is now generated from the gateway's OpenAPI contract,
so every operation has a method and every method is named the same way in all eight Oblodai SDKs;
the hand-written runtime keeps what 1.x did well — signing, safe retries, one idempotency key per
call, clock-skew correction, webhook verification — and adds per-call options, raw responses,
hooks, iterators and waiters for long-running jobs.

## 1. Module path and Go version

```bash
go get github.com/oblodai/oblodai-go/v2@v2.0.0
```

Imports gain `/v2`: `github.com/oblodai/oblodai-go/v2` and `github.com/oblodai/oblodai-go/v2/webhooks`.
Go ≥ 1.25 (1.x needed 1.22).

## 2. Call shape

A method takes the context, its parameters as a pointer to the generated request model (nil when
there are none to send), then call options. Optional fields are pointers — `oblodai.Ptr(v)` makes
one; amounts are `oblodai.Decimal`, a string type.

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

| 1.x | 2.0 |
| --- | --- |
| `oblodai.PaymentParams{…}` and the other hand-written `…Params` | the generated request model, by pointer: `&oblodai.PaymentRequest{…}` (see `go doc` of the method) |
| response types `Payment`, `Payout`, `PayoutLink`, … | the generated models: `PaymentView`, `PaymentInfoResult`, `PayoutItem`, `PayoutLinkView`, … |
| `oblodai.Money` (alias of `string`) | `oblodai.Decimal` (a string type; a JSON number in an amount is `sdk.float_amount`); fields that may be empty stay `string` |
| `oblodai.NetworkTron` and other vocabulary constants of the snapshot | the contract's enumerations (`PaymentStatusPaid`, `PayoutKindRefund`, …); free-form fields such as `network` are strings |
| `oblodai.ErrorCodes` | the generated `ErrorCode` enumeration |
| `Routes["POST /v1/payment"]` of type `Route` | `Routes["createPayment"]` of type `RouteSpec`, keyed by `operationId`; `RouteSpec.Key()` gives the request line |
| `ContractHash`, `ContractCoreCommit`, `ContractExportedAt`, `contract/` | gone: the source is the backend's `openapi.json`; `names.lock` pins the public names |
| `Merchants.Create` | gone — merchant provisioning is not part of the merchant API contract; `Sandbox.OnboardStore` onboards a sandbox store |

## 3. Options

| 1.x | 2.0 |
| --- | --- |
| `WithIdempotencyKey`, `WithRequestTimeout`, `WithRequestBudget`, `WithRequestHeader` | unchanged |
| — | `WithMaxRetries(n)`, `WithExtraHeaders(map)`, `WithRequestID(id)` (`X-Request-ID`, now sent on every call), `WithRawResponse(&raw)` |
| — | `client.WithOptions(opts...)` — a copy of the client with other options; `WithHooks(Hooks{OnRequest, OnResponse})` |
| `WithIdempotencyKey` on `Sandbox.Faucet` | fills the faucet's `idempotency_key` body field (no header) |

## 4. Lists, jobs, errors, webhooks, printing

- `List.All(max)` is `List.Collect(max)`; `List.Items()` and `List.ByPage()` are range-over-func
  iterators; `Page()` and `Pager()` are unchanged. `Page.Paginate` is the generated `Pagination`.
- Batches and document exports: `client.BatchJob(id).Wait(ctx)`, `client.DocumentJob(id).Wait(ctx)`
  and `.Download(ctx)` replace hand-written polling of `Batches.Info` / `Documents.JobInfo`.
- `err.Error()` reads `[code] message (request_id=…)` (was `oblodai: code: message (HTTP n, request
  …)`); an error without the core's request id carries the call's `X-Request-ID`.
- `webhooks.Verify`/`VerifyDelivery`/`Parse` return `*webhooks.Event` with one typed body —
  `Payment`, `Payout`, `Wallet`, `Conversion` (the generated webhook models) — instead of the
  `oblodai.WebhookEvent` interface and `PaymentEvent`/`PayoutEvent`/`WalletEvent`/`UnknownEvent`.
  `event.ID()`, `Sequence()`, `IsFinal()`, `IsTest()`, `IsKnown()` replace `ID()`, `Seq()`,
  `Final()`, `IsTest()`, `webhooks.IsKnownEvent`, `webhooks.IsTestEvent`; `Delivery.EventID`
  (`X-Webhook-Event-Id`) is the key to deduplicate on.
- Secrets still never print through `fmt`, but `json.Marshal` of a model is now faithful: store a
  secret by reading its field, and keep models holding one out of JSON logs.

## 5. Method names

Old method → new method, with the language-neutral name pinned in `names.lock`.

| 1.x | 2.0 | `names.lock` |
| --- | --- | ------------ |
| `Account.Balance` | `Account.GetBalance` | `account.get_balance` |
| `Account.Referral` | `Referrals.GetInfo` | `referrals.get_info` |
| `Account.SetVRCS` | `Settings.ConfigureVrcs` | `settings.configure_vrcs` |
| `Account.VRCS` | `Settings.ConfigureVrcs` | `settings.configure_vrcs` |
| `Batches.Info` | `Batches.GetInfo` | `batches.get_info` |
| `Catalog.Currencies` | `Checkout.ListCurrencies` | `checkout.list_currencies` |
| `Catalog.ExchangeRates` | `Account.ListExchangeRates` | `account.list_exchange_rates` |
| `Documents.BalanceCertificate` | `Documents.GetBalance` | `documents.get_balance` |
| `Documents.BatchReport` | `Documents.GetBatch` | `documents.get_batch` |
| `Documents.CreateJob` | `Documents.CreateJob` | `documents.create_job` |
| `Documents.Download` | `Documents.GetSigned` | `documents.get_signed` |
| `Documents.FeeSchedule` | `Documents.GetFees` | `documents.get_fees` |
| `Documents.JobFile` | `Documents.DownloadJobFile` | `documents.download_job_file` |
| `Documents.JobInfo` | `Documents.GetJob` | `documents.get_job` |
| `Documents.Ledger` | `Documents.GetLedger` | `documents.get_ledger` |
| `Documents.LinkReport` | `Documents.GetPaymentLink` | `documents.get_payment_link` |
| `Documents.ReferralsReport` | `Documents.GetReferrals` | `documents.get_referrals` |
| `Documents.SplitReport` | `Documents.GetSplit` | `documents.get_split` |
| `Documents.Statement` | `Documents.GetStatement` | `documents.get_statement` |
| `Documents.WalletStatement` | `Documents.GetWalletStatement` | `documents.get_wallet_statement` |
| `Merchants.CreateSandbox` | `Sandbox.OnboardStore` | `sandbox.onboard_store` |
| `PaymentLinks.Checkout` | `Checkout.PaymentLink` | `checkout.payment_link` |
| `PaymentLinks.Create` | `PaymentLinks.Create` | `payment_links.create` |
| `PaymentLinks.Get` | `PaymentLinks.Get` | `payment_links.get` |
| `PaymentLinks.Info` | `PaymentLinks.Get` | `payment_links.get` |
| `PaymentLinks.List` | `PaymentLinks.List` | `payment_links.list` |
| `PaymentLinks.PublicView` | `Checkout.GetPublicPaymentLink` | `checkout.get_public_payment_link` |
| `PaymentLinks.Toggle` | `PaymentLinks.Toggle` | `payment_links.toggle` |
| `Payments.Batch` | `Batches.CreatePayment` | `batches.create_payment` |
| `Payments.Cancel` | `Payments.Cancel` | `payments.cancel` |
| `Payments.Create` | `Payments.Create` | `payments.create` |
| `Payments.Get` | `Payments.GetInfo` | `payments.get_info` |
| `Payments.History` | `Payments.ListHistory` | `payments.list_history` |
| `Payments.Info` | `Payments.GetInfo` | `payments.get_info` |
| `Payments.List` | `Payments.ListHistory` | `payments.list_history` |
| `Payments.PublicQR` | `Checkout.GetQR` | `checkout.get_qr` |
| `Payments.PublicView` | `Checkout.Get` | `checkout.get` |
| `Payments.QR` | `Payments.GetQR` | `payments.get_qr` |
| `Payments.Resend` | `Webhooks.ResendPayment` | `webhooks.resend_payment` |
| `Payments.Select` | `Checkout.SelectMethod` | `checkout.select_method` |
| `Payments.SendEmail` | `Payments.SendEmail` | `payments.send_email` |
| `Payments.Services` | `Payments.ListServices` | `payments.list_services` |
| `PayoutLinks.Batch` | `PayoutLinks.CreateBatch` | `payout_links.create_batch` |
| `PayoutLinks.Cancel` | `PayoutLinks.Cancel` | `payout_links.cancel` |
| `PayoutLinks.Cheque` | `Documents.GetPayoutLinkCheque` | `documents.get_payout_link_cheque` |
| `PayoutLinks.ClaimPreview` | `PayoutLinks.GetPayoutClaim` | `payout_links.get_payout_claim` |
| `PayoutLinks.Claim` | `PayoutLinks.ClaimPayout` | `payout_links.claim_payout` |
| `PayoutLinks.Create` | `PayoutLinks.Create` | `payout_links.create` |
| `PayoutLinks.Get` | `PayoutLinks.Get` | `payout_links.get` |
| `PayoutLinks.Info` | `PayoutLinks.Get` | `payout_links.get` |
| `PayoutLinks.List` | `PayoutLinks.List` | `payout_links.list` |
| `Payouts.Approve` | `Payouts.Approve` | `payouts.approve` |
| `Payouts.Batch` | `Batches.CreatePayout` | `batches.create_payout` |
| `Payouts.Calculate` | `Payouts.Calculate` | `payouts.calculate` |
| `Payouts.Cancel` | `Payouts.Cancel` | `payouts.cancel` |
| `Payouts.Create` | `Payouts.Create` | `payouts.create` |
| `Payouts.GetFeeConfig` | `Settings.GetPayoutFeeConfig` | `settings.get_payout_fee_config` |
| `Payouts.GetRefundFeeConfig` | `Settings.GetRefundFeeConfig` | `settings.get_refund_fee_config` |
| `Payouts.Get` | `Payouts.GetInfo` | `payouts.get_info` |
| `Payouts.History` | `Payouts.ListHistory` | `payouts.list_history` |
| `Payouts.Info` | `Payouts.GetInfo` | `payouts.get_info` |
| `Payouts.List` | `Payouts.ListHistory` | `payouts.list_history` |
| `Payouts.Mass` | `Payouts.CreateMass` | `payouts.create_mass` |
| `Payouts.Services` | `Payouts.ListServices` | `payouts.list_services` |
| `Payouts.SetFeeConfig` | `Settings.SetPayoutFeeConfig` | `settings.set_payout_fee_config` |
| `Payouts.SetRefundFeeConfig` | `Settings.SetRefundFeeConfig` | `settings.set_refund_fee_config` |
| `Payouts.Validate` | `Payouts.Validate` | `payouts.validate` |
| `Refunds.Batch` | `Batches.CreateRefund` | `batches.create_refund` |
| `Refunds.Create` | `Refunds.Payment` | `refunds.payment` |
| `Refunds.Resolve` | `Payments.Resolve` | `payments.resolve` |
| `Sandbox.Deposit` | `Sandbox.SimulateDeposit` | `sandbox.simulate_deposit` |
| `Sandbox.Faucet` | `Sandbox.Faucet` | `sandbox.faucet` |
| `Sandbox.Replay` | `Sandbox.ReplayWebhook` | `sandbox.replay_webhook` |
| `Sandbox.Reset` | `Sandbox.Reset` | `sandbox.reset` |
| `Sandbox.Webhooks` | `Sandbox.ListWebhooks` | `sandbox.list_webhooks` |
| `Settings.AddAPIAllowlist` | `APIAllowlist.AddEntry` | `api_allowlist.add_entry` |
| `Settings.DeleteAutoWithdraw` | `Settings.DeleteAutoWithdrawRule` | `settings.delete_auto_withdraw_rule` |
| `Settings.EnableAPIAllowlist` | `APIAllowlist.SetEnabled` | `api_allowlist.set_enabled` |
| `Settings.GetAccuracy` | `Settings.GetAccuracy` | `settings.get_accuracy` |
| `Settings.GetAutoRefund` | `Settings.GetAutoRefund` | `settings.get_auto_refund` |
| `Settings.GetPaymentFeeConfig` | `Settings.GetPaymentFeeConfig` | `settings.get_payment_fee_config` |
| `Settings.ListAPIAllowlist` | `APIAllowlist.List` | `api_allowlist.list` |
| `Settings.ListAccepted` | `Settings.ListAcceptedCurrencies` | `settings.list_accepted_currencies` |
| `Settings.ListAutoWithdraw` | `Settings.ListAutoWithdrawRules` | `settings.list_auto_withdraw_rules` |
| `Settings.ListDiscounts` | `Settings.ListDiscounts` | `settings.list_discounts` |
| `Settings.RemoveAPIAllowlist` | `APIAllowlist.RemoveEntry` | `api_allowlist.remove_entry` |
| `Settings.SetAccepted` | `Settings.SetAcceptedCurrencies` | `settings.set_accepted_currencies` |
| `Settings.SetAccuracy` | `Settings.SetAccuracy` | `settings.set_accuracy` |
| `Settings.SetAutoRefund` | `Settings.SetAutoRefund` | `settings.set_auto_refund` |
| `Settings.SetAutoWithdraw` | `Settings.SetAutoWithdrawRule` | `settings.set_auto_withdraw_rule` |
| `Settings.SetDiscount` | `Settings.SetDiscount` | `settings.set_discount` |
| `Settings.SetPaymentFeeConfig` | `Settings.SetPaymentFeeConfig` | `settings.set_payment_fee_config` |
| `Splits.CreateRule` | `Splits.CreateRule` | `splits.create_rule` |
| `Splits.DeleteRule` | `Splits.DeleteRule` | `splits.delete_rule` |
| `Splits.GetConfig` | `Splits.GetConfig` | `splits.get_config` |
| `Splits.GetOptIn` | `Splits.GetRecipientOptIn` | `splits.get_recipient_opt_in` |
| `Splits.ListRules` | `Splits.ListRules` | `splits.list_rules` |
| `Splits.SetConfig` | `Splits.SetConfig` | `splits.set_config` |
| `Splits.SetOptIn` | `Splits.SetRecipientOptIn` | `splits.set_recipient_opt_in` |
| `Transfers.Batch` | `Payouts.CreateTransferBatch` | `payouts.create_transfer_batch` |
| `Transfers.ToPersonal` | `Payouts.TransferToPersonal` | `payouts.transfer_to_personal` |
| `Transfers.ToUser` | `Payouts.TransferToUser` | `payouts.transfer_to_user` |
| `Wallets.Block` | `Wallets.Block` | `wallets.block` |
| `Wallets.Create` | `Wallets.Create` | `wallets.create` |
| `Wallets.QR` | `Wallets.GetQR` | `wallets.get_qr` |
| `Wallets.RefundBlockedDeposit` | `Refunds.BlockedWallet` | `refunds.blocked_wallet` |
| `Webhooks.Deliveries` | `Webhooks.ListDeliveries` | `webhooks.list_deliveries` |
| `Webhooks.Register` | `Webhooks.Register` | `webhooks.register` |
| `Webhooks.RotateSecret` | `Webhooks.RotateSecret` | `webhooks.rotate_secret` |
| `Webhooks.Test(ctx, kind, …)` | `Webhooks.SendTestPayment` · `SendTestPayout` · `SendTestWallet` · `SendTestConversion` | `webhooks.send_test_*` |
| `Webhooks.TestLegacy` | `Webhooks.SendLegacyTest` | `webhooks.send_legacy_test` |
