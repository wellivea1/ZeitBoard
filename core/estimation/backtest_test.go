package estimation

import (
	"context"
	"math"
	"reflect"
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

func TestBacktestMatchesPrefixReferenceImplementation(t *testing.T) {
	zone := "UTC"
	first := syntheticSessions(36, time.Date(2023, 1, 1, 5, 0, 0, 0, time.UTC), 25*time.Hour, 8*time.Hour, zone)
	secondStart := first[len(first)-1].Intervals[0].Interval.Start.UTC.Add(36 * time.Hour)
	second := syntheticSessions(34, secondStart, 24*time.Hour+45*time.Minute, 8*time.Hour, zone)
	sessions := append(first, second...)

	nap := syntheticSessions(1, time.Date(2023, 2, 1, 17, 0, 0, 0, time.UTC), 24*time.Hour, 90*time.Minute, zone)[0]
	nap.ID = "nap"
	nap.IsNap = true
	suppressed := syntheticSessions(1, time.Date(2023, 2, 2, 4, 0, 0, 0, time.UTC), 24*time.Hour, 8*time.Hour, zone)[0]
	suppressed.ID = "suppressed"
	suppressed.Suppressed = true
	short := syntheticSessions(1, time.Date(2023, 2, 3, 4, 0, 0, 0, time.UTC), 24*time.Hour, 2*time.Hour, zone)[0]
	short.ID = "short"
	sessions = append(sessions, nap, suppressed, short)
	for left, right := 0, len(sessions)-1; left < right; left, right = left+1, right-1 {
		sessions[left], sessions[right] = sessions[right], sessions[left]
	}

	estimator := RobustEstimator{}
	got, err := estimator.Backtest(context.Background(), sessions)
	if err != nil {
		t.Fatal(err)
	}
	want, err := prefixReferenceBacktest(context.Background(), estimator, sessions)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("optimized backtest changed output semantics:\ngot:  %#v\nwant: %#v", got, want)
	}
}

// prefixReferenceBacktest retains the previous growing-prefix composition for
// regression comparison. Production intentionally avoids this quadratic path.
func prefixReferenceBacktest(ctx context.Context, estimator RobustEstimator, sessions []domain.SleepSession) (BacktestReport, error) {
	config := estimator.Config
	if config.MinimumEpisodes == 0 {
		config = DefaultConfig()
	}
	full, err := selectEpisodes(sessions, 0)
	if err != nil {
		return BacktestReport{}, err
	}
	if len(full) <= config.MinimumEpisodes {
		return BacktestReport{}, &EstimationRefusal{
			Code:    RefusalInsufficientData,
			Message: "not enough principal sleep episodes to hold any out for validation",
		}
	}
	report := BacktestReport{}
	var absoluteErrors []float64
	hits := 0
	buckets := map[domain.ConfidenceLevel]*bucketAccumulator{}

	for k := config.MinimumEpisodes; k < len(full); k++ {
		select {
		case <-ctx.Done():
			return BacktestReport{}, ctx.Err()
		default:
		}
		sub := full[:k]
		asOf := full[k-1].Intervals[0].Interval.End.UTC
		estimate, estimateErr := estimator.Estimate(ctx, sub, asOf)
		if estimateErr != nil {
			if appendBacktestRefusal(&report, k, full[k].Intervals[0].Interval.Start.UTC, estimateErr) {
				continue
			}
			return BacktestReport{}, estimateErr
		}
		modelEpisodes, selectErr := selectEpisodes(sub, config.MaximumEpisodes)
		if selectErr != nil {
			if appendBacktestRefusal(&report, k, full[k].Intervals[0].Interval.Start.UTC, selectErr) {
				continue
			}
			return BacktestReport{}, selectErr
		}
		fit, fitErr := fitOnsetTrend(modelEpisodes)
		if fitErr != nil {
			if appendBacktestRefusal(&report, k, full[k].Intervals[0].Interval.Start.UTC, fitErr) {
				continue
			}
			return BacktestReport{}, fitErr
		}

		actual := full[k].Intervals[0].Interval.Start.UTC
		lastModelOnset := modelEpisodes[len(modelEpisodes)-1].Intervals[0].Interval.Start.UTC
		horizon, horizonErr := cycleStep(actual.Sub(lastModelOnset))
		if horizonErr != nil {
			if appendBacktestRefusal(&report, k, actual, horizonErr) {
				continue
			}
			return BacktestReport{}, horizonErr
		}
		predicted := fit.onsetAt(fit.lastIndex + horizon)
		absoluteError := math.Abs(predicted.Sub(actual).Hours())
		within := false
		var windowStart, windowEnd time.Time
		var windowWidth float64
		if horizon >= 1 && horizon <= len(estimate.PredictedSleepWindows) {
			window := estimate.PredictedSleepWindows[horizon-1].Interval
			within = window.Contains(actual)
			windowStart = window.Start.UTC
			windowEnd = window.End.UTC
			windowWidth = window.Duration().Hours()
		}
		level := estimate.Confidence.Level

		report.Points = append(report.Points, BacktestPoint{
			EpisodesUsed:     k,
			HorizonCycles:    horizon,
			PredictedOnset:   predicted,
			ActualOnset:      actual,
			AbsErrorHours:    round2(absoluteError),
			WithinWindow:     within,
			WindowStart:      windowStart,
			WindowEnd:        windowEnd,
			WindowWidthHours: round2(windowWidth),
			Confidence:       level,
		})
		absoluteErrors = append(absoluteErrors, absoluteError)
		if within {
			hits++
		}
		accumulator := buckets[level]
		if accumulator == nil {
			accumulator = &bucketAccumulator{}
			buckets[level] = accumulator
		}
		accumulator.errors = append(accumulator.errors, absoluteError)
		if within {
			accumulator.hits++
		}
	}

	report.Evaluations = len(absoluteErrors)
	if report.Evaluations == 0 {
		return BacktestReport{}, &EstimationRefusal{
			Code:    RefusalInsufficientData,
			Message: "no prefix produced an evaluable prediction",
		}
	}
	report.MedianAbsErrorHours = round2(median(absoluteErrors))
	report.MeanAbsErrorHours = round2(mean(absoluteErrors))
	report.P90AbsErrorHours = round2(percentile(absoluteErrors, 90))
	report.HitRate = round2(float64(hits) / float64(report.Evaluations))

	for _, level := range []domain.ConfidenceLevel{domain.ConfidenceHigh, domain.ConfidenceMedium, domain.ConfidenceLow} {
		accumulator := buckets[level]
		if accumulator == nil || len(accumulator.errors) == 0 {
			continue
		}
		report.Calibration = append(report.Calibration, CalibrationBucket{
			Level:               level,
			Count:               len(accumulator.errors),
			HitRate:             round2(float64(accumulator.hits) / float64(len(accumulator.errors))),
			MedianAbsErrorHours: round2(median(accumulator.errors)),
		})
	}
	return report, nil
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
