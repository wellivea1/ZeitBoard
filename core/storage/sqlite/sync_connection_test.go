package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestEnrollmentAndReconciliationAreAtomicAndSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-enrollment.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first := SyncConnection{Enabled: true, BackendURL: "https://old.example.test", DeviceID: "device_old"}
	if err := s.SaveSyncEnrollment(ctx, first, "synthetic-old"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSleepSyncCursor(ctx, 41); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO local_sync_seen VALUES('obs_synthetic')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER synthetic_fail_scope BEFORE DELETE ON local_sync_seen BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	second := SyncConnection{Enabled: true, BackendURL: "https://new.example.test", DeviceID: "device_new"}
	if err := s.SaveSyncEnrollment(ctx, second, "synthetic-new"); err == nil {
		t.Fatal("failed scope update committed enrollment")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	current, token, err := s.LoadSyncConnection(ctx)
	if err != nil || current.DeviceID != first.DeviceID || token != "synthetic-old" {
		t.Fatal("failed enrollment damaged old credential")
	}
	if cursor, err := s.SleepSyncCursor(ctx); err != nil || cursor != 41 {
		t.Fatal("failed enrollment damaged cursor")
	}
	if _, err := s.db.Exec(`DROP TRIGGER synthetic_fail_scope`); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncEnrollment(ctx, second, "synthetic-new"); err != nil {
		t.Fatal(err)
	}
	if cursor, err := s.SleepSyncCursor(ctx); err != nil || cursor != 0 {
		t.Fatal("new enrollment did not reset cursor")
	}
	second.Enabled = false
	if err := s.UpdateSyncConnection(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, token, err := s.LoadSyncConnection(ctx); err != nil || token != "" {
		t.Fatal("disable retained credential")
	}
	second.Enabled = true
	if err := s.UpdateSyncConnection(ctx, second); err == nil {
		t.Fatal("stale status update revived disabled enrollment")
	}
}

func TestReconciliationRequeuesOnlyRecordsMissingFromCompleteDownload(t *testing.T) {
	s, ctx := newSyncPullTestStore(t)
	at := time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC)
	kept := testSleepObservation("obs_kept", at, at.Add(8*time.Hour))
	missing := testSleepObservation("obs_missing", at.Add(24*time.Hour), at.Add(32*time.Hour))
	for _, record := range []SleepObservationRecord{kept, missing} {
		if err := s.AppendSleepObservation(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	queued, err := s.UnpushedSleepSyncRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkSleepSyncRecordsPushed(ctx, queued, at); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncEnrollment(ctx, SyncConnection{Enabled: true, BackendURL: "https://restored.example.test", DeviceID: "device_restored"}, "synthetic-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 1, Records: []SyncPullRecord{SyncPullObservation{Observation: kept}}}); err != nil {
		t.Fatal(err)
	}
	if count, err := s.PendingSleepSyncRecordCount(ctx); err != nil || count != 0 {
		t.Fatal("accepted records replayed before download finished")
	}
	if err := s.FinishSyncReconciliation(ctx); err != nil {
		t.Fatal(err)
	}
	queued, err = s.UnpushedSleepSyncRecords(ctx)
	if err != nil || len(queued) != 1 || queued[0].RecordID != missing.ObservationID {
		t.Fatal("reconciliation did not isolate missing accepted data")
	}
}

func TestDeferredCorrectionSurvivesPagesAndErasureNeverResurrects(t *testing.T) {
	s, ctx := newSyncPullTestStore(t)
	at := time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC)
	source := testSleepObservation("obs_deferred", at, at.Add(8*time.Hour))
	end := at.Add(7 * time.Hour)
	correction := SleepCorrectionRecord{CorrectionID: "cor_deferred", TargetObservationID: source.ObservationID, CreatedAt: at.Add(9 * time.Hour), Reason: CorrectionReasonUserEdit, Changes: SleepCorrectionChanges{EndAt: &end}}
	if _, err := s.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 1, Records: []SyncPullRecord{SyncPullCorrection{Correction: correction}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 2, Records: []SyncPullRecord{SyncPullObservation{Observation: source}}}); err != nil {
		t.Fatal(err)
	}
	corrections, err := s.ListSleepCorrections(ctx)
	if err != nil || len(corrections) != 1 {
		t.Fatal("cursor advance lost correction waiting for its source")
	}
	if _, err := s.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 3, Records: []SyncPullRecord{SyncPullTombstone{RecordID: source.ObservationID, RecordKind: "observation"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 4, Records: []SyncPullRecord{SyncPullObservation{Observation: source}, SyncPullCorrection{Correction: correction}}}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListSleepObservations(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatal("replayed page resurrected erased evidence")
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM local_sync_deferred_corrections`).Scan(&count); err != nil || count != 0 {
		t.Fatal("erased correction retained a deferred payload")
	}
}

func TestDeletionBeforeUploadAcknowledgmentStillQueuesErasure(t *testing.T) {
	s, ctx := newSyncPullTestStore(t)
	at := time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC)
	source := testSleepObservation("obs_inflight", at, at.Add(8*time.Hour))
	if err := s.AppendSleepObservation(ctx, source); err != nil {
		t.Fatal(err)
	}
	queued, err := s.UnpushedSleepSyncRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSleepObservation(ctx, source.ObservationID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkSleepSyncRecordsPushed(ctx, queued, at); err != nil {
		t.Fatal(err)
	}
	erasures, err := s.PendingSyncErasures(ctx)
	if err != nil || len(erasures) != 1 || erasures[0] != source.ObservationID {
		t.Fatal("inflight deleted source lost its remote erasure")
	}
	if err := s.ClearSyncErasures(ctx, erasures); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendSleepObservation(ctx, source); err == nil {
		t.Fatal("acknowledged erasure lost suppression marker")
	}
}

func TestUnsentTaskEditIsNotOverwrittenOrFalselyAcknowledged(t *testing.T) {
	s, ctx := newSyncPullTestStore(t)
	at := time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC)
	local := syncPullTestTask("task_conflict", 2, at)
	if _, err := s.ApplySyncedTask(ctx, local); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM local_task_sync_records`); err != nil {
		t.Fatal(err)
	}
	remote := local
	remote.Title = "Different synthetic edit"
	if _, err := s.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 1, Records: []SyncPullRecord{SyncPullTask{Task: remote}}}); err != nil {
		t.Fatal(err)
	}
	remote.Revision = 3
	if _, err := s.ApplySyncPullPage(ctx, SyncPullPage{Cursor: 1, Records: []SyncPullRecord{SyncPullTask{Task: remote}}}); err != nil {
		t.Fatal(err)
	}
	actual, err := s.GetTask(ctx, local.TaskID)
	if err != nil || actual.Title != local.Title || actual.Revision != 2 {
		t.Fatal("conflict lost the local task")
	}
	if count, err := s.PendingTaskSyncRecordCount(ctx); err != nil || count != 0 {
		t.Fatal("conflicted task remained uploadable")
	}
	if cursor, err := s.SleepSyncCursor(ctx); err != nil || cursor != 1 {
		t.Fatal("retained conflict prevented cursor progress")
	}
	conflicts, err := s.ListTaskSyncConflicts(ctx)
	if err != nil || len(conflicts) != 1 || len(conflicts[0].Downloaded) != 2 {
		t.Fatal("conflicting payloads were not retained")
	}
	if err := s.DeleteTask(ctx, local.TaskID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplySyncedTask(ctx, remote); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTask(ctx, local.TaskID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatal("deleted task was resurrected by a later revision")
	}
}
