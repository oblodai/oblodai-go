// Create an invoice, show the payer where to send the money, then poll until it is final.
//
// Webhooks are the right way to learn about a state change; polling is the fallback this example
// uses so it stays a single file with no inbound network.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/oblodai/oblodai-go/v2"
)

func main() {
	// Credentials come from OBLODAI_PUBLIC_ID and OBLODAI_SECRET.
	client, err := oblodai.New()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := run(ctx, client, 10*time.Second); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, client *oblodai.Client, every time.Duration) error {
	invoice, err := client.Payments.Create(ctx, &oblodai.PaymentRequest{
		Amount:     "25", // a decimal string: never a float
		Currency:   "USDT",
		Network:    oblodai.Ptr("tron"), // omit to let the payer choose on the pay page
		OrderID:    oblodai.Ptr(fmt.Sprintf("order-%d", time.Now().Unix())),
		URLSuccess: oblodai.Ptr("https://shop.example/thanks"),
	})
	if err != nil {
		return fmt.Errorf("could not open the invoice: %w", err)
	}
	fmt.Printf("pay at %s\nor send %s %s to %s\n",
		invoice.URL, invoice.PayerAmount, invoice.PayerCurrency, invoice.Address)

	status := invoice.Status
	var current *oblodai.PaymentInfoResult
	for current == nil || !oblodai.IsPaymentFinal(status) {
		time.Sleep(every)
		current, err = client.Payments.GetInfo(ctx, &oblodai.LookupRequest{UUID: &invoice.UUID})
		if err != nil {
			return fmt.Errorf("could not read the invoice: %w", err)
		}
		status = current.Status
	}

	switch {
	case oblodai.IsPaymentPaid(status):
		fmt.Printf("paid %s %s\n", current.AmountPaid, current.PayerCurrency)
	case oblodai.IsPaymentUnderpaid(status):
		// The payer sent less than the invoice asked for: keep it or send it back.
		fmt.Printf("underpaid: %s of %s — resolve it with Payments.Resolve\n",
			current.AmountPaid, current.PayerAmount)
	default:
		fmt.Printf("ended as %s\n", status)
	}
	return nil
}
