package portal

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// OutboxEntry is one pending handoff to the private side. The portal never
// calls the private store itself; a bridge polls these and reports back.
type OutboxEntry struct {
	ID             int64
	Kind           string
	RequestID      string
	IdempotencyKey string
	Attempts       int
	Request        Request
}

// PendingOutbox returns undelivered handoffs oldest first.
func (s *Store) PendingOutbox(ctx context.Context, limit int) ([]OutboxEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, request_id, idempotency_key, attempts
		 FROM portal_outbox ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	var entries []OutboxEntry
	for rows.Next() {
		var entry OutboxEntry
		if err := rows.Scan(&entry.ID, &entry.Kind, &entry.RequestID, &entry.IdempotencyKey, &entry.Attempts); err != nil {
			_ = rows.Close()
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// Load each request after the cursor is closed: this store runs on a
	// single connection, so querying while rows are open would deadlock.
	loaded := make([]OutboxEntry, 0, len(entries))
	for _, entry := range entries {
		request, err := s.requestByID(ctx, entry.RequestID)
		if err != nil {
			if errors.Is(err, ErrRequestNotFound) {
				// The request was erased under the bridge. Drop the handoff
				// rather than retrying forever.
				if _, delErr := s.db.ExecContext(ctx, `DELETE FROM portal_outbox WHERE id = ?`, entry.ID); delErr != nil {
					return nil, delErr
				}
				continue
			}
			return nil, err
		}
		entry.Request = request
		loaded = append(loaded, entry)
	}
	return loaded, nil
}

func (s *Store) requestByID(ctx context.Context, requestID string) (Request, error) {
	row := s.db.QueryRowContext(ctx, `SELECT request_id, profile_id, status,
		window_start, window_end, zone_id, duration_minutes, beyond_horizon,
		decided_start, decided_end, created_at, updated_at, nonce, ciphertext
		FROM portal_requests WHERE request_id = ?`, requestID)
	return s.scanRequest(row)
}

// AckOutbox marks a handoff delivered and moves the request from the honest
// `queued` state to `pending`. Both happen in one transaction so a visitor
// never sees "waiting for a decision" for a request the owner never received.
func (s *Store) AckOutbox(ctx context.Context, entryID int64, requestID string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM portal_outbox WHERE id = ?`, entryID); err != nil {
		return err
	}
	// Only a queued request advances. A request already decided by a racing
	// status delivery must not be dragged back to pending.
	if _, err := tx.ExecContext(ctx,
		`UPDATE portal_requests SET status = ?, updated_at = ? WHERE request_id = ? AND status = ?`,
		RequestPending, formatTime(now), requestID, RequestQueued); err != nil {
		return err
	}
	return tx.Commit()
}

// NoteOutboxFailure records an attempt without dropping the handoff. The
// request stays `queued`, which is what the visitor is shown.
func (s *Store) NoteOutboxFailure(ctx context.Context, entryID int64, reason string) error {
	// The reason is bounded and stored for the operator; it is never rendered
	// to a visitor and never joins the access audit.
	if len(reason) > 200 {
		reason = reason[:200]
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE portal_outbox SET attempts = attempts + 1, last_error = ? WHERE id = ?`, reason, entryID)
	return err
}

// ApplyDecision records the owner's answer against a request. It is
// idempotent: replaying the same decision is a no-op, which is what lets the
// private side retry delivery without coordination.
func (s *Store) ApplyDecision(ctx context.Context, requestID, status string, decidedStart, decidedEnd time.Time, now time.Time) error {
	switch status {
	case RequestApproved, RequestDeclined, RequestClosed:
	default:
		return fmt.Errorf("unsupported portal request status %q", status)
	}
	if status == RequestApproved && (decidedStart.IsZero() || !decidedEnd.After(decidedStart)) {
		return errors.New("an approved request requires the exact chosen block")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE portal_requests
		 SET status = ?, decided_start = ?, decided_end = ?, updated_at = ?
		 WHERE request_id = ? AND status IN (?, ?)`,
		status, formatTime(decidedStart), formatTime(decidedEnd), formatTime(now),
		requestID, RequestQueued, RequestPending)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		// Either the request is gone or it was already decided. Both are
		// acceptable outcomes for a retry, so this is not an error.
		return nil
	}
	return nil
}

// ListRequestsForProfile is the owner-side read used when a link is revoked or
// erased, so open requests can be closed rather than left dangling.
func (s *Store) ListRequestsForProfile(ctx context.Context, profileID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT request_id FROM portal_requests WHERE profile_id = ? ORDER BY created_at`, profileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
