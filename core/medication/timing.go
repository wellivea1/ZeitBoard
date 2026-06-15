package medication

import (
	"time"

	"non24.app/core/domain"
)

func AttachRelativeTiming(event domain.MedicationEvent, anchors []domain.WakeAnchor, estimate *domain.PhaseEstimate) domain.MedicationEvent {
	var latest *domain.WakeAnchor
	for i := range anchors {
		anchor := &anchors[i]
		if anchor.At.UTC.After(event.TakenAt.UTC) {
			continue
		}
		if latest == nil || anchor.At.UTC.After(latest.At.UTC) {
			latest = anchor
		}
	}
	if latest != nil {
		duration := event.TakenAt.UTC.Sub(latest.At.UTC)
		event.TimeSinceWake = &duration
		event.WakeAnchorID = latest.ID
		event.Confidence = latest.Confidence
	}
	if estimate != nil {
		for _, window := range estimate.PredictedSleepWindows {
			if window.Interval.Start.UTC.After(event.TakenAt.UTC) {
				duration := window.Interval.Start.UTC.Sub(event.TakenAt.UTC)
				event.TimeBeforePredictedSleep = &duration
				event.EstimateID = estimate.ID
				if latest == nil {
					event.Confidence = estimate.Confidence
				}
				break
			}
		}
	}
	if event.TimeSinceWake == nil && event.TimeBeforePredictedSleep == nil {
		event.Confidence = domain.InferenceConfidence{Level: domain.ConfidenceUnknown, Reasons: []string{"no relevant wake anchor or predicted sleep window"}}
	}
	return event
}

func DurationHours(value *time.Duration) float64 {
	if value == nil {
		return 0
	}
	return value.Hours()
}
