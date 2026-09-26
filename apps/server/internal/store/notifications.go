package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Notification events (C6). When something happens that the owner should
// hear about while no desktop window is open — a visitor asks for a time,
// writes about a request, or a request is answered on another device — a row
// is appended here, and each enrolled device that has chosen to be notified
// reads the feed and raises its own notification.
//
// A row says only what kind of thing happened and to which request: no
// visitor text, no link label, no time. The words a device shows are its own,
// so a notification on a lock screen can never carry private content, even
// from a compromised server.
const (
	NotifyVisitorRequest = "visitor_request"
	NotifyVisitorMessage = "visitor_message"
	NotifyVisitorDecided = "visitor_decided"

	// NotificationRetention bounds the feed. A device that has been off for
	// longer simply starts from what is left; nothing here is a record.
	NotificationRetention = 7 * 24 * time.Hour
)

// ErrNoVisitorProposal means a portal request has not reached the owner's
// queue yet, so there is nothing for a notification to point at. The caller
// retries once it has.
var ErrNoVisitorProposal = errors.New("visitor request has not reached the owner's queue")

// NotificationEvent is one entry in the feed. Seq orders it; EventID makes a
// retried write land once.
type NotificationEvent struct {
	Seq       int64
	EventID   string
	Kind      string
	Subject   string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// recordNotification appends an event once: a retried write with the same id
// is a no-op, so every producer may be at-least-once.
func recordNotification(ctx context.Context, db execer, eventID, kind, subject string, at time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_events
		(event_id, kind, subject, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		eventID, kind, subject, at.UTC().Format(time.RFC3339Nano),
		at.Add(NotificationRetention).UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("record notification: %w", err)
	}
	return nil
}

// RecordVisitorMessageNotification notes that a visitor wrote about a request.
// It needs the request's proposal, which exists once the request has reached
// the owner's queue; before that it returns ErrNoVisitorProposal.
func (s *Store) RecordVisitorMessageNotification(ctx context.Context, portalRequestID, messageID string, at time.Time) error {
	proposalID, err := s.visitorProposalIDFor(ctx, portalRequestID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoVisitorProposal
	}
	if err != nil {
		return err
	}
	return recordNotification(ctx, s.db, NotifyVisitorMessage+":"+messageID, NotifyVisitorMessage, proposalID, at)
}

// NotificationsAfter returns up to limit unexpired events after seq, oldest
// first, and whether more remain.
func (s *Store) NotificationsAfter(ctx context.Context, afterSeq int64, limit int, now time.Time) ([]NotificationEvent, bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT seq, event_id, kind, subject, created_at, expires_at
		FROM notification_events WHERE seq > ? AND expires_at > ? ORDER BY seq LIMIT ?`,
		afterSeq, now.UTC().Format(time.RFC3339Nano), limit+1)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	events := []NotificationEvent{}
	for rows.Next() {
		var (
			event              NotificationEvent
			createdAt, expires string
		)
		if err := rows.Scan(&event.Seq, &event.EventID, &event.Kind, &event.Subject, &createdAt, &expires); err != nil {
			return nil, false, err
		}
		if event.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, false, err
		}
		if event.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires); err != nil {
			return nil, false, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	return events, hasMore, nil
}

// NotificationHead is the newest event's sequence number, or zero.
func (s *Store) NotificationHead(ctx context.Context) (int64, error) {
	var head sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(seq) FROM notification_events`).Scan(&head); err != nil {
		return 0, err
	}
	return head.Int64, nil
}

// PurgeNotifications deletes events past their retention.
func (s *Store) PurgeNotifications(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM notification_events WHERE expires_at <= ?`,
		now.UTC().Format(time.RFC3339Nano))
	return err
}
