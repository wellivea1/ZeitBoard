package inference

import (
	"strings"
	"testing"
	"time"

	"non24.app/core/platform/activity"
)

var night = time.Date(2026, 8, 4, 23, 0, 0, 0, time.UTC)

func quietInterval(start time.Time, d time.Duration) Interval {
	return Interval{Source: SourceDesktopActivity, StartAt: start, EndAt: start.Add(d)}
}

func TestQuietDesktopBecomesACandidate(t *testing.T) {
	got, refusal := Build([]Interval{quietInterval(night, 8*time.Hour)}, night.Add(9*time.Hour))
	if refusal != nil {
		t.Fatalf("refused: %v", refusal)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %d, want 1", len(got))
	}
	if got[0].AlgorithmVersion != AlgorithmVersion {
		t.Errorf("algorithm version = %q", got[0].AlgorithmVersion)
	}
	// Inactivity brackets sleep rather than marking it, so the boundaries must
	// carry uncertainty rather than being asserted as the sleep itself.
	if got[0].StartUncertainty == 0 || got[0].EndUncertainty == 0 {
		t.Error("a desktop-only candidate claimed exact boundaries")
	}
}

// TestShortAndLongQuietSpansAreRefused keeps an evening on the sofa and a
// weekend away from both becoming sleep.
func TestShortAndLongQuietSpansAreRefused(t *testing.T) {
	for name, duration := range map[string]time.Duration{
		"an evening":  2 * time.Hour,
		"a long trip": 48 * time.Hour,
	} {
		_, refusal := Build([]Interval{quietInterval(night, duration)}, night.Add(duration+time.Hour))
		if refusal == nil {
			t.Errorf("%s produced a candidate", name)
			continue
		}
		if refusal.Code != RefusalNoQuietInterval {
			t.Errorf("%s refusal code = %q", name, refusal.Code)
		}
	}
}

func TestNoEvidenceRefusesWithATypedCode(t *testing.T) {
	_, refusal := Build(nil, night)
	if refusal == nil || refusal.Code != RefusalNoEvidence {
		t.Fatalf("refusal = %v, want %q", refusal, RefusalNoEvidence)
	}
}

// TestWearableAgreementTightensTheBoundaries: a source that observed sleep
// directly knows its boundaries better than inactivity does.
func TestWearableAgreementTightensTheBoundaries(t *testing.T) {
	got, refusal := Build([]Interval{
		quietInterval(night, 8*time.Hour),
		{Source: SourceWearableSleep, StartAt: night.Add(30 * time.Minute), EndAt: night.Add(7 * time.Hour), Asleep: true},
	}, night.Add(9*time.Hour))
	if refusal != nil {
		t.Fatalf("refused: %v", refusal)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %d, want the two sources to produce one", len(got))
	}
	if len(got[0].Supporting) != 2 {
		t.Errorf("supporting = %v, want both sources", got[0].Supporting)
	}
	if got[0].StartUncertainty >= 45*time.Minute {
		t.Errorf("start uncertainty = %v; agreement should tighten it", got[0].StartUncertainty)
	}
}

// TestDisagreementIsRecordedNotDiscarded is the property that makes a later
// correction prompt possible: the conflict must survive into the candidate.
func TestDisagreementIsRecordedNotDiscarded(t *testing.T) {
	got, refusal := Build([]Interval{
		quietInterval(night, 8*time.Hour),
		{
			Source:  SourceWearableSleep,
			StartAt: night.Add(-9 * time.Hour),
			EndAt:   night.Add(-2 * time.Hour),
			Asleep:  true,
		},
	}, night.Add(9*time.Hour))
	if refusal != nil {
		t.Fatalf("refused: %v", refusal)
	}
	var conflicted bool
	for _, candidate := range got {
		if len(candidate.Conflicting) > 0 {
			conflicted = true
		}
	}
	if !conflicted {
		t.Error("two sources disagreeing about the same night recorded no conflict")
	}
}

func TestDescribeNamesTheDisagreement(t *testing.T) {
	candidate := Candidate{
		StartAt:     night,
		EndAt:       night.Add(7 * time.Hour),
		Supporting:  []SourceKind{SourceDesktopActivity},
		Conflicting: []SourceKind{SourceWearableSleep},
	}
	described := candidate.Describe()
	if !strings.Contains(described, "disagrees") {
		t.Errorf("description hides the conflict: %q", described)
	}
	// The prompt must be answerable by a person, not by someone reading raw
	// source identifiers.
	if strings.Contains(described, "desktop_activity") || strings.Contains(described, "wearable_sleep") {
		t.Errorf("description exposes internal source ids: %q", described)
	}
}

// TestShutdownDoesNotOpenAQuietInterval stops a holiday from becoming a
// fortnight of sleep: the machine being off says nothing about the person.
func TestShutdownDoesNotOpenAQuietInterval(t *testing.T) {
	intervals := FromActivity([]activity.Transition{
		{At: night, State: activity.StateShutdown},
		{At: night.Add(14 * 24 * time.Hour), State: activity.StateStartup},
	})
	for _, interval := range intervals {
		if interval.EndAt.Sub(interval.StartAt) > MaximumDuration {
			t.Errorf("a shutdown produced a %v quiet interval", interval.EndAt.Sub(interval.StartAt))
		}
	}
}

func TestFromActivityPairsIdleWithTheReturnToUse(t *testing.T) {
	intervals := FromActivity([]activity.Transition{
		{At: night, State: activity.StateActive},
		{At: night.Add(time.Hour), State: activity.StateIdle},
		{At: night.Add(8 * time.Hour), State: activity.StateActive},
	})
	if len(intervals) != 1 {
		t.Fatalf("intervals = %d, want 1", len(intervals))
	}
	if !intervals[0].StartAt.Equal(night.Add(time.Hour)) {
		t.Errorf("quiet started at %v", intervals[0].StartAt)
	}
	if intervals[0].Asleep {
		t.Error("desktop inactivity must not assert sleep on its own")
	}
}

// TestSuspendCountsAsQuiet: a suspended machine is not in use, and the gap is
// among the strongest desktop-side signals available.
func TestSuspendCountsAsQuiet(t *testing.T) {
	intervals := FromActivity([]activity.Transition{
		{At: night, State: activity.StateSuspended},
		{At: night.Add(7 * time.Hour), State: activity.StateResumed},
	})
	if len(intervals) != 1 || intervals[0].EndAt.Sub(intervals[0].StartAt) != 7*time.Hour {
		t.Fatalf("intervals = %+v, want one seven-hour quiet span", intervals)
	}
}

// Moments of account activity leave quiet stretches between them; only one
// long enough to hold a principal sleep becomes a quiet interval, and Build
// treats it as it treats an unused computer (ADR-0059).
func TestAGapInAccountActivitySuggestsANight(t *testing.T) {
	base := time.Date(2026, 3, 1, 22, 0, 0, 0, time.UTC)
	moments := []time.Time{
		base.Add(30 * time.Minute), // after a short gap, nothing
		base,
		base.Add(8 * time.Hour), // the night: 30 minutes past 22:00 to 06:00
		base.Add(8*time.Hour + 2*time.Hour),
		base.Add(30*time.Hour + 20*time.Hour), // a 20-hour gap is absence, not a night
	}
	quiet := FromMoments(SourceAccountActivity, moments)
	if len(quiet) != 2 {
		t.Fatalf("quiet intervals %+v, want the night and the long gap", quiet)
	}
	if !quiet[0].StartAt.Equal(base.Add(30*time.Minute)) || !quiet[0].EndAt.Equal(base.Add(8*time.Hour)) {
		t.Errorf("the night reads %s to %s", quiet[0].StartAt, quiet[0].EndAt)
	}

	recorded := Interval{
		Source: SourceUserConfirmed, Asleep: true,
		StartAt: base.Add(time.Hour), EndAt: base.Add(7 * time.Hour),
	}
	candidates, refusal := Build(append(quiet, recorded), base.Add(48*time.Hour))
	if refusal != nil {
		t.Fatal(refusal)
	}
	// The recorded night is the stronger claim and the gap corroborates it;
	// the twenty-hour gap is too long to be a night.
	if len(candidates) != 1 || len(candidates[0].Supporting) != 2 ||
		candidates[0].Supporting[1] != SourceAccountActivity {
		t.Fatalf("candidates %+v", candidates)
	}

	alone, refusal := Build(quiet[:1], base.Add(48*time.Hour))
	if refusal != nil || len(alone) != 1 || alone[0].Supporting[0] != SourceAccountActivity ||
		!strings.Contains(alone[0].Describe(), "a gap in your account's activity") {
		t.Fatalf("a gap alone reads %+v, %v", alone, refusal)
	}
}
