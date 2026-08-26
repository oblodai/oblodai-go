package oblodai

import "context"

// The coverage ledger: one call per route the core declares. contract_routes_test.go drives it —
// every route must appear here, and every entry must be a route.

// invoke calls one route through the client and discards the result; only the request matters.
type invoke func(ctx context.Context, c *Client) error

func discard[T any](_ *T, err error) error { return err }

func page[T any](l *List[T]) error { return discard(l.Page()) }

func coverage() map[string]invoke {
	yes, no := true, false
	amount := 1
	return map[string]invoke{
		"GET /v1/claim/{token}": func(ctx context.Context, c *Client) error {
			return discard(c.PayoutLinks.ClaimPreview(ctx, "tok"))
		},
		"GET /v1/currencies": func(ctx context.Context, c *Client) error {
			return discard(c.Catalog.Currencies(ctx))
		},
		"GET /v1/documents/balance": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.BalanceCertificate(ctx, DocumentQuery{}))
		},
		"GET /v1/documents/batch": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.BatchReport(ctx, "b1", FormatQuery{}))
		},
		"GET /v1/documents/fees": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.FeeSchedule(ctx, DocumentQuery{}))
		},
		"GET /v1/documents/jobs/file": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.JobFile(ctx, "j1"))
		},
		"GET /v1/documents/ledger": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.Ledger(ctx, PeriodQuery{}))
		},
		"GET /v1/documents/link": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.LinkReport(ctx, "l1", FormatQuery{}))
		},
		"GET /v1/documents/referrals": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.ReferralsReport(ctx, PeriodQuery{}))
		},
		"GET /v1/documents/split": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.SplitReport(ctx, "i1", DocumentQuery{}))
		},
		"GET /v1/documents/statement": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.Statement(ctx, PeriodQuery{From: "2026-01-01", To: "2026-02-01"}))
		},
		"GET /v1/documents/wallet/statement": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.WalletStatement(ctx, "w1", PeriodQuery{}))
		},
		"GET /v1/documents/{kind}/{id}": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.Download(ctx, "invoice", "i1", DownloadQuery{Exp: 1, Sig: "s"}))
		},
		"GET /v1/link/{id}": func(ctx context.Context, c *Client) error {
			return discard(c.PaymentLinks.PublicView(ctx, "l1"))
		},
		"GET /v1/pay/{id}": func(ctx context.Context, c *Client) error {
			return discard(c.Payments.PublicView(ctx, "i1"))
		},
		"GET /v1/pay/{id}/qr": func(ctx context.Context, c *Client) error {
			return discard(c.Payments.PublicQR(ctx, "i1"))
		},
		"GET /v1/sandbox/webhooks": func(ctx context.Context, c *Client) error {
			return page(c.Sandbox.Webhooks(ctx, SandboxWebhooksParams{}))
		},
		"POST /v1/api-allowlist/add": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.AddAPIAllowlist(ctx, "10.0.0.0/8"))
		},
		"POST /v1/api-allowlist/enable": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.EnableAPIAllowlist(ctx, true))
		},
		"POST /v1/api-allowlist/list": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.ListAPIAllowlist(ctx))
		},
		"POST /v1/api-allowlist/remove": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.RemoveAPIAllowlist(ctx, "10.0.0.0/8"))
		},
		"POST /v1/auto-withdraw/delete": func(ctx context.Context, c *Client) error {
			_, err := c.Settings.DeleteAutoWithdraw(ctx, "USDT")
			return err
		},
		"POST /v1/auto-withdraw/list": func(ctx context.Context, c *Client) error {
			_, err := c.Settings.ListAutoWithdraw(ctx)
			return err
		},
		"POST /v1/auto-withdraw/set": func(ctx context.Context, c *Client) error {
			_, err := c.Settings.SetAutoWithdraw(ctx, AutoWithdrawSetParams{Currency: "USDT", Network: NetworkTron, Address: "T"})
			return err
		},
		"POST /v1/balance": func(ctx context.Context, c *Client) error {
			return discard(c.Account.Balance(ctx))
		},
		"POST /v1/batch/info": func(ctx context.Context, c *Client) error {
			return discard(c.Batches.Info(ctx, BatchInfoParams{BatchID: "b1"}))
		},
		"POST /v1/claim/{token}": func(ctx context.Context, c *Client) error {
			return discard(c.PayoutLinks.Claim(ctx, "tok", PostClaimParams{Address: "T"}))
		},
		"POST /v1/exchange-rate/list": func(ctx context.Context, c *Client) error {
			return page(c.Catalog.ExchangeRates(ctx, ExchangeRateListParams{}))
		},
		"POST /v1/link/{id}/checkout": func(ctx context.Context, c *Client) error {
			return discard(c.PaymentLinks.Checkout(ctx, "l1", LinkCheckoutParams{}))
		},
		"POST /v1/pay/{id}/select": func(ctx context.Context, c *Client) error {
			return discard(c.Payments.Select(ctx, "i1", PaySelectParams{Currency: "USDT", Network: NetworkTron}))
		},
		"POST /v1/payment": func(ctx context.Context, c *Client) error {
			return discard(c.Payments.Create(ctx, PaymentParams{Amount: "1", Currency: "USDT"}))
		},
		"POST /v1/payment/accepted/list": func(ctx context.Context, c *Client) error {
			return page(c.Settings.ListAccepted(ctx, PaymentAcceptedListParams{}))
		},
		"POST /v1/payment/accepted/set": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.SetAccepted(ctx, PaymentAcceptedSetParams{Accepted: []PaymentAcceptedSetParamsAcceptedItem{}}))
		},
		"POST /v1/payment/accuracy/get": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.GetAccuracy(ctx))
		},
		"POST /v1/payment/accuracy/set": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.SetAccuracy(ctx, PaymentAccuracySetParams{Enabled: true}))
		},
		"POST /v1/payment/autorefund/get": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.GetAutoRefund(ctx))
		},
		"POST /v1/payment/autorefund/set": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.SetAutoRefund(ctx, PaymentAutorefundSetParams{Overpay: yes, Underpay: no}))
		},
		"POST /v1/payment/batch": func(ctx context.Context, c *Client) error {
			return discard(c.Payments.Batch(ctx, PaymentBatchParams{Payments: []PaymentBatchParamsPaymentItem{}}))
		},
		"POST /v1/payment/cancel": func(ctx context.Context, c *Client) error {
			return discard(c.Payments.Cancel(ctx, PaymentCancelParams{UUID: "i1"}))
		},
		"POST /v1/payment/discount/list": func(ctx context.Context, c *Client) error {
			return page(c.Settings.ListDiscounts(ctx, PaymentDiscountListParams{}))
		},
		"POST /v1/payment/discount/set": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.SetDiscount(ctx, PaymentDiscountSetParams{DiscountPercent: 1}))
		},
		"POST /v1/payment/fee-config/get": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.GetPaymentFeeConfig(ctx))
		},
		"POST /v1/payment/fee-config/set": func(ctx context.Context, c *Client) error {
			return discard(c.Settings.SetPaymentFeeConfig(ctx, PaymentFeeConfigSetParams{PayerPaysPercent: 50}))
		},
		"POST /v1/payment/history": func(ctx context.Context, c *Client) error {
			return page(c.Payments.History(ctx, PaymentHistoryParams{}))
		},
		"POST /v1/payment/info": func(ctx context.Context, c *Client) error {
			return discard(c.Payments.Info(ctx, PaymentInfoParams{UUID: "i1"}))
		},
		"POST /v1/payment/link": func(ctx context.Context, c *Client) error {
			return discard(c.PaymentLinks.Create(ctx, PaymentLinkParams{AmountMode: AmountModeOpen, Currency: "USDT"}))
		},
		"POST /v1/payment/link/info": func(ctx context.Context, c *Client) error {
			return discard(c.PaymentLinks.Info(ctx, PaymentLinkInfoParams{LinkID: "l1"}))
		},
		"POST /v1/payment/link/list": func(ctx context.Context, c *Client) error {
			return page(c.PaymentLinks.List(ctx, PaymentLinkListParams{}))
		},
		"POST /v1/payment/link/toggle": func(ctx context.Context, c *Client) error {
			return discard(c.PaymentLinks.Toggle(ctx, "l1", false))
		},
		"POST /v1/payment/qr": func(ctx context.Context, c *Client) error {
			return discard(c.Payments.QR(ctx, PaymentQRParams{UUID: "i1"}))
		},
		"POST /v1/payment/refund": func(ctx context.Context, c *Client) error {
			return discard(c.Refunds.Create(ctx, PaymentRefundParams{UUID: "i1"}))
		},
		"POST /v1/payment/resend": func(ctx context.Context, c *Client) error {
			return discard(c.Payments.Resend(ctx, PaymentResendParams{UUID: "i1"}))
		},
		"POST /v1/payment/resolve": func(ctx context.Context, c *Client) error {
			return discard(c.Refunds.Resolve(ctx, PaymentResolveParams{UUID: "i1", Action: "accept"}))
		},
		"POST /v1/payment/send-email": func(ctx context.Context, c *Client) error {
			return discard(c.Payments.SendEmail(ctx, PaymentSendEmailParams{UUID: "i1"}))
		},
		"POST /v1/payment/services": func(ctx context.Context, c *Client) error {
			return page(c.Payments.Services(ctx, PaymentServicesParams{}))
		},
		"POST /v1/payment/testing-webhook": func(ctx context.Context, c *Client) error {
			return discard(c.Webhooks.TestLegacy(ctx, PaymentTestingWebhookParams{URL: "https://x"}))
		},
		"POST /v1/payout": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.Create(ctx, PayoutParams{Amount: "1", Currency: "USDT", Address: "T", OrderID: "o"}))
		},
		"POST /v1/payout/approve": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.Approve(ctx, "p1"))
		},
		"POST /v1/payout/batch": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.Batch(ctx, PayoutBatchParams{Payouts: []PayoutBatchParamsPayoutItem{}}))
		},
		"POST /v1/payout/calculate": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.Calculate(ctx, PayoutCalculateParams{Amount: "1", Currency: "USDT"}))
		},
		"POST /v1/payout/cancel": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.Cancel(ctx, "p1"))
		},
		"POST /v1/payout/fee-config/get": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.GetFeeConfig(ctx))
		},
		"POST /v1/payout/fee-config/set": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.SetFeeConfig(ctx, PayoutFeeConfigSetParams{FeeOnRecipient: true}))
		},
		"POST /v1/payout/history": func(ctx context.Context, c *Client) error {
			return page(c.Payouts.History(ctx, PayoutHistoryParams{}))
		},
		"POST /v1/payout/info": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.Info(ctx, PayoutInfoParams{UUID: "p1"}))
		},
		"POST /v1/payout/link": func(ctx context.Context, c *Client) error {
			return discard(c.PayoutLinks.Create(ctx, PayoutLinkParams{Amount: "1", Currency: "USDT", Network: NetworkTron}))
		},
		"POST /v1/payout/link/batch": func(ctx context.Context, c *Client) error {
			_, err := c.PayoutLinks.Batch(ctx, PayoutLinkBatchParams{Items: []PayoutLinkBatchParamsItemItem{}})
			return err
		},
		"POST /v1/payout/link/cancel": func(ctx context.Context, c *Client) error {
			return discard(c.PayoutLinks.Cancel(ctx, "l1"))
		},
		"POST /v1/payout/link/cheque": func(ctx context.Context, c *Client) error {
			return discard(c.PayoutLinks.Cheque(ctx, PayoutLinkChequeParams{ClaimToken: "t"}))
		},
		"POST /v1/payout/link/info": func(ctx context.Context, c *Client) error {
			return discard(c.PayoutLinks.Info(ctx, "l1"))
		},
		"POST /v1/payout/link/list": func(ctx context.Context, c *Client) error {
			return page(c.PayoutLinks.List(ctx, PayoutLinkListParams{}))
		},
		"POST /v1/payout/mass": func(ctx context.Context, c *Client) error {
			_, err := c.Payouts.Mass(ctx, PayoutMassParams{Payouts: []PayoutMassParamsPayoutItem{}})
			return err
		},
		"POST /v1/payout/refund-fee-config/get": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.GetRefundFeeConfig(ctx))
		},
		"POST /v1/payout/refund-fee-config/set": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.SetRefundFeeConfig(ctx, PayoutRefundFeeConfigSetParams{FeeOnCustomer: true}))
		},
		"POST /v1/payout/services": func(ctx context.Context, c *Client) error {
			return page(c.Payouts.Services(ctx, PayoutServicesParams{}))
		},
		"POST /v1/payout/validate": func(ctx context.Context, c *Client) error {
			return discard(c.Payouts.Validate(ctx, PayoutValidateParams{Amount: "1", Currency: "USDT", Address: "T"}))
		},
		"POST /v1/referral/info": func(ctx context.Context, c *Client) error {
			return discard(c.Account.Referral(ctx))
		},
		"POST /v1/refund/batch": func(ctx context.Context, c *Client) error {
			return discard(c.Refunds.Batch(ctx, RefundBatchParams{Refunds: []RefundBatchParamsRefundItem{}}))
		},
		"POST /v1/sandbox/deposit": func(ctx context.Context, c *Client) error {
			return discard(c.Sandbox.Deposit(ctx, SandboxDepositParams{InvoiceID: "i1"}))
		},
		"POST /v1/sandbox/faucet": func(ctx context.Context, c *Client) error {
			return discard(c.Sandbox.Faucet(ctx, SandboxFaucetParams{Asset: "USDT", Amount: "1"}))
		},
		"POST /v1/sandbox/reset": func(ctx context.Context, c *Client) error {
			return discard(c.Sandbox.Reset(ctx))
		},
		"POST /v1/sandbox/webhooks/replay": func(ctx context.Context, c *Client) error {
			return discard(c.Sandbox.Replay(ctx, "d1"))
		},
		"POST /v1/split/config/get": func(ctx context.Context, c *Client) error {
			return discard(c.Splits.GetConfig(ctx))
		},
		"POST /v1/split/config/set": func(ctx context.Context, c *Client) error {
			return discard(c.Splits.SetConfig(ctx, SplitConfigSetParams{RefundHoldSeconds: amount}))
		},
		"POST /v1/split/recipient/optin": func(ctx context.Context, c *Client) error {
			return discard(c.Splits.SetOptIn(ctx, true))
		},
		"POST /v1/split/recipient/optin/get": func(ctx context.Context, c *Client) error {
			return discard(c.Splits.GetOptIn(ctx))
		},
		"POST /v1/split/rule": func(ctx context.Context, c *Client) error {
			return discard(c.Splits.CreateRule(ctx, SplitRuleParams{Percent: "10"}))
		},
		"POST /v1/split/rule/delete": func(ctx context.Context, c *Client) error {
			return discard(c.Splits.DeleteRule(ctx, "r1"))
		},
		"POST /v1/split/rule/list": func(ctx context.Context, c *Client) error {
			return page(c.Splits.ListRules(ctx, SplitRuleListParams{}))
		},
		"POST /v1/test-webhook/payment": func(ctx context.Context, c *Client) error {
			return discard(c.Webhooks.Test(ctx, WebhookKindPayment, WebhookTestParams{URLCallback: "https://x"}))
		},
		"POST /v1/test-webhook/payout": func(ctx context.Context, c *Client) error {
			return discard(c.Webhooks.Test(ctx, WebhookKindPayout, WebhookTestParams{URLCallback: "https://x"}))
		},
		"POST /v1/test-webhook/wallet": func(ctx context.Context, c *Client) error {
			return discard(c.Webhooks.Test(ctx, WebhookKindWallet, WebhookTestParams{URLCallback: "https://x"}))
		},
		"POST /v1/transfer/batch": func(ctx context.Context, c *Client) error {
			return discard(c.Transfers.Batch(ctx, TransferBatchParams{}))
		},
		"POST /v1/transfer/to-personal": func(ctx context.Context, c *Client) error {
			return discard(c.Transfers.ToPersonal(ctx, TransferToPersonalParams{Amount: "1", Currency: "USDT"}))
		},
		"POST /v1/transfer/to-user": func(ctx context.Context, c *Client) error {
			return discard(c.Transfers.ToUser(ctx, TransferToUserParams{ToUserID: "u", Amount: "1", Currency: "USDT"}))
		},
		"POST /v1/vrcs": func(ctx context.Context, c *Client) error {
			return discard(c.Account.VRCS(ctx))
		},
		"POST /v1/wallet": func(ctx context.Context, c *Client) error {
			return discard(c.Wallets.Create(ctx, WalletParams{Currency: "USDT", Network: NetworkTron}))
		},
		"POST /v1/wallet/block": func(ctx context.Context, c *Client) error {
			return discard(c.Wallets.Block(ctx, WalletBlockParams{Address: "T"}))
		},
		"POST /v1/wallet/blocked-address-refund": func(ctx context.Context, c *Client) error {
			return discard(c.Wallets.RefundBlockedDeposit(ctx, WalletBlockedAddressRefundParams{UUID: "w1", Address: "T"}))
		},
		"POST /v1/wallet/qr": func(ctx context.Context, c *Client) error {
			return discard(c.Wallets.QR(ctx, "T"))
		},
		"POST /v1/webhooks": func(ctx context.Context, c *Client) error {
			return discard(c.Webhooks.Register(ctx, "https://x"))
		},
		"POST /v1/webhooks/deliveries": func(ctx context.Context, c *Client) error {
			return page(c.Webhooks.Deliveries(ctx, WebhooksDeliveriesParams{}))
		},
		"POST /v1/webhooks/rotate-secret": func(ctx context.Context, c *Client) error {
			return discard(c.Webhooks.RotateSecret(ctx))
		},
		"POST /v1/documents/jobs": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.CreateJob(ctx, DocumentsJobsParams{Kind: "statement"}))
		},
		"POST /v1/documents/jobs/info": func(ctx context.Context, c *Client) error {
			return discard(c.Documents.JobInfo(ctx, "j1"))
		},
		"POST /v1/merchants": func(ctx context.Context, c *Client) error {
			return discard(c.Merchants.Create(ctx, MerchantsParams{Email: "a@b.c", Name: "A"}))
		},
		"POST /v1/merchants/{id}/sandbox": func(ctx context.Context, c *Client) error {
			return discard(c.Merchants.CreateSandbox(ctx, "m1"))
		},
	}
}
