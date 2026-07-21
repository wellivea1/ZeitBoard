package estimation

import (
	"context"
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestBacktestScoresACleanFreeRunningRhythmWell(t *testing.T) {
	// A perfectly linear 24.8h drift: the robust fit should recover it, so
	// predictions land on the actual onsets and the forecast windows contain them.
	sessions := syntheticSessions(16, time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC), 24*time.Hour+48*time.Minute, 8*time.Hour, "UTC")
	report, err := (RobustEstimator{}).Backtest(context.Background(), sessions)
	if err != nil {
		t.Fatal(err)
	}
	if report.Evaluations == 0 {
		t.Fatal("expected at least one evaluable split")
	}
	if report.MedianAbsErrorHours > 0.25 {
		t.Fatalf("clean rhythm median error too high: %v h", report.MedianAbsErrorHours)
	}
	if report.HitRate < 0.9 {
		t.Fatalf("clean rhythm hit-rate too low: %v", report.HitRate)
	}
	if len(report.Calibration) == 0 {
		t.Fatal("expected calibration buckets")
	}
	for _, bucket := range report.Calibration {
		if bucket.Count <= 0 || bucket.HitRate < 0 || bucket.HitRate > 1 {
			t.Fatalf("invalid calibration bucket: %#v", bucket)
		}
	}
}

func TestBacktestDetectsLinearModelMisfit(t *testing.T) {
	zone := "UTC"
	clean := syntheticSessions(16, time.Date(2026, 2, 1, 2, 0, 0, 0, time.UTC), 24*time.Hour+48*time.Minute, 8*time.Hour, zone)
	cleanReport, err := (RobustEstimator{}).Backtest(context.Background(), clean)
	if err != nil {
		t.Fatal(err)
	}

	// Relative coordination: a +3h phase jump partway through breaks the constant
	// drift the linear model assumes. The harness must show degraded accuracy.
	perturbed := syntheticSessions(16, time.Date(2026, 2, 1, 2, 0, 0, 0, time.UTC), 24*time.Hour+48*time.Minute, 8*time.Hour, zone)
	for i := 8; i < len(perturbed); i++ {
		perturbed[i] = shiftSession(perturbed[i], 3*time.Hour, zone)
	}
	perturbedReport, err := (RobustEstimator{}).Backtest(context.Background(), perturbed)
	if err != nil {
		t.Fatal(err)
	}

	if perturbedReport.Evaluations == 0 {
		t.Fatal("expected evaluable splits on the perturbed rhythm")
	}
	if perturbedReport.MedianAbsErrorHours <= cleanReport.MedianAbsErrorHours {
		t.Fatalf("expected the perturbation to raise error: clean %v h, perturbed %v h",
			cleanReport.MedianAbsErrorHours, perturbedReport.MedianAbsErrorHours)
	}
	if perturbedReport.P90AbsErrorHours < 1 {
		t.Fatalf("expected a large tail error after a 3h jump, got p90 %v h", perturbedReport.P90AbsErrorHours)
	}
}

func TestBacktestRefusesWithoutEnoughHistory(t *testing.T) {
	sessions := syntheticSessions(6, time.Now().UTC(), 25*time.Hour, 8*time.Hour, "UTC")
	_, err := (RobustEstimator{}).Backtest(context.Background(), sessions)
	if err == nil {
		t.Fatal("expected a refusal with too little history")
	}
}

func TestBacktestCountsAmbiguousGapsAndContinuesOnLaterCleanHistory(t *testing.T) {
	zone := "America/New_York"
	first := syntheticSessions(30, time.Date(2023, 1, 1, 5, 0, 0, 0, time.UTC), 25*time.Hour, 8*time.Hour, zone)
	secondStart := first[len(first)-1].Intervals[0].Interval.Start.UTC.Add(36 * time.Hour)
	second := syntheticSessions(30, secondStart, 25*time.Hour, 8*time.Hour, zone)
	sessions := append(first, second...)

	report, err := (RobustEstimator{}).Backtest(context.Background(), sessions)
	if err != nil {
		t.Fatal(err)
	}
	if report.Evaluations == 0 || report.Refusals == 0 {
		t.Fatalf("expected both evaluations and honest refusals: %#v", report)
	}
	if report.Evaluations+report.Refusals != len(sessions)-DefaultConfig().MinimumEpisodes {
		t.Fatalf("backtest did not account for every holdout: evaluations=%d refusals=%d", report.Evaluations, report.Refusals)
	}
	if len(report.RefusalPoints) != report.Refusals || report.RefusalPoints[0].Code != RefusalAmbiguousCycleIndex {
		t.Fatalf("refusal details were not retained: %#v", report.RefusalPoints)
	}
}

func TestBacktestPredictionUsesTheSameRecentFitAsEstimate(t *testing.T) {
	zone := "UTC"
	start := time.Date(2023, 1, 1, 5, 0, 0, 0, time.UTC)
	first := syntheticSessions(25, start, 24*time.Hour, 8*time.Hour, zone)
	secondStart := first[len(first)-1].Intervals[0].Interval.Start.UTC.Add(25 * time.Hour)
	second := syntheticSessions(25, secondStart, 25*time.Hour, 8*time.Hour, zone)
	sessions := append(first, second...)

	report, err := (RobustEstimator{}).Backtest(context.Background(), sessions)
	if err != nil {
		t.Fatal(err)
	}
	last := report.Points[len(report.Points)-1]
	if last.AbsErrorHours > 0.25 {
		t.Fatalf("last prediction did not use the estimator's recent fit: %#v", last)
	}
}

func shiftSession(session domain.SleepSession, by time.Duration, zone string) domain.SleepSession {
	interval := session.Intervals[0].Interval
	start := interval.Start.UTC.Add(by)
	end := interval.End.UTC.Add(by)
	session.Intervals[0].Interval = domain.TimeRange{
		Start: domain.MustZonedInstant(start, zone),
		End:   domain.MustZonedInstant(end, zone),
	}
	return session
}
