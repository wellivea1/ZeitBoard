package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	medicationcore "non24.app/core/medication"
)

// A dose an agent asked to record waits here, on this computer only, until the
// owner records or discards it or it lapses (ADR-0051). A dose is a health
// record, so only the owner's decision makes one: recording a proposal appends
// the same medication event a hand-logged dose would. Proposals are never
// synced or exported, and erasing a medication erases its proposals.

// The states of a dose proposal. A pending proposal past its expiry reads as
// expired; nothing rewrites it.
const (
	DoseProposalPending   = "pending"
	DoseProposalRecorded  = "recorded"
	DoseProposalDiscarded = "discarded"
	DoseProposalExpired   = "expired"
)

const (
	// DoseProposalLifetime is how long a proposed dose waits: long enough to
	// span a sleep, short enough not to meet the next day's dose.
	DoseProposalLifetime = 24 * time.Hour
	// MaxPendingDoseProposals bounds what agents can queue for the owner.
	MaxPendingDoseProposals = 20
	// doseProposalHistory is how long a decided or lapsed proposal stays in
	// the decision history.
	doseProposalHistory   = 30 * 24 * time.Hour
	doseProposalListLimit = 100
)

var (
	ErrDoseProposalNotFound  = errors.New("dose proposal not found")
	ErrDoseProposalDecided   = errors.New("dose proposal was already decided")
	ErrDoseProposalExpired   = errors.New("dose proposal has lapsed")
	ErrTooManyDoseProposals  = errors.New("too many doses are already waiting for the owner")
	ErrMedicationNotActive   = errors.New("medication is not active")
	errDoseProposalMalformed = errors.New("dose proposal is malformed")
)

// DoseProposal is one dose an agent asked the owner to record.
type DoseProposal struct {
	ProposalID   string
	MedicationID string
	Status       string // taken or skipped
	DoseAt       time.Time
	ZoneID       string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	State        string
	// DecidedAt and EventID are set once the owner decides; EventID names the
	// medication event a recorded proposal became.
	DecidedAt time.Time
	EventID   string
}

func (s *Store) initializeDoseProposals(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS local_dose_proposals(
			proposal_id TEXT PRIMARY KEY,
			medication_id TEXT NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('taken', 'skipped')),
			dose_at TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			decision TEXT NOT NULL CHECK(decision IN ('pending', 'recorded', 'discarded')),
			decided_at TEXT NOT NULL DEFAULT '',
			event_id TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(medication_id) REFERENCES local_medications(medication_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_dose_proposals_created
			ON local_dose_proposals(created_at, proposal_id)`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate dose proposals: %w", err)
		}
	}
	return nil
}

// ProposeDose queues a dose for the owner at now. The medication must exist
// and be active, and at most MaxPendingDoseProposals may wait at once. It
// lapses DoseProposalLifetime after now.
func (s *Store) ProposeDose(ctx context.Context, proposal DoseProposal, now time.Time) error {
	now = now.UTC()
	if !medicationcore.ValidIdentifier(proposal.ProposalID) || !medicationcore.ValidIdentifier(proposal.MedicationID) ||
		(proposal.Status != MedicationEventTaken && proposal.Status != MedicationEventSkipped) ||
		proposal.DoseAt.IsZero() || proposal.ZoneID == "" {
		return errDoseProposalMalformed
	}
	if _, err := time.LoadLocation(proposal.ZoneID); err != nil {
		return errDoseProposalMalformed
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cutoff := formatSQLiteTime(now.Add(-doseProposalHistory))
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_dose_proposals
		WHERE (decision = 'pending' AND expires_at < ?) OR (decision != 'pending' AND decided_at < ?)`, cutoff, cutoff); err != nil {
		return err
	}
	var active bool
	err = tx.QueryRowContext(ctx, `SELECT active FROM local_medications WHERE medication_id = ?`, proposal.MedicationID).Scan(&active)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrMedicationNotFound
	case err != nil:
		return err
	case !active:
		return ErrMedicationNotActive
	}
	var waiting int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_dose_proposals WHERE decision = 'pending' AND expires_at > ?`,
		formatSQLiteTime(now)).Scan(&waiting); err != nil {
		return err
	}
	if waiting >= MaxPendingDoseProposals {
		return ErrTooManyDoseProposals
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_dose_proposals(
		proposal_id, medication_id, status, dose_at, zone_id, created_at, expires_at, decision
	) VALUES(?, ?, ?, ?, ?, ?, ?, 'pending')`,
		proposal.ProposalID, proposal.MedicationID, proposal.Status, formatSQLiteTime(proposal.DoseAt),
		proposal.ZoneID, formatSQLiteTime(now), formatSQLiteTime(now.Add(DoseProposalLifetime)),
	); err != nil {
		return err
	}
	return tx.Commit()
}

// DoseProposals lists the waiting proposals and the recent history at now,
// newest first.
func (s *Store) DoseProposals(ctx context.Context, now time.Time) ([]DoseProposal, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT proposal_id, medication_id, status, dose_at, zone_id,
		created_at, expires_at, decision, decided_at, event_id
		FROM local_dose_proposals ORDER BY created_at DESC, proposal_id DESC LIMIT ?`, doseProposalListLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	proposals := make([]DoseProposal, 0)
	for rows.Next() {
		proposal, err := scanDoseProposal(rows, now)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	return proposals, rows.Err()
}

// RecordProposedDose makes a waiting proposal the owner's record: in one
// transaction it appends the medication event a hand-logged dose would, under
// eventID and recorded at now, and marks the proposal recorded.
func (s *Store) RecordProposedDose(ctx context.Context, proposalID, eventID string, now time.Time) (MedicationEventRecord, error) {
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MedicationEventRecord{}, err
	}
	defer tx.Rollback()
	proposal, err := pendingDoseProposalTx(ctx, tx, proposalID, now)
	if err != nil {
		return MedicationEventRecord{}, err
	}
	record := MedicationEventRecord{
		EventID:      eventID,
		MedicationID: proposal.MedicationID,
		DoseAt:       proposal.DoseAt,
		ZoneID:       proposal.ZoneID,
		Status:       proposal.Status,
		// Adherence counts only doses the owner marks scheduled (ADR-0027).
		Scheduled: false,
		Provenance: SleepObservationProvenance{
			AcquisitionMethod: ProvenanceAcquisitionManual,
			EvidenceStatus:    ProvenanceEvidenceUserReported,
			RecordedAt:        now,
		},
	}
	if err := appendMedicationEventTx(ctx, tx, record); err != nil {
		return MedicationEventRecord{}, err
	}
	if err := decideDoseProposalTx(ctx, tx, proposalID, DoseProposalRecorded, eventID, now); err != nil {
		return MedicationEventRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return MedicationEventRecord{}, err
	}
	return normalizeMedicationEvent(record), nil
}

// DiscardProposedDose declines a waiting proposal; nothing is recorded.
func (s *Store) DiscardProposedDose(ctx context.Context, proposalID string, now time.Time) error {
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := pendingDoseProposalTx(ctx, tx, proposalID, now); err != nil {
		return err
	}
	if err := decideDoseProposalTx(ctx, tx, proposalID, DoseProposalDiscarded, "", now); err != nil {
		return err
	}
	return tx.Commit()
}

func pendingDoseProposalTx(ctx context.Context, tx *sql.Tx, proposalID string, now time.Time) (DoseProposal, error) {
	row := tx.QueryRowContext(ctx, `SELECT proposal_id, medication_id, status, dose_at, zone_id,
		created_at, expires_at, decision, decided_at, event_id
		FROM local_dose_proposals WHERE proposal_id = ?`, proposalID)
	proposal, err := scanDoseProposal(row, now)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return DoseProposal{}, ErrDoseProposalNotFound
	case err != nil:
		return DoseProposal{}, err
	case proposal.State == DoseProposalExpired:
		return DoseProposal{}, ErrDoseProposalExpired
	case proposal.State != DoseProposalPending:
		return DoseProposal{}, ErrDoseProposalDecided
	}
	return proposal, nil
}

func decideDoseProposalTx(ctx context.Context, tx *sql.Tx, proposalID, decision, eventID string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE local_dose_proposals SET decision = ?, decided_at = ?, event_id = ?
		WHERE proposal_id = ? AND decision = 'pending'`, decision, formatSQLiteTime(now), eventID, proposalID)
	return err
}

func scanDoseProposal(row rowScanner, now time.Time) (DoseProposal, error) {
	var proposal DoseProposal
	var doseAt, createdAt, expiresAt, decidedAt string
	if err := row.Scan(&proposal.ProposalID, &proposal.MedicationID, &proposal.Status, &doseAt, &proposal.ZoneID,
		&createdAt, &expiresAt, &proposal.State, &decidedAt, &proposal.EventID); err != nil {
		return DoseProposal{}, err
	}
	var err error
	for _, field := range []struct {
		text  string
		value *time.Time
	}{{doseAt, &proposal.DoseAt}, {createdAt, &proposal.CreatedAt}, {expiresAt, &proposal.ExpiresAt}, {decidedAt, &proposal.DecidedAt}} {
		if field.text == "" {
			continue
		}
		if *field.value, err = time.Parse(time.RFC3339Nano, field.text); err != nil {
			return DoseProposal{}, fmt.Errorf("read dose proposal %s: %w", proposal.ProposalID, err)
		}
	}
	if proposal.State == DoseProposalPending && !proposal.ExpiresAt.After(now) {
		proposal.State = DoseProposalExpired
	}
	return proposal, nil
}
