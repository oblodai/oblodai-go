package oblodai

// FaucetResult is the body of /v1/sandbox/faucet: the credit the faucet minted.
type FaucetResult struct {
	Asset  string `json:"asset"`
	Amount Money  `json:"amount"`
	// JournalID is the ledger entry the credit was written as.
	JournalID string `json:"journal_id"`
}

// SandboxDeposit is the body of /v1/sandbox/deposit: the synthetic deposit paid into an invoice.
type SandboxDeposit struct {
	InvoiceID     string `json:"invoice_id"`
	Amount        Money  `json:"amount"`
	Confirmations int    `json:"confirmations"`
	TxID          string `json:"txid"`
}

// SandboxReset is the body of /v1/sandbox/reset: what wiping the dev store touched.
type SandboxReset struct {
	InvoicesCancelled int `json:"invoices_cancelled"`
	BalancesZeroed    int `json:"balances_zeroed"`
}

// SandboxReplay is the body of /v1/sandbox/webhooks/replay.
type SandboxReplay struct {
	OK         bool   `json:"ok"`
	DeliveryID string `json:"delivery_id"`
}
