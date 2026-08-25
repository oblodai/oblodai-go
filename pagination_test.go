package oblodai

import (
	"context"
	"testing"
)

func TestPageReturnsTheFirstPageAndPagerWalksThemAll(t *testing.T) {
	item := func(uuid string) any { return map[string]any{"uuid": uuid} }
	api := newFakeAPI(t,
		pageOf([]any{item("a"), item("b")}, 0, 5, 2),
		pageOf([]any{item("a"), item("b")}, 0, 5, 2),
		pageOf([]any{item("c"), item("d")}, 2, 5, 2),
		pageOf([]any{item("e")}, 4, 5, 2),
	)
	client := api.client()
	ctx := context.Background()
	limit := 2

	first, err := client.Payments.History(ctx, PaymentHistoryParams{Limit: &limit}).Page()
	if err != nil {
		t.Fatalf("Payments.History: %v", err)
	}
	if len(first.Items) != 2 || first.Items[0].UUID != "a" || !first.Paginate.HasPages {
		t.Fatalf("unexpected first page: %+v", first)
	}
	if api.count() != 1 {
		t.Fatalf("expected 1 request, saw %d", api.count())
	}

	var seen []string
	pager := client.Payments.History(ctx, PaymentHistoryParams{Limit: &limit}).Pager()
	for pager.Next() {
		seen = append(seen, pager.Item().UUID)
	}
	if err := pager.Err(); err != nil {
		t.Fatalf("pager: %v", err)
	}
	if got := len(seen); got != 5 {
		t.Fatalf("walked %d items (%v), want 5", got, seen)
	}
	if api.count() != 4 {
		t.Fatalf("expected 4 requests in total, saw %d", api.count())
	}
	// Each page asks for the next offset with the caller's page size.
	body := api.at(2).jsonBody(t)
	if body["limit"] != float64(2) || body["offset"] != float64(2) {
		t.Fatalf("second page asked for %v", body)
	}
}

func TestAllCollectsWithACap(t *testing.T) {
	item := func(uuid string) any { return map[string]any{"uuid": uuid} }
	api := newFakeAPI(t,
		pageOf([]any{item("a"), item("b")}, 0, 3, 2),
		pageOf([]any{item("c")}, 2, 3, 2),
	)
	limit := 2
	all, err := api.client().Payouts.History(context.Background(), PayoutHistoryParams{Limit: &limit}).All(0)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("collected %d items, want 3", len(all))
	}

	capped := newFakeAPI(t, pageOf([]any{item("a"), item("b")}, 0, 3, 2))
	some, err := capped.client().Payouts.History(context.Background(), PayoutHistoryParams{Limit: &limit}).All(2)
	if err != nil {
		t.Fatalf("All(2): %v", err)
	}
	if len(some) != 2 || capped.count() != 1 {
		t.Fatalf("a capped walk must stop at the cap: %d items in %d requests", len(some), capped.count())
	}
}

func TestPagerStopsOnAShortPageAndReportsErrors(t *testing.T) {
	item := func(uuid string) any { return map[string]any{"uuid": uuid} }
	// has_pages is true but the page is empty: the walk must end rather than loop for ever.
	api := newFakeAPI(t,
		ok(map[string]any{
			"items":    []any{item("a")},
			"paginate": map[string]any{"total": 99, "per_page": 1, "offset": 0, "has_pages": true},
		}),
		ok(map[string]any{
			"items":    []any{},
			"paginate": map[string]any{"total": 99, "per_page": 1, "offset": 1, "has_pages": true},
		}),
	)
	pager := api.client().Payments.History(context.Background(), PaymentHistoryParams{}).Pager()
	count := 0
	for pager.Next() {
		count++
	}
	if pager.Err() != nil || count != 1 {
		t.Fatalf("walked %d items, err %v", count, pager.Err())
	}

	failing := newFakeAPI(t, apiError(503, map[string]any{"code": "db.unavailable", "retryable": false}))
	broken := failing.client().Payments.History(context.Background(), PaymentHistoryParams{}).Pager()
	if broken.Next() {
		t.Fatal("a failing page must not yield an item")
	}
	if !IsCode(broken.Err(), "db.unavailable") {
		t.Fatalf("the failure must be reported by Err, got %v", broken.Err())
	}
}

func TestListDefaultsToTheDocumentedPageSize(t *testing.T) {
	api := newFakeAPI(t, emptyPage())
	if _, err := api.client().Payments.History(context.Background(), PaymentHistoryParams{}).Page(); err != nil {
		t.Fatalf("Payments.History: %v", err)
	}
	if got := api.last().jsonBody(t)["limit"]; got != float64(DefaultPageLimit) {
		t.Fatalf("limit = %v, want %d", got, DefaultPageLimit)
	}
}

func TestListParametersSurviveEveryPage(t *testing.T) {
	api := newFakeAPI(t, emptyPage())
	limit := 5
	list := api.client().Payouts.History(context.Background(), PayoutHistoryParams{
		Kind: "refund", Status: PayoutStatusConfirmed, Limit: &limit,
	})
	if _, err := list.Page(); err != nil {
		t.Fatalf("Payouts.History: %v", err)
	}
	body := api.last().jsonBody(t)
	if body["kind"] != "refund" || body["status"] != string(PayoutStatusConfirmed) || body["limit"] != float64(5) {
		t.Fatalf("filters were lost: %v", body)
	}
}
