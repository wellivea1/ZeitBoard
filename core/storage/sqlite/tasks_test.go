package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestTaskCRUDAndOpenDomainMapping(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	latest := created.Add(72 * time.Hour)
	afterWake := 90

	task := TaskRecord{
		TaskID:                    "task_paperwork_01",
		Title:                     "File the paperwork",
		DurationMinutes:           60,
		Status:                    TaskStatusOpen,
		CreatedAt:                 created,
		LatestFinishAt:            &latest,
		PreferredAfterWakeMinutes: &afterWake,
		MinimumConfidence:         "medium",
	}
	if err := store.AddTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	done := TaskRecord{
		TaskID:          "task_done_01",
		Title:           "Already handled",
		DurationMinutes: 30,
		Status:          TaskStatusDone,
		CreatedAt:       created.Add(-time.Hour),
	}
	if err := store.AddTask(ctx, done); err != nil {
		t.Fatal(err)
	}

	// Open tasks sort before done tasks.
	tasks, err := store.ListTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0].TaskID != task.TaskID || tasks[1].Status != TaskStatusDone {
		t.Fatalf("list order = %+v", tasks)
	}

	// Domain mapping carries constraints and always requires approval.
	domainTasks, open, err := store.OpenDomainTasks(ctx, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if len(domainTasks) != 1 || len(open) != 1 {
		t.Fatalf("open tasks = %d domain = %d, want 1/1", len(open), len(domainTasks))
	}
	mapped := domainTasks[0]
	if mapped.EstimatedDuration != 60*time.Minute || !mapped.Constraint.RequiresApproval {
		t.Fatalf("mapped task = %+v", mapped)
	}
	if mapped.Constraint.LatestFinish == nil || mapped.Constraint.PreferredAfterWake == nil {
		t.Fatalf("mapped constraints missing: %+v", mapped.Constraint)
	}

	// Update, status toggle, delete.
	task.Title = "File the paperwork (updated)"
	task.DurationMinutes = 45
	task.Revision = 2
	task.UpdatedAt = created.Add(time.Minute)
	if err := store.UpdateTask(ctx, task, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTaskStatus(ctx, task.TaskID, TaskStatusDone, 2); err != nil {
		t.Fatal(err)
	}
	domainTasks, _, err = store.OpenDomainTasks(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(domainTasks) != 0 {
		t.Fatalf("done tasks must not be planned: %+v", domainTasks)
	}
	if err := store.DeleteTask(ctx, task.TaskID, 3); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTask(ctx, task.TaskID, 3); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("second delete error = %v, want ErrTaskNotFound", err)
	}
}

func TestTaskValidationRejectsBadRecords(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	base := TaskRecord{TaskID: "task_valid_01", Title: "ok", DurationMinutes: 30, Status: TaskStatusOpen, CreatedAt: created}

	cases := []struct {
		name   string
		mutate func(TaskRecord) TaskRecord
	}{
		{"bad id", func(r TaskRecord) TaskRecord { r.TaskID = "Bad Id!"; return r }},
		{"empty title", func(r TaskRecord) TaskRecord { r.Title = ""; return r }},
		{"duration too short", func(r TaskRecord) TaskRecord { r.DurationMinutes = 1; return r }},
		{"bad status", func(r TaskRecord) TaskRecord { r.Status = "archived"; return r }},
		{"bad confidence", func(r TaskRecord) TaskRecord { r.MinimumConfidence = "certain"; return r }},
		{"inverted window", func(r TaskRecord) TaskRecord {
			earliest := created.Add(2 * time.Hour)
			latest := created.Add(time.Hour)
			r.EarliestStartAt = &earliest
			r.LatestFinishAt = &latest
			return r
		}},
	}
	for _, tc := range cases {
		if err := store.AddTask(ctx, tc.mutate(base)); err == nil {
			t.Fatalf("%s: expected validation error", tc.name)
		}
	}
	if err := store.AddTask(ctx, base); err != nil {
		t.Fatalf("valid task rejected: %v", err)
	}
}
