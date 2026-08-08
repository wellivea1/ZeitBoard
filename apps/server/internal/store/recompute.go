package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"non24.app/core/recompute"
)

// RecomputeJournal is the durable half of the analysis loop (ADR-0033). It
// records what was recomputed and when, so a process that restarts can tell
// whether the current inputs have already been processed rather than trusting a
// request it may never have received.
//
// The fingerprints are encrypted with everything else in this database. They are
// one-way digests, but a digest still answers "did the user sleep at 04:12 on
// Tuesday?" for anyone willing to guess, and the point of encrypting this file
// at rest is that a copy of it answers nothing.
type RecomputeJournal struct {
	Store *Store
}

var _ recompute.Journal = RecomputeJournal{}

// recomputeSecrets is the encrypted part of a journal row.
type recomputeSecrets struct {
	Inputs  string `json:"inputs"`
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

// MaxRecomputeRunHistory bounds the journal. It is an operational record, not
// an audit trail with a retention obligation, and an unbounded table on a
// self-hosted box is a slow disk leak.
const MaxRecomputeRunHistory = 200

func (j RecomputeJournal) store() (*Store, error) {
	if j.Store == nil {
		return nil, errors.New("recompute journal requires a store")
	}
	return j.Store, nil
}

func (j RecomputeJournal) Begin(ctx context.Context, run recompute.Run) (int64, error) {
	st, err := j.store()
	if err != nil {
		return 0, err
	}
	startedAt := formatJournalTime(run.StartedAt)
	nonce, ciphertext, err := st.sealRecompute(startedAt, string(run.Reason), recomputeSecrets{
		Inputs:  string(run.Inputs),
		Content: string(run.Content),
	})
	if err != nil {
		return 0, err
	}
	result, err := st.db.ExecContext(ctx, `INSERT INTO recompute_runs
		(reason, state, started_at, completed_at, content_changed_at, valid_until, nonce, ciphertext)
		VALUES (?, ?, ?, '', ?, ?, ?, ?)`,
		string(run.Reason), string(recompute.StateRunning), startedAt,
		formatJournalTime(run.ContentChangedAt), formatJournalTime(run.ValidUntil),
		nonce, ciphertext,
	)
	if err != nil {
		return 0, fmt.Errorf("begin recompute run: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (j RecomputeJournal) Complete(ctx context.Context, run recompute.Run) error {
	st, err := j.store()
	if err != nil {
		return err
	}
	if _, err := st.db.ExecContext(ctx,
		`UPDATE recompute_runs
		 SET state = ?, completed_at = ?, content_changed_at = ?, valid_until = ?
		 WHERE id = ?`,
		string(recompute.StateDone), formatJournalTime(run.CompletedAt),
		formatJournalTime(run.ContentChangedAt), formatJournalTime(run.ValidUntil), run.ID,
	); err != nil {
		return fmt.Errorf("complete recompute run: %w", err)
	}
	return st.pruneRecomputeRuns(ctx)
}

func (j RecomputeJournal) Fail(ctx context.Context, id int64, at time.Time, message string) error {
	st, err := j.store()
	if err != nil {
		return err
	}
	// Re-seal rather than update in place: the message is bound to the row's
	// own additional data, so it cannot be moved to another run.
	var (
		startedAt, reason string
		nonce, ciphertext []byte
	)
	if err := st.db.QueryRowContext(ctx,
		`SELECT started_at, reason, nonce, ciphertext FROM recompute_runs WHERE id = ?`, id,
	).Scan(&startedAt, &reason, &nonce, &ciphertext); err != nil {
		return fmt.Errorf("read recompute run: %w", err)
	}
	secrets, err := st.openRecompute(startedAt, reason, nonce, ciphertext)
	if err != nil {
		return err
	}
	secrets.Error = truncateRunes(message, 500)
	newNonce, newCiphertext, err := st.sealRecompute(startedAt, reason, secrets)
	if err != nil {
		return err
	}
	if _, err := st.db.ExecContext(ctx,
		`UPDATE recompute_runs SET state = ?, completed_at = ?, nonce = ?, ciphertext = ? WHERE id = ?`,
		string(recompute.StateFailed), formatJournalTime(at), newNonce, newCiphertext, id,
	); err != nil {
		return fmt.Errorf("fail recompute run: %w", err)
	}
	return nil
}

// LastCompleted returns the newest successful run. It is the whole of the
// orchestrator's memory: everything else about what to do next is derived from
// the inputs as they are now.
func (j RecomputeJournal) LastCompleted(ctx context.Context) (recompute.Run, bool, error) {
	st, err := j.store()
	if err != nil {
		return recompute.Run{}, false, err
	}
	row := st.db.QueryRowContext(ctx,
		`SELECT id, reason, state, started_at, completed_at, content_changed_at, valid_until, nonce, ciphertext
		 FROM recompute_runs WHERE state = ? ORDER BY id DESC LIMIT 1`,
		string(recompute.StateDone))
	run, err := st.scanRecomputeRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return recompute.Run{}, false, nil
	}
	if err != nil {
		return recompute.Run{}, false, err
	}
	return run, true, nil
}

// MarkInterrupted closes out runs a dead process left open. It is a record of
// what happened, not a recovery: the recovery is the next run, which compares
// the current fingerprint against the last completed one and redoes the work
// only if it is still needed.
func (j RecomputeJournal) MarkInterrupted(ctx context.Context, at time.Time) (int, error) {
	st, err := j.store()
	if err != nil {
		return 0, err
	}
	result, err := st.db.ExecContext(ctx,
		`UPDATE recompute_runs SET state = ?, completed_at = ? WHERE state = ?`,
		string(recompute.StateInterrupted), formatJournalTime(at), string(recompute.StateRunning),
	)
	if err != nil {
		return 0, fmt.Errorf("mark interrupted recompute runs: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// RecomputeRuns returns the most recent runs, newest first. It exists for
// operators and for tests; nothing in the running loop reads it.
func (j RecomputeJournal) RecomputeRuns(ctx context.Context, limit int) ([]recompute.Run, error) {
	st, err := j.store()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > MaxRecomputeRunHistory {
		limit = MaxRecomputeRunHistory
	}
	rows, err := st.db.QueryContext(ctx,
		`SELECT id, reason, state, started_at, completed_at, content_changed_at, valid_until, nonce, ciphertext
		 FROM recompute_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := make([]recompute.Run, 0, limit)
	for rows.Next() {
		run, err := st.scanRecomputeRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

type recomputeScanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanRecomputeRun(row recomputeScanner) (recompute.Run, error) {
	var (
		run                                                  recompute.Run
		reason, state                                        string
		startedAt, completedAt, contentChangedAt, validUntil string
		nonce, ciphertext                                    []byte
	)
	if err := row.Scan(&run.ID, &reason, &state, &startedAt, &completedAt,
		&contentChangedAt, &validUntil, &nonce, &ciphertext); err != nil {
		return recompute.Run{}, err
	}
	secrets, err := s.openRecompute(startedAt, reason, nonce, ciphertext)
	if err != nil {
		return recompute.Run{}, err
	}
	run.Reason = recompute.Reason(reason)
	run.State = recompute.RunState(state)
	run.Inputs = recompute.Fingerprint(secrets.Inputs)
	run.Content = recompute.Fingerprint(secrets.Content)
	run.Error = secrets.Error
	for _, field := range []struct {
		target *time.Time
		value  string
	}{
		{&run.StartedAt, startedAt},
		{&run.CompletedAt, completedAt},
		{&run.ContentChangedAt, contentChangedAt},
		{&run.ValidUntil, validUntil},
	} {
		parsed, err := parseJournalTime(field.value)
		if err != nil {
			return recompute.Run{}, err
		}
		*field.target = parsed
	}
	return run, nil
}

func (s *Store) sealRecompute(startedAt, reason string, secrets recomputeSecrets) ([]byte, []byte, error) {
	payload, err := json.Marshal(secrets)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, s.encrypt(nonce, payload, recomputeAAD(startedAt, reason)), nil
}

func (s *Store) openRecompute(startedAt, reason string, nonce, ciphertext []byte) (recomputeSecrets, error) {
	plaintext, err := s.decrypt(nonce, ciphertext, recomputeAAD(startedAt, reason))
	if err != nil {
		return recomputeSecrets{}, fmt.Errorf("decrypt recompute run: %w", err)
	}
	var secrets recomputeSecrets
	if err := json.Unmarshal(plaintext, &secrets); err != nil {
		return recomputeSecrets{}, err
	}
	return secrets, nil
}

// pruneRecomputeRuns keeps the journal bounded, always retaining the newest
// completed run because that one is load-bearing.
func (s *Store) pruneRecomputeRuns(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM recompute_runs WHERE id NOT IN (
			SELECT id FROM recompute_runs ORDER BY id DESC LIMIT ?
		)`, MaxRecomputeRunHistory)
	return err
}

func recomputeAAD(startedAt, reason string) []byte {
	return []byte(strings.Join([]string{"recompute", startedAt, reason}, "\x00"))
}

func formatJournalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseJournalTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
