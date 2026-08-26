package oblodai

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The live sweep touches every namespace against a REAL core. The point is not the business
// outcome but that the bodies this client sends are accepted (no 400 from our own shapes) and the
// bodies that come back decode (no sdk.bad_envelope). Routes that need a subsystem the stand may
// lack (documents, email, static wallets on a dev store) are probed and tolerated when the core
// says they are disabled.

// tolerate accepts a business refusal (403, 404, 409, a disabled feature) but fails the test on
// the two failures that would be OUR bug: a body the core rejects as malformed, and an answer this
// client cannot decode. Wrap the call in discard to drop the result it does not need.
func tolerate(t *testing.T, what string, err error) {
	t.Helper()
	switch {
	case err == nil:
	case IsValidation(err):
		t.Errorf("%s: the core rejected the body this SDK built: %v", what, err)
	case IsContract(err):
		t.Errorf("%s: the answer did not decode: %v", what, err)
	default:
		t.Logf("%s: refused by the core (tolerated): %v", what, err)
	}
}

func TestLiveSweep(t *testing.T) {
	base := liveURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, _ := onboardSandbox(t, base)
	anonymous, err := New(WithBaseURL(base), WithInsecureBaseURL(true))
	if err != nil {
		t.Fatal(err)
	}
	stamp := func(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }
	hook := liveHookURL()

	if _, err := client.Sandbox.Faucet(ctx, SandboxFaucetParams{Asset: "USDT", Amount: "1000"}); err != nil {
		t.Fatalf("Sandbox.Faucet: %v", err)
	}
	// A per-invoice url_callback needs a registered endpoint: the delivery is signed with its secret.
	if _, err := client.Webhooks.Register(ctx, hook); err != nil {
		t.Fatalf("Webhooks.Register: %v", err)
	}
	invoice, err := client.Payments.Create(ctx, PaymentParams{
		Amount: "25", Currency: "USDT", Network: NetworkTron, OrderID: stamp("sw"),
		PayerEmail: "buyer@example.com", URLCallback: hook,
	})
	if err != nil {
		t.Fatalf("Payments.Create: %v", err)
	}

	documentsEnabled := true
	if _, err := client.Documents.BalanceCertificate(ctx, DocumentQuery{}); err != nil {
		if IsNotFound(err) && IsCode(err, "document.disabled") {
			documentsEnabled = false
		}
	}

	t.Run("catalog and account", func(t *testing.T) {
		catalog, err := anonymous.Catalog.Currencies(ctx)
		if err != nil || len(catalog.Currencies) == 0 {
			t.Fatalf("Catalog.Currencies: %v", err)
		}
		if _, err := anonymous.Catalog.ExchangeRates(ctx, ExchangeRateListParams{CurrencyFrom: "BTC"}).Page(); err != nil {
			t.Errorf("Catalog.ExchangeRates: %v", err)
		}
		if _, err := client.Account.Balance(ctx); err != nil {
			t.Errorf("Account.Balance: %v", err)
		}
		if _, err := client.Account.Referral(ctx); err != nil {
			t.Errorf("Account.Referral: %v", err)
		}
		if _, err := client.Account.VRCS(ctx); err != nil {
			t.Errorf("Account.VRCS: %v", err)
		}
		if _, err := client.Account.SetVRCS(ctx, false); err != nil {
			t.Errorf("Account.SetVRCS: %v", err)
		}
	})

	t.Run("payments", func(t *testing.T) {
		if _, err := client.Payments.Get(ctx, PaymentInfoParams{UUID: invoice.UUID}); err != nil {
			t.Errorf("Payments.Get: %v", err)
		}
		if _, err := client.Payments.QR(ctx, PaymentQRParams{UUID: invoice.UUID}); err != nil {
			t.Errorf("Payments.QR: %v", err)
		}
		limit := 5
		services, err := client.Payments.Services(ctx, PaymentServicesParams{Limit: &limit}).Page()
		if err != nil || len(services.Items) == 0 {
			t.Errorf("Payments.Services: %v", err)
		}
		if _, err := anonymous.Payments.PublicView(ctx, invoice.UUID); err != nil {
			t.Errorf("Payments.PublicView: %v", err)
		}
		if _, err := anonymous.Payments.PublicQR(ctx, invoice.UUID); err != nil {
			t.Errorf("Payments.PublicQR: %v", err)
		}
		multi, err := client.Payments.Create(ctx, PaymentParams{Amount: "10", Currency: "USDT", OrderID: stamp("sw-multi")})
		if err != nil {
			t.Fatalf("Payments.Create (multi-network): %v", err)
		}
		selected, err := anonymous.Payments.Select(ctx, multi.UUID, PaySelectParams{Currency: "USDT", Network: NetworkTron})
		if err != nil {
			t.Errorf("Payments.Select: %v", err)
		} else if selected.Network != NetworkTron {
			t.Errorf("Select settled on %q", selected.Network)
		}
		tolerate(t, "Payments.Resend", discard(client.Payments.Resend(ctx, PaymentResendParams{UUID: invoice.UUID})))
		tolerate(t, "Payments.SendEmail", discard(client.Payments.SendEmail(ctx, PaymentSendEmailParams{UUID: invoice.UUID})))

		batch, err := client.Payments.Batch(ctx, PaymentBatchParams{
			OnError: BatchOnErrorContinue,
			Payments: []PaymentBatchParamsPaymentItem{{
				Amount: "3", Currency: "USDT", Network: NetworkTron, OrderID: stamp("sw-b"),
			}},
		})
		if err != nil {
			t.Fatalf("Payments.Batch: %v", err)
		}
		info, err := client.Batches.Info(ctx, BatchInfoParams{BatchID: batch.BatchID})
		if err != nil || info.BatchID != batch.BatchID {
			t.Errorf("Batches.Info: %v", err)
		}
		toCancel, err := client.Payments.Create(ctx, PaymentParams{
			Amount: "1", Currency: "USDT", Network: NetworkTron, OrderID: stamp("sw-c"),
		})
		if err != nil {
			t.Fatalf("Payments.Create: %v", err)
		}
		cancelled, err := client.Payments.Cancel(ctx, PaymentCancelParams{UUID: toCancel.UUID})
		if err != nil || cancelled.Status != PaymentStatusCancelled {
			t.Errorf("Payments.Cancel: %v (%v)", err, cancelled)
		}
		pageSize := 2
		pager := client.Payments.History(ctx, PaymentHistoryParams{Limit: &pageSize}).Pager()
		walked := 0
		for pager.Next() && walked < 6 {
			if pager.Item().UUID == "" {
				t.Error("an invoice came back without a uuid")
			}
			walked++
		}
		if err := pager.Err(); err != nil {
			t.Errorf("walking the history: %v", err)
		}
	})

	t.Run("deposit, refund and resolve", func(t *testing.T) {
		confirmations := 20
		if _, err := client.Sandbox.Deposit(ctx, SandboxDepositParams{
			InvoiceID: invoice.UUID, Amount: "25", Confirmations: &confirmations, TxID: stamp("sw-tx"),
		}); err != nil {
			t.Fatalf("Sandbox.Deposit: %v", err)
		}
		paid, err := client.Payments.Get(ctx, PaymentInfoParams{UUID: invoice.UUID})
		if err != nil {
			t.Fatalf("Payments.Get: %v", err)
		}
		if paid.Status != PaymentStatusPaid && paid.Status != PaymentStatusConfirmCheck {
			t.Errorf("the invoice is %q after a full deposit", paid.Status)
		}
		tolerate(t, "Refunds.Create", discard(client.Refunds.Create(ctx, PaymentRefundParams{
			UUID: invoice.UUID, Address: liveAddress, Amount: "5", Reference: stamp("sw-r"),
		})))
		tolerate(t, "Refunds.Resolve", discard(client.Refunds.Resolve(ctx, PaymentResolveParams{
			UUID: invoice.UUID, Action: "accept",
		})))
		tolerate(t, "Refunds.Batch", discard(client.Refunds.Batch(ctx, RefundBatchParams{
			Refunds: []RefundBatchParamsRefundItem{{
				UUID: invoice.UUID, Address: liveAddress, Amount: "1", Reference: stamp("sw-rb"),
			}},
		})))
	})

	t.Run("payouts", func(t *testing.T) {
		calculation, err := client.Payouts.Calculate(ctx, PayoutCalculateParams{Amount: "10", Currency: "USDT", Network: NetworkTron})
		if err != nil || calculation.Currency != "USDT" {
			t.Fatalf("Payouts.Calculate: %v", err)
		}
		validation, err := client.Payouts.Validate(ctx, PayoutValidateParams{
			Amount: "10", Currency: "USDT", Network: NetworkTron, Address: liveAddress,
		})
		if err != nil || !validation.Valid {
			t.Fatalf("Payouts.Validate: %v", err)
		}
		payout, err := client.Payouts.Create(ctx, PayoutParams{
			Amount: "10", Currency: "USDT", Network: NetworkTron, Address: liveAddress, OrderID: stamp("sw-po"),
		})
		if err != nil {
			t.Fatalf("Payouts.Create: %v", err)
		}
		if payout.OrderID != nil {
			byOrder, err := client.Payouts.Get(ctx, PayoutInfoParams{OrderID: *payout.OrderID})
			if err != nil || byOrder.UUID != payout.UUID {
				t.Errorf("Payouts.Get by order_id: %v", err)
			}
		}
		tolerate(t, "Payouts.Cancel", discard(client.Payouts.Cancel(ctx, payout.UUID)))
		tolerate(t, "Payouts.Approve", discard(client.Payouts.Approve(ctx, payout.UUID)))

		mass, err := client.Payouts.Mass(ctx, PayoutMassParams{Payouts: []PayoutMassParamsPayoutItem{{
			Amount: "1", Currency: "USDT", Network: NetworkTron, Address: liveAddress, OrderID: stamp("sw-m"),
		}}})
		if err != nil || len(mass) == 0 || mass[0].Idx != 0 {
			t.Errorf("Payouts.Mass: %v (%d elements)", err, len(mass))
		}
		batch, err := client.Payouts.Batch(ctx, PayoutBatchParams{Payouts: []PayoutBatchParamsPayoutItem{{
			Amount: "1", Currency: "USDT", Network: NetworkTron, Address: liveAddress, OrderID: stamp("sw-pb"),
		}}})
		if err != nil || batch.BatchID == "" {
			t.Errorf("Payouts.Batch: %v", err)
		}
		if _, err := client.Payouts.Services(ctx, PayoutServicesParams{}).Page(); err != nil {
			t.Errorf("Payouts.Services: %v", err)
		}
		if _, err := client.Payouts.SetFeeConfig(ctx, PayoutFeeConfigSetParams{FeeOnRecipient: true}); err != nil {
			t.Errorf("Payouts.SetFeeConfig: %v", err)
		}
		if _, err := client.Payouts.GetFeeConfig(ctx); err != nil {
			t.Errorf("Payouts.GetFeeConfig: %v", err)
		}
		if _, err := client.Payouts.SetRefundFeeConfig(ctx, PayoutRefundFeeConfigSetParams{FeeOnCustomer: true}); err != nil {
			t.Errorf("Payouts.SetRefundFeeConfig: %v", err)
		}
		if _, err := client.Payouts.GetRefundFeeConfig(ctx); err != nil {
			t.Errorf("Payouts.GetRefundFeeConfig: %v", err)
		}
		limit := 5
		if _, err := client.Payouts.History(ctx, PayoutHistoryParams{Kind: "refund", Limit: &limit}).Page(); err != nil {
			t.Errorf("Payouts.History: %v", err)
		}
	})

	var chequeToken string
	t.Run("payout links", func(t *testing.T) {
		expiry := 3600
		link, err := client.PayoutLinks.Create(ctx, PayoutLinkParams{
			Amount: "5", Currency: "USDT", Network: NetworkTron, Reference: stamp("sw-pl"),
			Title: "Bonus", ExpiresInSeconds: &expiry,
		})
		if err != nil {
			t.Fatalf("PayoutLinks.Create: %v", err)
		}
		if link.ClaimToken == "" {
			t.Fatal("a fresh payout link must return its claim token once")
		}
		chequeToken = link.ClaimToken
		if got, err := client.PayoutLinks.Get(ctx, link.LinkID); err != nil || got.Status != PayoutLinkStatusFunded {
			t.Errorf("PayoutLinks.Get: %v", err)
		}
		if page, err := client.PayoutLinks.List(ctx, PayoutLinkListParams{}).Page(); err != nil || len(page.Items) == 0 {
			t.Errorf("PayoutLinks.List: %v", err)
		}
		preview, err := anonymous.PayoutLinks.ClaimPreview(ctx, link.ClaimToken)
		if err != nil || !preview.Claimable {
			t.Errorf("PayoutLinks.ClaimPreview: %v", err)
		}
		claimed, err := anonymous.PayoutLinks.Claim(ctx, link.ClaimToken, PostClaimParams{Address: liveAddress})
		if err != nil || claimed.PayoutID == "" {
			t.Errorf("PayoutLinks.Claim: %v", err)
		}
		second, err := client.PayoutLinks.Create(ctx, PayoutLinkParams{
			Amount: "1", Currency: "USDT", Network: NetworkTron, Reference: stamp("sw-pl2"),
		})
		if err != nil {
			t.Fatalf("PayoutLinks.Create: %v", err)
		}
		if cancelled, err := client.PayoutLinks.Cancel(ctx, second.LinkID); err != nil || cancelled.Status != PayoutLinkStatusCancelled {
			t.Errorf("PayoutLinks.Cancel: %v", err)
		}
		batch, err := client.PayoutLinks.Batch(ctx, PayoutLinkBatchParams{Items: []PayoutLinkBatchParamsItemItem{{
			Amount: "1", Currency: "USDT", Network: NetworkTron, Reference: stamp("sw-plb"),
		}}})
		if err != nil || len(batch) == 0 || !batch[0].OK {
			t.Errorf("PayoutLinks.Batch: %v (%+v)", err, batch)
		}
	})

	t.Run("payment links", func(t *testing.T) {
		created, err := client.PaymentLinks.Create(ctx, PaymentLinkParams{
			Title: "Tip", AmountMode: AmountModeFixed, Currency: "USDT",
			AmountFixed: "10", PinnedNetwork: NetworkTron,
		})
		if err != nil || created.LinkID == "" {
			t.Fatalf("PaymentLinks.Create: %v", err)
		}
		if link, err := client.PaymentLinks.Get(ctx, PaymentLinkInfoParams{LinkID: created.LinkID}); err != nil || !link.Active {
			t.Errorf("PaymentLinks.Get: %v", err)
		}
		if page, err := client.PaymentLinks.List(ctx, PaymentLinkListParams{}).Page(); err != nil || len(page.Items) == 0 {
			t.Errorf("PaymentLinks.List: %v", err)
		}
		if view, err := anonymous.PaymentLinks.PublicView(ctx, created.LinkID); err != nil || view.AmountMode != AmountModeFixed {
			t.Errorf("PaymentLinks.PublicView: %v", err)
		}
		if checkout, err := anonymous.PaymentLinks.Checkout(ctx, created.LinkID, LinkCheckoutParams{
			Currency: "USDT", Network: NetworkTron,
		}); err != nil || checkout.UUID == "" {
			t.Errorf("PaymentLinks.Checkout: %v", err)
		}
		if toggled, err := client.PaymentLinks.Toggle(ctx, created.LinkID, false); err != nil || toggled.Active {
			t.Errorf("PaymentLinks.Toggle: %v", err)
		}
	})

	t.Run("splits and settings", func(t *testing.T) {
		rule, err := client.Splits.CreateRule(ctx, SplitRuleParams{
			Percent: "10", Address: liveAddress, Network: NetworkTron, Note: "partner",
		})
		if err != nil {
			t.Fatalf("Splits.CreateRule: %v", err)
		}
		rules, err := client.Splits.ListRules(ctx, SplitRuleListParams{}).All(0)
		if err != nil {
			t.Errorf("Splits.ListRules: %v", err)
		}
		found := false
		for _, r := range rules {
			found = found || r.RuleID == rule.RuleID
		}
		if !found {
			t.Error("the new split rule is missing from the list")
		}
		hold := 3600
		if config, err := client.Splits.SetConfig(ctx, SplitConfigSetParams{RefundHoldSeconds: hold}); err != nil || config.RefundHoldSeconds != hold {
			t.Errorf("Splits.SetConfig: %v", err)
		}
		if _, err := client.Splits.GetConfig(ctx); err != nil {
			t.Errorf("Splits.GetConfig: %v", err)
		}
		if optIn, err := client.Splits.SetOptIn(ctx, true); err != nil || !optIn.Enabled {
			t.Errorf("Splits.SetOptIn: %v", err)
		}
		if _, err := client.Splits.GetOptIn(ctx); err != nil {
			t.Errorf("Splits.GetOptIn: %v", err)
		}
		if deleted, err := client.Splits.DeleteRule(ctx, rule.RuleID); err != nil || !deleted.OK {
			t.Errorf("Splits.DeleteRule: %v", err)
		}

		if discount, err := client.Settings.SetDiscount(ctx, PaymentDiscountSetParams{
			Currency: "USDT", Network: NetworkTron, DiscountPercent: 2,
		}); err != nil || discount.DiscountPercent != 2 {
			t.Errorf("Settings.SetDiscount: %v", err)
		}
		if page, err := client.Settings.ListDiscounts(ctx, PaymentDiscountListParams{}).Page(); err != nil || len(page.Items) == 0 {
			t.Errorf("Settings.ListDiscounts: %v", err)
		}
		accuracy := 2
		if config, err := client.Settings.SetAccuracy(ctx, PaymentAccuracySetParams{
			Enabled: true, AccuracyPercent: &accuracy,
		}); err != nil || !config.Enabled {
			t.Errorf("Settings.SetAccuracy: %v", err)
		}
		if _, err := client.Settings.GetAccuracy(ctx); err != nil {
			t.Errorf("Settings.GetAccuracy: %v", err)
		}
		yes, no := true, false
		if config, err := client.Settings.SetAutoRefund(ctx, PaymentAutorefundSetParams{
			Overpay: yes, Underpay: no,
		}); err != nil || !config.Overpay {
			t.Errorf("Settings.SetAutoRefund: %v", err)
		}
		if _, err := client.Settings.GetAutoRefund(ctx); err != nil {
			t.Errorf("Settings.GetAutoRefund: %v", err)
		}
		if result, err := client.Settings.SetAccepted(ctx, PaymentAcceptedSetParams{
			Accepted: []PaymentAcceptedSetParamsAcceptedItem{{Currency: "USDT", Network: NetworkTron}},
		}); err != nil || !result.OK {
			t.Errorf("Settings.SetAccepted: %v", err)
		}
		if _, err := client.Settings.ListAccepted(ctx, PaymentAcceptedListParams{}).Page(); err != nil {
			t.Errorf("Settings.ListAccepted: %v", err)
		}
		if config, err := client.Settings.SetPaymentFeeConfig(ctx, PaymentFeeConfigSetParams{
			PayerPaysPercent: 50,
		}); err != nil || config.PayerPaysPercent != 50 {
			t.Errorf("Settings.SetPaymentFeeConfig: %v", err)
		}
		if _, err := client.Settings.GetPaymentFeeConfig(ctx); err != nil {
			t.Errorf("Settings.GetPaymentFeeConfig: %v", err)
		}
		if rules, err := client.Settings.SetAutoWithdraw(ctx, AutoWithdrawSetParams{
			Currency: "USDT", Network: NetworkTron, Address: liveAddress, MinAmount: "100",
		}); err != nil || len(rules) == 0 {
			t.Errorf("Settings.SetAutoWithdraw: %v", err)
		}
		if _, err := client.Settings.ListAutoWithdraw(ctx); err != nil {
			t.Errorf("Settings.ListAutoWithdraw: %v", err)
		}
		if _, err := client.Settings.DeleteAutoWithdraw(ctx, "USDT"); err != nil {
			t.Errorf("Settings.DeleteAutoWithdraw: %v", err)
		}
		const cidr = "203.0.113.0/24"
		if list, err := client.Settings.AddAPIAllowlist(ctx, cidr); err != nil || !contains(list.Items, cidr) {
			t.Errorf("Settings.AddAPIAllowlist: %v", err)
		}
		if list, err := client.Settings.ListAPIAllowlist(ctx); err != nil || !contains(list.Items, cidr) {
			t.Errorf("Settings.ListAPIAllowlist: %v", err)
		}
		if list, err := client.Settings.EnableAPIAllowlist(ctx, false); err != nil || list.Enabled {
			t.Errorf("Settings.EnableAPIAllowlist: %v", err)
		}
		if list, err := client.Settings.RemoveAPIAllowlist(ctx, cidr); err != nil || contains(list.Items, cidr) {
			t.Errorf("Settings.RemoveAPIAllowlist: %v", err)
		}
	})

	t.Run("webhooks and the sandbox inspector", func(t *testing.T) {
		endpoint, err := client.Webhooks.Register(ctx, hook)
		if err != nil || endpoint.EndpointID == "" {
			t.Fatalf("Webhooks.Register: %v", err)
		}
		rotated, err := client.Webhooks.RotateSecret(ctx)
		if err != nil || rotated.Secret == "" {
			t.Errorf("Webhooks.RotateSecret: %v", err)
		}
		limit := 5
		if _, err := client.Webhooks.Deliveries(ctx, WebhooksDeliveriesParams{Limit: &limit}).Page(); err != nil {
			t.Errorf("Webhooks.Deliveries: %v", err)
		}
		tolerate(t, "Webhooks.Test", discard(client.Webhooks.Test(ctx, WebhookKindPayment, WebhookTestParams{
			URLCallback: hook, Currency: "USDT", Network: NetworkTron, Status: string(PaymentStatusPaid),
		})))
		tolerate(t, "Webhooks.TestLegacy", discard(client.Webhooks.TestLegacy(ctx, PaymentTestingWebhookParams{
			URL: hook, Status: PaymentStatusPaid,
		})))
		inspector, err := client.Sandbox.Webhooks(ctx, SandboxWebhooksParams{Limit: &limit}).Page()
		if err != nil {
			t.Fatalf("Sandbox.Webhooks: %v", err)
		}
		for _, delivery := range inspector.Items {
			if delivery.Status == DeliveryStatusDelivered || delivery.Status == DeliveryStatusDead {
				tolerate(t, "Sandbox.Replay", discard(client.Sandbox.Replay(ctx, delivery.ID)))
				break
			}
		}
	})

	t.Run("wallets and transfers", func(t *testing.T) {
		// A dev store has no static wallets; the point is that the request shapes are accepted.
		tolerate(t, "Wallets.Create", discard(client.Wallets.Create(ctx, WalletParams{
			Currency: "USDT", Network: NetworkTron, OrderID: stamp("sw-w"),
		})))
		tolerate(t, "Wallets.QR", discard(client.Wallets.QR(ctx, liveAddress)))
		tolerate(t, "Wallets.Block", discard(client.Wallets.Block(ctx, WalletBlockParams{Address: liveAddress})))
		// A well-formed but unknown uuid: the core must answer "not found", not "malformed".
		tolerate(t, "Wallets.RefundBlockedDeposit", discard(client.Wallets.RefundBlockedDeposit(ctx,
			WalletBlockedAddressRefundParams{UUID: randomUUID(t), Address: liveAddress})))
		tolerate(t, "Transfers.ToPersonal", discard(client.Transfers.ToPersonal(ctx, TransferToPersonalParams{
			Amount: "1", Currency: "USDT",
		})))
		tolerate(t, "Transfers.ToUser", discard(client.Transfers.ToUser(ctx, TransferToUserParams{
			ToUserID: randomUUID(t), Amount: "1", Currency: "USDT",
		})))
		tolerate(t, "Transfers.Batch", discard(client.Transfers.Batch(ctx, TransferBatchParams{
			Transfers: []TransferBatchParamsTransferItem{{
				ToUserID: randomUUID(t), Amount: "1", Currency: "USDT", OrderID: stamp("sw-tb"),
			}},
		})))
	})

	t.Run("documents", func(t *testing.T) {
		if !documentsEnabled {
			t.Skip("this stand has no document renderer")
		}
		statement, err := client.Documents.Statement(ctx, PeriodQuery{From: "2026-01-01", To: "2026-12-31", Lang: "en"})
		if err != nil {
			t.Fatalf("Documents.Statement: %v", err)
		}
		if !strings.Contains(statement.ContentType, "pdf") || len(statement.Bytes) == 0 {
			t.Errorf("statement = %s (%d bytes)", statement.ContentType, len(statement.Bytes))
		}
		if fees, err := client.Documents.FeeSchedule(ctx, DocumentQuery{}); err != nil || len(fees.Bytes) == 0 {
			t.Errorf("Documents.FeeSchedule: %v", err)
		}
		if _, err := client.Documents.Ledger(ctx, PeriodQuery{Format: "csv"}); err != nil {
			t.Errorf("Documents.Ledger: %v", err)
		}
		if chequeToken != "" {
			tolerate(t, "PayoutLinks.Cheque", discard(client.PayoutLinks.Cheque(ctx, PayoutLinkChequeParams{
				ClaimToken: chequeToken, Lang: "en",
			})))
		}
		job, err := client.Documents.CreateJob(ctx, DocumentsJobsParams{
			Kind: "statement", Format: "csv", Lang: "en", From: "2026-01-01", To: "2026-08-25",
		})
		if err != nil {
			t.Fatalf("Documents.CreateJob: %v", err)
		}
		if info, err := client.Documents.JobInfo(ctx, job.JobID); err != nil || info.JobID != job.JobID {
			t.Errorf("Documents.JobInfo: %v", err)
		}
		tolerate(t, "Documents.JobFile", discard(client.Documents.JobFile(ctx, job.JobID)))

		// A signed public document link, exactly as document_url hands it out.
		current, err := client.Payments.Get(ctx, PaymentInfoParams{UUID: invoice.UUID})
		if err != nil {
			t.Fatalf("Payments.Get: %v", err)
		}
		parsed, err := url.Parse(current.DocumentURL)
		if err != nil || parsed.Path == "" {
			t.Fatalf("document_url = %q", current.DocumentURL)
		}
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(segments) < 4 {
			t.Fatalf("document_url path = %q", parsed.Path)
		}
		exp, _ := strconv.ParseInt(parsed.Query().Get("exp"), 10, 64)
		download, err := anonymous.Documents.Download(ctx, segments[2], segments[3], DownloadQuery{
			Exp: exp, Sig: parsed.Query().Get("sig"),
		})
		if err != nil {
			t.Errorf("Documents.Download: %v", err)
		} else if !strings.Contains(download.ContentType, "pdf") {
			t.Errorf("the signed link returned %s", download.ContentType)
		}
	})

	t.Run("sandbox reset last", func(t *testing.T) {
		if _, err := client.Sandbox.Reset(ctx); err != nil {
			t.Errorf("Sandbox.Reset: %v", err)
		}
	})

	if os.Getenv("OBLODAI_LIVE_VERBOSE") != "" {
		t.Logf("swept %s", base)
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// randomUUID is a well-formed uuid for a lookup that must answer "not found" rather than
// "malformed". The generator is the idempotency-key one; only its shape matters here.
func randomUUID(t *testing.T) string {
	t.Helper()
	key, err := NewIdempotencyKey()
	if err != nil {
		t.Fatalf("NewIdempotencyKey: %v", err)
	}
	return key
}
