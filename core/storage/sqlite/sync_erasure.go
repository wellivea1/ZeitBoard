package sqlite

import (
	"context"
	"database/sql"
	"time"
)

// Erasure of synced records (ADR-0017, ADR-0048). Deleting a record on this
// device and applying another device's deletion share one path: both remember
// the erased ids so a later download cannot bring them back, and forget their
// acknowledgments and waiting copies. A deletion made here also queues the
// erasure for the server; one that came from the server drops any queued one.

// erasureOrigin says who deleted a record: the owner on this device, which the
// server still has to learn, or another device, whose tombstone this is.
type erasureOrigin int

const (
	erasedHere erasureOrigin = iota
	erasedElsewhere
)

type erasedRecord struct{ id, kind string }

// eraseHere deletes a record the owner chose to delete on this device.
func (s *Store) eraseHere(ctx context.Context, id string,
	erase func(context.Context, *sql.Tx, string, erasureOrigin) (bool, error), notFound error,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	deleted, err := erase(ctx, tx, id, erasedHere)
	if err != nil {
		return err
	}
	if !deleted {
		return notFound
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.compactDeletedData(ctx)
}

// syncSuppressedTx reports whether a downloaded record belongs to
// something deleted on this device or erased through a tombstone.
func syncSuppressedTx(ctx context.Context, tx *sql.Tx, medicationID string, recordIDs ...string) (bool, error) {
	var found bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_sync_erased_medications WHERE medication_id = ?)`,
		medicationID).Scan(&found); err != nil || found {
		return found, err
	}
	for _, recordID := range recordIDs {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_sync_tombstones WHERE record_id = ?)`,
			recordID).Scan(&found); err != nil || found {
			return found, err
		}
	}
	return false, nil
}

func collectErasedRecordsTx(ctx context.Context, tx *sql.Tx, records *[]erasedRecord, query string, args ...any) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var record erasedRecord
		if err := rows.Scan(&record.id, &record.kind); err != nil {
			return err
		}
		*records = append(*records, record)
	}
	return rows.Err()
}

// settleErasureTx remembers erased records so they are never applied again,
// forgets their acknowledgments and waiting copies, and either queues their
// erasure for the server or, when the server sent it, drops any queued one.
func settleErasureTx(ctx context.Context, tx *sql.Tx, records []erasedRecord, origin erasureOrigin) error {
	erasedAt := formatSQLiteTime(time.Now().UTC())
	for _, record := range records {
		queue := `DELETE FROM local_sleep_erasures WHERE record_id = ?`
		queueArgs := []any{record.id}
		if origin == erasedHere {
			queue = `INSERT OR IGNORE INTO local_sleep_erasures(record_id, erased_at) VALUES(?, ?)`
			queueArgs = append(queueArgs, erasedAt)
		}
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`INSERT OR IGNORE INTO local_sync_tombstones(record_id, record_kind) VALUES(?, ?)`, []any{record.id, record.kind}},
			{`DELETE FROM local_medication_sync_records WHERE record_id = ?`, []any{record.id}},
			{`DELETE FROM local_evidence_sync_records WHERE record_id = ?`, []any{record.id}},
			{`DELETE FROM local_sync_deferred_medication WHERE record_id = ?`, []any{record.id}},
			{queue, queueArgs},
		} {
			if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
				return err
			}
		}
	}
	return nil
}

func deleteRowTx(ctx context.Context, tx *sql.Tx, query string, id string) (bool, error) {
	result, err := tx.ExecContext(ctx, query, id)
	if err != nil {
		return false, err
	}
	deleted, err := result.RowsAffected()
	return deleted > 0, err
}
