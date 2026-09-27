package oblodai

import (
	"sync"
	"time"
)

// Injectable clock for signing. The core rejects timestamps more than +/-SkewSeconds from its own time,
// so a host with a drifting clock would get merchant.bad_signature on every call. The transport
// learns the server's time from the Date header of a signature-failure response and re-signs that
// one call with it; the offset is adopted for later calls only when the re-signed attempt
// succeeded (2xx), and discarded otherwise.

// maxPlausibleOffset bounds what the client accepts as clock drift: a single response can never
// move the signing clock further than this. A Date header beyond it is treated as broken (a
// misconfigured proxy) or hostile — whoever could shift the clock by hours could make captured
// signed requests replayable long after they were made.
const maxPlausibleOffset = 900 * time.Second

// skewClock is a clock with a learned server offset. It is safe for concurrent use: one client
// serves many goroutines and any of them may discover the offset.
type skewClock struct {
	mu     sync.RWMutex
	offset time.Duration
	base   func() time.Time
}

func newSkewClock(base func() time.Time) *skewClock {
	if base == nil {
		base = time.Now
	}
	return &skewClock{base: base}
}

// stampWith is the current unix time in seconds with offset applied instead of the learned one: a
// trial offset for the single re-signed attempt, not yet adopted.
func (c *skewClock) stampWith(offset time.Duration) int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.base().Add(offset).Unix()
}

// stamp is the current unix time in seconds with the learned offset applied, plus the offset it
// was produced with. A caller that re-signs after a signature
// failure compares the server's time with the offset ITS request carried, not with whatever
// another goroutine has installed since.
func (c *skewClock) stamp() (int64, time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.base().Add(c.offset).Unix(), c.offset
}

// currentOffset reports the offset in force.
func (c *skewClock) currentOffset() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.offset
}

// observeServerDate measures the server-minus-local offset from a response Date header. The second
// result is false when the header is absent, unparsable or implausible.
func (c *skewClock) observeServerDate(dateHeader string) (time.Duration, bool) {
	if dateHeader == "" {
		return 0, false
	}
	serverTime, err := http1123(dateHeader)
	if err != nil {
		return 0, false
	}
	c.mu.RLock()
	local := c.base()
	c.mu.RUnlock()
	offset := serverTime.Sub(local).Round(time.Second)
	if offset > maxPlausibleOffset || offset < -maxPlausibleOffset {
		return 0, false
	}
	return offset, true
}

// correct adopts an offset for every subsequent signature. The transport calls it only after a
// request signed with that offset succeeded.
func (c *skewClock) correct(offset time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset = offset
}
