package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

var ErrTaskNeedsReview = errors.New("task has conflicting edits; review them in Approvals before editing or placing it")
var ErrTaskReviewChanged = errors.New("the task or its downloaded versions changed; refresh and review them again")

type TaskSyncVersion struct {
	ChoiceID string     `json:"choice_id"`
	Task     TaskRecord `json:"task"`
}
type TaskSyncConflict struct {
	TaskID      string            `json:"task_id"`
	ReviewToken string            `json:"review_token"`
	Local       TaskRecord        `json:"local"`
	Downloaded  []TaskSyncVersion `json:"downloaded"`
}
type TaskSyncResolution struct {
	ReviewToken string           `json:"review_token"`
	TaskID      string           `json:"task_id"`
	ChosenID    string           `json:"chosen_id"`
	DecidedAt   time.Time        `json:"decided_at"`
	Before      TaskSyncConflict `json:"before"`
	Result      TaskRecord       `json:"result"`
}

func (s *Store) initializeTaskConflicts(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS local_task_sync_fingerprints(record_id TEXT NOT NULL,task_id TEXT NOT NULL,revision INTEGER NOT NULL,payload_hash TEXT NOT NULL,PRIMARY KEY(record_id,payload_hash))`,
		`CREATE INDEX IF NOT EXISTS idx_task_fingerprint_task ON local_task_sync_fingerprints(task_id,revision)`,
		`CREATE TABLE IF NOT EXISTS local_task_sync_conflicts(choice_id TEXT PRIMARY KEY,record_id TEXT NOT NULL,task_id TEXT NOT NULL,revision INTEGER NOT NULL,payload_json BLOB NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_task_conflict_task ON local_task_sync_conflicts(task_id,revision)`,
		`CREATE TABLE IF NOT EXISTS local_task_sync_resolutions(review_token TEXT PRIMARY KEY,task_id TEXT NOT NULL,decided_at TEXT NOT NULL,payload_json BLOB NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_task_resolution_task ON local_task_sync_resolutions(task_id)`,
		`CREATE TRIGGER IF NOT EXISTS local_task_review_erasure AFTER DELETE ON local_tasks BEGIN
    DELETE FROM local_task_sync_conflicts WHERE task_id=OLD.task_id;
    DELETE FROM local_task_sync_fingerprints WHERE task_id=OLD.task_id;
    DELETE FROM local_task_sync_resolutions WHERE task_id=OLD.task_id; END`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func taskPayloadHash(payload []byte) string {
	value := sha256.Sum256(payload)
	return hex.EncodeToString(value[:])
}
func taskNeedsReview(ctx context.Context, q taskRowQueryer, taskID string) (bool, error) {
	var found bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_task_sync_conflicts WHERE task_id=?)`, taskID).Scan(&found)
	return found, err
}
func requireTaskReviewed(ctx context.Context, q taskRowQueryer, taskID string) error {
	found, err := taskNeedsReview(ctx, q, taskID)
	if err != nil {
		return err
	}
	if found {
		return ErrTaskNeedsReview
	}
	return nil
}
func (s *Store) TaskSyncConflictCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT task_id) FROM local_task_sync_conflicts`).Scan(&count)
	return count, err
}
func (s *Store) TaskSyncConflictIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT task_id FROM local_task_sync_conflicts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}
func (s *Store) ListTaskSyncConflicts(ctx context.Context) ([]TaskSyncConflict, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT task_id FROM local_task_sync_conflicts ORDER BY task_id`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := make([]TaskSyncConflict, 0, len(ids))
	for _, id := range ids {
		value, err := taskSyncConflictFrom(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, tx.Commit()
}
func taskSyncConflictFrom(ctx context.Context, tx *sql.Tx, taskID string) (TaskSyncConflict, error) {
	local, err := taskByIDFrom(ctx, tx, taskID)
	if err != nil {
		return TaskSyncConflict{}, err
	}
	result := TaskSyncConflict{TaskID: taskID, Local: local, Downloaded: []TaskSyncVersion{}}
	rows, err := tx.QueryContext(ctx, `SELECT choice_id,payload_json FROM local_task_sync_conflicts WHERE task_id=? ORDER BY revision DESC,choice_id`, taskID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var value TaskSyncVersion
		var payload []byte
		if err := rows.Scan(&value.ChoiceID, &payload); err != nil {
			return result, err
		}
		if err := json.Unmarshal(payload, &value.Task); err != nil {
			return result, err
		}
		result.Downloaded = append(result.Downloaded, value)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(result.Downloaded) == 0 {
		return result, ErrTaskReviewChanged
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	result.ReviewToken = "task_review_" + taskPayloadHash(encoded)
	return result, nil
}
func retainTaskConflict(ctx context.Context, tx *sql.Tx, prepared preparedSyncedTask) (bool, error) {
	id := taskRevisionRecordID(prepared.record.TaskID, prepared.record.Revision)
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_task_sync_conflicts VALUES(?,?,?,?,?)`, "task_choice_"+taskPayloadHash(prepared.encoded), id, prepared.record.TaskID, prepared.record.Revision, prepared.encoded)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	return changed > 0, err
}

// A choice is bound to every version displayed and the current local record.
// Resolution creates a new revision; it never rewrites the server's immutable id.
func (s *Store) ResolveTaskSyncConflict(ctx context.Context, taskID, reviewToken, choiceID string, now time.Time) (TaskSyncResolution, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskSyncResolution{}, err
	}
	defer tx.Rollback()
	review, err := taskSyncConflictFrom(ctx, tx, taskID)
	if err != nil {
		return TaskSyncResolution{}, err
	}
	if review.ReviewToken != reviewToken {
		return TaskSyncResolution{}, ErrTaskReviewChanged
	}
	chosen := review.Local
	found := choiceID == "local"
	revision := review.Local.Revision
	for _, candidate := range review.Downloaded {
		if candidate.Task.Revision > revision {
			revision = candidate.Task.Revision
		}
		if candidate.ChoiceID == choiceID {
			chosen = candidate.Task
			found = true
		}
	}
	if !found || int64(revision) >= 9007199254740991 || now.IsZero() {
		return TaskSyncResolution{}, errors.New("choose a reviewed task version")
	}
	chosen.CreatedAt = review.Local.CreatedAt
	chosen.Revision = revision + 1
	chosen.UpdatedAt = now.UTC()
	if err := validateTask(chosen); err != nil {
		return TaskSyncResolution{}, err
	}
	encoded, err := json.Marshal(normalizeTaskTimes(chosen))
	if err != nil {
		return TaskSyncResolution{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE local_tasks SET status=?,revision=?,payload_json=? WHERE task_id=?`, chosen.Status, chosen.Revision, encoded, taskID); err != nil {
		return TaskSyncResolution{}, err
	}
	// Remember reviewed remote payloads so restore/replay cannot reopen the same
	// conflict or mistake a rejected local payload for one accepted by the server.
	for _, candidate := range review.Downloaded {
		payload, err := json.Marshal(normalizeTaskTimes(candidate.Task))
		if err != nil {
			return TaskSyncResolution{}, err
		}
		if err := recordTaskFingerprint(ctx, tx, TaskSyncRecord{RecordID: taskRevisionRecordID(taskID, candidate.Task.Revision), TaskID: taskID, Payload: payload}); err != nil {
			return TaskSyncResolution{}, err
		}
	}
	result := TaskSyncResolution{ReviewToken: reviewToken, TaskID: taskID, ChosenID: choiceID, DecidedAt: now.UTC(), Before: review, Result: chosen}
	history, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_task_sync_resolutions VALUES(?,?,?,?)`, reviewToken, taskID, formatSQLiteTime(now), history); err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_task_sync_conflicts WHERE task_id=?`, taskID); err != nil {
		return result, err
	}
	return result, tx.Commit()
}
func recordTaskFingerprint(ctx context.Context, tx *sql.Tx, record TaskSyncRecord) error {
	if len(record.Payload) == 0 {
		return nil
	}
	var task TaskRecord
	if err := json.Unmarshal(record.Payload, &task); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_task_sync_fingerprints VALUES(?,?,?,?)`, record.RecordID, record.TaskID, task.Revision, taskPayloadHash(record.Payload))
	return err
}
func (s *Store) ListTaskSyncResolutions(ctx context.Context) ([]TaskSyncResolution, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM local_task_sync_resolutions ORDER BY decided_at DESC,review_token LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []TaskSyncResolution{}
	for rows.Next() {
		var payload []byte
		var item TaskSyncResolution
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
