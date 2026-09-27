package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
)

// Writing accepted times to the owner's CalDAV calendar (ADR-0053). Off until
// the owner turns it on for one imported CalDAV calendar and gives ZeitBoard a
// sign-in for it. Then every accepted time that has not ended is written there
// as ZeitBoard's own event, and undoing one removes it, unless the owner has
// changed it in their calendar since. ZeitBoard writes and removes only the
// events it created, named by their placement; it never changes an imported
// event.
//
// Triggers queue the writes, so every path that adds or removes an accepted
// time (accepting, undoing, deleting the task, erasing data) queues the right
// one without code of its own.

// The states of one written time.
const (
	CalendarWriteCreate   = "create"   // accepted; not yet in the calendar
	CalendarWriteWritten  = "written"  // in the calendar, under Href at ETag
	CalendarWriteDelete   = "delete"   // undone; still in the calendar
	CalendarWriteConflict = "conflict" // needs the owner: see Detail
)

var (
	ErrCalendarWriteTargetNotCalDAV = errors.New("accepted times can be written only to an imported CalDAV calendar")
	ErrCalendarWriteNotFound        = errors.New("no such written time")
)

// CalendarWriteTarget is the calendar accepted times are written to, and the
// sign-in to write with. The sign-in is a credential: it stays in this
// owner-protected database, is never exported or synced, and is erased when
// writing stops.
type CalendarWriteTarget struct {
	SourceID      string
	Label         string
	CollectionURL string
	Username      string
	Password      string
	EnabledAt     time.Time
}

// CalendarWrite is one accepted time's place in the owner's calendar.
type CalendarWrite struct {
	EventID       string
	State         string
	Href          string
	ETag          string
	Title         string
	StartAt       time.Time
	Attempts      int
	NextAttemptAt time.Time // zero: due now
	Detail        string    // why the last attempt failed, or what the conflict is
}

func (s *Store) initializeCalendarWriteBack(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS local_calendar_write_target(
			id INTEGER PRIMARY KEY CHECK(id = 1),
			source_id TEXT NOT NULL,
			collection_url TEXT NOT NULL,
			username TEXT NOT NULL,
			password TEXT NOT NULL,
			enabled_at TEXT NOT NULL,
			FOREIGN KEY(source_id) REFERENCES local_calendar_sources(source_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS local_calendar_writes(
			event_id TEXT PRIMARY KEY,
			state TEXT NOT NULL CHECK(state IN ('create', 'written', 'delete', 'conflict')),
			href TEXT NOT NULL DEFAULT '',
			etag TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			start_at TEXT NOT NULL DEFAULT '',
			attempts INTEGER NOT NULL DEFAULT 0,
			next_attempt_at TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		// A time accepted while writing is on is written.
		`CREATE TRIGGER IF NOT EXISTS local_calendar_write_on_accept AFTER INSERT ON local_calendar_events
		WHEN NEW.ownership = 'app_owned' AND EXISTS(SELECT 1 FROM local_calendar_write_target) BEGIN
		INSERT OR IGNORE INTO local_calendar_writes(event_id, state, title, start_at, updated_at)
		VALUES(NEW.event_id, 'create', NEW.title, NEW.start_at, strftime('%Y-%m-%dT%H:%M:%fZ', 'now')); END`,
		// A time that goes (undone, its task deleted, data erased) is not
		// written if it was not yet, and is removed if it was.
		`CREATE TRIGGER IF NOT EXISTS local_calendar_write_on_remove AFTER DELETE ON local_calendar_events
		WHEN OLD.ownership = 'app_owned' BEGIN
		DELETE FROM local_calendar_writes WHERE event_id = OLD.event_id AND state = 'create';
		UPDATE local_calendar_writes SET state = 'delete', attempts = 0, next_attempt_at = '', detail = '',
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE event_id = OLD.event_id AND state = 'written'; END`,
		// Without a calendar to write to, ZeitBoard forgets what it wrote; the
		// owner's calendar keeps it.
		`CREATE TRIGGER IF NOT EXISTS local_calendar_write_on_stop AFTER DELETE ON local_calendar_write_target BEGIN
		DELETE FROM local_calendar_writes; END`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate calendar write-back: %w", err)
		}
	}
	return nil
}

// CalendarWriteTarget returns the calendar accepted times are written to, if
// writing is on.
func (s *Store) CalendarWriteTarget(ctx context.Context) (CalendarWriteTarget, bool, error) {
	var target CalendarWriteTarget
	var enabledAt string
	err := s.db.QueryRowContext(ctx, `SELECT target.source_id, source.label, target.collection_url, target.username,
		target.password, target.enabled_at
		FROM local_calendar_write_target AS target JOIN local_calendar_sources AS source ON source.source_id = target.source_id`,
	).Scan(&target.SourceID, &target.Label, &target.CollectionURL, &target.Username, &target.Password, &enabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CalendarWriteTarget{}, false, nil
	}
	if err != nil {
		return CalendarWriteTarget{}, false, err
	}
	if target.EnabledAt, err = time.Parse(time.RFC3339Nano, enabledAt); err != nil {
		return CalendarWriteTarget{}, false, err
	}
	return target, true, nil
}

// SetCalendarWriteTarget turns writing on for an imported CalDAV calendar at
// now, replacing any other target, and queues every accepted time that has
// not ended.
func (s *Store) SetCalendarWriteTarget(ctx context.Context, target CalendarWriteTarget, now time.Time) error {
	if err := s.FilePermissionError(); err != nil {
		return errors.New("a calendar sign-in requires owner-only local storage")
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var kind, endpoint string
	err = tx.QueryRowContext(ctx, `SELECT kind, endpoint FROM local_calendar_sources WHERE source_id = ?`, target.SourceID).Scan(&kind, &endpoint)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrCalendarSourceNotFound
	case err != nil:
		return err
	case kind != string(calendarcore.SourceCalDAV) || endpoint == "":
		return ErrCalendarWriteTargetNotCalDAV
	}
	// Replacing the target forgets what was written to the old one.
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_calendar_write_target`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_calendar_write_target(id, source_id, collection_url, username, password, enabled_at)
		VALUES(1, ?, ?, ?, ?, ?)`, target.SourceID, endpoint, target.Username, target.Password, formatSQLiteTime(now)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_calendar_writes(event_id, state, title, start_at, updated_at)
		SELECT event_id, 'create', title, start_at, ? FROM local_calendar_events
		WHERE ownership = 'app_owned' AND end_at > ?`, formatSQLiteTime(now), formatSQLiteTime(now)); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearCalendarWriteTarget turns writing off and erases the sign-in. Times
// already written stay in the owner's calendar.
func (s *Store) ClearCalendarWriteTarget(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM local_calendar_write_target`); err != nil {
		return err
	}
	return s.compactDeletedData(ctx)
}

// DueCalendarWrites lists the writes due at now, earliest time first.
func (s *Store) DueCalendarWrites(ctx context.Context, now time.Time, limit int) ([]CalendarWrite, error) {
	rows, err := s.db.QueryContext(ctx, calendarWriteSelect+` WHERE state IN ('create', 'delete')
		AND (next_attempt_at = '' OR next_attempt_at <= ?) ORDER BY start_at, event_id LIMIT ?`, formatSQLiteTime(now.UTC()), limit)
	if err != nil {
		return nil, err
	}
	return scanCalendarWrites(rows)
}

// CalendarWrites lists every write ZeitBoard is tracking, earliest time first.
func (s *Store) CalendarWrites(ctx context.Context) ([]CalendarWrite, error) {
	rows, err := s.db.QueryContext(ctx, calendarWriteSelect+` ORDER BY start_at, event_id`)
	if err != nil {
		return nil, err
	}
	return scanCalendarWrites(rows)
}

// CalendarWriteEvent is the accepted time a create writes.
func (s *Store) CalendarWriteEvent(ctx context.Context, eventID string) (calendarcore.Event, bool, error) {
	event, err := scanCalendarEvent(s.db.QueryRowContext(ctx, `SELECT
		event_id, source_id, source_record_id, title, start_at, end_at, zone_id,
		all_day, busy, ownership, created_at, location, notes, task_id, task_revision, proposal_id
		FROM local_calendar_events WHERE event_id = ? AND ownership = 'app_owned'`, eventID))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return calendarcore.Event{}, false, nil
	case err != nil:
		return calendarcore.Event{}, false, err
	}
	return event, true, nil
}

// MarkCalendarWritten records that an accepted time is in the calendar. If it
// was undone while being written, it is queued for removal instead.
func (s *Store) MarkCalendarWritten(ctx context.Context, event calendarcore.Event, href, etag string, now time.Time) error {
	stamp := formatSQLiteTime(now.UTC())
	result, err := s.db.ExecContext(ctx, `UPDATE local_calendar_writes SET state = 'written', href = ?, etag = ?,
		attempts = 0, next_attempt_at = '', detail = '', updated_at = ? WHERE event_id = ? AND state = 'create'`,
		href, etag, stamp, event.EventID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 1 {
		return nil
	}
	// Undone while it was being written: the undo found nothing to remove, so
	// remove what was just written.
	_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO local_calendar_writes(event_id, state, href, etag, title, start_at, updated_at)
		SELECT ?, 'delete', ?, ?, ?, ?, ? WHERE EXISTS(SELECT 1 FROM local_calendar_write_target)`,
		event.EventID, href, etag, event.Title, formatSQLiteTime(event.StartAt.UTC()), stamp)
	return err
}

// MarkCalendarWriteRemoved records that an undone time left the calendar.
func (s *Store) MarkCalendarWriteRemoved(ctx context.Context, eventID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM local_calendar_writes WHERE event_id = ? AND state = 'delete'`, eventID)
	return err
}

// MarkCalendarWriteConflict leaves a write for the owner to settle.
func (s *Store) MarkCalendarWriteConflict(ctx context.Context, eventID, detail string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE local_calendar_writes SET state = 'conflict', detail = ?, updated_at = ?
		WHERE event_id = ?`, detail, formatSQLiteTime(now.UTC()), eventID)
	return err
}

// DeferCalendarWrite records a failed attempt and when to try again.
func (s *Store) DeferCalendarWrite(ctx context.Context, eventID, detail string, retryAt, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE local_calendar_writes SET attempts = attempts + 1, next_attempt_at = ?,
		detail = ?, updated_at = ? WHERE event_id = ? AND state IN ('create', 'delete')`,
		formatSQLiteTime(retryAt.UTC()), detail, formatSQLiteTime(now.UTC()), eventID)
	return err
}

// RetryCalendarWrites makes every waiting write due now.
func (s *Store) RetryCalendarWrites(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE local_calendar_writes SET next_attempt_at = ''
		WHERE state IN ('create', 'delete')`)
	return err
}

// OwnCalendarEventIDs names the accepted times ZeitBoard holds, and the undone
// ones it is still removing from the owner's calendar. An import drops its
// copies of these: each is already on the board as ZeitBoard's own, or on its
// way out of the calendar.
func (s *Store) OwnCalendarEventIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT event_id FROM local_calendar_events WHERE ownership = 'app_owned'
		UNION SELECT event_id FROM local_calendar_writes WHERE state = 'delete'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// ResolveCalendarWriteConflict settles a conflict as the owner decided: remove
// the event from their calendar after all, or leave it there and forget it.
func (s *Store) ResolveCalendarWriteConflict(ctx context.Context, eventID string, remove bool, now time.Time) error {
	var result sql.Result
	var err error
	if remove {
		// The owner saw that it changed: remove it whatever its revision.
		result, err = s.db.ExecContext(ctx, `UPDATE local_calendar_writes SET state = 'delete', etag = '', attempts = 0,
			next_attempt_at = '', detail = '', updated_at = ? WHERE event_id = ? AND state = 'conflict' AND href <> ''`,
			formatSQLiteTime(now.UTC()), eventID)
	} else {
		result, err = s.db.ExecContext(ctx, `DELETE FROM local_calendar_writes WHERE event_id = ? AND state = 'conflict'`, eventID)
	}
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrCalendarWriteNotFound
	}
	return nil
}

const calendarWriteSelect = `SELECT event_id, state, href, etag, title, start_at, attempts, next_attempt_at, detail
	FROM local_calendar_writes`

func scanCalendarWrites(rows *sql.Rows) ([]CalendarWrite, error) {
	defer rows.Close()
	writes := make([]CalendarWrite, 0)
	for rows.Next() {
		var write CalendarWrite
		var startAt, nextAttemptAt string
		if err := rows.Scan(&write.EventID, &write.State, &write.Href, &write.ETag, &write.Title, &startAt,
			&write.Attempts, &nextAttemptAt, &write.Detail); err != nil {
			return nil, err
		}
		for _, field := range []struct {
			text  string
			value *time.Time
		}{{startAt, &write.StartAt}, {nextAttemptAt, &write.NextAttemptAt}} {
			if strings.TrimSpace(field.text) == "" {
				continue
			}
			parsed, err := time.Parse(time.RFC3339Nano, field.text)
			if err != nil {
				return nil, fmt.Errorf("read calendar write %s: %w", write.EventID, err)
			}
			*field.value = parsed
		}
		writes = append(writes, write)
	}
	return writes, rows.Err()
}
