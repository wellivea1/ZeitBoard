package main

import (
	"errors"
	"sync"
	"time"
)

// Home's overview and Rhythm's actogram ask ZeitBoard's server first. When it
// cannot be reached (its machine is off, or the network is down), each read
// used to wait out the whole request timeout before showing the local
// estimate, on every visit and every refresh. backendOutage remembers the
// failure, so those reads go straight to the local estimate for a while. Sync
// keeps trying on its own schedule, and its first success ends the outage.
type backendOutage struct {
	mu    sync.Mutex
	until time.Time
}

// backendOutageHold is how long a read skips the server after it could not be
// reached; background sync tries again within a minute regardless.
const backendOutageHold = 2 * time.Minute

// errBackendUnreachable is a request that got no answer at all. An answer,
// even an error status, came back quickly and marks no outage.
var errBackendUnreachable = errors.New("Could not reach ZeitBoard's server.")

func (o *backendOutage) active(now time.Time) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return now.Before(o.until)
}

// note starts an outage if err says the server could not be reached.
func (o *backendOutage) note(err error, now time.Time) {
	if !errors.Is(err, errBackendUnreachable) {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.until = now.Add(backendOutageHold)
}

func (o *backendOutage) clear() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.until = time.Time{}
}
