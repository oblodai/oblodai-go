package oblodai

// The model ledger: which model decodes which recorded body, and where inside the result it sits.
// contract_models_test.go drives it — every recorded success body must appear here.

// modelRow names a route, how to reach the object inside its result, and the model that decodes it.
type modelRow struct {
	route string
	// path walks into the result: "" is the result itself, "items.0" its first item.
	path  string
	model any
	// alsoOptional lists wire fields this particular recording may omit even though the model
	// declares them (a route that returns a narrower view of a shared object).
	alsoOptional []string
}

// autoWithdrawList is the shape of the plain {items} result the auto-withdraw routes answer with.
type autoWithdrawList struct {
	Items []AutoWithdrawRule `json:"items"`
}

func modelRows() []modelRow {
	return []modelRow{
		{route: "POST /v1/payment", model: Payment{}},
		{route: "POST /v1/payment/info", model: Payment{}},
		{route: "POST /v1/payment/cancel", model: Payment{}},
		{route: "POST /v1/payment/history", path: "items.0", model: Payment{}},
		{route: "GET /v1/pay/{id}", model: PublicPayment{}},
		{route: "POST /v1/pay/{id}/select", model: PublicPayment{}},
		{route: "POST /v1/link/{id}/checkout", model: PublicPayment{}},
		{route: "POST /v1/payment/qr", model: QRCode{}},
		{route: "GET /v1/pay/{id}/qr", model: QRCode{}},
		{route: "POST /v1/payment/services", path: "items.0", model: ServiceMethod{}},
		{route: "POST /v1/payout/services", path: "items.0", model: ServiceMethod{}},
		{route: "POST /v1/payment/batch", model: BatchSubmitted{}},
		{route: "POST /v1/payout/batch", model: BatchSubmitted{}},
		{route: "POST /v1/refund/batch", model: BatchSubmitted{}},
		{route: "POST /v1/transfer/batch", model: BatchSubmitted{}},
		{route: "POST /v1/batch/info", model: BatchInfo{}},
		{route: "POST /v1/payout", model: Payout{}},
		{route: "POST /v1/payout/info", model: Payout{}},
		{route: "POST /v1/payout/cancel", model: Payout{}},
		{route: "POST /v1/payout/history", path: "items.0", model: Payout{}},
		{route: "POST /v1/payout/mass", path: "items.0.result", model: Payout{}},
		{route: "POST /v1/payment/refund", model: Payout{}},
		{route: "POST /v1/payment/resolve", model: Resolution{}},
		{route: "POST /v1/payout/calculate", model: PayoutCalculation{}},
		{route: "POST /v1/payout/validate", model: PayoutValidation{}},
		{route: "POST /v1/payout/link", model: PayoutLink{}},
		{route: "POST /v1/payout/link/info", model: PayoutLink{}},
		{route: "POST /v1/payout/link/list", path: "items.0", model: PayoutLink{}},
		{route: "POST /v1/payout/link/cancel", model: PayoutLink{}},
		{route: "POST /v1/payout/link/batch", path: "items.0.result", model: PayoutLink{}},
		{route: "GET /v1/claim/{token}", model: ClaimPreview{}},
		{route: "POST /v1/claim/{token}", model: ClaimResult{}},
		{route: "POST /v1/payment/link", model: PaymentLinkCreated{}},
		{route: "POST /v1/payment/link/info", model: PaymentLink{}},
		{route: "POST /v1/payment/link/list", path: "items.0", model: PaymentLink{}},
		{route: "GET /v1/link/{id}", model: PublicPaymentLink{}},
		{route: "POST /v1/payment/link/toggle", model: PaymentLinkToggled{}},
		{route: "POST /v1/balance", model: Balance{}},
		{route: "POST /v1/referral/info", model: ReferralInfo{}},
		{route: "POST /v1/auto-withdraw/list", path: "items.0", model: AutoWithdrawRule{}},
		{route: "POST /v1/auto-withdraw/set", path: "items.0", model: AutoWithdrawRule{}},
		{route: "POST /v1/auto-withdraw/delete", model: autoWithdrawList{}},
		{route: "POST /v1/api-allowlist/list", model: APIAllowlist{}},
		{route: "POST /v1/api-allowlist/add", model: APIAllowlist{}},
		{route: "POST /v1/api-allowlist/remove", model: APIAllowlist{}},
		{route: "POST /v1/api-allowlist/enable", model: APIAllowlist{}},
		{route: "POST /v1/payment/discount/list", path: "items.0", model: DiscountRule{}},
		{route: "POST /v1/payment/discount/set", model: DiscountRule{}},
		{route: "POST /v1/split/rule", model: SplitRule{}},
		{route: "POST /v1/split/rule/list", path: "items.0", model: SplitRule{}},
		{route: "POST /v1/split/rule/delete", model: OkResult{}},
		{route: "POST /v1/split/config/get", model: SplitConfig{}},
		{route: "POST /v1/split/config/set", model: SplitConfig{}},
		{route: "POST /v1/split/recipient/optin", model: SplitOptIn{}},
		{route: "POST /v1/split/recipient/optin/get", model: SplitOptIn{}},
		{route: "GET /v1/currencies", model: Currencies{}},
		{route: "GET /v1/currencies", path: "currencies.0", model: CurrencyInfo{}},
		{route: "GET /v1/currencies", path: "currencies.0.networks.0", model: CurrencyNetwork{}},
		{route: "GET /v1/currencies", path: "pricing_currencies.0", model: PricingCurrency{}},
		{route: "POST /v1/exchange-rate/list", path: "items.0", model: ExchangeRate{}},
		{route: "POST /v1/webhooks", model: WebhookEndpoint{}},
		{route: "POST /v1/webhooks/rotate-secret", model: WebhookSecretRotated{}},
		{route: "POST /v1/webhooks/deliveries", path: "items.0", model: WebhookDelivery{}},
		{route: "GET /v1/sandbox/webhooks", path: "items.0", model: WebhookDelivery{}},
		{route: "POST /v1/payment/send-email", model: EmailSent{}},
		{route: "POST /v1/payment/resend", model: OkResult{}},
		{route: "POST /v1/payment/accepted/set", model: OkResult{}},
		{route: "POST /v1/payment/accepted/list", path: "items.0", model: AcceptedMethod{}},
		{route: "POST /v1/payment/accuracy/get", model: AccuracyConfig{}},
		{route: "POST /v1/payment/accuracy/set", model: AccuracyConfig{}},
		{route: "POST /v1/payment/autorefund/get", model: AutoRefundConfig{}},
		{route: "POST /v1/payment/autorefund/set", model: AutoRefundConfig{}},
		{route: "POST /v1/payment/fee-config/get", model: PaymentFeeConfig{}},
		{route: "POST /v1/payment/fee-config/set", model: PaymentFeeConfig{}},
		{route: "POST /v1/payout/fee-config/get", model: PayoutFeeConfig{}},
		{route: "POST /v1/payout/fee-config/set", model: PayoutFeeConfig{}},
		{route: "POST /v1/payout/refund-fee-config/get", model: RefundFeeConfig{}},
		{route: "POST /v1/payout/refund-fee-config/set", model: RefundFeeConfig{}},
		{route: "POST /v1/vrcs", model: VRCSStatus{}},
		{route: "POST /v1/wallet", model: Wallet{}},
		{route: "POST /v1/wallet/block", model: WalletBlocked{}},
		{route: "POST /v1/wallet/qr", model: WalletQR{}},
		{route: "POST /v1/wallet/blocked-address-refund", model: Payout{}},
		{route: "POST /v1/transfer/to-personal", model: TransferToPersonal{}},
		{route: "POST /v1/transfer/to-user", model: TransferToUser{}},
		{route: "POST /v1/documents/jobs", model: DocumentJob{}},
		{route: "POST /v1/documents/jobs/info", model: DocumentJob{}},
		{route: "POST /v1/documents/jobs/info", path: "file", model: DocumentFile{}},
		{route: "POST /v1/documents/jobs/info", path: "period", model: DocumentPeriod{}},
		{route: "POST /v1/test-webhook/payment", model: WebhookTestResult{}},
		{route: "POST /v1/test-webhook/payout", model: WebhookTestResult{}},
		{route: "POST /v1/test-webhook/wallet", model: WebhookTestResult{}},
		{route: "POST /v1/payment/testing-webhook", model: WebhookTestResult{}},
		{route: "POST /v1/sandbox/faucet", model: FaucetResult{}},
		{route: "POST /v1/sandbox/deposit", model: SandboxDeposit{}},
		{route: "POST /v1/sandbox/reset", model: SandboxReset{}},
		{route: "POST /v1/sandbox/webhooks/replay", model: SandboxReplay{}},
		{route: "POST /v1/merchants", model: MerchantOnboarded{}},
		{route: "POST /v1/merchants", path: "api_key", model: APIKeyPair{}},
		{route: "POST /v1/merchants/{id}/sandbox", model: SandboxStore{}},
	}
}
