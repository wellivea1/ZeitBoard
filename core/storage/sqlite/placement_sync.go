package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Placement sync (ADR-0047). An accepted suggestion is an app-owned calendar
// block on this computer. The companion draws the day's accepted times on its
// dial, so each block also travels as an immutable "placement" record: which
// task, when, in which zone. The block's event id is the record id. Undoing
// the acceptance, deleting the task or erasing all data deletes the block, and
// a trigger queues the record's erasure on the server the same moment, so no
// path forgets it.

// PlacementSyncRecord is one accepted time ready for the push endpoint.
type PlacementSyncRecord struct {
	RecordID  string
	CreatedAt time.Time
	Payload   []byte
}

type placementSyncPayload struct {
	PlacementID string    `json:"placement_id"`
	TaskID      string    `json:"task_id"`
	StartAt     time.Time `json:"start_at"`
	EndAt       time.Time `json:"end_at"`
	ZoneID      string    `json:"zone_id"`
	CreatedAt   time.Time `json:"created_at"`
}

const MaxPlacementSyncPageSize = 100

func (s *Store) initializePlacementSync(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS local_placement_sync_records(record_id TEXT PRIMARY KEY, pushed_at TEXT NOT NULL)`,
		`CREATE TRIGGER IF NOT EXISTS local_placement_erasure AFTER DELETE ON local_calendar_events
		WHEN OLD.ownership = 'app_owned' AND OLD.task_id <> '' BEGIN
		INSERT OR IGNORE INTO local_sleep_erasures(record_id, erased_at) VALUES(OLD.event_id, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
		DELETE FROM local_placement_sync_records WHERE record_id = OLD.event_id; END`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

const pendingPlacementSyncRecordsFrom = `FROM local_calendar_events AS event
	LEFT JOIN local_placement_sync_records AS synced ON synced.record_id = event.event_id
	WHERE event.ownership = 'app_owned' AND event.task_id <> '' AND synced.record_id IS NULL`

// PendingPlacementSyncRecords returns accepted times not yet acknowledged by
// the server, earliest first.
func (s *Store) PendingPlacementSyncRecords(ctx context.Context, limit int) ([]PlacementSyncRecord, error) {
	if limit < 1 || limit > MaxPlacementSyncPageSize {
		return nil, fmt.Errorf("placement sync page limit must be between 1 and %d", MaxPlacementSyncPageSize)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event.event_id, event.task_id, event.start_at, event.end_at, event.zone_id, event.created_at `+
		pendingPlacementSyncRecordsFrom+` ORDER BY event.start_at, event.event_id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pending []PlacementSyncRecord
	for rows.Next() {
		var eventID, taskID, startAt, endAt, zoneID, createdAt string
		if err := rows.Scan(&eventID, &taskID, &startAt, &endAt, &zoneID, &createdAt); err != nil {
			return nil, err
		}
		payload := placementSyncPayload{PlacementID: eventID, TaskID: taskID, ZoneID: zoneID}
		for _, field := range []struct {
			value  string
			target *time.Time
		}{{startAt, &payload.StartAt}, {endAt, &payload.EndAt}, {createdAt, &payload.CreatedAt}} {
			parsed, err := time.Parse(time.RFC3339Nano, field.value)
			if err != nil {
				return nil, fmt.Errorf("placement %s: %w", eventID, err)
			}
			*field.target = parsed.UTC()
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		pending = append(pending, PlacementSyncRecord{RecordID: eventID, CreatedAt: payload.CreatedAt, Payload: encoded})
	}
	return pending, rows.Err()
}

func (s *Store) PendingPlacementSyncRecordCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+pendingPlacementSyncRecordsFrom).Scan(&count)
	return count, err
}

// MarkPlacementSyncRecordsPushed records the server's acknowledgment. A block
// removed while its upload was in flight is not marked: its erasure is
// already queued.
func (s *Store) MarkPlacementSyncRecordsPushed(ctx context.Context, records []PlacementSyncRecord, pushedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, record := range records {
		if err := markPlacementPushedTx(ctx, tx, record.RecordID, pushedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func markPlacementPushedTx(ctx context.Context, tx *sql.Tx, recordID string, pushedAt time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_placement_sync_records(record_id, pushed_at)
		SELECT ?, ? WHERE EXISTS(SELECT 1 FROM local_calendar_events WHERE event_id = ? AND ownership = 'app_owned')`,
		recordID, formatSQLiteTime(pushedAt), recordID)
	return err
}

// erasePlacementRecordTx applies a server tombstone for a placement. This
// computer owns its blocks, so the tombstone only settles bookkeeping: the
// acknowledgment and any queued erasure of the same record.
func erasePlacementRecordTx(ctx context.Context, tx *sql.Tx, recordID string) (bool, error) {
	result, err := tx.ExecContext(ctx, `DELETE FROM local_placement_sync_records WHERE record_id = ?`, recordID)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_sleep_erasures WHERE record_id = ?`, recordID); err != nil {
		return false, err
	}
	removed, err := result.RowsAffected()
	return removed > 0, err
}
