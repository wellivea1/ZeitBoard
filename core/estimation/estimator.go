package estimation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"time"

	"non24.app/core/civilfmt"
	"non24.app/core/domain"
)

// AlgorithmVersion names the fit. v2 counts the cycles between two recorded
// sleeps against the rhythm's own period, where v1 counted them against 24
// hours, and fits the nights since a gap it cannot count across.
const AlgorithmVersion = "theil-sen-sleep-start/v2"

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
	// Set with insufficient_data: the usable principal sleep episodes found,
	// and how many an estimate needs, so a reader can show the way to one.
	Episodes        int `json:"episodes,omitempty"`
	MinimumEpisodes int `json:"minimum_episodes,omitempty"`
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

	// Clock is the clock the rhythm projection's labels are written on
	// (ADR-0057). The zero value is the twelve-hour clock, for a caller that
	// does not know its reader.
	Clock civilfmt.Clock
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
	return fitIndexedOnsets(episodes, indices), nil
}

// fitIndexedOnsets fits sleep starts whose cycles have been counted.
func fitIndexedOnsets(episodes []domain.SleepSession, indices []int) onsetFit {
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
	}
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
	// The fit may rest on fewer nights than it was offered: those since a gap
	// the cycles could not be counted across. The estimate names the nights
	// it used.
	episodes = model.episodes
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
	// episodes are the nights the fit rests on: all it was offered, or those
	// since the last gap the cycles could not be counted across.
	episodes []domain.SleepSession
}

func fitOrderedEpisodes(episodes []domain.SleepSession, config Config) (fittedEpisodeModel, error) {
	if len(episodes) < config.MinimumEpisodes {
		return fittedEpisodeModel{}, &EstimationRefusal{
			Code:            RefusalInsufficientData,
			Message:         fmt.Sprintf("need at least %d usable principal sleep episodes; found %d", config.MinimumEpisodes, len(episodes)),
			Episodes:        len(episodes),
			MinimumEpisodes: config.MinimumEpisodes,
		}
	}
	// Nights go unrecorded, and a gap can be too long or too odd to say how
	// many cycles passed in it. That used to refuse the whole estimate until
	// the gap left the fitting window, about three weeks later. The nights
	// since the gap are a record of the rhythm in their own right, so once
	// there are enough of them they are fitted alone.
	offered := len(episodes)
	counted, err := countCycles(episodes)
	if err != nil {
		return fittedEpisodeModel{}, err
	}
	if counted.uncountedAt > 0 {
		since := episodes[counted.uncountedAt:]
		if len(since) < config.MinimumEpisodes {
			return fittedEpisodeModel{}, &EstimationRefusal{
				Code: RefusalAmbiguousCycleIndex,
				Message: fmt.Sprintf("cannot identify missing sleep cycles across a %.1f-hour gap; %d of the %d nights needed are recorded since it",
					counted.uncountedGap.Hours(), len(since), config.MinimumEpisodes),
			}
		}
		episodes = since
		if counted, err = countCycles(episodes); err != nil {
			return fittedEpisodeModel{}, err
		}
		if counted.uncountedAt > 0 {
			// The nights since the gap read differently on their own.
			return fittedEpisodeModel{}, &EstimationRefusal{
				Code:    RefusalAmbiguousCycleIndex,
				Message: fmt.Sprintf("cannot identify missing sleep cycles across a %.1f-hour gap", counted.uncountedGap.Hours()),
			}
		}
	}
	fit := fitIndexedOnsets(episodes, counted.indices)
	period := hoursDuration(fit.slope)
	if period < config.MinimumPeriod || period > config.MaximumPeriod {
		return fittedEpisodeModel{}, &EstimationRefusal{
			Code:    RefusalUnsupportedInput,
			Message: fmt.Sprintf("observed cycle length %s is outside the range the estimator was checked on", period.Round(time.Minute)),
		}
	}
	confidence := assessConfidence(episodes, fit.residualMAD, fit.slopeChange)
	if len(episodes) < offered {
		confidence.Reasons = append(confidence.Reasons,
			fmt.Sprintf("fitted to the %d nights since a gap the cycles could not be counted across", len(episodes)))
	}
	return fittedEpisodeModel{onset: fit, period: period, confidence: confidence, episodes: episodes}, nil
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
	return boundedEpisodeWindow(oneOnsetPerCycle(episodes), maximum), nil
}

// oneOnsetPerCycle keeps one sleep start for each cycle. Two sleeps that begin
// within less than the shortest cycle of each other cannot both open a cycle:
// one of them is a long nap or the second half of a broken night, whatever it
// was labelled. A wearable labels neither, and every session it reports
// arrives as main sleep, so a three-hour nap used to read as a cycle of twelve
// hours and stop the estimate. The longer of the two is taken as the cycle's
// sleep; the other stays in the record and out of the fit.
func oneOnsetPerCycle(ordered []domain.SleepSession) []domain.SleepSession {
	kept := make([]domain.SleepSession, 0, len(ordered))
	for _, episode := range ordered {
		if len(kept) == 0 {
			kept = append(kept, episode)
			continue
		}
		last := kept[len(kept)-1].Intervals[0].Interval
		next := episode.Intervals[0].Interval
		// Two records of the same start are a record imported twice, not two
		// sleeps. They are left for the count to refuse.
		if gap := next.Start.UTC.Sub(last.Start.UTC); gap <= 0 || gap >= singleCycleMinimum {
			kept = append(kept, episode)
			continue
		}
		if next.Duration() > last.Duration() {
			kept[len(kept)-1] = episode
		}
	}
	return kept
}

func boundedEpisodeWindow(episodes []domain.SleepSession, maximum int) []domain.SleepSession {
	if maximum > 0 && len(episodes) > maximum {
		return episodes[len(episodes)-maximum:]
	}
	return episodes
}

// A gap between two recorded sleep starts is a whole number of cycles, and
// how many depends on how long a cycle is. Counted against 24 hours, a rhythm
// of 27 lost its place whenever nights went unrecorded: two missing nights are
// 81 hours, which is three cycles of 27 and neither three nor four of 24, and
// six missing nights read as seven. The count is therefore made against the
// rhythm's own period, which the same records give.
const (
	// A gap this long can only be one cycle of a rhythm the estimator
	// supports, so such gaps say what the period is before any are counted.
	singleCycleMinimum = 16 * time.Hour
	singleCycleMaximum = 36 * time.Hour
	// Fewer single-cycle gaps than this say too little, and the day is used.
	singleCycleGapsNeeded = 3
	// How far a gap may sit from a whole number of cycles, as a share of one
	// cycle, and how many cycles a single gap may span.
	cycleTolerance   = 0.35
	maximumCycleStep = 7
)

type cycleCount struct {
	indices []int
	period  time.Duration
	// uncountedAt is the index of the first night after the latest gap whose
	// cycles could not be counted, or zero when every gap could be. The
	// indices are then not to be used across it.
	uncountedAt  int
	uncountedGap time.Duration
}

// countCycles numbers each sleep start by its cycle. The period is taken from
// the gaps that can only be one cycle, and every gap is counted against it.
//
// The period is not then refined from the counted starts and the gaps counted
// again. With few nights, one miscounted gap moves a fitted period far enough
// to make its own miscount look right, which is the mistake this exists to
// stop. The middle of the single-cycle gaps does not depend on any count.
func countCycles(episodes []domain.SleepSession) (cycleCount, error) {
	gaps := make([]time.Duration, 0, len(episodes))
	for i := 1; i < len(episodes); i++ {
		gap := episodes[i].Intervals[0].Interval.Start.UTC.Sub(episodes[i-1].Intervals[0].Interval.Start.UTC)
		if gap <= 0 {
			return cycleCount{}, &EstimationRefusal{Code: RefusalUnsupportedInput, Message: "sleep starts must increase in absolute time"}
		}
		gaps = append(gaps, gap)
	}
	result := cycleCount{indices: make([]int, len(episodes)), period: referencePeriod(gaps)}
	for i, gap := range gaps {
		result.indices[i+1] = result.indices[i] + wholeCycles(gap, result.period)
		if !countable(gap, result.period) {
			result.uncountedAt, result.uncountedGap = i+1, gap
		}
	}
	return result, nil
}

// referencePeriod is the middle of the gaps that can only be one cycle, or a
// day when there are too few of them to say.
func referencePeriod(gaps []time.Duration) time.Duration {
	var single []float64
	for _, gap := range gaps {
		if gap >= singleCycleMinimum && gap <= singleCycleMaximum {
			single = append(single, gap.Hours())
		}
	}
	if len(single) < singleCycleGapsNeeded {
		return 24 * time.Hour
	}
	return hoursDuration(median(single))
}

func wholeCycles(gap, period time.Duration) int {
	cycles := int(math.Round(gap.Hours() / period.Hours()))
	if cycles < 1 {
		return 1
	}
	return cycles
}

// countable reports whether a gap is near enough a whole number of cycles,
// and few enough of them, to be counted.
func countable(gap, period time.Duration) bool {
	ratio := gap.Hours() / period.Hours()
	cycles := wholeCycles(gap, period)
	return cycles <= maximumCycleStep && math.Abs(ratio-float64(cycles)) <= cycleTolerance
}

// cycleIndices numbers each sleep start by its cycle, and refuses when any
// gap cannot be counted.
func cycleIndices(episodes []domain.SleepSession) ([]int, error) {
	counted, err := countCycles(episodes)
	if err != nil {
		return nil, err
	}
	if counted.uncountedAt > 0 {
		return nil, &EstimationRefusal{
			Code:    RefusalAmbiguousCycleIndex,
			Message: fmt.Sprintf("cannot identify missing sleep cycles across a %.1f-hour gap", counted.uncountedGap.Hours()),
		}
	}
	return counted.indices, nil
}

// cycleStep counts the cycles in one gap against a known period.
func cycleStep(delta, period time.Duration) (int, error) {
	if delta <= 0 {
		return 0, &EstimationRefusal{Code: RefusalUnsupportedInput, Message: "sleep starts must increase in absolute time"}
	}
	if !countable(delta, period) {
		return 0, &EstimationRefusal{
			Code:    RefusalAmbiguousCycleIndex,
			Message: fmt.Sprintf("cannot identify missing sleep cycles across a %.1f-hour gap", delta.Hours()),
		}
	}
	return wholeCycles(delta, period), nil
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
