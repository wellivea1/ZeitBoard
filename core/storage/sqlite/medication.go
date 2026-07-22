package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"non24.app/core/domain"
)

const (
	MedicationScheduleAsNeeded   = "as_needed"
	MedicationScheduleFixedClock = "fixed_clock"
	MedicationScheduleCycling    = "cycling"

	MedicationEventTaken   = "taken"
	MedicationEventSkipped = "skipped"

	MedicationCorrectionUserEdit    = "user_edit"
	MedicationCorrectionDuplicate   = "duplicate"
	MedicationCorrectionInvalidTime = "invalid_time"
)

var medicationCivilTime = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)

var (
	ErrMedicationNotFound           = errors.New("medication does not exist")
	ErrMedicationRevisionConflict   = errors.New("medication revision conflict")
	ErrMedicationEventNotFound      = errors.New("medication event does not exist")
	ErrMedicationCorrectionConflict = errors.New("medication correction chain changed")
)

type MedicationSchedule struct {
	Kind           string   `json:"kind"`
	CivilTimes     []string `json:"civil_times,omitempty"`
	DaysOn         int      `json:"days_on,omitempty"`
	DaysOff        int      `json:"days_off,omitempty"`
	CycleStartedOn string   `json:"cycle_started_on,omitempty"`
}

// MedicationRecord matches medication-set.schema.json#/$defs/medication.
type MedicationRecord struct {
	MedicationID  string              `json:"medication_id"`
	Label         string              `json:"label"`
	Form          string              `json:"form,omitempty"`
	StrengthLabel string              `json:"strength_label,omitempty"`
	Active        bool                `json:"active"`
	StartedAt     *time.Time          `json:"started_at,omitempty"`
	StartedZoneID string              `json:"started_zone_id,omitempty"`
	Schedule      *MedicationSchedule `json:"schedule,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	Revision      int                 `json:"revision"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

type MedicationEventRecord struct {
	EventID      string                     `json:"event_id"`
	MedicationID string                     `json:"medication_id"`
	DoseAt       time.Time                  `json:"dose_at"`
	ZoneID       string                     `json:"zone_id"`
	Status       string                     `json:"status"`
	Scheduled    bool                       `json:"scheduled"`
	Note         string                     `json:"note,omitempty"`
	Provenance   SleepObservationProvenance `json:"provenance"`
}

type MedicationEventCorrectionRecord struct {
	CorrectionID           string                           `json:"correction_id"`
	TargetEventID          string                           `json:"target_event_id"`
	SupersedesCorrectionID string                           `json:"supersedes_correction_id,omitempty"`
	CreatedAt              time.Time                        `json:"created_at"`
	Reason                 string                           `json:"reason"`
	Changes                MedicationEventCorrectionChanges `json:"changes"`
}

type MedicationEventCorrectionChanges struct {
	DoseAt    *time.Time `json:"dose_at,omitempty"`
	ZoneID    *string    `json:"zone_id,omitempty"`
	Status    *string    `json:"status,omitempty"`
	Scheduled *bool      `json:"scheduled,omitempty"`
	Note      *string    `json:"note,omitempty"`
	Excluded  *bool      `json:"excluded,omitempty"`
}

type EffectiveMedicationEvent struct {
	Event       MedicationEventRecord
	Excluded    bool
	Corrections []MedicationEventCorrectionRecord
}

type MedicationSet struct {
	SchemaVersion string             `json:"schema_version"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Medications   []MedicationRecord `json:"medications"`
}

type MedicationEventSet struct {
	SchemaVersion string                            `json:"schema_version"`
	GeneratedAt   time.Time                         `json:"generated_at"`
	Events        []MedicationEventRecord           `json:"events"`
	Corrections   []MedicationEventCorrectionRecord `json:"corrections"`
}

type MedicationDataExport struct {
	SchemaVersion string             `json:"schema_version"`
	GeneratedAt   time.Time          `json:"generated_at"`
	MedicationSet MedicationSet      `json:"medication_set"`
	EventSet      MedicationEventSet `json:"event_set"`
}

func (s *Store) CreateMedication(ctx context.Context, record MedicationRecord) error {
	record = normalizeMedicationRecord(record)
	if err := validateMedicationRecord(record); err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO local_medications(
		medication_id, active, revision, created_at, updated_at, payload_json
	) VALUES(?, ?, ?, ?, ?, ?)`,
		record.MedicationID, boolInt(record.Active), record.Revision,
		formatSQLiteTime(record.CreatedAt), formatSQLiteTime(record.UpdatedAt), encoded,
	)
	return err
}

func (s *Store) UpdateMedication(ctx context.Context, record MedicationRecord, expectedRevision int) error {
	record = normalizeMedicationRecord(record)
	if expectedRevision < 1 || record.Revision != expectedRevision+1 {
		return errors.New("updated medication revision must increment the expected revision")
	}
	if err := validateMedicationRecord(record); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentRevision int
	var currentPayload []byte
	err = tx.QueryRowContext(ctx, `SELECT revision, payload_json FROM local_medications WHERE medication_id = ?`, record.MedicationID).Scan(&currentRevision, &currentPayload)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMedicationNotFound
	}
	if err != nil {
		return err
	}
	if currentRevision != expectedRevision {
		return ErrMedicationRevisionConflict
	}
	var current MedicationRecord
	if err := json.Unmarshal(currentPayload, &current); err != nil {
		return err
	}
	if !record.CreatedAt.Equal(current.CreatedAt) {
		return errors.New("medication created_at is immutable")
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE local_medications
		SET active = ?, revision = ?, updated_at = ?, payload_json = ?
		WHERE medication_id = ? AND revision = ?`,
		boolInt(record.Active), record.Revision, formatSQLiteTime(record.UpdatedAt), encoded,
		record.MedicationID, expectedRevision,
	)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 1 {
		return tx.Commit()
	}
	return ErrMedicationRevisionConflict
}

func (s *Store) ListMedications(ctx context.Context) ([]MedicationRecord, error) {
	records := make([]MedicationRecord, 0)
	err := s.readJSONRows(ctx, `SELECT payload_json FROM local_medications ORDER BY created_at, medication_id`, func(value []byte) error {
		var record MedicationRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	return records, err
}

func (s *Store) MedicationByID(ctx context.Context, medicationID string) (MedicationRecord, error) {
	if !contractIdentifier.MatchString(medicationID) {
		return MedicationRecord{}, errors.New("medication_id must match the v1 identifier format")
	}
	var encoded []byte
	if err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM local_medications WHERE medication_id = ?`, medicationID).Scan(&encoded); errors.Is(err, sql.ErrNoRows) {
		return MedicationRecord{}, ErrMedicationNotFound
	} else if err != nil {
		return MedicationRecord{}, err
	}
	var record MedicationRecord
	if err := json.Unmarshal(encoded, &record); err != nil {
		return MedicationRecord{}, err
	}
	return record, nil
}

func (s *Store) AppendMedicationEvent(ctx context.Context, record MedicationEventRecord) error {
	record = normalizeMedicationEvent(record)
	if err := validateMedicationEvent(record); err != nil {
		return err
	}
	if err := s.requireMedication(ctx, record.MedicationID); err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO local_medication_events(
		event_id, medication_id, dose_at, status, scheduled, recorded_at, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		record.EventID, record.MedicationID, formatSQLiteTime(record.DoseAt), record.Status,
		boolInt(record.Scheduled), formatSQLiteTime(record.Provenance.RecordedAt), encoded,
	)
	return err
}

func (s *Store) ListMedicationEvents(ctx context.Context) ([]MedicationEventRecord, error) {
	records := make([]MedicationEventRecord, 0)
	err := s.readJSONRows(ctx, `SELECT payload_json FROM local_medication_events ORDER BY dose_at, event_id`, func(value []byte) error {
		var record MedicationEventRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	return records, err
}

func (s *Store) AppendMedicationEventCorrection(ctx context.Context, record MedicationEventCorrectionRecord) error {
	record = normalizeMedicationCorrection(record)
	if err := validateMedicationCorrection(record); err != nil {
		return err
	}
	changes, err := json.Marshal(record.Changes)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var eventRecordedAtText string
	if err := tx.QueryRowContext(ctx, `SELECT recorded_at FROM local_medication_events WHERE event_id = ?`, record.TargetEventID).Scan(&eventRecordedAtText); errors.Is(err, sql.ErrNoRows) {
		return ErrMedicationEventNotFound
	} else if err != nil {
		return err
	}
	eventRecordedAt, err := time.Parse(time.RFC3339Nano, eventRecordedAtText)
	if err != nil {
		return err
	}
	if !record.CreatedAt.After(eventRecordedAt) {
		return errors.New("medication correction must be created after the event was recorded")
	}
	var latest, latestAtText string
	err = tx.QueryRowContext(ctx, `SELECT correction_id, created_at FROM local_medication_event_corrections
		WHERE target_event_id = ? ORDER BY created_at DESC, correction_id DESC LIMIT 1`, record.TargetEventID).Scan(&latest, &latestAtText)
	if errors.Is(err, sql.ErrNoRows) {
		latest = ""
	} else if err != nil {
		return err
	}
	if latest != record.SupersedesCorrectionID {
		return ErrMedicationCorrectionConflict
	}
	if latest != "" {
		latestAt, err := time.Parse(time.RFC3339Nano, latestAtText)
		if err != nil {
			return err
		}
		if !record.CreatedAt.After(latestAt) {
			return errors.New("medication correction must be created after the correction it supersedes")
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_medication_event_corrections(
		correction_id, target_event_id, supersedes_correction_id, created_at, reason, changes_json, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		record.CorrectionID, record.TargetEventID, record.SupersedesCorrectionID,
		formatSQLiteTime(record.CreatedAt), record.Reason, changes, encoded,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListMedicationEventCorrections(ctx context.Context) ([]MedicationEventCorrectionRecord, error) {
	records := make([]MedicationEventCorrectionRecord, 0)
	err := s.readJSONRows(ctx, `SELECT payload_json FROM local_medication_event_corrections ORDER BY created_at, correction_id`, func(value []byte) error {
		var record MedicationEventCorrectionRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	return records, err
}

func (s *Store) EffectiveMedicationEvents(ctx context.Context) ([]EffectiveMedicationEvent, error) {
	events, err := s.ListMedicationEvents(ctx)
	if err != nil {
		return nil, err
	}
	corrections, err := s.ListMedicationEventCorrections(ctx)
	if err != nil {
		return nil, err
	}
	byEvent := make(map[string][]MedicationEventCorrectionRecord)
	for _, correction := range corrections {
		byEvent[correction.TargetEventID] = append(byEvent[correction.TargetEventID], correction)
	}
	result := make([]EffectiveMedicationEvent, 0, len(events))
	for _, event := range events {
		item := EffectiveMedicationEvent{Event: event, Corrections: byEvent[event.EventID]}
		for _, correction := range item.Corrections {
			changes := correction.Changes
			if changes.DoseAt != nil {
				item.Event.DoseAt = changes.DoseAt.UTC()
			}
			if changes.ZoneID != nil {
				item.Event.ZoneID = *changes.ZoneID
			}
			if changes.Status != nil {
				item.Event.Status = *changes.Status
			}
			if changes.Scheduled != nil {
				item.Event.Scheduled = *changes.Scheduled
			}
			if changes.Note != nil {
				item.Event.Note = *changes.Note
			}
			if changes.Excluded != nil {
				item.Excluded = *changes.Excluded
			}
		}
		if err := validateMedicationEvent(item.Event); err != nil {
			return nil, fmt.Errorf("effective medication event %s: %w", event.EventID, err)
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Event.DoseAt.Equal(result[j].Event.DoseAt) {
			return result[i].Event.EventID < result[j].Event.EventID
		}
		return result[i].Event.DoseAt.Before(result[j].Event.DoseAt)
	})
	return result, nil
}

func (s *Store) LatestMedicationEventCorrectionID(ctx context.Context, eventID string) (string, error) {
	var correctionID string
	err := s.db.QueryRowContext(ctx, `SELECT correction_id FROM local_medication_event_corrections
		WHERE target_event_id = ? ORDER BY created_at DESC, correction_id DESC LIMIT 1`, eventID).Scan(&correctionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return correctionID, err
}

func (s *Store) ExportMedicationData(ctx context.Context, generatedAt time.Time) (MedicationDataExport, error) {
	medications, err := s.ListMedications(ctx)
	if err != nil {
		return MedicationDataExport{}, err
	}
	events, err := s.ListMedicationEvents(ctx)
	if err != nil {
		return MedicationDataExport{}, err
	}
	corrections, err := s.ListMedicationEventCorrections(ctx)
	if err != nil {
		return MedicationDataExport{}, err
	}
	generatedAt = generatedAt.UTC()
	return MedicationDataExport{
		SchemaVersion: "v1",
		GeneratedAt:   generatedAt,
		MedicationSet: MedicationSet{
			SchemaVersion: "v1",
			GeneratedAt:   generatedAt,
			Medications:   medications,
		},
		EventSet: MedicationEventSet{
			SchemaVersion: "v1",
			GeneratedAt:   generatedAt,
			Events:        events,
			Corrections:   corrections,
		},
	}, nil
}

func (s *Store) DeleteMedication(ctx context.Context, medicationID string) error {
	if !contractIdentifier.MatchString(medicationID) {
		return errors.New("medication_id must match the v1 identifier format")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM local_medications WHERE medication_id = ?`, medicationID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrMedicationNotFound
	}
	return s.compactDeletedData(ctx)
}

func (s *Store) DeleteMedicationEvent(ctx context.Context, eventID string) error {
	if !contractIdentifier.MatchString(eventID) {
		return errors.New("event_id must match the v1 identifier format")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM local_medication_events WHERE event_id = ?`, eventID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrMedicationEventNotFound
	}
	return s.compactDeletedData(ctx)
}

func normalizeMedicationRecord(record MedicationRecord) MedicationRecord {
	record.CreatedAt = record.CreatedAt.UTC()
	record.UpdatedAt = record.UpdatedAt.UTC()
	if record.StartedAt != nil {
		started := record.StartedAt.UTC()
		record.StartedAt = &started
	}
	if record.Schedule != nil {
		schedule := *record.Schedule
		schedule.CivilTimes = append([]string(nil), schedule.CivilTimes...)
		sort.Strings(schedule.CivilTimes)
		record.Schedule = &schedule
	}
	return record
}

func normalizeMedicationEvent(record MedicationEventRecord) MedicationEventRecord {
	record.DoseAt = record.DoseAt.UTC()
	record.Provenance.RecordedAt = record.Provenance.RecordedAt.UTC()
	return record
}

func normalizeMedicationCorrection(record MedicationEventCorrectionRecord) MedicationEventCorrectionRecord {
	record.CreatedAt = record.CreatedAt.UTC()
	if record.Changes.DoseAt != nil {
		doseAt := record.Changes.DoseAt.UTC()
		record.Changes.DoseAt = &doseAt
	}
	return record
}

func validateMedicationRecord(record MedicationRecord) error {
	if !contractIdentifier.MatchString(record.MedicationID) {
		return errors.New("medication_id must match the v1 identifier format")
	}
	if err := canonicalPrivateText("label", record.Label, 120, true); err != nil {
		return err
	}
	if err := canonicalPrivateText("form", record.Form, 80, false); err != nil {
		return err
	}
	if err := canonicalPrivateText("strength_label", record.StrengthLabel, 80, false); err != nil {
		return err
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return errors.New("created_at and updated_at are required")
	}
	if record.UpdatedAt.Before(record.CreatedAt) {
		return errors.New("updated_at must not precede created_at")
	}
	if record.Revision < 1 {
		return errors.New("revision must be at least 1")
	}
	if (record.StartedAt == nil) != (record.StartedZoneID == "") {
		return errors.New("started_at and started_zone_id must be set together")
	}
	if record.StartedAt != nil {
		if _, err := domain.NewZonedInstant(*record.StartedAt, record.StartedZoneID); err != nil {
			return fmt.Errorf("medication start: %w", err)
		}
	}
	if record.Schedule != nil {
		if err := validateMedicationSchedule(*record.Schedule); err != nil {
			return err
		}
	}
	return nil
}

func validateMedicationSchedule(schedule MedicationSchedule) error {
	seen := make(map[string]struct{}, len(schedule.CivilTimes))
	for _, civilTime := range schedule.CivilTimes {
		if !medicationCivilTime.MatchString(civilTime) {
			return errors.New("schedule civil times must use HH:MM")
		}
		if _, duplicate := seen[civilTime]; duplicate {
			return errors.New("schedule civil times must be unique")
		}
		seen[civilTime] = struct{}{}
	}
	switch schedule.Kind {
	case MedicationScheduleAsNeeded:
		if len(schedule.CivilTimes) != 0 || schedule.DaysOn != 0 || schedule.DaysOff != 0 || schedule.CycleStartedOn != "" {
			return errors.New("as-needed schedules cannot include clock or cycle fields")
		}
	case MedicationScheduleFixedClock:
		if len(schedule.CivilTimes) == 0 || len(schedule.CivilTimes) > 8 || schedule.DaysOn != 0 || schedule.DaysOff != 0 || schedule.CycleStartedOn != "" {
			return errors.New("fixed-clock schedules require 1 to 8 civil times and no cycle fields")
		}
	case MedicationScheduleCycling:
		if len(schedule.CivilTimes) == 0 || len(schedule.CivilTimes) > 8 || schedule.DaysOn < 1 || schedule.DaysOn > 365 || schedule.DaysOff < 1 || schedule.DaysOff > 365 {
			return errors.New("cycling schedules require civil times and 1 to 365 on/off days")
		}
		parsed, err := time.Parse("2006-01-02", schedule.CycleStartedOn)
		if err != nil || parsed.Format("2006-01-02") != schedule.CycleStartedOn {
			return errors.New("cycling schedule start must be a real YYYY-MM-DD date")
		}
	default:
		return errors.New("schedule kind must be as_needed, fixed_clock, or cycling")
	}
	return nil
}

func validateMedicationEvent(record MedicationEventRecord) error {
	if !contractIdentifier.MatchString(record.EventID) {
		return errors.New("event_id must match the v1 identifier format")
	}
	if !contractIdentifier.MatchString(record.MedicationID) {
		return errors.New("medication_id must match the v1 identifier format")
	}
	if record.Status != MedicationEventTaken && record.Status != MedicationEventSkipped {
		return errors.New("status must be taken or skipped")
	}
	if record.DoseAt.IsZero() {
		return errors.New("dose_at is required")
	}
	if _, err := domain.NewZonedInstant(record.DoseAt, record.ZoneID); err != nil {
		return err
	}
	if err := canonicalPrivateText("note", record.Note, 500, false); err != nil {
		return err
	}
	if !validAcquisition(record.Provenance.AcquisitionMethod) || !validEvidenceStatus(record.Provenance.EvidenceStatus) {
		return errors.New("medication event provenance is not supported")
	}
	if record.Provenance.RecordedAt.IsZero() {
		return errors.New("provenance.recorded_at is required")
	}
	if sourceRecordID := record.Provenance.SourceRecordID; sourceRecordID != "" {
		if strings.TrimSpace(sourceRecordID) != sourceRecordID || len(sourceRecordID) > 128 {
			return errors.New("provenance.source_record_id must be canonical text up to 128 characters")
		}
	}
	return nil
}

func validateMedicationCorrection(record MedicationEventCorrectionRecord) error {
	if !contractIdentifier.MatchString(record.CorrectionID) || !contractIdentifier.MatchString(record.TargetEventID) {
		return errors.New("correction_id and target_event_id must match the v1 identifier format")
	}
	if record.SupersedesCorrectionID != "" && !contractIdentifier.MatchString(record.SupersedesCorrectionID) {
		return errors.New("supersedes_correction_id must match the v1 identifier format")
	}
	if record.CreatedAt.IsZero() {
		return errors.New("created_at is required")
	}
	if record.Reason != MedicationCorrectionUserEdit && record.Reason != MedicationCorrectionDuplicate && record.Reason != MedicationCorrectionInvalidTime {
		return errors.New("medication correction reason is not supported")
	}
	changes := record.Changes
	count := 0
	if changes.DoseAt != nil {
		if changes.DoseAt.IsZero() {
			return errors.New("changes.dose_at is invalid")
		}
		count++
	}
	if changes.ZoneID != nil {
		if strings.TrimSpace(*changes.ZoneID) == "" {
			return errors.New("changes.zone_id is invalid")
		}
		if _, err := domain.NewZonedInstant(time.Unix(0, 0).UTC(), *changes.ZoneID); err != nil {
			return fmt.Errorf("changes.zone_id: %w", err)
		}
		count++
	}
	if changes.Status != nil {
		if *changes.Status != MedicationEventTaken && *changes.Status != MedicationEventSkipped {
			return errors.New("changes.status must be taken or skipped")
		}
		count++
	}
	if changes.Scheduled != nil {
		count++
	}
	if changes.Note != nil {
		if len(*changes.Note) > 500 || strings.TrimSpace(*changes.Note) != *changes.Note {
			return errors.New("changes.note must be canonical private text up to 500 characters")
		}
		count++
	}
	if changes.Excluded != nil {
		count++
	}
	if count == 0 {
		return errors.New("medication correction changes must not be empty")
	}
	return nil
}

func canonicalPrivateText(field, value string, maximum int, required bool) error {
	if strings.TrimSpace(value) != value || len(value) > maximum || (required && value == "") {
		return fmt.Errorf("%s must be canonical private text up to %d characters", field, maximum)
	}
	return nil
}

func (s *Store) requireMedication(ctx context.Context, medicationID string) error {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM local_medications WHERE medication_id = ?`, medicationID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMedicationNotFound
	}
	return err
}
