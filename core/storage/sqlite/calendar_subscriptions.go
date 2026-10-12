package sqlite

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
)

// Calendars by the private address their provider publishes (ADR-0062). A
// subscribed calendar is an imported ics source like any other; its address,
// a secret anyone could read the calendar with, is kept here and nowhere
// else, and goes with the source.

// CalendarSubscription is a subscribed calendar and how its last refresh went.
type CalendarSubscription struct {
	SourceID string
	Label    string
	Address  string

	// RefreshedAt is when the kept copy was fetched, and AttemptedAt when a
	// fetch was last tried. Failure says why that try failed, or is empty.
	RefreshedAt time.Time
	AttemptedAt time.Time
	Failure     string
}

func (s *Store) initializeCalendarSubscriptions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS local_calendar_subscriptions (
		source_id TEXT PRIMARY KEY,
		address TEXT NOT NULL,
		refreshed_at TEXT NOT NULL,
		attempted_at TEXT NOT NULL,
		failure TEXT NOT NULL DEFAULT '',
		FOREIGN KEY(source_id) REFERENCES local_calendar_sources(source_id) ON DELETE CASCADE
	)`)
	if err != nil {
		return fmt.Errorf("migrate calendar subscriptions: %w", err)
	}
	return nil
}

// ReplaceSubscribedCalendar keeps a fetched copy of a subscribed calendar
// and its address together: either both are kept or neither is.
func (s *Store) ReplaceSubscribedCalendar(ctx context.Context, source calendarcore.Source, events []calendarcore.Event, address string) error {
	if source.Kind != calendarcore.SourceICS {
		return errors.New("a subscribed calendar is an ics source")
	}
	if err := validateSubscriptionAddress(address); err != nil {
		return err
	}
	if err := validateImportedCalendar(source, events, ""); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := replaceImportedCalendarTx(ctx, tx, source, events, ""); err != nil {
		return err
	}
	fetched := formatSQLiteTime(source.LastImportedAt)
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_calendar_subscriptions(source_id, address, refreshed_at, attempted_at, failure)
		VALUES(?, ?, ?, ?, '')
		ON CONFLICT(source_id) DO UPDATE SET address = excluded.address, refreshed_at = excluded.refreshed_at,
			attempted_at = excluded.attempted_at, failure = ''`,
		source.SourceID, address, fetched, fetched); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordCalendarSubscriptionFailure notes a refresh that failed. The copy
// kept from the last good one stays.
func (s *Store) RecordCalendarSubscriptionFailure(ctx context.Context, sourceID string, at time.Time, failure string) error {
	if strings.TrimSpace(failure) == "" {
		return errors.New("a failed refresh needs its reason")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE local_calendar_subscriptions SET attempted_at = ?, failure = ? WHERE source_id = ?`,
		formatSQLiteTime(at), failure, sourceID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed == 0 {
		return errors.Join(err, ErrCalendarSourceNotFound)
	}
	return nil
}

// CalendarSubscriptions lists the subscribed calendars by label.
func (s *Store) CalendarSubscriptions(ctx context.Context) ([]CalendarSubscription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT subscription.source_id, source.label, subscription.address,
			subscription.refreshed_at, subscription.attempted_at, subscription.failure
		FROM local_calendar_subscriptions AS subscription
		JOIN local_calendar_sources AS source ON source.source_id = subscription.source_id
		ORDER BY source.label, subscription.source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subscriptions []CalendarSubscription
	for rows.Next() {
		var item CalendarSubscription
		var refreshed, attempted string
		if err := rows.Scan(&item.SourceID, &item.Label, &item.Address, &refreshed, &attempted, &item.Failure); err != nil {
			return nil, err
		}
		if item.RefreshedAt, err = parseCalendarTime(refreshed); err != nil {
			return nil, err
		}
		if item.AttemptedAt, err = parseCalendarTime(attempted); err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, item)
	}
	return subscriptions, rows.Err()
}

// validateSubscriptionAddress accepts an absolute HTTPS address without
// credentials, or plain HTTP on loopback for testing against a local server.
func validateSubscriptionAddress(address string) error {
	parsed, err := url.Parse(address)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("a calendar address must be an absolute URL without credentials or fragment")
	}
	if strings.EqualFold(parsed.Scheme, "https") || (strings.EqualFold(parsed.Scheme, "http") && isLoopbackHost(parsed.Hostname())) {
		return nil
	}
	return errors.New("a calendar address must use HTTPS")
}
