// A webhook receiver: verify every delivery over the raw bytes, deduplicate by event id, ignore
// events that arrive out of order, and never act on a rehearsal (test) delivery as if money moved.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/oblodai/oblodai-go/v2"
	"github.com/oblodai/oblodai-go/v2/webhooks"
)

// seen is a stand-in for your database: deliveries are retried and states resent, so the same
// event can arrive more than once, and a retried older event can arrive after a newer one.
type seen struct {
	mu       sync.Mutex
	handled  map[string]bool
	sequence map[string]int64
}

func main() {
	options := webhooks.Options{
		Secret: os.Getenv("OBLODAI_WEBHOOK_SECRET"),
		// During a rotation keep the outgoing secret here for at least 26 hours: deliveries queued
		// before the rotation stay signed with it for their whole retry life.
		PreviousSecret: os.Getenv("OBLODAI_WEBHOOK_SECRET_PREVIOUS"),
	}
	http.Handle("/oblodai/webhook", handler(options, newSeen()))
	log.Fatal(http.ListenAndServe(":8096", nil))
}

func newSeen() *seen {
	return &seen{handled: map[string]bool{}, sequence: map[string]int64{}}
}

func handler(options webhooks.Options, store *seen) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		delivery, err := webhooks.VerifyRequest(r, options)
		if err != nil {
			// Never act on a body that did not verify. An authentic body that could not be read
			// (webhook.bad_payload) gets a 5xx so the gateway retries it.
			log.Printf("rejected a delivery: %v", err)
			status := http.StatusBadRequest
			if oblodai.IsWebhookPayload(err) {
				status = http.StatusInternalServerError
			}
			http.Error(w, "rejected", status)
			return
		}
		if delivery.IsTest {
			// A rehearsal delivery (webhook tests, sandbox): signed like a live one, but no money
			// moved. Acknowledge it and let it touch no order and no balance.
			log.Printf("test delivery %s (%s): acknowledged, not acted on", delivery.ID, delivery.EventType)
			w.WriteHeader(http.StatusOK)
			return
		}
		if store.alreadyHandled(delivery) {
			w.WriteHeader(http.StatusOK) // acknowledge, do nothing
			return
		}

		event := delivery.Event
		switch {
		case event.Payment != nil:
			if oblodai.IsPaymentPaid(event.Payment.Status) {
				fmt.Printf("order %s paid: %s %s\n", event.Payment.OrderID, event.Payment.PaymentAmount, event.Payment.PayerCurrency)
			}
		case event.Payout != nil:
			fmt.Printf("payout %s is %s\n", event.Payout.UUID, event.Payout.Status)
		case event.Wallet != nil:
			fmt.Printf("wallet %s received %s %s\n", event.Wallet.Address, event.Wallet.PaymentAmount, event.Wallet.PayerCurrency)
		default:
			fmt.Printf("event %s of a kind this release does not model: acknowledged\n", event.Type)
		}

		// Answer 2xx quickly; do the slow work elsewhere, or the gateway will retry.
		w.WriteHeader(http.StatusOK)
	}
}

// alreadyHandled reports whether this delivery's state was handled before (by X-Webhook-Event-Id,
// else the delivery id), or carries an event older than the last one processed for its object.
func (s *seen) alreadyHandled(delivery *webhooks.Delivery) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := delivery.EventID
	if key == "" {
		key = delivery.ID
	}
	if key != "" && s.handled[key] {
		return true
	}
	event := delivery.Event
	if webhooks.IsStale(event, s.sequence[event.ID()]) {
		return true
	}
	s.handled[key] = true
	s.sequence[event.ID()] = event.Sequence()
	return false
}
