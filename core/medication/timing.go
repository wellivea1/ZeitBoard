package medication

import (
	"time"

	"non24.app/core/domain"
)

type RelativeTiming struct {
	TimeSinceWake            *time.Duration
	TimeBeforePredictedSleep *time.Duration
	WakeAnchorID             string
	EstimateID               domain.PhaseEstimateID
	Confidence               domain.InferenceConfidence
}

// ResolveRelativeTiming derives context for a user-recorded instant. It does
// not decide whether a medication, dose, or timing is appropriate.
func ResolveRelativeTiming(at domain.ZonedInstant, anchors []domain.WakeAnchor, estimate *domain.PhaseEstimate) RelativeTiming {
	result := RelativeTiming{}
	var latest *domain.WakeAnchor
	for i := range anchors {
		anchor := &anchors[i]
		if anchor.At.UTC.After(at.UTC) {
			continue
		}
		if latest == nil || anchor.At.UTC.After(latest.At.UTC) {
			latest = anchor
		}
	}
	if latest != nil {
		duration := at.UTC.Sub(latest.At.UTC)
		result.TimeSinceWake = &duration
		result.WakeAnchorID = latest.ID
		result.Confidence = latest.Confidence
	}
	if estimate != nil {
		for _, window := range estimate.PredictedSleepWindows {
			if window.Interval.Start.UTC.After(at.UTC) {
				duration := window.Interval.Start.UTC.Sub(at.UTC)
				result.TimeBeforePredictedSleep = &duration
				result.EstimateID = estimate.ID
				if latest == nil {
					result.Confidence = estimate.Confidence
				}
				break
			}
		}
	}
	if result.TimeSinceWake == nil && result.TimeBeforePredictedSleep == nil {
		result.Confidence = domain.InferenceConfidence{Level: domain.ConfidenceUnknown, Reasons: []string{"no relevant wake anchor or predicted sleep window"}}
	}
	return result
}

func AttachRelativeTiming(event domain.MedicationEvent, anchors []domain.WakeAnchor, estimate *domain.PhaseEstimate) domain.MedicationEvent {
	relative := ResolveRelativeTiming(event.TakenAt, anchors, estimate)
	event.TimeSinceWake = relative.TimeSinceWake
	event.TimeBeforePredictedSleep = relative.TimeBeforePredictedSleep
	event.WakeAnchorID = relative.WakeAnchorID
	event.EstimateID = relative.EstimateID
	event.Confidence = relative.Confidence
	return event
}

func DurationHours(value *time.Duration) float64 {
	if value == nil {
		return 0
	}
	return value.Hours()
}
