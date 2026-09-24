package oblodai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Secrets a program holds get printed by accident: a struct dumped with %+v into a log, a client
// rendered by a debugger. Every model and the client therefore render a secret as [redacted]
// through fmt (String/GoString), while the field itself keeps the real value for the code that
// needs it.
//
// The one place a secret is meant to leave the process is the API call it signs.

// describe is what fmt prints for a generated model (its String and GoString): the fields that
// are set, by their JSON names, with the value of every secret-looking field — a webhook secret, a
// claim token or URL, a passcode — replaced by [redacted] at any depth. JSON encoding is left
// faithful: it is the model as it is sent and stored.
func describe(name string, model any) string {
	encoded, err := json.Marshal(model)
	if err != nil {
		return "oblodai." + name + "{}"
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil {
		return "oblodai." + name + "{}"
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("oblodai." + name + "{")
	first := true
	for _, key := range keys {
		value := redactTree(key, fields[key])
		if unset(value) {
			continue
		}
		rendered, err := json.Marshal(value)
		if err != nil {
			continue
		}
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(key + ": " + string(rendered))
	}
	b.WriteString("}")
	return b.String()
}

// secretFields are words that make a model field's string value a secret when printed.
var secretFields = append([]string{"claim_url"}, sensitiveWords...)

// redactTree replaces the string value of a secret-looking key, looking into objects and arrays.
func redactTree(key string, value any) any {
	switch typed := value.(type) {
	case string:
		lower := strings.ToLower(key)
		for _, word := range secretFields {
			if typed != "" && strings.Contains(lower, word) {
				return redactedPlaceholder
			}
		}
		return typed
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = redactTree(k, v)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = redactTree(key, v)
		}
		return out
	}
	return value
}

// unset reports a value not worth printing: null, "", [] or {}. false and 0 are values.
func unset(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	}
	return false
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
	return fmt.Sprintf("oblodai.Client{baseURL: %q, credentials: %s, timeout: %s, budget: %s}",
		t.baseURL, t.creds, t.timeout, t.budget)
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
	return fmt.Sprintf("oblodai.config{baseURL: %q, publicID: %q, secret: %s, adminToken: %s}",
		c.baseURL, c.publicID, redactedPlaceholder, admin)
}

// GoString renders the resolved configuration without its keys (%#v).
func (c *config) GoString() string { return c.String() }
