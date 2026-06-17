package estimation

import (
	"context"
	"math"
	"sort"
	"time"

	"non24.app/core/domain"
)

// BacktestReport answers the question the rest of the app cannot: is the estimate
// any good? It walks the observation history forward, refits at each step, predicts
// the next sleep onset, and scores the prediction against what actually happened —
// reporting error, forecast-window hit-rate, and whether the confidence levels are
// calibrated. It is a measurement of the core engine, not a new estimate.
type BacktestReport struct {
	Evaluations         int                 `json:"evaluations"`
	Refusals            int                 `json:"refusals"`
	MedianAbsErrorHours float64             `json:"medianAbsErrorHours"`
	MeanAbsErrorHours   float64             `json:"meanAbsErrorHours"`
	P90AbsErrorHours    float64             `json:"p90AbsErrorHours"`
	HitRate             float64             `json:"hitRate"`
	Calibration         []CalibrationBucket `json:"calibration"`
	Points              []BacktestPoint     `json:"-"`
}

// CalibrationBucket groups evaluations by the confidence the estimator reported, so
// a miscalibrated model (e.g. "high" confidence with large error) is visible.
type CalibrationBucket struct {
	Level               domain.ConfidenceLevel `json:"level"`
	Count               int                    `json:"count"`
	HitRate             float64                `json:"hitRate"`
	MedianAbsErrorHours float64                `json:"medianAbsErrorHours"`
}

// BacktestPoint is one walk-forward evaluation (detail; not serialized to the UI).
type BacktestPoint struct {
	EpisodesUsed   int
	HorizonCycles  int
	PredictedOnset time.Time
	ActualOnset    time.Time
	AbsErrorHours  float64
	WithinWindow   bool
	Confidence     domain.ConfidenceLevel
}

// Backtest evaluates the estimator against its own observation history by
// walk-forward validation: for each prefix of the principal sleep episodes it
// refits, predicts the onset of the next actual episode, and scores it.
func (e RobustEstimator) Backtest(ctx context.Context, sessions []domain.SleepSession) (BacktestReport, error) {
	config := e.Config
	if config.MinimumEpisodes == 0 {
		config = DefaultConfig()
	}
	// Use the full principal-episode history (no recency cap) as the timeline.
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
	fullIndices, err := cycleIndices(full)
	if err != nil {
		return BacktestReport{}, err
	}

	report := BacktestReport{}
	var errors []float64
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
		estimate, perr := e.Estimate(ctx, sub, asOf)
		if perr != nil {
			report.Refusals++
			continue
		}
		fit, ferr := fitOnsetTrend(sub)
		if ferr != nil {
			report.Refusals++
			continue
		}

		predicted := fit.onsetAt(fullIndices[k])
		actual := full[k].Intervals[0].Interval.Start.UTC
		absError := math.Abs(predicted.Sub(actual).Hours())
		horizon := fullIndices[k] - fullIndices[k-1]
		within := false
		if horizon >= 1 && horizon <= len(estimate.PredictedSleepWindows) {
			within = estimate.PredictedSleepWindows[horizon-1].Interval.Contains(actual)
		}
		level := estimate.Confidence.Level

		report.Points = append(report.Points, BacktestPoint{
			EpisodesUsed:   k,
			HorizonCycles:  horizon,
			PredictedOnset: predicted,
			ActualOnset:    actual,
			AbsErrorHours:  round2(absError),
			WithinWindow:   within,
			Confidence:     level,
		})
		errors = append(errors, absError)
		if within {
			hits++
		}
		acc := buckets[level]
		if acc == nil {
			acc = &bucketAccumulator{}
			buckets[level] = acc
		}
		acc.errors = append(acc.errors, absError)
		if within {
			acc.hits++
		}
	}

	report.Evaluations = len(errors)
	if report.Evaluations == 0 {
		return BacktestReport{}, &EstimationRefusal{
			Code:    RefusalInsufficientData,
			Message: "no prefix produced an evaluable prediction",
		}
	}
	report.MedianAbsErrorHours = round2(median(errors))
	report.MeanAbsErrorHours = round2(mean(errors))
	report.P90AbsErrorHours = round2(percentile(errors, 90))
	report.HitRate = round2(float64(hits) / float64(report.Evaluations))

	// Report buckets high -> low so a reader sees the supposedly strongest first.
	for _, level := range []domain.ConfidenceLevel{domain.ConfidenceHigh, domain.ConfidenceMedium, domain.ConfidenceLow} {
		acc := buckets[level]
		if acc == nil || len(acc.errors) == 0 {
			continue
		}
		report.Calibration = append(report.Calibration, CalibrationBucket{
			Level:               level,
			Count:               len(acc.errors),
			HitRate:             round2(float64(acc.hits) / float64(len(acc.errors))),
			MedianAbsErrorHours: round2(median(acc.errors)),
		})
	}
	return report, nil
}

type bucketAccumulator struct {
	errors []float64
	hits   int
}

// fitOnsetTrend is the shared robust sleep-onset fit: a Theil-Sen slope (hours per
// cycle) and median intercept over cycle-indexed onsets. It is the single place the
// linear model lives for prediction outside Estimate.
type onsetFit struct {
	base      time.Time
	slope     float64
	intercept float64
}

func fitOnsetTrend(episodes []domain.SleepSession) (onsetFit, error) {
	indices, err := cycleIndices(episodes)
	if err != nil {
		return onsetFit{}, err
	}
	base := episodes[0].Intervals[0].Interval.Start.UTC
	x := make([]float64, len(episodes))
	y := make([]float64, len(episodes))
	for i, episode := range episodes {
		x[i] = float64(indices[i])
		y[i] = episode.Intervals[0].Interval.Start.UTC.Sub(base).Hours()
	}
	slope := theilSenSlope(x, y)
	intercepts := make([]float64, len(x))
	for i := range x {
		intercepts[i] = y[i] - slope*x[i]
	}
	return onsetFit{base: base, slope: slope, intercept: median(intercepts)}, nil
}

func (f onsetFit) onsetAt(index int) time.Time {
	return f.base.Add(hoursDuration(f.intercept + f.slope*float64(index)))
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
