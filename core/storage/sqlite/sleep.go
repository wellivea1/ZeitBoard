package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"non24.app/core/domain"
	"non24.app/core/ingest"
)

const (
	SleepKindEpisode = "sleep_episode"

	SleepClassificationPrincipal = "principal"
	SleepClassificationNap       = "nap"
	SleepClassificationUnknown   = "unknown"

	ProvenanceAcquisitionManual        = "manual"
	ProvenanceAcquisitionHealthConnect = "health_connect"
	ProvenanceAcquisitionOSActivity    = "os_activity"
	ProvenanceAcquisitionFileImport    = "file_import"
	ProvenanceAcquisitionSynthetic     = "synthetic"

	ProvenanceEvidenceDirectlyObserved = "directly_observed"
	ProvenanceEvidenceUserReported     = "user_reported"
	ProvenanceEvidenceInferred         = "inferred"

	CorrectionReasonUserEdit       = "user_edit"
	CorrectionReasonDuplicate      = "duplicate"
	CorrectionReasonInvalidRange   = "invalid_range"
	CorrectionReasonSourceConflict = "source_conflict"

	SleepSyncKindObservation = "observation"
	SleepSyncKindCorrection  = "correction"
)

var contractIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)

// SleepObservationRecord is the local desktop record shape for
// observation-set.schema.json#/$defs/observation when kind=sleep_episode.
type SleepObservationRecord struct {
	ObservationID string                     `json:"observation_id"`
	Kind          string                     `json:"kind"`
	StartAt       time.Time                  `json:"start_at"`
	EndAt         time.Time                  `json:"end_at"`
	ZoneID        string                     `json:"zone_id"`
	Sleep         SleepObservationDetails    `json:"sleep"`
	Provenance    SleepObservationProvenance `json:"provenance"`
}

type SleepObservationDetails struct {
	Classification string `json:"classification"`
}

type SleepObservationProvenance struct {
	AcquisitionMethod string    `json:"acquisition_method"`
	EvidenceStatus    string    `json:"evidence_status"`
	RecordedAt        time.Time `json:"recorded_at"`
	SourceRecordID    string    `json:"source_record_id,omitempty"`
}

// SleepCorrectionRecord is the local desktop record shape for
// correction-set.schema.json#/$defs/correction.
type SleepCorrectionRecord struct {
	CorrectionID           string                 `json:"correction_id"`
	TargetObservationID    string                 `json:"target_observation_id"`
	SupersedesCorrectionID string                 `json:"supersedes_correction_id,omitempty"`
	CreatedAt              time.Time              `json:"created_at"`
	Reason                 string                 `json:"reason"`
	Changes                SleepCorrectionChanges `json:"changes"`
}

type SleepCorrectionChanges struct {
	StartAt             *time.Time `json:"start_at,omitempty"`
	EndAt               *time.Time `json:"end_at,omitempty"`
	SleepClassification *string    `json:"sleep_classification,omitempty"`
	Excluded            *bool      `json:"excluded,omitempty"`
}

type SleepObservationSet struct {
	SchemaVersion string                   `json:"schema_version"`
	GeneratedAt   time.Time                `json:"generated_at"`
	Observations  []SleepObservationRecord `json:"observations"`
}

type SleepCorrectionSet struct {
	SchemaVersion string                  `json:"schema_version"`
	GeneratedAt   time.Time               `json:"generated_at"`
	Corrections   []SleepCorrectionRecord `json:"corrections"`
}

type SleepDataExport struct {
	SchemaVersion  string              `json:"schema_version"`
	GeneratedAt    time.Time           `json:"generated_at"`
	ObservationSet SleepObservationSet `json:"observation_set"`
	CorrectionSet  SleepCorrectionSet  `json:"correction_set"`
}

type SleepSyncRecord struct {
	RecordID    string
	Kind        string
	CreatedAt   time.Time
	Payload     json.RawMessage
	PayloadHash string
}

// SleepPlanningFingerprint identifies the immutable sleep observations and
// corrections used by local estimation without exposing their contents.
func (s *Store) SleepPlanningFingerprint(ctx context.Context) (string, error) {
	return sleepPlanningFingerprint(ctx, s.db)
}

func sleepPlanningFingerprint(ctx context.Context, query queryContext) (string, error) {
	hash := sha256.New()
	sets := []struct {
		marker string
		query  string
	}{
		{
			marker: "observations",
			query:  `SELECT observation_id, payload_json FROM local_sleep_observations ORDER BY start_at, observation_id`,
		},
		{
			marker: "corrections",
			query:  `SELECT correction_id, payload_json FROM local_sleep_corrections ORDER BY created_at, correction_id`,
		},
	}
	for _, set := range sets {
		hash.Write([]byte(set.marker))
		hash.Write([]byte{0})
		rows, err := query.QueryContext(ctx, set.query)
		if err != nil {
			return "", err
		}
		for rows.Next() {
			var recordID string
			var payload []byte
			if err := rows.Scan(&recordID, &payload); err != nil {
				_ = rows.Close()
				return "", err
			}
			hash.Write([]byte(recordID))
			hash.Write([]byte{0})
			hash.Write(payload)
			hash.Write([]byte{0})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return "", err
		}
		if err := rows.Close(); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Store) AppendSleepObservation(ctx context.Context, record SleepObservationRecord) error {
	if err := validateSleepObservation(record); err != nil {
		return err
	}
	record.StartAt = record.StartAt.UTC()
	record.EndAt = record.EndAt.UTC()
	record.Provenance.RecordedAt = record.Provenance.RecordedAt.UTC()
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO local_sleep_observations(
		observation_id, kind, start_at, end_at, zone_id, classification,
		acquisition_method, evidence_status, recorded_at, source_record_id, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ObservationID, record.Kind, formatSQLiteTime(record.StartAt), formatSQLiteTime(record.EndAt),
		record.ZoneID, record.Sleep.Classification, record.Provenance.AcquisitionMethod,
		record.Provenance.EvidenceStatus, formatSQLiteTime(record.Provenance.RecordedAt),
		record.Provenance.SourceRecordID, encoded,
	)
	return err
}

func (s *Store) ListSleepObservations(ctx context.Context) ([]SleepObservationRecord, error) {
	var records []SleepObservationRecord
	err := s.readJSONRows(ctx, `SELECT payload_json FROM local_sleep_observations ORDER BY start_at, observation_id`, func(value []byte) error {
		var record SleepObservationRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	return records, err
}

func (s *Store) AppendSleepCorrection(ctx context.Context, record SleepCorrectionRecord) error {
	if err := validateSleepCorrection(record); err != nil {
		return err
	}
	if err := s.requireSleepObservation(ctx, record.TargetObservationID); err != nil {
		return err
	}
	record.CreatedAt = record.CreatedAt.UTC()
	if record.Changes.StartAt != nil {
		start := record.Changes.StartAt.UTC()
		record.Changes.StartAt = &start
	}
	if record.Changes.EndAt != nil {
		end := record.Changes.EndAt.UTC()
		record.Changes.EndAt = &end
	}
	changes, err := json.Marshal(record.Changes)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO local_sleep_corrections(
		correction_id, target_observation_id, supersedes_correction_id, created_at, reason, changes_json, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		record.CorrectionID, record.TargetObservationID, record.SupersedesCorrectionID,
		formatSQLiteTime(record.CreatedAt), record.Reason, changes, encoded,
	)
	return err
}

func (s *Store) ListSleepCorrections(ctx context.Context) ([]SleepCorrectionRecord, error) {
	var records []SleepCorrectionRecord
	err := s.readJSONRows(ctx, `SELECT payload_json FROM local_sleep_corrections ORDER BY created_at, correction_id`, func(value []byte) error {
		var record SleepCorrectionRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	return records, err
}

func (s *Store) LocalSleepSyncRecords(ctx context.Context) ([]SleepSyncRecord, error) {
	observations, err := s.ListSleepObservations(ctx)
	if err != nil {
		return nil, err
	}
	corrections, err := s.ListSleepCorrections(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]SleepSyncRecord, 0, len(observations)+len(corrections))
	for _, observation := range observations {
		payload, err := json.Marshal(observation)
		if err != nil {
			return nil, err
		}
		records = append(records, newSleepSyncRecord(observation.ObservationID, SleepSyncKindObservation, observation.Provenance.RecordedAt, payload))
	}
	for _, correction := range corrections {
		payload, err := json.Marshal(correction)
		if err != nil {
			return nil, err
		}
		records = append(records, newSleepSyncRecord(correction.CorrectionID, SleepSyncKindCorrection, correction.CreatedAt, payload))
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].RecordID < records[j].RecordID
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records, nil
}

func (s *Store) UnpushedSleepSyncRecords(ctx context.Context) ([]SleepSyncRecord, error) {
	records, err := s.LocalSleepSyncRecords(ctx)
	if err != nil {
		return nil, err
	}
	pushed := map[string]string{}
	rows, err := s.db.QueryContext(ctx, `SELECT record_id, payload_hash FROM local_sleep_sync_records`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, hash string
		if err := rows.Scan(&id, &hash); err != nil {
			return nil, err
		}
		pushed[id] = hash
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	unpushed := make([]SleepSyncRecord, 0, len(records))
	for _, record := range records {
		if pushed[record.RecordID] == record.PayloadHash {
			continue
		}
		unpushed = append(unpushed, record)
	}
	return unpushed, nil
}

func (s *Store) MarkSleepSyncRecordsPushed(ctx context.Context, records []SleepSyncRecord, pushedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, record := range records {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO local_sleep_sync_records(record_id, kind, payload_hash, pushed_at) VALUES(?, ?, ?, ?)`,
			record.RecordID, record.Kind, record.PayloadHash, formatSQLiteTime(pushedAt)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SleepSyncCursor(ctx context.Context) (int64, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM local_sync_state WHERE key = 'sleep_sync_cursor'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var cursor int64
	if _, err := fmt.Sscanf(raw, "%d", &cursor); err != nil {
		return 0, err
	}
	return cursor, nil
}

func (s *Store) SaveSleepSyncCursor(ctx context.Context, cursor int64) error {
	if cursor < 0 {
		return errors.New("sleep sync cursor must not be negative")
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO local_sync_state(key, value) VALUES('sleep_sync_cursor', ?)`, fmt.Sprintf("%d", cursor))
	return err
}

func (s *Store) InsertSyncedSleepObservation(ctx context.Context, record SleepObservationRecord) (bool, error) {
	if err := validateSleepObservation(record); err != nil {
		return false, err
	}
	record.StartAt = record.StartAt.UTC()
	record.EndAt = record.EndAt.UTC()
	record.Provenance.RecordedAt = record.Provenance.RecordedAt.UTC()
	encoded, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO local_sleep_observations(
		observation_id, kind, start_at, end_at, zone_id, classification,
		acquisition_method, evidence_status, recorded_at, source_record_id, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ObservationID, record.Kind, formatSQLiteTime(record.StartAt), formatSQLiteTime(record.EndAt),
		record.ZoneID, record.Sleep.Classification, record.Provenance.AcquisitionMethod,
		record.Provenance.EvidenceStatus, formatSQLiteTime(record.Provenance.RecordedAt),
		record.Provenance.SourceRecordID, encoded,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	synced := newSleepSyncRecord(record.ObservationID, SleepSyncKindObservation, record.Provenance.RecordedAt, encoded)
	if err := s.MarkSleepSyncRecordsPushed(ctx, []SleepSyncRecord{synced}, time.Now().UTC()); err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Store) InsertSyncedSleepCorrection(ctx context.Context, record SleepCorrectionRecord) (bool, error) {
	if err := validateSleepCorrection(record); err != nil {
		return false, err
	}
	if err := s.requireSleepObservation(ctx, record.TargetObservationID); err != nil {
		return false, err
	}
	record.CreatedAt = record.CreatedAt.UTC()
	if record.Changes.StartAt != nil {
		start := record.Changes.StartAt.UTC()
		record.Changes.StartAt = &start
	}
	if record.Changes.EndAt != nil {
		end := record.Changes.EndAt.UTC()
		record.Changes.EndAt = &end
	}
	changes, err := json.Marshal(record.Changes)
	if err != nil {
		return false, err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO local_sleep_corrections(
		correction_id, target_observation_id, supersedes_correction_id, created_at, reason, changes_json, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		record.CorrectionID, record.TargetObservationID, record.SupersedesCorrectionID,
		formatSQLiteTime(record.CreatedAt), record.Reason, changes, encoded,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	synced := newSleepSyncRecord(record.CorrectionID, SleepSyncKindCorrection, record.CreatedAt, encoded)
	if err := s.MarkSleepSyncRecordsPushed(ctx, []SleepSyncRecord{synced}, time.Now().UTC()); err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Store) ExportSleepData(ctx context.Context) (SleepDataExport, error) {
	generatedAt := time.Now().UTC()
	result := SleepDataExport{
		SchemaVersion: "v1",
		GeneratedAt:   generatedAt,
		ObservationSet: SleepObservationSet{
			SchemaVersion: "v1",
			GeneratedAt:   generatedAt,
			Observations:  []SleepObservationRecord{},
		},
		CorrectionSet: SleepCorrectionSet{
			SchemaVersion: "v1",
			GeneratedAt:   generatedAt,
			Corrections:   []SleepCorrectionRecord{},
		},
	}
	observations, err := s.ListSleepObservations(ctx)
	if err != nil {
		return result, err
	}
	corrections, err := s.ListSleepCorrections(ctx)
	if err != nil {
		return result, err
	}
	if observations == nil {
		observations = []SleepObservationRecord{}
	}
	if corrections == nil {
		corrections = []SleepCorrectionRecord{}
	}
	result.ObservationSet.Observations = observations
	result.CorrectionSet.Corrections = corrections
	return result, nil
}

func (s *Store) DeleteSleepObservation(ctx context.Context, observationID string) error {
	if !contractIdentifier.MatchString(observationID) {
		return errors.New("observation_id must match the v1 identifier format")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Records that already reached the synced backend need server-side erasure
	// too (ADR-0017): enqueue them in the erasure outbox before their tracking
	// rows disappear. Never-pushed records never left this device, so they
	// need no tombstone.
	erasedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_sleep_erasures(record_id, erased_at)
		SELECT record_id, ? FROM local_sleep_sync_records
		WHERE pushed_at != ''
			AND (record_id = ?
				OR record_id IN (
					SELECT correction_id FROM local_sleep_corrections WHERE target_observation_id = ?
				))`, erasedAt, observationID, observationID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_sleep_sync_records
		WHERE record_id = ?
			OR record_id IN (
				SELECT correction_id FROM local_sleep_corrections WHERE target_observation_id = ?
			)`, observationID, observationID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_sleep_corrections WHERE target_observation_id = ?`, observationID); err != nil {
		_ = tx.Rollback()
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM local_sleep_observations WHERE observation_id = ?`, observationID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if deleted == 0 {
		_ = tx.Rollback()
		return fmt.Errorf("sleep observation %s does not exist", observationID)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.compactDeletedData(ctx)
}

func (s *Store) DeleteAllSleepData(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	erasedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_sleep_erasures(record_id, erased_at)
		SELECT record_id, ? FROM local_sleep_sync_records WHERE pushed_at != ''`, erasedAt); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, table := range []string{"local_sleep_sync_records", "local_sleep_corrections", "local_sleep_observations"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.compactDeletedData(ctx)
}

// PendingSyncErasures lists record ids (any kind: sleep records or task
// revisions) that were hard-deleted locally after having been pushed, and
// still await server-side erasure (ADR-0017). The backing table keeps its
// historical name local_sleep_erasures; its rows are opaque record ids.
func (s *Store) PendingSyncErasures(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT record_id FROM local_sleep_erasures ORDER BY record_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ClearSyncErasures removes outbox entries once the backend confirmed their
// tombstones.
func (s *Store) ClearSyncErasures(ctx context.Context, recordIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, id := range recordIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM local_sleep_erasures WHERE record_id = ?`, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// EraseSyncedSleepRecord applies a pulled tombstone: it hard-deletes the local
// observation (with its corrections) or correction matching the record id,
// plus any tracking rows — without enqueueing a new erasure (the tombstone
// came FROM the server). Idempotent: absent records return false, nil.
func (s *Store) EraseSyncedSleepRecord(ctx context.Context, recordID string) (bool, error) {
	if !contractIdentifier.MatchString(recordID) {
		return false, errors.New("record_id must match the v1 identifier format")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM local_sleep_sync_records
		WHERE record_id = ?
			OR record_id IN (
				SELECT correction_id FROM local_sleep_corrections WHERE target_observation_id = ?
			)`, recordID, recordID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_sleep_erasures WHERE record_id = ?`, recordID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_sleep_corrections WHERE target_observation_id = ?`, recordID); err != nil {
		return false, err
	}
	observations, err := tx.ExecContext(ctx, `DELETE FROM local_sleep_observations WHERE observation_id = ?`, recordID)
	if err != nil {
		return false, err
	}
	deletedObservations, err := observations.RowsAffected()
	if err != nil {
		return false, err
	}
	corrections, err := tx.ExecContext(ctx, `DELETE FROM local_sleep_corrections WHERE correction_id = ?`, recordID)
	if err != nil {
		return false, err
	}
	deletedCorrections, err := corrections.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	committed = true
	if deletedObservations+deletedCorrections == 0 {
		return false, nil
	}
	return true, s.compactDeletedData(ctx)
}

func (s *Store) LatestSleepCorrectionID(ctx context.Context, targetObservationID string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT correction_id
		FROM local_sleep_corrections
		WHERE target_observation_id = ?
		ORDER BY created_at DESC, correction_id DESC
		LIMIT 1`, targetObservationID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *Store) RawSleepSessions(ctx context.Context) ([]domain.SleepSession, error) {
	observations, err := s.ListSleepObservations(ctx)
	if err != nil {
		return nil, err
	}
	sessions := make([]domain.SleepSession, 0, len(observations))
	for _, observation := range observations {
		session, err := sleepSessionFromObservation(observation)
		if err != nil {
			return nil, fmt.Errorf("observation %s: %w", observation.ObservationID, err)
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// CorrectedSleepSessions applies active append-only corrections but does not
// merge overlapping reports. It is useful for entry-list UI where every source
// observation still needs a row.
func (s *Store) CorrectedSleepSessions(ctx context.Context) ([]domain.SleepSession, error) {
	raw, err := s.RawSleepSessions(ctx)
	if err != nil {
		return nil, err
	}
	corrections, err := s.activeDomainSleepCorrections(ctx)
	if err != nil {
		return nil, err
	}
	return domain.ApplySleepCorrections(raw, corrections)
}

func (s *Store) EffectiveSleepSessions(ctx context.Context) ([]domain.SleepSession, error) {
	corrected, err := s.CorrectedSleepSessions(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := ingest.ResolveOverlappingSleepReports(corrected)
	if err != nil {
		return nil, err
	}
	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].Intervals[0].Interval.Start.UTC.Before(resolved[j].Intervals[0].Interval.Start.UTC)
	})
	return resolved, nil
}

func (s *Store) activeDomainSleepCorrections(ctx context.Context) ([]domain.ManualCorrection, error) {
	observations, err := s.ListSleepObservations(ctx)
	if err != nil {
		return nil, err
	}
	zoneByObservation := make(map[string]string, len(observations))
	for _, observation := range observations {
		zoneByObservation[observation.ObservationID] = observation.ZoneID
	}
	records, err := s.ListSleepCorrections(ctx)
	if err != nil {
		return nil, err
	}
	var corrections []domain.ManualCorrection
	for _, record := range records {
		items, err := domainCorrectionsFromSleepRecord(record, zoneByObservation[record.TargetObservationID])
		if err != nil {
			return nil, fmt.Errorf("correction %s: %w", record.CorrectionID, err)
		}
		corrections = append(corrections, items...)
	}
	return markActiveCorrections(corrections), nil
}

func validateSleepObservation(record SleepObservationRecord) error {
	if !contractIdentifier.MatchString(record.ObservationID) {
		return errors.New("observation_id must match the v1 identifier format")
	}
	if record.Kind != SleepKindEpisode {
		return errors.New("sleep observation kind must be sleep_episode")
	}
	if !validSleepClassification(record.Sleep.Classification) {
		return errors.New("sleep.classification must be principal, nap, or unknown")
	}
	if !validAcquisition(record.Provenance.AcquisitionMethod) {
		return errors.New("provenance.acquisition_method is not supported")
	}
	if !validEvidenceStatus(record.Provenance.EvidenceStatus) {
		return errors.New("provenance.evidence_status is not supported")
	}
	if record.Provenance.RecordedAt.IsZero() {
		return errors.New("provenance.recorded_at is required")
	}
	start, err := domain.NewZonedInstant(record.StartAt, record.ZoneID)
	if err != nil {
		return err
	}
	end, err := domain.NewZonedInstant(record.EndAt, record.ZoneID)
	if err != nil {
		return err
	}
	return (domain.TimeRange{Start: start, End: end}).Validate()
}

func validateSleepCorrection(record SleepCorrectionRecord) error {
	if !contractIdentifier.MatchString(record.CorrectionID) {
		return errors.New("correction_id must match the v1 identifier format")
	}
	if !contractIdentifier.MatchString(record.TargetObservationID) {
		return errors.New("target_observation_id must match the v1 identifier format")
	}
	if record.SupersedesCorrectionID != "" && !contractIdentifier.MatchString(record.SupersedesCorrectionID) {
		return errors.New("supersedes_correction_id must match the v1 identifier format")
	}
	if record.CreatedAt.IsZero() {
		return errors.New("created_at is required")
	}
	if !validCorrectionReason(record.Reason) {
		return errors.New("correction reason is not supported")
	}
	changes := record.Changes
	count := 0
	if changes.StartAt != nil {
		count++
	}
	if changes.EndAt != nil {
		count++
	}
	if changes.SleepClassification != nil {
		if !validSleepClassification(*changes.SleepClassification) {
			return errors.New("changes.sleep_classification must be principal, nap, or unknown")
		}
		count++
	}
	if changes.Excluded != nil {
		count++
	}
	if count == 0 {
		return errors.New("correction changes must not be empty")
	}
	if changes.StartAt != nil && changes.EndAt != nil && !changes.EndAt.After(*changes.StartAt) {
		return errors.New("changes.end_at must be after changes.start_at")
	}
	return nil
}

// ErrSleepObservationMissing marks operations whose target observation is not
// in the local store — e.g. a synced correction whose observation was erased
// locally. Callers may treat it as a permanent, skippable orphan.
var ErrSleepObservationMissing = errors.New("sleep observation does not exist")

func (s *Store) requireSleepObservation(ctx context.Context, observationID string) error {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM local_sleep_observations WHERE observation_id = ?`, observationID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrSleepObservationMissing, observationID)
	}
	return err
}

func sleepSessionFromObservation(obs SleepObservationRecord) (domain.SleepSession, error) {
	start, err := domain.NewZonedInstant(obs.StartAt, obs.ZoneID)
	if err != nil {
		return domain.SleepSession{}, err
	}
	end, err := domain.NewZonedInstant(obs.EndAt, obs.ZoneID)
	if err != nil {
		return domain.SleepSession{}, err
	}
	interval := domain.TimeRange{Start: start, End: end}
	if err := interval.Validate(); err != nil {
		return domain.SleepSession{}, err
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
		ID:          domain.SleepSessionID(obs.ObservationID),
		IsNap:       obs.Sleep.Classification == SleepClassificationNap,
		SourceLabel: sourceLabel(obs.Provenance.AcquisitionMethod),
		CreatedAt:   obs.Provenance.RecordedAt.UTC(),
		Intervals: []domain.SleepInterval{{
			Interval:      interval,
			StartEvidence: evidence,
			EndEvidence:   evidence,
		}},
	}, nil
}

func domainCorrectionsFromSleepRecord(record SleepCorrectionRecord, zoneID string) ([]domain.ManualCorrection, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("target observation %s has no zone", record.TargetObservationID)
	}
	base := domain.ManualCorrection{
		ID:           domain.CorrectionID(record.CorrectionID),
		TargetID:     domain.SleepSessionID(record.TargetObservationID),
		CreatedAt:    record.CreatedAt.UTC(),
		SupersedesID: correctionIDPtr(record.SupersedesCorrectionID),
		Active:       true,
	}
	var result []domain.ManualCorrection
	if record.Changes.StartAt != nil {
		item := base
		item.Kind = domain.CorrectionSetSleepStart
		instant, err := domain.NewZonedInstant(*record.Changes.StartAt, zoneID)
		if err != nil {
			return nil, err
		}
		item.InstantValue = &instant
		result = append(result, item)
	}
	if record.Changes.EndAt != nil {
		item := base
		item.ID = suffixedCorrectionID(base.ID, "end")
		item.Kind = domain.CorrectionSetWakeTime
		instant, err := domain.NewZonedInstant(*record.Changes.EndAt, zoneID)
		if err != nil {
			return nil, err
		}
		item.InstantValue = &instant
		result = append(result, item)
	}
	if record.Changes.SleepClassification != nil {
		item := base
		item.ID = suffixedCorrectionID(base.ID, "class")
		item.Kind = domain.CorrectionClassifyNap
		value := *record.Changes.SleepClassification == SleepClassificationNap
		item.BoolValue = &value
		result = append(result, item)
	}
	if record.Changes.Excluded != nil {
		item := base
		item.ID = suffixedCorrectionID(base.ID, "excluded")
		item.Kind = domain.CorrectionSuppress
		value := *record.Changes.Excluded
		item.BoolValue = &value
		result = append(result, item)
	}
	return result, nil
}

func markActiveCorrections(corrections []domain.ManualCorrection) []domain.ManualCorrection {
	superseded := map[domain.CorrectionID]struct{}{}
	for _, correction := range corrections {
		if correction.SupersedesID == nil {
			continue
		}
		superseded[*correction.SupersedesID] = struct{}{}
		superseded[suffixedCorrectionID(*correction.SupersedesID, "end")] = struct{}{}
		superseded[suffixedCorrectionID(*correction.SupersedesID, "class")] = struct{}{}
		superseded[suffixedCorrectionID(*correction.SupersedesID, "excluded")] = struct{}{}
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
	case ProvenanceAcquisitionHealthConnect, ProvenanceAcquisitionFileImport:
		return domain.AcquisitionImported
	case ProvenanceAcquisitionOSActivity:
		return domain.AcquisitionCollected
	default:
		return domain.AcquisitionManual
	}
}

func evidenceStatus(value string) domain.EvidenceStatus {
	switch value {
	case ProvenanceEvidenceDirectlyObserved:
		return domain.StatusObserved
	case ProvenanceEvidenceInferred:
		return domain.StatusInferred
	default:
		return domain.StatusUserConfirmed
	}
}

func sourceLabel(value string) string {
	switch value {
	case ProvenanceAcquisitionHealthConnect:
		return "Health Connect sleep"
	case ProvenanceAcquisitionFileImport:
		return "Imported sleep"
	case ProvenanceAcquisitionOSActivity:
		return "Device activity"
	case ProvenanceAcquisitionSynthetic:
		return "Synthetic sleep"
	default:
		return "Manual sleep log"
	}
}

func validSleepClassification(value string) bool {
	return value == SleepClassificationPrincipal || value == SleepClassificationNap || value == SleepClassificationUnknown
}

func validAcquisition(value string) bool {
	switch value {
	case ProvenanceAcquisitionManual, ProvenanceAcquisitionHealthConnect, ProvenanceAcquisitionOSActivity, ProvenanceAcquisitionFileImport, ProvenanceAcquisitionSynthetic:
		return true
	default:
		return false
	}
}

func validEvidenceStatus(value string) bool {
	switch value {
	case ProvenanceEvidenceDirectlyObserved, ProvenanceEvidenceUserReported, ProvenanceEvidenceInferred:
		return true
	default:
		return false
	}
}

func validCorrectionReason(value string) bool {
	switch value {
	case CorrectionReasonUserEdit, CorrectionReasonDuplicate, CorrectionReasonInvalidRange, CorrectionReasonSourceConflict:
		return true
	default:
		return false
	}
}

func newSleepSyncRecord(id, kind string, createdAt time.Time, payload []byte) SleepSyncRecord {
	sum := sha256.Sum256(payload)
	return SleepSyncRecord{
		RecordID:    id,
		Kind:        kind,
		CreatedAt:   createdAt.UTC(),
		Payload:     append(json.RawMessage(nil), payload...),
		PayloadHash: hex.EncodeToString(sum[:]),
	}
}

func formatSQLiteTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
