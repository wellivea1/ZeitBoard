// Package recompute is the analysis loop's scheduler.
//
// Before it, analysis ran when somebody looked: the desktop recomputed on a
// screen load, and the server republished the shared projection inside whatever
// HTTP request happened to arrive. That is enough while a person is watching and
// wrong the rest of the time, and ADR-0031 made it worse before it made it
// better — it moved the freshness decision to materialization, so the decision
// now only happens when something else causes a materialization. Sleep that was
// expected and never recorded should withhold the current-state claim at the
// moment the grace period passes, not at the moment the next unrelated push
// arrives.
//
// The package is therefore built around three ideas.
//
// **Durability by reconciliation, not by queue.** A recompute is a pure function
// of the inputs, so there is nothing worth remembering about a request. What is
// worth remembering is which inputs have already been processed. A restart
// compares the current fingerprint against the last completed run and re-derives
// the need from the data, which cannot lose a request the way a dropped queue
// message can.
//
// **A result expires on a clock.** The freshness policy's verdict changes with
// time alone, so a run reports when its own answer could stop being true and the
// caller schedules a wake for that instant.
//
// **Emission is keyed, not counted.** Apply receives a Stamp derived from the
// content, so re-running produces the same output rather than a second one.
//
// It holds no LLM, no provider client, and no network client: an import test in
// this package asserts that structurally rather than by convention.
package recompute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"non24.app/core/domain"
)

// Reason names why a recompute was asked for. It is a closed set so an operator
// reading the journal sees a cause rather than prose, and so a test can assert
// on the trigger rather than on a log line.
type Reason string

const (
	// ReasonStartup is the reconciliation pass a process makes when it starts,
	// because it cannot know what changed while it was not running.
	ReasonStartup Reason = "startup"

	// ReasonEvidence is new or corrected evidence arriving from a device.
	ReasonEvidence Reason = "evidence"

	// ReasonErasure is a record the user deleted. Analysis derived from it must
	// stop being published.
	ReasonErasure Reason = "erasure"

	// ReasonSharing is a change to who a projection is published for.
	ReasonSharing Reason = "sharing"

	// ReasonFreshnessExpiry is the run that happens because time passed. It is
	// the one that closes ADR-0031's gap: nothing changed, and the honest answer
	// changed anyway.
	ReasonFreshnessExpiry Reason = "freshness_expiry"

	// ReasonRetry is a re-attempt after a failed run.
	ReasonRetry Reason = "retry"

	// ReasonHeartbeat is the periodic backstop. Nothing should depend on it;
	// it exists so that state drifting underneath the process — a database
	// restored from a backup, say — is repaired without a restart.
	ReasonHeartbeat Reason = "heartbeat"
)

// Fingerprint is a digest. Two kinds are used and they answer different
// questions: an input fingerprint says whether the analysis would read the same
// thing, and a content fingerprint says whether it would publish the same thing.
type Fingerprint string

// Stamp is what an Apply is allowed to know about its own run. `At` is when the
// published content last changed rather than when this run happened, so a run
// that changes nothing does not advertise itself as an update. `Inputs` is the
// idempotency key any future emitter must derive its record ids from: that is
// how "cannot emit a duplicate proposal" is enforced by construction rather than
// by remembering to check.
type Stamp struct {
	At     time.Time
	Inputs Fingerprint
}

// Prepared is a computed, not yet published, analysis result.
//
// Reading and publishing are separated because they answer to different rules.
// Reading must happen every time, or the orchestrator cannot tell whether
// anything changed. Publishing must be idempotent, and is handed the stamp that
// makes it so.
type Prepared struct {
	// Inputs digests everything the analysis read.
	Inputs Fingerprint

	// Content digests what would be published. It excludes anything derived
	// from the current instant, so an unchanged rhythm produces an unchanged
	// content fingerprint however often the analysis runs.
	Content Fingerprint

	// ValidUntil is the earliest instant at which this result could stop being
	// true with no new input at all — normally a freshness threshold. Zero means
	// nothing further expires on its own.
	ValidUntil time.Time

	// Apply publishes the result. It must be safe to call more than once with
	// the same stamp.
	Apply func(ctx context.Context, stamp Stamp) error
}

// Analysis is the work being scheduled.
type Analysis interface {
	Prepare(ctx context.Context, now time.Time) (Prepared, error)
}

// RunState is the durable lifecycle of one run.
type RunState string

const (
	StateRunning RunState = "running"
	StateDone    RunState = "done"
	StateFailed  RunState = "failed"

	// StateInterrupted is written on startup for a run that was still marked
	// running, which means the process died mid-flight. It is a record, not a
	// recovery: recovery is the fingerprint comparison that follows.
	StateInterrupted RunState = "interrupted"
)

// Run is one journal entry.
type Run struct {
	ID     int64
	Reason Reason
	State  RunState

	Inputs  Fingerprint
	Content Fingerprint

	StartedAt   time.Time
	CompletedAt time.Time

	// ContentChangedAt is when the published content last differed. It carries
	// forward across runs that change nothing, which is what keeps a projection
	// from claiming to be fresher than the evidence behind it.
	ContentChangedAt time.Time

	ValidUntil time.Time
	Error      string
}

// Journal is the durable record. It is the only persistence this package needs:
// there is no work queue, because the work is derivable from the inputs.
type Journal interface {
	// LastCompleted returns the newest run that finished successfully.
	LastCompleted(ctx context.Context) (Run, bool, error)

	// Begin records a run as started and returns its id.
	Begin(ctx context.Context, run Run) (int64, error)

	// Complete records a successful finish.
	Complete(ctx context.Context, run Run) error

	// Fail records an unsuccessful finish. The run is not retried here; the
	// caller's schedule decides that.
	Fail(ctx context.Context, id int64, at time.Time, message string) error

	// MarkInterrupted closes out runs left in StateRunning by a process that
	// died, and reports how many there were.
	MarkInterrupted(ctx context.Context, at time.Time) (int, error)
}

// Orchestrator runs one analysis against one journal. It owns no goroutine and
// no timer: a caller drives it, so its behaviour is fully determined by the
// arguments and can be tested without waiting for anything.
type Orchestrator struct {
	Analysis Analysis
	Journal  Journal
}

// Outcome reports what one pass did.
type Outcome struct {
	Reason         Reason
	Inputs         Fingerprint
	Content        Fingerprint
	ContentChanged bool
	InputsChanged  bool
	ValidUntil     time.Time
	Stamp          Stamp
}

// Recover closes out any run left running by a previous process. The count is
// returned so a daemon can log that it happened; the actual recovery is the next
// Run, which compares fingerprints and redoes the work if it is still needed.
func (o Orchestrator) Recover(ctx context.Context, now time.Time) (int, error) {
	if o.Journal == nil {
		return 0, errors.New("recompute: a journal is required")
	}
	return o.Journal.MarkInterrupted(ctx, now.UTC())
}

// Run performs one recomputation and records it.
//
// It always publishes. Skipping the publish when nothing changed would be a
// small saving and a real hazard: it would make the published state depend on
// what this process happens to remember, so a projection lost to an external
// restore could never be rebuilt. Instead the publish is made idempotent — the
// stamp is a function of the content, so repeating a run rewrites the same
// answer rather than announcing a new one.
func (o Orchestrator) Run(ctx context.Context, reason Reason, now time.Time) (Outcome, error) {
	if o.Analysis == nil {
		return Outcome{}, errors.New("recompute: an analysis is required")
	}
	if o.Journal == nil {
		return Outcome{}, errors.New("recompute: a journal is required")
	}
	now = now.UTC()

	previous, hadPrevious, err := o.Journal.LastCompleted(ctx)
	if err != nil {
		return Outcome{}, fmt.Errorf("read recompute journal: %w", err)
	}

	prepared, err := o.Analysis.Prepare(ctx, now)
	if err != nil {
		return Outcome{}, fmt.Errorf("prepare analysis: %w", err)
	}

	contentChanged := !hadPrevious || prepared.Content != previous.Content
	stamp := Stamp{At: now, Inputs: prepared.Inputs}
	if !contentChanged && !previous.ContentChangedAt.IsZero() {
		// The projection is unchanged, so it keeps the time it last changed.
		// Advancing it here is how a surface ends up reporting "updated just
		// now" over week-old evidence.
		stamp.At = previous.ContentChangedAt.UTC()
	}

	// There is no attempt counter. A streak of failures is a streak of rows
	// whose reason is `retry`, and a second place to record the same fact is a
	// second place for it to be wrong.
	run := Run{
		Reason:           reason,
		State:            StateRunning,
		Inputs:           prepared.Inputs,
		Content:          prepared.Content,
		StartedAt:        now,
		ContentChangedAt: stamp.At,
		ValidUntil:       prepared.ValidUntil.UTC(),
	}
	id, err := o.Journal.Begin(ctx, run)
	if err != nil {
		return Outcome{}, fmt.Errorf("begin recompute run: %w", err)
	}
	run.ID = id

	if prepared.Apply != nil {
		if err := prepared.Apply(ctx, stamp); err != nil {
			message := err.Error()
			if failErr := o.Journal.Fail(ctx, id, now, message); failErr != nil {
				return Outcome{}, fmt.Errorf("apply analysis: %v (and recording the failure failed: %w)", err, failErr)
			}
			return Outcome{}, fmt.Errorf("apply analysis: %w", err)
		}
	}

	run.State = StateDone
	run.CompletedAt = now
	if err := o.Journal.Complete(ctx, run); err != nil {
		return Outcome{}, fmt.Errorf("complete recompute run: %w", err)
	}

	return Outcome{
		Reason:         reason,
		Inputs:         prepared.Inputs,
		Content:        prepared.Content,
		ContentChanged: contentChanged,
		InputsChanged:  !hadPrevious || prepared.Inputs != previous.Inputs,
		ValidUntil:     run.ValidUntil,
		Stamp:          stamp,
	}, nil
}

// Inputs is everything one recompute reads. It exists to be fingerprinted, so
// it deliberately holds the raw inputs rather than anything derived from them:
// a digest of a conclusion cannot tell you whether the premises changed.
type Inputs struct {
	// Sleep is the effective sleep history the estimator would consume.
	Sleep []domain.SleepSession

	// Consumers are the surfaces the result will be published to, as stable
	// opaque strings. They belong in the fingerprint because a new consumer
	// needs a row written even when the rhythm has not moved at all.
	Consumers []string
}

// Fingerprint digests the inputs.
//
// It is order-independent and clock-independent by construction: the entries are
// sorted before hashing and no field derived from the current instant is
// included. A fingerprint that moved with the clock would report a change on
// every run and make the whole mechanism decorative.
func (in Inputs) Fingerprint() Fingerprint {
	entries := make([]string, 0, len(in.Sleep)+len(in.Consumers))
	for _, session := range in.Sleep {
		entries = append(entries, sleepEntry(session))
	}
	for _, consumer := range in.Consumers {
		entries = append(entries, "consumer\x1f"+consumer)
	}
	return digest(entries)
}

// sleepEntry renders one session into a stable string. Interval boundaries and
// the recorded-at times matter because the estimator and the freshness policy
// read exactly those; the private text of a session does not appear here and
// does not need to.
func sleepEntry(session domain.SleepSession) string {
	parts := make([]string, 0, 1+len(session.Intervals))
	parts = append(parts, "sleep\x1f"+
		string(session.ID)+"\x1f"+
		string(session.EffectiveClassification())+"\x1f"+
		strconv.FormatBool(session.Suppressed)+"\x1f"+
		instant(session.CreatedAt))
	for _, interval := range session.Intervals {
		parts = append(parts,
			instant(interval.Interval.Start.UTC)+"\x1f"+
				instant(interval.Interval.End.UTC)+"\x1f"+
				interval.Interval.Start.ZoneID+"\x1f"+
				interval.Interval.End.ZoneID+"\x1f"+
				string(interval.StartEvidence.Status)+"\x1f"+
				string(interval.EndEvidence.Status)+"\x1f"+
				instant(interval.StartEvidence.RecordedAt)+"\x1f"+
				instant(interval.EndEvidence.RecordedAt))
	}
	sort.Strings(parts[1:])
	return strings.Join(parts, "\x1d")
}

// Digest builds a content fingerprint from already-rendered parts. Callers use
// it for the published side, which this package cannot model without importing
// the surfaces it schedules.
func Digest(parts []string) Fingerprint {
	return digest(parts)
}

func digest(entries []string) Fingerprint {
	sorted := append([]string(nil), entries...)
	sort.Strings(sorted)
	hash := sha256.New()
	for _, entry := range sorted {
		// The length prefix stops two different entry lists from colliding by
		// running into each other across the separator.
		hash.Write([]byte(strconv.Itoa(len(entry))))
		hash.Write([]byte{0x1e})
		hash.Write([]byte(entry))
		hash.Write([]byte{0x1e})
	}
	return Fingerprint(hex.EncodeToString(hash.Sum(nil)))
}

// instant renders a time so that equal instants in different locations produce
// equal strings, and a zero time produces a fixed marker rather than an
// arbitrary year.
func instant(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return strconv.FormatInt(value.UTC().UnixNano(), 10)
}
