package oblodai

import (
	"sync"
	"time"
)

// Injectable clock for signing. The core rejects timestamps more than +/-300 s from its own time,
// so a host with a drifting clock would get merchant.bad_signature on every call. The transport
// learns the server's time from the Date header of a signature-failure response, re-signs once,
// and keeps the offset only if that re-signed attempt got past authentication.

// maxPlausibleOffset bounds what the client accepts as clock drift; beyond it the Date header is
// more likely broken (a misconfigured proxy) than the local clock.
const maxPlausibleOffset = 24 * time.Hour

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

// now is the current unix time in seconds, with the learned offset applied.
func (c *skewClock) now() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.base().Add(c.offset).Unix()
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

// correct applies an offset to every subsequent signature.
func (c *skewClock) correct(offset time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset = offset
}
