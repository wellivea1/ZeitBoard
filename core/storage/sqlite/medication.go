package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	medicationcore "non24.app/core/medication"
)

const (
	MedicationScheduleAsNeeded   = medicationcore.ScheduleAsNeeded
	MedicationScheduleFixedClock = medicationcore.ScheduleFixedClock
	MedicationScheduleCycling    = medicationcore.ScheduleCycling

	MedicationEventTaken   = medicationcore.EventTaken
	MedicationEventSkipped = medicationcore.EventSkipped

	MedicationCorrectionUserEdit    = medicationcore.CorrectionUserEdit
	MedicationCorrectionDuplicate   = medicationcore.CorrectionDuplicate
	MedicationCorrectionInvalidTime = medicationcore.CorrectionInvalidTime
)

var (
	ErrMedicationNotFound           = errors.New("medication does not exist")
	ErrMedicationRevisionConflict   = errors.New("medication revision conflict")
	ErrMedicationEventNotFound      = errors.New("medication event does not exist")
	ErrMedicationCorrectionConflict = errors.New("medication correction chain changed")
)

type MedicationSchedule = medicationcore.Schedule

// The contract records are defined and validated in core/medication, shared
// with the server's sync validation (ADR-0048).
type (
	MedicationRecord                 = medicationcore.Record
	MedicationEventRecord            = medicationcore.EventRecord
	MedicationEventCorrectionRecord  = medicationcore.CorrectionRecord
	MedicationEventCorrectionChanges = medicationcore.CorrectionChanges
)

type MedicationReminderClaim struct {
	OccurrenceID string
	MedicationID string
	ScheduledAt  time.Time
	ClaimedAt    time.Time
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

// ClaimMedicationReminder durably claims an occurrence before notification.
// A false result means the same medication occurrence was already claimed.
func (s *Store) ClaimMedicationReminder(ctx context.Context, claim MedicationReminderClaim) (bool, error) {
	if !contractIdentifier.MatchString(claim.OccurrenceID) {
		return false, errors.New("occurrence_id must match the v1 identifier format")
	}
	if !contractIdentifier.MatchString(claim.MedicationID) {
		return false, errors.New("medication_id must match the v1 identifier format")
	}
	if claim.ScheduledAt.IsZero() || claim.ClaimedAt.IsZero() {
		return false, errors.New("scheduled_at and claimed_at are required")
	}
	if claim.ClaimedAt.Before(claim.ScheduledAt) {
		return false, errors.New("claimed_at must not precede scheduled_at")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM local_medications WHERE medication_id = ?`, claim.MedicationID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return false, ErrMedicationNotFound
	} else if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_medication_reminder_claims(
		occurrence_id, medication_id, scheduled_at, claimed_at
	) VALUES(?, ?, ?, ?)`, claim.OccurrenceID, claim.MedicationID,
		formatSQLiteTime(claim.ScheduledAt.UTC()), formatSQLiteTime(claim.ClaimedAt.UTC()))
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if changed == 0 {
		var existingMedicationID, existingScheduledAt string
		if err := tx.QueryRowContext(ctx, `SELECT medication_id, scheduled_at
			FROM local_medication_reminder_claims WHERE occurrence_id = ?`, claim.OccurrenceID).
			Scan(&existingMedicationID, &existingScheduledAt); err != nil {
			return false, err
		}
		if existingMedicationID != claim.MedicationID || existingScheduledAt != formatSQLiteTime(claim.ScheduledAt.UTC()) {
			return false, errors.New("occurrence_id is already assigned to a different medication occurrence")
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed == 1, nil
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
		SchemaVersion: "v2",
		GeneratedAt:   generatedAt,
		MedicationSet: MedicationSet{
			SchemaVersion: "v2",
			GeneratedAt:   generatedAt,
			Medications:   medications,
		},
		EventSet: MedicationEventSet{
			SchemaVersion: "v2",
			GeneratedAt:   generatedAt,
			Events:        events,
			Corrections:   corrections,
		},
	}, nil
}

// DeleteMedication erases a definition with every dose, correction, schedule
// and reminder claim of it, and queues the erasure for the server so the
// owner's other devices lose it too (ADR-0048).
func (s *Store) DeleteMedication(ctx context.Context, medicationID string) error {
	if !contractIdentifier.MatchString(medicationID) {
		return errors.New("medication_id must match the v1 identifier format")
	}
	return s.eraseHere(ctx, medicationID, eraseMedicationTx, ErrMedicationNotFound)
}

// DeleteMedicationEvent erases a dose with its corrections, here and, through
// the server, everywhere.
func (s *Store) DeleteMedicationEvent(ctx context.Context, eventID string) error {
	if !contractIdentifier.MatchString(eventID) {
		return errors.New("event_id must match the v1 identifier format")
	}
	return s.eraseHere(ctx, eventID, eraseMedicationEventTx, ErrMedicationEventNotFound)
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

func validateMedicationRecord(record MedicationRecord) error { return record.Validate() }

func validateMedicationEvent(record MedicationEventRecord) error { return record.Validate() }

func validateMedicationCorrection(record MedicationEventCorrectionRecord) error {
	return record.Validate()
}

func (s *Store) requireMedication(ctx context.Context, medicationID string) error {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM local_medications WHERE medication_id = ?`, medicationID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMedicationNotFound
	}
	return err
}
