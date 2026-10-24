package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// maxSealedSettingBytes bounds one sealed setting.
const maxSealedSettingBytes = 64 * 1024

// SealedSetting reads one of the server's own settings, which are kept
// encrypted whole. ok is false when none is kept under name.
func (s *Store) SealedSetting(ctx context.Context, name string) ([]byte, bool, error) {
	var (
		updatedAt         string
		nonce, ciphertext []byte
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT updated_at, nonce, ciphertext FROM sealed_settings WHERE name = ?`, name).
		Scan(&updatedAt, &nonce, &ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	value, err := s.decrypt(nonce, ciphertext, sealedSettingAAD(name, updatedAt))
	if err != nil {
		return nil, false, fmt.Errorf("decrypt sealed setting: %w", err)
	}
	return value, true, nil
}

// PutSealedSetting replaces one setting, encrypting it whole. An event names
// a change the owner made from a device, and is audited with it in one
// transaction; a change the server makes on its own, such as keeping a
// refresh token a provider rotated, passes no event. Secure delete, which the
// database runs with, overwrites what is replaced.
func (s *Store) PutSealedSetting(ctx context.Context, name string, value []byte, at time.Time, event, deviceID string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("a sealed setting needs a name")
	}
	if len(value) > maxSealedSettingBytes {
		return fmt.Errorf("a sealed setting must be at most %d bytes", maxSealedSettingBytes)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	updatedAt := at.UTC().Format(time.RFC3339Nano)
	ciphertext := s.encrypt(nonce, value, sealedSettingAAD(name, updatedAt))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sealed_settings(name, updated_at, nonce, ciphertext)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			updated_at = excluded.updated_at,
			nonce = excluded.nonce,
			ciphertext = excluded.ciphertext`,
		name, updatedAt, nonce, ciphertext); err != nil {
		return fmt.Errorf("store sealed setting: %w", err)
	}
	if event != "" {
		payload, err := json.Marshal(map[string]string{"setting": name})
		if err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, event, "", deviceID, at, payload); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func sealedSettingAAD(name, updatedAt string) []byte {
	return []byte(strings.Join([]string{"sealed-setting", name, updatedAt}, "\x00"))
}
