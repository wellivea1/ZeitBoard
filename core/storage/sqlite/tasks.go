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
}

var ErrTaskNotFound = errors.New("task does not exist")

func (s *Store) AddTask(ctx context.Context, record TaskRecord) error {
	if err := validateTask(record); err != nil {
		return err
	}
	encoded, err := json.Marshal(normalizeTaskTimes(record))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO local_tasks(task_id, status, created_at, payload_json) VALUES(?, ?, ?, ?)`,
		record.TaskID, record.Status, formatSQLiteTime(record.CreatedAt), encoded,
	)
	return err
}

func (s *Store) ListTasks(ctx context.Context) ([]TaskRecord, error) {
	var records []TaskRecord
	err := s.readJSONRows(ctx,
		`SELECT payload_json FROM local_tasks
		 ORDER BY CASE status WHEN 'open' THEN 0 ELSE 1 END, created_at, task_id`,
		func(value []byte) error {
			var record TaskRecord
			if err := json.Unmarshal(value, &record); err != nil {
				return err
			}
			records = append(records, record)
			return nil
		})
	return records, err
}

// UpdateTask replaces the stored task (same id) with the given record.
func (s *Store) UpdateTask(ctx context.Context, record TaskRecord) error {
	if err := validateTask(record); err != nil {
		return err
	}
	encoded, err := json.Marshal(normalizeTaskTimes(record))
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE local_tasks SET status = ?, payload_json = ? WHERE task_id = ?`,
		record.Status, encoded, record.TaskID,
	)
	if err != nil {
		return err
	}
	return requireTaskAffected(result)
}

func (s *Store) SetTaskStatus(ctx context.Context, taskID, status string) error {
	if status != TaskStatusOpen && status != TaskStatusDone {
		return errors.New("task status must be open or done")
	}
	record, err := s.taskByID(ctx, taskID)
	if err != nil {
		return err
	}
	record.Status = status
	return s.UpdateTask(ctx, record)
}

func (s *Store) DeleteTask(ctx context.Context, taskID string) error {
	if !contractIdentifier.MatchString(taskID) {
		return errors.New("task_id must match the v1 identifier format")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM local_tasks WHERE task_id = ?`, taskID)
	if err != nil {
		return err
	}
	return requireTaskAffected(result)
}

func (s *Store) taskByID(ctx context.Context, taskID string) (TaskRecord, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM local_tasks WHERE task_id = ?`, taskID).Scan(&payload)
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

func normalizeTaskTimes(record TaskRecord) TaskRecord {
	record.CreatedAt = record.CreatedAt.UTC()
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
