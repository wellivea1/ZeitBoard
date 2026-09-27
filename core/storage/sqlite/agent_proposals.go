package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	medicationcore "non24.app/core/medication"
)

// Proposals an agent made that wait on this computer for the owner (ADR-0051):
// a dose to record, a task to add. Only the owner's acceptance makes the
// record, and it makes the same record the owner's own entry would, in one
// transaction. Proposals are never synced or exported, and erasing a
// medication erases its dose proposals.

// The kinds of proposal, named by their action in the registry.
const (
	AgentProposalLogDose = "propose_log_dose"
	AgentProposalAddTask = "propose_add_task"
)

// The states of a proposal: the owner approved or rejected it, or it waits. A
// waiting proposal past its expiry reads as expired; nothing rewrites it.
const (
	AgentProposalPending  = "pending"
	AgentProposalApproved = "approved"
	AgentProposalRejected = "rejected"
	AgentProposalExpired  = "expired"
)

const (
	// AgentProposalLifetime is how long a proposal waits: long enough to span
	// a sleep, short enough not to meet the next day's dose.
	AgentProposalLifetime = 24 * time.Hour
	// MaxPendingAgentProposals bounds what agents can queue for the owner.
	MaxPendingAgentProposals = 20
	// agentProposalHistory is how long a decided or lapsed proposal stays in
	// the decision history.
	agentProposalHistory   = 30 * 24 * time.Hour
	agentProposalListLimit = 100
)

var (
	ErrAgentProposalNotFound  = errors.New("proposal not found")
	ErrAgentProposalDecided   = errors.New("proposal was already decided")
	ErrAgentProposalExpired   = errors.New("proposal has lapsed")
	ErrTooManyAgentProposals  = errors.New("too many proposals are already waiting for the owner")
	ErrMedicationNotActive    = errors.New("medication is not active")
	errAgentProposalMalformed = errors.New("proposal is malformed")
)

// ProposedDose is a dose an agent asked to record.
type ProposedDose struct {
	MedicationID string    `json:"medication_id"`
	Status       string    `json:"status"`
	DoseAt       time.Time `json:"dose_at"`
	ZoneID       string    `json:"zone_id"`
}

// ProposedTask is a task an agent asked to add, in the owner's words.
type ProposedTask struct {
	Title           string     `json:"title"`
	DurationMinutes int        `json:"duration_minutes"`
	EarliestStartAt *time.Time `json:"earliest_start_at,omitempty"`
	LatestFinishAt  *time.Time `json:"latest_finish_at,omitempty"`
}

// AgentProposal is one proposal waiting for, or decided by, the owner. Exactly
// one of Dose and Task is set, by ActionID.
type AgentProposal struct {
	ProposalID string
	ActionID   string
	Dose       *ProposedDose
	Task       *ProposedTask
	CreatedAt  time.Time
	ExpiresAt  time.Time
	State      string
	// DecidedAt and ResultID are set once the owner decides; ResultID names
	// the dose event or the task an approval made.
	DecidedAt time.Time
	ResultID  string
}

func (s *Store) initializeAgentProposals(ctx context.Context) error {
	for _, statement := range []string{
		// Dose proposals lived in their own table before task proposals
		// joined them, on the day both were built; nothing depends on it.
		`DROP TABLE IF EXISTS local_dose_proposals`,
		`CREATE TABLE IF NOT EXISTS local_agent_proposals(
			proposal_id TEXT PRIMARY KEY,
			action_id TEXT NOT NULL CHECK(action_id IN ('propose_log_dose', 'propose_add_task')),
			medication_id TEXT,
			payload_json BLOB NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			decision TEXT NOT NULL CHECK(decision IN ('pending', 'approved', 'rejected')),
			decided_at TEXT NOT NULL DEFAULT '',
			result_id TEXT NOT NULL DEFAULT '',
			CHECK((action_id = 'propose_log_dose') = (medication_id IS NOT NULL)),
			FOREIGN KEY(medication_id) REFERENCES local_medications(medication_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_agent_proposals_created
			ON local_agent_proposals(created_at, proposal_id)`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate agent proposals: %w", err)
		}
	}
	return nil
}

// ProposeDose queues a dose for the owner at now. The medication must exist
// and be active.
func (s *Store) ProposeDose(ctx context.Context, proposalID string, dose ProposedDose, now time.Time) error {
	if !medicationcore.ValidIdentifier(dose.MedicationID) ||
		(dose.Status != MedicationEventTaken && dose.Status != MedicationEventSkipped) ||
		dose.DoseAt.IsZero() || !loadableZone(dose.ZoneID) {
		return errAgentProposalMalformed
	}
	dose.DoseAt = dose.DoseAt.UTC()
	return s.propose(ctx, proposalID, AgentProposalLogDose, dose.MedicationID, dose, now, func(tx *sql.Tx) error {
		var active bool
		err := tx.QueryRowContext(ctx, `SELECT active FROM local_medications WHERE medication_id = ?`, dose.MedicationID).Scan(&active)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrMedicationNotFound
		case err != nil:
			return err
		case !active:
			return ErrMedicationNotActive
		}
		return nil
	})
}

// ProposeTask queues a task for the owner to add, at now.
func (s *Store) ProposeTask(ctx context.Context, proposalID string, task ProposedTask, now time.Time) error {
	task.Title = strings.TrimSpace(task.Title)
	// The task the approval adds must be valid; test it as that task now.
	if err := validateTask(proposedTaskRecord("task_proposal_check", task, now)); err != nil {
		return fmt.Errorf("%w: %v", errAgentProposalMalformed, err)
	}
	return s.propose(ctx, proposalID, AgentProposalAddTask, "", task, now, nil)
}

func (s *Store) propose(ctx context.Context, proposalID, actionID, medicationID string, payload any, now time.Time, check func(*sql.Tx) error) error {
	now = now.UTC()
	if !medicationcore.ValidIdentifier(proposalID) {
		return errAgentProposalMalformed
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cutoff := formatSQLiteTime(now.Add(-agentProposalHistory))
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_agent_proposals
		WHERE (decision = 'pending' AND expires_at < ?) OR (decision != 'pending' AND decided_at < ?)`, cutoff, cutoff); err != nil {
		return err
	}
	if check != nil {
		if err := check(tx); err != nil {
			return err
		}
	}
	var waiting int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_agent_proposals WHERE decision = 'pending' AND expires_at > ?`,
		formatSQLiteTime(now)).Scan(&waiting); err != nil {
		return err
	}
	if waiting >= MaxPendingAgentProposals {
		return ErrTooManyAgentProposals
	}
	var medication any
	if medicationID != "" {
		medication = medicationID
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_agent_proposals(
		proposal_id, action_id, medication_id, payload_json, created_at, expires_at, decision
	) VALUES(?, ?, ?, ?, ?, ?, 'pending')`,
		proposalID, actionID, medication, encoded, formatSQLiteTime(now), formatSQLiteTime(now.Add(AgentProposalLifetime)),
	); err != nil {
		return err
	}
	return tx.Commit()
}

// AgentProposals lists the waiting proposals and the recent history at now,
// newest first.
func (s *Store) AgentProposals(ctx context.Context, now time.Time) ([]AgentProposal, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT proposal_id, action_id, payload_json, created_at, expires_at,
		decision, decided_at, result_id
		FROM local_agent_proposals ORDER BY created_at DESC, proposal_id DESC LIMIT ?`, agentProposalListLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	proposals := make([]AgentProposal, 0)
	for rows.Next() {
		proposal, err := scanAgentProposal(rows, now)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	return proposals, rows.Err()
}

// ApproveAgentProposal makes a waiting proposal the owner's record at now, in
// one transaction: the dose event or the task, under an id newID makes for
// that kind of record ("dose" or "task").
func (s *Store) ApproveAgentProposal(ctx context.Context, proposalID string, now time.Time, newID func(kind string) string) (AgentProposal, error) {
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentProposal{}, err
	}
	defer tx.Rollback()
	proposal, err := pendingAgentProposalTx(ctx, tx, proposalID, now)
	if err != nil {
		return AgentProposal{}, err
	}
	var resultID string
	switch {
	case proposal.Dose != nil:
		resultID = newID("dose")
		err = appendMedicationEventTx(ctx, tx, MedicationEventRecord{
			EventID:      resultID,
			MedicationID: proposal.Dose.MedicationID,
			DoseAt:       proposal.Dose.DoseAt,
			ZoneID:       proposal.Dose.ZoneID,
			Status:       proposal.Dose.Status,
			// Adherence counts only doses the owner marks scheduled (ADR-0027).
			Scheduled: false,
			Provenance: SleepObservationProvenance{
				AcquisitionMethod: ProvenanceAcquisitionManual,
				EvidenceStatus:    ProvenanceEvidenceUserReported,
				RecordedAt:        now,
			},
		})
	case proposal.Task != nil:
		resultID = newID("task")
		err = addTaskTx(ctx, tx, proposedTaskRecord(resultID, *proposal.Task, now))
	default:
		err = errAgentProposalMalformed
	}
	if err != nil {
		return AgentProposal{}, err
	}
	if err := decideAgentProposalTx(ctx, tx, proposalID, AgentProposalApproved, resultID, now); err != nil {
		return AgentProposal{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentProposal{}, err
	}
	proposal.State, proposal.DecidedAt, proposal.ResultID = AgentProposalApproved, now, resultID
	return proposal, nil
}

// RejectAgentProposal declines a waiting proposal; nothing is recorded.
func (s *Store) RejectAgentProposal(ctx context.Context, proposalID string, now time.Time) error {
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := pendingAgentProposalTx(ctx, tx, proposalID, now); err != nil {
		return err
	}
	if err := decideAgentProposalTx(ctx, tx, proposalID, AgentProposalRejected, "", now); err != nil {
		return err
	}
	return tx.Commit()
}

// proposedTaskRecord is the open task an approval adds, created at now.
func proposedTaskRecord(taskID string, task ProposedTask, now time.Time) TaskRecord {
	return TaskRecord{
		TaskID:          taskID,
		Title:           task.Title,
		DurationMinutes: task.DurationMinutes,
		Status:          TaskStatusOpen,
		CreatedAt:       now,
		UpdatedAt:       now,
		Revision:        1,
		EarliestStartAt: task.EarliestStartAt,
		LatestFinishAt:  task.LatestFinishAt,
	}
}

func pendingAgentProposalTx(ctx context.Context, tx *sql.Tx, proposalID string, now time.Time) (AgentProposal, error) {
	row := tx.QueryRowContext(ctx, `SELECT proposal_id, action_id, payload_json, created_at, expires_at,
		decision, decided_at, result_id
		FROM local_agent_proposals WHERE proposal_id = ?`, proposalID)
	proposal, err := scanAgentProposal(row, now)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return AgentProposal{}, ErrAgentProposalNotFound
	case err != nil:
		return AgentProposal{}, err
	case proposal.State == AgentProposalExpired:
		return AgentProposal{}, ErrAgentProposalExpired
	case proposal.State != AgentProposalPending:
		return AgentProposal{}, ErrAgentProposalDecided
	}
	return proposal, nil
}

func decideAgentProposalTx(ctx context.Context, tx *sql.Tx, proposalID, decision, resultID string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE local_agent_proposals SET decision = ?, decided_at = ?, result_id = ?
		WHERE proposal_id = ? AND decision = 'pending'`, decision, formatSQLiteTime(now), resultID, proposalID)
	return err
}

func scanAgentProposal(row rowScanner, now time.Time) (AgentProposal, error) {
	var proposal AgentProposal
	var payload []byte
	var createdAt, expiresAt, decidedAt string
	if err := row.Scan(&proposal.ProposalID, &proposal.ActionID, &payload, &createdAt, &expiresAt,
		&proposal.State, &decidedAt, &proposal.ResultID); err != nil {
		return AgentProposal{}, err
	}
	var err error
	switch proposal.ActionID {
	case AgentProposalLogDose:
		proposal.Dose = &ProposedDose{}
		err = json.Unmarshal(payload, proposal.Dose)
	case AgentProposalAddTask:
		proposal.Task = &ProposedTask{}
		err = json.Unmarshal(payload, proposal.Task)
	default:
		err = errAgentProposalMalformed
	}
	if err != nil {
		return AgentProposal{}, fmt.Errorf("read proposal %s: %w", proposal.ProposalID, err)
	}
	for _, field := range []struct {
		text  string
		value *time.Time
	}{{createdAt, &proposal.CreatedAt}, {expiresAt, &proposal.ExpiresAt}, {decidedAt, &proposal.DecidedAt}} {
		if field.text == "" {
			continue
		}
		if *field.value, err = time.Parse(time.RFC3339Nano, field.text); err != nil {
			return AgentProposal{}, fmt.Errorf("read proposal %s: %w", proposal.ProposalID, err)
		}
	}
	if proposal.State == AgentProposalPending && !proposal.ExpiresAt.After(now) {
		proposal.State = AgentProposalExpired
	}
	return proposal, nil
}

func loadableZone(id string) bool {
	if id == "" {
		return false
	}
	_, err := time.LoadLocation(id)
	return err == nil
}
