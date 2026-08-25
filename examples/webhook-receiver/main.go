// A webhook receiver: verify every delivery over the raw bytes, deduplicate by delivery id, and
// ignore events that arrive out of order.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/oblodai/oblodai-go"
	"github.com/oblodai/oblodai-go/webhooks"
)

// seen is a stand-in for your database: deliveries are retried, so the same event id can arrive
// more than once, and a retried older event can arrive after a newer one.
type seen struct {
	mu        sync.Mutex
	delivered map[string]bool
	sequence  map[string]int64
}

func main() {
	options := webhooks.Options{
		Secret: os.Getenv("OBLODAI_WEBHOOK_SECRET"),
		// During a rotation keep the outgoing secret here for at least 26 hours: deliveries queued
		// before the rotation stay signed with it for their whole retry life.
		PreviousSecret: os.Getenv("OBLODAI_WEBHOOK_SECRET_PREVIOUS"),
	}
	store := &seen{delivered: map[string]bool{}, sequence: map[string]int64{}}

	http.HandleFunc("/oblodai/webhook", func(w http.ResponseWriter, r *http.Request) {
		delivery, err := webhooks.VerifyRequest(r, options)
		if err != nil {
			// Never act on a body that did not verify.
			log.Printf("rejected a delivery: %v", err)
			http.Error(w, "bad signature", http.StatusBadRequest)
			return
		}
		if store.alreadyHandled(delivery.ID, delivery.Event) {
			w.WriteHeader(http.StatusOK) // acknowledge, do nothing
			return
		}

		switch event := delivery.Event.(type) {
		case *oblodai.PaymentEvent:
			if event.Status == oblodai.PaymentStatusPaid || event.Status == oblodai.PaymentStatusPaidOver {
				fmt.Printf("order %v paid: %s %s\n", event.OrderID, event.PaymentAmount, event.PayerCurrency)
			}
		case *oblodai.PayoutEvent:
			fmt.Printf("payout %s is %s\n", event.UUID, event.Status)
		case *oblodai.WalletEvent:
			fmt.Printf("wallet %s received %s %s\n", event.Address, event.PaymentAmount, event.PayerCurrency)
		}

		// Answer 2xx quickly; do the slow work elsewhere, or the gateway will retry.
		w.WriteHeader(http.StatusOK)
	})

	log.Fatal(http.ListenAndServe(":8096", nil))
}

// alreadyHandled reports whether this delivery was seen before, or carries an event older than the
// last one processed for the same object.
func (s *seen) alreadyHandled(id string, event oblodai.WebhookEvent) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != "" && s.delivered[id] {
		return true
	}
	if webhooks.IsStale(event, s.sequence[event.ID()]) {
		return true
	}
	s.delivered[id] = true
	s.sequence[event.ID()] = event.Seq()
	return false
}
