package medication

import (
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestMedicationTimingRelativeToWakeAndPredictedSleep(t *testing.T) {
	wake := time.Date(2026, 6, 15, 15, 0, 0, 0, time.UTC)
	dose := wake.Add(3 * time.Hour)
	sleep := wake.Add(16 * time.Hour)
	anchors := []domain.WakeAnchor{{
		ID: "wake-1", At: domain.MustZonedInstant(wake, "UTC"),
		Confidence: domain.InferenceConfidence{Level: domain.ConfidenceHigh},
	}}
	estimate := domain.PhaseEstimate{
		ID: "phase-1", Confidence: domain.InferenceConfidence{Level: domain.ConfidenceMedium},
		PredictedSleepWindows: []domain.AvailabilityWindow{{
			Interval: domain.TimeRange{Start: domain.MustZonedInstant(sleep, "UTC"), End: domain.MustZonedInstant(sleep.Add(9*time.Hour), "UTC")},
		}},
	}
	event := AttachRelativeTiming(domain.MedicationEvent{TakenAt: domain.MustZonedInstant(dose, "UTC")}, anchors, &estimate)
	if event.TimeSinceWake == nil || *event.TimeSinceWake != 3*time.Hour {
		t.Fatalf("time since wake = %v", event.TimeSinceWake)
	}
	if event.TimeBeforePredictedSleep == nil || *event.TimeBeforePredictedSleep != 13*time.Hour {
		t.Fatalf("time before sleep = %v", event.TimeBeforePredictedSleep)
	}
}
