package oblodai

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
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
	// adminToken is set only on onboarding routes. It travels in its own field, never through
	// extraHeaders, so a caller header named X-Admin-Token can be dropped without dropping this.
	adminToken string
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
// than allowed to break the signature, impersonate another merchant or claim admin rights on a
// route that is not an onboarding route. The comparison is case-insensitive.
var reservedHeaders = map[string]bool{
	strings.ToLower(HeaderPublicID):       true,
	strings.ToLower(HeaderSignature):      true,
	strings.ToLower(HeaderTimestamp):      true,
	strings.ToLower(HeaderIdempotencyKey): true,
	strings.ToLower(HeaderAdminToken):     true,
	"accept":                              true,
	"user-agent":                          true,
	"content-type":                        true,
	"content-length":                      true,
	"host":                                true,
}

// ReservedHeaders lists, in lower case, the header names the client owns: WithHeader and
// WithRequestHeader ignore them.
func ReservedHeaders() []string {
	out := make([]string, 0, len(reservedHeaders))
	for name := range reservedHeaders {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// checkHeader refuses a caller header the client cannot send verbatim. A CR or LF would split the
// request; a non-ASCII byte is not a legal header value and different HTTP stacks disagree on what
// to do with it, so it is refused here rather than mangled on the wire.
func checkHeader(name, value string) *Error {
	if name == "" {
		return newConfigError(CodeBadHeader, "a request header needs a name", "headers")
	}
	for _, part := range [2]string{name, value} {
		for i := 0; i < len(part); i++ {
			c := part[i]
			if c == '\r' || c == '\n' {
				return newConfigError(CodeBadHeader, fmt.Sprintf(
					"the %q request header contains a line break; header names and values must be one line", name), "headers")
			}
			if c < 0x20 || c > 0x7e {
				return newConfigError(CodeBadHeader, fmt.Sprintf(
					"the %q request header must be printable ASCII (got byte %#02x)", name, c), "headers")
			}
		}
	}
	return nil
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
		if err := checkHeader(k, v); err != nil {
			return nil, err
		}
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
	if in.adminToken != "" && in.route.Auth == AuthOnboard {
		headers[HeaderAdminToken] = in.adminToken
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

// fillPath substitutes {name} segments and returns the path with its values still unescaped:
// url.URL escapes the path exactly once when the request is built, and escaping here as well
// would send "a b" as "a%2520b". Every placeholder must be supplied and each value must be a
// single path segment: an empty value, "." or ".." or anything containing a slash would rewrite
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
		out.WriteString(value)
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
