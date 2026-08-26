package oblodai

// APIKeyPair is the merchant's API key pair as onboarding mints it: a public id
// (oblodai_<hex>, or test_oblodai_<hex> in the sandbox) and a secret shown once.
type APIKeyPair struct {
	PublicID string `json:"public_id"`
	Secret   string `json:"secret"`
}

// MerchantOnboarded is the body of POST /v1/merchants: a freshly provisioned merchant and its
// one API key.
type MerchantOnboarded struct {
	MerchantID string `json:"merchant_id"`
	ProjectID  string `json:"project_id"`
	// APIKey signs every route the gateway gates; the secret is shown here and never again.
	APIKey APIKeyPair `json:"api_key"`
}

// SandboxStore is the body of POST /v1/merchants/{id}/sandbox: the merchant's dev store and its
// test_ key.
type SandboxStore struct {
	MerchantID string     `json:"merchant_id"`
	ProjectID  string     `json:"project_id"`
	APIKey     APIKeyPair `json:"api_key"`
	// Created is false when the dev store already existed: the call is idempotent.
	Created bool `json:"created"`
}
