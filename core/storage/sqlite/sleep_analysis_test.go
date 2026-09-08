package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"non24.app/core/recompute"
)

func TestAnalysisJournalRecoversInterruptedRunAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-analysis.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	input, err := s.ReadSleepAnalysisInput(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (SleepAnalysisJournal{Store: s}).Begin(ctx, recompute.Run{Inputs: recompute.Fingerprint(input.Fingerprint), State: recompute.StateRunning, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	j := SleepAnalysisJournal{Store: s}
	if count, err := (recompute.Orchestrator{Journal: j}).Recover(ctx, now.Add(time.Minute)); err != nil || count != 1 {
		t.Fatalf("recovery = %d, %v", count, err)
	}
	if _, found, err := j.LastCompleted(ctx); err != nil || found {
		t.Fatal("interrupted run was treated as completed")
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM local_recompute_runs WHERE state='interrupted' AND payload_json NOT LIKE ?`, "%"+input.Fingerprint+"%").Scan(&count); err != nil || count != 1 {
		t.Fatal("interrupted journal did not retain a sanitized state")
	}
}

func TestAnalysisJournalIsBoundedAndErasureClearsEvenEmptyProjection(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	input, err := s.ReadSleepAnalysisInput(ctx)
	if err != nil {
		t.Fatal(err)
	}
	j := SleepAnalysisJournal{Store: s}
	for i := 0; i < 205; i++ {
		run := recompute.Run{Inputs: recompute.Fingerprint(input.Fingerprint), StartedAt: now, State: recompute.StateRunning}
		id, err := j.Begin(ctx, run)
		if err != nil {
			t.Fatal(err)
		}
		if err := j.Fail(ctx, id, now, "synthetic health text must not persist"); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM local_recompute_runs`).Scan(&count); err != nil || count != 200 {
		t.Fatalf("unbounded journal: %d %v", count, err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM local_recompute_runs WHERE payload_json LIKE '%synthetic health%'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("journal retained private error text")
	}
	value := SleepAnalysis{Inputs: input.Fingerprint, AsOf: now, ValidUntil: now.Add(time.Minute), Status: "empty"}
	if err := s.SaveSleepAnalysis(ctx, value); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAllSleepData(ctx); err != nil {
		t.Fatal(err)
	}
	input, err = s.ReadSleepAnalysisInput(ctx)
	if err != nil || input.Cached != nil {
		t.Fatal("empty projection was not erased")
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM local_recompute_runs`).Scan(&count); err != nil || count != 0 {
		t.Fatal("erase left analysis journal")
	}
}

func TestAnalysisPublicationRejectsChangedEvidenceAndCorruptCacheRecovers(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	before, err := s.ReadSleepAnalysisInput(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row := testSleepObservation("obs_synthetic_analysis", now.Add(-10*time.Hour), now.Add(-2*time.Hour))
	if err := s.AppendSleepObservation(ctx, row); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSleepAnalysis(ctx, SleepAnalysis{Inputs: before.Fingerprint, AsOf: now, ValidUntil: now.Add(time.Minute)}); !errors.Is(err, ErrStaleAnalysis) {
		t.Fatalf("stale publish accepted: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO local_sleep_analysis VALUES(1,'broken-json')`); err != nil {
		t.Fatal(err)
	}
	input, err := s.ReadSleepAnalysisInput(ctx)
	if err != nil || input.Cached != nil || len(input.Snapshot.EffectiveSessions) != 1 {
		t.Fatalf("corrupt derived cache blocked evidence: %v", err)
	}
}
