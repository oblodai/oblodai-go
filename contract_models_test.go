package oblodai

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/oblodai/oblodai-go/internal/fixtures"
)

// The wire models against the golden bodies the core recorded. Two rules, both one-directional
// failures of the same drift:
//
//   - every key the core sent must exist in the model — otherwise a field silently disappears
//     from a user's program the day the core starts sending it;
//   - every model field that is not `omitempty` must be present on the wire — otherwise the model
//     promises a field the core no longer sends.
//
// Optional (omitempty) fields may be absent: that is what optional means.

// modelRow names a route, how to reach the object inside its result, and the model that decodes it.
type modelRow struct {
	route string
	// path walks into the result: "" is the result itself, "items.0" its first item.
	path  string
	model any
	// alsoOptional lists wire fields this particular recording may omit even though the model
	// declares them (a route that returns a narrower view of a shared object).
	alsoOptional []string
}

// autoWithdrawList is the shape of the plain {items} result the auto-withdraw routes answer with.
type autoWithdrawList struct {
	Items []AutoWithdrawRule `json:"items"`
}

func modelRows() []modelRow {
	return []modelRow{
		{route: "POST /v1/payment", model: Payment{}},
		{route: "POST /v1/payment/info", model: Payment{}},
		{route: "POST /v1/payment/cancel", model: Payment{}},
		{route: "POST /v1/payment/history", path: "items.0", model: Payment{}},
		{route: "GET /v1/pay/{id}", model: PublicPayment{}},
		{route: "POST /v1/pay/{id}/select", model: PublicPayment{}},
		{route: "POST /v1/link/{id}/checkout", model: PublicPayment{}},
		{route: "POST /v1/payment/qr", model: QRCode{}},
		{route: "GET /v1/pay/{id}/qr", model: QRCode{}},
		{route: "POST /v1/payment/services", path: "items.0", model: ServiceMethod{}},
		{route: "POST /v1/payout/services", path: "items.0", model: ServiceMethod{}},
		{route: "POST /v1/payment/batch", model: BatchSubmitted{}},
		{route: "POST /v1/payout/batch", model: BatchSubmitted{}},
		{route: "POST /v1/refund/batch", model: BatchSubmitted{}},
		{route: "POST /v1/transfer/batch", model: BatchSubmitted{}},
		{route: "POST /v1/batch/info", model: BatchInfo{}},
		{route: "POST /v1/payout", model: Payout{}},
		{route: "POST /v1/payout/info", model: Payout{}},
		{route: "POST /v1/payout/cancel", model: Payout{}},
		{route: "POST /v1/payout/history", path: "items.0", model: Payout{}},
		{route: "POST /v1/payout/mass", path: "items.0.result", model: Payout{}},
		{route: "POST /v1/payment/refund", model: Payout{}},
		{route: "POST /v1/payment/resolve", model: Resolution{}},
		{route: "POST /v1/payout/calculate", model: PayoutCalculation{}},
		{route: "POST /v1/payout/validate", model: PayoutValidation{}},
		{route: "POST /v1/payout/link", model: PayoutLink{}},
		{route: "POST /v1/payout/link/info", model: PayoutLink{}},
		{route: "POST /v1/payout/link/list", path: "items.0", model: PayoutLink{}},
		{route: "POST /v1/payout/link/cancel", model: PayoutLink{}},
		{route: "POST /v1/payout/link/batch", path: "items.0.result", model: PayoutLink{}},
		{route: "GET /v1/claim/{token}", model: ClaimPreview{}},
		{route: "POST /v1/claim/{token}", model: ClaimResult{}},
		{route: "POST /v1/payment/link", model: PaymentLinkCreated{}},
		{route: "POST /v1/payment/link/info", model: PaymentLink{}},
		{route: "POST /v1/payment/link/list", path: "items.0", model: PaymentLink{}},
		{route: "GET /v1/link/{id}", model: PublicPaymentLink{}},
		{route: "POST /v1/payment/link/toggle", model: PaymentLinkToggled{}},
		{route: "POST /v1/balance", model: Balance{}},
		{route: "POST /v1/referral/info", model: ReferralInfo{}},
		{route: "POST /v1/auto-withdraw/list", path: "items.0", model: AutoWithdrawRule{}},
		{route: "POST /v1/auto-withdraw/set", path: "items.0", model: AutoWithdrawRule{}},
		{route: "POST /v1/auto-withdraw/delete", model: autoWithdrawList{}},
		{route: "POST /v1/api-allowlist/list", model: APIAllowlist{}},
		{route: "POST /v1/api-allowlist/add", model: APIAllowlist{}},
		{route: "POST /v1/api-allowlist/remove", model: APIAllowlist{}},
		{route: "POST /v1/api-allowlist/enable", model: APIAllowlist{}},
		{route: "POST /v1/payment/discount/list", path: "items.0", model: DiscountRule{}},
		{route: "POST /v1/payment/discount/set", model: DiscountRule{}},
		{route: "POST /v1/split/rule", model: SplitRule{}},
		{route: "POST /v1/split/rule/list", path: "items.0", model: SplitRule{}},
		{route: "POST /v1/split/rule/delete", model: OkResult{}},
		{route: "POST /v1/split/config/get", model: SplitConfig{}},
		{route: "POST /v1/split/config/set", model: SplitConfig{}},
		{route: "POST /v1/split/recipient/optin", model: SplitOptIn{}},
		{route: "POST /v1/split/recipient/optin/get", model: SplitOptIn{}},
		{route: "GET /v1/currencies", model: Currencies{}},
		{route: "GET /v1/currencies", path: "currencies.0", model: CurrencyInfo{}},
		{route: "GET /v1/currencies", path: "currencies.0.networks.0", model: CurrencyNetwork{}},
		{route: "GET /v1/currencies", path: "pricing_currencies.0", model: PricingCurrency{}},
		{route: "POST /v1/exchange-rate/list", path: "items.0", model: ExchangeRate{}},
		{route: "POST /v1/webhooks", model: WebhookEndpoint{}},
		{route: "POST /v1/webhooks/rotate-secret", model: WebhookSecretRotated{}},
		{route: "POST /v1/webhooks/deliveries", path: "items.0", model: WebhookDelivery{}},
		{route: "GET /v1/sandbox/webhooks", path: "items.0", model: WebhookDelivery{}},
		{route: "POST /v1/payment/send-email", model: EmailSent{}},
		{route: "POST /v1/payment/resend", model: OkResult{}},
		{route: "POST /v1/payment/accepted/set", model: OkResult{}},
		{route: "POST /v1/payment/accepted/list", path: "items.0", model: AcceptedMethod{}},
		{route: "POST /v1/payment/accuracy/get", model: AccuracyConfig{}},
		{route: "POST /v1/payment/accuracy/set", model: AccuracyConfig{}},
		{route: "POST /v1/payment/autorefund/get", model: AutoRefundConfig{}},
		{route: "POST /v1/payment/autorefund/set", model: AutoRefundConfig{}},
		{route: "POST /v1/payment/fee-config/get", model: PaymentFeeConfig{}},
		{route: "POST /v1/payment/fee-config/set", model: PaymentFeeConfig{}},
		{route: "POST /v1/payout/fee-config/get", model: PayoutFeeConfig{}},
		{route: "POST /v1/payout/fee-config/set", model: PayoutFeeConfig{}},
		{route: "POST /v1/payout/refund-fee-config/get", model: RefundFeeConfig{}},
		{route: "POST /v1/payout/refund-fee-config/set", model: RefundFeeConfig{}},
		{route: "POST /v1/vrcs", model: VRCSStatus{}},
		{route: "POST /v1/wallet", model: Wallet{}},
		{route: "POST /v1/wallet/block", model: WalletBlocked{}},
		{route: "POST /v1/wallet/qr", model: WalletQR{}},
		{route: "POST /v1/wallet/blocked-address-refund", model: Payout{}},
		{route: "POST /v1/transfer/to-personal", model: TransferToPersonal{}},
		{route: "POST /v1/transfer/to-user", model: TransferToUser{}},
		{route: "POST /v1/documents/jobs", model: DocumentJob{}},
		{route: "POST /v1/documents/jobs/info", model: DocumentJob{}},
		{route: "POST /v1/documents/jobs/info", path: "file", model: DocumentFile{}},
		{route: "POST /v1/documents/jobs/info", path: "period", model: DocumentPeriod{}},
		{route: "POST /v1/test-webhook/payment", model: WebhookTestResult{}},
		{route: "POST /v1/test-webhook/payout", model: WebhookTestResult{}},
		{route: "POST /v1/test-webhook/wallet", model: WebhookTestResult{}},
		{route: "POST /v1/payment/testing-webhook", model: WebhookTestResult{}},
		{route: "POST /v1/sandbox/faucet", model: FaucetResult{}},
		{route: "POST /v1/sandbox/deposit", model: SandboxDeposit{}},
		{route: "POST /v1/sandbox/reset", model: SandboxReset{}},
		{route: "POST /v1/sandbox/webhooks/replay", model: SandboxReplay{}},
		{route: "POST /v1/merchants", model: MerchantOnboarded{}},
		{route: "POST /v1/merchants", path: "api_key", model: APIKeyPair{}},
		{route: "POST /v1/merchants/{id}/sandbox", model: SandboxStore{}},
	}
}

// Routes the API guarantees to refuse for API keys, so no success body exists to model.
var notModelled = map[string]bool{
	// API-key payouts approve themselves; approve serves the cabinet's maker-checker flow.
	"POST /v1/payout/approve": true,
}

func TestModelsMatchTheGoldenBodies(t *testing.T) {
	recorded := fixtures.LoadFixtures(t)
	for _, row := range modelRows() {
		name := row.route
		if row.path != "" {
			name += " " + row.path
		}
		t.Run(name, func(t *testing.T) {
			fixture, ok := recorded[row.route]
			if !ok {
				t.Fatalf("no recorded body for %s", row.route)
			}
			if fixture.Status < 200 || fixture.Status >= 300 {
				t.Skipf("%s was recorded as a refusal (HTTP %d) in this environment", row.route, fixture.Status)
			}
			body, err := walk(fixture.Response.Result, row.path)
			if err != nil {
				t.Fatalf("%s: %v", row.route, err)
			}

			// The model must decode the recorded body without losing its footing.
			decoded := reflect.New(reflect.TypeOf(row.model))
			if err := json.Unmarshal(body, decoded.Interface()); err != nil {
				t.Fatalf("%T cannot decode the recorded body: %v", row.model, err)
			}

			wire := map[string]bool{}
			var asObject map[string]json.RawMessage
			if err := json.Unmarshal(body, &asObject); err != nil {
				t.Fatalf("the recorded body at %q is not an object: %v", row.path, err)
			}
			for key := range asObject {
				wire[key] = true
			}
			optional := map[string]bool{}
			for _, key := range row.alsoOptional {
				optional[key] = true
			}

			required, known := modelFields(reflect.TypeOf(row.model))
			for key := range wire {
				if !known[key] {
					t.Errorf("the core sends %q on %s but %T has no field for it", key, row.route, row.model)
				}
			}
			for _, key := range required {
				if !wire[key] && !optional[key] {
					t.Errorf("%T declares %q as always present, but %s did not send it", row.model, key, row.route)
				}
			}
		})
	}
}

func TestEveryRecordedSuccessBodyHasAModel(t *testing.T) {
	covered := map[string]bool{}
	for _, row := range modelRows() {
		covered[row.route] = true
	}
	for route, fixture := range fixtures.LoadFixtures(t) {
		if fixture.Status < 200 || fixture.Status >= 300 || notModelled[route] {
			continue
		}
		if !strings.Contains(fixture.Headers["Content-Type"], "json") {
			continue // a document: bytes, not a model
		}
		if !covered[route] {
			t.Errorf("%s has a recorded success body but no model row", route)
		}
	}
}

func TestEnumsCoverWhatTheWireCarries(t *testing.T) {
	recorded := fixtures.LoadFixtures(t)
	vocabulary := func(values any) map[string]bool {
		out := map[string]bool{}
		list := reflect.ValueOf(values)
		for i := 0; i < list.Len(); i++ {
			out[list.Index(i).String()] = true
		}
		return out
	}

	var payments Page[Payment]
	decode(t, recorded["POST /v1/payment/history"].Response.Result, &payments)
	known := vocabulary(PaymentStatuses)
	for _, payment := range payments.Items {
		if !known[string(payment.Status)] {
			t.Errorf("payment status %q is not in the vocabulary", payment.Status)
		}
	}

	var payouts Page[Payout]
	decode(t, recorded["POST /v1/payout/history"].Response.Result, &payouts)
	known = vocabulary(PayoutStatuses)
	for _, payout := range payouts.Items {
		if !known[string(payout.Status)] {
			t.Errorf("payout status %q is not in the vocabulary", payout.Status)
		}
	}

	var links Page[PayoutLink]
	decode(t, recorded["POST /v1/payout/link/list"].Response.Result, &links)
	known = vocabulary(PayoutLinkStatuses)
	for _, link := range links.Items {
		if !known[string(link.Status)] {
			t.Errorf("payout link status %q is not in the vocabulary", link.Status)
		}
	}

	var deliveries Page[WebhookDelivery]
	decode(t, recorded["POST /v1/webhooks/deliveries"].Response.Result, &deliveries)
	known = vocabulary(DeliveryStatuses)
	events := vocabulary(EventTypes)
	for _, delivery := range deliveries.Items {
		if !known[string(delivery.Status)] {
			t.Errorf("delivery status %q is not in the vocabulary", delivery.Status)
		}
		if !events[string(delivery.EventType)] {
			t.Errorf("event type %q is not in the vocabulary", delivery.EventType)
		}
	}
}

func TestRecordedErrorSamplesAreDocumentedCodes(t *testing.T) {
	codes := map[string]bool{}
	for _, code := range ErrorCodes {
		codes[code] = true
	}
	for code, fixture := range fixtures.LoadErrorSamples(t) {
		if !codes[code] {
			t.Errorf("the core emitted %q, which is not in ErrorCodes", code)
		}
		body := fixture.Response.Error
		if body == nil {
			t.Fatalf("%s: the recorded sample carries no error envelope", code)
		}
		var envelopeCode string
		decode(t, body["code"], &envelopeCode)
		if envelopeCode != code {
			t.Errorf("%s: the envelope says %q", code, envelopeCode)
		}
		if _, ok := body["retryable"]; !ok {
			t.Errorf("%s: the envelope has no retryable flag", code)
		}
		if _, ok := body["request_id"]; !ok {
			t.Errorf("%s: the envelope has no request id to quote to support", code)
		}
		// The error decoder must reach the same conclusion from the recorded bytes.
		raw, err := json.Marshal(fixture.Response)
		if err != nil {
			t.Fatal(err)
		}
		_, decoded := decodeEnvelope(fixture.Status, raw, decodeContext{})
		if decoded == nil || decoded.Code != code {
			t.Errorf("%s: the client decoded %v", code, decoded)
		}
		if decoded.Synthetic {
			t.Errorf("%s: an enveloped error must not be classified as synthetic", code)
		}
	}
}

// modelFields returns the json names a struct declares: those that must always be present, and
// the full set. Embedded structs are flattened the way encoding/json flattens them.
func modelFields(t reflect.Type) (required []string, known map[string]bool) {
	known = map[string]bool{}
	var visit func(reflect.Type)
	visit = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("json")
			if field.Anonymous && tag == "" {
				visit(field.Type)
				continue
			}
			parts := strings.Split(tag, ",")
			name := parts[0]
			if name == "" || name == "-" {
				continue
			}
			known[name] = true
			if !slicesContains(parts[1:], "omitempty") {
				required = append(required, name)
			}
		}
	}
	visit(t)
	sort.Strings(required)
	return required, known
}

func slicesContains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// walk follows a dotted path into a JSON document; numeric segments index arrays.
func walk(document json.RawMessage, path string) (json.RawMessage, error) {
	current := document
	if path == "" {
		return current, nil
	}
	for _, segment := range strings.Split(path, ".") {
		if index, err := strconv.Atoi(segment); err == nil {
			var list []json.RawMessage
			if err := json.Unmarshal(current, &list); err != nil {
				return nil, fmt.Errorf("%q: not an array at %q", path, segment)
			}
			if index >= len(list) {
				return nil, fmt.Errorf("%q: the recording has no element %d", path, index)
			}
			current = list[index]
			continue
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(current, &object); err != nil {
			return nil, fmt.Errorf("%q: not an object at %q", path, segment)
		}
		next, ok := object[segment]
		if !ok {
			return nil, fmt.Errorf("%q: the recording has no field %q", path, segment)
		}
		current = next
	}
	return current, nil
}

func decode(t *testing.T, raw json.RawMessage, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("cannot decode %T: %v", into, err)
	}
}
