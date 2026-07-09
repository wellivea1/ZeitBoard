package sqlite

import (
	"context"
	"encoding/json"
	"errors"
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

// UnpushedTaskSyncRecords returns the current revision of every task whose
// current revision has not been pushed (or applied from a pull) yet.
func (s *Store) UnpushedTaskSyncRecords(ctx context.Context) ([]TaskSyncRecord, error) {
	tasks, err := s.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	known := map[string]struct{}{}
	rows, err := s.db.QueryContext(ctx, `SELECT record_id FROM local_task_sync_records`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		known[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var unpushed []TaskSyncRecord
	for _, task := range tasks {
		task.Revision = effectiveRevision(task)
		if task.UpdatedAt.IsZero() {
			task.UpdatedAt = task.CreatedAt
		}
		recordID := taskRevisionRecordID(task.TaskID, task.Revision)
		if _, ok := known[recordID]; ok {
			continue
		}
		payload, err := json.Marshal(normalizeTaskTimes(task))
		if err != nil {
			return nil, err
		}
		unpushed = append(unpushed, TaskSyncRecord{
			RecordID:  recordID,
			TaskID:    task.TaskID,
			CreatedAt: task.UpdatedAt.UTC(),
			Payload:   payload,
		})
	}
	return unpushed, nil
}

func (s *Store) MarkTaskSyncRecordsPushed(ctx context.Context, records []TaskSyncRecord, pushedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, record := range records {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO local_task_sync_records(record_id, task_id, pushed_at) VALUES(?, ?, ?)`,
			record.RecordID, record.TaskID, formatSQLiteTime(pushedAt)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ApplySyncedTask upserts a pulled task revision, last-writer-wins by
// revision: unknown tasks are created, newer revisions replace older state,
// stale revisions are skipped. The applied revision is recorded as already
// synced so it is never pushed back.
func (s *Store) ApplySyncedTask(ctx context.Context, record TaskRecord) (bool, error) {
	if err := validateTask(record); err != nil {
		return false, err
	}
	record = normalizeTaskTimes(record)
	encoded, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	existing, err := s.taskByID(ctx, record.TaskID)
	applied := false
	switch {
	case errors.Is(err, ErrTaskNotFound):
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO local_tasks(task_id, status, created_at, payload_json) VALUES(?, ?, ?, ?)`,
			record.TaskID, record.Status, formatSQLiteTime(record.CreatedAt), encoded); err != nil {
			return false, err
		}
		applied = true
	case err != nil:
		return false, err
	case record.Revision > effectiveRevision(existing):
		if _, err := s.db.ExecContext(ctx,
			`UPDATE local_tasks SET status = ?, payload_json = ? WHERE task_id = ?`,
			record.Status, encoded, record.TaskID); err != nil {
			return false, err
		}
		applied = true
	}
	synced := TaskSyncRecord{
		RecordID: taskRevisionRecordID(record.TaskID, record.Revision),
		TaskID:   record.TaskID,
	}
	if err := s.MarkTaskSyncRecordsPushed(ctx, []TaskSyncRecord{synced}, time.Now().UTC()); err != nil {
		return false, err
	}
	return applied, nil
}

// EraseSyncedTaskRecord applies a pulled tombstone for a task revision: any
// tombstoned revision means the task was deleted somewhere, so the local task
// is hard-deleted (idempotent; nothing is re-enqueued — the server already
// holds the tombstones).
func (s *Store) EraseSyncedTaskRecord(ctx context.Context, recordID string) (bool, error) {
	match := taskRevisionIDPattern.FindStringSubmatch(recordID)
	if match == nil {
		return false, fmt.Errorf("record id %q is not a task revision id", recordID)
	}
	taskID := match[1]
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_task_sync_records WHERE task_id = ?`, taskID); err != nil {
		_ = tx.Rollback()
		return false, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM local_tasks WHERE task_id = ?`, taskID)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return affected > 0, nil
}

// IsTaskRevisionID reports whether a synced record id names a task revision.
func IsTaskRevisionID(recordID string) bool {
	return taskRevisionIDPattern.MatchString(recordID) && len(recordID) >= 6 && recordID[:5] == "task_"
}
