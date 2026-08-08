package store

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	ErrDeviceNotFound       = errors.New("device not found")
	ErrProposalNotFound     = errors.New("proposal not found")
	ErrProposalNotPending   = errors.New("proposal is not pending")
	ErrRecordConflict       = errors.New("record id already exists with different payload")
	ErrInvalidApprovalToken = errors.New("invalid approval token")
	ErrExpiredApprovalToken = errors.New("approval token expired")
	ErrUsedApprovalToken    = errors.New("approval token already used")
)

type Store struct {
	db         *sql.DB
	aead       cipher.AEAD
	signingKey []byte
}

type Device struct {
	ID        string     `json:"deviceId"`
	Label     string     `json:"label"`
	CreatedAt time.Time  `json:"createdAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

type ProposalStatus string

const (
	ProposalPending  ProposalStatus = "pending"
	ProposalApproved ProposalStatus = "approved"
	ProposalRejected ProposalStatus = "rejected"

	DefaultProposalPageLimit = 50
	MaxProposalPageLimit     = 100
)

type ProposalInput struct {
	ID        string
	ActionID  string
	DeviceID  string
	CreatedAt time.Time
	ExpiresAt time.Time
	Payload   json.RawMessage
	Audit     json.RawMessage
}

type ProposalRecord struct {
	ID            string          `json:"proposalId"`
	ActionID      string          `json:"actionId"`
	DeviceID      string          `json:"deviceId"`
	Status        ProposalStatus  `json:"status"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	ExpiresAt     time.Time       `json:"expiresAt"`
	Payload       json.RawMessage `json:"payload"`
	DecisionToken string          `json:"decisionToken,omitempty"`
}

type ProposalPageCursor struct {
	AfterRowID   int64
	ThroughRowID int64
	Active       bool
	AsOf         time.Time
}

type ProposalPage struct {
	Records    []ProposalRecord
	NextCursor ProposalPageCursor
	HasMore    bool
}

type approvalClaims struct {
	ProposalID string `json:"proposalId"`
	ActionID   string `json:"actionId"`
	DeviceID   string `json:"deviceId"`
	TargetHash string `json:"targetHash"`
	Nonce      string `json:"nonce"`
	ExpiresAt  int64  `json:"expiresAt"`
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
	signingSeed := sha256.Sum256(append([]byte("zeitboard-approval-token\x00"), dataKey...))
	store := &Store{db: db, aead: aead, signingKey: signingSeed[:]}
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
			created_at TEXT NOT NULL,
			revoked_at TEXT NOT NULL DEFAULT ''
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
		`CREATE INDEX IF NOT EXISTS idx_sync_records_kind_seq
			ON sync_records(kind, seq)`,
		`CREATE TABLE IF NOT EXISTS sync_tombstones (
			record_id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			erased_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sync_task_tombstones (
			task_id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			erased_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS proposals (
			id TEXT PRIMARY KEY,
			action_id TEXT NOT NULL,
			device_id TEXT NOT NULL REFERENCES devices(id),
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS approval_nonces (
			nonce TEXT PRIMARY KEY,
			proposal_id TEXT NOT NULL REFERENCES proposals(id),
			expires_at TEXT NOT NULL,
			used_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_approval_nonces_proposal_unused
			ON approval_nonces(proposal_id)
			WHERE used_at = ''`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			proposal_id TEXT NOT NULL DEFAULT '',
			device_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		)`,
		// Share-link labels live here, in the private database, and never in
		// the portal database. The portal only needs an opaque profile id;
		// "Mum", "work", or a clinician's name is owner data.
		`CREATE TABLE IF NOT EXISTS portal_profile_labels (
			profile_id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		)`,
		// The unique portal_request_id is what makes bridge submission
		// idempotent: a retry after a lost acknowledgement finds the existing
		// proposal instead of creating a second one.
		`CREATE TABLE IF NOT EXISTS portal_request_proposals (
			portal_request_id TEXT PRIMARY KEY,
			proposal_id TEXT NOT NULL UNIQUE REFERENCES proposals(id),
			profile_id TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		// Decisions bound for the portal store. Written inside the same
		// transaction as the decision itself, so a decision the visitor never
		// learns about cannot happen without losing the decision too.
		`CREATE TABLE IF NOT EXISTS portal_status_outbox (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			portal_request_id TEXT NOT NULL,
			status TEXT NOT NULL,
			decided_start TEXT NOT NULL DEFAULT '',
			decided_end TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		// The analysis loop's durable memory (ADR-0033). There is no queue of
		// pending recomputes here on purpose: a recompute is a pure function of
		// the inputs, so what is worth remembering is which inputs have already
		// been processed, not who asked. A restart compares fingerprints and
		// re-derives the need, which cannot lose work the way a dropped queue
		// message can. The fingerprints are encrypted because a digest still
		// answers a guess about when someone slept.
		`CREATE TABLE IF NOT EXISTS recompute_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			reason TEXT NOT NULL,
			state TEXT NOT NULL,
			started_at TEXT NOT NULL,
			completed_at TEXT NOT NULL DEFAULT '',
			content_changed_at TEXT NOT NULL DEFAULT '',
			valid_until TEXT NOT NULL DEFAULT '',
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recompute_runs_state_id
			ON recompute_runs(state, id)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate server sqlite: %w", err)
		}
	}
	if err := s.ensureColumn(ctx, "devices", "revoked_at", `TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?)`,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
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
		`SELECT id, label, created_at FROM devices WHERE token_hash = ? AND revoked_at = ''`,
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

func (s *Store) ListDevices(ctx context.Context) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, label, created_at, revoked_at FROM devices ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []Device
	for rows.Next() {
		var device Device
		var createdAt, revokedAt string
		if err := rows.Scan(&device.ID, &device.Label, &createdAt, &revokedAt); err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		device.CreatedAt = created.UTC()
		if revokedAt != "" {
			parsed, err := time.Parse(time.RFC3339Nano, revokedAt)
			if err != nil {
				return nil, err
			}
			parsed = parsed.UTC()
			device.RevokedAt = &parsed
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (s *Store) RevokeDevice(ctx context.Context, id string, revokedAt time.Time) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE devices SET revoked_at = ? WHERE id = ? AND revoked_at = ''`,
		revokedAt.UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrDeviceNotFound
	}
	return nil
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
		// A tombstoned record id can never be resurrected: a stale device
		// re-pushing an erased record is a silent no-op, not a conflict.
		var tombstoned int
		err := tx.QueryRowContext(ctx,
			`SELECT 1 FROM sync_tombstones WHERE record_id = ?`, record.RecordID,
		).Scan(&tombstoned)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, 0, err
		}
		if record.Kind == syncmodel.KindTask {
			taskID, ok := taskIDFromRevisionRecordID(record.RecordID)
			if ok {
				err = tx.QueryRowContext(ctx,
					`SELECT 1 FROM sync_task_tombstones WHERE task_id = ?`, taskID,
				).Scan(&tombstoned)
				if err == nil {
					continue
				}
				if !errors.Is(err, sql.ErrNoRows) {
					return 0, 0, err
				}
			}
		}

		sum := sha256.Sum256(record.Payload)
		payloadHash := sum[:]

		var existingSeq int64
		var existingHash []byte
		err = tx.QueryRowContext(ctx,
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
		ciphertext := s.encrypt(nonce, record.Payload, recordAAD(record.RecordID, record.Kind, deviceID, createdAt))
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

// EraseSyncRecords hard-deletes the named records' encrypted payloads and
// mints one tombstone per id: a registry row (which blocks any future push of
// that id) plus a tombstone envelope in the pull stream so every device learns
// to erase its local copy. Tombstone payloads carry only the record id — no
// record content. Its optional original kind prevents identifier-based routing.
// Erasing an id that was never synced still mints a kindless tombstone;
// erasing an already-tombstoned id is a no-op.
func (s *Store) EraseSyncRecords(ctx context.Context, deviceID string, recordIDs []string, erasedAt time.Time) (int, int, int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	erased := 0
	tombstones := 0
	erasedAtText := erasedAt.UTC().Format(time.RFC3339Nano)
	recordIDs, err = expandTaskErasureTargets(ctx, tx, deviceID, recordIDs, erasedAtText)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, recordID := range recordIDs {
		var erasedKind string
		err := tx.QueryRowContext(ctx,
			`SELECT kind FROM sync_records WHERE record_id = ? AND kind != ?`,
			recordID, string(syncmodel.KindTombstone),
		).Scan(&erasedKind)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, 0, 0, err
		}

		// Never delete a tombstone envelope: repeat erasures must leave the
		// durable erase signal in the pull stream intact.
		result, err := tx.ExecContext(ctx,
			`DELETE FROM sync_records WHERE record_id = ? AND kind != ?`,
			recordID, string(syncmodel.KindTombstone),
		)
		if err != nil {
			return 0, 0, 0, err
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return 0, 0, 0, err
		}
		erased += int(deleted)

		registry, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO sync_tombstones(record_id, device_id, erased_at) VALUES(?, ?, ?)`,
			recordID, deviceID, erasedAtText,
		)
		if err != nil {
			return 0, 0, 0, err
		}
		inserted, err := registry.RowsAffected()
		if err != nil {
			return 0, 0, 0, err
		}
		if inserted == 0 {
			continue // already tombstoned; envelope exists
		}
		tombstones++

		payload, err := json.Marshal(syncmodel.TombstonePayload{
			RecordID: recordID, RecordKind: syncmodel.Kind(erasedKind),
		})
		if err != nil {
			return 0, 0, 0, err
		}
		nonce := make([]byte, s.aead.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return 0, 0, 0, err
		}
		sum := sha256.Sum256(payload)
		ciphertext := s.encrypt(nonce, payload, recordAAD(recordID, syncmodel.KindTombstone, deviceID, erasedAtText))
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sync_records(record_id, kind, device_id, created_at, nonce, ciphertext, payload_hash)
			 VALUES(?, ?, ?, ?, ?, ?, ?)`,
			recordID, string(syncmodel.KindTombstone), deviceID, erasedAtText, nonce, ciphertext, sum[:],
		); err != nil {
			return 0, 0, 0, err
		}
	}

	cursor, err := maxCursor(ctx, tx)
	if err != nil {
		return 0, 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, 0, err
	}
	committed = true
	return erased, tombstones, cursor, nil
}

// expandTaskErasureTargets turns one known task revision into a logical task
// deletion. All retained revisions are erased now, and the task-level registry
// prevents an offline device from introducing a later revision after deletion.
func expandTaskErasureTargets(
	ctx context.Context,
	tx *sql.Tx,
	deviceID string,
	recordIDs []string,
	erasedAt string,
) ([]string, error) {
	result := make([]string, 0, len(recordIDs))
	seen := make(map[string]struct{}, len(recordIDs))
	appendID := func(recordID string) {
		if _, exists := seen[recordID]; exists {
			return
		}
		seen[recordID] = struct{}{}
		result = append(result, recordID)
	}

	for _, recordID := range recordIDs {
		appendID(recordID)
		var kind string
		err := tx.QueryRowContext(ctx,
			`SELECT kind FROM sync_records WHERE record_id = ? AND kind != ?`,
			recordID, string(syncmodel.KindTombstone),
		).Scan(&kind)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if syncmodel.Kind(kind) != syncmodel.KindTask {
			continue
		}
		taskID, ok := taskIDFromRevisionRecordID(recordID)
		if !ok {
			return nil, fmt.Errorf("stored task record %q has an invalid revision id", recordID)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO sync_task_tombstones(task_id, device_id, erased_at) VALUES(?, ?, ?)`,
			taskID, deviceID, erasedAt,
		); err != nil {
			return nil, err
		}
		revisions, err := taskRevisionRecordIDsTx(ctx, tx, taskID)
		if err != nil {
			return nil, err
		}
		for _, revisionID := range revisions {
			appendID(revisionID)
		}
	}
	return result, nil
}

func taskRevisionRecordIDsTx(ctx context.Context, tx *sql.Tx, taskID string) ([]string, error) {
	pattern := escapeSQLLike(taskID) + `\_r%`
	rows, err := tx.QueryContext(ctx, `SELECT record_id FROM sync_records
		WHERE kind = ? AND record_id LIKE ? ESCAPE '\'
		ORDER BY seq`, string(syncmodel.KindTask), pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var recordID string
		if err := rows.Scan(&recordID); err != nil {
			return nil, err
		}
		parsedTaskID, ok := taskIDFromRevisionRecordID(recordID)
		if ok && parsedTaskID == taskID {
			result = append(result, recordID)
		}
	}
	return result, rows.Err()
}

func taskIDFromRevisionRecordID(recordID string) (string, bool) {
	marker := strings.LastIndex(recordID, "_r")
	if marker < 1 || marker+2 == len(recordID) {
		return "", false
	}
	for _, char := range recordID[marker+2:] {
		if char < '0' || char > '9' {
			return "", false
		}
	}
	return recordID[:marker], true
}

func escapeSQLLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func (s *Store) CreateProposal(ctx context.Context, input ProposalInput) (ProposalRecord, error) {
	if input.ID == "" || input.ActionID == "" || input.DeviceID == "" {
		return ProposalRecord{}, errors.New("proposal id, action id, and device id are required")
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	if input.ExpiresAt.IsZero() || !input.ExpiresAt.After(input.CreatedAt) {
		return ProposalRecord{}, errors.New("proposal expiry must be after creation")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProposalRecord{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return ProposalRecord{}, err
	}
	createdAt := input.CreatedAt.UTC().Format(time.RFC3339Nano)
	updatedAt := createdAt
	expiresAt := input.ExpiresAt.UTC().Format(time.RFC3339Nano)
	ciphertext := s.encrypt(nonce, input.Payload, proposalAAD(input.ID, input.ActionID, createdAt))
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO proposals(id, action_id, device_id, status, created_at, updated_at, expires_at, nonce, ciphertext)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.ActionID, input.DeviceID, string(ProposalPending), createdAt, updatedAt, expiresAt, nonce, ciphertext,
	); err != nil {
		return ProposalRecord{}, err
	}

	tokenNonce, err := randomTokenNonce()
	if err != nil {
		return ProposalRecord{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO approval_nonces(nonce, proposal_id, expires_at) VALUES(?, ?, ?)`,
		tokenNonce, input.ID, expiresAt,
	); err != nil {
		return ProposalRecord{}, err
	}
	if err := s.appendAuditTx(ctx, tx, "proposal.created", input.ID, input.DeviceID, input.CreatedAt, input.Audit); err != nil {
		return ProposalRecord{}, err
	}
	claims := approvalClaims{
		ProposalID: input.ID,
		ActionID:   input.ActionID,
		DeviceID:   input.DeviceID,
		TargetHash: payloadHash(input.Payload),
		Nonce:      tokenNonce,
		ExpiresAt:  input.ExpiresAt.UTC().Unix(),
	}
	token, err := s.signApprovalClaims(claims)
	if err != nil {
		return ProposalRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProposalRecord{}, err
	}
	committed = true
	return ProposalRecord{
		ID:            input.ID,
		ActionID:      input.ActionID,
		DeviceID:      input.DeviceID,
		Status:        ProposalPending,
		CreatedAt:     input.CreatedAt.UTC(),
		UpdatedAt:     input.CreatedAt.UTC(),
		ExpiresAt:     input.ExpiresAt.UTC(),
		Payload:       append(json.RawMessage(nil), input.Payload...),
		DecisionToken: token,
	}, nil
}

// ListProposalPage returns active proposals before bounded newest-first history. A high-water row and snapshot time keep continuation pages stable.
func (s *Store) ListProposalPage(ctx context.Context, cursor ProposalPageCursor, limit int, now time.Time) (ProposalPage, error) {
	if limit <= 0 || limit > MaxProposalPageLimit {
		return ProposalPage{}, errors.New("proposal page limit is out of range")
	}
	if now.IsZero() {
		return ProposalPage{}, errors.New("proposal page time is required")
	}

	requestTime := now.UTC()
	if cursor.AfterRowID < 0 || cursor.ThroughRowID < 0 {
		return ProposalPage{}, errors.New("proposal cursor must not be negative")
	}
	if cursor.AfterRowID == 0 {
		if cursor.ThroughRowID != 0 || cursor.Active || !cursor.AsOf.IsZero() {
			return ProposalPage{}, errors.New("initial proposal cursor must be empty")
		}
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(rowid), 0) FROM proposals`).Scan(&cursor.ThroughRowID); err != nil {
			return ProposalPage{}, err
		}
		cursor.AsOf = requestTime
	} else if cursor.ThroughRowID < cursor.AfterRowID || cursor.AsOf.IsZero() || cursor.AsOf.After(requestTime) {
		return ProposalPage{}, errors.New("proposal continuation cursor is invalid")
	}
	if cursor.ThroughRowID == 0 {
		return ProposalPage{Records: []ProposalRecord{}}, nil
	}

	requestTimeText := requestTime.Format(time.RFC3339Nano)
	snapshotTime := cursor.AsOf.UTC()
	snapshotTimeText := snapshotTime.Format(time.RFC3339Nano)
	query := `SELECT p.rowid, p.id, p.action_id, p.device_id, p.status,
			p.created_at, p.updated_at, p.expires_at, p.nonce, p.ciphertext,
			approval.nonce
		 FROM proposals AS p
		 LEFT JOIN approval_nonces AS approval
		   ON approval.proposal_id = p.id
		  AND approval.used_at = ''
		  AND approval.expires_at > ?
		  AND p.status = ?
		  AND p.expires_at > ?
		 WHERE p.rowid <= ?`
	args := []any{requestTimeText, string(ProposalPending), requestTimeText, cursor.ThroughRowID}
	if cursor.AfterRowID > 0 {
		if cursor.Active {
			query += ` AND (
				(p.status = ? AND p.expires_at > ? AND p.rowid < ?)
				OR NOT (p.status = ? AND p.expires_at > ?)
			)`
			args = append(args,
				string(ProposalPending), snapshotTimeText, cursor.AfterRowID,
				string(ProposalPending), snapshotTimeText,
			)
		} else {
			query += ` AND NOT (p.status = ? AND p.expires_at > ?)
				AND p.rowid < ?`
			args = append(args, string(ProposalPending), snapshotTimeText, cursor.AfterRowID)
		}
	}
	query += ` ORDER BY
			CASE WHEN p.status = ? AND p.expires_at > ? THEN 0 ELSE 1 END,
			p.rowid DESC
		LIMIT ?`
	args = append(args, string(ProposalPending), snapshotTimeText, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ProposalPage{}, err
	}
	defer rows.Close()

	listed := make([]proposalPageRow, 0, limit+1)
	for rows.Next() {
		row, err := s.scanProposalPageRow(rows)
		if err != nil {
			return ProposalPage{}, err
		}
		listed = append(listed, row)
	}
	if err := rows.Err(); err != nil {
		return ProposalPage{}, err
	}

	page := ProposalPage{Records: make([]ProposalRecord, 0, min(len(listed), limit))}
	if len(listed) > limit {
		page.HasMore = true
		listed = listed[:limit]
		last := listed[len(listed)-1]
		page.NextCursor = ProposalPageCursor{
			AfterRowID:   last.rowID,
			ThroughRowID: cursor.ThroughRowID,
			Active:       proposalIsActiveAt(last.record, snapshotTime),
			AsOf:         snapshotTime,
		}
	}
	for _, row := range listed {
		record := row.record
		if proposalIsActiveAt(record, requestTime) && row.decisionNonce.Valid {
			token, err := s.signApprovalClaims(approvalClaims{
				ProposalID: record.ID,
				ActionID:   record.ActionID,
				DeviceID:   record.DeviceID,
				TargetHash: payloadHash(record.Payload),
				Nonce:      row.decisionNonce.String,
				ExpiresAt:  record.ExpiresAt.UTC().Unix(),
			})
			if err != nil {
				return ProposalPage{}, err
			}
			record.DecisionToken = token
		}
		page.Records = append(page.Records, record)
	}
	return page, nil
}

func proposalIsActiveAt(record ProposalRecord, at time.Time) bool {
	return record.Status == ProposalPending && record.ExpiresAt.After(at.UTC())
}

// decideHook runs inside the decision transaction, after the proposal row is
// updated and the one-use nonce consumed but before commit. It exists so a
// caller can bind extra state to the same atomic decision — the portal binds
// the visitor-status handoff — without reimplementing the token checks.
type decideHook func(ctx context.Context, tx *sql.Tx, record ProposalRecord, decision ProposalStatus, decidedAt time.Time) error

func (s *Store) DecideProposal(ctx context.Context, proposalID, deviceID string, decision ProposalStatus, token string, decidedAt time.Time, audit json.RawMessage) (ProposalRecord, error) {
	return s.decideProposal(ctx, proposalID, deviceID, decision, token, decidedAt, audit, nil)
}

func (s *Store) decideProposal(ctx context.Context, proposalID, deviceID string, decision ProposalStatus, token string, decidedAt time.Time, audit json.RawMessage, hook decideHook) (ProposalRecord, error) {
	if decision != ProposalApproved && decision != ProposalRejected {
		return ProposalRecord{}, errors.New("unsupported proposal decision")
	}
	claims, err := s.verifyApprovalToken(token)
	if err != nil {
		return ProposalRecord{}, err
	}
	if claims.ProposalID != proposalID {
		return ProposalRecord{}, ErrInvalidApprovalToken
	}
	if time.Unix(claims.ExpiresAt, 0).Before(decidedAt.UTC()) {
		return ProposalRecord{}, ErrExpiredApprovalToken
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProposalRecord{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var nonceExpiresAt, usedAt string
	if err := tx.QueryRowContext(ctx,
		`SELECT expires_at, used_at FROM approval_nonces WHERE nonce = ? AND proposal_id = ?`,
		claims.Nonce, proposalID,
	).Scan(&nonceExpiresAt, &usedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProposalRecord{}, ErrInvalidApprovalToken
		}
		return ProposalRecord{}, err
	}
	if usedAt != "" {
		return ProposalRecord{}, ErrUsedApprovalToken
	}
	nonceExpiry, err := time.Parse(time.RFC3339Nano, nonceExpiresAt)
	if err != nil {
		return ProposalRecord{}, err
	}
	if !nonceExpiry.After(decidedAt.UTC()) {
		return ProposalRecord{}, ErrExpiredApprovalToken
	}

	record, err := s.proposalByIDTx(ctx, tx, proposalID)
	if err != nil {
		return ProposalRecord{}, err
	}
	if record.Status != ProposalPending {
		return ProposalRecord{}, ErrProposalNotPending
	}
	// The claims must match the stored proposal (creator device included), but
	// the DECIDING device may be any of the user's enrolled devices — approval
	// is a single-user, cross-device action, authenticated and audited with the
	// deciding device's identity (ADR-0016).
	if record.ActionID != claims.ActionID || record.DeviceID != claims.DeviceID {
		return ProposalRecord{}, ErrInvalidApprovalToken
	}
	if payloadHash(record.Payload) != claims.TargetHash {
		return ProposalRecord{}, ErrInvalidApprovalToken
	}
	nowText := decidedAt.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx,
		`UPDATE proposals SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		string(decision), nowText, proposalID, string(ProposalPending),
	); err != nil {
		return ProposalRecord{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE approval_nonces SET used_at = ? WHERE nonce = ?`,
		nowText, claims.Nonce,
	); err != nil {
		return ProposalRecord{}, err
	}
	if err := s.appendAuditTx(ctx, tx, "proposal."+string(decision), proposalID, deviceID, decidedAt, audit); err != nil {
		return ProposalRecord{}, err
	}
	if hook != nil {
		if err := hook(ctx, tx, record, decision, decidedAt); err != nil {
			return ProposalRecord{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ProposalRecord{}, err
	}
	committed = true
	record.Status = decision
	record.UpdatedAt = decidedAt.UTC()
	return record, nil
}

type proposalScanner interface {
	Scan(dest ...any) error
}

type proposalPageRow struct {
	rowID         int64
	record        ProposalRecord
	decisionNonce sql.NullString
}

func (s *Store) scanProposalPageRow(row proposalScanner) (proposalPageRow, error) {
	var result proposalPageRow
	var status, createdAt, updatedAt, expiresAt string
	var nonce, ciphertext []byte
	if err := row.Scan(
		&result.rowID,
		&result.record.ID,
		&result.record.ActionID,
		&result.record.DeviceID,
		&status,
		&createdAt,
		&updatedAt,
		&expiresAt,
		&nonce,
		&ciphertext,
		&result.decisionNonce,
	); err != nil {
		return proposalPageRow{}, err
	}
	record, err := s.decodeProposal(result.record, status, createdAt, updatedAt, expiresAt, nonce, ciphertext)
	if err != nil {
		return proposalPageRow{}, err
	}
	result.record = record
	return result, nil
}

func (s *Store) scanProposal(row proposalScanner) (ProposalRecord, error) {
	var record ProposalRecord
	var status, createdAt, updatedAt, expiresAt string
	var nonce, ciphertext []byte
	if err := row.Scan(&record.ID, &record.ActionID, &record.DeviceID, &status, &createdAt, &updatedAt, &expiresAt, &nonce, &ciphertext); err != nil {
		return ProposalRecord{}, err
	}
	return s.decodeProposal(record, status, createdAt, updatedAt, expiresAt, nonce, ciphertext)
}

func (s *Store) decodeProposal(record ProposalRecord, status, createdAt, updatedAt, expiresAt string, nonce, ciphertext []byte) (ProposalRecord, error) {
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ProposalRecord{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return ProposalRecord{}, err
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return ProposalRecord{}, err
	}
	payload, err := s.decrypt(nonce, ciphertext, proposalAAD(record.ID, record.ActionID, createdAt))
	if err != nil {
		return ProposalRecord{}, err
	}
	record.Status = ProposalStatus(status)
	record.CreatedAt = created.UTC()
	record.UpdatedAt = updated.UTC()
	record.ExpiresAt = expires.UTC()
	record.Payload = append(json.RawMessage(nil), payload...)
	return record, nil
}

// ProposalByID reads one proposal. The returned record carries the decrypted
// payload but no decision token: minting a token is a separate, listed action.
func (s *Store) ProposalByID(ctx context.Context, id string) (ProposalRecord, error) {
	record, err := s.scanProposal(s.db.QueryRowContext(ctx,
		`SELECT id, action_id, device_id, status, created_at, updated_at, expires_at, nonce, ciphertext FROM proposals WHERE id = ?`,
		id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return ProposalRecord{}, ErrProposalNotFound
	}
	return record, err
}

func (s *Store) proposalByIDTx(ctx context.Context, tx *sql.Tx, id string) (ProposalRecord, error) {
	record, err := s.scanProposal(tx.QueryRowContext(ctx,
		`SELECT id, action_id, device_id, status, created_at, updated_at, expires_at, nonce, ciphertext FROM proposals WHERE id = ?`,
		id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return ProposalRecord{}, ErrProposalNotFound
	}
	return record, err
}

func (s *Store) appendAuditTx(ctx context.Context, tx *sql.Tx, eventType, proposalID, deviceID string, createdAt time.Time, payload json.RawMessage) error {
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	created := createdAt.UTC().Format(time.RFC3339Nano)
	ciphertext := s.encrypt(nonce, payload, auditAAD(eventType, proposalID, deviceID, created))
	_, err := tx.ExecContext(ctx,
		`INSERT INTO audit_events(event_type, proposal_id, device_id, created_at, nonce, ciphertext)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		eventType, proposalID, deviceID, created, nonce, ciphertext,
	)
	return err
}

func (s *Store) CountProposals(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM proposals`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// RecordHighWater captures a stable upper cursor for a multi-page read. Callers
// can continue serving a coherent snapshot while newer records are appended.
func (s *Store) RecordHighWater(ctx context.Context) (int64, error) {
	var cursor sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(seq) FROM sync_records`).Scan(&cursor); err != nil {
		return 0, err
	}
	if !cursor.Valid {
		return 0, nil
	}
	return cursor.Int64, nil
}

// PullKindThrough returns one encrypted-log kind without decrypting unrelated
// records. The through cursor must come from RecordHighWater when callers page.
func (s *Store) PullKindThrough(ctx context.Context, kind syncmodel.Kind, since, through int64, limit int) ([]syncmodel.Envelope, int64, error) {
	if since < 0 || through < 0 {
		return nil, 0, errors.New("cursors must not be negative")
	}
	switch kind {
	case syncmodel.KindObservation, syncmodel.KindCorrection, syncmodel.KindTask:
	default:
		return nil, 0, errors.New("stored record kind is not supported")
	}
	if limit <= 0 || limit > syncmodel.MaxPullRecords {
		limit = syncmodel.MaxPullRecords
	}
	if since >= through {
		return []syncmodel.Envelope{}, since, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, record_id, kind, device_id, created_at, nonce, ciphertext
		 FROM sync_records
		 WHERE kind = ? AND seq > ? AND seq <= ?
		 ORDER BY seq
		 LIMIT ?`,
		string(kind), since, through, limit,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	return s.scanSyncRows(rows, since)
}

func (s *Store) scanSyncRows(rows *sql.Rows, cursor int64) ([]syncmodel.Envelope, int64, error) {
	records := make([]syncmodel.Envelope, 0)
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
		plaintext, err := s.decrypt(nonce, ciphertext, recordAAD(record.RecordID, record.Kind, record.DeviceID, createdAt))
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

	return s.scanSyncRows(rows, since)
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

func proposalAAD(proposalID, actionID, createdAt string) []byte {
	return []byte(strings.Join([]string{"proposal", proposalID, actionID, createdAt}, "\x00"))
}

func auditAAD(eventType, proposalID, deviceID, createdAt string) []byte {
	return []byte(strings.Join([]string{"audit", eventType, proposalID, deviceID, createdAt}, "\x00"))
}

func (s *Store) encrypt(nonce []byte, plaintext []byte, aad []byte) []byte {
	return s.aead.Seal(nil, nonce, plaintext, aad)
}

func (s *Store) decrypt(nonce []byte, ciphertext []byte, aad []byte) ([]byte, error) {
	return s.aead.Open(nil, nonce, ciphertext, aad)
}

func randomTokenNonce() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func payloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func (s *Store) signApprovalClaims(claims approvalClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.signingKey)
	_, _ = mac.Write([]byte(encodedPayload))
	signature := mac.Sum(nil)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *Store) verifyApprovalToken(token string) (approvalClaims, error) {
	left, right, ok := strings.Cut(token, ".")
	if !ok || left == "" || right == "" {
		return approvalClaims{}, ErrInvalidApprovalToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(right)
	if err != nil {
		return approvalClaims{}, ErrInvalidApprovalToken
	}
	mac := hmac.New(sha256.New, s.signingKey)
	_, _ = mac.Write([]byte(left))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return approvalClaims{}, ErrInvalidApprovalToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(left)
	if err != nil {
		return approvalClaims{}, ErrInvalidApprovalToken
	}
	var claims approvalClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return approvalClaims{}, ErrInvalidApprovalToken
	}
	if claims.ProposalID == "" || claims.ActionID == "" || claims.DeviceID == "" ||
		claims.TargetHash == "" || claims.Nonce == "" || claims.ExpiresAt == 0 {
		return approvalClaims{}, ErrInvalidApprovalToken
	}
	return claims, nil
}
