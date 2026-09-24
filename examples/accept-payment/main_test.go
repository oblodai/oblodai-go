package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/oblodai/oblodai-go/v2/internal/fakeapi"
)

// The example runs end to end against a fake gateway: the invoice is created, polled while it is
// open and reported once it is paid.
func TestAcceptPayment(t *testing.T) {
	api := fakeapi.New(t, map[string]any{
		"createPayment":  map[string]any{"uuid": "u1", "status": "created", "url": "https://pay.example/u1"},
		"getPaymentInfo": []any{map[string]any{"uuid": "u1", "status": "confirm_check"}, map[string]any{"uuid": "u1", "status": "paid", "amount_paid": "25"}},
	})
	if err := run(context.Background(), api.Client(t), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	want := []string{"createPayment", "getPaymentInfo", "getPaymentInfo"}
	if got := api.Operations(); !slices.Equal(got, want) {
		t.Fatalf("operations %v, want %v", got, want)
	}
	if body := api.Requests()[0].Body; body["amount"] != "25" || body["network"] != "tron" {
		t.Fatalf("create body %v", body)
	}
	if body := api.Requests()[1].Body; body["uuid"] != "u1" {
		t.Fatalf("poll body %v", body)
	}
}
