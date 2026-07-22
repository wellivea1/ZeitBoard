package estimation

import (
	"math"
	"time"

	"non24.app/core/domain"
)

const (
	TemporalAssociationAvailable       = "available"
	TemporalAssociationInsufficient    = "insufficient_data"
	TemporalAssociationAmbiguousCycles = "ambiguous_cycle_index"
	TemporalAssociationUnsupported     = "unsupported_input"

	temporalAssociationMinimumEpisodes = 5
	temporalAssociationMaximumEpisodes = 14
)

// TemporalAssociation is a descriptive split of observed sleep-onset drift
// around a user-recorded event. It deliberately carries no effect estimate or
// causal conclusion.
type TemporalAssociation struct {
	Status      string
	Message     string
	StartedAt   time.Time
	WindowStart time.Time
	WindowEnd   time.Time
	Before      DriftSegment
	After       DriftSegment
}

type DriftSegment struct {
	EpisodeCount int
	FromAt       time.Time
	ToAt         time.Time
	Drift        time.Duration
	Confidence   domain.InferenceConfidence
}

// DescribeTemporalAssociation fits the same robust sleep-onset trajectory used
// by the estimator on each side of a recorded start. Five usable episodes are
// required on each side, and at most fourteen are used per side. The result is
// observational context only.
func DescribeTemporalAssociation(sessions []domain.SleepSession, startedAt time.Time) TemporalAssociation {
	result := TemporalAssociation{
		Status:    TemporalAssociationUnsupported,
		Message:   "A real medication start instant is required for a descriptive comparison.",
		StartedAt: startedAt.UTC(),
	}
	if startedAt.IsZero() {
		return result
	}
	episodes, err := selectEpisodes(sessions, 0)
	if err != nil {
		result.Message = "The recorded sleep episodes cannot support a descriptive comparison."
		return result
	}
	before := make([]domain.SleepSession, 0, len(episodes))
	after := make([]domain.SleepSession, 0, len(episodes))
	for _, episode := range episodes {
		if episode.Intervals[0].Interval.Start.UTC.Before(startedAt.UTC()) {
			before = append(before, episode)
		} else {
			after = append(after, episode)
		}
	}
	if len(before) > temporalAssociationMaximumEpisodes {
		before = before[len(before)-temporalAssociationMaximumEpisodes:]
	}
	if len(after) > temporalAssociationMaximumEpisodes {
		after = after[:temporalAssociationMaximumEpisodes]
	}
	result.Before.EpisodeCount = len(before)
	result.After.EpisodeCount = len(after)
	if len(before) > 0 {
		result.WindowStart = before[0].Intervals[0].Interval.Start.UTC
	}
	if len(after) > 0 {
		result.WindowEnd = after[len(after)-1].Intervals[0].Interval.Start.UTC
	}
	if len(before) < temporalAssociationMinimumEpisodes || len(after) < temporalAssociationMinimumEpisodes {
		result.Status = TemporalAssociationInsufficient
		result.Message = "Need at least five usable principal sleep episodes before and after the recorded start for a descriptive comparison."
		return result
	}
	result.Before, err = describeDriftSegment(before)
	if err != nil {
		result.Status = TemporalAssociationAmbiguousCycles
		result.Message = "Sleep-cycle gaps are too ambiguous for a before/after drift comparison."
		return result
	}
	result.After, err = describeDriftSegment(after)
	if err != nil {
		result.Status = TemporalAssociationAmbiguousCycles
		result.Message = "Sleep-cycle gaps are too ambiguous for a before/after drift comparison."
		return result
	}
	result.Status = TemporalAssociationAvailable
	result.Message = "Descriptive sleep-onset slopes are available on both sides of the recorded start. Their temporal alignment does not establish cause."
	return result
}

func describeDriftSegment(episodes []domain.SleepSession) (DriftSegment, error) {
	indices, err := cycleIndices(episodes)
	if err != nil {
		return DriftSegment{}, err
	}
	base := episodes[0].Intervals[0].Interval.Start.UTC
	x := make([]float64, len(episodes))
	y := make([]float64, len(episodes))
	for i, episode := range episodes {
		x[i] = float64(indices[i])
		y[i] = episode.Intervals[0].Interval.Start.UTC.Sub(base).Hours()
	}
	periodHours := theilSenSlope(x, y)
	period := hoursDuration(periodHours)
	config := DefaultConfig()
	if period < config.MinimumPeriod || period > config.MaximumPeriod {
		return DriftSegment{}, &EstimationRefusal{Code: RefusalUnsupportedInput, Message: "descriptive segment is outside the validated cycle-length range"}
	}
	intercepts := make([]float64, len(x))
	for i := range x {
		intercepts[i] = y[i] - periodHours*x[i]
	}
	intercept := median(intercepts)
	residuals := make([]float64, len(x))
	for i := range x {
		residuals[i] = math.Abs(y[i] - (intercept + periodHours*x[i]))
	}
	residualMAD := median(residuals)
	return DriftSegment{
		EpisodeCount: len(episodes),
		FromAt:       episodes[0].Intervals[0].Interval.Start.UTC,
		ToAt:         episodes[len(episodes)-1].Intervals[0].Interval.Start.UTC,
		Drift:        period - 24*time.Hour,
		Confidence:   assessConfidence(episodes, residualMAD, segmentedSlopeChange(x, y)),
	}, nil
}
