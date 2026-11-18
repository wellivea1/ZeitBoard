package ingest

import (
	"fmt"
	"sort"
	"time"

	"non24.app/core/domain"
)

// ResolveOverlappingSleepReports creates an effective read model while retaining
// every source observation in the boundary evidence.
//
// Reports that overlap describe one sleep, and are merged as a group. They
// used to be merged two at a time, each result averaged with the next report,
// which weighted the later reports more: starts of 00:00, 01:00 and 02:00 gave
// 01:15, where the middle one is 01:00. The same pairing let an automatic
// source move the boundaries of a night the owner had confirmed while the
// result still read as confirmed.
func ResolveOverlappingSleepReports(reports []domain.SleepSession) ([]domain.SleepSession, error) {
	ordered := append([]domain.SleepSession(nil), reports...)
	sort.Slice(ordered, func(i, j int) bool {
		return firstInterval(ordered[i]).Start.UTC.Before(firstInterval(ordered[j]).Start.UTC)
	})

	var (
		result   []domain.SleepSession
		group    []domain.SleepSession
		groupEnd time.Time
	)
	flush := func() error {
		if len(group) == 0 {
			return nil
		}
		merged, err := mergeGroup(group)
		if err != nil {
			return err
		}
		result = append(result, merged)
		group = nil
		return nil
	}
	for _, report := range ordered {
		if len(report.Intervals) == 0 {
			return nil, fmt.Errorf("sleep report %s has no interval", report.ID)
		}
		interval := report.Intervals[0].Interval
		if err := interval.Validate(); err != nil {
			return nil, fmt.Errorf("sleep report %s: %w", report.ID, err)
		}
		// A report joins the group it overlaps: it starts before the group's
		// latest end. Intervals are half-open, so one that starts where the
		// group ends is the next sleep.
		if len(group) > 0 && !interval.Start.UTC.Before(groupEnd) {
			if err := flush(); err != nil {
				return nil, err
			}
		}
		group = append(group, report)
		if interval.End.UTC.After(groupEnd) || len(group) == 1 {
			groupEnd = interval.End.UTC
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return result, nil
}

// mergeGroup is the one sleep a group of overlapping reports describes.
//
// What the owner confirmed stands: where any report in the group is
// user-confirmed at both ends, the boundaries are the middle of the confirmed
// reports alone, and the result reads as confirmed. Otherwise the boundaries
// are the middle of every report and the result reads as imported, which is
// what a time no single source gave is. Every report's identifiers are kept
// either way.
func mergeGroup(group []domain.SleepSession) (domain.SleepSession, error) {
	if len(group) == 1 {
		return group[0], nil
	}
	deciding := group
	var confirmed []domain.SleepSession
	for _, member := range group {
		interval := member.Intervals[0]
		if interval.StartEvidence.Status == domain.StatusUserConfirmed && interval.EndEvidence.Status == domain.StatusUserConfirmed {
			confirmed = append(confirmed, member)
		}
	}
	if len(confirmed) > 0 {
		deciding = confirmed
	}
	starts := make([]time.Time, 0, len(deciding))
	ends := make([]time.Time, 0, len(deciding))
	for _, member := range deciding {
		starts = append(starts, member.Intervals[0].Interval.Start.UTC)
		ends = append(ends, member.Intervals[0].Interval.End.UTC)
	}
	start, end := medianTime(starts), medianTime(ends)
	if !end.After(start) {
		// The middles of reports that barely overlap can cross. The group's
		// own span is then the honest reading of it.
		start, end = starts[0], ends[0]
		for i := range starts {
			if starts[i].Before(start) {
				start = starts[i]
			}
			if ends[i].After(end) {
				end = ends[i]
			}
		}
	}
	zone := group[0].Intervals[0].Interval.Start.ZoneID
	startInstant, err := domain.NewZonedInstant(start, zone)
	if err != nil {
		return domain.SleepSession{}, err
	}
	endInstant, err := domain.NewZonedInstant(end, zone)
	if err != nil {
		return domain.SleepSession{}, err
	}

	classification := group[0].EffectiveClassification()
	id := string(group[0].ID)
	createdAt := group[0].CreatedAt
	startEvidence := make([]domain.Evidence, 0, len(group))
	endEvidence := make([]domain.Evidence, 0, len(group))
	for index, member := range group {
		if member.EffectiveClassification() != classification {
			classification = domain.SleepClassificationUnknown
		}
		if index > 0 {
			id += "+" + string(member.ID)
		}
		createdAt = maxTime(createdAt, member.CreatedAt)
		startEvidence = append(startEvidence, member.Intervals[0].StartEvidence)
		endEvidence = append(endEvidence, member.Intervals[0].EndEvidence)
	}
	status := domain.StatusImported
	if len(confirmed) > 0 {
		status = domain.StatusUserConfirmed
	}
	return domain.SleepSession{
		ID: domain.SleepSessionID(id),
		Intervals: []domain.SleepInterval{{
			Interval:      domain.TimeRange{Start: startInstant, End: endInstant},
			StartEvidence: mergeEvidence(status, startEvidence...),
			EndEvidence:   mergeEvidence(status, endEvidence...),
		}},
		Classification: classification,
		IsNap:          classification == domain.SleepClassificationNap,
		CreatedAt:      createdAt,
	}, nil
}

// mergeEvidence keeps every report's identifiers under the status the merged
// boundary has earned.
func mergeEvidence(status domain.EvidenceStatus, values ...domain.Evidence) domain.Evidence {
	result := domain.Evidence{Acquisition: domain.AcquisitionImported, Status: status}
	for _, value := range values {
		result.SourceIDs = append(result.SourceIDs, value.SourceIDs...)
		result.ObservationIDs = append(result.ObservationIDs, value.ObservationIDs...)
		result.CorrectionIDs = append(result.CorrectionIDs, value.CorrectionIDs...)
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
