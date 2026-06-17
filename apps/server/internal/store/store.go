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
		`CREATE TABLE IF NOT EXISTS audit_events (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			proposal_id TEXT NOT NULL DEFAULT '',
			device_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		)`,
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

func (s *Store) ListProposals(ctx context.Context) ([]ProposalRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, action_id, device_id, status, created_at, updated_at, expires_at, nonce, ciphertext
		 FROM proposals ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []ProposalRecord
	for rows.Next() {
		record, err := s.scanProposal(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) DecideProposal(ctx context.Context, proposalID, deviceID string, decision ProposalStatus, token string, decidedAt time.Time, audit json.RawMessage) (ProposalRecord, error) {
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
	if record.ActionID != claims.ActionID || record.DeviceID != claims.DeviceID || record.DeviceID != deviceID {
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

func (s *Store) scanProposal(row proposalScanner) (ProposalRecord, error) {
	var record ProposalRecord
	var status, createdAt, updatedAt, expiresAt string
	var nonce, ciphertext []byte
	if err := row.Scan(&record.ID, &record.ActionID, &record.DeviceID, &status, &createdAt, &updatedAt, &expiresAt, &nonce, &ciphertext); err != nil {
		return ProposalRecord{}, err
	}
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
