package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// Task sync (ADR-0020): tasks are mutable, the sync log is append-only. Each
// edit bumps the task's revision, and the revision travels as an immutable
// record with id "<task_id>_r<revision>". Consumers keep the highest revision
// per task; deleting a task erases all its pushed revisions (ADR-0017).

// TaskSyncRecord is one immutable task revision ready for the push endpoint.
type TaskSyncRecord struct {
	RecordID  string
	TaskID    string
	CreatedAt time.Time
	Payload   []byte // contract-shaped task payload including revision/updated_at
}

var taskRevisionIDPattern = regexp.MustCompile(`^([a-z][a-z0-9_-]*)_r[0-9]+$`)

func taskRevisionRecordID(taskID string, revision int) string {
	return fmt.Sprintf("%s_r%d", taskID, revision)
}

const MaxTaskSyncPageSize = 100

const pendingTaskSyncRecordsFrom = `FROM local_tasks AS task
	LEFT JOIN local_task_sync_records AS synced
		ON synced.record_id = task.task_id || '_r' || task.revision
	WHERE synced.record_id IS NULL`

func (s *Store) PendingTaskSyncRecords(ctx context.Context, limit int) ([]TaskSyncRecord, error) {
	if limit < 1 || limit > MaxTaskSyncPageSize {
		return nil, fmt.Errorf("task sync page limit must be between 1 and %d", MaxTaskSyncPageSize)
	}
	return s.pendingTaskSyncRecords(ctx, limit)
}

// UnpushedTaskSyncRecords remains available for diagnostics and tests. The
// production sync path uses PendingTaskSyncRecords to keep memory bounded.
func (s *Store) UnpushedTaskSyncRecords(ctx context.Context) ([]TaskSyncRecord, error) {
	return s.pendingTaskSyncRecords(ctx, 0)
}

func (s *Store) PendingTaskSyncRecordCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+pendingTaskSyncRecordsFrom).Scan(&count)
	return count, err
}

func (s *Store) pendingTaskSyncRecords(ctx context.Context, limit int) ([]TaskSyncRecord, error) {
	query := `SELECT task.task_id, task.created_at, task.revision, task.payload_json ` + pendingTaskSyncRecordsFrom + `
		ORDER BY task.task_id`
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
	var unpushed []TaskSyncRecord
	for rows.Next() {
		var taskID, createdAt string
		var revision int
		var payload []byte
		if err := rows.Scan(&taskID, &createdAt, &revision, &payload); err != nil {
			return nil, err
		}
		var task TaskRecord
		if err := json.Unmarshal(payload, &task); err != nil {
			return nil, err
		}
		task.Revision = revision
		if task.UpdatedAt.IsZero() {
			parsed, err := time.Parse(time.RFC3339Nano, createdAt)
			if err != nil {
				return nil, err
			}
			task.UpdatedAt = parsed
		}
		payload, err = json.Marshal(normalizeTaskTimes(task))
		if err != nil {
			return nil, err
		}
		unpushed = append(unpushed, TaskSyncRecord{
			RecordID:  taskRevisionRecordID(taskID, revision),
			TaskID:    taskID,
			CreatedAt: task.UpdatedAt.UTC(),
			Payload:   payload,
		})
	}
	return unpushed, rows.Err()
}

func (s *Store) MarkTaskSyncRecordsPushed(ctx context.Context, records []TaskSyncRecord, pushedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := markTaskSyncRecordsPushed(ctx, tx, records, pushedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func markTaskSyncRecordsPushed(ctx context.Context, tx *sql.Tx, records []TaskSyncRecord, pushedAt time.Time) error {
	for _, record := range records {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO local_task_sync_records(record_id, task_id, pushed_at) VALUES(?, ?, ?)`,
			record.RecordID, record.TaskID, formatSQLiteTime(pushedAt)); err != nil {
			return err
		}
	}
	return nil
}

// ApplySyncedTask upserts a pulled task revision, last-writer-wins by
// revision: unknown tasks are created, newer revisions replace older state,
// stale revisions are skipped. The applied revision is recorded as already
// synced so it is never pushed back.
func (s *Store) ApplySyncedTask(ctx context.Context, record TaskRecord) (bool, error) {
	prepared, err := prepareSyncedTask(record)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	applied, err := applySyncedTaskTx(ctx, tx, prepared, time.Now().UTC())
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return applied, nil
}

// EraseSyncedTaskRecord applies a pulled tombstone for a task revision: any
// tombstoned revision means the task was deleted somewhere, so the local task
// is hard-deleted (idempotent; nothing is re-enqueued — the server already
// holds the tombstones).
func (s *Store) EraseSyncedTaskRecord(ctx context.Context, recordID string) (bool, error) {
	if !taskRevisionIDPattern.MatchString(recordID) {
		return false, fmt.Errorf("record id %q is not a task revision id", recordID)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	applied, err := eraseSyncedTaskRecordTx(ctx, tx, recordID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return applied, nil
}

// IsTaskRevisionID reports whether a synced record id names a task revision.
func IsTaskRevisionID(recordID string) bool {
	return taskRevisionIDPattern.MatchString(recordID) && len(recordID) >= 6 && recordID[:5] == "task_"
}
