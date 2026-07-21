package estimation

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestEstimatorFindsTwentyFourPointTwoHourRhythmAcrossCivilDates(t *testing.T) {
	sessions := syntheticSessions(14, time.Date(2026, 1, 1, 4, 0, 0, 0, time.UTC), 24*time.Hour+12*time.Minute, 8*time.Hour, "America/New_York")
	estimate, err := (RobustEstimator{}).Estimate(context.Background(), sessions, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if difference := math.Abs(float64(estimate.ObservedDriftPerCycle - 12*time.Minute)); difference > float64(time.Minute) {
		t.Fatalf("drift = %v", estimate.ObservedDriftPerCycle)
	}
	if len(estimate.PredictedSleepWindows) != 7 {
		t.Fatalf("forecast cycles = %d", len(estimate.PredictedSleepWindows))
	}
}

func TestEstimatorFindsStableNocturnalTwentyFourHourRhythm(t *testing.T) {
	sessions := syntheticSessions(12, time.Date(2026, 1, 1, 4, 0, 0, 0, time.UTC), 24*time.Hour, 8*time.Hour, "America/New_York")
	estimate, err := (RobustEstimator{}).Estimate(context.Background(), sessions, time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := estimate.ObservedDriftPerCycle.Round(time.Minute); got != 0 {
		t.Fatalf("stable rhythm drift = %v", got)
	}
}

func TestEstimatorFindsFreeRunningTwentyFiveAndHalfHourRhythm(t *testing.T) {
	sessions := syntheticSessions(12, time.Date(2026, 2, 1, 1, 0, 0, 0, time.UTC), 25*time.Hour+30*time.Minute, 8*time.Hour, "UTC")
	estimate, err := (RobustEstimator{}).Estimate(context.Background(), sessions, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got := estimate.ObservedCycleLength.Round(time.Minute); got != 25*time.Hour+30*time.Minute {
		t.Fatalf("period = %v", got)
	}
}

func TestEstimatorIgnoresNapsAndIndexesMissingDays(t *testing.T) {
	sessions := syntheticSessions(10, time.Date(2026, 3, 1, 2, 0, 0, 0, time.UTC), 25*time.Hour, 8*time.Hour, "UTC")
	sessions = append(sessions[:4], sessions[5:]...)
	nap := syntheticSessions(1, time.Date(2026, 3, 2, 18, 0, 0, 0, time.UTC), 24*time.Hour, 90*time.Minute, "UTC")[0]
	nap.IsNap = true
	sessions = append(sessions, nap)
	estimate, err := (RobustEstimator{}).Estimate(context.Background(), sessions, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got := estimate.ObservedCycleLength.Round(time.Minute); got != 25*time.Hour {
		t.Fatalf("period = %v", got)
	}
}

func TestEstimatorRefusesInsufficientData(t *testing.T) {
	_, err := (RobustEstimator{}).Estimate(context.Background(), syntheticSessions(3, time.Now().UTC(), 24*time.Hour, 8*time.Hour, "UTC"), time.Now().UTC())
	var refusal *EstimationRefusal
	if !errors.As(err, &refusal) || refusal.Code != RefusalInsufficientData {
		t.Fatalf("error = %#v", err)
	}
}

func TestRefusalCodesMatchV1Contract(t *testing.T) {
	codes := []RefusalCode{
		RefusalInsufficientData,
		RefusalAmbiguousCycleIndex,
		RefusalConflictingObservations,
		RefusalUnsupportedInput,
	}
	want := []string{
		"insufficient_data",
		"ambiguous_cycle_index",
		"conflicting_observations",
		"unsupported_input",
	}
	for i, code := range codes {
		if string(code) != want[i] {
			t.Fatalf("refusal code %d = %q, want %q", i, code, want[i])
		}
	}
}

func TestForecastUncertaintyWidens(t *testing.T) {
	sessions := syntheticSessions(12, time.Date(2026, 4, 1, 5, 0, 0, 0, time.UTC), 24*time.Hour+20*time.Minute, 8*time.Hour, "UTC")
	sessions[3].Intervals[0].Interval.Start.UTC = sessions[3].Intervals[0].Interval.Start.UTC.Add(45 * time.Minute)
	estimate, err := (RobustEstimator{}).Estimate(context.Background(), sessions, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	first := estimate.PredictedSleepWindows[0].Interval.Duration()
	last := estimate.PredictedSleepWindows[len(estimate.PredictedSleepWindows)-1].Interval.Duration()
	if last <= first {
		t.Fatalf("last uncertainty window %v did not exceed first %v", last, first)
	}
}

func TestDSTDoesNotCreateArtificialDrift(t *testing.T) {
	sessions := syntheticSessions(10, time.Date(2026, 3, 5, 8, 0, 0, 0, time.UTC), 24*time.Hour, 8*time.Hour, "America/New_York")
	estimate, err := (RobustEstimator{}).Estimate(context.Background(), sessions, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(estimate.ObservedDriftPerCycle)) > float64(time.Minute) {
		t.Fatalf("DST introduced drift %v", estimate.ObservedDriftPerCycle)
	}
}

func TestTemporaryEntrainmentThenRenewedDriftLowersConfidence(t *testing.T) {
	start := time.Date(2026, 5, 1, 3, 0, 0, 0, time.UTC)
	periods := []time.Duration{
		24 * time.Hour, 24 * time.Hour, 24 * time.Hour, 24 * time.Hour, 24 * time.Hour, 24 * time.Hour, 24 * time.Hour,
		25*time.Hour + 30*time.Minute, 25*time.Hour + 30*time.Minute, 25*time.Hour + 30*time.Minute, 25*time.Hour + 30*time.Minute,
		25*time.Hour + 30*time.Minute, 25*time.Hour + 30*time.Minute, 25*time.Hour + 30*time.Minute, 25*time.Hour + 30*time.Minute,
	}
	sessions := make([]domain.SleepSession, 0, len(periods)+1)
	current := start
	for i := 0; i <= len(periods); i++ {
		sessions = append(sessions, syntheticSessions(1, current, 24*time.Hour, 8*time.Hour, "UTC")[0])
		sessions[len(sessions)-1].ID = domain.SleepSessionID(time.Duration(i).String())
		if i < len(periods) {
			current = current.Add(periods[i])
		}
	}
	estimate, err := (RobustEstimator{}).Estimate(context.Background(), sessions, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if estimate.Confidence.Level != domain.ConfidenceLow {
		t.Fatalf("confidence = %s, reasons = %v", estimate.Confidence.Level, estimate.Confidence.Reasons)
	}
	foundReason := false
	for _, reason := range estimate.Confidence.Reasons {
		if strings.Contains(reason, "slopes differ") {
			foundReason = true
		}
	}
	if !foundReason {
		t.Fatalf("missing change explanation: %v", estimate.Confidence.Reasons)
	}
}

func TestUncertaintyScaleTightensForecastWithoutChangingTheFit(t *testing.T) {
	sessions := syntheticSessions(16, time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC), 25*time.Hour, 8*time.Hour, "UTC")
	asOf := sessions[len(sessions)-1].Intervals[0].Interval.End.UTC
	baseline, err := (RobustEstimator{}).Estimate(context.Background(), sessions, asOf)
	if err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.UncertaintyScale = 0.5
	tightened, err := (RobustEstimator{Config: config}).Estimate(context.Background(), sessions, asOf)
	if err != nil {
		t.Fatal(err)
	}
	baselineWindow := baseline.PredictedSleepWindows[0].Interval
	tightenedWindow := tightened.PredictedSleepWindows[0].Interval
	if tightenedWindow.Duration() >= baselineWindow.Duration() {
		t.Fatalf("tightened window duration = %s, baseline = %s", tightenedWindow.Duration(), baselineWindow.Duration())
	}
	if !tightened.CharacteristicSleepStart.UTC.Equal(baseline.CharacteristicSleepStart.UTC) || tightened.ObservedCycleLength != baseline.ObservedCycleLength {
		t.Fatalf("uncertainty scale changed the fitted rhythm: baseline=%#v tightened=%#v", baseline, tightened)
	}
}

func syntheticSessions(count int, first time.Time, period, duration time.Duration, zone string) []domain.SleepSession {
	sessions := make([]domain.SleepSession, 0, count)
	for i := 0; i < count; i++ {
		start := first.Add(time.Duration(i) * period)
		evidence := domain.Evidence{Acquisition: domain.AcquisitionManual, Status: domain.StatusUserConfirmed}
		sessions = append(sessions, domain.SleepSession{
			ID: domain.SleepSessionID(time.Duration(i).String()),
			Intervals: []domain.SleepInterval{{
				Interval: domain.TimeRange{
					Start: domain.MustZonedInstant(start, zone),
					End:   domain.MustZonedInstant(start.Add(duration), zone),
				},
				StartEvidence: evidence,
				EndEvidence:   evidence,
			}},
		})
	}
	return sessions
}
