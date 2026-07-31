package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// MaxPortalLabelLength bounds the owner's private note about a share link.
const MaxPortalLabelLength = 80

// PutPortalLabel stores the owner's private name for a share profile. The
// label is encrypted at rest and lives only in the private database: the
// portal store is given the opaque profile id and nothing else, so a compromise
// of the public surface cannot reveal who a link was shared with.
func (s *Store) PutPortalLabel(ctx context.Context, profileID, label, createdAt string) error {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return errors.New("portal profile id is required")
	}
	if len([]rune(label)) > MaxPortalLabelLength {
		return fmt.Errorf("portal label must be at most %d characters", MaxPortalLabelLength)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ciphertext := s.encrypt(nonce, []byte(label), portalLabelAAD(profileID, createdAt))
	_, err := s.db.ExecContext(ctx, `INSERT INTO portal_profile_labels
		(profile_id, created_at, nonce, ciphertext) VALUES (?, ?, ?, ?)
		ON CONFLICT(profile_id) DO UPDATE SET
			created_at = excluded.created_at,
			nonce = excluded.nonce,
			ciphertext = excluded.ciphertext`,
		profileID, createdAt, nonce, ciphertext)
	if err != nil {
		return fmt.Errorf("store portal label: %w", err)
	}
	return nil
}

// PortalLabel returns the private label, or an empty string when none exists.
func (s *Store) PortalLabel(ctx context.Context, profileID string) (string, error) {
	var (
		createdAt         string
		nonce, ciphertext []byte
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT created_at, nonce, ciphertext FROM portal_profile_labels WHERE profile_id = ?`, profileID).
		Scan(&createdAt, &nonce, &ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	plaintext, err := s.decrypt(nonce, ciphertext, portalLabelAAD(profileID, createdAt))
	if err != nil {
		return "", fmt.Errorf("decrypt portal label: %w", err)
	}
	return string(plaintext), nil
}

// PortalLabels returns every stored label keyed by profile id.
func (s *Store) PortalLabels(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, created_at, nonce, ciphertext FROM portal_profile_labels`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	labels := make(map[string]string)
	for rows.Next() {
		var (
			profileID, createdAt string
			nonce, ciphertext    []byte
		)
		if err := rows.Scan(&profileID, &createdAt, &nonce, &ciphertext); err != nil {
			return nil, err
		}
		plaintext, err := s.decrypt(nonce, ciphertext, portalLabelAAD(profileID, createdAt))
		if err != nil {
			return nil, fmt.Errorf("decrypt portal label: %w", err)
		}
		labels[profileID] = string(plaintext)
	}
	return labels, rows.Err()
}

// DeletePortalLabel erases the private label. Revoking a link removes portal
// state; this removes the owner-side name for it.
func (s *Store) DeletePortalLabel(ctx context.Context, profileID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM portal_profile_labels WHERE profile_id = ?`, profileID)
	return err
}

func portalLabelAAD(profileID, createdAt string) []byte {
	return []byte(strings.Join([]string{"portal-label", profileID, createdAt}, "\x00"))
}
