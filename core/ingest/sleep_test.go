package ingest

import (
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestResolveOverlappingSleepReportsPreservesProvenance(t *testing.T) {
	reports := []domain.SleepSession{
		report("wearable", time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC), time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)),
		report("desktop", time.Date(2026, 6, 1, 3, 30, 0, 0, time.UTC), time.Date(2026, 6, 1, 10, 30, 0, 0, time.UTC)),
	}

	resolved, err := ResolveOverlappingSleepReports(reports)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 {
		t.Fatalf("resolved count = %d", len(resolved))
	}
	evidence := resolved[0].Intervals[0].StartEvidence
	if len(evidence.ObservationIDs) != 2 {
		t.Fatalf("observation IDs = %v", evidence.ObservationIDs)
	}
}

func report(id string, start, end time.Time) domain.SleepSession {
	evidence := domain.Evidence{
		Acquisition:    domain.AcquisitionImported,
		Status:         domain.StatusImported,
		ObservationIDs: []domain.ObservationID{domain.ObservationID(id)},
	}
	return domain.SleepSession{
		ID: domain.SleepSessionID(id),
		Intervals: []domain.SleepInterval{{
			Interval: domain.TimeRange{
				Start: domain.MustZonedInstant(start, "UTC"),
				End:   domain.MustZonedInstant(end, "UTC"),
			},
			StartEvidence: evidence,
			EndEvidence:   evidence,
		}},
	}
}
