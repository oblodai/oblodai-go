package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oblodai/oblodai-go/v2"
	"github.com/oblodai/oblodai-go/v2/webhooks"
)

// The receiver runs against signed deliveries: a genuine one is acknowledged, a repeat of the same
// state too (and not acted on twice), a forged one is refused.
func TestWebhookReceiver(t *testing.T) {
	now := time.Now().Unix()
	serve := handler(webhooks.Options{Secret: "whsec"}, newSeen())
	deliver := func(body, secret, eventID string) int {
		r := httptest.NewRequest(http.MethodPost, "/oblodai/webhook", strings.NewReader(body))
		r.Header.Set(webhooks.HeaderTimestamp, strconv.FormatInt(now, 10))
		r.Header.Set(webhooks.HeaderSignature, oblodai.SignWebhook(secret, now, []byte(body)))
		r.Header.Set(webhooks.HeaderEventID, eventID)
		w := httptest.NewRecorder()
		serve(w, r)
		return w.Code
	}
	paid := `{"type":"payment","uuid":"u1","order_id":"o1","status":"paid","sequence":3,"payment_amount":"25","payer_currency":"USDT"}`
	if code := deliver(paid, "whsec", "e1"); code != http.StatusOK {
		t.Fatalf("genuine delivery: %d", code)
	}
	if code := deliver(paid, "whsec", "e1"); code != http.StatusOK {
		t.Fatalf("repeat: %d", code)
	}
	if code := deliver(paid, "forged", "e2"); code != http.StatusBadRequest {
		t.Fatalf("forged delivery: %d", code)
	}
	if code := deliver(`{"type":"payment","uuid":"u1","sequence":"x"}`, "whsec", "e3"); code != http.StatusInternalServerError {
		t.Fatalf("authentic but unreadable delivery: %d", code)
	}
}

// Ordering is per object: a conversion B arriving after conversion A with a lower sequence is B's
// first state, not a straggler of A; a stale repeat of A is. Kinds this release does not know have
// no known object id and are never ordered against each other.
func TestWebhookReceiverOrdersPerObject(t *testing.T) {
	s := newSeen()
	handled := func(body, eventID string) bool {
		event, err := webhooks.Parse([]byte(body))
		if err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		return s.alreadyHandled(&webhooks.Delivery{Event: event, EventID: eventID})
	}
	if handled(`{"type":"conversion","id":"A","status":"completed","sequence":5}`, "e1") {
		t.Fatal("conversion A is new")
	}
	if handled(`{"type":"conversion","id":"B","status":"completed","sequence":3}`, "e2") {
		t.Fatal("conversion B with a lower sequence than A was dropped as stale")
	}
	if !handled(`{"type":"conversion","id":"A","status":"refunded","sequence":4}`, "e3") {
		t.Fatal("an older state of conversion A must be stale")
	}
	if handled(`{"type":"refund","refund_id":"r1","sequence":9}`, "e4") ||
		handled(`{"type":"refund","refund_id":"r2","sequence":1}`, "e5") {
		t.Fatal("events of an unknown kind must not be ordered against each other")
	}
}
