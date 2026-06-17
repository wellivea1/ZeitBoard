package readmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"non24.app/core/domain"
	"non24.app/core/ingest"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

type SleepReader struct {
	Store *store.Store
}

func (r SleepReader) EffectiveSleepSessions(ctx context.Context) ([]domain.SleepSession, error) {
	if r.Store == nil {
		return nil, errors.New("store is required")
	}
	records, err := allRecords(ctx, r.Store)
	if err != nil {
		return nil, err
	}
	var sessions []domain.SleepSession
	var corrections []domain.ManualCorrection
	for _, record := range records {
		switch record.Kind {
		case syncmodel.KindObservation:
			session, ok, err := sleepSessionFromPayload(record.Payload)
			if err != nil {
				return nil, fmt.Errorf("record %s: %w", record.RecordID, err)
			}
			if ok {
				sessions = append(sessions, session)
			}
		case syncmodel.KindCorrection:
			items, err := correctionsFromPayload(record.Payload)
			if err != nil {
				return nil, fmt.Errorf("record %s: %w", record.RecordID, err)
			}
			corrections = append(corrections, items...)
		}
	}
	corrections = markActiveCorrections(corrections)
	effective, err := domain.ApplySleepCorrections(sessions, corrections)
	if err != nil {
		return nil, err
	}
	resolved, err := ingest.ResolveOverlappingSleepReports(effective)
	if err != nil {
		return nil, err
	}
	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].Intervals[0].Interval.Start.UTC.Before(resolved[j].Intervals[0].Interval.Start.UTC)
	})
	return resolved, nil
}

func allRecords(ctx context.Context, st *store.Store) ([]syncmodel.Envelope, error) {
	var result []syncmodel.Envelope
	since := int64(0)
	for {
		records, cursor, err := st.Pull(ctx, since, syncmodel.MaxPullRecords)
		if err != nil {
			return nil, err
		}
		result = append(result, records...)
		if len(records) < syncmodel.MaxPullRecords || cursor == since {
			return result, nil
		}
		since = cursor
	}
}

type observationPayload struct {
	ObservationID string          `json:"observation_id"`
	Kind          string          `json:"kind"`
	StartAt       time.Time       `json:"start_at"`
	EndAt         time.Time       `json:"end_at"`
	ZoneID        string          `json:"zone_id"`
	Sleep         *sleepPayload   `json:"sleep,omitempty"`
	Provenance    provenance      `json:"provenance"`
	Activity      json.RawMessage `json:"activity,omitempty"`
}

type sleepPayload struct {
	Classification string `json:"classification"`
}

type provenance struct {
	AcquisitionMethod string    `json:"acquisition_method"`
	EvidenceStatus    string    `json:"evidence_status"`
	RecordedAt        time.Time `json:"recorded_at"`
	SourceRecordID    string    `json:"source_record_id,omitempty"`
}

func sleepSessionFromPayload(payload json.RawMessage) (domain.SleepSession, bool, error) {
	var obs observationPayload
	if err := json.Unmarshal(payload, &obs); err != nil {
		return domain.SleepSession{}, false, err
	}
	if obs.Kind != "sleep_episode" {
		return domain.SleepSession{}, false, nil
	}
	if obs.Sleep == nil {
		return domain.SleepSession{}, false, errors.New("sleep observation missing sleep details")
	}
	start, err := domain.NewZonedInstant(obs.StartAt, obs.ZoneID)
	if err != nil {
		return domain.SleepSession{}, false, err
	}
	end, err := domain.NewZonedInstant(obs.EndAt, obs.ZoneID)
	if err != nil {
		return domain.SleepSession{}, false, err
	}
	interval := domain.TimeRange{Start: start, End: end}
	if err := interval.Validate(); err != nil {
		return domain.SleepSession{}, false, err
	}
	evidence := domain.Evidence{
		Acquisition:    acquisitionKind(obs.Provenance.AcquisitionMethod),
		Status:         evidenceStatus(obs.Provenance.EvidenceStatus),
		ObservationIDs: []domain.ObservationID{domain.ObservationID(obs.ObservationID)},
		RecordedAt:     obs.Provenance.RecordedAt.UTC(),
	}
	if obs.Provenance.SourceRecordID != "" {
		evidence.SourceIDs = []domain.DataSourceID{domain.DataSourceID(obs.Provenance.SourceRecordID)}
	}
	return domain.SleepSession{
		ID:        domain.SleepSessionID(obs.ObservationID),
		IsNap:     obs.Sleep.Classification == "nap",
		CreatedAt: obs.Provenance.RecordedAt.UTC(),
		Intervals: []domain.SleepInterval{{
			Interval:      interval,
			StartEvidence: evidence,
			EndEvidence:   evidence,
		}},
		SourceLabel: sourceLabel(obs.Provenance.AcquisitionMethod),
	}, true, nil
}

type correctionPayload struct {
	CorrectionID           string            `json:"correction_id"`
	TargetObservationID    string            `json:"target_observation_id"`
	SupersedesCorrectionID string            `json:"supersedes_correction_id,omitempty"`
	CreatedAt              time.Time         `json:"created_at"`
	Reason                 string            `json:"reason"`
	Changes                correctionChanges `json:"changes"`
}

type correctionChanges struct {
	StartAt             *time.Time `json:"start_at,omitempty"`
	EndAt               *time.Time `json:"end_at,omitempty"`
	SleepClassification *string    `json:"sleep_classification,omitempty"`
	Excluded            *bool      `json:"excluded,omitempty"`
}

func correctionsFromPayload(payload json.RawMessage) ([]domain.ManualCorrection, error) {
	var correction correctionPayload
	if err := json.Unmarshal(payload, &correction); err != nil {
		return nil, err
	}
	base := domain.ManualCorrection{
		ID:           domain.CorrectionID(correction.CorrectionID),
		TargetID:     domain.SleepSessionID(correction.TargetObservationID),
		CreatedAt:    correction.CreatedAt.UTC(),
		SupersedesID: correctionIDPtr(correction.SupersedesCorrectionID),
		Active:       true,
	}
	var result []domain.ManualCorrection
	if correction.Changes.StartAt != nil {
		item := base
		item.Kind = domain.CorrectionSetSleepStart
		instant, err := domain.NewZonedInstant(*correction.Changes.StartAt, "UTC")
		if err != nil {
			return nil, err
		}
		item.InstantValue = &instant
		result = append(result, item)
	}
	if correction.Changes.EndAt != nil {
		item := base
		item.ID = suffixedCorrectionID(base.ID, "end")
		item.Kind = domain.CorrectionSetWakeTime
		instant, err := domain.NewZonedInstant(*correction.Changes.EndAt, "UTC")
		if err != nil {
			return nil, err
		}
		item.InstantValue = &instant
		result = append(result, item)
	}
	if correction.Changes.SleepClassification != nil {
		item := base
		item.ID = suffixedCorrectionID(base.ID, "class")
		item.Kind = domain.CorrectionClassifyNap
		value := *correction.Changes.SleepClassification == "nap"
		item.BoolValue = &value
		result = append(result, item)
	}
	if correction.Changes.Excluded != nil {
		item := base
		item.ID = suffixedCorrectionID(base.ID, "excluded")
		item.Kind = domain.CorrectionSuppress
		value := *correction.Changes.Excluded
		item.BoolValue = &value
		result = append(result, item)
	}
	return result, nil
}

func markActiveCorrections(corrections []domain.ManualCorrection) []domain.ManualCorrection {
	superseded := map[domain.CorrectionID]struct{}{}
	for _, correction := range corrections {
		if correction.SupersedesID != nil {
			superseded[*correction.SupersedesID] = struct{}{}
			superseded[suffixedCorrectionID(*correction.SupersedesID, "end")] = struct{}{}
			superseded[suffixedCorrectionID(*correction.SupersedesID, "class")] = struct{}{}
			superseded[suffixedCorrectionID(*correction.SupersedesID, "excluded")] = struct{}{}
		}
	}
	result := append([]domain.ManualCorrection(nil), corrections...)
	for i := range result {
		_, inactive := superseded[result[i].ID]
		result[i].Active = !inactive
	}
	return result
}

func correctionIDPtr(value string) *domain.CorrectionID {
	if value == "" {
		return nil
	}
	id := domain.CorrectionID(value)
	return &id
}

func suffixedCorrectionID(id domain.CorrectionID, suffix string) domain.CorrectionID {
	return domain.CorrectionID(string(id) + "_" + suffix)
}

func acquisitionKind(value string) domain.AcquisitionKind {
	switch value {
	case "health_connect", "file_import":
		return domain.AcquisitionImported
	case "os_activity":
		return domain.AcquisitionCollected
	default:
		return domain.AcquisitionManual
	}
}

func evidenceStatus(value string) domain.EvidenceStatus {
	switch value {
	case "directly_observed":
		return domain.StatusObserved
	case "inferred":
		return domain.StatusInferred
	default:
		return domain.StatusUserConfirmed
	}
}

func sourceLabel(value string) string {
	switch value {
	case "health_connect":
		return "Health Connect sleep"
	case "file_import":
		return "Imported sleep"
	case "os_activity":
		return "Device activity"
	case "synthetic":
		return "Synthetic sleep"
	default:
		return "Manual sleep log"
	}
}
