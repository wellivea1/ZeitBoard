package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Marker sync (ADR-0048). A rhythm context marker is immutable and travels as
// one "context_marker" record whose id is its own. Deleting it erases it here,
// on the server and on every other device.

const (
	SyncKindContextMarker = "context_marker"

	MaxMarkerSyncPageSize = 100
)

// MarkerSyncRecord is one marker ready for the push endpoint.
type MarkerSyncRecord struct {
	RecordID  string
	CreatedAt time.Time
	Payload   []byte
}

// SyncPullMarker is a downloaded marker.
type SyncPullMarker struct {
	Marker RhythmMarkerRecord
}

func (SyncPullMarker) isSyncPullRecord() {}

const pendingMarkerSyncRecordsFrom = `FROM local_rhythm_markers AS marker
	WHERE NOT EXISTS(SELECT 1 FROM local_evidence_sync_records AS synced WHERE synced.record_id = marker.marker_id)`

// PendingMarkerSyncRecords returns markers the server has not acknowledged,
// in the order they were recorded.
func (s *Store) PendingMarkerSyncRecords(ctx context.Context, limit int) ([]MarkerSyncRecord, error) {
	if limit < 1 || limit > MaxMarkerSyncPageSize {
		return nil, fmt.Errorf("marker sync page limit must be between 1 and %d", MaxMarkerSyncPageSize)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT marker.marker_id, marker.recorded_at, marker.payload_json `+
		pendingMarkerSyncRecordsFrom+` ORDER BY marker.recorded_at, marker.marker_id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pending []MarkerSyncRecord
	for rows.Next() {
		var record MarkerSyncRecord
		var recordedAt string
		if err := rows.Scan(&record.RecordID, &recordedAt, &record.Payload); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, recordedAt)
		if err != nil {
			return nil, fmt.Errorf("pending marker %s: %w", record.RecordID, err)
		}
		record.CreatedAt = parsed.UTC()
		pending = append(pending, record)
	}
	return pending, rows.Err()
}

func (s *Store) PendingMarkerSyncRecordCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+pendingMarkerSyncRecordsFrom).Scan(&count)
	return count, err
}

// MarkMarkerSyncRecordsPushed records the server's acknowledgment. A marker
// deleted while its upload was in flight already has its erasure queued.
func (s *Store) MarkMarkerSyncRecordsPushed(ctx context.Context, records []MarkerSyncRecord, pushedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, record := range records {
		if err := acknowledgeEvidenceTx(ctx, tx, SyncKindContextMarker, record.RecordID, pushedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type preparedMarkerSync struct {
	record  RhythmMarkerRecord
	encoded []byte
}

func prepareSyncedMarker(record RhythmMarkerRecord) (preparedMarkerSync, error) {
	record = normalizeRhythmMarker(record)
	if err := record.Validate(); err != nil {
		return preparedMarkerSync{}, err
	}
	encoded, err := json.Marshal(record)
	return preparedMarkerSync{record: record, encoded: encoded}, err
}

// insertSyncedMarkerTx applies a downloaded marker. A second copy must match
// the first byte for byte.
func insertSyncedMarkerTx(ctx context.Context, tx *sql.Tx, prepared preparedMarkerSync, syncedAt time.Time) (bool, error) {
	record := prepared.record
	if suppressed, err := syncSuppressedTx(ctx, tx, "", record.MarkerID); err != nil || suppressed {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_rhythm_markers(`+rhythmMarkerColumns+`)
		VALUES(?, ?, ?, ?, ?, ?, ?)`, rhythmMarkerRow(record, prepared.encoded)...)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted == 0 {
		var existing []byte
		err := tx.QueryRowContext(ctx, `SELECT payload_json FROM local_rhythm_markers WHERE marker_id = ?`, record.MarkerID).Scan(&existing)
		if err != nil || !bytes.Equal(existing, prepared.encoded) {
			return false, errors.New("downloaded marker conflicts with an immutable local record")
		}
	}
	return inserted > 0, acknowledgeEvidenceTx(ctx, tx, SyncKindContextMarker, record.MarkerID, syncedAt)
}

func eraseMarkerTx(ctx context.Context, tx *sql.Tx, markerID string, origin erasureOrigin) (bool, error) {
	if err := settleErasureTx(ctx, tx, []erasedRecord{{markerID, SyncKindContextMarker}}, origin); err != nil {
		return false, err
	}
	return deleteRowTx(ctx, tx, `DELETE FROM local_rhythm_markers WHERE marker_id = ?`, markerID)
}
