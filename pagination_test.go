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
	limit := int64(2)

	first, err := client.Payments.ListHistory(ctx, &HistoryRequest{Limit: &limit}).Page()
	if err != nil {
		t.Fatalf("Payments.ListHistory: %v", err)
	}
	if len(first.Items) != 2 || first.Items[0].UUID != "a" || !first.Paginate.HasPages {
		t.Fatalf("unexpected first page: %+v", first)
	}
	if api.count() != 1 {
		t.Fatalf("expected 1 request, saw %d", api.count())
	}

	var seen []string
	pager := client.Payments.ListHistory(ctx, &HistoryRequest{Limit: &limit}).Pager()
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

func TestCollectGathersWithACap(t *testing.T) {
	item := func(uuid string) any { return map[string]any{"uuid": uuid} }
	api := newFakeAPI(t,
		pageOf([]any{item("a"), item("b")}, 0, 3, 2),
		pageOf([]any{item("c")}, 2, 3, 2),
	)
	limit := int64(2)
	all, err := api.client().Payouts.ListHistory(context.Background(), &HistoryRequest{Limit: &limit}).Collect(0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("collected %d items, want 3", len(all))
	}

	capped := newFakeAPI(t, pageOf([]any{item("a"), item("b")}, 0, 3, 2))
	some, err := capped.client().Payouts.ListHistory(context.Background(), &HistoryRequest{Limit: &limit}).Collect(2)
	if err != nil {
		t.Fatalf("Collect(2): %v", err)
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
	pager := api.client().Payments.ListHistory(context.Background(), nil).Pager()
	count := 0
	for pager.Next() {
		count++
	}
	if pager.Err() != nil || count != 1 {
		t.Fatalf("walked %d items, err %v", count, pager.Err())
	}

	failing := newFakeAPI(t, apiError(503, map[string]any{"code": "db.unavailable", "retryable": false}))
	broken := failing.client().Payments.ListHistory(context.Background(), nil).Pager()
	if broken.Next() {
		t.Fatal("a failing page must not yield an item")
	}
	if !IsCode(broken.Err(), "db.unavailable") {
		t.Fatalf("the failure must be reported by Err, got %v", broken.Err())
	}
}

func TestListDefaultsToTheDocumentedPageSize(t *testing.T) {
	api := newFakeAPI(t, emptyPage())
	if _, err := api.client().Payments.ListHistory(context.Background(), nil).Page(); err != nil {
		t.Fatalf("Payments.ListHistory: %v", err)
	}
	if got := api.last().jsonBody(t)["limit"]; got != float64(DefaultPageLimit) {
		t.Fatalf("limit = %v, want %d", got, DefaultPageLimit)
	}
}

func TestListParametersSurviveEveryPage(t *testing.T) {
	api := newFakeAPI(t, emptyPage())
	limit := int64(5)
	list := api.client().Payouts.ListHistory(context.Background(), &HistoryRequest{
		Kind: Ptr(PayoutKindRefund), Status: Ptr(string(PayoutStatusConfirmed)), Limit: &limit,
	})
	if _, err := list.Page(); err != nil {
		t.Fatalf("Payouts.ListHistory: %v", err)
	}
	body := api.last().jsonBody(t)
	if body["kind"] != "refund" || body["status"] != string(PayoutStatusConfirmed) || body["limit"] != float64(5) {
		t.Fatalf("filters were lost: %v", body)
	}
}

func TestItemsRangesOverEveryItemAcrossPages(t *testing.T) {
	item := func(uuid string) any { return map[string]any{"uuid": uuid} }
	api := newFakeAPI(t,
		pageOf([]any{item("a"), item("b")}, 0, 3, 2),
		pageOf([]any{item("c")}, 2, 3, 2),
	)
	var seen []string
	for payment, err := range api.client().Payments.ListHistory(context.Background(), nil).Items() {
		if err != nil {
			t.Fatalf("Items: %v", err)
		}
		seen = append(seen, payment.UUID)
	}
	if len(seen) != 3 || seen[2] != "c" || api.count() != 2 {
		t.Fatalf("saw %v in %d requests", seen, api.count())
	}

	// Breaking out early fetches no further page.
	early := newFakeAPI(t, pageOf([]any{item("a"), item("b")}, 0, 3, 2))
	for range early.client().Payments.ListHistory(context.Background(), nil).Items() {
		break
	}
	if early.count() != 1 {
		t.Fatalf("an early break fetched %d pages", early.count())
	}

	// An error ends the range with the error.
	failing := newFakeAPI(t, apiError(404, map[string]any{"code": "payment.not_found", "retryable": false}))
	var last error
	n := 0
	for _, err := range failing.client().Payments.ListHistory(context.Background(), nil).Items() {
		n++
		last = err
	}
	if n != 1 || !IsCode(last, "payment.not_found") {
		t.Fatalf("%d yields, last error %v", n, last)
	}
}

func TestByPageRangesOverPagesOneRequestEach(t *testing.T) {
	item := func(uuid string) any { return map[string]any{"uuid": uuid} }
	api := newFakeAPI(t,
		pageOf([]any{item("a"), item("b")}, 0, 3, 2),
		pageOf([]any{item("c")}, 2, 3, 2),
	)
	var sizes []int
	for page, err := range api.client().Payments.ListHistory(context.Background(), &HistoryRequest{Limit: Ptr(int64(2))}).ByPage() {
		if err != nil {
			t.Fatalf("ByPage: %v", err)
		}
		sizes = append(sizes, len(page.Items))
		if page.Paginate.Total != 3 {
			t.Fatalf("paginate = %+v", page.Paginate)
		}
	}
	if len(sizes) != 2 || sizes[0] != 2 || sizes[1] != 1 || api.count() != 2 {
		t.Fatalf("pages %v in %d requests", sizes, api.count())
	}
	if body := api.at(1).jsonBody(t); body["offset"] != float64(2) || body["limit"] != float64(2) {
		t.Fatalf("second page asked for %v", body)
	}
}

// A GET list pages through its query: the caller's filters stay, limit and offset advance.
func TestAGETListPagesThroughTheQuery(t *testing.T) {
	item := func(id string) any { return map[string]any{"id": id} }
	api := newFakeAPI(t,
		pageOf([]any{item("d1")}, 0, 2, 1),
		pageOf([]any{item("d2")}, 1, 2, 1),
	)
	all, err := api.client().Sandbox.ListWebhooks(context.Background(), &SandboxListWebhooksParams{Limit: Ptr(int64(1))}).Collect(0)
	if err != nil || len(all) != 2 {
		t.Fatalf("Collect: %d items, %v", len(all), err)
	}
	if got := api.at(1).rawQuery; got != "limit=1&offset=1" {
		t.Fatalf("second page query = %q", got)
	}
	if api.at(1).body != "" {
		t.Fatal("a GET list page carries no body")
	}
}
