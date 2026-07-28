package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func newTaskSyncStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, context.Background()
}

func TestTaskRevisionsFlowThroughSyncBookkeeping(t *testing.T) {
	store, ctx := newTaskSyncStore(t)
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	task := TaskRecord{TaskID: "task_paperwork_01", Title: "File paperwork", DurationMinutes: 45, Status: TaskStatusOpen, CreatedAt: created}
	if err := store.AddTask(ctx, task); err != nil {
		t.Fatal(err)
	}

	// Revision 1 is unpushed with a contract-shaped payload.
	unpushed, err := store.UnpushedTaskSyncRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(unpushed) != 1 || unpushed[0].RecordID != "task_paperwork_01_r1" {
		t.Fatalf("unpushed = %+v", unpushed)
	}
	var payload map[string]any
	if err := json.Unmarshal(unpushed[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["revision"] != float64(1) || payload["task_id"] != "task_paperwork_01" || payload["updated_at"] == nil {
		t.Fatalf("payload = %v", payload)
	}

	count, err := store.PendingTaskSyncRecordCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("pending task count = %d, want 1", count)
	}
	page, err := store.PendingTaskSyncRecords(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].RecordID != unpushed[0].RecordID {
		t.Fatalf("pending task page = %+v", page)
	}
	if _, err := store.PendingTaskSyncRecords(ctx, 0); err == nil {
		t.Fatal("zero task page limit should be rejected")
	}
	if _, err := store.PendingTaskSyncRecords(ctx, MaxTaskSyncPageSize+1); err == nil {
		t.Fatal("oversized task page limit should be rejected")
	}

	// Marking pushed empties the queue; an edit bumps the revision and queues
	// exactly the new revision.
	if err := store.MarkTaskSyncRecordsPushed(ctx, unpushed, created.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if unpushed, err = store.UnpushedTaskSyncRecords(ctx); err != nil || len(unpushed) != 0 {
		t.Fatalf("after push: %v %v", unpushed, err)
	}
	task.Title = "File paperwork (rescoped)"
	task.Revision = 2
	task.UpdatedAt = created.Add(2 * time.Minute)
	if err := store.UpdateTask(ctx, task, 1); err != nil {
		t.Fatal(err)
	}
	if unpushed, err = store.UnpushedTaskSyncRecords(ctx); err != nil || len(unpushed) != 1 || unpushed[0].RecordID != "task_paperwork_01_r2" {
		t.Fatalf("after edit: %+v %v", unpushed, err)
	}
	if err := store.MarkTaskSyncRecordsPushed(ctx, unpushed, created.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	// Deleting the task enqueues erasures for BOTH pushed revisions in the
	// shared erasure outbox, inside the delete transaction.
	if err := store.DeleteTask(ctx, "task_paperwork_01", 2); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingSyncErasures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 || pending[0] != "task_paperwork_01_r1" || pending[1] != "task_paperwork_01_r2" {
		t.Fatalf("pending erasures = %v", pending)
	}
}

func TestNeverPushedTaskDeletesWithoutErasures(t *testing.T) {
	store, ctx := newTaskSyncStore(t)
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	if err := store.AddTask(ctx, TaskRecord{TaskID: "task_local_only", Title: "Never synced", DurationMinutes: 30, Status: TaskStatusOpen, CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTask(ctx, "task_local_only", 1); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingSyncErasures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("never-pushed task must not enqueue erasures: %v", pending)
	}
}

func TestApplySyncedTaskIsLastWriterWinsAndNotRepushed(t *testing.T) {
	store, ctx := newTaskSyncStore(t)
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	remote := TaskRecord{
		TaskID: "task_from_phone", Title: "Call clinic", DurationMinutes: 20,
		Status: TaskStatusOpen, CreatedAt: created, Revision: 2, UpdatedAt: created.Add(time.Hour),
	}

	// Unknown task: created, and its revision is not queued for re-push.
	applied, err := store.ApplySyncedTask(ctx, remote)
	if err != nil || !applied {
		t.Fatalf("apply = %v %v", applied, err)
	}
	unpushed, err := store.UnpushedTaskSyncRecords(ctx)
	if err != nil || len(unpushed) != 0 {
		t.Fatalf("pulled task must not be re-pushed: %+v %v", unpushed, err)
	}

	// Stale revision: skipped. Newer revision: applied.
	stale := remote
	stale.Revision = 1
	stale.Title = "Old title"
	if applied, err = store.ApplySyncedTask(ctx, stale); err != nil || applied {
		t.Fatalf("stale apply = %v %v", applied, err)
	}
	newer := remote
	newer.Revision = 3
	newer.Status = TaskStatusDone
	if applied, err = store.ApplySyncedTask(ctx, newer); err != nil || !applied {
		t.Fatalf("newer apply = %v %v", applied, err)
	}
	tasks, err := store.ListTasks(ctx)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks = %+v %v", tasks, err)
	}
	if tasks[0].Status != TaskStatusDone || tasks[0].Title != "Call clinic" || tasks[0].Revision != 3 {
		t.Fatalf("LWW state = %+v", tasks[0])
	}

	// A pulled tombstone for any revision deletes the local task, idempotently,
	// without enqueueing new erasures.
	erased, err := store.EraseSyncedTaskRecord(ctx, "task_from_phone_r2")
	if err != nil || !erased {
		t.Fatalf("erase = %v %v", erased, err)
	}
	if erased, err = store.EraseSyncedTaskRecord(ctx, "task_from_phone_r2"); err != nil || erased {
		t.Fatalf("second erase = %v %v", erased, err)
	}
	pending, err := store.PendingSyncErasures(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("tombstone application must not re-enqueue: %v %v", pending, err)
	}
	if _, err := store.ListTasks(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTaskRevisionIDRouting(t *testing.T) {
	if !IsTaskRevisionID("task_abc123_r2") {
		t.Fatal("task revision id not recognized")
	}
	for _, id := range []string{"obs_sleep_01", "corr_sleep_01", "task_abc123", "unrelated_r2"} {
		if IsTaskRevisionID(id) {
			t.Fatalf("%q wrongly routed as task revision", id)
		}
	}
}
