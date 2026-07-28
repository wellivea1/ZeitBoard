package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestTaskMutationsRejectStaleRevisions(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	created := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	if err := store.AddTask(ctx, TaskRecord{
		TaskID: "task_cas_01", Title: "Original", DurationMinutes: 30,
		Status: TaskStatusOpen, CreatedAt: created,
	}); err != nil {
		t.Fatal(err)
	}

	original, err := store.GetTask(ctx, "task_cas_01")
	if err != nil {
		t.Fatal(err)
	}
	updated := original
	updated.Title = "Current"
	updated.Revision = 2
	updated.UpdatedAt = created.Add(time.Minute)
	if err := store.UpdateTask(ctx, updated, 1); err != nil {
		t.Fatal(err)
	}

	stale := original
	stale.Title = "Stale overwrite"
	stale.Revision = 2
	stale.UpdatedAt = created.Add(2 * time.Minute)
	if err := store.UpdateTask(ctx, stale, 1); !errors.Is(err, ErrTaskRevisionConflict) {
		t.Fatalf("stale update error = %v, want revision conflict", err)
	}
	if err := store.SetTaskStatus(ctx, stale.TaskID, TaskStatusDone, 1); !errors.Is(err, ErrTaskRevisionConflict) {
		t.Fatalf("stale status error = %v, want revision conflict", err)
	}
	if err := store.DeleteTask(ctx, stale.TaskID, 1); !errors.Is(err, ErrTaskRevisionConflict) {
		t.Fatalf("stale delete error = %v, want revision conflict", err)
	}
	current, err := store.GetTask(ctx, stale.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Title != "Current" || current.Revision != 2 || current.Status != TaskStatusOpen {
		t.Fatalf("stale mutation changed task: %#v", current)
	}
}

func TestApplySyncedTaskRollsBackStateWhenBookkeepingFails(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "task-sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER fail_task_sync_bookkeeping
		BEFORE INSERT ON local_task_sync_records
		BEGIN SELECT RAISE(ABORT, 'injected bookkeeping failure'); END`); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	remote := TaskRecord{
		TaskID: "task_remote_atomic", Title: "Remote", DurationMinutes: 30,
		Status: TaskStatusOpen, CreatedAt: created, UpdatedAt: created, Revision: 2,
	}
	if applied, err := store.ApplySyncedTask(ctx, remote); err == nil || applied {
		t.Fatalf("apply = %v, %v; want failed atomic apply", applied, err)
	}
	if _, err := store.GetTask(ctx, remote.TaskID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("failed bookkeeping left task state behind: %v", err)
	}
}
