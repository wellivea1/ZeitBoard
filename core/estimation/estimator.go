package estimation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"time"

	"non24.app/core/domain"
)

const AlgorithmVersion = "theil-sen-sleep-start/v1"

type RefusalCode string

const (
	RefusalInsufficientData        RefusalCode = "insufficient_data"
	RefusalAmbiguousCycleIndex     RefusalCode = "ambiguous_cycle_index"
	RefusalConflictingObservations RefusalCode = "conflicting_observations"
	RefusalUnsupportedInput        RefusalCode = "unsupported_input"
)

type EstimationRefusal struct {
	Code    RefusalCode `json:"code"`
	Message string      `json:"message"`
}

func (r *EstimationRefusal) Error() string { return r.Message }

type Config struct {
	MinimumEpisodes int
	MaximumEpisodes int
	ForecastCycles  int
	MinimumPeriod   time.Duration
	MaximumPeriod   time.Duration
	BaseUncertainty time.Duration
	// UncertaintyScale changes forecast bounds without changing the fitted
	// rhythm. Production uses 1; other values exist for measured candidates.
	UncertaintyScale float64
}

func DefaultConfig() Config {
	return Config{
		MinimumEpisodes:  7,
		MaximumEpisodes:  21,
		ForecastCycles:   7,
		MinimumPeriod:    20 * time.Hour,
		MaximumPeriod:    30 * time.Hour,
		BaseUncertainty:  30 * time.Minute,
		UncertaintyScale: 1,
	}
}

type Estimator interface {
	Estimate(context.Context, []domain.SleepSession, time.Time) (domain.PhaseEstimate, error)
}

type RobustEstimator struct {
	Config Config
}

func (e RobustEstimator) Estimate(ctx context.Context, sessions []domain.SleepSession, asOf time.Time) (domain.PhaseEstimate, error) {
	select {
	case <-ctx.Done():
		return domain.PhaseEstimate{}, ctx.Err()
	default:
	}
	config, err := normalizedConfig(e.Config)
	if err != nil {
		return domain.PhaseEstimate{}, err
	}
	episodes, err := selectEpisodes(sessions, config.MaximumEpisodes)
	if err != nil {
		return domain.PhaseEstimate{}, err
	}
	estimate, _, err := estimateOrderedEpisodes(episodes, asOf, config)
	return estimate, err
}

func configWithDefaults(config Config) Config {
	if config.MinimumEpisodes == 0 {
		return DefaultConfig()
	}
	return config
}

func normalizedConfig(config Config) (Config, error) {
	config = configWithDefaults(config)
	if config.UncertaintyScale == 0 {
		config.UncertaintyScale = 1
	}
	if config.UncertaintyScale <= 0 {
		return Config{}, &EstimationRefusal{Code: RefusalUnsupportedInput, Message: "uncertainty scale must be positive"}
	}
	return config, nil
}

// fitOnsetTrend is the shared bounded-window fit used by estimate generation and
// holdout scoring.
type onsetFit struct {
	base           time.Time
	slope          float64
	intercept      float64
	lastIndex      int
	residualMAD    float64
	durationMedian float64
	durationMAD    float64
	slopeChange    float64
}

func fitOnsetTrend(episodes []domain.SleepSession) (onsetFit, error) {
	indices, err := cycleIndices(episodes)
	if err != nil {
		return onsetFit{}, err
	}
	base := episodes[0].Intervals[0].Interval.Start.UTC
	x := make([]float64, len(episodes))
	y := make([]float64, len(episodes))
	durations := make([]float64, len(episodes))
	for i, episode := range episodes {
		x[i] = float64(indices[i])
		y[i] = episode.Intervals[0].Interval.Start.UTC.Sub(base).Hours()
		durations[i] = episode.Intervals[0].Interval.Duration().Hours()
	}
	slope := theilSenSlope(x, y)
	intercepts := make([]float64, len(x))
	for i := range x {
		intercepts[i] = y[i] - slope*x[i]
	}
	intercept := median(intercepts)
	residuals := make([]float64, len(x))
	for i := range x {
		residuals[i] = math.Abs(y[i] - (intercept + slope*x[i]))
	}
	durationMedian := median(durations)
	durationDeviations := make([]float64, len(durations))
	for i, value := range durations {
		durationDeviations[i] = math.Abs(value - durationMedian)
	}
	return onsetFit{
		base:           base,
		slope:          slope,
		intercept:      intercept,
		lastIndex:      indices[len(indices)-1],
		residualMAD:    median(residuals),
		durationMedian: durationMedian,
		durationMAD:    median(durationDeviations),
		slopeChange:    segmentedSlopeChange(x, y),
	}, nil
}

func (f onsetFit) onsetAt(index int) time.Time {
	return f.base.Add(hoursDuration(f.intercept + f.slope*float64(index)))
}

// estimateOrderedEpisodes estimates from an already filtered, chronological,
// bounded principal-episode window. Keeping this path selection-free lets
// backtesting walk one ordered history without rescanning each growing prefix.
func estimateOrderedEpisodes(episodes []domain.SleepSession, asOf time.Time, config Config) (domain.PhaseEstimate, fittedEpisodeModel, error) {
	model, err := fitOrderedEpisodes(episodes, config)
	if err != nil {
		return domain.PhaseEstimate{}, fittedEpisodeModel{}, err
	}
	zoneID := episodes[len(episodes)-1].Intervals[0].Interval.Start.ZoneID
	characteristicInstant, err := domain.NewZonedInstant(model.onset.onsetAt(model.onset.lastIndex), zoneID)
	if err != nil {
		return domain.PhaseEstimate{}, fittedEpisodeModel{}, err
	}
	asOfInstant, err := domain.NewZonedInstant(asOf.UTC(), zoneID)
	if err != nil {
		return domain.PhaseEstimate{}, fittedEpisodeModel{}, err
	}
	id := estimateID(episodes, asOf)
	estimate := domain.PhaseEstimate{
		ID:                       id,
		AsOf:                     asOfInstant,
		CharacteristicSleepStart: characteristicInstant,
		ObservedCycleLength:      model.period,
		ObservedDriftPerCycle:    model.period - 24*time.Hour,
		TypicalSleepDuration:     hoursDuration(model.onset.durationMedian),
		Confidence:               model.confidence,
		AlgorithmVersion:         AlgorithmVersion,
		CreatedAt:                time.Now().UTC(),
	}
	for _, episode := range episodes {
		estimate.InputSessionIDs = append(estimate.InputSessionIDs, episode.ID)
	}
	for horizon := 1; horizon <= config.ForecastCycles; horizon++ {
		sleepInterval, wakeInterval := forecastIntervals(model, config, horizon, zoneID)
		estimate.PredictedSleepWindows = append(estimate.PredictedSleepWindows, domain.AvailabilityWindow{
			ID:          domain.AvailabilityWindowID(fmt.Sprintf("%s-sleep-%d", id, horizon)),
			Kind:        domain.AvailabilityPredictedSleep,
			Interval:    sleepInterval,
			Confidence:  model.confidence,
			EstimateID:  id,
			Explanation: "Robust projection from recent principal sleep-start trajectory; bounds widen with forecast distance.",
		})
		estimate.PredictedWakingWindows = append(estimate.PredictedWakingWindows, domain.AvailabilityWindow{
			ID:          domain.AvailabilityWindowID(fmt.Sprintf("%s-wake-%d", id, horizon)),
			Kind:        domain.AvailabilityPredictedWake,
			Interval:    wakeInterval,
			Confidence:  model.confidence,
			EstimateID:  id,
			Explanation: "Conservative waking window between uncertain wake and next projected sleep.",
		})
	}
	return estimate, model, nil
}

type fittedEpisodeModel struct {
	onset      onsetFit
	period     time.Duration
	confidence domain.InferenceConfidence
}

func fitOrderedEpisodes(episodes []domain.SleepSession, config Config) (fittedEpisodeModel, error) {
	if len(episodes) < config.MinimumEpisodes {
		return fittedEpisodeModel{}, &EstimationRefusal{
			Code:    RefusalInsufficientData,
			Message: fmt.Sprintf("need at least %d usable principal sleep episodes; found %d", config.MinimumEpisodes, len(episodes)),
		}
	}
	fit, err := fitOnsetTrend(episodes)
	if err != nil {
		return fittedEpisodeModel{}, err
	}
	period := hoursDuration(fit.slope)
	if period < config.MinimumPeriod || period > config.MaximumPeriod {
		return fittedEpisodeModel{}, &EstimationRefusal{
			Code:    RefusalUnsupportedInput,
			Message: fmt.Sprintf("observed cycle length %s is outside the prototype validation range", period.Round(time.Minute)),
		}
	}
	return fittedEpisodeModel{
		onset:      fit,
		period:     period,
		confidence: assessConfidence(episodes, fit.residualMAD, fit.slopeChange),
	}, nil
}

func forecastIntervals(model fittedEpisodeModel, config Config, horizon int, zoneID string) (domain.TimeRange, domain.TimeRange) {
	fit := model.onset
	baseUncertaintyHours := config.UncertaintyScale * math.Max(config.BaseUncertainty.Hours(), 1.4826*fit.residualMAD)
	center := fit.onsetAt(fit.lastIndex + horizon)
	uncertainty := hoursDuration(baseUncertaintyHours + config.UncertaintyScale*float64(horizon)*math.Max(0.25, fit.residualMAD*0.35))
	durationUncertainty := hoursDuration(config.UncertaintyScale * math.Max(0.25, 1.4826*fit.durationMAD))
	sleepStart, _ := domain.NewZonedInstant(center.Add(-uncertainty), zoneID)
	sleepEnd, _ := domain.NewZonedInstant(center.Add(hoursDuration(fit.durationMedian)).Add(uncertainty+durationUncertainty), zoneID)
	wakeStart, _ := domain.NewZonedInstant(center.Add(hoursDuration(fit.durationMedian)).Add(-uncertainty-durationUncertainty), zoneID)
	wakeEnd, _ := domain.NewZonedInstant(center.Add(model.period).Add(uncertainty), zoneID)
	return domain.TimeRange{Start: sleepStart, End: sleepEnd}, domain.TimeRange{Start: wakeStart, End: wakeEnd}
}

func selectEpisodes(sessions []domain.SleepSession, maximum int) ([]domain.SleepSession, error) {
	var episodes []domain.SleepSession
	for _, session := range sessions {
		if !session.IsPrincipalSleep() {
			continue
		}
		if len(session.Intervals) == 0 {
			continue
		}
		if err := session.Intervals[0].Interval.Validate(); err != nil {
			return nil, &EstimationRefusal{Code: RefusalUnsupportedInput, Message: fmt.Sprintf("sleep session %s: %v", session.ID, err)}
		}
		if session.Intervals[0].Interval.Duration() < 3*time.Hour {
			continue
		}
		episodes = append(episodes, session)
	}
	sort.Slice(episodes, func(i, j int) bool {
		return episodes[i].Intervals[0].Interval.Start.UTC.Before(episodes[j].Intervals[0].Interval.Start.UTC)
	})
	return boundedEpisodeWindow(episodes, maximum), nil
}

func boundedEpisodeWindow(episodes []domain.SleepSession, maximum int) []domain.SleepSession {
	if maximum > 0 && len(episodes) > maximum {
		return episodes[len(episodes)-maximum:]
	}
	return episodes
}

func cycleIndices(episodes []domain.SleepSession) ([]int, error) {
	indices := make([]int, len(episodes))
	for i := 1; i < len(episodes); i++ {
		delta := episodes[i].Intervals[0].Interval.Start.UTC.Sub(episodes[i-1].Intervals[0].Interval.Start.UTC)
		cycles, err := cycleStep(delta)
		if err != nil {
			return nil, err
		}
		indices[i] = indices[i-1] + cycles
	}
	return indices, nil
}

func cycleStep(delta time.Duration) (int, error) {
	if delta <= 0 {
		return 0, &EstimationRefusal{Code: RefusalUnsupportedInput, Message: "sleep starts must increase in absolute time"}
	}
	ratio := delta.Hours() / 24
	cycles := int(math.Round(ratio))
	if cycles < 1 {
		cycles = 1
	}
	if cycles > 7 || math.Abs(ratio-float64(cycles)) > 0.35 {
		return 0, &EstimationRefusal{
			Code:    RefusalAmbiguousCycleIndex,
			Message: fmt.Sprintf("cannot identify missing sleep cycles across a %.1f-hour gap", delta.Hours()),
		}
	}
	return cycles, nil
}

func theilSenSlope(x, y []float64) float64 {
	var slopes []float64
	for i := 0; i < len(x); i++ {
		for j := i + 1; j < len(x); j++ {
			if x[j] == x[i] {
				continue
			}
			slopes = append(slopes, (y[j]-y[i])/(x[j]-x[i]))
		}
	}
	return median(slopes)
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	middle := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[middle]
	}
	return (copyValues[middle-1] + copyValues[middle]) / 2
}

func hoursDuration(hours float64) time.Duration {
	return time.Duration(math.Round(hours * float64(time.Hour)))
}

func assessConfidence(episodes []domain.SleepSession, residualMADHours, slopeChangeHours float64) domain.InferenceConfidence {
	confirmed := 0
	for _, episode := range episodes {
		interval := episode.Intervals[0]
		if interval.StartEvidence.Status == domain.StatusUserConfirmed {
			confirmed++
		}
	}
	reasons := []string{fmt.Sprintf("%d usable principal sleep episodes", len(episodes))}
	if confirmed > 0 {
		reasons = append(reasons, fmt.Sprintf("%d user-confirmed sleep starts", confirmed))
	}
	reasons = append(reasons, fmt.Sprintf("robust residual spread about %s", hoursDuration(residualMADHours).Round(time.Minute)))
	level := domain.ConfidenceLow
	if len(episodes) >= 12 && residualMADHours <= 0.5 {
		level = domain.ConfidenceHigh
	} else if len(episodes) >= 8 && residualMADHours <= 1.5 {
		level = domain.ConfidenceMedium
	}
	if !math.IsNaN(slopeChangeHours) && slopeChangeHours >= 0.5 {
		level = domain.ConfidenceLow
		reasons = append(reasons, fmt.Sprintf("recent and earlier sleep-start slopes differ by about %s per cycle", hoursDuration(slopeChangeHours).Round(time.Minute)))
	} else if !math.IsNaN(slopeChangeHours) && slopeChangeHours >= 0.25 && level == domain.ConfidenceHigh {
		level = domain.ConfidenceMedium
		reasons = append(reasons, "recent sleep-start drift differs from the earlier segment")
	}
	return domain.InferenceConfidence{Level: level, Reasons: reasons}
}

func segmentedSlopeChange(x, y []float64) float64 {
	if len(x) < 10 {
		return math.NaN()
	}
	middle := len(x) / 2
	left := theilSenSlope(x[:middle], y[:middle])
	right := theilSenSlope(x[middle:], y[middle:])
	return math.Abs(right - left)
}

func estimateID(episodes []domain.SleepSession, asOf time.Time) domain.PhaseEstimateID {
	hash := sha256.New()
	for _, episode := range episodes {
		_, _ = fmt.Fprintln(hash, episode.ID, episode.Intervals[0].Interval.Start.UTC.Format(time.RFC3339Nano))
	}
	_, _ = fmt.Fprintln(hash, asOf.UTC().Format(time.RFC3339Nano), AlgorithmVersion)
	return domain.PhaseEstimateID("phase-" + hex.EncodeToString(hash.Sum(nil))[:12])
}
