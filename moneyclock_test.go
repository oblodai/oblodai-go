package oblodai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Amounts and the clock: two places where a "close enough" answer is worse than an error.

func TestMoneyHelpersRefuseWhatIsNotAnAmount(t *testing.T) {
	bad := []string{
		"", "5.", ".5", "-", "-.", "1.2.3", "1e3", " 5", "5 ", "+5", "5,0", "abc", "0x10",
		"--1", "1_000", strings.Repeat("9", MaxAmountLength+1),
	}
	for _, amount := range bad {
		if _, err := AddAmounts(amount, "1"); err == nil {
			t.Errorf("AddAmounts(%q) was accepted", amount)
			continue
		} else if !IsConfig(err) || !IsCode(err, CodeBadAmount) {
			t.Errorf("AddAmounts(%q) = %v, want a ConfigError with %s", amount, err, CodeBadAmount)
		}
		if _, err := CompareAmounts("1", amount); !IsCode(err, CodeBadAmount) {
			t.Errorf("CompareAmounts(%q) = %v", amount, err)
		}
		if AmountsEqual(amount, amount) {
			t.Errorf("AmountsEqual(%q) must be false for a malformed amount", amount)
		}
		if IsZeroAmount(amount) {
			t.Errorf("IsZeroAmount(%q) must be false for a malformed amount", amount)
		}
	}

	// A trailing dot used to be read as the integer part alone: "5." + "1" answered "6".
	if sum, err := AddAmounts("5.", "1"); err == nil {
		t.Fatalf(`AddAmounts("5.", "1") = %q, want an error`, sum)
	}

	for _, amount := range []string{"0", "25", "-0.5", "10.000000", strings.Repeat("9", MaxAmountLength)} {
		if _, err := AddAmounts(amount, "0"); err != nil {
			t.Errorf("AddAmounts(%q) = %v, want it accepted", amount, err)
		}
	}

	// Amounts are strings; text order is not money order, which is why CompareAmounts exists.
	if "9" < "10" {
		t.Fatal("this test assumes lexicographic string comparison")
	}
	cmp, err := CompareAmounts("9", "10")
	if err != nil || cmp != -1 {
		t.Fatalf("CompareAmounts(9, 10) = %d, %v", cmp, err)
	}
}

// A signed document link travels as its query: exp and sig verbatim, lang only when set.
func TestSignedDocumentSendsItsLinkQuery(t *testing.T) {
	api := newFakeAPI(t, step{status: 200, body: "%PDF", headers: map[string]string{"Content-Type": "application/pdf"}})
	file, err := api.client().Documents.GetSigned(context.Background(), "invoice", "i1",
		&GetSignedDocumentParams{Exp: 1_800_000_000, Sig: "s"})
	if err != nil {
		t.Fatalf("GetSigned: %v", err)
	}
	if string(file.Bytes) != "%PDF" || file.ContentType != "application/pdf" {
		t.Fatalf("file = %+v", file)
	}
	got := api.last()
	if got.path != "/v1/documents/invoice/i1" || got.rawQuery != "exp=1800000000&sig=s" {
		t.Fatalf("request = %s?%s", got.path, got.rawQuery)
	}
}

// skewedAPI refuses every request whose HeaderTimestamp is more than SkewSeconds from its own clock, the way
// the core does, and reports its time in the Date header.
type skewedAPI struct {
	serverTime time.Time

	mu          sync.Mutex
	corrections int
	rejections  int
}

func (s *skewedAPI) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ts := r.Header.Get(HeaderTimestamp)
		var seconds int64
		if _, err := fmt.Sscan(ts, &seconds); err != nil {
			t.Errorf("no timestamp header: %q", ts)
		}
		drift := seconds - s.serverTime.Unix()
		w.Header().Set("Date", s.serverTime.UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Type", "application/json")
		if drift > SkewSeconds || drift < -SkewSeconds {
			s.mu.Lock()
			s.rejections++
			s.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"merchant.bad_signature","message":"stale timestamp","retryable":false}}`))
			return
		}
		s.mu.Lock()
		s.corrections++
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"state":0,"result":{"uuid":"p1"}}`))
	}
}

// Many calls in flight against a core an hour ahead: every one must succeed, and the correction
// must be learned once rather than fought over.
func TestClockSkewCorrectionUnderConcurrency(t *testing.T) {
	api := &skewedAPI{serverTime: time.Now().Add(time.Hour)}
	server := newRawServer(t, api.handler(t))
	client, err := New(WithBaseURL(server), WithCredentials("pk_test_1", "secret-1"),
		WithRetry(RetryOptions{MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}))
	if err != nil {
		t.Fatal(err)
	}

	const callers = 8
	var wg sync.WaitGroup
	errs := make([]error, callers)
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = client.Payments.GetInfo(context.Background(), &LookupRequest{UUID: Ptr("p1")})
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d: %v", i, err)
		}
	}
	if offset := client.ClockOffset(); offset < 55*time.Minute || offset > 65*time.Minute {
		t.Fatalf("learned offset = %s, want about an hour", offset)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.rejections > callers {
		t.Fatalf("%d rejections for %d callers: each call may re-sign once", api.rejections, callers)
	}
}

// A correction is reverted only by the call that installed it: a concurrent call that measured its
// own offset must keep it.
func TestClockCorrectionRevertsOnlyItsOwnOffset(t *testing.T) {
	clock := newSkewClock(time.Now)
	clock.correct(time.Hour)
	if clock.revert(30*time.Minute, 0) {
		t.Fatal("a revert must not undo an offset another call installed")
	}
	if clock.currentOffset() != time.Hour {
		t.Fatalf("offset = %s, want an hour", clock.currentOffset())
	}
	if !clock.revert(time.Hour, 0) {
		t.Fatal("a call must be able to revert its own correction")
	}
	if clock.currentOffset() != 0 {
		t.Fatalf("offset = %s, want zero", clock.currentOffset())
	}
}

// newRawServer starts a server with a handler of the test's own, and returns its URL.
func newRawServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL
}
