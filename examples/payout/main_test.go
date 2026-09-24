package main

import (
	"context"
	"slices"
	"testing"

	"github.com/oblodai/oblodai-go/v2"
	"github.com/oblodai/oblodai-go/v2/internal/fakeapi"
)

// The example runs end to end against a fake gateway: validate, then create with the caller's
// own idempotency key.
func TestPayout(t *testing.T) {
	api := fakeapi.New(t, map[string]any{
		"validatePayout": map[string]any{"valid": true, "payer_amount": "10.5", "currency": "USDT"},
		"createPayout":   map[string]any{"uuid": "p1", "status": "pending"},
	})
	if err := run(context.Background(), api.Client(t)); err != nil {
		t.Fatal(err)
	}
	if got := api.Operations(); !slices.Equal(got, []string{"validatePayout", "createPayout"}) {
		t.Fatalf("operations %v", got)
	}
	create := api.Requests()[1]
	if create.Header.Get(oblodai.HeaderIdempotencyKey) == "" || create.Body["order_id"] != "payout-42" {
		t.Fatalf("create = %v %v", create.Header, create.Body)
	}
}
