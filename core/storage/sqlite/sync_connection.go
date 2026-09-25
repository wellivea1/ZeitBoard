package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// SyncConnection is non-secret display/configuration state. The bearer token
// lives in a separate column and is never part of this serializable value.
type SyncConnection struct {
	Enabled            bool      `json:"enabled"`
	BackendURL         string    `json:"backendUrl"`
	DeviceID           string    `json:"deviceId"`
	InsecureSkipVerify bool      `json:"insecureSkipVerify,omitempty"`
	LastSyncAt         time.Time `json:"lastSyncAt,omitempty"`
	LastError          string    `json:"lastError,omitempty"`
}

func (s *Store) initializeSyncConnection(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS local_sync_connection(id INTEGER PRIMARY KEY CHECK(id=1), settings_json BLOB NOT NULL, token TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS local_sync_seen(record_id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS local_sync_tombstones(record_id TEXT PRIMARY KEY, record_kind TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS local_sync_erased_tasks(task_id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS local_sync_deferred_corrections(correction_id TEXT PRIMARY KEY, target_id TEXT NOT NULL, payload_json BLOB NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_deferred_target ON local_sync_deferred_corrections(target_id)`,
		`CREATE TRIGGER IF NOT EXISTS local_sleep_observation_suppression AFTER DELETE ON local_sleep_observations BEGIN
		INSERT OR IGNORE INTO local_sync_tombstones VALUES(OLD.observation_id,'observation');
		DELETE FROM local_sync_deferred_corrections WHERE target_id=OLD.observation_id; END`,
		`CREATE TRIGGER IF NOT EXISTS local_sleep_correction_suppression AFTER DELETE ON local_sleep_corrections BEGIN
		INSERT OR IGNORE INTO local_sync_tombstones VALUES(OLD.correction_id,'correction'); END`,
		`CREATE TRIGGER IF NOT EXISTS local_task_suppression AFTER DELETE ON local_tasks BEGIN
		INSERT OR IGNORE INTO local_sync_erased_tasks VALUES(OLD.task_id);
		INSERT OR IGNORE INTO local_sync_tombstones VALUES(OLD.task_id || '_r' || OLD.revision,'task'); END`,
		`CREATE TRIGGER IF NOT EXISTS local_sleep_observation_no_resurrection BEFORE INSERT ON local_sleep_observations
		WHEN EXISTS(SELECT 1 FROM local_sync_tombstones WHERE record_id=NEW.observation_id)
		BEGIN SELECT RAISE(ABORT,'erased sleep record cannot be restored'); END`,
		`CREATE TRIGGER IF NOT EXISTS local_sleep_correction_no_resurrection BEFORE INSERT ON local_sleep_corrections
		WHEN EXISTS(SELECT 1 FROM local_sync_tombstones WHERE record_id=NEW.correction_id OR record_id=NEW.target_observation_id)
		BEGIN SELECT RAISE(ABORT,'erased sleep record cannot be restored'); END`,
		`CREATE TRIGGER IF NOT EXISTS local_task_no_resurrection BEFORE INSERT ON local_tasks
		WHEN EXISTS(SELECT 1 FROM local_sync_erased_tasks WHERE task_id=NEW.task_id)
		BEGIN SELECT RAISE(ABORT,'erased task cannot be restored'); END`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LoadSyncConnection(ctx context.Context) (SyncConnection, string, error) {
	var data []byte
	var token string
	err := s.db.QueryRowContext(ctx, `SELECT settings_json,token FROM local_sync_connection WHERE id=1`).Scan(&data, &token)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncConnection{}, "", nil
	}
	if err != nil {
		return SyncConnection{}, "", err
	}
	var value SyncConnection
	err = json.Unmarshal(data, &value)
	return value, token, err
}

// SaveSyncEnrollment publishes the credential and its reconciliation boundary
// in the same database transaction as the records they authorize. Failed remote
// enrollment never calls this; a failed commit leaves the old connection intact.
func (s *Store) SaveSyncEnrollment(ctx context.Context, value SyncConnection, token string) error {
	if !value.Enabled || value.BackendURL == "" || value.DeviceID == "" || token == "" {
		return errors.New("incomplete backend enrollment")
	}
	if err := s.FilePermissionError(); err != nil {
		return errors.New("backend credentials require owner-only local storage")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_sync_connection VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET settings_json=excluded.settings_json,token=excluded.token`, data, token); err != nil {
		return err
	}
	for _, statement := range []string{
		`DELETE FROM local_sync_seen`,
		`INSERT OR REPLACE INTO local_sync_state VALUES('sleep_sync_cursor','0')`,
		`INSERT OR REPLACE INTO local_sync_state VALUES('sync_reconcile','1')`,
		`INSERT OR IGNORE INTO local_sleep_erasures(record_id,erased_at) SELECT record_id,'' FROM local_sync_tombstones`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateSyncConnection cannot swap credentials. Disabled state clears the token
// atomically; ordinary status updates preserve it.
func (s *Store) UpdateSyncConnection(ctx context.Context, value SyncConnection) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !value.Enabled {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO local_sync_connection VALUES(1,?,'') ON CONFLICT(id) DO UPDATE SET settings_json=excluded.settings_json,token=''`, data); err != nil {
			return err
		}
		return s.compactDeletedData(ctx)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE local_sync_connection SET settings_json=? WHERE id=1 AND token<>''
		AND json_extract(settings_json,'$.enabled')=1 AND json_extract(settings_json,'$.deviceId')=? AND json_extract(settings_json,'$.backendUrl')=?`, data, value.DeviceID, value.BackendURL)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("backend enrollment changed; stale status update was discarded")
	}
	return nil
}

func (s *Store) DeferredSyncCorrectionCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_sync_deferred_corrections`).Scan(&count)
	return count, err
}

// FinishSyncReconciliation runs only after downloading a complete stream. Until
// then, accepted local records cannot be replayed into a restored/new server.
func (s *Store) FinishSyncReconciliation(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var pending string
	err = tx.QueryRowContext(ctx, `SELECT value FROM local_sync_state WHERE key='sync_reconcile'`).Scan(&pending)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && pending != "1") {
		return nil
	}
	if err != nil {
		return err
	}
	for _, statement := range []string{
		`DELETE FROM local_sleep_sync_records WHERE record_id NOT IN(SELECT record_id FROM local_sync_seen)`,
		`DELETE FROM local_task_sync_records WHERE record_id NOT IN(SELECT record_id FROM local_sync_seen)`,
		`DELETE FROM local_sync_seen`,
		`DELETE FROM local_sync_state WHERE key='sync_reconcile'`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func syncRecordSuppressed(ctx context.Context, tx *sql.Tx, recordID, targetID, taskID string) (bool, error) {
	var found bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM local_sync_tombstones WHERE record_id=? OR record_id=?) OR EXISTS(SELECT 1 FROM local_sync_erased_tasks WHERE task_id=?)`, recordID, targetID, taskID).Scan(&found)
	return found, err
}
