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

// Writing accepted times to the owner's calendar (ADR-0053). Off until the
// owner turns it on for one calendar: an imported CalDAV calendar with a
// sign-in for it, or one brought in from a Google or Microsoft account that
// allowed adding events (ADR-0065). Then every accepted time that has not
// ended is written there as ZeitBoard's own event, and undoing one removes it,
// unless the owner has changed it in their calendar since. ZeitBoard writes
// and removes only the events it created, named by their placement; it never
// changes an imported event.
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
	ErrCalendarNotWritable   = errors.New("accepted times can be written only to a CalDAV calendar or one brought in from an account")
	ErrCalendarWriteNotFound = errors.New("no such written time")
)

// CalendarWriteTarget is the calendar accepted times are written to, and how
// to write there: a CalDAV collection with the sign-in to write with, or an
// account's calendar, written with the account's own sign-in. A CalDAV
// sign-in is a credential: it stays in this owner-protected database, is
// never exported or synced, and is erased when writing stops.
type CalendarWriteTarget struct {
	SourceID      string
	Label         string
	CollectionURL string
	Username      string
	Password      string
	EnabledAt     time.Time

	// AccountID and CalendarID name the account calendar written to; both
	// are empty for a CalDAV calendar.
	AccountID  string
	CalendarID string
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

	// Approval is, for a create, the approval in force for its time when it
	// was listed. What becomes of a create is recorded only while that
	// approval still is: a time undone and accepted again is another write.
	Approval string
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
		// A create that met a conflict is moot once its time is undone, so a
		// time accepted again is written afresh.
		`CREATE TRIGGER IF NOT EXISTS local_calendar_write_undone_conflict AFTER DELETE ON local_calendar_events
		WHEN OLD.ownership = 'app_owned' BEGIN
		DELETE FROM local_calendar_writes WHERE event_id = OLD.event_id AND state = 'conflict' AND href = ''; END`,
		// A time accepted again while its earlier acceptance was still being
		// removed found that removal in its place. Once it is removed, the new
		// acceptance is written.
		`CREATE TRIGGER IF NOT EXISTS local_calendar_write_after_removal AFTER DELETE ON local_calendar_writes
		WHEN OLD.state = 'delete' AND EXISTS(SELECT 1 FROM local_calendar_write_target) BEGIN
		INSERT OR IGNORE INTO local_calendar_writes(event_id, state, title, start_at, updated_at)
		SELECT event_id, 'create', title, start_at, strftime('%Y-%m-%dT%H:%M:%fZ', 'now') FROM local_calendar_events
		WHERE event_id = OLD.event_id AND ownership = 'app_owned'; END`,
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
		target.password, target.enabled_at, COALESCE(link.account_id, ''), COALESCE(link.calendar_id, '')
		FROM local_calendar_write_target AS target JOIN local_calendar_sources AS source ON source.source_id = target.source_id
		LEFT JOIN local_calendar_account_calendars AS link ON link.source_id = target.source_id`,
	).Scan(&target.SourceID, &target.Label, &target.CollectionURL, &target.Username, &target.Password, &enabledAt,
		&target.AccountID, &target.CalendarID)
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

// SetCalendarWriteTarget turns writing on at now, replacing any other target,
// and queues every accepted time that has not ended. The calendar is an
// imported CalDAV calendar, written with the sign-in given, or one brought
// in from an account, written with the account's own; the caller has checked
// that the account allows it.
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
	var fromAccount bool
	err = tx.QueryRowContext(ctx, `SELECT kind, endpoint,
			EXISTS(SELECT 1 FROM local_calendar_account_calendars WHERE source_id = source.source_id)
		FROM local_calendar_sources AS source WHERE source_id = ?`, target.SourceID).Scan(&kind, &endpoint, &fromAccount)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrCalendarSourceNotFound
	case err != nil:
		return err
	case fromAccount:
		// Written with the account's own sign-in; nothing more is kept.
		endpoint, target.Username, target.Password = "", "", ""
	case kind != string(calendarcore.SourceCalDAV) || endpoint == "":
		return ErrCalendarNotWritable
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

// DueCalendarWrites lists the writes due at now, earliest time first, while
// writing is on as target says: none once it stops or moves.
func (s *Store) DueCalendarWrites(ctx context.Context, target CalendarWriteTarget, now time.Time, limit int) ([]CalendarWrite, error) {
	rows, err := s.db.QueryContext(ctx, calendarWriteSelect+` WHERE state IN ('create', 'delete')
		AND (next_attempt_at = '' OR next_attempt_at <= ?) AND `+inThisWriting+` ORDER BY start_at, event_id LIMIT ?`,
		formatSQLiteTime(now.UTC()), formatSQLiteTime(target.EnabledAt), limit)
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

// approvalOfEvent is the approval in force for an accepted time, named
// event: the latest approval of its time, or empty where none is recorded.
const approvalOfEvent = `COALESCE((SELECT approval.decision_id FROM local_proposal_decisions AS approval
	WHERE approval.proposal_id = event.proposal_id AND approval.event_id = event.event_id
	AND approval.decision = 'approved' ORDER BY approval.rowid DESC LIMIT 1), '')`

// approvalInForce is the approval in force for a write's time, or NULL
// once the time is undone.
const approvalInForce = `(SELECT ` + approvalOfEvent + ` FROM local_calendar_events AS event
	WHERE event.event_id = local_calendar_writes.event_id AND event.ownership = 'app_owned')`

// AcceptedTime is an accepted time as a create writes it: the event, and the
// approval that accepted it. Each acceptance is written as an event of its
// own, named by its approval, so a time accepted again after an undo is never
// taken for the event removed.
type AcceptedTime struct {
	Event    calendarcore.Event
	Approval string
}

// CalendarWriteEvent is the accepted time a create writes.
func (s *Store) CalendarWriteEvent(ctx context.Context, eventID string) (AcceptedTime, bool, error) {
	var accepted AcceptedTime
	event, err := scanCalendarEvent(s.db.QueryRowContext(ctx, `SELECT
		event_id, source_id, source_record_id, title, start_at, end_at, zone_id,
		all_day, busy, ownership, created_at, location, notes, task_id, task_revision, proposal_id, `+approvalOfEvent+`
		FROM local_calendar_events AS event WHERE event_id = ? AND ownership = 'app_owned'`, eventID), &accepted.Approval)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return AcceptedTime{}, false, nil
	case err != nil:
		return AcceptedTime{}, false, err
	}
	accepted.Event = event
	return accepted, true, nil
}

// MarkCalendarWritten records that an acceptance is in the calendar written
// to as target says. If it was undone while being written, what was written
// is queued for removal instead, and a time accepted again meanwhile is
// written once that is done.
func (s *Store) MarkCalendarWritten(ctx context.Context, target CalendarWriteTarget, accepted AcceptedTime, href, etag string, now time.Time) error {
	event, stamp, writing := accepted.Event, formatSQLiteTime(now.UTC()), formatSQLiteTime(target.EnabledAt)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE local_calendar_writes SET state = 'written', href = ?, etag = ?,
		attempts = 0, next_attempt_at = '', detail = '', updated_at = ? WHERE event_id = ? AND state = 'create'
		AND `+approvalInForce+` = ? AND `+inThisWriting,
		href, etag, stamp, event.EventID, accepted.Approval, writing)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		// Undone while it was being written, and perhaps accepted again: the
		// write waiting for the new acceptance removes this one first.
		result, err = tx.ExecContext(ctx, `UPDATE local_calendar_writes SET state = 'delete', href = ?, etag = ?,
			attempts = 0, next_attempt_at = '', detail = '', updated_at = ? WHERE event_id = ? AND state = 'create'
			AND `+inThisWriting, href, etag, stamp, event.EventID, writing)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_calendar_writes(event_id, state, href, etag, title, start_at, updated_at)
				SELECT ?, 'delete', ?, ?, ?, ?, ? WHERE `+inThisWriting,
				event.EventID, href, etag, event.Title, formatSQLiteTime(event.StartAt.UTC()), stamp, writing); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// MarkCalendarWriteRemoved records that an undone time left the calendar
// written to as target says.
func (s *Store) MarkCalendarWriteRemoved(ctx context.Context, target CalendarWriteTarget, eventID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM local_calendar_writes WHERE event_id = ? AND state = 'delete'
		AND `+inThisWriting, eventID, formatSQLiteTime(target.EnabledAt))
	return err
}

// MarkCalendarWriteConflict leaves a write for the owner to settle.
func (s *Store) MarkCalendarWriteConflict(ctx context.Context, target CalendarWriteTarget, write CalendarWrite, detail string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE local_calendar_writes SET state = 'conflict', detail = ?, updated_at = ?
		WHERE `+theWriteCarried, detail, formatSQLiteTime(now.UTC()),
		write.EventID, write.State, write.Approval, formatSQLiteTime(target.EnabledAt))
	return err
}

// DeferCalendarWrite records a failed attempt and when to try again.
func (s *Store) DeferCalendarWrite(ctx context.Context, target CalendarWriteTarget, write CalendarWrite, detail string, retryAt, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE local_calendar_writes SET attempts = attempts + 1, next_attempt_at = ?,
		detail = ?, updated_at = ? WHERE `+theWriteCarried, formatSQLiteTime(retryAt.UTC()), detail, formatSQLiteTime(now.UTC()),
		write.EventID, write.State, write.Approval, formatSQLiteTime(target.EnabledAt))
	return err
}

// WritingStill reports whether writing is on as target says, so a pass
// stops once writing stops or moves to another calendar.
func (s *Store) WritingStill(ctx context.Context, target CalendarWriteTarget) (bool, error) {
	var still bool
	err := s.db.QueryRowContext(ctx, `SELECT `+inThisWriting, formatSQLiteTime(target.EnabledAt)).Scan(&still)
	return still, err
}

// theWriteCarried picks a write as it was carried, by its time, state and,
// for a create, the approval in force, while writing is on as it was: an
// outcome never lands on a write that has taken its place since.
const theWriteCarried = `event_id = ? AND state = ? AND state IN ('create', 'delete')
	AND (state = 'delete' OR ` + approvalInForce + ` = ?) AND ` + inThisWriting

// inThisWriting holds while writing is on as it was turned on at the given
// moment, kept to the nanosecond: stopping, or turning writing on again or
// for another calendar, ends that time of writing.
const inThisWriting = `EXISTS(SELECT 1 FROM local_calendar_write_target WHERE enabled_at = ?)`

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
// the event from their calendar after all, or leave it there and forget it,
// unless the time has been accepted again, which it then stands for.
func (s *Store) ResolveCalendarWriteConflict(ctx context.Context, eventID string, remove bool, now time.Time) error {
	// One transaction, so a time accepted again meanwhile finds the conflict
	// either still there, to be kept for it, or already gone.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var result sql.Result
	if remove {
		// The owner saw that it changed: remove it whatever its revision.
		result, err = tx.ExecContext(ctx, `UPDATE local_calendar_writes SET state = 'delete', etag = '', attempts = 0,
			next_attempt_at = '', detail = '', updated_at = ? WHERE event_id = ? AND state = 'conflict' AND href <> ''`,
			formatSQLiteTime(now.UTC()), eventID)
	} else if result, err = keepCalendarWrite(ctx, tx, eventID, now); err == nil {
		if changed, _ := result.RowsAffected(); changed == 0 {
			result, err = tx.ExecContext(ctx, `DELETE FROM local_calendar_writes WHERE event_id = ? AND state = 'conflict'`, eventID)
		}
	}
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrCalendarWriteNotFound
	}
	return tx.Commit()
}

// keepCalendarWrite leaves a changed event in the calendar for a time that
// has been accepted again since it was undone: that event stands for the
// time, as written at the version ZeitBoard wrote, so removing it later
// still asks the owner first. It changes nothing for any other conflict.
func keepCalendarWrite(ctx context.Context, tx *sql.Tx, eventID string, now time.Time) (sql.Result, error) {
	return tx.ExecContext(ctx, `UPDATE local_calendar_writes SET state = 'written', attempts = 0, next_attempt_at = '',
		detail = '', updated_at = ? WHERE event_id = ? AND state = 'conflict' AND href <> ''
		AND EXISTS(SELECT 1 FROM local_calendar_events WHERE event_id = ? AND ownership = 'app_owned')`,
		formatSQLiteTime(now.UTC()), eventID, eventID)
}

const calendarWriteSelect = `SELECT event_id, state, href, etag, title, start_at, attempts, next_attempt_at, detail,
	CASE WHEN state = 'create' THEN COALESCE(` + approvalInForce + `, '') ELSE '' END FROM local_calendar_writes`

func scanCalendarWrites(rows *sql.Rows) ([]CalendarWrite, error) {
	defer rows.Close()
	writes := make([]CalendarWrite, 0)
	for rows.Next() {
		var write CalendarWrite
		var startAt, nextAttemptAt string
		if err := rows.Scan(&write.EventID, &write.State, &write.Href, &write.ETag, &write.Title, &startAt,
			&write.Attempts, &nextAttemptAt, &write.Detail, &write.Approval); err != nil {
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
