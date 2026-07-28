package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newSyncPullTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "sync-pull.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, context.Background()
}

func TestApplySyncPullPageHandlesMixedOrderingLWWOrphansAndReplay(t *testing.T) {
	store, ctx := newSyncPullTestStore(t)
	start := time.Date(2026, 7, 28, 4, 0, 0, 0, time.UTC)
	observation := testSleepObservation("obs_pull_mixed_01", start, start.Add(8*time.Hour))
	correctedEnd := start.Add(7*time.Hour + 30*time.Minute)
	correction := SleepCorrectionRecord{
		CorrectionID:        "corr_pull_mixed_01",
		TargetObservationID: observation.ObservationID,
		CreatedAt:           start.Add(9 * time.Hour),
		Reason:              CorrectionReasonUserEdit,
		Changes:             SleepCorrectionChanges{EndAt: &correctedEnd},
	}
	orphanEnd := start.Add(6 * time.Hour)
	orphan := SleepCorrectionRecord{
		CorrectionID:        "corr_pull_orphan_01",
		TargetObservationID: "obs_pull_erased_01",
		CreatedAt:           start.Add(10 * time.Hour),
		Reason:              CorrectionReasonUserEdit,
		Changes:             SleepCorrectionChanges{EndAt: &orphanEnd},
	}
	taskV2 := syncPullTestTask("task_pull_mixed_01", 2, start.Add(2*time.Hour))
	taskV1 := taskV2
	taskV1.Revision = 1
	taskV1.Title = "Stale title"
	taskV1.UpdatedAt = start.Add(time.Hour)

	page := SyncPullPage{
		Cursor: 12,
		Records: []SyncPullRecord{
			// Corrections may precede their observation in sequence order.
			SyncPullCorrection{Correction: correction},
			SyncPullTask{Task: taskV2},
			SyncPullObservation{Observation: observation},
			SyncPullTask{Task: taskV1},
			SyncPullCorrection{Correction: orphan},
		},
	}
	result, err := store.ApplySyncPullPage(ctx, page)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied != 3 || result.Skipped != 2 || result.TombstonesApplied != 0 {
		t.Fatalf("result = %#v, want 3 applied and 2 skipped", result)
	}
	cursor, err := store.SleepSyncCursor(ctx)
	if err != nil || cursor != page.Cursor {
		t.Fatalf("cursor = %d, %v; want %d", cursor, err, page.Cursor)
	}
	observations, err := store.ListSleepObservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	corrections, err := store.ListSleepCorrections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || len(corrections) != 1 || corrections[0].CorrectionID != correction.CorrectionID {
		t.Fatalf("sleep records = observations %#v, corrections %#v", observations, corrections)
	}
	task, err := store.GetTask(ctx, taskV2.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Revision != 2 || task.Title != taskV2.Title {
		t.Fatalf("task LWW state = %#v", task)
	}
	if pending, err := store.UnpushedSleepSyncRecords(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("pulled sleep records queued for push: %#v, %v", pending, err)
	}
	if pending, err := store.UnpushedTaskSyncRecords(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("pulled task queued for push: %#v, %v", pending, err)
	}

	replayed, err := store.ApplySyncPullPage(ctx, page)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Applied != 0 || replayed.Skipped != 5 || replayed.TombstonesApplied != 0 {
		t.Fatalf("idempotent replay result = %#v", replayed)
	}
}

func TestApplySyncPullPageRollsBackRecordsAndCursorOnCursorFailure(t *testing.T) {
	store, ctx := newSyncPullTestStore(t)
	if err := store.SaveSleepSyncCursor(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER fail_sync_pull_cursor
		BEFORE INSERT ON local_sync_state
		WHEN NEW.key = 'sleep_sync_cursor'
		BEGIN SELECT RAISE(ABORT, 'injected cursor failure'); END`); err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 7, 28, 5, 0, 0, 0, time.UTC)
	observation := testSleepObservation("obs_pull_rollback_01", start, start.Add(8*time.Hour))
	task := syncPullTestTask("task_pull_rollback_01", 1, start.Add(time.Hour))
	page := SyncPullPage{
		Cursor: 5,
		Records: []SyncPullRecord{
			SyncPullObservation{Observation: observation},
			SyncPullTask{Task: task},
		},
	}
	if result, err := store.ApplySyncPullPage(ctx, page); err == nil || result != (SyncPullPageResult{}) {
		t.Fatalf("failed page = %#v, %v; want rolled-back error", result, err)
	}
	cursor, err := store.SleepSyncCursor(ctx)
	if err != nil || cursor != 4 {
		t.Fatalf("cursor after rollback = %d, %v; want 4", cursor, err)
	}
	observations, err := store.ListSleepObservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 0 {
		t.Fatalf("failed page left observations: %#v", observations)
	}
	if _, err := store.GetTask(ctx, task.TaskID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("failed page left task state: %v", err)
	}
	var tracked int
	if err := store.db.QueryRowContext(ctx,
		`SELECT (SELECT COUNT(*) FROM local_sleep_sync_records)
			+ (SELECT COUNT(*) FROM local_task_sync_records)`).Scan(&tracked); err != nil {
		t.Fatal(err)
	}
	if tracked != 0 {
		t.Fatalf("failed page left %d sync tracking rows", tracked)
	}

	if _, err := store.db.ExecContext(ctx, `DROP TRIGGER fail_sync_pull_cursor`); err != nil {
		t.Fatal(err)
	}
	result, err := store.ApplySyncPullPage(ctx, page)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied != 2 {
		t.Fatalf("retry result = %#v, want 2 applied", result)
	}
}

func TestApplySyncPullPageRejectsRegressedAndUnboundedPages(t *testing.T) {
	store, ctx := newSyncPullTestStore(t)
	if err := store.SaveSleepSyncCursor(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 7}); err == nil {
		t.Fatal("regressed cursor should be rejected")
	}
	oversized := make([]SyncPullRecord, MaxSyncPullPageSize+1)
	if _, err := store.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 9, Records: oversized}); err == nil {
		t.Fatal("unbounded pull page should be rejected")
	}
	cursor, err := store.SleepSyncCursor(ctx)
	if err != nil || cursor != 8 {
		t.Fatalf("cursor after rejected pages = %d, %v; want 8", cursor, err)
	}
}

func TestApplySyncPullPageBatchesMixedTombstonesIntoOneCompaction(t *testing.T) {
	store, ctx := newSyncPullTestStore(t)
	start := time.Date(2026, 7, 28, 6, 0, 0, 0, time.UTC)
	first := testSleepObservation("obs_pull_erase_01", start, start.Add(8*time.Hour))
	second := testSleepObservation("obs_pull_erase_02", start.Add(24*time.Hour), start.Add(32*time.Hour))
	correctedEnd := start.Add(7 * time.Hour)
	correction := SleepCorrectionRecord{
		CorrectionID:        "corr_pull_erase_01",
		TargetObservationID: first.ObservationID,
		CreatedAt:           start.Add(9 * time.Hour),
		Reason:              CorrectionReasonUserEdit,
		Changes:             SleepCorrectionChanges{EndAt: &correctedEnd},
	}
	task := syncPullTestTask("task_pull_erase_01", 3, start.Add(10*time.Hour))
	if inserted, err := store.InsertSyncedSleepObservation(ctx, first); err != nil || !inserted {
		t.Fatalf("seed first observation = %v, %v", inserted, err)
	}
	if inserted, err := store.InsertSyncedSleepObservation(ctx, second); err != nil || !inserted {
		t.Fatalf("seed second observation = %v, %v", inserted, err)
	}
	if inserted, err := store.InsertSyncedSleepCorrection(ctx, correction); err != nil || !inserted {
		t.Fatalf("seed correction = %v, %v", inserted, err)
	}
	if applied, err := store.ApplySyncedTask(ctx, task); err != nil || !applied {
		t.Fatalf("seed task = %v, %v", applied, err)
	}
	for _, recordID := range []string{first.ObservationID, second.ObservationID, correction.CorrectionID, taskRevisionRecordID(task.TaskID, task.Revision)} {
		if _, err := store.db.ExecContext(ctx,
			`INSERT INTO local_sleep_erasures(record_id, erased_at) VALUES(?, ?)`,
			recordID, formatSQLiteTime(start.Add(11*time.Hour))); err != nil {
			t.Fatal(err)
		}
	}

	compactions := 0
	page := SyncPullPage{
		Cursor: 20,
		Records: []SyncPullRecord{
			SyncPullTombstone{RecordID: correction.CorrectionID},
			SyncPullTombstone{RecordID: taskRevisionRecordID(task.TaskID, task.Revision)},
			SyncPullTombstone{RecordID: first.ObservationID},
			SyncPullTombstone{RecordID: second.ObservationID},
		},
	}
	result, err := store.applySyncPullPage(ctx, page, func(ctx context.Context) error {
		compactions++
		return store.compactDeletedData(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TombstonesApplied != 4 || compactions != 1 {
		t.Fatalf("result = %#v, compactions = %d; want 4 erasures and 1 compaction", result, compactions)
	}
	observations, err := store.ListSleepObservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	corrections, err := store.ListSleepCorrections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 0 || len(corrections) != 0 {
		t.Fatalf("tombstones left sleep data: observations %#v, corrections %#v", observations, corrections)
	}
	if _, err := store.GetTask(ctx, task.TaskID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("tombstone left task state: %v", err)
	}
	if pending, err := store.PendingSyncErasures(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("incoming tombstones left outbound erasures: %#v, %v", pending, err)
	}
	if pending, err := syncCompactionPending(ctx, store.db); err != nil || pending {
		t.Fatalf("compaction pending after success = %v, %v", pending, err)
	}

	replayed, err := store.applySyncPullPage(ctx, page, func(context.Context) error {
		compactions++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.TombstonesApplied != 0 || compactions != 2 {
		t.Fatalf("idempotent tombstone replay = %#v, compactions = %d", replayed, compactions)
	}
}

func TestApplySyncPullPageRetainsPendingCompactionAfterPostCommitFailure(t *testing.T) {
	store, ctx := newSyncPullTestStore(t)
	start := time.Date(2026, 7, 28, 7, 0, 0, 0, time.UTC)
	observation := testSleepObservation("obs_pull_compact_01", start, start.Add(8*time.Hour))
	if inserted, err := store.InsertSyncedSleepObservation(ctx, observation); err != nil || !inserted {
		t.Fatalf("seed observation = %v, %v", inserted, err)
	}

	result, err := store.applySyncPullPage(ctx, SyncPullPage{
		Cursor:  30,
		Records: []SyncPullRecord{SyncPullTombstone{RecordID: observation.ObservationID}},
	}, func(context.Context) error {
		return errors.New("injected compaction failure")
	})
	if err == nil || result.TombstonesApplied != 1 {
		t.Fatalf("post-commit result = %#v, %v", result, err)
	}
	cursor, cursorErr := store.SleepSyncCursor(ctx)
	if cursorErr != nil || cursor != 30 {
		t.Fatalf("committed page cursor = %d, %v; want 30", cursor, cursorErr)
	}
	if pending, pendingErr := syncCompactionPending(ctx, store.db); pendingErr != nil || !pending {
		t.Fatalf("durable pending compaction = %v, %v; want true", pending, pendingErr)
	}

	compactions := 0
	_, err = store.applySyncPullPage(ctx, SyncPullPage{Cursor: 30}, func(context.Context) error {
		compactions++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if compactions != 1 {
		t.Fatalf("pending compaction retries = %d, want 1", compactions)
	}
	if pending, pendingErr := syncCompactionPending(ctx, store.db); pendingErr != nil || pending {
		t.Fatalf("pending compaction after retry = %v, %v", pending, pendingErr)
	}
}

func syncPullTestTask(taskID string, revision int, updatedAt time.Time) TaskRecord {
	return TaskRecord{
		TaskID:          taskID,
		Title:           "Remote task",
		DurationMinutes: 30,
		Status:          TaskStatusOpen,
		CreatedAt:       updatedAt.Add(-time.Hour),
		Revision:        revision,
		UpdatedAt:       updatedAt,
	}
}
