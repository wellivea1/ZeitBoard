package freshness

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 8, 4, 15, 0, 0, 0, time.UTC)

// day is one cycle of a source that reports a night once it is over: the
// record arrives at the wake, the forecast sleep opens awake hours later at
// the earliest, and its latest likely wake closes the cycle.
type day struct {
	awake   time.Duration // wake to the earliest likely onset
	asleep  time.Duration // earliest likely onset to the latest likely wake
	elapsed time.Duration // since the wake record
}

func (d day) inputs() Inputs {
	wake := now.Add(-d.elapsed)
	return Inputs{
		Now:                now,
		NewestEvidence:     wake,
		LatestSleepEnd:     wake,
		ExpectedSleepOnset: wake.Add(d.awake),
		ExpectedSleepEnd:   wake.Add(d.awake + d.asleep),
	}
}

// The owner's choice (ADR-0071): the records are current for as long as the
// day goes the way the forecast expects, whatever the cycle length. The old
// rule called the same hours uncertain from the sixth, and showed nothing in
// the second half of each sleep.
func TestTheRecordsAreCurrentWhileTheDayGoesAsForecast(t *testing.T) {
	for name, cycle := range map[string]day{
		"a 24-hour day": {awake: 15 * time.Hour, asleep: 11 * time.Hour},
		"a 27-hour day": {awake: 17 * time.Hour, asleep: 12 * time.Hour},
		"a 30-hour day": {awake: 19 * time.Hour, asleep: 13 * time.Hour},
	} {
		whole := cycle.awake + cycle.asleep
		for elapsed := time.Duration(0); elapsed < whole; elapsed += 30 * time.Minute {
			cycle.elapsed = elapsed
			got := Default().Assess(cycle.inputs())
			if got.State != StateCurrent || got.Reason != ReasonFresh {
				t.Fatalf("%s, %s after the wake record: state = %q reason = %q", name, elapsed, got.State, got.Reason)
			}
		}
	}
}

// Once the latest likely wake has passed with no record of the sleep, the
// claim stands with a caution for six hours. A watch that has not synced yet
// looks exactly like this.
func TestAnOverdueRecordIsACautionForTheGrace(t *testing.T) {
	cycle := day{awake: 17 * time.Hour, asleep: 12 * time.Hour}
	for _, past := range []time.Duration{0, time.Minute, 3 * time.Hour, 6*time.Hour - time.Minute} {
		cycle.elapsed = cycle.awake + cycle.asleep + past
		got := Default().Assess(cycle.inputs())
		if got.State != StateStale || got.Reason != ReasonSleepRecordDue {
			t.Fatalf("%s past the latest likely wake: state = %q reason = %q", past, got.State, got.Reason)
		}
		if !got.MayClaimCurrentState() {
			t.Fatalf("%s past the latest likely wake: the claim was dropped inside the grace", past)
		}
		if !strings.Contains(got.Explanation, "caution") {
			t.Fatalf("the caution is not said: %q", got.Explanation)
		}
	}
}

// TestUnrecordedExpectedSleepWithholds is the defect this package exists for:
// the forecast sleep is over, the grace has run out, and nothing recorded it.
// Saying "likely awake" here is how the claim survived for days.
func TestUnrecordedExpectedSleepWithholds(t *testing.T) {
	cycle := day{awake: 17 * time.Hour, asleep: 12 * time.Hour}
	for _, past := range []time.Duration{6 * time.Hour, 7 * time.Hour, 5 * 24 * time.Hour} {
		cycle.elapsed = cycle.awake + cycle.asleep + past
		got := Default().Assess(cycle.inputs())
		if got.State != StateWithheld || got.Reason != ReasonExpectedSleepUnrecorded {
			t.Fatalf("%s past the latest likely wake: state = %q reason = %q", past, got.State, got.Reason)
		}
		if got.MayClaimCurrentState() {
			t.Fatalf("%s past the latest likely wake: a claim was still made", past)
		}
		if strings.Contains(strings.ToLower(got.Explanation), "awake") {
			t.Errorf("explanation should not assert wakefulness: %q", got.Explanation)
		}
	}
}

// A record entered today about a night last week is fresh evidence, but the
// forecast it leaves behind is for a sleep long over. The age of the record
// must not carry a claim the forecast cannot.
func TestAFreshRecordOfAnOldNightDoesNotMakeTheForecastCurrent(t *testing.T) {
	got := Default().Assess(Inputs{
		Now:                now,
		NewestEvidence:     now.Add(-time.Minute),
		LatestSleepEnd:     now.Add(-6 * 24 * time.Hour),
		ExpectedSleepOnset: now.Add(-5*24*time.Hour - 8*time.Hour),
		ExpectedSleepEnd:   now.Add(-5 * 24 * time.Hour),
	})
	if got.State != StateWithheld || got.Reason != ReasonExpectedSleepUnrecorded {
		t.Fatalf("state = %q reason = %q", got.State, got.Reason)
	}
}

// TestRecentSleepDischargesTheExpectation keeps the rule from firing on
// someone who slept as predicted while the estimate lags behind. With the
// expectation spent there is no forecast to follow, so the age decides.
func TestRecentSleepDischargesTheExpectation(t *testing.T) {
	in := Inputs{
		Now:                now,
		NewestEvidence:     now.Add(-time.Hour),
		ExpectedSleepOnset: now.Add(-20 * time.Hour),
		ExpectedSleepEnd:   now.Add(-8 * time.Hour),
		LatestSleepEnd:     now.Add(-2 * time.Hour),
	}
	if got := Default().Assess(in); got.State != StateCurrent {
		t.Fatalf("state = %q, want current after the sleep was recorded", got.State)
	}
	in.NewestEvidence = now.Add(-7 * time.Hour)
	if got := Default().Assess(in); got.State != StateStale || got.Reason != ReasonEvidenceAging {
		t.Fatalf("with the expectation spent the age decides: state = %q reason = %q", got.State, got.Reason)
	}
}

// Without a forecast the fixed hours of ADR-0031 still hold.
func TestWithoutAForecastTheAgeDecides(t *testing.T) {
	fresh := Default().Assess(Inputs{Now: now, NewestEvidence: now.Add(-90 * time.Minute)})
	if fresh.State != StateCurrent || !fresh.MayClaimCurrentState() {
		t.Fatalf("fresh: state = %q", fresh.State)
	}

	aging := Default().Assess(Inputs{Now: now, NewestEvidence: now.Add(-7 * time.Hour)})
	if aging.State != StateStale || aging.Reason != ReasonEvidenceAging {
		t.Fatalf("aging: state = %q reason = %q", aging.State, aging.Reason)
	}
	if !aging.MayClaimCurrentState() {
		t.Error("stale evidence must still permit a qualified claim")
	}
	if !strings.Contains(aging.Explanation, "7 hours") {
		t.Errorf("explanation does not state the age: %q", aging.Explanation)
	}

	old := Default().Assess(Inputs{Now: now, NewestEvidence: now.Add(-30 * time.Hour)})
	if old.State != StateWithheld || old.Reason != ReasonEvidenceStale {
		t.Fatalf("old: state = %q reason = %q", old.State, old.Reason)
	}
	if old.MayClaimCurrentState() {
		t.Error("a day-old record still permitted a current-state claim")
	}

	// A window that does not run forward is no forecast.
	backwards := Default().Assess(Inputs{
		Now: now, NewestEvidence: now.Add(-7 * time.Hour),
		ExpectedSleepOnset: now.Add(time.Hour), ExpectedSleepEnd: now.Add(time.Hour),
	})
	if backwards.Reason != ReasonEvidenceAging {
		t.Fatalf("a window with no length was followed: reason = %q", backwards.Reason)
	}
}

func TestNoEvidenceIsDistinctFromOldEvidence(t *testing.T) {
	got := Default().Assess(Inputs{Now: now})
	if got.Reason != ReasonNoEvidence {
		t.Fatalf("reason = %q, want %q", got.Reason, ReasonNoEvidence)
	}
	if got.HasEvidence {
		t.Error("HasEvidence must be false when nothing was ever recorded")
	}
	if caution, withheld := Default().Deadlines(Inputs{Now: now}); !caution.IsZero() || !withheld.IsZero() {
		t.Errorf("no evidence has deadlines: %s, %s", caution, withheld)
	}
}

func TestConfiguredButSilentSourcesWithhold(t *testing.T) {
	got := Default().Assess(Inputs{
		Now:               now,
		NewestEvidence:    now.Add(-time.Hour),
		SourcesConfigured: 2,
		SourcesReporting:  0,
	})
	if got.State != StateWithheld || got.Reason != ReasonNoSourcesReporting {
		t.Fatalf("state = %q reason = %q", got.State, got.Reason)
	}

	// No configured source is a setup state, not a failure.
	quiet := Default().Assess(Inputs{Now: now, NewestEvidence: now.Add(-time.Hour)})
	if quiet.State != StateCurrent {
		t.Errorf("state = %q; having no sources configured must not withhold", quiet.State)
	}
}

// TestEvidenceFromTheFutureDoesNotProduceNegativeAge guards a clock-skew case
// that would otherwise render as a nonsensical age.
func TestEvidenceFromTheFutureDoesNotProduceNegativeAge(t *testing.T) {
	got := Default().Assess(Inputs{Now: now, NewestEvidence: now.Add(2 * time.Hour)})
	if got.EvidenceAge < 0 {
		t.Errorf("age = %v, want it clamped to zero", got.EvidenceAge)
	}
	if got.State != StateCurrent {
		t.Errorf("state = %q", got.State)
	}
}

func TestZeroPolicyFallsBackToDefault(t *testing.T) {
	got := Policy{}.Assess(Inputs{Now: now, NewestEvidence: now.Add(-30 * time.Hour)})
	if got.State != StateWithheld {
		t.Errorf("an unconfigured policy did not apply the default thresholds: %q", got.State)
	}
	cycle := day{awake: 17 * time.Hour, asleep: 12 * time.Hour, elapsed: 36 * time.Hour}
	if got := (Policy{StaleAfter: time.Hour}).Assess(cycle.inputs()); got.State != StateWithheld {
		t.Errorf("a half-configured policy did not apply the default grace: %q", got.State)
	}
}

// TestExplanationsCarryNoPrivateVocabulary keeps every explanation safe to
// render on the public surface as well as the private ones.
func TestExplanationsCarryNoPrivateVocabulary(t *testing.T) {
	cycle := day{awake: 17 * time.Hour, asleep: 12 * time.Hour}
	due, gone := cycle, cycle
	due.elapsed, gone.elapsed = 30*time.Hour, 40*time.Hour
	cases := []Inputs{
		{Now: now},
		{Now: now, NewestEvidence: now.Add(-30 * time.Hour)},
		{Now: now, NewestEvidence: now.Add(-7 * time.Hour)},
		{Now: now, NewestEvidence: now.Add(-time.Hour)},
		due.inputs(),
		gone.inputs(),
		{Now: now, NewestEvidence: now.Add(-time.Hour), SourcesConfigured: 1},
	}
	forbidden := []string{"medication", "dose", "task", "calendar", "diagnos", "disorder", "non-24", "melatonin"}
	seen := map[Reason]bool{}
	for _, in := range cases {
		assessment := Default().Assess(in)
		seen[assessment.Reason] = true
		explanation := strings.ToLower(assessment.Explanation)
		if explanation == "" {
			t.Error("an assessment produced no explanation")
		}
		for _, word := range forbidden {
			if strings.Contains(explanation, word) {
				t.Errorf("explanation mentions %q: %q", word, explanation)
			}
		}
	}
	for _, reason := range []Reason{
		ReasonFresh, ReasonNoEvidence, ReasonEvidenceAging, ReasonEvidenceStale,
		ReasonSleepRecordDue, ReasonExpectedSleepUnrecorded, ReasonNoSourcesReporting,
	} {
		if !seen[reason] {
			t.Errorf("no case reaches the reason %q", reason)
		}
	}
}
