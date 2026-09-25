package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/freshness"
	"non24.app/core/recompute"
)

var ErrStaleAnalysis = errors.New("sleep evidence changed while analysis was running")

// SleepAnalysis contains derived data only. Source sessions are read alongside
// it in one transaction, rather than copied into another retained history.
type SleepAnalysis struct {
	Inputs     string                        `json:"inputs"`
	Content    string                        `json:"content"`
	Algorithm  string                        `json:"algorithm"`
	AsOf       time.Time                     `json:"asOf"`
	ComputedAt time.Time                     `json:"computedAt"`
	ChangedAt  time.Time                     `json:"changedAt"`
	ValidUntil time.Time                     `json:"validUntil"`
	Status     string                        `json:"status"`
	Message    string                        `json:"message"`
	Estimate   domain.PhaseEstimate          `json:"estimate"`
	Refusal    *estimation.EstimationRefusal `json:"refusal,omitempty"`
	Freshness  *freshness.Assessment         `json:"freshness,omitempty"`
}

type SleepAnalysisInput struct {
	Fingerprint string
	Snapshot    SleepSnapshot
	Cached      *SleepAnalysis
}

func (s *Store) initializeSleepAnalysis(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS local_sleep_analysis(id INTEGER PRIMARY KEY CHECK(id=1), payload_json BLOB NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS local_recompute_runs(id INTEGER PRIMARY KEY AUTOINCREMENT, state TEXT NOT NULL, payload_json BLOB NOT NULL)`,
	}
	for _, table := range []string{"local_sleep_observations", "local_sleep_corrections"} {
		for _, event := range []string{"INSERT", "UPDATE", "DELETE"} {
			journalErase := ""
			if event == "DELETE" {
				journalErase = "DELETE FROM local_recompute_runs;"
			}
			statements = append(statements, fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %s_analysis_%s AFTER %s ON %s BEGIN DELETE FROM local_sleep_analysis; %s END`, table, event, event, table, journalErase))
		}
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// The fingerprint, corrected evidence and cached result are from one snapshot.
// A conflicting review returns its existing error and never falls back to cache.
func (s *Store) ReadSleepAnalysisInput(ctx context.Context) (SleepAnalysisInput, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SleepAnalysisInput{}, err
	}
	defer tx.Rollback()
	input := SleepAnalysisInput{}
	if input.Snapshot, err = readSleepSnapshot(ctx, tx); err != nil {
		return input, err
	}
	if input.Fingerprint, err = sleepPlanningFingerprint(ctx, tx); err != nil {
		return input, err
	}
	var data []byte
	err = tx.QueryRowContext(ctx, `SELECT CASE WHEN length(payload_json)<=1048576 THEN payload_json ELSE NULL END FROM local_sleep_analysis WHERE id=1`).Scan(&data)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return input, err
	}
	if err == nil {
		var cached SleepAnalysis
		// Corrupt derived data can be recomputed; it cannot substitute for evidence.
		if json.Unmarshal(data, &cached) == nil && cached.Inputs == input.Fingerprint {
			input.Cached = &cached
		}
	}
	return input, tx.Commit()
}

func (s *Store) SaveSleepAnalysis(ctx context.Context, value SleepAnalysis) error {
	if value.Inputs == "" || value.AsOf.IsZero() || !value.ValidUntil.After(value.AsOf) {
		return errors.New("invalid sleep analysis")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > 1024*1024 {
		return errors.New("sleep analysis exceeds its storage bound")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := sleepPlanningFingerprint(ctx, tx)
	if err != nil {
		return err
	}
	if current != value.Inputs {
		return ErrStaleAnalysis
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_sleep_analysis(id,payload_json) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET payload_json=excluded.payload_json`, data); err != nil {
		return err
	}
	return tx.Commit()
}

type SleepAnalysisJournal struct{ Store *Store }

var _ recompute.Journal = SleepAnalysisJournal{}

func (j SleepAnalysisJournal) LastCompleted(ctx context.Context) (recompute.Run, bool, error) {
	var data []byte
	err := j.Store.db.QueryRowContext(ctx, `SELECT CASE WHEN length(payload_json)<=16384 THEN payload_json ELSE NULL END FROM local_recompute_runs WHERE state='done' ORDER BY id DESC LIMIT 1`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return recompute.Run{}, false, nil
	}
	if err != nil {
		return recompute.Run{}, false, err
	}
	var run recompute.Run
	if err := json.Unmarshal(data, &run); err != nil || run.State != recompute.StateDone || run.Content == "" || run.ContentChangedAt.IsZero() {
		return recompute.Run{}, false, nil
	}
	return run, true, nil
}

func (j SleepAnalysisJournal) Begin(ctx context.Context, run recompute.Run) (int64, error) {
	data, err := json.Marshal(run)
	if err != nil {
		return 0, err
	}
	tx, err := j.Store.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	current, err := sleepPlanningFingerprint(ctx, tx)
	if err != nil {
		return 0, err
	}
	if current != string(run.Inputs) {
		return 0, ErrStaleAnalysis
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO local_recompute_runs(state,payload_json) VALUES('running',?)`, data)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (j SleepAnalysisJournal) Complete(ctx context.Context, run recompute.Run) error {
	data, err := json.Marshal(run)
	if err != nil {
		return err
	}
	result, err := j.Store.db.ExecContext(ctx, `UPDATE local_recompute_runs SET state='done',payload_json=? WHERE id=? AND state='running'`, data, run.ID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrStaleAnalysis
	}
	_, err = j.Store.db.ExecContext(ctx, `DELETE FROM local_recompute_runs WHERE id NOT IN (SELECT id FROM local_recompute_runs ORDER BY id DESC LIMIT 200)`)
	return err
}

func (j SleepAnalysisJournal) Fail(ctx context.Context, id int64, at time.Time, _ string) error {
	// Do not retain raw health/SQL errors in the operational journal.
	run := recompute.Run{ID: id, State: recompute.StateFailed, CompletedAt: at, Error: "local analysis could not be published"}
	data, _ := json.Marshal(run)
	_, err := j.Store.db.ExecContext(ctx, `UPDATE local_recompute_runs SET state='failed',payload_json=? WHERE id=?`, data, id)
	if err != nil {
		return err
	}
	_, err = j.Store.db.ExecContext(ctx, `DELETE FROM local_recompute_runs WHERE id NOT IN (SELECT id FROM local_recompute_runs ORDER BY id DESC LIMIT 200)`)
	return err
}

func (j SleepAnalysisJournal) MarkInterrupted(ctx context.Context, at time.Time) (int, error) {
	data, _ := json.Marshal(recompute.Run{State: recompute.StateInterrupted, CompletedAt: at})
	result, err := j.Store.db.ExecContext(ctx, `UPDATE local_recompute_runs SET state='interrupted',payload_json=? WHERE state='running'`, data)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	_, err = j.Store.db.ExecContext(ctx, `DELETE FROM local_recompute_runs WHERE id NOT IN (SELECT id FROM local_recompute_runs ORDER BY id DESC LIMIT 200)`)
	return int(count), err
}
