package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	// MaxSyncPullPageSize matches the bounded page returned by the v1 sync
	// service. Keeping the limit here prevents accidental unbounded write
	// transactions if a server returns a malformed response.
	MaxSyncPullPageSize = 500

	syncCompactionPendingKey = "sync_secure_compaction_pending"
)

// SyncPullRecord is the closed set of records accepted by ApplySyncPullPage.
// Concrete variants keep wire decoding outside storage while preventing
// callers from combining incompatible record fields.
type SyncPullRecord interface {
	isSyncPullRecord()
}

type SyncPullObservation struct {
	Observation SleepObservationRecord
}

func (SyncPullObservation) isSyncPullRecord() {}

type SyncPullCorrection struct {
	Correction SleepCorrectionRecord
}

func (SyncPullCorrection) isSyncPullRecord() {}

type SyncPullTask struct {
	Task TaskRecord
}

func (SyncPullTask) isSyncPullRecord() {}

type SyncPullTombstone struct {
	RecordID   string
	RecordKind string
}

func (SyncPullTombstone) isSyncPullRecord() {}

type SyncPullPage struct {
	Cursor  int64
	Records []SyncPullRecord
}

type SyncPullPageResult struct {
	Applied           int
	Skipped           int
	TombstonesApplied int
}

type preparedSyncPullKind uint8

const (
	preparedSyncPullObservation preparedSyncPullKind = iota + 1
	preparedSyncPullCorrection
	preparedSyncPullTask
	preparedSyncPullTombstone
)

type preparedSyncPullRecord struct {
	kind        preparedSyncPullKind
	observation preparedSyncedSleepObservation
	correction  preparedSyncedSleepCorrection
	task        preparedSyncedTask
	recordID    string
	recordKind  string
}

type preparedSyncedSleepObservation struct {
	record  SleepObservationRecord
	encoded []byte
}

type preparedSyncedSleepCorrection struct {
	record  SleepCorrectionRecord
	changes []byte
	encoded []byte
}

type preparedSyncedTask struct {
	record  TaskRecord
	encoded []byte
}

// ApplySyncPullPage applies one server page and advances its cursor atomically.
// Corrections are deferred until all observations in the page exist, and
// tombstones are applied last so erasure wins over records in the same page.
// A page containing tombstones performs one secure checkpoint/compaction after
// commit. Pending compaction state is durable, so a post-commit failure is
// retried by the next page without replaying or losing the cursor.
func (s *Store) ApplySyncPullPage(ctx context.Context, page SyncPullPage) (SyncPullPageResult, error) {
	return s.applySyncPullPage(ctx, page, s.compactDeletedData)
}

func (s *Store) applySyncPullPage(
	ctx context.Context,
	page SyncPullPage,
	compact func(context.Context) error,
) (SyncPullPageResult, error) {
	prepared, hasTombstones, err := prepareSyncPullPage(page)
	if err != nil {
		return SyncPullPageResult{}, err
	}
	if compact == nil {
		return SyncPullPageResult{}, errors.New("sync pull compactor is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncPullPageResult{}, err
	}
	defer tx.Rollback()

	currentCursor, err := sleepSyncCursorFrom(ctx, tx)
	if err != nil {
		return SyncPullPageResult{}, fmt.Errorf("read current sync cursor: %w", err)
	}
	if page.Cursor < currentCursor {
		return SyncPullPageResult{}, fmt.Errorf(
			"sync pull cursor %d is behind current cursor %d",
			page.Cursor,
			currentCursor,
		)
	}
	compactionPending, err := syncCompactionPending(ctx, tx)
	if err != nil {
		return SyncPullPageResult{}, fmt.Errorf("read sync compaction state: %w", err)
	}

	syncedAt := time.Now().UTC()
	result := SyncPullPageResult{}
	for index, record := range prepared {
		var applied bool
		handled := true
		switch record.kind {
		case preparedSyncPullObservation:
			applied, err = insertSyncedSleepObservationTx(ctx, tx, record.observation, syncedAt)
		case preparedSyncPullTask:
			applied, err = applySyncedTaskTx(ctx, tx, record.task, syncedAt)
		default:
			handled = false
		}
		if err != nil {
			return SyncPullPageResult{}, fmt.Errorf("apply sync pull record %d: %w", index, err)
		}
		if applied {
			result.Applied++
		} else if handled {
			result.Skipped++
		}
	}

	for index, record := range prepared {
		if record.kind != preparedSyncPullCorrection {
			continue
		}
		applied, applyErr := insertSyncedSleepCorrectionTx(ctx, tx, record.correction, syncedAt)
		if errors.Is(applyErr, ErrSleepObservationMissing) {
			result.Skipped++
			continue
		}
		if applyErr != nil {
			return SyncPullPageResult{}, fmt.Errorf("apply sync pull correction %d: %w", index, applyErr)
		}
		if applied {
			result.Applied++
		} else {
			result.Skipped++
		}
	}

	for index, record := range prepared {
		if record.kind != preparedSyncPullTombstone {
			continue
		}
		var applied bool
		recordKind, err := syncPullTombstoneKindTx(ctx, tx, record.recordID, record.recordKind)
		if err != nil {
			return SyncPullPageResult{}, fmt.Errorf("classify sync pull tombstone %d: %w", index, err)
		}
		if recordKind == "task" {
			applied, err = eraseSyncedTaskRecordTx(ctx, tx, record.recordID)
			if err == nil {
				_, err = tx.ExecContext(ctx,
					`DELETE FROM local_sleep_erasures WHERE record_id = ?`,
					record.recordID,
				)
			}
		} else {
			applied, err = eraseSyncedSleepRecordTx(ctx, tx, record.recordID)
		}
		if err != nil {
			return SyncPullPageResult{}, fmt.Errorf("apply sync pull tombstone %d: %w", index, err)
		}
		if applied {
			result.TombstonesApplied++
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO local_sync_state(key, value) VALUES('sleep_sync_cursor', ?)`,
		strconv.FormatInt(page.Cursor, 10)); err != nil {
		return SyncPullPageResult{}, fmt.Errorf("save sync pull cursor: %w", err)
	}
	if hasTombstones {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO local_sync_state(key, value) VALUES(?, '1')`,
			syncCompactionPendingKey); err != nil {
			return SyncPullPageResult{}, fmt.Errorf("record pending sync compaction: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return SyncPullPageResult{}, fmt.Errorf("commit sync pull page: %w", err)
	}

	if !hasTombstones && !compactionPending {
		return result, nil
	}
	if err := compact(ctx); err != nil {
		return result, fmt.Errorf("compact synced hard erasures after page commit: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM local_sync_state WHERE key = ?`,
		syncCompactionPendingKey); err != nil {
		return result, fmt.Errorf("clear pending sync compaction: %w", err)
	}
	return result, nil
}

func prepareSyncPullPage(page SyncPullPage) ([]preparedSyncPullRecord, bool, error) {
	if page.Cursor < 0 {
		return nil, false, errors.New("sync pull cursor must not be negative")
	}
	if len(page.Records) > MaxSyncPullPageSize {
		return nil, false, fmt.Errorf("sync pull page contains %d records; maximum is %d", len(page.Records), MaxSyncPullPageSize)
	}
	prepared := make([]preparedSyncPullRecord, 0, len(page.Records))
	hasTombstones := false
	for index, record := range page.Records {
		item, err := prepareSyncPullRecord(record)
		if err != nil {
			return nil, false, fmt.Errorf("prepare sync pull record %d: %w", index, err)
		}
		if item.kind == preparedSyncPullTombstone {
			hasTombstones = true
		}
		prepared = append(prepared, item)
	}
	return prepared, hasTombstones, nil
}

func prepareSyncPullRecord(record SyncPullRecord) (preparedSyncPullRecord, error) {
	switch value := record.(type) {
	case SyncPullObservation:
		prepared, err := prepareSyncedSleepObservation(value.Observation)
		return preparedSyncPullRecord{kind: preparedSyncPullObservation, observation: prepared}, err
	case *SyncPullObservation:
		if value == nil {
			return preparedSyncPullRecord{}, errors.New("sleep observation record is nil")
		}
		prepared, err := prepareSyncedSleepObservation(value.Observation)
		return preparedSyncPullRecord{kind: preparedSyncPullObservation, observation: prepared}, err
	case SyncPullCorrection:
		prepared, err := prepareSyncedSleepCorrection(value.Correction)
		return preparedSyncPullRecord{kind: preparedSyncPullCorrection, correction: prepared}, err
	case *SyncPullCorrection:
		if value == nil {
			return preparedSyncPullRecord{}, errors.New("sleep correction record is nil")
		}
		prepared, err := prepareSyncedSleepCorrection(value.Correction)
		return preparedSyncPullRecord{kind: preparedSyncPullCorrection, correction: prepared}, err
	case SyncPullTask:
		prepared, err := prepareSyncedTask(value.Task)
		return preparedSyncPullRecord{kind: preparedSyncPullTask, task: prepared}, err
	case *SyncPullTask:
		if value == nil {
			return preparedSyncPullRecord{}, errors.New("task record is nil")
		}
		prepared, err := prepareSyncedTask(value.Task)
		return preparedSyncPullRecord{kind: preparedSyncPullTask, task: prepared}, err
	case SyncPullTombstone:
		if err := validateSyncPullTombstone(value.RecordID, value.RecordKind); err != nil {
			return preparedSyncPullRecord{}, err
		}
		return preparedSyncPullRecord{kind: preparedSyncPullTombstone, recordID: value.RecordID, recordKind: value.RecordKind}, nil
	case *SyncPullTombstone:
		if value == nil {
			return preparedSyncPullRecord{}, errors.New("tombstone record is nil")
		}
		if err := validateSyncPullTombstone(value.RecordID, value.RecordKind); err != nil {
			return preparedSyncPullRecord{}, err
		}
		return preparedSyncPullRecord{kind: preparedSyncPullTombstone, recordID: value.RecordID, recordKind: value.RecordKind}, nil
	default:
		return preparedSyncPullRecord{}, fmt.Errorf("unsupported sync pull record type %T", record)
	}
}

func prepareSyncedSleepObservation(record SleepObservationRecord) (preparedSyncedSleepObservation, error) {
	if err := validateSleepObservation(record); err != nil {
		return preparedSyncedSleepObservation{}, err
	}
	record.StartAt = record.StartAt.UTC()
	record.EndAt = record.EndAt.UTC()
	record.Provenance.RecordedAt = record.Provenance.RecordedAt.UTC()
	encoded, err := json.Marshal(record)
	if err != nil {
		return preparedSyncedSleepObservation{}, err
	}
	return preparedSyncedSleepObservation{record: record, encoded: encoded}, nil
}

func prepareSyncedSleepCorrection(record SleepCorrectionRecord) (preparedSyncedSleepCorrection, error) {
	if err := validateSleepCorrection(record); err != nil {
		return preparedSyncedSleepCorrection{}, err
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
		return preparedSyncedSleepCorrection{}, err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return preparedSyncedSleepCorrection{}, err
	}
	return preparedSyncedSleepCorrection{record: record, changes: changes, encoded: encoded}, nil
}

func prepareSyncedTask(record TaskRecord) (preparedSyncedTask, error) {
	if err := validateTask(record); err != nil {
		return preparedSyncedTask{}, err
	}
	record = normalizeTaskTimes(record)
	encoded, err := json.Marshal(record)
	if err != nil {
		return preparedSyncedTask{}, err
	}
	return preparedSyncedTask{record: record, encoded: encoded}, nil
}

func insertSyncedSleepObservationTx(
	ctx context.Context,
	tx *sql.Tx,
	prepared preparedSyncedSleepObservation,
	syncedAt time.Time,
) (bool, error) {
	record := prepared.record
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_sleep_observations(
		observation_id, kind, start_at, end_at, zone_id, classification,
		acquisition_method, evidence_status, recorded_at, source_record_id, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ObservationID, record.Kind, formatSQLiteTime(record.StartAt), formatSQLiteTime(record.EndAt),
		record.ZoneID, record.Sleep.Classification, record.Provenance.AcquisitionMethod,
		record.Provenance.EvidenceStatus, formatSQLiteTime(record.Provenance.RecordedAt),
		record.Provenance.SourceRecordID, prepared.encoded,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	synced := newSleepSyncRecord(
		record.ObservationID,
		SleepSyncKindObservation,
		record.Provenance.RecordedAt,
		prepared.encoded,
	)
	if err := markSleepSyncRecordsPushedTx(ctx, tx, []SleepSyncRecord{synced}, syncedAt); err != nil {
		return false, err
	}
	return rows > 0, nil
}

func insertSyncedSleepCorrectionTx(
	ctx context.Context,
	tx *sql.Tx,
	prepared preparedSyncedSleepCorrection,
	syncedAt time.Time,
) (bool, error) {
	record := prepared.record
	var exists int
	err := tx.QueryRowContext(ctx,
		`SELECT 1 FROM local_sleep_observations WHERE observation_id = ?`,
		record.TargetObservationID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("%w: %s", ErrSleepObservationMissing, record.TargetObservationID)
	}
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_sleep_corrections(
		correction_id, target_observation_id, supersedes_correction_id, created_at, reason, changes_json, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		record.CorrectionID, record.TargetObservationID, record.SupersedesCorrectionID,
		formatSQLiteTime(record.CreatedAt), record.Reason, prepared.changes, prepared.encoded,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	synced := newSleepSyncRecord(
		record.CorrectionID,
		SleepSyncKindCorrection,
		record.CreatedAt,
		prepared.encoded,
	)
	if err := markSleepSyncRecordsPushedTx(ctx, tx, []SleepSyncRecord{synced}, syncedAt); err != nil {
		return false, err
	}
	return rows > 0, nil
}

func applySyncedTaskTx(
	ctx context.Context,
	tx *sql.Tx,
	prepared preparedSyncedTask,
	syncedAt time.Time,
) (bool, error) {
	record := prepared.record
	existing, err := taskByIDFrom(ctx, tx, record.TaskID)
	applied := false
	switch {
	case errors.Is(err, ErrTaskNotFound):
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO local_tasks(task_id, status, revision, created_at, payload_json) VALUES(?, ?, ?, ?, ?)`,
			record.TaskID, record.Status, record.Revision, formatSQLiteTime(record.CreatedAt), prepared.encoded); err != nil {
			return false, err
		}
		applied = true
	case err != nil:
		return false, err
	case record.Revision > effectiveRevision(existing):
		result, err := tx.ExecContext(ctx,
			`UPDATE local_tasks SET status = ?, revision = ?, payload_json = ?
			 WHERE task_id = ? AND revision = ?`,
			record.Status, record.Revision, prepared.encoded, record.TaskID, effectiveRevision(existing))
		if err != nil {
			return false, err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		applied = changed == 1
	}
	synced := TaskSyncRecord{
		RecordID: taskRevisionRecordID(record.TaskID, record.Revision),
		TaskID:   record.TaskID,
	}
	if err := markTaskSyncRecordsPushed(ctx, tx, []TaskSyncRecord{synced}, syncedAt); err != nil {
		return false, err
	}
	return applied, nil
}

func eraseSyncedSleepRecordTx(ctx context.Context, tx *sql.Tx, recordID string) (bool, error) {
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
	return deletedObservations+deletedCorrections > 0, nil
}

func eraseSyncedTaskRecordTx(ctx context.Context, tx *sql.Tx, recordID string) (bool, error) {
	match := taskRevisionIDPattern.FindStringSubmatch(recordID)
	if match == nil {
		return false, fmt.Errorf("record id %q is not a task revision id", recordID)
	}
	taskID := match[1]
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_task_sync_records WHERE task_id = ?`, taskID); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM local_tasks WHERE task_id = ?`, taskID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func markSleepSyncRecordsPushedTx(
	ctx context.Context,
	tx *sql.Tx,
	records []SleepSyncRecord,
	pushedAt time.Time,
) error {
	for _, record := range records {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO local_sleep_sync_records(record_id, kind, payload_hash, pushed_at) VALUES(?, ?, ?, ?)`,
			record.RecordID, record.Kind, record.PayloadHash, formatSQLiteTime(pushedAt)); err != nil {
			return err
		}
	}
	return nil
}

func sleepSyncCursorFrom(ctx context.Context, queryer taskRowQueryer) (int64, error) {
	var raw string
	err := queryer.QueryRowContext(ctx,
		`SELECT value FROM local_sync_state WHERE key = 'sleep_sync_cursor'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	cursor, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse sleep sync cursor %q: %w", raw, err)
	}
	if cursor < 0 {
		return 0, errors.New("stored sleep sync cursor must not be negative")
	}
	return cursor, nil
}

func syncCompactionPending(ctx context.Context, queryer taskRowQueryer) (bool, error) {
	var value string
	err := queryer.QueryRowContext(ctx,
		`SELECT value FROM local_sync_state WHERE key = ?`,
		syncCompactionPendingKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return value == "1", nil
}

func validateSyncPullTombstone(recordID, recordKind string) error {
	if !contractIdentifier.MatchString(recordID) {
		return errors.New("tombstone record_id must match a sleep or task revision identifier")
	}
	switch recordKind {
	case "", SleepSyncKindObservation, SleepSyncKindCorrection:
		return nil
	case "task":
		if !taskRevisionIDPattern.MatchString(recordID) {
			return errors.New("task tombstone record_id must identify a task revision")
		}
		return nil
	default:
		return fmt.Errorf("unsupported tombstone record kind %q", recordKind)
	}
}

func syncPullTombstoneKindTx(
	ctx context.Context,
	tx *sql.Tx,
	recordID string,
	recordKind string,
) (string, error) {
	if recordKind != "" {
		return recordKind, nil
	}

	match := taskRevisionIDPattern.FindStringSubmatch(recordID)
	taskID := ""
	if match != nil {
		taskID = match[1]
	}
	var taskEvidence, sleepEvidence int
	if err := tx.QueryRowContext(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM local_task_sync_records WHERE record_id = ?)
			OR EXISTS(SELECT 1 FROM local_tasks WHERE task_id = ?),
			EXISTS(SELECT 1 FROM local_sleep_sync_records WHERE record_id = ?)
			OR EXISTS(SELECT 1 FROM local_sleep_observations WHERE observation_id = ?)
			OR EXISTS(SELECT 1 FROM local_sleep_corrections WHERE correction_id = ?)`,
		recordID, taskID, recordID, recordID, recordID,
	).Scan(&taskEvidence, &sleepEvidence); err != nil {
		return "", err
	}
	if taskEvidence != 0 && sleepEvidence != 0 {
		return "", fmt.Errorf("legacy tombstone %q is ambiguous between task and sleep data", recordID)
	}
	if taskEvidence != 0 || IsTaskRevisionID(recordID) {
		return "task", nil
	}
	return SleepSyncKindObservation, nil
}
