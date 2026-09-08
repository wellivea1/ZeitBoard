package store

import (
	"context"
	"database/sql"
	"errors"

	syncmodel "non24.app/server/internal/sync"
)

const MaxCompanionRecords = 100_000
const MaxCompanionPayloadBytes = 16 << 20

var ErrCompanionHistoryLimit = errors.New("companion history exceeds the supported snapshot limit")

// SleepSnapshot reads a coherent encrypted-log snapshot. Erasure and new appends
// cannot mix different input generations within one projection.
func (s *Store) SleepSnapshot(ctx context.Context) ([]syncmodel.Envelope, int64, error) {
	return s.sleepSnapshot(ctx, "", MaxCompanionRecords, MaxCompanionPayloadBytes)
}

func (s *Store) SleepReviewSnapshot(ctx context.Context, observationID string) ([]syncmodel.Envelope, int64, error) {
	return s.sleepSnapshot(ctx, observationID, 10_000, 2<<20)
}

func (s *Store) sleepSnapshot(ctx context.Context, observationID string, recordLimit, byteLimit int) ([]syncmodel.Envelope, int64, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	through, err := maxCursor(ctx, tx)
	if err != nil {
		return nil, 0, err
	}
	var records []syncmodel.Envelope
	since, totalBytes := int64(0), 0
	for since < through {
		filter := "kind IN ('observation', 'correction')"
		args := []any{since, through}
		if observationID != "" {
			filter = "(record_id = ? OR record_id IN (SELECT record_id FROM sync_correction_targets WHERE observation_id = ?))"
			args = append(args, observationID, observationID)
		}
		rows, err := tx.QueryContext(ctx, `SELECT seq, record_id, kind, device_id, created_at, nonce, ciphertext
			FROM sync_records WHERE seq > ? AND seq <= ? AND `+filter+` ORDER BY seq LIMIT 100`, args...)
		if err != nil {
			return nil, 0, err
		}
		page, cursor, err := s.scanSyncRows(rows, since)
		rows.Close()
		if err != nil {
			return nil, 0, err
		}
		for _, record := range page {
			totalBytes += len(record.Payload)
			if totalBytes > byteLimit || len(records) >= recordLimit {
				return nil, through, ErrCompanionHistoryLimit
			}
			records = append(records, record)
		}
		if len(page) < 100 {
			break
		}
		since = cursor
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return records, through, nil
}
