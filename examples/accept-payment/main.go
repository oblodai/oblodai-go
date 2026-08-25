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

	"github.com/oblodai/oblodai-go"
)

func main() {
	// Credentials come from OBLODAI_PUBLIC_ID and OBLODAI_SECRET.
	client, err := oblodai.New()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	invoice, err := client.Payments.Create(ctx, oblodai.PaymentParams{
		Amount:     "25", // a decimal string: never a float
		Currency:   "USDT",
		Network:    oblodai.NetworkTron, // omit to let the payer choose on the pay page
		OrderID:    fmt.Sprintf("order-%d", time.Now().Unix()),
		URLSuccess: "https://shop.example/thanks",
	})
	if err != nil {
		log.Fatalf("could not open the invoice: %v", err)
	}
	fmt.Printf("pay at %s\nor send %s %s to %s\n",
		invoice.URL, invoice.PayerAmount, invoice.PayerCurrency, invoice.Address)

	current := invoice
	for !oblodai.IsPaymentFinal(current.Status) {
		time.Sleep(10 * time.Second)
		current, err = client.Payments.Info(ctx, oblodai.PaymentInfoParams{UUID: invoice.UUID})
		if err != nil {
			log.Fatalf("could not read the invoice: %v", err)
		}
	}

	switch {
	case oblodai.IsPaymentPaid(current.Status):
		fmt.Printf("paid %s %s\n", current.AmountPaid, current.PayerCurrency)
	case oblodai.IsPaymentUnderpaid(current.Status):
		// The payer sent less than the invoice asked for: keep it or send it back.
		fmt.Printf("underpaid: %s of %s — resolve it with Refunds.Resolve\n",
			current.AmountPaid, current.PayerAmount)
	default:
		fmt.Printf("ended as %s\n", current.Status)
	}
}
