package recompute

import "time"

// Default schedule parameters. They are deliberately unexciting: the work is
// cheap, nobody is watching it, and the cost of being a minute late is nil while
// the cost of thrashing a laptop's disk is not.
const (
	// DefaultDebounce is how long a burst is allowed to settle. A device sync
	// arrives as a run of pushes, and recomputing after each one would repeat
	// the same answer several times over.
	DefaultDebounce = 3 * time.Second

	// DefaultMinInterval is the floor between two runs. It bounds the cost of a
	// device that pushes continuously.
	DefaultMinInterval = 30 * time.Second

	// DefaultRetryBase and DefaultMaxBackoff govern re-attempts after a failure.
	// A failing run is usually a locked database or a disk that filled up, and
	// neither is fixed by trying again immediately.
	DefaultRetryBase  = 30 * time.Second
	DefaultMaxBackoff = 15 * time.Minute

	// DefaultHeartbeat is the longest a process will go without recomputing
	// even if nothing asks it to and nothing is known to expire. It is the
	// backstop for a missed wake, not the mechanism: expiry scheduling is.
	DefaultHeartbeat = time.Hour
)

// Schedule decides when the next run should happen. It is a state machine with
// no timer and no goroutine, so its behaviour is a function of the calls made
// to it and tests need not wait for anything.
//
// Coalescing rule: the first request in a burst sets the deadline and later
// requests do not push it back. Extending on each request would coalesce a
// steady trickle into never running at all, which is the failure mode that
// looks like the feature working.
type Schedule struct {
	Debounce    time.Duration
	MinInterval time.Duration
	RetryBase   time.Duration
	MaxBackoff  time.Duration
	Heartbeat   time.Duration

	pending    bool
	reason     Reason
	dueAt      time.Time
	lastRun    time.Time
	validUntil time.Time
	failures   int
	coalesced  int
}

func (s *Schedule) debounce() time.Duration {
	if s.Debounce > 0 {
		return s.Debounce
	}
	return DefaultDebounce
}

func (s *Schedule) minInterval() time.Duration {
	if s.MinInterval > 0 {
		return s.MinInterval
	}
	return DefaultMinInterval
}

func (s *Schedule) retryBase() time.Duration {
	if s.RetryBase > 0 {
		return s.RetryBase
	}
	return DefaultRetryBase
}

func (s *Schedule) maxBackoff() time.Duration {
	if s.MaxBackoff > 0 {
		return s.MaxBackoff
	}
	return DefaultMaxBackoff
}

func (s *Schedule) heartbeat() time.Duration {
	if s.Heartbeat > 0 {
		return s.Heartbeat
	}
	return DefaultHeartbeat
}

// Request records that something changed. Calls during an already-pending burst
// are counted and dropped, which is the coalescing.
func (s *Schedule) Request(reason Reason, now time.Time) {
	now = now.UTC()
	if s.pending {
		s.coalesced++
		return
	}
	s.pending = true
	s.reason = reason
	s.coalesced = 0
	due := now.Add(s.debounce())
	if !s.lastRun.IsZero() {
		if floor := s.lastRun.Add(s.minInterval()); due.Before(floor) {
			due = floor
		}
	}
	s.dueAt = due
}

// Expires records when the last result stops being trustworthy on its own. This
// is what turns "recompute when something changes" into "recompute when the
// answer changes", which for a policy keyed on evidence age is not the same
// thing at all.
func (s *Schedule) Expires(at time.Time) {
	s.validUntil = at.UTC()
}

// Claim is the work one run is taking on.
type Claim struct {
	Reason Reason

	// Coalesced is how many requests were folded into this run. It is worth
	// logging: it says whether the coalescing is doing anything.
	Coalesced int
}

// Begin claims the work that is due, if any.
//
// The pending flag is cleared **here**, at the start of the run, and not when
// the run finishes. Clearing it at the end is the obvious implementation and it
// silently drops every request that arrives while a run is in flight — which is
// exactly when requests arrive, because a run takes long enough to overlap the
// burst that triggered it. Those requests are not redundant either: the run may
// have read its inputs before the change they are reporting landed.
func (s *Schedule) Begin(now time.Time) (Claim, bool) {
	now = now.UTC()
	if s.pending && !now.Before(s.dueAt) {
		claim := Claim{Reason: s.reason, Coalesced: s.coalesced}
		s.pending = false
		s.reason = ""
		s.coalesced = 0
		return claim, true
	}
	if !s.validUntil.IsZero() && !now.Before(s.validUntil) && s.pastFloor(now) {
		return Claim{Reason: ReasonFreshnessExpiry}, true
	}
	if !s.lastRun.IsZero() && now.Sub(s.lastRun) >= s.heartbeat() {
		return Claim{Reason: ReasonHeartbeat}, true
	}
	return Claim{}, false
}

func (s *Schedule) pastFloor(now time.Time) bool {
	return s.lastRun.IsZero() || !now.Before(s.lastRun.Add(s.minInterval()))
}

// NextWake reports when Due could next become true, so a caller can sleep
// exactly that long instead of polling. The second return is false when nothing
// is scheduled at all.
func (s *Schedule) NextWake(now time.Time) (time.Time, bool) {
	now = now.UTC()
	var next time.Time
	consider := func(candidate time.Time) {
		if candidate.IsZero() {
			return
		}
		if candidate.Before(now) {
			candidate = now
		}
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	if s.pending {
		consider(s.dueAt)
	}
	if !s.validUntil.IsZero() {
		floor := s.validUntil
		if !s.lastRun.IsZero() && floor.Before(s.lastRun.Add(s.minInterval())) {
			floor = s.lastRun.Add(s.minInterval())
		}
		consider(floor)
	}
	if !s.lastRun.IsZero() {
		consider(s.lastRun.Add(s.heartbeat()))
	}
	return next, !next.IsZero()
}

// Succeeded records a completed run. It deliberately does not touch the pending
// flag: anything pending now arrived after Begin claimed the work, and is a
// request this run may not have seen.
func (s *Schedule) Succeeded(now time.Time) {
	s.failures = 0
	s.lastRun = now.UTC()
}

// Failed re-marks the work pending and backs off. The request is not lost: a
// recompute is derivable from the inputs, so retrying is always safe.
func (s *Schedule) Failed(now time.Time) time.Duration {
	now = now.UTC()
	s.failures++
	s.lastRun = now
	delay := s.retryBase()
	for i := 1; i < s.failures; i++ {
		delay *= 2
		if delay >= s.maxBackoff() {
			delay = s.maxBackoff()
			break
		}
	}
	retryAt := now.Add(delay)
	// A request that arrived during the failed run must not bypass the backoff:
	// the thing that broke is no less broken for someone having asked again.
	if !s.pending || s.dueAt.Before(retryAt) {
		s.dueAt = retryAt
	}
	s.pending = true
	s.reason = ReasonRetry
	return delay
}

// Failures reports the current consecutive failure count.
func (s *Schedule) Failures() int { return s.failures }

// Pending reports whether a run is waiting to start.
func (s *Schedule) Pending() bool { return s.pending }
