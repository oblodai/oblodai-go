package oblodai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
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
var secretFields = sensitiveWords

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
		return redactLinkString(typed)
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

// secretQueryKeys are the query parameters of a signed link (a presigned document, a receipt):
// whoever holds the link with them opens the document, so they are redacted wherever a URL is
// printed — a hook, an error, a model's String.
var secretQueryKeys = map[string]bool{"sig": true, "exp": true, "token": true, "signature": true}

// isSecretParam reports a path or query parameter whose value is a bearer secret: a claim or AML
// token, a code, a passcode, a signed link's sig/exp.
func isSecretParam(name string) bool {
	lower := strings.ToLower(name)
	if secretQueryKeys[lower] || lower == "code" || strings.HasSuffix(lower, "_code") {
		return true
	}
	for _, word := range sensitiveWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// redactQuery returns a copy of query with the values of secret parameters redacted.
func redactQuery(query url.Values) url.Values {
	out := make(url.Values, len(query))
	for k, v := range query {
		if isSecretParam(k) {
			out[k] = []string{redactedPlaceholder}
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}

// redactedEscaped undoes the escaping url.URL applies to the placeholder, so a printed URL reads
// [redacted] rather than %5Bredacted%5D.
var redactedEscaped = strings.NewReplacer(url.PathEscape(redactedPlaceholder), redactedPlaceholder,
	url.QueryEscape(redactedPlaceholder), redactedPlaceholder)

// redactLinkString scrubs the userinfo and the secret query parameters out of a string that is an
// absolute URL; any other string is returned as it is.
func redactLinkString(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	parsed, err := url.Parse(s)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return s
	}
	changed := false
	if parsed.User != nil {
		parsed.User = nil
		changed = true
	}
	if parsed.RawQuery != "" {
		query := parsed.Query()
		for k := range query {
			if isSecretParam(k) {
				changed = true
			}
		}
		if changed {
			parsed.RawQuery = redactQuery(query).Encode()
		}
	}
	if !changed {
		return s
	}
	return redactedEscaped.Replace(parsed.String())
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

// String renders the resolved configuration without its keys.
func (c *config) String() string {
	if c == nil {
		return "oblodai.config{}"
	}
	return fmt.Sprintf("oblodai.config{baseURL: %q, publicID: %q, secret: %s}",
		c.baseURL, c.publicID, redactedPlaceholder)
}

// GoString renders the resolved configuration without its keys (%#v).
func (c *config) GoString() string { return c.String() }
