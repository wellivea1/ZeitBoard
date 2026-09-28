package main

import (
	"errors"
	"sync"
	"time"
)

// Some backend reads happen on their own: Home's overview, Rhythm, the
// decision queue's synced sources, the list of share links. When the server
// cannot be reached (its machine is off, or the network is down), each used to
// wait out the whole request timeout, on every visit and every refresh.
// backendOutage remembers that no answer came, so those reads give up at once
// for a while. Every request reports whether the server answered: sync keeps
// trying on its own schedule, and the first answer ends the outage.
type backendOutage struct {
	mu    sync.Mutex
	until time.Time
}

// backendOutageHold is how long passive reads skip the server after it did not
// answer; background sync tries again within a minute regardless.
const backendOutageHold = 2 * time.Minute

// errBackendUnreachable is a request that got no answer at all. An answer,
// even an error status, came back quickly and is not an outage.
var errBackendUnreachable = errors.New("Could not reach ZeitBoard's server.")

func (o *backendOutage) active(now time.Time) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return now.Before(o.until)
}

// start records that a request got no answer.
func (o *backendOutage) start(now time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.until = now.Add(backendOutageHold)
}

// clear records that the server answered.
func (o *backendOutage) clear() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.until = time.Time{}
}
