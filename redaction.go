package oblodai

import (
	"encoding/json"
	"fmt"
)

// Secrets a program holds get printed by accident: a struct dumped with %+v into a log, a model
// serialized into an audit trail, a client rendered by a debugger. Every value in this package
// that carries a secret therefore renders it as [redacted] in both paths a Go program prints
// through — fmt (String/GoString) and encoding/json (MarshalJSON) — while the field itself keeps
// the real value for the code that needs it.
//
// The one place a secret is meant to leave the process is the API call it signs.

// debugString renders a value through its own (redacting) MarshalJSON, so a printed struct and a
// serialized one can never disagree about what is hidden.
func debugString(name string, value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "oblodai." + name + "{}"
	}
	return "oblodai." + name + string(encoded)
}

// String renders the endpoint with its secret hidden.
func (e WebhookEndpoint) String() string { return debugString("WebhookEndpoint", e) }

// GoString renders the endpoint with its secret hidden (%#v).
func (e WebhookEndpoint) GoString() string { return e.String() }

// MarshalJSON serializes the endpoint with its secret replaced by [redacted]. Read the Secret
// field itself to store it where it belongs.
func (e WebhookEndpoint) MarshalJSON() ([]byte, error) {
	type plain WebhookEndpoint
	out := plain(e)
	if out.Secret != "" {
		out.Secret = redactedPlaceholder
	}
	return json.Marshal(out)
}

// String renders the rotation result with its secret hidden.
func (e WebhookSecretRotated) String() string { return debugString("WebhookSecretRotated", e) }

// GoString renders the rotation result with its secret hidden (%#v).
func (e WebhookSecretRotated) GoString() string { return e.String() }

// MarshalJSON serializes the rotation result with its secret replaced by [redacted].
func (e WebhookSecretRotated) MarshalJSON() ([]byte, error) {
	type plain WebhookSecretRotated
	out := plain(e)
	if out.Secret != "" {
		out.Secret = redactedPlaceholder
	}
	return json.Marshal(out)
}

// String renders the key pair with its secret hidden.
func (k APIKeyPair) String() string { return debugString("APIKeyPair", k) }

// GoString renders the key pair with its secret hidden (%#v).
func (k APIKeyPair) GoString() string { return k.String() }

// MarshalJSON serializes the key pair with its secret replaced by [redacted].
func (k APIKeyPair) MarshalJSON() ([]byte, error) {
	type plain APIKeyPair
	out := plain(k)
	if out.Secret != "" {
		out.Secret = redactedPlaceholder
	}
	return json.Marshal(out)
}

// String renders the payout link with its claim token and passcode hidden — both are bearer
// secrets: whoever reads one can claim the money.
func (l PayoutLink) String() string { return debugString("PayoutLink", l) }

// GoString renders the payout link with its bearer secrets hidden (%#v).
func (l PayoutLink) GoString() string { return l.String() }

// MarshalJSON serializes the payout link with ClaimToken and Passcode replaced by [redacted].
// ClaimURL keeps the token, so treat it as a secret too: send it to the recipient, do not log it.
func (l PayoutLink) MarshalJSON() ([]byte, error) {
	type plain PayoutLink
	out := plain(l)
	if out.ClaimToken != "" {
		out.ClaimToken = redactedPlaceholder
	}
	if out.Passcode != "" {
		out.Passcode = redactedPlaceholder
	}
	return json.Marshal(out)
}

// String renders the client without its keys.
func (c *Client) String() string {
	if c == nil || c.transport == nil {
		return "oblodai.Client{}"
	}
	return c.transport.String()
}

// GoString renders the client without its keys (%#v).
func (c *Client) GoString() string { return c.String() }

// String renders the transport without its keys.
func (t *transport) String() string {
	if t == nil {
		return "oblodai.Client{}"
	}
	return fmt.Sprintf("oblodai.Client{baseURL: %q, credentials: %s, payoutCredentials: %s, timeout: %s, budget: %s}",
		t.baseURL, t.creds, t.payoutCreds, t.timeout, t.budget)
}

// GoString renders the transport without its keys (%#v).
func (t *transport) GoString() string { return t.String() }

// String renders a key pair as its public id only; the secret never reaches a log line.
func (c *credentials) String() string {
	if c == nil {
		return "<none>"
	}
	return fmt.Sprintf("{publicID: %q, secret: %s}", c.publicID, redactedPlaceholder)
}

// GoString renders a key pair without its secret (%#v).
func (c *credentials) GoString() string { return c.String() }

// String renders the resolved configuration without its keys or admin token.
func (c *config) String() string {
	if c == nil {
		return "oblodai.config{}"
	}
	admin := "<none>"
	if c.adminToken != "" {
		admin = redactedPlaceholder
	}
	return fmt.Sprintf("oblodai.config{baseURL: %q, publicID: %q, secret: %s, payoutPublicID: %q, payoutSecret: %s, adminToken: %s}",
		c.baseURL, c.publicID, redactedPlaceholder, c.payoutPublicID, redactedPlaceholder, admin)
}

// GoString renders the resolved configuration without its keys (%#v).
func (c *config) GoString() string { return c.String() }
