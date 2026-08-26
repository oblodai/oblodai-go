package oblodai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// The failures a client hides best: a path parameter escaped twice, a lazy list raced by two
// goroutines, an error identity lost when the budget runs out, a secret printed by accident.

// A path parameter is escaped exactly once. Escaping it here and again in net/url would send
// "a b" as "a%2520b", and the core would look up an order id nobody has.
func TestPathParametersAreEscapedExactlyOnce(t *testing.T) {
	for _, value := range []string{"a b", "id+plus", "%41", "ünïcode", "a?b#c"} {
		api := newFakeAPI(t, ok(map[string]any{"link_id": "l1"}))
		if _, err := api.client().PaymentLinks.PublicView(context.Background(), value); err != nil {
			t.Fatalf("PublicView(%q): %v", value, err)
		}
		got := api.last()
		if want := "/v1/link/" + value; got.path != want {
			t.Errorf("the core saw %q, want %q (raw %q)", got.path, want, got.rawPath)
		}
		if strings.Contains(got.rawPath, "%25") && !strings.Contains(value, "%") {
			t.Errorf("the path was escaped twice: %q", got.rawPath)
		}
	}
}

// The signature covers the escaped path, so what is signed and what is sent must agree exactly
// once: buildRequest is the only place both are produced.
func TestWhatIsSignedIsWhatIsSent(t *testing.T) {
	built, err := buildRequest(buildInput{
		baseURL:    "https://api.example",
		route:      Route{Method: "POST", Path: "/v1/claim/{token}", Auth: AuthKey},
		pathParams: map[string]string{"token": "tok en"},
		body:       []byte("{}"),
		creds:      &credentials{publicID: "pk", secret: "sk"},
		ts:         1_755_600_000,
		userAgent:  "test",
	})
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	if built.requestURI != "/v1/claim/tok%20en" {
		t.Fatalf("signed %q, want /v1/claim/tok%%20en", built.requestURI)
	}
	if built.url != "https://api.example/v1/claim/tok%20en" {
		t.Fatalf("sent %q", built.url)
	}
	want := SignRequest("sk", SignInput{TS: 1_755_600_000, Method: "POST", RequestURI: built.requestURI, Body: []byte("{}")})
	if built.headers[HeaderSignature] != want {
		t.Fatal("the signature does not cover the URI that was sent")
	}
}

// Two goroutines asking a list for its first page must make one request and see one answer.
// Run with -race: before the fix this wrote first/firstErr/fetched without synchronisation.
func TestListFirstPageIsFetchedOnceUnderConcurrency(t *testing.T) {
	api := newFakeAPI(t, pageOf([]any{map[string]any{"uuid": "p1"}}, 0, 1, 50))
	list := api.client().Payments.History(context.Background(), PaymentHistoryParams{})

	const readers = 8
	var wg sync.WaitGroup
	pages := make([]*Page[Payment], readers)
	errs := make([]error, readers)
	start := make(chan struct{})
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			pages[i], errs[i] = list.Page()
		}(i)
	}
	close(start)
	wg.Wait()

	for i := range pages {
		if errs[i] != nil {
			t.Fatalf("reader %d: %v", i, errs[i])
		}
		if pages[i] != pages[0] {
			t.Fatalf("reader %d got a different page than reader 0", i)
		}
	}
	if api.count() != 1 {
		t.Fatalf("the first page was fetched %d times, want once", api.count())
	}
}

// When the call budget runs out, the error the caller receives must still say what the API said:
// errors.As has to bind the outer error, so the outer error carries the identity.
func TestDeadlineErrorCarriesTheLastAPIError(t *testing.T) {
	rateLimited := apiError(429, map[string]any{
		"code": "request.rate_limited", "message": "slow down", "retryable": true,
		"retry_after": 5, "request_id": "req-42",
	})
	api := newFakeAPI(t, rateLimited, rateLimited, rateLimited)
	client := api.client(
		WithCallBudget(30*time.Millisecond),
		WithRetry(RetryOptions{MaxRetries: 5, BaseDelay: time.Second, MaxDelay: time.Second, MaxRetryAfter: time.Second}),
	)
	_, err := client.Payments.Info(context.Background(), PaymentInfoParams{UUID: "p1"})

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As found no *Error in %v", err)
	}
	if apiErr.Code != CodeTransportDeadline {
		t.Fatalf("code = %q, want %q", apiErr.Code, CodeTransportDeadline)
	}
	if apiErr.LastCode != "request.rate_limited" {
		t.Errorf("lastCode = %q, want request.rate_limited", apiErr.LastCode)
	}
	if apiErr.HTTPStatus != 429 {
		t.Errorf("httpStatus = %d, want 429", apiErr.HTTPStatus)
	}
	if apiErr.RequestID != "req-42" {
		t.Errorf("requestId = %q, want req-42", apiErr.RequestID)
	}
	if apiErr.RetryAfter == nil || *apiErr.RetryAfter != 5 {
		t.Errorf("retryAfter = %v, want 5", apiErr.RetryAfter)
	}
	// The last error is still reachable as the cause.
	cause := errors.Unwrap(apiErr)
	var last *Error
	if !errors.As(cause, &last) || last.Code != "request.rate_limited" {
		t.Fatalf("the cause must be the last API error, got %v", cause)
	}
	// And a structured log of it keeps the identity.
	encoded, err := json.Marshal(apiErr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"lastCode":"request.rate_limited"`) {
		t.Fatalf("serialized error lost the last code: %s", encoded)
	}
}

// Headers the client owns cannot be set by a caller, and a header it cannot send verbatim is
// refused before anything reaches the wire.
func TestCallerHeadersCannotClaimWhatTheClientOwns(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"merchant_id": "m1", "project_id": "p1"}))
	client := api.client(
		WithAdminToken("real-admin"),
		WithHeader("User-Agent", "not-the-sdk"),
		WithHeader("X-Admin-Token", "stolen"),
		WithHeader("X-Trace", "keep-me"),
	)
	if _, err := client.Merchants.Create(context.Background(), MerchantsParams{Email: "a@b.c"}); err != nil {
		t.Fatalf("Merchants.Create: %v", err)
	}
	got := api.last()
	if agent := got.header.Get("User-Agent"); !strings.HasPrefix(agent, "oblodai-go/") {
		t.Errorf("User-Agent = %q, the client owns it", agent)
	}
	if token := got.header.Get(HeaderAdminToken); token != "real-admin" {
		t.Errorf("X-Admin-Token = %q, want the configured one", token)
	}
	if got.header.Get("X-Trace") != "keep-me" {
		t.Error("an ordinary caller header must survive")
	}

	// A payment route must not carry the admin token at all, however the caller asks.
	payments := newFakeAPI(t, ok(map[string]any{"uuid": "p1"}))
	client = payments.client(WithAdminToken("real-admin"), WithHeader("x-admin-token", "stolen"))
	if _, err := client.Payments.Info(context.Background(), PaymentInfoParams{UUID: "p1"}); err != nil {
		t.Fatalf("Payments.Info: %v", err)
	}
	if token := payments.last().header.Get(HeaderAdminToken); token != "" {
		t.Errorf("a merchant route carried X-Admin-Token %q", token)
	}
}

func TestUnsendableCallerHeadersAreRefused(t *testing.T) {
	for name, header := range map[string][2]string{
		"a line break in the value": {"X-Trace", "one\r\nX-Admin-Token: stolen"},
		"a line break in the name":  {"X-Tr\nace", "v"},
		"a non-ASCII value":         {"X-Trace", "naïve"},
	} {
		api := newFakeAPI(t, ok(map[string]any{"uuid": "p1"}))
		client := api.client(WithHeader(header[0], header[1]))
		_, err := client.Payments.Info(context.Background(), PaymentInfoParams{UUID: "p1"})
		if !IsConfig(err) || !IsCode(err, CodeBadHeader) {
			t.Errorf("%s: want sdk.bad_header, got %v", name, err)
		}
		if api.count() != 0 {
			t.Errorf("%s: the request reached the network", name)
		}
	}
}

func TestPerCallHeaderWinsOverTheClientOne(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"uuid": "p1"}))
	client := api.client(WithHeader("X-Trace", "client"))
	if _, err := client.Payments.Info(context.Background(), PaymentInfoParams{UUID: "p1"},
		WithRequestHeader("X-Trace", "call")); err != nil {
		t.Fatalf("Payments.Info: %v", err)
	}
	if got := api.last().header.Get("X-Trace"); got != "call" {
		t.Fatalf("X-Trace = %q, want the per-call value", got)
	}
}

// An injected http.Client whose transport follows a redirect itself must not go unnoticed: the
// answer would come from an origin the request was not signed for.
func TestARedirectFollowedByAnInjectedClientIsCaught(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"uuid": "p1"}))
	client := api.client(WithHTTPClient(&http.Client{Transport: redirectingTransport{}}))
	_, err := client.Payments.Info(context.Background(), PaymentInfoParams{UUID: "p1"})
	apiErr := mustError(t, err)
	if !strings.Contains(apiErr.Message, "unexpected redirect") {
		t.Fatalf("want an unexpected-redirect error, got %v", err)
	}
}

// redirectingTransport answers from a different URL than the one that was requested, the way an
// HTTP stack that follows redirects on its own would.
type redirectingTransport struct{}

func (redirectingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	elsewhere := req.Clone(req.Context())
	elsewhere.URL.Host = "elsewhere.example"
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"state":0,"result":{}}`)),
		Request:    elsewhere,
	}, nil
}

// A secret reads normally as a field and never prints.
func TestSecretsAreRedactedWhenPrintedAndSerialized(t *testing.T) {
	endpoint := WebhookEndpoint{EndpointID: "e1", URL: "https://shop.example", Secret: "whsec_live"}
	rotated := WebhookSecretRotated{EndpointID: "e1", Secret: "whsec_new"}
	keys := APIKeyPair{PublicID: "pk_live_1", Secret: "sk_live_1"}
	link := PayoutLink{
		LinkID:     "l1",
		ClaimToken: "cl4im-tok3n",
		ClaimURL:   "https://pay.test/claim/cl4im-tok3n",
		Passcode:   "1234",
	}

	if endpoint.Secret != "whsec_live" || keys.Secret != "sk_live_1" || link.Passcode != "1234" {
		t.Fatal("the fields themselves must keep the real value")
	}
	if link.ClaimURL != "https://pay.test/claim/cl4im-tok3n" {
		t.Fatal("the claim URL must stay readable as a field")
	}
	for _, rendered := range []string{
		fmt.Sprintf("%v", endpoint), fmt.Sprintf("%+v", endpoint), fmt.Sprintf("%#v", endpoint),
		fmt.Sprintf("%v", rotated), fmt.Sprintf("%v", keys), fmt.Sprintf("%+v", link),
		mustJSON(t, endpoint), mustJSON(t, rotated), mustJSON(t, keys), mustJSON(t, link),
		mustJSON(t, MerchantOnboarded{APIKey: keys}),
	} {
		for _, secret := range []string{"whsec_live", "whsec_new", "sk_live_1", "cl4im-tok3n", "pay.test/claim", `"1234"`} {
			if strings.Contains(rendered, secret) {
				t.Errorf("a secret leaked into %s", rendered)
			}
		}
		if !strings.Contains(rendered, redactedPlaceholder) {
			t.Errorf("nothing was redacted in %s", rendered)
		}
	}
}

// A claim URL embeds the claim token, so it is a bearer secret wherever it travels — including
// inside a batch element, which is how a bulk mint hands links back.
func TestAClaimURLIsRedactedInsideABatchElement(t *testing.T) {
	link := PayoutLink{LinkID: "l1", ClaimToken: "cl4im-tok3n", ClaimURL: "https://pay.test/claim/cl4im-tok3n"}
	element := BatchElement[PayoutLink]{Idx: 0, OK: true, OrderID: "order-1", Result: &link}

	if element.Result.ClaimURL != "https://pay.test/claim/cl4im-tok3n" {
		t.Fatal("the element's own field must keep the real value")
	}
	for _, rendered := range []string{
		fmt.Sprintf("%v", element), fmt.Sprintf("%+v", element), mustJSON(t, element),
		mustJSON(t, []BatchElement[PayoutLink]{element}),
	} {
		if strings.Contains(rendered, "cl4im-tok3n") || strings.Contains(rendered, "pay.test/claim") {
			t.Errorf("a claim URL leaked into %s", rendered)
		}
		if !strings.Contains(rendered, redactedPlaceholder) {
			t.Errorf("nothing was redacted in %s", rendered)
		}
		if !strings.Contains(rendered, "order-1") {
			t.Errorf("the safe half of the element is gone from %s", rendered)
		}
	}
}

func TestClientAndCredentialsNeverPrintTheirKeys(t *testing.T) {
	api := newFakeAPI(t)
	client := api.client(WithAdminToken("admin-token"))
	for _, rendered := range []string{
		fmt.Sprintf("%v", client), fmt.Sprintf("%+v", client), fmt.Sprintf("%#v", client),
		fmt.Sprintf("%v", client.transport), fmt.Sprintf("%v", client.transport.creds),
	} {
		for _, secret := range []string{"secret-1", "admin-token"} {
			if strings.Contains(rendered, secret) {
				t.Fatalf("a secret leaked into %s", rendered)
			}
		}
	}
	cfg, err := resolve([]Option{WithCredentials("pk", "sk-secret"), WithAdminToken("admin-token")})
	if err != nil {
		t.Fatal(err)
	}
	if rendered := fmt.Sprintf("%v", cfg); strings.Contains(rendered, "sk-secret") || strings.Contains(rendered, "admin-token") {
		t.Fatalf("the configuration printed a secret: %s", rendered)
	}
}

// recordingLogger is a caller's own logger: it must never receive a secret value.
type recordingLogger struct {
	mu     sync.Mutex
	fields []LogFields
}

func (l *recordingLogger) record(fields LogFields) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fields = append(l.fields, fields)
}

func (l *recordingLogger) Debug(_ string, fields LogFields) { l.record(fields) }
func (l *recordingLogger) Info(_ string, fields LogFields)  { l.record(fields) }
func (l *recordingLogger) Warn(_ string, fields LogFields)  { l.record(fields) }
func (l *recordingLogger) Error(_ string, fields LogFields) { l.record(fields) }

func TestAnInjectedLoggerNeverReceivesASecret(t *testing.T) {
	recorder := &recordingLogger{}
	api := newFakeAPI(t, ok(map[string]any{"uuid": "p1"}))
	client := api.client(WithLogger(recorder))
	if _, err := client.Payments.Info(context.Background(), PaymentInfoParams{UUID: "p1"}); err != nil {
		t.Fatalf("Payments.Info: %v", err)
	}

	raw := LogFields{"secret": "whsec_live", "x-signature": "deadbeef", "passcode": "1234", "route": "POST /v1/payment"}
	client.transport.logger.Debug("probe", raw)
	if raw["secret"] != "whsec_live" {
		t.Fatal("redaction must not rewrite the caller's own map")
	}
	seen := false
	for _, fields := range recorder.fields {
		for key, value := range fields {
			rendered := fmt.Sprintf("%v", value)
			if strings.Contains(rendered, "whsec_live") || strings.Contains(rendered, "deadbeef") || rendered == "1234" {
				t.Fatalf("the logger received %s=%v", key, value)
			}
			if key == "secret" {
				seen = true
				if rendered != redactedPlaceholder {
					t.Fatalf("secret = %v, want %s", value, redactedPlaceholder)
				}
			}
			if key == "route" && !strings.HasPrefix(rendered, "POST /v1/payment") {
				t.Fatalf("an ordinary field was mangled: %v", value)
			}
		}
	}
	if !seen {
		t.Fatal("the probe never reached the logger")
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(encoded)
}
