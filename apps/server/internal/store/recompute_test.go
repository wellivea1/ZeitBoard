package store_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"non24.app/core/recompute"
	"non24.app/server/internal/store"
)

var journalNow = time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)

func newJournal(t *testing.T) (store.RecomputeJournal, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "journal.db")
	st, err := store.Open(path, bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return store.RecomputeJournal{Store: st}, path
}

func TestJournalRoundTripsARun(t *testing.T) {
	journal, _ := newJournal(t)
	ctx := context.Background()

	if _, ok, err := journal.LastCompleted(ctx); err != nil || ok {
		t.Fatalf("a fresh journal reported a completed run: ok=%v err=%v", ok, err)
	}

	id, err := journal.Begin(ctx, recompute.Run{
		Reason:           recompute.ReasonEvidence,
		Inputs:           "input-fingerprint",
		Content:          "content-fingerprint",
		StartedAt:        journalNow,
		ContentChangedAt: journalNow,
		ValidUntil:       journalNow.Add(6 * time.Hour),
	})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	// A run that has begun is not yet a run that finished.
	if _, ok, err := journal.LastCompleted(ctx); err != nil || ok {
		t.Fatalf("an unfinished run was reported as completed: ok=%v err=%v", ok, err)
	}

	if err := journal.Complete(ctx, recompute.Run{
		ID:               id,
		CompletedAt:      journalNow.Add(time.Second),
		ContentChangedAt: journalNow,
		ValidUntil:       journalNow.Add(6 * time.Hour),
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	last, ok, err := journal.LastCompleted(ctx)
	if err != nil || !ok {
		t.Fatalf("last completed: ok=%v err=%v", ok, err)
	}
	if last.Inputs != "input-fingerprint" || last.Content != "content-fingerprint" {
		t.Errorf("fingerprints did not survive the round trip: %+v", last)
	}
	if !last.ContentChangedAt.Equal(journalNow) {
		t.Errorf("content changed at = %s, want %s", last.ContentChangedAt, journalNow)
	}
	if !last.ValidUntil.Equal(journalNow.Add(6 * time.Hour)) {
		t.Errorf("valid until = %s, want the expiry it was given", last.ValidUntil)
	}
	if last.Reason != recompute.ReasonEvidence {
		t.Errorf("reason = %q, want evidence", last.Reason)
	}
}

// TestFingerprintsAreNotReadableInTheFile. A digest is one-way, but it still
// answers "did they sleep at 04:12 on Tuesday?" for anyone willing to guess, and
// the reason this database is encrypted at rest is that a copy of it answers
// nothing.
func TestFingerprintsAreNotReadableInTheFile(t *testing.T) {
	journal, path := newJournal(t)
	ctx := context.Background()

	const canary = "b57f0c9d1e2a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4"
	id, err := journal.Begin(ctx, recompute.Run{
		Reason: recompute.ReasonStartup, Inputs: canary, Content: canary,
		StartedAt: journalNow,
	})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := journal.Complete(ctx, recompute.Run{ID: id, CompletedAt: journalNow}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		contents, err := os.ReadFile(path + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", path+suffix, err)
		}
		if bytes.Contains(contents, []byte(canary)) {
			t.Errorf("the input fingerprint is stored in the clear in %s", path+suffix)
		}
	}
}

func TestFailureIsRecordedWithItsReason(t *testing.T) {
	journal, _ := newJournal(t)
	ctx := context.Background()

	id, err := journal.Begin(ctx, recompute.Run{
		Reason: recompute.ReasonEvidence, Inputs: "in", Content: "out",
		StartedAt: journalNow,
	})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := journal.Fail(ctx, id, journalNow.Add(time.Second), "database is locked"); err != nil {
		t.Fatalf("fail: %v", err)
	}

	runs, err := journal.RecomputeRuns(ctx, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].State != recompute.StateFailed {
		t.Errorf("state = %q, want failed", runs[0].State)
	}
	if runs[0].Error != "database is locked" {
		t.Errorf("error = %q, want the message it failed with", runs[0].Error)
	}
	// The fingerprints must survive the re-seal that writing the message needs.
	if runs[0].Inputs != "in" || runs[0].Content != "out" {
		t.Errorf("recording the failure lost the fingerprints: %+v", runs[0])
	}
	if _, ok, err := journal.LastCompleted(ctx); err != nil || ok {
		t.Errorf("a failed run was reported as the last completed one: ok=%v err=%v", ok, err)
	}
}

// TestARunLeftOpenByACrashIsClosedOut. The recovery is the fingerprint
// comparison that follows; this makes the crash visible in the record.
func TestARunLeftOpenByACrashIsClosedOut(t *testing.T) {
	journal, _ := newJournal(t)
	ctx := context.Background()

	if _, err := journal.Begin(ctx, recompute.Run{
		Reason: recompute.ReasonEvidence, Inputs: "in", Content: "out",
		StartedAt: journalNow,
	}); err != nil {
		t.Fatalf("begin: %v", err)
	}

	count, err := journal.MarkInterrupted(ctx, journalNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if count != 1 {
		t.Errorf("marked %d runs, want 1", count)
	}

	runs, err := journal.RecomputeRuns(ctx, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if runs[0].State != recompute.StateInterrupted {
		t.Errorf("state = %q, want interrupted", runs[0].State)
	}

	// Idempotent: a second startup finds nothing left open.
	again, err := journal.MarkInterrupted(ctx, journalNow.Add(2*time.Hour))
	if err != nil || again != 0 {
		t.Errorf("second sweep marked %d runs (err %v), want 0", again, err)
	}
}

// TestTheJournalStaysBounded: this is an operational record on somebody's own
// machine, not an audit trail with a retention obligation.
func TestTheJournalStaysBounded(t *testing.T) {
	journal, _ := newJournal(t)
	ctx := context.Background()

	for i := 0; i < store.MaxRecomputeRunHistory+25; i++ {
		at := journalNow.Add(time.Duration(i) * time.Minute)
		id, err := journal.Begin(ctx, recompute.Run{
			Reason: recompute.ReasonHeartbeat, Inputs: "in", Content: "out",
			StartedAt: at, ContentChangedAt: journalNow,
		})
		if err != nil {
			t.Fatalf("begin %d: %v", i, err)
		}
		if err := journal.Complete(ctx, recompute.Run{
			ID: id, CompletedAt: at, ContentChangedAt: journalNow,
		}); err != nil {
			t.Fatalf("complete %d: %v", i, err)
		}
	}

	runs, err := journal.RecomputeRuns(ctx, store.MaxRecomputeRunHistory)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != store.MaxRecomputeRunHistory {
		t.Errorf("kept %d runs, want the %d-row bound", len(runs), store.MaxRecomputeRunHistory)
	}
	// Pruning must never take the row the orchestrator actually reads.
	if _, ok, err := journal.LastCompleted(ctx); err != nil || !ok {
		t.Errorf("pruning removed the newest completed run: ok=%v err=%v", ok, err)
	}
}
