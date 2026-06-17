package store

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	syncmodel "non24.app/server/internal/sync"

	_ "modernc.org/sqlite"
)

var (
	ErrDeviceNotFound = errors.New("device not found")
	ErrRecordConflict = errors.New("record id already exists with different payload")
)

type Store struct {
	db   *sql.DB
	aead cipher.AEAD
}

type Device struct {
	ID        string
	Label     string
	CreatedAt time.Time
}

func Open(path string, dataKey []byte) (*Store, error) {
	if len(dataKey) != 32 {
		return nil, errors.New("data key must be 32 bytes")
	}
	if path == "" {
		return nil, errors.New("database path is required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create data directory: %w", err)
		}
	}
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, aead: aead}
	if err := store.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA secure_delete = ON`,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			token_hash BLOB NOT NULL UNIQUE,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sync_records (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			record_id TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL,
			device_id TEXT NOT NULL REFERENCES devices(id),
			created_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL,
			payload_hash BLOB NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate server sqlite: %w", err)
		}
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?)`,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) RegisterDevice(ctx context.Context, id, label string, tokenHash []byte, createdAt time.Time) error {
	if id == "" || len(tokenHash) != sha256.Size {
		return errors.New("device id and token hash are required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO devices(id, label, token_hash, created_at) VALUES(?, ?, ?, ?)`,
		id, label, append([]byte(nil), tokenHash...), createdAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) FindDeviceByTokenHash(ctx context.Context, tokenHash []byte) (Device, error) {
	var device Device
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, label, created_at FROM devices WHERE token_hash = ?`,
		append([]byte(nil), tokenHash...),
	).Scan(&device.ID, &device.Label, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrDeviceNotFound
	}
	if err != nil {
		return Device{}, err
	}
	device.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Device{}, err
	}
	return device, nil
}

func (s *Store) Append(ctx context.Context, deviceID string, records []syncmodel.PushRecord) (int64, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	accepted := 0
	for _, record := range records {
		sum := sha256.Sum256(record.Payload)
		payloadHash := sum[:]

		var existingSeq int64
		var existingHash []byte
		err := tx.QueryRowContext(ctx,
			`SELECT seq, payload_hash FROM sync_records WHERE record_id = ?`,
			record.RecordID,
		).Scan(&existingSeq, &existingHash)
		switch {
		case err == nil:
			if !bytes.Equal(existingHash, payloadHash) {
				return 0, 0, ErrRecordConflict
			}
			continue
		case errors.Is(err, sql.ErrNoRows):
		default:
			return 0, 0, err
		}

		createdAt := record.CreatedAt.UTC().Format(time.RFC3339Nano)
		nonce := make([]byte, s.aead.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return 0, 0, err
		}
		ciphertext := s.aead.Seal(nil, nonce, record.Payload, recordAAD(record.RecordID, record.Kind, deviceID, createdAt))
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sync_records(record_id, kind, device_id, created_at, nonce, ciphertext, payload_hash)
			 VALUES(?, ?, ?, ?, ?, ?, ?)`,
			record.RecordID, string(record.Kind), deviceID, createdAt, nonce, ciphertext, payloadHash,
		); err != nil {
			return 0, 0, err
		}
		accepted++
	}

	cursor, err := maxCursor(ctx, tx)
	if err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	committed = true
	return cursor, accepted, nil
}

func (s *Store) Pull(ctx context.Context, since int64, limit int) ([]syncmodel.Envelope, int64, error) {
	if since < 0 {
		return nil, 0, errors.New("cursor must not be negative")
	}
	if limit <= 0 || limit > syncmodel.MaxPullRecords {
		limit = syncmodel.MaxPullRecords
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, record_id, kind, device_id, created_at, nonce, ciphertext
		 FROM sync_records
		 WHERE seq > ?
		 ORDER BY seq
		 LIMIT ?`,
		since, limit,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	cursor := since
	var records []syncmodel.Envelope
	for rows.Next() {
		var record syncmodel.Envelope
		var kind, createdAt string
		var nonce, ciphertext []byte
		if err := rows.Scan(&record.Seq, &record.RecordID, &kind, &record.DeviceID, &createdAt, &nonce, &ciphertext); err != nil {
			return nil, 0, err
		}
		record.Kind = syncmodel.Kind(kind)
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, 0, err
		}
		plaintext, err := s.aead.Open(nil, nonce, ciphertext, recordAAD(record.RecordID, record.Kind, record.DeviceID, createdAt))
		if err != nil {
			return nil, 0, err
		}
		record.CreatedAt = created.UTC()
		record.Payload = append(record.Payload[:0], plaintext...)
		records = append(records, record)
		cursor = record.Seq
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return records, cursor, nil
}

func (s *Store) CountRecords(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_records`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func maxCursor(ctx context.Context, tx *sql.Tx) (int64, error) {
	var cursor sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(seq) FROM sync_records`).Scan(&cursor); err != nil {
		return 0, err
	}
	if !cursor.Valid {
		return 0, nil
	}
	return cursor.Int64, nil
}

func recordAAD(recordID string, kind syncmodel.Kind, deviceID, createdAt string) []byte {
	return []byte(strings.Join([]string{recordID, string(kind), deviceID, createdAt}, "\x00"))
}
