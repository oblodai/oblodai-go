package oblodai

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

// Amounts are Decimal strings. A float cannot be passed where an amount goes — the signature does
// not take one — and a JSON number decoded into an amount is refused before anything is sent.

func TestDecimalTakesNoFloat(t *testing.T) {
	dec := reflect.TypeFor[Decimal]()
	for _, f := range []reflect.Type{reflect.TypeFor[float64](), reflect.TypeFor[float32]()} {
		if f.AssignableTo(dec) || f.ConvertibleTo(dec) {
			t.Errorf("%s fits Decimal: an amount must never be a float", f)
		}
	}
	if dec.Kind() != reflect.String {
		t.Fatalf("Decimal is %s, want a string type", dec.Kind())
	}
}

func TestDecimalJSONIsAStringOnTheWire(t *testing.T) {
	out, err := json.Marshal(PaymentRequest{Amount: "25.10", Currency: "USDT"})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(out, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["amount"] != "25.10" {
		t.Fatalf("amount on the wire = %#v, want the string \"25.10\" verbatim", wire["amount"])
	}
	var back PaymentRequest
	if err := json.Unmarshal(out, &back); err != nil || back.Amount != "25.10" {
		t.Fatalf("round trip: %q, %v", back.Amount, err)
	}
}

func TestDecimalRefusesAJSONNumber(t *testing.T) {
	for _, body := range []string{`{"amount": 25.5, "currency": "USDT"}`, `{"amount": 25, "currency": "USDT"}`, `{"amount": 1e3}`} {
		var req PaymentRequest
		err := json.Unmarshal([]byte(body), &req)
		e := requireCode(t, err, CodeFloatAmount)
		if e.Kind != KindConfig || e.Field != "amount" && e.Field != "" {
			t.Errorf("%s: %+v", body, e)
		}
	}
	var d Decimal = "7"
	if err := json.Unmarshal([]byte(`null`), &d); err != nil || d != "7" {
		t.Fatalf("null must leave the amount alone: %q, %v", d, err)
	}
}

func TestMoneyHelpersTakeDecimal(t *testing.T) {
	sum, err := AddAmounts("1.5", Decimal("2"))
	if err != nil || sum != Decimal("3.5") {
		t.Fatalf("AddAmounts = %q, %v", sum, err)
	}
	view := PaymentView{Amount: "25", MerchantAmount: "25.000000"}
	if !AmountsEqual(view.Amount, view.MerchantAmount) { // Decimal and string alike
		t.Fatal("25 and 25.000000 are the same amount")
	}
	if _, err := AddAmounts("1,5", "1"); err == nil {
		t.Fatal("a malformed amount must be refused")
	}
}

// A free-form member (a model's Extra, a map[string]any) can carry a float where the contract has
// no float: it is refused before anything is sent. The numeric fields the contract declares are
// not money (generated nonMoneyNumbers) go out as numbers.
func TestAFloatInAFreeFormMemberIsRefusedBeforeTheNetwork(t *testing.T) {
	api := newFakeAPI(t, ok(map[string]any{"uuid": "p1"}))
	client := api.client()
	ctx := context.Background()
	_, err := client.Payouts.Create(ctx, &PayoutRequest{Amount: "1", Currency: "USDT",
		Extra: map[string]json.RawMessage{"convert": json.RawMessage(`{"to_amount": 10.5}`)}})
	e := requireCode(t, err, CodeFloatAmount)
	if !IsConfig(err) || e.Field != "to_amount" {
		t.Fatalf("expected a config error on to_amount, got %+v", e)
	}
	_, err = client.Payments.Create(ctx, &PaymentRequest{Amount: "1", Currency: "USDT",
		Extra: map[string]json.RawMessage{"tip": json.RawMessage(`1e2`)}})
	_ = requireCode(t, err, CodeFloatAmount)
	if api.count() != 0 {
		t.Fatalf("a float must be refused before anything is sent, saw %d requests", api.count())
	}

	if _, err := client.Payments.Create(ctx, &PaymentRequest{Amount: "1", Currency: "USDT",
		AccuracyPaymentPercent: Ptr(2.5), Extra: map[string]json.RawMessage{"count": json.RawMessage(`3`)}}); err != nil {
		t.Fatalf("a non-money number and an integer go out: %v", err)
	}
	if got := api.at(0).jsonBody(t)["accuracy_payment_percent"]; got != 2.5 {
		t.Fatalf("accuracy_payment_percent %v", got)
	}
	if !nonMoneyNumbers["accuracy_payment_percent"] {
		t.Fatal("nonMoneyNumbers is generated from the contract's number fields")
	}
}
