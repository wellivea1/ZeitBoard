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
	"non24.app/core/sleepv1"
)

const (
	SleepKindEpisode = "sleep_episode"

	SleepClassificationPrincipal = sleepv1.ClassificationPrincipal
	SleepClassificationNap       = sleepv1.ClassificationNap
	SleepClassificationUnknown   = sleepv1.ClassificationUnknown

	ProvenanceAcquisitionManual        = sleepv1.AcquisitionManual
	ProvenanceAcquisitionHealthConnect = sleepv1.AcquisitionHealthConnect
	ProvenanceAcquisitionOSActivity    = sleepv1.AcquisitionOSActivity
	ProvenanceAcquisitionFileImport    = sleepv1.AcquisitionFileImport
	ProvenanceAcquisitionSynthetic     = sleepv1.AcquisitionSynthetic

	ProvenanceEvidenceDirectlyObserved = sleepv1.EvidenceDirectlyObserved
	ProvenanceEvidenceUserReported     = sleepv1.EvidenceUserReported
	ProvenanceEvidenceInferred         = sleepv1.EvidenceInferred

	CorrectionReasonUserEdit       = sleepv1.CorrectionUserEdit
	CorrectionReasonDuplicate      = sleepv1.CorrectionDuplicate
	CorrectionReasonInvalidRange   = sleepv1.CorrectionInvalidRange
	CorrectionReasonSourceConflict = sleepv1.CorrectionSourceConflict

	SleepSyncKindObservation = "observation"
	SleepSyncKindCorrection  = "correction"
)

var contractIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)

type SleepObservationRecord = sleepv1.Observation
type SleepObservationDetails = sleepv1.Details
type SleepObservationProvenance = sleepv1.Provenance
type SleepCorrectionRecord = sleepv1.Correction
type SleepCorrectionChanges = sleepv1.CorrectionChanges

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

// SleepSnapshot is a transactionally consistent local sleep read model. All
// projections are derived from the same immutable observation and correction
// rows.
type SleepSnapshot struct {
	Observations      []SleepObservationRecord
	Corrections       []SleepCorrectionRecord
	RawSessions       []domain.SleepSession
	CorrectedSessions []domain.SleepSession
	EffectiveSessions []domain.SleepSession
}

type SleepObservationSnapshot struct {
	Observation      SleepObservationRecord
	Corrections      []SleepCorrectionRecord
	RawSession       domain.SleepSession
	CorrectedSession domain.SleepSession
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
	return listSleepObservations(ctx, s.db)
}

func listSleepObservations(ctx context.Context, query queryContext) ([]SleepObservationRecord, error) {
	var records []SleepObservationRecord
	err := readJSONRows(ctx, query, `SELECT payload_json FROM local_sleep_observations ORDER BY start_at, observation_id`, func(value []byte) error {
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
	return listSleepCorrections(ctx, s.db)
}

func listSleepCorrections(ctx context.Context, query queryContext) ([]SleepCorrectionRecord, error) {
	var records []SleepCorrectionRecord
	err := readJSONRows(ctx, query, `SELECT payload_json FROM local_sleep_corrections ORDER BY created_at, correction_id`, func(value []byte) error {
		var record SleepCorrectionRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	return records, err
}

func (s *Store) ReadSleepSnapshot(ctx context.Context) (SleepSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SleepSnapshot{}, err
	}
	defer tx.Rollback()

	snapshot, err := readSleepSnapshot(ctx, tx)
	if err != nil {
		return SleepSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return SleepSnapshot{}, err
	}
	return snapshot, nil
}

func readSleepSnapshot(ctx context.Context, query queryContext) (SleepSnapshot, error) {
	observations, err := listSleepObservations(ctx, query)
	if err != nil {
		return SleepSnapshot{}, err
	}
	corrections, err := listSleepCorrections(ctx, query)
	if err != nil {
		return SleepSnapshot{}, err
	}
	raw, err := sleepSessionsFromObservations(observations)
	if err != nil {
		return SleepSnapshot{}, err
	}
	corrected, err := sleepv1.Fold(observations, corrections)
	if err != nil {
		return SleepSnapshot{}, err
	}
	effective, err := sleepv1.ResolveOverlaps(corrected)
	if err != nil {
		return SleepSnapshot{}, err
	}
	return SleepSnapshot{
		Observations:      observations,
		Corrections:       corrections,
		RawSessions:       raw,
		CorrectedSessions: corrected,
		EffectiveSessions: effective,
	}, nil
}

func (s *Store) ReadSleepObservationSnapshot(ctx context.Context, observationID string) (SleepObservationSnapshot, error) {
	if !contractIdentifier.MatchString(observationID) {
		return SleepObservationSnapshot{}, errors.New("observation_id must match the v1 identifier format")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SleepObservationSnapshot{}, err
	}
	defer tx.Rollback()

	observation, found, err := sleepObservationByID(ctx, tx, observationID)
	if err != nil {
		return SleepObservationSnapshot{}, err
	}
	if !found {
		return SleepObservationSnapshot{}, fmt.Errorf("%w: %s", ErrSleepObservationMissing, observationID)
	}
	corrections, err := sleepCorrectionsForObservation(ctx, tx, observationID)
	if err != nil {
		return SleepObservationSnapshot{}, err
	}
	raw, err := sleepv1.SessionFromObservation(observation)
	if err != nil {
		return SleepObservationSnapshot{}, fmt.Errorf("observation %s: %w", observationID, err)
	}
	corrected, err := sleepv1.Fold([]SleepObservationRecord{observation}, corrections)
	if err != nil {
		return SleepObservationSnapshot{}, err
	}
	if len(corrected) != 1 {
		return SleepObservationSnapshot{}, fmt.Errorf("sleep observation %s produced %d corrected sessions", observationID, len(corrected))
	}
	if err := tx.Commit(); err != nil {
		return SleepObservationSnapshot{}, err
	}
	return SleepObservationSnapshot{
		Observation:      observation,
		Corrections:      corrections,
		RawSession:       raw,
		CorrectedSession: corrected[0],
	}, nil
}

func sleepObservationByID(ctx context.Context, query queryContext, observationID string) (SleepObservationRecord, bool, error) {
	var records []SleepObservationRecord
	err := readJSONRows(ctx, query, `SELECT payload_json
		FROM local_sleep_observations
		WHERE observation_id = ?`, func(value []byte) error {
		var record SleepObservationRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	}, observationID)
	if err != nil {
		return SleepObservationRecord{}, false, err
	}
	if len(records) == 0 {
		return SleepObservationRecord{}, false, nil
	}
	if len(records) != 1 {
		return SleepObservationRecord{}, false, fmt.Errorf("observation_id %s matched %d rows", observationID, len(records))
	}
	return records[0], true, nil
}

func sleepCorrectionsForObservation(ctx context.Context, query queryContext, observationID string) ([]SleepCorrectionRecord, error) {
	var records []SleepCorrectionRecord
	err := readJSONRows(ctx, query, `SELECT payload_json
		FROM local_sleep_corrections
		WHERE target_observation_id = ?
		ORDER BY created_at, correction_id`, func(value []byte) error {
		var record SleepCorrectionRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	}, observationID)
	return records, err
}

func sleepSessionsFromObservations(observations []SleepObservationRecord) ([]domain.SleepSession, error) {
	sessions := make([]domain.SleepSession, 0, len(observations))
	for _, observation := range observations {
		session, err := sleepv1.SessionFromObservation(observation)
		if err != nil {
			return nil, fmt.Errorf("observation %s: %w", observation.ObservationID, err)
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
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

const MaxSleepSyncPageSize = 100

const pendingSleepSyncRecordsFrom = `FROM (
	SELECT observation.observation_id AS record_id,
		'observation' AS kind,
		observation.recorded_at AS created_at,
		observation.payload_json AS payload_json
	FROM local_sleep_observations AS observation
	LEFT JOIN local_sleep_sync_records AS synced
		ON synced.record_id = observation.observation_id
	WHERE synced.record_id IS NULL
	UNION ALL
	SELECT correction.correction_id AS record_id,
		'correction' AS kind,
		correction.created_at AS created_at,
		correction.payload_json AS payload_json
	FROM local_sleep_corrections AS correction
	LEFT JOIN local_sleep_sync_records AS synced
		ON synced.record_id = correction.correction_id
	WHERE synced.record_id IS NULL
) AS pending`

// PendingSleepSyncRecords returns one server-sized page without decoding or
// re-encoding immutable payloads from unrelated history.
func (s *Store) PendingSleepSyncRecords(ctx context.Context, limit int) ([]SleepSyncRecord, error) {
	if limit < 1 || limit > MaxSleepSyncPageSize {
		return nil, fmt.Errorf("sleep sync page limit must be between 1 and %d", MaxSleepSyncPageSize)
	}
	return s.pendingSleepSyncRecords(ctx, limit)
}

// UnpushedSleepSyncRecords remains available for diagnostics and tests. The
// production sync path uses PendingSleepSyncRecords to keep memory bounded.
func (s *Store) UnpushedSleepSyncRecords(ctx context.Context) ([]SleepSyncRecord, error) {
	return s.pendingSleepSyncRecords(ctx, 0)
}

func (s *Store) pendingSleepSyncRecords(ctx context.Context, limit int) ([]SleepSyncRecord, error) {
	query := `SELECT record_id, kind, created_at, payload_json ` + pendingSleepSyncRecordsFrom + `
		ORDER BY created_at, record_id`
	var args []any
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]SleepSyncRecord, 0, max(limit, 0))
	for rows.Next() {
		var recordID, kind, createdAt string
		var payload []byte
		if err := rows.Scan(&recordID, &kind, &createdAt, &payload); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("pending sleep sync record %s created_at: %w", recordID, err)
		}
		records = append(records, newSleepSyncRecord(recordID, kind, parsed, payload))
	}
	return records, rows.Err()
}

func (s *Store) PendingSleepSyncRecordCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+pendingSleepSyncRecordsFrom).Scan(&count)
	return count, err
}

func (s *Store) MarkSleepSyncRecordsPushed(ctx context.Context, records []SleepSyncRecord, pushedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := markSleepSyncRecordsPushedTx(ctx, tx, records, pushedAt); err != nil {
		return err
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
	prepared, err := prepareSyncedSleepObservation(record)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	inserted, err := insertSyncedSleepObservationTx(ctx, tx, prepared, time.Now().UTC())
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return inserted, nil
}

func (s *Store) InsertSyncedSleepCorrection(ctx context.Context, record SleepCorrectionRecord) (bool, error) {
	prepared, err := prepareSyncedSleepCorrection(record)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	inserted, err := insertSyncedSleepCorrectionTx(ctx, tx, prepared, time.Now().UTC())
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return inserted, nil
}

func (s *Store) ExportSleepData(ctx context.Context) (SleepDataExport, error) {
	generatedAt := time.Now().UTC()
	result := newSleepDataExport(generatedAt)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	result, err = readSleepDataExport(ctx, tx, generatedAt)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func readSleepDataExport(ctx context.Context, query queryContext, generatedAt time.Time) (SleepDataExport, error) {
	result := newSleepDataExport(generatedAt)
	observations, err := listSleepObservations(ctx, query)
	if err != nil {
		return result, err
	}
	corrections, err := listSleepCorrections(ctx, query)
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

func newSleepDataExport(generatedAt time.Time) SleepDataExport {
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
	return result
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
	for _, table := range []string{
		"local_sleep_sync_records",
		"local_sleep_corrections",
		"local_sleep_observations",
		// Including the unfinished one: otherwise erasing everything leaves a
		// marked onset behind that would write a fresh row on the next tap.
		"local_sleep_pending",
	} {
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
	defer tx.Rollback()
	applied, err := eraseSyncedSleepRecordTx(ctx, tx, recordID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	if !applied {
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
	return sleepSessionsFromObservations(observations)
}

// CorrectedSleepSessions applies active append-only corrections but does not
// merge overlapping reports. It is useful for entry-list UI where every source
// observation still needs a row.
func (s *Store) CorrectedSleepSessions(ctx context.Context) ([]domain.SleepSession, error) {
	observations, err := s.ListSleepObservations(ctx)
	if err != nil {
		return nil, err
	}
	corrections, err := s.ListSleepCorrections(ctx)
	if err != nil {
		return nil, err
	}
	return sleepv1.Fold(observations, corrections)
}

func (s *Store) EffectiveSleepSessions(ctx context.Context) ([]domain.SleepSession, error) {
	corrected, err := s.CorrectedSleepSessions(ctx)
	if err != nil {
		return nil, err
	}
	return sleepv1.ResolveOverlaps(corrected)
}

func validateSleepObservation(record SleepObservationRecord) error {
	return sleepv1.ValidateObservation(record)
}

func validateSleepCorrection(record SleepCorrectionRecord) error {
	return sleepv1.ValidateCorrection(record)
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

func validSleepClassification(value string) bool {
	return sleepv1.ValidClassification(value)
}

func validAcquisition(value string) bool {
	return sleepv1.ValidAcquisition(value)
}

func validEvidenceStatus(value string) bool {
	return sleepv1.ValidEvidenceStatus(value)
}

func validCorrectionReason(value string) bool {
	return sleepv1.ValidCorrectionReason(value)
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
