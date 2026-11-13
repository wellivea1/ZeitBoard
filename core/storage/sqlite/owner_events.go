package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	calendarcore "non24.app/core/calendar"
	"non24.app/core/ownerevents"
)

// Events the owner adds (ADR-0068). Each event's current revision is kept in
// local_owner_events and mirrored into local_calendar_events under its own
// source, so the planner, the week board, Home and the assistant read it as
// the fixed commitment it is. Each change syncs as an immutable revision,
// "<event_id>_r<revision>". Deleting an event erases its revisions everywhere,
// and the event cannot come back.

const (
	// OwnerEventsCalendarSourceID is the source the owner's events are
	// mirrored under, beside the one accepted times are kept under.
	OwnerEventsCalendarSourceID = "calendar_source_owner_events"
	// SyncKindOwnerEvent is the record kind an event syncs as.
	SyncKindOwnerEvent        = "event"
	MaxOwnerEventSyncPageSize = 100
)

var (
	ErrOwnerEventNotFound = errors.New("that event is no longer here")
	ErrOwnerEventChanged  = errors.New("that event changed meanwhile; look at it again before changing it")
)

func (s *Store) initializeOwnerEvents(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS local_owner_events(
			event_id TEXT PRIMARY KEY,
			revision INTEGER NOT NULL CHECK(revision >= 1),
			payload_json BLOB NOT NULL
		)`,
		// Revisions the server holds: sent from here, or received.
		`CREATE TABLE IF NOT EXISTS local_owner_event_sync_records(
			record_id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL,
			synced_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS local_sync_erased_events(event_id TEXT PRIMARY KEY)`,
		`CREATE TRIGGER IF NOT EXISTS local_owner_event_no_resurrection BEFORE INSERT ON local_owner_events
		WHEN EXISTS(SELECT 1 FROM local_sync_erased_events WHERE event_id = NEW.event_id)
		BEGIN SELECT RAISE(ABORT, 'erased event cannot be restored'); END`,
		// However an event goes, its mirror goes with it, and it stays gone.
		`CREATE TRIGGER IF NOT EXISTS local_owner_event_erased AFTER DELETE ON local_owner_events BEGIN
		INSERT OR IGNORE INTO local_sync_erased_events VALUES(OLD.event_id);
		DELETE FROM local_calendar_events WHERE event_id = OLD.event_id AND source_id = '` + OwnerEventsCalendarSourceID + `'; END`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate owner events: %w", err)
		}
	}
	return nil
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// OwnerEvent reads one event as it stands.
func (s *Store) OwnerEvent(ctx context.Context, eventID string) (ownerevents.Record, error) {
	record, _, err := ownerEventFrom(ctx, s.db, eventID)
	return record, err
}

func ownerEventFrom(ctx context.Context, query rowQuerier, eventID string) (ownerevents.Record, []byte, error) {
	var payload []byte
	err := query.QueryRowContext(ctx, `SELECT payload_json FROM local_owner_events WHERE event_id = ?`, eventID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ownerevents.Record{}, nil, ErrOwnerEventNotFound
	}
	if err != nil {
		return ownerevents.Record{}, nil, err
	}
	var record ownerevents.Record
	if err := json.Unmarshal(payload, &record); err != nil {
		return ownerevents.Record{}, nil, err
	}
	return record, payload, nil
}

// AddOwnerEvent keeps a new event as its first revision.
func (s *Store) AddOwnerEvent(ctx context.Context, record ownerevents.Record) error {
	if record.Revision != 1 {
		return errors.New("a new event is its first revision")
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		return putOwnerEventTx(ctx, tx, record)
	})
}

// ChangeOwnerEvent keeps a changed event as the revision after the one the
// owner was looking at. A change made elsewhere meanwhile refuses it.
func (s *Store) ChangeOwnerEvent(ctx context.Context, record ownerevents.Record) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		held, _, err := ownerEventFrom(ctx, tx, record.EventID)
		if err != nil {
			return err
		}
		if held.Revision != record.Revision-1 {
			return ErrOwnerEventChanged
		}
		record.CreatedAt = held.CreatedAt
		return putOwnerEventTx(ctx, tx, record)
	})
}

// DeleteOwnerEvent erases an event: here, on the owner's server and on every
// device. Every revision sent or received is erased, and the current one too,
// whose upload may have reached the server with its answer lost.
func (s *Store) DeleteOwnerEvent(ctx context.Context, eventID string, revision int) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		held, _, err := ownerEventFrom(ctx, tx, eventID)
		if err != nil {
			return err
		}
		if held.Revision != revision {
			return ErrOwnerEventChanged
		}
		erasedAt := formatSQLiteTime(time.Now().UTC())
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_sleep_erasures(record_id, erased_at)
			SELECT record_id, ? FROM local_owner_event_sync_records WHERE event_id = ?
			UNION SELECT ?, ?`,
			erasedAt, eventID, ownerevents.RecordID(eventID, revision), erasedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM local_owner_event_sync_records WHERE event_id = ?`, eventID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM local_owner_events WHERE event_id = ?`, eventID)
		return err
	})
}

func (s *Store) inTx(ctx context.Context, work func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := work(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// putOwnerEventTx keeps a revision and mirrors it as a fixed commitment.
func putOwnerEventTx(ctx context.Context, tx *sql.Tx, record ownerevents.Record) error {
	record = canonicalOwnerEvent(record)
	if err := record.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_owner_events(event_id, revision, payload_json) VALUES(?, ?, ?)
		ON CONFLICT(event_id) DO UPDATE SET revision = excluded.revision, payload_json = excluded.payload_json`,
		record.EventID, record.Revision, payload); err != nil {
		return err
	}
	return mirrorOwnerEventTx(ctx, tx, record)
}

func canonicalOwnerEvent(record ownerevents.Record) ownerevents.Record {
	record.StartAt = record.StartAt.UTC()
	record.EndAt = record.EndAt.UTC()
	record.CreatedAt = record.CreatedAt.UTC()
	record.UpdatedAt = record.UpdatedAt.UTC()
	return record
}

// mirrorOwnerEventTx puts an event in the calendar every reader reads, as busy
// time no plan may move. Rows from outside ZeitBoard's planner are never
// changed in place, so a change replaces the row.
func mirrorOwnerEventTx(ctx context.Context, tx *sql.Tx, record ownerevents.Record) error {
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_calendar_sources(
		source_id, label, kind, read_only, coverage_start_at, coverage_end_at, last_imported_at, endpoint
	) VALUES(?, 'Your events', 'zeitboard', 0, '1970-01-01T00:00:00Z', '9999-12-31T23:59:59Z', ?, '')`,
		OwnerEventsCalendarSourceID, formatSQLiteTime(record.UpdatedAt)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_calendar_events WHERE event_id = ? AND source_id = ?`,
		record.EventID, OwnerEventsCalendarSourceID); err != nil {
		return err
	}
	return insertCalendarEvent(ctx, tx, calendarcore.Event{
		EventID:        record.EventID,
		SourceID:       OwnerEventsCalendarSourceID,
		SourceRecordID: record.EventID,
		Title:          record.Title,
		StartAt:        record.StartAt,
		EndAt:          record.EndAt,
		ZoneID:         record.ZoneID,
		Busy:           true,
		Ownership:      calendarcore.OwnershipImported,
		CreatedAt:      record.CreatedAt,
	})
}

// OwnerEventSyncRecord is an event's current revision, ready to send.
type OwnerEventSyncRecord struct {
	RecordID  string
	EventID   string
	Revision  int
	CreatedAt time.Time
	Payload   []byte
}

// PendingOwnerEventSyncRecords lists the events whose current revision the
// server does not hold.
func (s *Store) PendingOwnerEventSyncRecords(ctx context.Context, limit int) ([]OwnerEventSyncRecord, error) {
	if limit < 1 || limit > MaxOwnerEventSyncPageSize {
		return nil, fmt.Errorf("event sync page limit must be between 1 and %d", MaxOwnerEventSyncPageSize)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event.payload_json FROM local_owner_events AS event
		LEFT JOIN local_owner_event_sync_records AS synced ON synced.record_id = event.event_id || '_r' || event.revision
		WHERE synced.record_id IS NULL ORDER BY event.event_id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pending []OwnerEventSyncRecord
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var record ownerevents.Record
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, err
		}
		pending = append(pending, OwnerEventSyncRecord{
			RecordID:  ownerevents.RecordID(record.EventID, record.Revision),
			EventID:   record.EventID,
			Revision:  record.Revision,
			CreatedAt: record.UpdatedAt,
			Payload:   payload,
		})
	}
	return pending, rows.Err()
}

// MarkOwnerEventSyncRecordsPushed notes revisions the server acknowledged.
// One whose event was deleted while it was on its way is erased instead.
func (s *Store) MarkOwnerEventSyncRecordsPushed(ctx context.Context, records []OwnerEventSyncRecord, pushedAt time.Time) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, record := range records {
			var erased bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_sync_erased_events WHERE event_id = ?)`,
				record.EventID).Scan(&erased); err != nil {
				return err
			}
			statement := `INSERT OR REPLACE INTO local_owner_event_sync_records(record_id, event_id, synced_at) VALUES(?, ?, ?)`
			args := []any{record.RecordID, record.EventID, formatSQLiteTime(pushedAt)}
			if erased {
				statement = `INSERT OR IGNORE INTO local_sleep_erasures(record_id, erased_at) VALUES(?, ?)`
				args = []any{record.RecordID, formatSQLiteTime(pushedAt)}
			}
			if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
				return err
			}
		}
		return nil
	})
}

// SyncPullOwnerEvent is one revision of an event, from the server.
type SyncPullOwnerEvent struct {
	Event ownerevents.Record
}

func (SyncPullOwnerEvent) isSyncPullRecord() {}

type preparedOwnerEvent struct {
	record  ownerevents.Record
	encoded []byte
}

func prepareSyncedOwnerEvent(record ownerevents.Record) (preparedOwnerEvent, error) {
	record = canonicalOwnerEvent(record)
	if err := record.Validate(); err != nil {
		return preparedOwnerEvent{}, err
	}
	encoded, err := json.Marshal(record)
	return preparedOwnerEvent{record: record, encoded: encoded}, err
}

// applySyncedOwnerEventTx keeps a pulled revision when it is the one every
// device keeps. Against a revision this device has sent, the higher one wins.
// Against an edit this device has not sent, the later edit wins; a local edit
// that is later is renumbered past the pulled revision and sent next.
func applySyncedOwnerEventTx(ctx context.Context, tx *sql.Tx, prepared preparedOwnerEvent, syncedAt time.Time) (bool, error) {
	pulled := prepared.record
	recordID := ownerevents.RecordID(pulled.EventID, pulled.Revision)
	var erased bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_sync_erased_events WHERE event_id = ?)
		OR EXISTS(SELECT 1 FROM local_sync_tombstones WHERE record_id = ?)`, pulled.EventID, recordID).Scan(&erased); err != nil || erased {
		return false, err
	}
	held, heldPayload, err := ownerEventFrom(ctx, tx, pulled.EventID)
	if errors.Is(err, ErrOwnerEventNotFound) {
		return true, takeSyncedOwnerEventTx(ctx, tx, pulled, recordID, syncedAt)
	}
	if err != nil {
		return false, err
	}
	var sent bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_owner_event_sync_records WHERE record_id = ?)`,
		ownerevents.RecordID(held.EventID, held.Revision)).Scan(&sent); err != nil {
		return false, err
	}
	switch {
	case sent && pulled.Revision > held.Revision,
		!sent && !bytes.Equal(prepared.encoded, heldPayload) && ownerevents.Later(pulled, prepared.encoded, held, heldPayload):
		return true, takeSyncedOwnerEventTx(ctx, tx, pulled, recordID, syncedAt)
	case !sent && !bytes.Equal(prepared.encoded, heldPayload):
		held.Revision = max(held.Revision, pulled.Revision) + 1
		if err := putOwnerEventTx(ctx, tx, held); err != nil {
			return false, err
		}
	}
	return false, markOwnerEventSyncedTx(ctx, tx, recordID, pulled.EventID, syncedAt)
}

func takeSyncedOwnerEventTx(ctx context.Context, tx *sql.Tx, record ownerevents.Record, recordID string, syncedAt time.Time) error {
	if err := putOwnerEventTx(ctx, tx, record); err != nil {
		return err
	}
	return markOwnerEventSyncedTx(ctx, tx, recordID, record.EventID, syncedAt)
}

func markOwnerEventSyncedTx(ctx context.Context, tx *sql.Tx, recordID, eventID string, syncedAt time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_owner_event_sync_records(record_id, event_id, synced_at) VALUES(?, ?, ?)`,
		recordID, eventID, formatSQLiteTime(syncedAt))
	return err
}

// eraseSyncedOwnerEventTx deletes an event erased elsewhere, and sends no
// erasure back.
func eraseSyncedOwnerEventTx(ctx context.Context, tx *sql.Tx, recordID string) (bool, error) {
	eventID, _, ok := ownerevents.EventOf(recordID)
	if !ok {
		return false, fmt.Errorf("record id %q is not an event revision id", recordID)
	}
	for _, statement := range []string{
		`INSERT OR IGNORE INTO local_sync_tombstones VALUES(?, '` + SyncKindOwnerEvent + `')`,
		`DELETE FROM local_sleep_erasures WHERE record_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, recordID); err != nil {
			return false, err
		}
	}
	for _, statement := range []string{
		`INSERT OR IGNORE INTO local_sync_erased_events VALUES(?)`,
		`DELETE FROM local_owner_event_sync_records WHERE event_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, eventID); err != nil {
			return false, err
		}
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM local_owner_events WHERE event_id = ?`, eventID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}
