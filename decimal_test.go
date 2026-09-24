package oblodai

import (
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
