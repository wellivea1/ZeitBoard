package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"non24.app/core/domain"
)

const (
	TaskStatusOpen = "open"
	TaskStatusDone = "done"
)

// TaskRecord is the local record shape for task-set.schema.json#/$defs/task.
// Tasks are user-owned planning items, not health observations: unlike sleep
// records they are mutable in place (no correction chain) — editing a task is
// changing an intention, not rewriting evidence (ADR-0018). Titles are private
// user text: they stay local, never enter trusted views or LLM context.
type TaskRecord struct {
	TaskID                    string     `json:"task_id"`
	Title                     string     `json:"title"`
	DurationMinutes           int        `json:"duration_minutes"`
	Status                    string     `json:"status"`
	CreatedAt                 time.Time  `json:"created_at"`
	EarliestStartAt           *time.Time `json:"earliest_start_at,omitempty"`
	LatestFinishAt            *time.Time `json:"latest_finish_at,omitempty"`
	PreferredAfterWakeMinutes *int       `json:"preferred_after_wake_minutes,omitempty"`
	MinimumConfidence         string     `json:"minimum_confidence,omitempty"`
	// Revision makes mutable tasks syncable over the append-only log: each
	// edit bumps it, and the revision syncs as an immutable record with id
	// "<task_id>_r<revision>"; consumers keep the highest revision (ADR-0020).
	Revision  int       `json:"revision,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

var (
	ErrTaskNotFound         = errors.New("task does not exist")
	ErrTaskRevisionConflict = errors.New("task revision conflict")
)

type taskRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) AddTask(ctx context.Context, record TaskRecord) error {
	record.Revision = 1
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	if err := validateTask(record); err != nil {
		return err
	}
	encoded, err := json.Marshal(normalizeTaskTimes(record))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO local_tasks(task_id, status, revision, created_at, payload_json) VALUES(?, ?, ?, ?, ?)`,
		record.TaskID, record.Status, record.Revision, formatSQLiteTime(record.CreatedAt), encoded,
	)
	return err
}

func (s *Store) ListTasks(ctx context.Context) ([]TaskRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT revision, payload_json FROM local_tasks
		ORDER BY CASE status WHEN 'open' THEN 0 ELSE 1 END, created_at, task_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []TaskRecord
	for rows.Next() {
		var revision int
		var payload []byte
		if err := rows.Scan(&revision, &payload); err != nil {
			return nil, err
		}
		var record TaskRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, err
		}
		record.Revision = revision
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = record.CreatedAt
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// UpdateTask replaces the stored task (same id) with the given record and
// bumps its sync revision if the caller still holds the current revision.
func (s *Store) UpdateTask(ctx context.Context, record TaskRecord, expectedRevision int) error {
	if expectedRevision < 1 || record.Revision != expectedRevision+1 {
		return errors.New("updated task revision must increment the expected revision")
	}
	if err := validateTask(record); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	existing, err := taskByIDFrom(ctx, tx, record.TaskID)
	if err != nil {
		return err
	}
	if effectiveRevision(existing) != expectedRevision {
		return ErrTaskRevisionConflict
	}
	if !record.CreatedAt.Equal(existing.CreatedAt) {
		return errors.New("task created_at is immutable")
	}
	encoded, err := json.Marshal(normalizeTaskTimes(record))
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE local_tasks SET status = ?, revision = ?, payload_json = ?
		 WHERE task_id = ? AND revision = ?`,
		record.Status, record.Revision, encoded, record.TaskID, expectedRevision,
	)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrTaskRevisionConflict
	}
	return tx.Commit()
}

func (s *Store) SetTaskStatus(ctx context.Context, taskID, status string, expectedRevision int) error {
	if status != TaskStatusOpen && status != TaskStatusDone {
		return errors.New("task status must be open or done")
	}
	if expectedRevision < 1 {
		return errors.New("expected task revision must be at least 1")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	record, err := taskByIDFrom(ctx, tx, taskID)
	if err != nil {
		return err
	}
	if effectiveRevision(record) != expectedRevision {
		return ErrTaskRevisionConflict
	}
	record.Status = status
	record.Revision = expectedRevision + 1
	record.UpdatedAt = time.Now().UTC()
	if err := validateTask(record); err != nil {
		return err
	}
	encoded, err := json.Marshal(normalizeTaskTimes(record))
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE local_tasks SET status = ?, revision = ?, payload_json = ?
		 WHERE task_id = ? AND revision = ?`,
		record.Status, record.Revision, encoded, taskID, expectedRevision)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrTaskRevisionConflict
	}
	return tx.Commit()
}

func (s *Store) DeleteTask(ctx context.Context, taskID string, expectedRevision int) error {
	if !contractIdentifier.MatchString(taskID) {
		return errors.New("task_id must match the v1 identifier format")
	}
	if expectedRevision < 1 {
		return errors.New("expected task revision must be at least 1")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentRevision int
	err = tx.QueryRowContext(ctx, `SELECT revision FROM local_tasks WHERE task_id = ?`, taskID).Scan(&currentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	if err != nil {
		return err
	}
	if currentRevision != expectedRevision {
		return ErrTaskRevisionConflict
	}
	// Revisions that already reached the synced backend need server-side
	// erasure too (ADR-0017): enqueue every pushed revision before its
	// bookkeeping disappears. Never-pushed tasks never left this device.
	erasedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_sleep_erasures(record_id, erased_at)
		SELECT record_id, ? FROM local_task_sync_records WHERE task_id = ?`, erasedAt, taskID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_task_sync_records WHERE task_id = ?`, taskID); err != nil {
		_ = tx.Rollback()
		return err
	}
	result, err := tx.ExecContext(ctx,
		`DELETE FROM local_tasks WHERE task_id = ? AND revision = ?`, taskID, expectedRevision)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := requireTaskAffected(result); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) taskByID(ctx context.Context, taskID string) (TaskRecord, error) {
	return taskByIDFrom(ctx, s.db, taskID)
}

func (s *Store) GetTask(ctx context.Context, taskID string) (TaskRecord, error) {
	return s.taskByID(ctx, taskID)
}

func taskByIDFrom(ctx context.Context, queryer taskRowQueryer, taskID string) (TaskRecord, error) {
	var revision int
	var payload []byte
	err := queryer.QueryRowContext(ctx, `SELECT revision, payload_json FROM local_tasks WHERE task_id = ?`, taskID).Scan(&revision, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskRecord{}, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	if err != nil {
		return TaskRecord{}, err
	}
	var record TaskRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return TaskRecord{}, err
	}
	record.Revision = revision
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	return record, nil
}

// OpenDomainTasks maps open tasks to scheduler inputs. Every task requires
// approval: the scheduler proposes, the human decides.
func (s *Store) OpenDomainTasks(ctx context.Context, zoneID string) ([]domain.FlexibleTask, []TaskRecord, error) {
	records, err := s.ListTasks(ctx)
	if err != nil {
		return nil, nil, err
	}
	var tasks []domain.FlexibleTask
	var open []TaskRecord
	for _, record := range records {
		if record.Status != TaskStatusOpen {
			continue
		}
		task, err := domainTaskFromRecord(record, zoneID)
		if err != nil {
			return nil, nil, fmt.Errorf("task %s: %w", record.TaskID, err)
		}
		tasks = append(tasks, task)
		open = append(open, record)
	}
	return tasks, open, nil
}

func domainTaskFromRecord(record TaskRecord, zoneID string) (domain.FlexibleTask, error) {
	constraint := domain.TaskConstraint{
		MinimumConfidence: taskConfidenceLevel(record.MinimumConfidence),
		RequiresApproval:  true,
	}
	if record.EarliestStartAt != nil {
		instant, err := domain.NewZonedInstant(*record.EarliestStartAt, zoneID)
		if err != nil {
			return domain.FlexibleTask{}, err
		}
		constraint.EarliestStart = &instant
	}
	if record.LatestFinishAt != nil {
		instant, err := domain.NewZonedInstant(*record.LatestFinishAt, zoneID)
		if err != nil {
			return domain.FlexibleTask{}, err
		}
		constraint.LatestFinish = &instant
	}
	if record.PreferredAfterWakeMinutes != nil {
		duration := time.Duration(*record.PreferredAfterWakeMinutes) * time.Minute
		constraint.PreferredAfterWake = &duration
	}
	return domain.FlexibleTask{
		ID:                domain.FlexibleTaskID(record.TaskID),
		Title:             record.Title,
		EstimatedDuration: time.Duration(record.DurationMinutes) * time.Minute,
		Constraint:        constraint,
	}, nil
}

func taskConfidenceLevel(value string) domain.ConfidenceLevel {
	switch value {
	case "high":
		return domain.ConfidenceHigh
	case "medium":
		return domain.ConfidenceMedium
	default:
		return domain.ConfidenceLow
	}
}

func validateTask(record TaskRecord) error {
	if !contractIdentifier.MatchString(record.TaskID) {
		return errors.New("task_id must match the v1 identifier format")
	}
	if record.Title == "" || len(record.Title) > 120 {
		return errors.New("title must be 1-120 characters")
	}
	if record.DurationMinutes < 5 || record.DurationMinutes > 720 {
		return errors.New("duration_minutes must be between 5 and 720")
	}
	if record.Status != TaskStatusOpen && record.Status != TaskStatusDone {
		return errors.New("status must be open or done")
	}
	if record.CreatedAt.IsZero() {
		return errors.New("created_at is required")
	}
	if record.Revision < 1 {
		return errors.New("revision must be at least 1")
	}
	if record.UpdatedAt.IsZero() {
		return errors.New("updated_at is required")
	}
	if record.PreferredAfterWakeMinutes != nil &&
		(*record.PreferredAfterWakeMinutes < 0 || *record.PreferredAfterWakeMinutes > 1440) {
		return errors.New("preferred_after_wake_minutes must be between 0 and 1440")
	}
	if record.MinimumConfidence != "" &&
		record.MinimumConfidence != "low" && record.MinimumConfidence != "medium" && record.MinimumConfidence != "high" {
		return errors.New("minimum_confidence must be low, medium, or high")
	}
	if record.EarliestStartAt != nil && record.LatestFinishAt != nil &&
		!record.LatestFinishAt.After(*record.EarliestStartAt) {
		return errors.New("latest_finish_at must be after earliest_start_at")
	}
	return nil
}

func effectiveRevision(record TaskRecord) int {
	if record.Revision < 1 {
		return 1 // rows written before revisions existed count as revision 1
	}
	return record.Revision
}

func normalizeTaskTimes(record TaskRecord) TaskRecord {
	record.CreatedAt = record.CreatedAt.UTC()
	if !record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.UpdatedAt.UTC()
	}
	if record.EarliestStartAt != nil {
		value := record.EarliestStartAt.UTC()
		record.EarliestStartAt = &value
	}
	if record.LatestFinishAt != nil {
		value := record.LatestFinishAt.UTC()
		record.LatestFinishAt = &value
	}
	return record
}

func requireTaskAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrTaskNotFound
	}
	return nil
}
