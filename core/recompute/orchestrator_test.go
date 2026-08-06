package recompute_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"non24.app/core/recompute"
)

// memoryJournal is the durable half, in memory. It keeps the full history so a
// test can assert on what was recorded, not only on what is current.
type memoryJournal struct {
	runs   []recompute.Run
	nextID int64
	failOn string
}

func (j *memoryJournal) LastCompleted(context.Context) (recompute.Run, bool, error) {
	for i := len(j.runs) - 1; i >= 0; i-- {
		if j.runs[i].State == recompute.StateDone {
			return j.runs[i], true, nil
		}
	}
	return recompute.Run{}, false, nil
}

func (j *memoryJournal) Begin(_ context.Context, run recompute.Run) (int64, error) {
	if j.failOn == "begin" {
		return 0, errors.New("journal unavailable")
	}
	j.nextID++
	run.ID = j.nextID
	j.runs = append(j.runs, run)
	return run.ID, nil
}

func (j *memoryJournal) Complete(_ context.Context, run recompute.Run) error {
	for i := range j.runs {
		if j.runs[i].ID == run.ID {
			run.State = recompute.StateDone
			j.runs[i] = run
			return nil
		}
	}
	return errors.New("no such run")
}

func (j *memoryJournal) Fail(_ context.Context, id int64, at time.Time, message string) error {
	for i := range j.runs {
		if j.runs[i].ID == id {
			j.runs[i].State = recompute.StateFailed
			j.runs[i].CompletedAt = at
			j.runs[i].Error = message
			return nil
		}
	}
	return errors.New("no such run")
}

func (j *memoryJournal) MarkInterrupted(_ context.Context, at time.Time) (int, error) {
	count := 0
	for i := range j.runs {
		if j.runs[i].State == recompute.StateRunning {
			j.runs[i].State = recompute.StateInterrupted
			j.runs[i].CompletedAt = at
			count++
		}
	}
	return count, nil
}

// scriptedAnalysis returns whatever the test currently says the world looks
// like, and records every stamp it was published under.
type scriptedAnalysis struct {
	inputs  string
	content string
	expires time.Time
	fail    error

	applied []recompute.Stamp
}

func (a *scriptedAnalysis) Prepare(context.Context, time.Time) (recompute.Prepared, error) {
	return recompute.Prepared{
		Inputs:     recompute.Fingerprint(a.inputs),
		Content:    recompute.Fingerprint(a.content),
		ValidUntil: a.expires,
		Apply: func(_ context.Context, stamp recompute.Stamp) error {
			if a.fail != nil {
				return a.fail
			}
			a.applied = append(a.applied, stamp)
			return nil
		},
	}, nil
}

func newOrchestrator(analysis recompute.Analysis) (recompute.Orchestrator, *memoryJournal) {
	journal := &memoryJournal{}
	return recompute.Orchestrator{Analysis: analysis, Journal: journal}, journal
}

// TestUnchangedContentKeepsItsOriginalStamp is the correction that makes the
// whole thing honest. A projection that has not changed must not present itself
// as freshly updated, or a page reads "updated just now" over week-old evidence
// — the exact defect ADR-0031 found in the portal and only half fixed.
func TestUnchangedContentKeepsItsOriginalStamp(t *testing.T) {
	analysis := &scriptedAnalysis{inputs: "in-1", content: "out-1"}
	orchestrator, _ := newOrchestrator(analysis)
	ctx := context.Background()

	first, err := orchestrator.Run(ctx, recompute.ReasonStartup, base)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if !first.ContentChanged {
		t.Error("the first run should count as a change; there was nothing before it")
	}

	sixHoursLater := base.Add(6 * time.Hour)
	second, err := orchestrator.Run(ctx, recompute.ReasonFreshnessExpiry, sixHoursLater)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.ContentChanged {
		t.Error("identical content was reported as a change")
	}
	if !second.Stamp.At.Equal(first.Stamp.At) {
		t.Errorf("stamp moved to %s; unchanged content must keep %s", second.Stamp.At, first.Stamp.At)
	}
	if len(analysis.applied) != 2 {
		t.Fatalf("apply ran %d times, want 2: publishing must stay self-healing", len(analysis.applied))
	}
	if !analysis.applied[1].At.Equal(base) {
		t.Errorf("the second publication carried %s, want the original %s", analysis.applied[1].At, base)
	}
}

func TestChangedContentTakesANewStamp(t *testing.T) {
	analysis := &scriptedAnalysis{inputs: "in-1", content: "out-1"}
	orchestrator, _ := newOrchestrator(analysis)
	ctx := context.Background()

	if _, err := orchestrator.Run(ctx, recompute.ReasonStartup, base); err != nil {
		t.Fatalf("first run: %v", err)
	}
	analysis.inputs = "in-2"
	analysis.content = "out-2"

	later := base.Add(90 * time.Minute)
	outcome, err := orchestrator.Run(ctx, recompute.ReasonEvidence, later)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !outcome.ContentChanged || !outcome.InputsChanged {
		t.Fatalf("changed inputs and content were not reported: %+v", outcome)
	}
	if !outcome.Stamp.At.Equal(later) {
		t.Errorf("stamp = %s, want the moment the content changed (%s)", outcome.Stamp.At, later)
	}
}

// TestInputsCanRevert is the trap in "skip when the fingerprint matches the last
// run": a fingerprint that already appeared is not a reason to skip. Erase a
// record and re-push it and the inputs are legitimately back where they were,
// and the projection still has to be rebuilt.
func TestInputsCanRevert(t *testing.T) {
	analysis := &scriptedAnalysis{inputs: "A", content: "out-A"}
	orchestrator, _ := newOrchestrator(analysis)
	ctx := context.Background()

	if _, err := orchestrator.Run(ctx, recompute.ReasonStartup, base); err != nil {
		t.Fatalf("run A: %v", err)
	}
	analysis.inputs, analysis.content = "B", "out-B"
	if _, err := orchestrator.Run(ctx, recompute.ReasonEvidence, base.Add(time.Hour)); err != nil {
		t.Fatalf("run B: %v", err)
	}
	analysis.inputs, analysis.content = "A", "out-A"
	back := base.Add(2 * time.Hour)
	outcome, err := orchestrator.Run(ctx, recompute.ReasonErasure, back)
	if err != nil {
		t.Fatalf("run back to A: %v", err)
	}

	if !outcome.ContentChanged {
		t.Error("reverting to earlier content was treated as no change")
	}
	if !outcome.Stamp.At.Equal(back) {
		t.Errorf("stamp = %s, want %s: the content really did change back", outcome.Stamp.At, back)
	}
	if len(analysis.applied) != 3 {
		t.Errorf("apply ran %d times, want 3", len(analysis.applied))
	}
}

// TestTheStampCarriesTheInputFingerprint pins the idempotency key any future
// emitter must derive record ids from. Without it, "cannot emit duplicate
// proposals" would depend on each emitter remembering to check.
func TestTheStampCarriesTheInputFingerprint(t *testing.T) {
	analysis := &scriptedAnalysis{inputs: "in-1", content: "out-1"}
	orchestrator, _ := newOrchestrator(analysis)

	outcome, err := orchestrator.Run(context.Background(), recompute.ReasonStartup, base)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if outcome.Stamp.Inputs != recompute.Fingerprint("in-1") {
		t.Errorf("stamp carried inputs %q, want the run's own fingerprint", outcome.Stamp.Inputs)
	}
}

// TestAFailedApplyDoesNotBecomeTheNewTruth: a half-finished run must not be
// remembered as completed, or the next run would skip the work it never did.
func TestAFailedApplyDoesNotBecomeTheNewTruth(t *testing.T) {
	analysis := &scriptedAnalysis{inputs: "in-1", content: "out-1"}
	orchestrator, journal := newOrchestrator(analysis)
	ctx := context.Background()

	if _, err := orchestrator.Run(ctx, recompute.ReasonStartup, base); err != nil {
		t.Fatalf("first run: %v", err)
	}
	analysis.inputs, analysis.content = "in-2", "out-2"
	analysis.fail = errors.New("disk full")

	if _, err := orchestrator.Run(ctx, recompute.ReasonEvidence, base.Add(time.Hour)); err == nil {
		t.Fatal("a failing publication was reported as success")
	}

	last, ok, err := journal.LastCompleted(ctx)
	if err != nil || !ok {
		t.Fatalf("last completed: %v ok=%v", err, ok)
	}
	if last.Content != recompute.Fingerprint("out-1") {
		t.Errorf("last completed content = %q, want the run that actually finished", last.Content)
	}
	if journal.runs[1].State != recompute.StateFailed || journal.runs[1].Error == "" {
		t.Errorf("the failed run was recorded as %+v, want a failure with its message", journal.runs[1])
	}
}

// TestRecoverClosesOutRunsLeftByADeadProcess. The recovery itself is the next
// run's fingerprint comparison; this only makes the crash visible.
func TestRecoverClosesOutRunsLeftByADeadProcess(t *testing.T) {
	journal := &memoryJournal{}
	journal.runs = append(journal.runs, recompute.Run{
		ID: 1, State: recompute.StateRunning, StartedAt: base.Add(-time.Hour),
	})
	orchestrator := recompute.Orchestrator{Analysis: &scriptedAnalysis{}, Journal: journal}

	count, err := orchestrator.Recover(context.Background(), base)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if count != 1 {
		t.Errorf("recovered %d runs, want 1", count)
	}
	if journal.runs[0].State != recompute.StateInterrupted {
		t.Errorf("state = %q, want interrupted", journal.runs[0].State)
	}
}

// TestARestartRedoesWorkItCannotProveWasDone: after an interrupted run, the
// next process sees no completed run carrying that content and republishes.
func TestARestartRedoesWorkItCannotProveWasDone(t *testing.T) {
	journal := &memoryJournal{}
	journal.runs = append(journal.runs, recompute.Run{
		ID: 1, State: recompute.StateRunning,
		Inputs: "in-1", Content: "out-1", StartedAt: base.Add(-time.Hour),
	})
	journal.nextID = 1
	analysis := &scriptedAnalysis{inputs: "in-1", content: "out-1"}
	orchestrator := recompute.Orchestrator{Analysis: analysis, Journal: journal}
	ctx := context.Background()

	if _, err := orchestrator.Recover(ctx, base); err != nil {
		t.Fatalf("recover: %v", err)
	}
	outcome, err := orchestrator.Run(ctx, recompute.ReasonStartup, base)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(analysis.applied) != 1 {
		t.Fatalf("apply ran %d times after a crash, want 1", len(analysis.applied))
	}
	if !outcome.ContentChanged {
		t.Error("an interrupted run left content the restart trusted; it must not")
	}
}

func TestRunRequiresItsCollaborators(t *testing.T) {
	if _, err := (recompute.Orchestrator{Journal: &memoryJournal{}}).Run(context.Background(), recompute.ReasonStartup, base); err == nil {
		t.Error("an orchestrator with no analysis ran anyway")
	}
	if _, err := (recompute.Orchestrator{Analysis: &scriptedAnalysis{}}).Run(context.Background(), recompute.ReasonStartup, base); err == nil {
		t.Error("an orchestrator with no journal ran anyway")
	}
}
