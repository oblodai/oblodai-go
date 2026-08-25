package oblodai

// CurrencyNetwork is one network a currency lives on.
type CurrencyNetwork struct {
	Network Network `json:"network"`
	// Kind is native or token.
	Kind string `json:"kind"`
	// Contract is the token contract address, for tokens.
	Contract         string `json:"contract,omitempty"`
	MinConfirmations int    `json:"min_confirmations"`
	// Available reports that deposits and payouts are both possible right now.
	Available        bool `json:"available"`
	DepositAvailable bool `json:"deposit_available"`
	PayoutAvailable  bool `json:"payout_available"`
	// DefaultOffer marks the network offered first on the pay page.
	DefaultOffer bool `json:"default_offer"`
}

// CurrencyInfo is one asset of the catalog and every network it settles on.
type CurrencyInfo struct {
	Currency string            `json:"currency"`
	Decimals int               `json:"decimals"`
	Networks []CurrencyNetwork `json:"networks"`
}

// PricingCurrency is a currency an invoice may be priced in.
type PricingCurrency struct {
	Currency string `json:"currency"`
	Decimals int    `json:"decimals"`
	Fiat     bool   `json:"fiat"`
}

// Currencies is the body of GET /v1/currencies: the settlement catalog and the pricing
// currencies.
type Currencies struct {
	Currencies        []CurrencyInfo    `json:"currencies"`
	PricingCurrencies []PricingCurrency `json:"pricing_currencies"`
}

// ExchangeRate is one item of /v1/exchange-rate/list: 1 From equals Course To.
type ExchangeRate struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Course string `json:"course"`
}
