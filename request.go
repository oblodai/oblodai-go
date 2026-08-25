package oblodai

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Building the outgoing request — URL, headers, body — is a pure function of its inputs, so the
// signing material (what is signed) and the wire bytes (what is sent) come from one place and
// cannot disagree. Nothing here touches the network or the clock.

// credentials is one API key pair.
type credentials struct {
	publicID string
	secret   string
}

// buildInput is everything one attempt needs.
type buildInput struct {
	baseURL        string
	route          Route
	pathParams     map[string]string
	query          url.Values
	body           []byte
	creds          *credentials
	idempotencyKey string
	ts             int64
	userAgent      string
	extraHeaders   map[string]string
}

// builtRequest is the request as it will go on the wire.
type builtRequest struct {
	url     string
	method  string
	headers map[string]string
	body    []byte
	// requestURI is what was signed (path plus query); kept for debugging signature mismatches.
	requestURI string
}

// Headers the client owns; a caller-supplied header with one of these names is dropped rather
// than allowed to break the signature or impersonate another merchant.
var reservedHeaders = map[string]bool{
	strings.ToLower(HeaderPublicID):       true,
	strings.ToLower(HeaderSignature):      true,
	strings.ToLower(HeaderTimestamp):      true,
	strings.ToLower(HeaderIdempotencyKey): true,
	"content-type":                        true,
	"content-length":                      true,
	"host":                                true,
}

func buildRequest(in buildInput) (*builtRequest, *Error) {
	path, err := fillPath(in.route.Path, in.pathParams)
	if err != nil {
		return nil, err
	}
	full, err := joinURL(in.baseURL, path)
	if err != nil {
		return nil, err
	}
	if len(in.query) > 0 {
		full.RawQuery = in.query.Encode()
	}
	requestURI := full.EscapedPath()
	if full.RawQuery != "" {
		requestURI += "?" + full.RawQuery
	}

	headers := map[string]string{}
	for k, v := range in.extraHeaders {
		if !reservedHeaders[strings.ToLower(k)] {
			headers[k] = v
		}
	}
	headers["Accept"] = "application/json"
	headers["User-Agent"] = in.userAgent
	hasBody := in.route.Method != "GET"
	if hasBody {
		headers["Content-Type"] = "application/json"
	}
	if in.idempotencyKey != "" {
		headers[HeaderIdempotencyKey] = in.idempotencyKey
	}

	if in.route.Auth != AuthPublic && in.route.Auth != AuthOnboard {
		if in.creds == nil {
			kind := string(in.route.Auth)
			if in.route.Auth == AuthAny {
				kind = "merchant"
			}
			return nil, newConfigError(CodeMissingCredentials, fmt.Sprintf(
				"%s %s needs a %s API key: pass oblodai.WithCredentials(publicID, secret) or set OBLODAI_PUBLIC_ID and OBLODAI_SECRET",
				in.route.Method, in.route.Path, kind), "")
		}
		signed := in.body
		if !hasBody {
			signed = nil
		}
		headers[HeaderPublicID] = in.creds.publicID
		headers[HeaderTimestamp] = strconv.FormatInt(in.ts, 10)
		headers[HeaderSignature] = SignRequest(in.creds.secret, SignInput{
			TS:             in.ts,
			Method:         in.route.Method,
			RequestURI:     requestURI,
			IdempotencyKey: in.idempotencyKey,
			Body:           signed,
		})
	}

	sent := in.body
	if !hasBody {
		sent = nil
	}
	return &builtRequest{url: full.String(), method: in.route.Method, headers: headers, body: sent, requestURI: requestURI}, nil
}

// joinURL appends a route path to the base URL, keeping any path prefix the base carries
// (https://gw.corp/oblodai + /v1/balance -> https://gw.corp/oblodai/v1/balance).
func joinURL(baseURL, routePath string) (*url.URL, *Error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, newConfigError(CodeBadConfig, "the base URL is not a valid URL: "+baseURL, "baseURL")
	}
	prefix := strings.TrimRight(base.Path, "/")
	base.Path = prefix + routePath
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	return base, nil
}

// fillPath substitutes {name} segments. Every placeholder must be supplied and each value must be
// a single path segment: an empty value, "." or ".." or anything containing a slash would rewrite
// the URL and send a signed request somewhere else entirely.
func fillPath(template string, params map[string]string) (string, *Error) {
	var out strings.Builder
	rest := template
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			out.WriteString(rest)
			return out.String(), nil
		}
		close := strings.IndexByte(rest[open:], '}')
		if close < 0 {
			out.WriteString(rest)
			return out.String(), nil
		}
		close += open
		name := rest[open+1 : close]
		value := params[name]
		if value == "" || value == "." || value == ".." || strings.Contains(value, "/") {
			return "", newConfigError(CodeBadPathParam, fmt.Sprintf(
				"path parameter %q for %s must be a non-empty single segment (got %q)", name, template, value), name)
		}
		out.WriteString(rest[:open])
		out.WriteString(url.PathEscape(value))
		rest = rest[close+1:]
	}
}

// serializeBody encodes a request body once. GETs carry none; a nil body on a POST becomes {}
// because the core's decoders expect an object.
func serializeBody(body any, method string) ([]byte, *Error) {
	if method == "GET" {
		return nil, nil
	}
	if body == nil {
		return []byte("{}"), nil
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, newConfigError(CodeBadConfig, "the request body cannot be encoded as JSON: "+err.Error(), "")
	}
	if string(encoded) == "null" {
		return []byte("{}"), nil
	}
	return encoded, nil
}
