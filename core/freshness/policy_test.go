package freshness

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 8, 4, 15, 0, 0, 0, time.UTC)

func TestFreshEvidenceAllowsAPlainClaim(t *testing.T) {
	got := Default().Assess(Inputs{Now: now, NewestEvidence: now.Add(-90 * time.Minute)})
	if got.State != StateCurrent {
		t.Fatalf("state = %q, want %q", got.State, StateCurrent)
	}
	if !got.MayClaimCurrentState() {
		t.Error("a fresh assessment refused a current-state claim")
	}
}

func TestAgingEvidenceIsShownWithItsAge(t *testing.T) {
	got := Default().Assess(Inputs{Now: now, NewestEvidence: now.Add(-7 * time.Hour)})
	if got.State != StateStale || got.Reason != ReasonEvidenceAging {
		t.Fatalf("state = %q reason = %q", got.State, got.Reason)
	}
	if !got.MayClaimCurrentState() {
		t.Error("stale evidence must still permit a qualified claim")
	}
	if !strings.Contains(got.Explanation, "7 hours") {
		t.Errorf("explanation does not state the age: %q", got.Explanation)
	}
}

func TestStaleEvidenceWithholdsTheClaim(t *testing.T) {
	got := Default().Assess(Inputs{Now: now, NewestEvidence: now.Add(-30 * time.Hour)})
	if got.State != StateWithheld || got.Reason != ReasonEvidenceStale {
		t.Fatalf("state = %q reason = %q", got.State, got.Reason)
	}
	if got.MayClaimCurrentState() {
		t.Error("a day-old record still permitted a current-state claim")
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
}

// TestUnrecordedExpectedSleepWithholds is the defect this package exists for.
// The evidence is only a few hours old, so no age threshold fires, but the
// estimate said sleep should have happened and nothing recorded it. Saying
// "likely awake" here is how the claim survived for days.
func TestUnrecordedExpectedSleepWithholds(t *testing.T) {
	got := Default().Assess(Inputs{
		Now:                now,
		NewestEvidence:     now.Add(-5 * time.Hour),
		ExpectedSleepOnset: now.Add(-9 * time.Hour),
		LatestSleepEnd:     now.Add(-20 * time.Hour),
	})
	if got.State != StateWithheld {
		t.Fatalf("state = %q, want the claim withheld", got.State)
	}
	if got.Reason != ReasonExpectedSleepUnrecorded {
		t.Fatalf("reason = %q, want %q", got.Reason, ReasonExpectedSleepUnrecorded)
	}
	if strings.Contains(strings.ToLower(got.Explanation), "awake") &&
		!strings.Contains(strings.ToLower(got.Explanation), "rather than awake") {
		t.Errorf("explanation should not assert wakefulness: %q", got.Explanation)
	}
}

// TestRecentSleepDischargesTheExpectation keeps the rule from firing on
// someone who slept exactly as predicted while the estimate lags behind.
func TestRecentSleepDischargesTheExpectation(t *testing.T) {
	got := Default().Assess(Inputs{
		Now:                now,
		NewestEvidence:     now.Add(-time.Hour),
		ExpectedSleepOnset: now.Add(-10 * time.Hour),
		LatestSleepEnd:     now.Add(-2 * time.Hour),
	})
	if got.State != StateCurrent {
		t.Fatalf("state = %q, want current after the sleep was recorded", got.State)
	}
}

// TestGraceAbsorbsOrdinaryForecastError checks the threshold against the
// measured onset error: a sleep two hours later than predicted is normal, not
// a reason to stop answering.
func TestGraceAbsorbsOrdinaryForecastError(t *testing.T) {
	got := Default().Assess(Inputs{
		Now:                now,
		NewestEvidence:     now.Add(-time.Hour),
		ExpectedSleepOnset: now.Add(-2 * time.Hour),
	})
	if got.State != StateCurrent {
		t.Errorf("state = %q; a 2 h late onset is inside measured error and must not withhold", got.State)
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
}

// TestExplanationsCarryNoPrivateVocabulary keeps every explanation safe to
// render on the public surface as well as the private ones.
func TestExplanationsCarryNoPrivateVocabulary(t *testing.T) {
	cases := []Inputs{
		{Now: now},
		{Now: now, NewestEvidence: now.Add(-30 * time.Hour)},
		{Now: now, NewestEvidence: now.Add(-7 * time.Hour)},
		{Now: now, NewestEvidence: now.Add(-time.Hour)},
		{Now: now, NewestEvidence: now.Add(-5 * time.Hour), ExpectedSleepOnset: now.Add(-9 * time.Hour)},
		{Now: now, NewestEvidence: now.Add(-time.Hour), SourcesConfigured: 1},
	}
	forbidden := []string{"medication", "dose", "task", "calendar", "diagnos", "disorder", "non-24", "melatonin"}
	for _, in := range cases {
		explanation := strings.ToLower(Default().Assess(in).Explanation)
		if explanation == "" {
			t.Error("an assessment produced no explanation")
		}
		for _, word := range forbidden {
			if strings.Contains(explanation, word) {
				t.Errorf("explanation mentions %q: %q", word, explanation)
			}
		}
	}
}
