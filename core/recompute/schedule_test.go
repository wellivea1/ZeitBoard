package recompute_test

import (
	"testing"
	"time"

	"non24.app/core/recompute"
)

func testSchedule() *recompute.Schedule {
	return &recompute.Schedule{
		Debounce:    3 * time.Second,
		MinInterval: 30 * time.Second,
		RetryBase:   time.Minute,
		MaxBackoff:  10 * time.Minute,
		Heartbeat:   time.Hour,
	}
}

// TestABurstCollapsesToOneRun is the coalescing requirement. A device sync
// arrives as a run of pushes; recomputing per push would repeat one answer
// dozens of times.
func TestABurstCollapsesToOneRun(t *testing.T) {
	s := testSchedule()
	for i := 0; i < 50; i++ {
		s.Request(recompute.ReasonEvidence, base.Add(time.Duration(i)*10*time.Millisecond))
	}
	if _, due := s.Begin(base.Add(time.Second)); due {
		t.Error("a run started before the burst had settled")
	}
	claim, due := s.Begin(base.Add(4 * time.Second))
	if !due {
		t.Fatal("the burst never became due")
	}
	if claim.Reason != recompute.ReasonEvidence {
		t.Errorf("reason = %q, want the one that started the burst", claim.Reason)
	}
	if claim.Coalesced != 49 {
		t.Errorf("coalesced = %d, want the 49 requests folded in", claim.Coalesced)
	}
	s.Succeeded(base.Add(4 * time.Second))
	if _, due := s.Begin(base.Add(5 * time.Second)); due {
		t.Error("the run repeated after succeeding")
	}
}

// TestARequestDuringARunIsNotSwallowedByItsCompletion.
//
// Found by a live burst against the running worker, not by reasoning: forty
// requests arrived while the startup run was still finishing, the completion
// cleared the pending flag, and every one of them was lost. Clearing at the end
// of a run is the obvious implementation and it drops exactly the requests that
// matter, because a run takes long enough to overlap the burst that caused it.
// They are not redundant either — this run may have read its inputs before the
// change they are reporting landed.
func TestARequestDuringARunIsNotSwallowedByItsCompletion(t *testing.T) {
	s := testSchedule()
	s.Request(recompute.ReasonEvidence, base)

	claim, due := s.Begin(base.Add(4 * time.Second))
	if !due || claim.Reason != recompute.ReasonEvidence {
		t.Fatalf("the first run never began: %+v due=%v", claim, due)
	}

	// Arrives mid-run.
	s.Request(recompute.ReasonEvidence, base.Add(5*time.Second))
	s.Succeeded(base.Add(6 * time.Second))

	if !s.Pending() {
		t.Fatal("a request that arrived during the run was cleared by its completion")
	}
	if _, due := s.Begin(base.Add(40 * time.Second)); !due {
		t.Error("the mid-run request never ran")
	}
}

// TestABackoffIsNotBypassedByAskingAgain: a request arriving during a failed run
// must not pull the retry forward. The thing that broke is no less broken for
// someone having asked again.
func TestABackoffIsNotBypassedByAskingAgain(t *testing.T) {
	s := testSchedule()
	s.Request(recompute.ReasonEvidence, base)
	if _, due := s.Begin(base.Add(4 * time.Second)); !due {
		t.Fatal("the first run never began")
	}
	s.Request(recompute.ReasonEvidence, base.Add(5*time.Second))
	s.Failed(base.Add(6 * time.Second))

	if _, due := s.Begin(base.Add(10 * time.Second)); due {
		t.Error("a run started inside the backoff because a request arrived during the failure")
	}
	if _, due := s.Begin(base.Add(70 * time.Second)); !due {
		t.Error("the retry never became due")
	}
}

// TestASteadyTrickleStillRuns is the failure mode that looks like the feature
// working: a debounce that restarts on every request never fires while requests
// keep arriving, and the projection silently stops updating under exactly the
// load that needs it most.
func TestASteadyTrickleStillRuns(t *testing.T) {
	s := testSchedule()
	for i := 0; i < 20; i++ {
		s.Request(recompute.ReasonEvidence, base.Add(time.Duration(i)*time.Second))
	}
	if _, due := s.Begin(base.Add(19 * time.Second)); !due {
		t.Error("continuous requests postponed the run indefinitely")
	}
}

// TestMinIntervalFloorsTheRate bounds what a chatty device can cost.
func TestMinIntervalFloorsTheRate(t *testing.T) {
	s := testSchedule()
	s.Request(recompute.ReasonEvidence, base)
	if _, due := s.Begin(base.Add(4 * time.Second)); !due {
		t.Fatal("the first run never became due")
	}
	s.Succeeded(base.Add(4 * time.Second))

	s.Request(recompute.ReasonEvidence, base.Add(5*time.Second))
	if _, due := s.Begin(base.Add(10 * time.Second)); due {
		t.Error("a second run started inside the minimum interval")
	}
	if _, due := s.Begin(base.Add(40 * time.Second)); !due {
		t.Error("the second run never became due after the interval passed")
	}
}

// TestExpiryRunsWithNobodyAsking is the point of the slice. Nothing changed and
// the honest answer changed anyway, because the freshness policy is a function
// of time.
func TestExpiryRunsWithNobodyAsking(t *testing.T) {
	s := testSchedule()
	// Long enough that the heartbeat cannot be the thing that fires; this test
	// is about the expiry and nothing else.
	s.Heartbeat = 24 * time.Hour
	s.Request(recompute.ReasonEvidence, base)
	if _, due := s.Begin(base.Add(4 * time.Second)); !due {
		t.Fatal("the first run never became due")
	}
	s.Succeeded(base.Add(4 * time.Second))
	s.Expires(base.Add(6 * time.Hour))

	if _, due := s.Begin(base.Add(3 * time.Hour)); due {
		t.Error("a run started before the result expired")
	}
	claim, due := s.Begin(base.Add(6 * time.Hour))
	if !due {
		t.Fatal("the expiry never became due")
	}
	if claim.Reason != recompute.ReasonFreshnessExpiry {
		t.Errorf("reason = %q, want freshness_expiry", claim.Reason)
	}
}

// TestNextWakeIsExactlyTheNextDeadline: the worker sleeps on this value, so a
// wake later than the deadline is a late withholding.
func TestNextWakeIsExactlyTheNextDeadline(t *testing.T) {
	s := testSchedule()
	s.Request(recompute.ReasonEvidence, base)
	next, scheduled := s.NextWake(base)
	if !scheduled {
		t.Fatal("nothing was scheduled after a request")
	}
	if want := base.Add(3 * time.Second); !next.Equal(want) {
		t.Errorf("next wake = %s, want %s", next, want)
	}

	if _, due := s.Begin(next); !due {
		t.Error("the run was not due at the instant NextWake pointed at")
	}
}

func TestNextWakeIsNeverInThePast(t *testing.T) {
	s := testSchedule()
	s.Request(recompute.ReasonEvidence, base)
	next, scheduled := s.NextWake(base.Add(time.Hour))
	if !scheduled {
		t.Fatal("nothing was scheduled")
	}
	if next.Before(base.Add(time.Hour)) {
		t.Errorf("next wake = %s is before now; a negative sleep is a spin", next)
	}
}

// TestFailureBacksOffAndKeepsTheWorkPending. A failed run is not lost work: the
// inputs still say it is needed.
func TestFailureBacksOffAndKeepsTheWorkPending(t *testing.T) {
	s := testSchedule()
	s.Request(recompute.ReasonEvidence, base)

	first := s.Failed(base.Add(4 * time.Second))
	if first != time.Minute {
		t.Errorf("first backoff = %s, want the base delay", first)
	}
	if !s.Pending() {
		t.Fatal("a failed run dropped the work")
	}
	second := s.Failed(base.Add(2 * time.Minute))
	if second != 2*time.Minute {
		t.Errorf("second backoff = %s, want double the base", second)
	}
	for i := 0; i < 10; i++ {
		if got := s.Failed(base.Add(time.Duration(i+3) * time.Hour)); got > 10*time.Minute {
			t.Fatalf("backoff = %s exceeded the ceiling", got)
		}
	}
	if s.Failures() != 12 {
		t.Errorf("failures = %d, want every attempt counted", s.Failures())
	}

	claim, due := s.Begin(base.Add(24 * time.Hour))
	if !due || claim.Reason != recompute.ReasonRetry {
		t.Errorf("after backing off: reason = %q due = %v, want a retry", claim.Reason, due)
	}
	s.Succeeded(base.Add(24 * time.Hour))
	if s.Failures() != 0 {
		t.Error("a success did not clear the backoff")
	}
}

// TestHeartbeatIsTheBackstopNotTheMechanism. Nothing should depend on it, but a
// process that has published nothing for an hour rechecks, so state that drifted
// underneath it is repaired without a restart.
func TestHeartbeatIsTheBackstop(t *testing.T) {
	s := testSchedule()
	s.Request(recompute.ReasonEvidence, base)
	if _, due := s.Begin(base.Add(4 * time.Second)); !due {
		t.Fatal("the first run never became due")
	}
	s.Succeeded(base.Add(4 * time.Second))

	if _, due := s.Begin(base.Add(30 * time.Minute)); due {
		t.Error("the heartbeat fired early")
	}
	claim, due := s.Begin(base.Add(65 * time.Minute))
	if !due || claim.Reason != recompute.ReasonHeartbeat {
		t.Errorf("reason = %q due = %v, want a heartbeat after an idle hour", claim.Reason, due)
	}
}
