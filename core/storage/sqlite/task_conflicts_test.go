package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func taskConflictFixture(t *testing.T, s *Store) (TaskRecord, TaskRecord) {
	t.Helper()
	ctx := t.Context()
	at := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	base := syncPullTestTask("task_shared", 1, at)
	if _, err := s.ApplySyncedTask(ctx, base); err != nil {
		t.Fatal(err)
	}
	local := base
	local.Revision = 2
	local.Title = "Local synthetic intention"
	local.UpdatedAt = at.Add(time.Minute)
	if err := s.UpdateTask(ctx, local, 1); err != nil {
		t.Fatal(err)
	}
	remote := local
	remote.Title = "Remote synthetic intention"
	remote.DurationMinutes = 90
	if _, err := s.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 2, Records: []SyncPullRecord{SyncPullTask{Task: remote}}}); err != nil {
		t.Fatal(err)
	}
	return local, remote
}
func TestTaskConflictPersistsAndDoesNotBlockOtherSync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	local, remote := taskConflictFixture(t, s)
	unrelated := syncPullTestTask("task_other", 1, remote.CreatedAt)
	if err := s.AddTask(t.Context(), unrelated); err != nil {
		t.Fatal(err)
	}
	pending, err := s.PendingTaskSyncRecords(t.Context(), 100)
	if err != nil || len(pending) != 1 || pending[0].TaskID != unrelated.TaskID {
		t.Fatal("conflict blocked unrelated uploads or was uploaded")
	}
	if err := s.SetTaskStatus(t.Context(), local.TaskID, TaskStatusDone, local.Revision); !errors.Is(err, ErrTaskNeedsReview) {
		t.Fatalf("review bypassed by status edit: %v", err)
	}
	edit := local
	edit.Revision++
	if err := s.UpdateTask(t.Context(), edit, local.Revision); !errors.Is(err, ErrTaskNeedsReview) {
		t.Fatalf("review bypassed by edit: %v", err)
	}
	planned, _, err := s.OpenDomainTasks(t.Context(), "UTC")
	if err != nil || len(planned) != 1 || string(planned[0].ID) != unrelated.TaskID {
		t.Fatal("conflicted task was offered for placement")
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	reviews, err := s.ListTaskSyncConflicts(t.Context())
	if err != nil || len(reviews) != 1 || reviews[0].Local.Title != local.Title || reviews[0].Downloaded[0].Task.Title != remote.Title {
		t.Fatal("restart lost a reviewed version")
	}
	if cursor, err := s.SleepSyncCursor(t.Context()); err != nil || cursor != 2 {
		t.Fatal("cursor did not advance with durable conflict")
	}
}
func TestTaskResolutionChecksAllVersionsAndCreatesNewRevision(t *testing.T) {
	s, ctx := newSyncPullTestStore(t)
	local, remote := taskConflictFixture(t, s)
	reviews, err := s.ListTaskSyncConflicts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	review := reviews[0]
	newer := remote
	newer.Revision = 3
	newer.Title = "New remote intention"
	if _, err := s.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 3, Records: []SyncPullRecord{SyncPullTask{Task: newer}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveTaskSyncConflict(ctx, local.TaskID, review.ReviewToken, "local", remote.UpdatedAt); !errors.Is(err, ErrTaskReviewChanged) {
		t.Fatalf("stale choice accepted: %v", err)
	}
	reviews, err = s.ListTaskSyncConflicts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	review = reviews[0]
	chosen := review.Downloaded[0]
	result, err := s.ResolveTaskSyncConflict(ctx, local.TaskID, review.ReviewToken, chosen.ChoiceID, remote.UpdatedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Title != chosen.Task.Title || result.Result.Revision != 4 || !result.Result.CreatedAt.Equal(local.CreatedAt) {
		t.Fatal("resolution changed chosen fields or reused a revision")
	}
	pending, err := s.PendingTaskSyncRecords(ctx, 100)
	if err != nil || len(pending) != 1 || pending[0].RecordID != "task_shared_r4" {
		t.Fatal("resolved version not queued")
	}
	for _, record := range []TaskRecord{remote, newer} {
		if _, err := s.ApplySyncedTask(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := s.TaskSyncConflictCount(ctx); err != nil || count != 0 {
		t.Fatal("reviewed payload replay reopened conflict")
	}
	history, err := s.ListTaskSyncResolutions(ctx)
	if err != nil || len(history) != 1 || len(history[0].Before.Downloaded) != 2 {
		t.Fatal("resolution history lost rejected versions")
	}
	if _, err := s.ResolveTaskSyncConflict(ctx, local.TaskID, review.ReviewToken, "local", remote.UpdatedAt); !errors.Is(err, ErrTaskReviewChanged) {
		t.Fatal("resolution replay was accepted")
	}
}
func TestTaskConflictOlderThanUnsentHeadIsNotDropped(t *testing.T) {
	s, ctx := newSyncPullTestStore(t)
	at := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	base := syncPullTestTask("task_branch", 1, at)
	if _, err := s.ApplySyncedTask(ctx, base); err != nil {
		t.Fatal(err)
	}
	local := base
	for revision := 2; revision <= 4; revision++ {
		local.Revision = revision
		local.Title = "More local edits"
		if err := s.UpdateTask(ctx, local, revision-1); err != nil {
			t.Fatal(err)
		}
	}
	remote := base
	remote.Revision = 2
	remote.Title = "Concurrent remote edit"
	if _, err := s.ApplySyncedTask(ctx, remote); err != nil {
		t.Fatal(err)
	}
	reviews, err := s.ListTaskSyncConflicts(ctx)
	if err != nil || len(reviews) != 1 {
		t.Fatal("older concurrent branch silently skipped")
	}
	resolved, err := s.ResolveTaskSyncConflict(ctx, local.TaskID, reviews[0].ReviewToken, "local", at.Add(time.Hour))
	if err != nil || resolved.Result.Revision != 5 || resolved.Result.Title != local.Title {
		t.Fatalf("keep-local failed: %v", err)
	}
}
func TestTaskDeletionErasesConflictAndResolutionPayloads(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "resolved"}[resolved], func(t *testing.T) {
			s, ctx := newSyncPullTestStore(t)
			local, remote := taskConflictFixture(t, s)
			if resolved {
				reviews, _ := s.ListTaskSyncConflicts(ctx)
				result, err := s.ResolveTaskSyncConflict(ctx, local.TaskID, reviews[0].ReviewToken, "local", remote.UpdatedAt)
				if err != nil {
					t.Fatal(err)
				}
				local = result.Result
			}
			if err := s.DeleteTask(ctx, local.TaskID, local.Revision); err != nil {
				t.Fatal(err)
			}
			assertTaskReviewErased(t, s, local.TaskID)
			if _, err := s.ApplySyncedTask(ctx, remote); err != nil {
				t.Fatal(err)
			}
			if _, err := s.GetTask(ctx, local.TaskID); !errors.Is(err, ErrTaskNotFound) {
				t.Fatal("deleted task resurrected")
			}
		})
	}
}
func TestTaskTombstoneWinsOverConflictInSamePage(t *testing.T) {
	s, ctx := newSyncPullTestStore(t)
	local, remote := taskConflictFixture(t, s)
	if _, err := s.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 3, Records: []SyncPullRecord{SyncPullTask{Task: remote}, SyncPullTombstone{RecordID: "task_shared_r1", RecordKind: "task"}}}); err != nil {
		t.Fatal(err)
	}
	assertTaskReviewErased(t, s, local.TaskID)
}
func assertTaskReviewErased(t *testing.T, s *Store, id string) {
	t.Helper()
	for _, table := range []string{"local_task_sync_conflicts", "local_task_sync_fingerprints", "local_task_sync_resolutions"} {
		var count int
		if err := s.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table+" WHERE task_id=?", id).Scan(&count); err != nil || count != 0 {
			t.Fatalf("task review data remains in %s", table)
		}
	}
}
