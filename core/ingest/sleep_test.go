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

func at(hour, minute int) time.Time {
	return time.Date(2026, 6, 1, hour, minute, 0, 0, time.UTC)
}

// Three reports of one sleep were merged two at a time, each result averaged
// with the next, which gave 01:15 for starts of 00:00, 01:00 and 02:00.
func TestOverlappingReportsMergeToTheirMiddleWhateverTheirOrder(t *testing.T) {
	reports := []domain.SleepSession{
		report("a", at(0, 0), at(8, 0)),
		report("b", at(1, 0), at(9, 0)),
		report("c", at(2, 0), at(10, 0)),
	}
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}, {1, 2, 0}} {
		shuffled := []domain.SleepSession{reports[order[0]], reports[order[1]], reports[order[2]]}
		resolved, err := ResolveOverlappingSleepReports(shuffled)
		if err != nil {
			t.Fatal(err)
		}
		if len(resolved) != 1 {
			t.Fatalf("resolved count = %d", len(resolved))
		}
		interval := resolved[0].Intervals[0]
		if !interval.Interval.Start.UTC.Equal(at(1, 0)) || !interval.Interval.End.UTC.Equal(at(9, 0)) {
			t.Fatalf("order %v merged to %s – %s, want the middle report's 01:00 – 09:00",
				order, interval.Interval.Start.UTC, interval.Interval.End.UTC)
		}
		if len(interval.StartEvidence.ObservationIDs) != 3 || interval.StartEvidence.Status != domain.StatusImported {
			t.Fatalf("evidence = %+v, want all three reports kept under an imported status", interval.StartEvidence)
		}
	}
}

// A night the owner confirmed keeps its times when an automatic source also
// reports it, and only then does the merged night read as confirmed.
func TestAConfirmedNightKeepsItsTimesBesideAnAutomaticReport(t *testing.T) {
	confirmed := report("corrected", at(1, 0), at(9, 0))
	confirmed.Intervals[0].StartEvidence.Status = domain.StatusUserConfirmed
	confirmed.Intervals[0].EndEvidence.Status = domain.StatusUserConfirmed
	wearable := report("wearable", at(0, 10), at(9, 50))

	resolved, err := ResolveOverlappingSleepReports([]domain.SleepSession{wearable, confirmed})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 {
		t.Fatalf("resolved count = %d", len(resolved))
	}
	interval := resolved[0].Intervals[0]
	if !interval.Interval.Start.UTC.Equal(at(1, 0)) || !interval.Interval.End.UTC.Equal(at(9, 0)) {
		t.Fatalf("merged to %s – %s; the wearable moved a night the owner confirmed",
			interval.Interval.Start.UTC, interval.Interval.End.UTC)
	}
	if interval.StartEvidence.Status != domain.StatusUserConfirmed || len(interval.StartEvidence.ObservationIDs) != 2 {
		t.Fatalf("evidence = %+v, want confirmed, with both reports kept", interval.StartEvidence)
	}

	// With nothing confirmed, a time neither source gave does not read as
	// confirmed.
	half := report("half", at(1, 0), at(9, 0))
	half.Intervals[0].StartEvidence.Status = domain.StatusUserConfirmed
	resolved, err = ResolveOverlappingSleepReports([]domain.SleepSession{wearable, half})
	if err != nil {
		t.Fatal(err)
	}
	if status := resolved[0].Intervals[0].StartEvidence.Status; status != domain.StatusImported {
		t.Fatalf("a night confirmed at one end only merged as %q", status)
	}
}

func TestSleepsThatOnlyTouchStaySeparate(t *testing.T) {
	resolved, err := ResolveOverlappingSleepReports([]domain.SleepSession{
		report("first", at(0, 0), at(8, 0)),
		report("second", at(8, 0), at(12, 0)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 {
		t.Fatalf("resolved count = %d, want two sleeps", len(resolved))
	}
}
