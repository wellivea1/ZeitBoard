package ingest

import (
	"fmt"
	"sort"
	"time"

	"non24.app/core/domain"
)

// ResolveOverlappingSleepReports creates an effective read model while retaining
// every source observation in the boundary evidence.
func ResolveOverlappingSleepReports(reports []domain.SleepSession) ([]domain.SleepSession, error) {
	principal := append([]domain.SleepSession(nil), reports...)
	sort.Slice(principal, func(i, j int) bool {
		return firstInterval(principal[i]).Start.UTC.Before(firstInterval(principal[j]).Start.UTC)
	})

	var result []domain.SleepSession
	for _, report := range principal {
		if len(report.Intervals) == 0 {
			return nil, fmt.Errorf("sleep report %s has no interval", report.ID)
		}
		if err := report.Intervals[0].Interval.Validate(); err != nil {
			return nil, fmt.Errorf("sleep report %s: %w", report.ID, err)
		}
		if len(result) == 0 || !firstInterval(result[len(result)-1]).Overlaps(firstInterval(report)) {
			result = append(result, report)
			continue
		}
		merged, err := mergeSessions(result[len(result)-1], report)
		if err != nil {
			return nil, err
		}
		result[len(result)-1] = merged
	}
	return result, nil
}

func mergeSessions(left, right domain.SleepSession) (domain.SleepSession, error) {
	leftInterval := left.Intervals[0]
	rightInterval := right.Intervals[0]
	zone := leftInterval.Interval.Start.ZoneID
	start := medianTime([]time.Time{leftInterval.Interval.Start.UTC, rightInterval.Interval.Start.UTC})
	end := medianTime([]time.Time{leftInterval.Interval.End.UTC, rightInterval.Interval.End.UTC})
	startInstant, err := domain.NewZonedInstant(start, zone)
	if err != nil {
		return domain.SleepSession{}, err
	}
	endInstant, err := domain.NewZonedInstant(end, zone)
	if err != nil {
		return domain.SleepSession{}, err
	}
	classification := left.EffectiveClassification()
	if classification != right.EffectiveClassification() {
		classification = domain.SleepClassificationUnknown
	}
	merged := domain.SleepSession{
		ID: domain.SleepSessionID(string(left.ID) + "+" + string(right.ID)),
		Intervals: []domain.SleepInterval{{
			Interval:      domain.TimeRange{Start: startInstant, End: endInstant},
			StartEvidence: mergeEvidence(leftInterval.StartEvidence, rightInterval.StartEvidence),
			EndEvidence:   mergeEvidence(leftInterval.EndEvidence, rightInterval.EndEvidence),
		}},
		Classification: classification,
		IsNap:          classification == domain.SleepClassificationNap,
		CreatedAt:      maxTime(left.CreatedAt, right.CreatedAt),
	}
	return merged, nil
}

func mergeEvidence(values ...domain.Evidence) domain.Evidence {
	result := domain.Evidence{Acquisition: domain.AcquisitionImported, Status: domain.StatusImported}
	for _, value := range values {
		result.SourceIDs = append(result.SourceIDs, value.SourceIDs...)
		result.ObservationIDs = append(result.ObservationIDs, value.ObservationIDs...)
		result.CorrectionIDs = append(result.CorrectionIDs, value.CorrectionIDs...)
		if value.Status == domain.StatusUserConfirmed {
			result.Status = domain.StatusUserConfirmed
		}
		if value.RecordedAt.After(result.RecordedAt) {
			result.RecordedAt = value.RecordedAt
		}
	}
	return result
}

func medianTime(values []time.Time) time.Time {
	sort.Slice(values, func(i, j int) bool { return values[i].Before(values[j]) })
	if len(values)%2 == 1 {
		return values[len(values)/2]
	}
	left := values[len(values)/2-1]
	right := values[len(values)/2]
	return left.Add(right.Sub(left) / 2)
}

func firstInterval(session domain.SleepSession) domain.TimeRange {
	if len(session.Intervals) == 0 {
		return domain.TimeRange{}
	}
	return session.Intervals[0].Interval
}

func maxTime(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}
