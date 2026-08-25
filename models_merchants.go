package oblodai

// APIKeyPair is an API key pair as onboarding mints it. The secret is shown once.
type APIKeyPair struct {
	PublicID string `json:"public_id"`
	Secret   string `json:"secret"`
	// Kind is "api", the unified key kind current merchants receive.
	Kind string `json:"kind"`
}

// MerchantOnboarded is the body of POST /v1/merchants: a freshly provisioned merchant and its
// keys.
type MerchantOnboarded struct {
	MerchantID string `json:"merchant_id"`
	ProjectID  string `json:"project_id"`
	// APIKey is the unified key, the same as PaymentKey and PayoutKey for merchants created now.
	APIKey     APIKeyPair `json:"api_key"`
	PaymentKey APIKeyPair `json:"payment_key"`
	PayoutKey  APIKeyPair `json:"payout_key"`
}

// SandboxStore is the body of POST /v1/merchants/{id}/sandbox: the merchant's dev store and its
// test_ key.
type SandboxStore struct {
	MerchantID string     `json:"merchant_id"`
	ProjectID  string     `json:"project_id"`
	APIKey     APIKeyPair `json:"api_key"`
	PaymentKey APIKeyPair `json:"payment_key"`
	PayoutKey  APIKeyPair `json:"payout_key"`
	// Created is false when the dev store already existed: the call is idempotent.
	Created bool `json:"created"`
}
