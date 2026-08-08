package freshness_test

import (
	"testing"
	"time"

	"non24.app/core/freshness"
)

var expiryNow = time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)

// TestNextChangeIsNeverLateIsTheWholeGuarantee walks the clock forward a minute
// at a time and checks that the state never changes before NextChange said it
// could.
//
// The direction is the point. A prediction that is early costs one extra
// recompute, which republishes the same answer. A prediction that is late leaves
// a confident claim standing over evidence that no longer supports it, which is
// the defect this whole slice exists to close.
func TestNextChangeIsNeverLate(t *testing.T) {
	policy := freshness.Default()

	cases := []struct {
		name  string
		build func(now time.Time) freshness.Inputs
	}{
		{
			name: "evidence ages out with no prediction",
			build: func(now time.Time) freshness.Inputs {
				return freshness.Inputs{
					Now:            now,
					NewestEvidence: expiryNow,
					LatestSleepEnd: expiryNow,
				}
			},
		},
		{
			name: "sleep predicted and never recorded",
			build: func(now time.Time) freshness.Inputs {
				return freshness.Inputs{
					Now:                now,
					NewestEvidence:     expiryNow,
					LatestSleepEnd:     expiryNow,
					ExpectedSleepOnset: expiryNow.Add(3 * time.Hour),
				}
			},
		},
		{
			name: "sleep predicted before the stale threshold",
			build: func(now time.Time) freshness.Inputs {
				return freshness.Inputs{
					Now:                now,
					NewestEvidence:     expiryNow,
					LatestSleepEnd:     expiryNow,
					ExpectedSleepOnset: expiryNow.Add(-30 * time.Minute),
				}
			},
		},
		{
			name: "configured sources but none reporting",
			build: func(now time.Time) freshness.Inputs {
				return freshness.Inputs{
					Now:               now,
					NewestEvidence:    expiryNow,
					LatestSleepEnd:    expiryNow,
					SourcesConfigured: 2,
					SourcesReporting:  0,
				}
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			state := policy.Assess(testCase.build(expiryNow)).State
			predicted := policy.NextChange(testCase.build(expiryNow))

			for minute := 1; minute <= 40*60; minute++ {
				at := expiryNow.Add(time.Duration(minute) * time.Minute)
				inputs := testCase.build(at)
				current := policy.Assess(inputs).State
				if current == state {
					continue
				}
				if predicted.IsZero() {
					t.Fatalf("state changed to %q at %s but no change was predicted", current, at)
				}
				if at.Before(predicted) {
					t.Fatalf("state changed to %q at %s, before the predicted %s", current, at, predicted)
				}
				// Re-anchor and keep walking: later thresholds must hold too.
				state = current
				predicted = policy.NextChange(inputs)
			}
		})
	}
}

// TestNoEvidenceNeverExpires. Waiting does not make an empty history usable, and
// scheduling a wake for it would be a loop that recomputes nothing forever.
func TestNoEvidenceNeverExpires(t *testing.T) {
	next := freshness.Default().NextChange(freshness.Inputs{Now: expiryNow})
	if !next.IsZero() {
		t.Errorf("next change = %s, want none: no evidence cannot age into evidence", next)
	}
}

// TestARecordedSleepDischargesItsOwnDeadline: once sleep is recorded at or after
// the predicted onset, the unrecorded-sleep rule is spent and must not schedule
// a wake that would find nothing changed.
func TestARecordedSleepDischargesItsOwnDeadline(t *testing.T) {
	onset := expiryNow.Add(-8 * time.Hour)
	next := freshness.Default().NextChange(freshness.Inputs{
		Now:                expiryNow,
		NewestEvidence:     expiryNow.Add(-time.Hour),
		ExpectedSleepOnset: onset,
		LatestSleepEnd:     onset.Add(7 * time.Hour),
	})
	// The only remaining deadlines are the age thresholds on the evidence.
	if want := expiryNow.Add(5 * time.Hour); !next.Equal(want) {
		t.Errorf("next change = %s, want the stale threshold at %s", next, want)
	}
}

// TestAWithheldStateStopsScheduling closes the loop: once every threshold has
// passed there is nothing left to wake up for.
func TestAWithheldStateStopsScheduling(t *testing.T) {
	policy := freshness.Default()
	inputs := freshness.Inputs{
		Now:            expiryNow,
		NewestEvidence: expiryNow.Add(-3 * 24 * time.Hour),
		LatestSleepEnd: expiryNow.Add(-3 * 24 * time.Hour),
	}
	if assessment := policy.Assess(inputs); assessment.State != freshness.StateWithheld {
		t.Fatalf("state = %q, want withheld", assessment.State)
	}
	if next := policy.NextChange(inputs); !next.IsZero() {
		t.Errorf("next change = %s, want none once everything has expired", next)
	}
}

// TestTheGracePeriodIsScheduledNotJustChecked pins the deadline against the
// measured onset error ADR-0022 recorded, so a change to the grace period shows
// up here as well as in the assessment.
func TestTheGracePeriodIsScheduled(t *testing.T) {
	// Evidence recorded just now about a night that ended long ago — a
	// correction, say — with the predicted onset already behind us. The grace
	// deadline is then the earliest of the three, so it is the one returned.
	onset := expiryNow.Add(-5 * time.Hour)
	next := freshness.Default().NextChange(freshness.Inputs{
		Now:                expiryNow,
		NewestEvidence:     expiryNow,
		LatestSleepEnd:     expiryNow.Add(-10 * time.Hour),
		ExpectedSleepOnset: onset,
	})
	if want := onset.Add(6 * time.Hour).Add(time.Nanosecond); !next.Equal(want) {
		t.Errorf("next change = %s, want the onset plus the grace period (%s)", next, want)
	}
}

// TestARunAtTheReturnedInstantSeesTheChange is the sharp edge the minute-by-
// minute walk cannot see. The overdue rule is a strict comparison, so a wake
// scheduled for the exact deadline would find nothing changed, re-derive its
// next wake from the unchanged state, and leave the claim standing until some
// unrelated threshold happened to catch it.
func TestARunAtTheReturnedInstantSeesTheChange(t *testing.T) {
	policy := freshness.Default()
	onset := expiryNow.Add(-5 * time.Hour)
	inputs := freshness.Inputs{
		Now:                expiryNow,
		NewestEvidence:     expiryNow,
		LatestSleepEnd:     expiryNow.Add(-10 * time.Hour),
		ExpectedSleepOnset: onset,
	}
	if state := policy.Assess(inputs).State; state == freshness.StateWithheld {
		t.Fatalf("the fixture starts withheld (%q); there is nothing to observe changing", state)
	}

	next := policy.NextChange(inputs)
	at := inputs
	at.Now = next
	assessment := policy.Assess(at)
	if assessment.State != freshness.StateWithheld {
		t.Errorf("state at the predicted instant = %q, want withheld", assessment.State)
	}
	if assessment.Reason != freshness.ReasonExpectedSleepUnrecorded {
		t.Errorf("reason = %q, want the unrecorded-sleep rule", assessment.Reason)
	}
}
