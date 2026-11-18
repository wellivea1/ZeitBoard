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
			name: "evidence ages out with no forecast",
			build: func(now time.Time) freshness.Inputs {
				return freshness.Inputs{
					Now:            now,
					NewestEvidence: expiryNow,
					LatestSleepEnd: expiryNow,
				}
			},
		},
		{
			name: "a sleep forecast and never recorded",
			build: func(now time.Time) freshness.Inputs {
				return freshness.Inputs{
					Now:                now,
					NewestEvidence:     expiryNow,
					LatestSleepEnd:     expiryNow,
					ExpectedSleepOnset: expiryNow.Add(3 * time.Hour),
					ExpectedSleepEnd:   expiryNow.Add(14 * time.Hour),
				}
			},
		},
		{
			name: "a forecast sleep already under way",
			build: func(now time.Time) freshness.Inputs {
				return freshness.Inputs{
					Now:                now,
					NewestEvidence:     expiryNow.Add(-16 * time.Hour),
					LatestSleepEnd:     expiryNow.Add(-16 * time.Hour),
					ExpectedSleepOnset: expiryNow.Add(-30 * time.Minute),
					ExpectedSleepEnd:   expiryNow.Add(10 * time.Hour),
				}
			},
		},
		{
			name: "a forecast sleep already recorded",
			build: func(now time.Time) freshness.Inputs {
				return freshness.Inputs{
					Now:                now,
					NewestEvidence:     expiryNow,
					LatestSleepEnd:     expiryNow,
					ExpectedSleepOnset: expiryNow.Add(-9 * time.Hour),
					ExpectedSleepEnd:   expiryNow.Add(2 * time.Hour),
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
// the forecast onset, the forecast's deadline is spent and must not schedule a
// wake that would find nothing changed.
func TestARecordedSleepDischargesItsOwnDeadline(t *testing.T) {
	onset := expiryNow.Add(-8 * time.Hour)
	next := freshness.Default().NextChange(freshness.Inputs{
		Now:                expiryNow,
		NewestEvidence:     expiryNow.Add(-time.Hour),
		ExpectedSleepOnset: onset,
		ExpectedSleepEnd:   onset.Add(11 * time.Hour),
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
	for name, inputs := range map[string]freshness.Inputs{
		"by age": {
			Now:            expiryNow,
			NewestEvidence: expiryNow.Add(-3 * 24 * time.Hour),
			LatestSleepEnd: expiryNow.Add(-3 * 24 * time.Hour),
		},
		"by the forecast": {
			Now:                expiryNow,
			NewestEvidence:     expiryNow.Add(-time.Hour),
			LatestSleepEnd:     expiryNow.Add(-3 * 24 * time.Hour),
			ExpectedSleepOnset: expiryNow.Add(-2*24*time.Hour - 10*time.Hour),
			ExpectedSleepEnd:   expiryNow.Add(-2 * 24 * time.Hour),
		},
	} {
		if assessment := policy.Assess(inputs); assessment.State != freshness.StateWithheld {
			t.Fatalf("%s: state = %q, want withheld", name, assessment.State)
		}
		if next := policy.NextChange(inputs); !next.IsZero() {
			t.Errorf("%s: next change = %s, want none once everything has expired", name, next)
		}
	}
}

// TestTheForecastSetsBothDeadlines pins the deadlines to the forecast sleep's
// latest likely wake and the grace ADR-0071 chose, and not to the age of the
// evidence.
func TestTheForecastSetsBothDeadlines(t *testing.T) {
	policy := freshness.Default()
	wake := expiryNow.Add(-time.Hour)
	latestWake := wake.Add(29 * time.Hour)
	inputs := freshness.Inputs{
		Now:                expiryNow,
		NewestEvidence:     wake,
		LatestSleepEnd:     wake,
		ExpectedSleepOnset: wake.Add(17 * time.Hour),
		ExpectedSleepEnd:   latestWake,
	}
	caution, withheld := policy.Deadlines(inputs)
	if !caution.Equal(latestWake) || !withheld.Equal(latestWake.Add(6*time.Hour)) {
		t.Fatalf("deadlines = %s and %s, want the latest likely wake (%s) and six hours on", caution, withheld, latestWake)
	}
	if next := policy.NextChange(inputs); !next.Equal(latestWake) {
		t.Errorf("next change = %s, want the latest likely wake", next)
	}
	inputs.Now = latestWake.Add(time.Minute)
	if next := policy.NextChange(inputs); !next.Equal(latestWake.Add(6 * time.Hour)) {
		t.Errorf("next change inside the grace = %s, want its end", next)
	}
}

// TestARunAtTheReturnedInstantSeesTheChange is the sharp edge the minute-by-
// minute walk cannot see. A wake scheduled for a deadline that the assessment
// treated as exclusive would find nothing changed, re-derive its next wake
// from the unchanged state, and leave the claim standing.
func TestARunAtTheReturnedInstantSeesTheChange(t *testing.T) {
	policy := freshness.Default()
	inputs := freshness.Inputs{
		Now:                expiryNow,
		NewestEvidence:     expiryNow,
		LatestSleepEnd:     expiryNow,
		ExpectedSleepOnset: expiryNow.Add(17 * time.Hour),
		ExpectedSleepEnd:   expiryNow.Add(29 * time.Hour),
	}
	want := []struct {
		state  freshness.State
		reason freshness.Reason
	}{
		{freshness.StateStale, freshness.ReasonSleepRecordDue},
		{freshness.StateWithheld, freshness.ReasonExpectedSleepUnrecorded},
	}
	for _, step := range want {
		next := policy.NextChange(inputs)
		if next.IsZero() {
			t.Fatalf("no change was scheduled before %q", step.state)
		}
		inputs.Now = next
		assessment := policy.Assess(inputs)
		if assessment.State != step.state || assessment.Reason != step.reason {
			t.Fatalf("at %s: state = %q reason = %q, want %q for %q", next, assessment.State, assessment.Reason, step.state, step.reason)
		}
	}
	if next := policy.NextChange(inputs); !next.IsZero() {
		t.Errorf("a change is still scheduled after the claim was withheld: %s", next)
	}
}
