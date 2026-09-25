package oblodai

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/url"
	"strconv"
	"sync"
)

// Offset pagination over the core's {items, paginate} lists. paginate.has_pages is the server's
// own "there is more" flag; iteration stops on it, or on a short page, whichever comes first.
//
// A list method returns a *List, which has requested nothing yet: Items ranges over every item
// and ByPage over every page, one request per page; Page fetches the first page, Pager walks the
// items with an explicit cursor, Collect gathers them. Nothing here starts work on its own, so an
// unused list costs nothing and a failing one cannot surprise the program.

// DefaultPageLimit is the page size used when a list call does not set one.
const DefaultPageLimit = 50

// List is a lazy handle on a paged list route. The first page is fetched at most once, however
// many goroutines ask for it; a Pager, by contrast, is single-consumer state and belongs to one
// goroutine.
type List[T any] struct {
	ctx    context.Context
	fetch  func(ctx context.Context, limit, offset int) (*Page[T], error)
	limit  int
	offset int

	// once guards the memoized first page: two goroutines calling Page concurrently must make one
	// request and see the same answer, not race over these three fields.
	once     sync.Once
	first    *Page[T]
	firstErr error
}

// Page fetches the first page. Calling it twice does not repeat the request, and calling it from
// several goroutines at once still makes exactly one.
func (l *List[T]) Page() (*Page[T], error) {
	page, err := l.pageAt(l.offset)
	return page, err
}

// Items ranges over every item across pages, fetching one page per request as it goes; the
// iteration stops at the first error, which it yields with the zero item:
//
//	for payment, err := range client.Payments.ListHistory(ctx, nil).Items() {
//		if err != nil {
//			return err
//		}
//		fmt.Println(payment.UUID)
//	}
func (l *List[T]) Items() iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		p := l.Pager()
		for p.Next() {
			if !yield(p.Item(), nil) {
				return
			}
		}
		if err := p.Err(); err != nil {
			var zero T
			yield(zero, err)
		}
	}
}

// ByPage ranges over the pages themselves, one request per page — for a caller that works a page
// at a time or wants each page's paginate block. It stops at the first error, which it yields
// with a nil page.
func (l *List[T]) ByPage() iter.Seq2[*Page[T], error] {
	return func(yield func(*Page[T], error) bool) {
		offset := l.offset
		for {
			page, err := l.pageAt(offset)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(page, nil) || len(page.Items) == 0 || !page.Paginate.HasPages {
				return
			}
			offset += len(page.Items)
		}
	}
}

// Collect walks every page and gathers the items. maxItems caps the result; 0 means no cap.
func (l *List[T]) Collect(maxItems int) ([]T, error) {
	out := []T{}
	p := l.Pager()
	// The cap is checked before advancing, so a bounded walk never fetches a page it will not use.
	for maxItems <= 0 || len(out) < maxItems {
		if !p.Next() {
			break
		}
		out = append(out, p.Item())
	}
	return out, p.Err()
}

// Pager walks every item across pages, fetching at most one page per call to Next:
//
//	p := client.Payments.ListHistory(ctx, params).Pager()
//	for p.Next() {
//		invoice := p.Item()
//	}
//	if err := p.Err(); err != nil { … }
func (l *List[T]) Pager() *Pager[T] {
	return &Pager[T]{list: l, offset: l.offset}
}

// pageAt fetches one page, reusing the memoized first page when it is the one asked for.
func (l *List[T]) pageAt(offset int) (*Page[T], error) {
	if offset == l.offset {
		l.once.Do(func() {
			l.first, l.firstErr = l.fetch(l.ctx, l.limit, offset)
		})
		return l.first, l.firstErr
	}
	return l.fetch(l.ctx, l.limit, offset)
}

// Pager iterates the items of a list across pages. It is not safe for concurrent use.
type Pager[T any] struct {
	list   *List[T]
	page   *Page[T]
	cur    T
	idx    int
	offset int
	err    error
	done   bool
}

// Next advances to the next item, fetching the next page when the current one runs out. It
// returns false at the end of the list and on the first error, which Err then reports.
func (p *Pager[T]) Next() bool {
	if p.done || p.err != nil {
		return false
	}
	for {
		if p.page != nil && p.idx < len(p.page.Items) {
			p.cur = p.page.Items[p.idx]
			p.idx++
			return true
		}
		if p.page != nil {
			if len(p.page.Items) == 0 || !p.page.Paginate.HasPages {
				p.done = true
				return false
			}
			p.offset += len(p.page.Items)
		}
		page, err := p.list.pageAt(p.offset)
		if err != nil {
			p.err = err
			p.done = true
			return false
		}
		p.page, p.idx = page, 0
		if len(page.Items) == 0 {
			p.done = true
			return false
		}
	}
}

// Item is the item Next stopped on.
func (p *Pager[T]) Item() T { return p.cur }

// Page is the page the current item came from, including its paginate block.
func (p *Pager[T]) Page() *Page[T] { return p.page }

// Err reports why iteration stopped, if it was not the end of the list.
func (p *Pager[T]) Err() error { return p.err }

// newList builds the lazy list handle for a paged route. Limit and offset are taken from the
// caller's params (the body of a POST, the query of a GET) and then driven by the pager, so a
// caller-set limit survives while the offset advances page by page.
func newList[T any](ctx context.Context, r Requester, call Call, o callOptions) *List[T] {
	route := call.Route
	get := route.Method == "GET"
	var fields map[string]any
	var err *Error
	limit, offset := DefaultPageLimit, 0
	if get {
		if v, e := strconv.Atoi(call.Query.Get("limit")); e == nil && v > 0 {
			limit = v
		}
		if v, e := strconv.Atoi(call.Query.Get("offset")); e == nil && v > 0 {
			offset = v
		}
	} else {
		fields, err = toFields(call.Body)
		if v, ok := intField(fields, "limit"); ok && v > 0 {
			limit = v
		}
		if v, ok := intField(fields, "offset"); ok && v > 0 {
			offset = v
		}
		delete(fields, "limit")
		delete(fields, "offset")
	}

	// One idempotency key per page would be wrong on both sides: the core would replay page one
	// for ever. A list is a read, and reads are safe to repeat without a key — so a caller who
	// passed one is told, not quietly ignored: silently dropping it would leave them believing a
	// re-send was deduplicated.
	if o.idempotencyKey != "" && err == nil && !route.Idempotent {
		err = newConfigError(CodeIdempotencyUnsupported, fmt.Sprintf(
			"%s %s is a list route and does not deduplicate by %s; drop WithIdempotencyKey from this call",
			route.Method, route.Path, HeaderIdempotencyKey), "idempotencyKey")
	}
	o.idempotencyKey = ""

	return &List[T]{
		ctx:    ctx,
		limit:  limit,
		offset: offset,
		fetch: func(ctx context.Context, limit, offset int) (*Page[T], error) {
			if err != nil {
				return nil, err
			}
			page := call
			if get {
				query := url.Values{}
				for k, v := range call.Query {
					query[k] = append([]string(nil), v...)
				}
				query.Set("limit", strconv.Itoa(limit))
				query.Set("offset", strconv.Itoa(offset))
				page.Query = query
			} else {
				body := map[string]any{}
				for k, v := range fields {
					body[k] = v
				}
				body["limit"] = limit
				body["offset"] = offset
				page.Body = body
			}
			raw, reqErr := r.request(ctx, page, o)
			if reqErr != nil {
				return nil, reqErr
			}
			result, decodeErr := decodeResult[Page[T]](route, raw)
			if decodeErr != nil {
				return nil, decodeErr
			}
			return result, nil
		},
	}
}

// toFields renders a params struct as a field map so pagination can override limit and offset
// without every list method having to expose them separately.
func toFields(params any) (map[string]any, *Error) {
	fields := map[string]any{}
	if params == nil {
		return fields, nil
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return fields, newConfigError(CodeBadConfig, "the list parameters cannot be encoded as JSON: "+err.Error(), "")
	}
	if string(encoded) == "null" {
		return fields, nil
	}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return fields, newConfigError(CodeBadConfig, "the list parameters must encode to a JSON object", "")
	}
	return fields, nil
}

func intField(fields map[string]any, name string) (int, bool) {
	switch v := fields[name].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}
