package oblodai

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// Invoke lets a caller drive an operation by its operationId — the contract, not a typed method —
// through the same transport a generated method uses: signing, idempotency, retries, error
// classification. These tests exercise the transport wiring and the three failure modes that must
// be caught before any request is sent.

func TestInvokeSignsAndDecodes(t *testing.T) {
	f := newFakeAPI(t, ok(map[string]any{"uuid": "p1", "status": "paid"}))
	res, err := f.client().Invoke(context.Background(), "createPayment",
		InvokeInput{Body: json.RawMessage(`{"amount":"10","currency":"USDT","order_id":"o1"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res), `"p1"`) {
		t.Fatalf("result %s", res)
	}
	r := f.last()
	if r.method != "POST" || r.path != Routes["createPayment"].Path {
		t.Fatalf("%s %s", r.method, r.path)
	}
	if r.header.Get(HeaderSignature) == "" || r.header.Get(HeaderIdempotencyKey) == "" {
		t.Fatal("not signed/idempotent")
	}
}

func TestInvokeUnknownOperation(t *testing.T) {
	f := newFakeAPI(t)
	_, err := f.client().Invoke(context.Background(), "noSuchOp", InvokeInput{})
	_ = requireCode(t, err, CodeUnknownOperation)
	if f.count() != 0 {
		t.Fatal("went to network")
	}
}

func TestInvokeWrongKind(t *testing.T) {
	var paged string
	for id, r := range Routes {
		if r.ListKind == ListPaged {
			paged = id
			break
		}
	}
	_, err := newFakeAPI(t).client().Invoke(context.Background(), paged, InvokeInput{})
	_ = requireCode(t, err, CodeWrongInvoke)
}

func TestInvokeListPages(t *testing.T) {
	var paged string
	for id, r := range Routes {
		if r.ListKind == ListPaged && r.Method == "GET" {
			paged = id
			break
		}
	}
	f := newFakeAPI(t, pageOf([]any{map[string]any{"uuid": "a"}}, 0, 2, 1), pageOf([]any{map[string]any{"uuid": "b"}}, 1, 2, 1))
	var got []string
	for item, err := range f.client().InvokeList(context.Background(), paged, InvokeInput{Query: url.Values{"limit": {"1"}}}).Items() {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, string(item))
	}
	if len(got) != 2 {
		t.Fatalf("%v", got)
	}
}
