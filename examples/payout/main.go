// Validate a payout first (no side effects), then send it with your own idempotency key so a
// crash between the two lines cannot pay twice.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/oblodai/oblodai-go/v2"
)

func main() {
	// One API key signs everything, payouts included.
	client, err := oblodai.New(oblodai.WithCredentials(
		os.Getenv("OBLODAI_PUBLIC_ID"), os.Getenv("OBLODAI_SECRET")))
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := run(ctx, client); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, client *oblodai.Client) error {
	const orderID = "payout-42" // your reference: the core deduplicates by it as well
	params := &oblodai.PayoutRequest{
		Amount:   "10",
		Currency: "USDT",
		Network:  oblodai.Ptr("tron"),
		Address:  "TQrY8bkbpXKPt2LZbU8jqfnpFbUSF15sbx",
		OrderID:  orderID,
	}

	check, err := client.Payouts.Validate(ctx, &oblodai.PayoutValidateRequest{
		Amount: params.Amount, Currency: params.Currency, Network: params.Network,
		Address: params.Address, OrderID: &params.OrderID,
	})
	if err != nil {
		return fmt.Errorf("the payout would fail: %w", err)
	}
	fmt.Printf("will debit %s %s (fee %s, borne by %s)\n",
		check.PayerAmount, check.Currency, check.Commission, check.FeeBearer)

	// Store this key next to the order: reusing it after a restart replays the first answer
	// instead of creating a second payout.
	key, err := oblodai.NewIdempotencyKey()
	if err != nil {
		return err
	}
	payout, err := client.Payouts.Create(ctx, params, oblodai.WithIdempotencyKey(key))
	if err != nil {
		var apiErr *oblodai.Error
		if errors.As(err, &apiErr) && apiErr.Retryable {
			// The client already retried what was safe to retry; this needs a later attempt with
			// the SAME key — the balance may still arrive.
			return fmt.Errorf("try again later (%s): %w", apiErr.Code, err)
		}
		return fmt.Errorf("payout refused: %w", err)
	}
	fmt.Printf("payout %s is %s\n", payout.UUID, payout.Status)
	return nil
}
