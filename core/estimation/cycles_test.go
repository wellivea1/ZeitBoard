package estimation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"non24.app/core/domain"
)

// A rhythm of 27 hours with nights missing from the record. Counted against
// 24 hours, each of these either refused the estimate for weeks or fitted the
// wrong period; nights go unrecorded all the time, so they are ordinary input.

const longCycle = 27 * time.Hour

// rhythmWithout is a regular rhythm from which the nights at the given cycle
// numbers are missing.
func rhythmWithout(cycles int, period time.Duration, missing ...int) []domain.SleepSession {
	skip := map[int]bool{}
	for _, index := range missing {
		skip[index] = true
	}
	first := time.Date(2026, 3, 1, 22, 0, 0, 0, time.UTC)
	var sessions []domain.SleepSession
	for cycle := 0; cycle < cycles; cycle++ {
		if skip[cycle] {
			continue
		}
		night := syntheticSessions(1, first.Add(time.Duration(cycle)*period), period, 8*time.Hour, "UTC")[0]
		night.ID = domain.SleepSessionID(fmt.Sprintf("night-%02d", cycle))
		sessions = append(sessions, night)
	}
	return sessions
}

func estimateAfter(t *testing.T, sessions []domain.SleepSession) (domain.PhaseEstimate, error) {
	t.Helper()
	last := sessions[len(sessions)-1].Intervals[0].Interval.End.UTC
	return RobustEstimator{}.Estimate(context.Background(), sessions, last)
}

func wantPeriod(t *testing.T, estimate domain.PhaseEstimate, want time.Duration) {
	t.Helper()
	if difference := estimate.ObservedCycleLength - want; difference < -time.Minute || difference > time.Minute {
		t.Fatalf("fitted cycle length %s, want %s", estimate.ObservedCycleLength, want)
	}
}

func TestTwoMissingNightsDoNotStopA27HourRhythm(t *testing.T) {
	// Two nights missing in a row leave an 81-hour gap: three cycles of 27
	// hours, and neither three nor four of 24.
	sessions := rhythmWithout(14, longCycle, 8, 9)
	estimate, err := estimateAfter(t, sessions)
	if err != nil {
		t.Fatalf("a 27-hour rhythm with two nights missing was refused: %v", err)
	}
	wantPeriod(t, estimate, longCycle)
	if len(estimate.InputSessionIDs) != len(sessions) {
		t.Errorf("the fit used %d of %d nights; the gap is countable, so it should use them all",
			len(estimate.InputSessionIDs), len(sessions))
	}
}

func TestALongGapIsCountedInTheRhythmsOwnCycles(t *testing.T) {
	// Five nights missing leave 162 hours: six cycles of 27, which read as
	// seven of 24 and bent the fit to about 25 hours 43 minutes.
	sessions := rhythmWithout(18, longCycle, 7, 8, 9, 10, 11)
	estimate, err := estimateAfter(t, sessions)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	wantPeriod(t, estimate, longCycle)
}

func TestAnUncountableGapIsLeftBehindOnceEnoughNightsFollowIt(t *testing.T) {
	// A gap of a cycle and a half cannot be counted in any rhythm: the night
	// after it is shifted by 13 hours, as after a flight or an illness.
	shifted := func(after int) []domain.SleepSession {
		before := rhythmWithout(10, longCycle)
		gap := 13 * time.Hour
		start := before[len(before)-1].Intervals[0].Interval.Start.UTC.Add(longCycle + gap)
		later := syntheticSessions(after, start, longCycle, 8*time.Hour, "UTC")
		for index := range later {
			later[index].ID = domain.SleepSessionID(fmt.Sprintf("later-%02d", index))
		}
		return append(before, later...)
	}

	// With too few nights since the gap, the estimate is refused, and says how
	// far there is to go.
	_, err := estimateAfter(t, shifted(4))
	var refusal *EstimationRefusal
	if !errors.As(err, &refusal) || refusal.Code != RefusalAmbiguousCycleIndex {
		t.Fatalf("four nights after an uncountable gap gave %v, want an ambiguous-cycle refusal", err)
	}
	if !strings.Contains(refusal.Message, "4 of the 7 nights") {
		t.Errorf("refusal %q does not say how many nights are recorded since the gap", refusal.Message)
	}

	// Seven nights since the gap are a rhythm in their own right. This used
	// to stay refused until the gap left the 21-night window.
	sessions := shifted(7)
	estimate, err := estimateAfter(t, sessions)
	if err != nil {
		t.Fatalf("seven clean nights after the gap were still refused: %v", err)
	}
	wantPeriod(t, estimate, longCycle)
	if len(estimate.InputSessionIDs) != 7 || estimate.InputSessionIDs[0] != "later-00" {
		t.Errorf("the fit used %v, want only the seven nights since the gap", estimate.InputSessionIDs)
	}
	said := false
	for _, reason := range estimate.Confidence.Reasons {
		if strings.Contains(reason, "since a gap") {
			said = true
		}
	}
	if !said {
		t.Errorf("confidence reasons %v do not say the fit rests on the nights since a gap", estimate.Confidence.Reasons)
	}
}

func TestCyclesAreCountedAgainstADayWhenTheRecordsCannotSayOtherwise(t *testing.T) {
	// Every gap here spans two cycles, so none says what one cycle is. The
	// count then falls back on the day, as it always did.
	sessions := rhythmWithout(16, 24*time.Hour, 1, 3, 5, 7, 9, 11, 13, 15)
	counted, err := countCycles(sessions)
	if err != nil {
		t.Fatal(err)
	}
	if counted.period != 24*time.Hour || counted.uncountedAt != 0 {
		t.Fatalf("period %s, uncounted at %d; want a day and every gap counted", counted.period, counted.uncountedAt)
	}
	if last := counted.indices[len(counted.indices)-1]; last != 14 {
		t.Errorf("the last night is numbered %d, want 14", last)
	}
}

func TestALongNapDoesNotCountAsACycle(t *testing.T) {
	// A wearable reports every session as main sleep. A three-hour sleep twelve
	// hours after a night's start is not the next cycle, and used to stop the
	// estimate as a cycle of twelve hours.
	sessions := rhythmWithout(12, longCycle)
	napStart := sessions[6].Intervals[0].Interval.Start.UTC.Add(12 * time.Hour)
	nap := syntheticSessions(1, napStart, longCycle, 3*time.Hour, "UTC")[0]
	nap.ID = "long-nap"
	withNap := append(append([]domain.SleepSession{}, sessions...), nap)

	estimate, err := estimateAfter(t, withNap)
	if err != nil {
		t.Fatalf("a long nap stopped the estimate: %v", err)
	}
	wantPeriod(t, estimate, longCycle)
	for _, id := range estimate.InputSessionIDs {
		if id == "long-nap" {
			t.Fatal("the nap was fitted as a cycle's sleep")
		}
	}

	// The longer of two sleeps in one cycle is the one kept, whichever came
	// first: a short sleep before the night does not displace the night.
	early := syntheticSessions(1, sessions[6].Intervals[0].Interval.Start.UTC.Add(-10*time.Hour), longCycle, 3*time.Hour, "UTC")[0]
	early.ID = "early-nap"
	estimate, err = estimateAfter(t, append(append([]domain.SleepSession{}, sessions...), early))
	if err != nil {
		t.Fatalf("a nap before the night stopped the estimate: %v", err)
	}
	wantPeriod(t, estimate, longCycle)
}
