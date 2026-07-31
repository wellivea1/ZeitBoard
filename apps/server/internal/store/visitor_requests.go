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
)

// ActionVisitorRequest is the action id of a proposal that originated from a
// share-link visitor rather than from the user or an agent.
//
// It is deliberately absent from the assistant's action registry. An agent
// must not be able to mint a proposal that presents itself as coming from an
// outside person: that would let a model manufacture social pressure the user
// cannot verify. Only the portal bridge creates these.
const ActionVisitorRequest = "place_visitor_request"

var (
	ErrVisitorRequestExists   = errors.New("visitor request already has a proposal")
	ErrVisitorSlotOutOfWindow = errors.New("chosen block is outside the requested window")
	ErrNotVisitorProposal     = errors.New("proposal did not originate from a visitor")
)

// VisitorRequestInput is what the portal bridge hands over. Handle and message
// are visitor-authored private text; they are stored inside the proposal's
// encrypted payload so the owner can read them, and they never travel onward
// to a provider, projection, or notification body.
type VisitorRequestInput struct {
	PortalRequestID string
	ProfileID       string
	DeviceID        string
	WindowStart     time.Time
	WindowEnd       time.Time
	ZoneID          string
	DurationMinutes int
	BeyondHorizon   bool
	Handle          string
	Message         string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// VisitorProposalPayload is the decrypted proposal body.
type VisitorProposalPayload struct {
	ProposalID      string    `json:"proposalId"`
	ActionID        string    `json:"actionId"`
	Origin          string    `json:"origin"`
	PortalRequestID string    `json:"portalRequestId"`
	ProfileID       string    `json:"profileId"`
	WindowStart     time.Time `json:"windowStartAt"`
	WindowEnd       time.Time `json:"windowEndAt"`
	ZoneID          string    `json:"zoneId"`
	DurationMinutes int       `json:"durationMinutes,omitempty"`
	BeyondHorizon   bool      `json:"beyondHorizon"`
	Handle          string    `json:"handle,omitempty"`
	Message         string    `json:"message,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

// VisitorSlot is the exact block the owner picked inside the requested window.
type VisitorSlot struct {
	StartAt time.Time
	EndAt   time.Time
}

// CreateVisitorProposal is idempotent on the portal request id. The bridge may
// retry after any failure, including one where the private commit succeeded
// but the acknowledgement was lost, and still produce exactly one proposal.
func (s *Store) CreateVisitorProposal(ctx context.Context, input VisitorRequestInput) (ProposalRecord, error) {
	if input.PortalRequestID == "" || input.ProfileID == "" || input.DeviceID == "" {
		return ProposalRecord{}, errors.New("visitor proposal requires a portal request, profile, and device")
	}
	if !input.WindowEnd.After(input.WindowStart) {
		return ProposalRecord{}, errors.New("visitor request window must be non-empty")
	}

	if existing, err := s.visitorProposalIDFor(ctx, input.PortalRequestID); err == nil {
		record, err := s.ProposalByID(ctx, existing)
		if err != nil {
			return ProposalRecord{}, err
		}
		return record, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ProposalRecord{}, err
	}

	proposalID, err := newVisitorProposalID()
	if err != nil {
		return ProposalRecord{}, err
	}
	payload, err := json.Marshal(VisitorProposalPayload{
		ProposalID:      proposalID,
		ActionID:        ActionVisitorRequest,
		Origin:          "visitor",
		PortalRequestID: input.PortalRequestID,
		ProfileID:       input.ProfileID,
		WindowStart:     input.WindowStart.UTC(),
		WindowEnd:       input.WindowEnd.UTC(),
		ZoneID:          input.ZoneID,
		DurationMinutes: input.DurationMinutes,
		BeyondHorizon:   input.BeyondHorizon,
		Handle:          input.Handle,
		Message:         input.Message,
		CreatedAt:       input.CreatedAt.UTC(),
		ExpiresAt:       input.ExpiresAt.UTC(),
	})
	if err != nil {
		return ProposalRecord{}, err
	}

	record, err := s.CreateProposal(ctx, ProposalInput{
		ID:        proposalID,
		ActionID:  ActionVisitorRequest,
		DeviceID:  input.DeviceID,
		CreatedAt: input.CreatedAt,
		ExpiresAt: input.ExpiresAt,
		Payload:   payload,
		// The audit records the origin, never the visitor's text.
		Audit: json.RawMessage(`{"source":"portal","event":"proposal_created","origin":"visitor"}`),
	})
	if err != nil {
		return ProposalRecord{}, err
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO portal_request_proposals (portal_request_id, proposal_id, profile_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		input.PortalRequestID, proposalID, input.ProfileID, input.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return ProposalRecord{}, fmt.Errorf("link visitor proposal: %w", err)
	}
	return record, nil
}

func (s *Store) visitorProposalIDFor(ctx context.Context, portalRequestID string) (string, error) {
	var proposalID string
	err := s.db.QueryRowContext(ctx,
		`SELECT proposal_id FROM portal_request_proposals WHERE portal_request_id = ?`,
		portalRequestID).Scan(&proposalID)
	return proposalID, err
}

// DecideVisitorProposal records the owner's answer. Approval requires an exact
// block inside the requested window: the visitor asked for a range, and the
// owner is the only party who may narrow it to a time.
//
// The decision, the one-use token consumption, and the status handoff row all
// commit together, so a decision the visitor never learns about is impossible
// without also losing the decision itself.
func (s *Store) DecideVisitorProposal(
	ctx context.Context,
	proposalID, deviceID string,
	decision ProposalStatus,
	token string,
	slot VisitorSlot,
	decidedAt time.Time,
) (ProposalRecord, error) {
	record, err := s.ProposalByID(ctx, proposalID)
	if err != nil {
		return ProposalRecord{}, err
	}
	if record.ActionID != ActionVisitorRequest {
		return ProposalRecord{}, ErrNotVisitorProposal
	}
	var payload VisitorProposalPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return ProposalRecord{}, err
	}

	status := statusForDecision(decision)
	if decision == ProposalApproved {
		if err := validateVisitorSlot(payload, slot); err != nil {
			return ProposalRecord{}, err
		}
	} else {
		slot = VisitorSlot{}
	}

	audit, err := json.Marshal(map[string]string{
		"source": "portal",
		"event":  "visitor_request_" + status,
		"origin": "visitor",
	})
	if err != nil {
		return ProposalRecord{}, err
	}

	return s.decideProposal(ctx, proposalID, deviceID, decision, token, decidedAt, audit,
		func(ctx context.Context, tx *sql.Tx, _ ProposalRecord, _ ProposalStatus, at time.Time) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO portal_status_outbox
				(portal_request_id, status, decided_start, decided_end, created_at)
				VALUES (?, ?, ?, ?, ?)`,
				payload.PortalRequestID, status,
				formatOptionalTime(slot.StartAt), formatOptionalTime(slot.EndAt),
				at.UTC().Format(time.RFC3339Nano))
			return err
		})
}

// validateVisitorSlot enforces the one substantive rule of approval: the owner
// may pick any block inside the window the visitor asked for, and nothing
// outside it. A counteroffer needs a new request, not a silently moved time.
func validateVisitorSlot(payload VisitorProposalPayload, slot VisitorSlot) error {
	if slot.StartAt.IsZero() || slot.EndAt.IsZero() {
		return fmt.Errorf("%w: approval requires a chosen block", ErrVisitorSlotOutOfWindow)
	}
	start := slot.StartAt.UTC()
	end := slot.EndAt.UTC()
	if !end.After(start) {
		return fmt.Errorf("%w: the block must end after it starts", ErrVisitorSlotOutOfWindow)
	}
	if start.Before(payload.WindowStart) || end.After(payload.WindowEnd) {
		return ErrVisitorSlotOutOfWindow
	}
	if payload.DurationMinutes > 0 {
		// A supplied duration must be preserved: the visitor asked for a
		// meeting of that length, and shortening it silently changes the ask.
		wanted := time.Duration(payload.DurationMinutes) * time.Minute
		if end.Sub(start) != wanted {
			return fmt.Errorf("%w: the block must be exactly %d minutes", ErrVisitorSlotOutOfWindow, payload.DurationMinutes)
		}
	}
	return nil
}

func statusForDecision(decision ProposalStatus) string {
	if decision == ProposalApproved {
		return "approved"
	}
	return "declined"
}

// PortalStatusEntry is one undelivered decision bound for the portal store.
type PortalStatusEntry struct {
	ID              int64
	PortalRequestID string
	Status          string
	DecidedStart    time.Time
	DecidedEnd      time.Time
}

// PendingPortalStatus lists decisions the portal has not yet applied.
func (s *Store) PendingPortalStatus(ctx context.Context, limit int) ([]PortalStatusEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, portal_request_id, status, decided_start, decided_end
		 FROM portal_status_outbox ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []PortalStatusEntry
	for rows.Next() {
		var (
			entry      PortalStatusEntry
			startRaw   string
			endRaw     string
			parseError error
		)
		if err := rows.Scan(&entry.ID, &entry.PortalRequestID, &entry.Status, &startRaw, &endRaw); err != nil {
			return nil, err
		}
		if entry.DecidedStart, parseError = parseOptionalTime(startRaw); parseError != nil {
			return nil, parseError
		}
		if entry.DecidedEnd, parseError = parseOptionalTime(endRaw); parseError != nil {
			return nil, parseError
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// AckPortalStatus removes a delivered decision.
func (s *Store) AckPortalStatus(ctx context.Context, entryID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM portal_status_outbox WHERE id = ?`, entryID)
	return err
}

// VisitorProposalPayloadOf decodes a visitor proposal for the owner UI.
func VisitorProposalPayloadOf(record ProposalRecord) (VisitorProposalPayload, error) {
	if record.ActionID != ActionVisitorRequest {
		return VisitorProposalPayload{}, ErrNotVisitorProposal
	}
	var payload VisitorProposalPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return VisitorProposalPayload{}, err
	}
	return payload, nil
}

func newVisitorProposalID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "visitor-" + hexString(raw), nil
}

func hexString(raw []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(raw)*2)
	for _, b := range raw {
		out = append(out, digits[b>>4], digits[b&0x0f])
	}
	return string(out)
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseOptionalTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}
