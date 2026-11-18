package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"non24.app/core/sharedsetting"
)

// Choices that travel between the owner's devices (ADR-0074): sharing a
// status by default, the sleep buttons reporting one, and the clock. Each
// change is kept as made, here or on another device, and uploaded to the
// owner's server; for each choice the newest record stands. The settings
// files hold the value in use, and follow the record that stands.

const (
	SyncKindSetting        = "setting"
	MaxSettingSyncPageSize = 100
)

// SettingSyncRecord is one choice ready for the push endpoint.
type SettingSyncRecord struct {
	RecordID  string
	CreatedAt time.Time
	Payload   []byte
}

// SyncPullSetting is a downloaded choice.
type SyncPullSetting struct {
	Record sharedsetting.Record
}

func (SyncPullSetting) isSyncPullRecord() {}

type preparedSettingRecord struct {
	record  sharedsetting.Record
	encoded []byte
}

func prepareSettingRecord(record sharedsetting.Record) (preparedSettingRecord, error) {
	record.ChangedAt = record.ChangedAt.UTC()
	if err := record.Validate(); err != nil {
		return preparedSettingRecord{}, fmt.Errorf("setting: %w", err)
	}
	encoded, err := json.Marshal(record)
	return preparedSettingRecord{record: record, encoded: encoded}, err
}

func insertSettingRecordTx(ctx context.Context, tx *sql.Tx, prepared preparedSettingRecord) (bool, error) {
	record := prepared.record
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_setting_records(
			setting_id, name, changed_at, payload_json)
		VALUES(?, ?, ?, ?)`,
		record.SettingID, record.Name, formatSQLiteTime(record.ChangedAt), prepared.encoded)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted == 0 {
		var existing []byte
		err := tx.QueryRowContext(ctx, `SELECT payload_json FROM local_setting_records WHERE setting_id = ?`, record.SettingID).Scan(&existing)
		if err != nil || !bytes.Equal(existing, prepared.encoded) {
			return false, errors.New("a setting with this id is already kept, and differs")
		}
	}
	return inserted > 0, nil
}

// AddSettingRecord keeps a choice the owner made on this device. It uploads
// with the next sync.
func (s *Store) AddSettingRecord(ctx context.Context, record sharedsetting.Record) error {
	prepared, err := prepareSettingRecord(record)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := insertSettingRecordTx(ctx, tx, prepared); err != nil {
		return err
	}
	return tx.Commit()
}

// CurrentSettings returns the record that stands for each choice with any.
func (s *Store) CurrentSettings(ctx context.Context) (map[string]sharedsetting.Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM local_setting_records`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []sharedsetting.Record
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var record sharedsetting.Record
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, fmt.Errorf("stored setting: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sharedsetting.Current(records), nil
}

// PendingSettingSyncRecords returns choices the server has not acknowledged,
// in the order they were made.
func (s *Store) PendingSettingSyncRecords(ctx context.Context, limit int) ([]SettingSyncRecord, error) {
	if limit < 1 || limit > MaxSettingSyncPageSize {
		return nil, fmt.Errorf("setting sync page limit must be between 1 and %d", MaxSettingSyncPageSize)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT setting.setting_id, setting.changed_at, setting.payload_json
		FROM local_setting_records AS setting
		WHERE NOT EXISTS(SELECT 1 FROM local_evidence_sync_records AS synced WHERE synced.record_id = setting.setting_id)
		ORDER BY setting.changed_at, setting.setting_id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pending []SettingSyncRecord
	for rows.Next() {
		var record SettingSyncRecord
		var changedAt string
		if err := rows.Scan(&record.RecordID, &changedAt, &record.Payload); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, changedAt)
		if err != nil {
			return nil, fmt.Errorf("pending setting %s: %w", record.RecordID, err)
		}
		record.CreatedAt = parsed.UTC()
		pending = append(pending, record)
	}
	return pending, rows.Err()
}

// MarkSettingSyncRecordsPushed records that the server holds these choices.
func (s *Store) MarkSettingSyncRecordsPushed(ctx context.Context, records []SettingSyncRecord, pushedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, record := range records {
		if err := acknowledgeEvidenceTx(ctx, tx, SyncKindSetting, record.RecordID, pushedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// insertSyncedSettingTx keeps a downloaded choice. A second copy must match
// the first byte for byte, and an erased one is not kept again.
func insertSyncedSettingTx(ctx context.Context, tx *sql.Tx, prepared preparedSettingRecord, syncedAt time.Time) (bool, error) {
	if suppressed, err := syncSuppressedTx(ctx, tx, "", prepared.record.SettingID); err != nil || suppressed {
		return false, err
	}
	inserted, err := insertSettingRecordTx(ctx, tx, prepared)
	if err != nil {
		return false, err
	}
	return inserted, acknowledgeEvidenceTx(ctx, tx, SyncKindSetting, prepared.record.SettingID, syncedAt)
}

func eraseSettingRecordTx(ctx context.Context, tx *sql.Tx, settingID string, origin erasureOrigin) (bool, error) {
	if err := settleErasureTx(ctx, tx, []erasedRecord{{settingID, SyncKindSetting}}, origin); err != nil {
		return false, err
	}
	return deleteRowTx(ctx, tx, `DELETE FROM local_setting_records WHERE setting_id = ?`, settingID)
}
