// Validate a payout first (free, no side effects), then send it with your own idempotency key so a
// crash between the two lines cannot pay twice.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/oblodai/oblodai-go"
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

	const orderID = "payout-42" // your reference: the core deduplicates by it as well
	params := oblodai.PayoutParams{
		Amount:   "10",
		Currency: "USDT",
		Network:  oblodai.NetworkTron,
		Address:  "TQrY8bkbpXKPt2LZbU8jqfnpFbUSF15sbx",
		OrderID:  orderID,
	}

	check, err := client.Payouts.Validate(ctx, oblodai.PayoutValidateParams{
		Amount: params.Amount, Currency: params.Currency, Network: params.Network,
		Address: params.Address, OrderID: params.OrderID,
	})
	if err != nil {
		log.Fatalf("the payout would fail: %v", err)
	}
	fmt.Printf("will debit %s %s (fee %s, borne by %s)\n",
		check.PayerAmount, check.Currency, check.Commission, check.FeeBearer)

	// Store this key next to the order: reusing it after a restart replays the first answer
	// instead of creating a second payout.
	key, err := oblodai.NewIdempotencyKey()
	if err != nil {
		log.Fatal(err)
	}
	payout, err := client.Payouts.Create(ctx, params, oblodai.WithIdempotencyKey(key))
	if err != nil {
		var apiErr *oblodai.Error
		if errors.As(err, &apiErr) && apiErr.Retryable {
			// The client already retried what was safe to retry; this needs a later attempt with
			// the SAME key — the balance may still arrive.
			log.Fatalf("try again later (%s): %v", apiErr.Code, err)
		}
		log.Fatalf("payout refused: %v", err)
	}
	fmt.Printf("payout %s is %s\n", payout.UUID, payout.Status)
}
