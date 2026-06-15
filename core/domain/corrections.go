package domain

import (
	"fmt"
	"sort"
)

func ApplySleepCorrections(source []SleepSession, corrections []ManualCorrection) ([]SleepSession, error) {
	effective := make([]SleepSession, len(source))
	for i := range source {
		effective[i] = cloneSleepSession(source[i])
	}
	index := make(map[SleepSessionID]int, len(effective))
	for i := range effective {
		index[effective[i].ID] = i
	}

	ordered := append([]ManualCorrection(nil), corrections...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].CreatedAt.Before(ordered[j].CreatedAt) })
	for _, correction := range ordered {
		if !correction.Active {
			continue
		}
		position, ok := index[correction.TargetID]
		if !ok {
			return nil, fmt.Errorf("correction %s targets unknown sleep session %s", correction.ID, correction.TargetID)
		}
		session := &effective[position]
		if len(session.Intervals) == 0 {
			return nil, fmt.Errorf("sleep session %s has no intervals", session.ID)
		}
		interval := &session.Intervals[0]
		switch correction.Kind {
		case CorrectionSetSleepStart:
			if correction.InstantValue == nil {
				return nil, fmt.Errorf("correction %s requires instantValue", correction.ID)
			}
			interval.Interval.Start = *correction.InstantValue
			interval.StartEvidence = correctedEvidence(interval.StartEvidence, correction.ID, StatusUserConfirmed)
		case CorrectionSetWakeTime:
			if correction.InstantValue == nil {
				return nil, fmt.Errorf("correction %s requires instantValue", correction.ID)
			}
			interval.Interval.End = *correction.InstantValue
			interval.EndEvidence = correctedEvidence(interval.EndEvidence, correction.ID, StatusUserConfirmed)
		case CorrectionConfirmStart:
			interval.StartEvidence = correctedEvidence(interval.StartEvidence, correction.ID, StatusUserConfirmed)
		case CorrectionConfirmWake:
			interval.EndEvidence = correctedEvidence(interval.EndEvidence, correction.ID, StatusUserConfirmed)
		case CorrectionClassifyNap:
			if correction.BoolValue == nil {
				return nil, fmt.Errorf("correction %s requires boolValue", correction.ID)
			}
			session.IsNap = *correction.BoolValue
		case CorrectionAwakeInactive:
			if correction.IntervalValue == nil {
				return nil, fmt.Errorf("correction %s requires intervalValue", correction.ID)
			}
			if err := correction.IntervalValue.Validate(); err != nil {
				return nil, fmt.Errorf("correction %s has invalid awake-but-inactive interval: %w", correction.ID, err)
			}
			// This correction is retained as provenance for the effective read model.
			// It does not convert the inactive interval into a sleep interval.
			interval.StartEvidence.CorrectionIDs = append(interval.StartEvidence.CorrectionIDs, correction.ID)
			interval.EndEvidence.CorrectionIDs = append(interval.EndEvidence.CorrectionIDs, correction.ID)
		case CorrectionSuppress:
			if correction.BoolValue == nil {
				return nil, fmt.Errorf("correction %s requires boolValue", correction.ID)
			}
			session.Suppressed = *correction.BoolValue
		default:
			return nil, fmt.Errorf("unsupported correction kind %q", correction.Kind)
		}
		if err := interval.Interval.Validate(); err != nil {
			return nil, fmt.Errorf("correction %s creates invalid interval: %w", correction.ID, err)
		}
	}
	return effective, nil
}

func cloneSleepSession(source SleepSession) SleepSession {
	cloned := source
	cloned.Intervals = append([]SleepInterval(nil), source.Intervals...)
	for i := range cloned.Intervals {
		cloned.Intervals[i].StartEvidence = cloneEvidence(source.Intervals[i].StartEvidence)
		cloned.Intervals[i].EndEvidence = cloneEvidence(source.Intervals[i].EndEvidence)
	}
	return cloned
}

func cloneEvidence(source Evidence) Evidence {
	cloned := source
	cloned.SourceIDs = append([]DataSourceID(nil), source.SourceIDs...)
	cloned.ObservationIDs = append([]ObservationID(nil), source.ObservationIDs...)
	cloned.CorrectionIDs = append([]CorrectionID(nil), source.CorrectionIDs...)
	return cloned
}

func correctedEvidence(source Evidence, correctionID CorrectionID, status EvidenceStatus) Evidence {
	result := cloneEvidence(source)
	result.Status = status
	result.CorrectionIDs = append(result.CorrectionIDs, correctionID)
	return result
}
