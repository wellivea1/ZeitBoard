package projection

import (
	"time"

	"non24.app/core/domain"
	"non24.app/core/sleepv1"
)

// SleepReviewResponse is owner-only editing context, never a sharing or agent DTO.
type SleepReviewResponse struct {
	SchemaVersion     string            `json:"schema_version"`
	SourceCursor      int64             `json:"source_cursor"`
	GeneratedAt       time.Time         `json:"generated_at"`
	ObservationID     string            `json:"observation_id"`
	SourceRevision    time.Time         `json:"source_revision"`
	SourceWindow      CompanionWindow   `json:"source_window"`
	EffectiveWindow   CompanionWindow   `json:"effective_window"`
	Classification    string            `json:"sleep_classification"`
	Excluded          bool              `json:"excluded"`
	NeedsReview       bool              `json:"needs_review"`
	ManualCorrections []SleepReviewEdit `json:"manual_corrections"`
}

type SleepReviewEdit struct {
	CorrectionID string             `json:"correction_id"`
	CreatedAt    time.Time          `json:"created_at"`
	Changes      SleepReviewChanges `json:"changes"`
}

type SleepReviewChanges struct {
	StartAt        *time.Time `json:"start_at,omitempty"`
	EndAt          *time.Time `json:"end_at,omitempty"`
	Classification *string    `json:"sleep_classification,omitempty"`
	Excluded       *bool      `json:"excluded,omitempty"`
}

func SleepReview(review sleepv1.ReviewContext, cursor int64, now time.Time) SleepReviewResponse {
	window := func(session domain.SleepSession) CompanionWindow {
		interval := session.Intervals[0].Interval
		return CompanionWindow{StartAt: interval.Start.UTC, EndAt: interval.End.UTC, ZoneID: interval.Start.ZoneID}
	}
	result := SleepReviewResponse{
		SchemaVersion: "v1", SourceCursor: cursor, GeneratedAt: now.UTC(), ObservationID: review.ObservationID,
		SourceRevision: review.SourceRevision.UTC(), SourceWindow: window(review.Source), EffectiveWindow: window(review.Effective),
		Classification: string(review.Effective.EffectiveClassification()), Excluded: review.Effective.Suppressed,
		NeedsReview: review.NeedsReview, ManualCorrections: []SleepReviewEdit{},
	}
	for _, correction := range review.ManualCorrections {
		result.ManualCorrections = append(result.ManualCorrections, SleepReviewEdit{
			CorrectionID: correction.CorrectionID, CreatedAt: correction.CreatedAt,
			Changes: SleepReviewChanges{StartAt: correction.Changes.StartAt, EndAt: correction.Changes.EndAt,
				Classification: correction.Changes.SleepClassification, Excluded: correction.Changes.Excluded},
		})
	}
	return result
}
