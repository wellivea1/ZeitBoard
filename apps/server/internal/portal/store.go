// Package portal implements the public-facing availability portal (phase 5).
//
// Every type here is constructed with the portal database only. The package
// deliberately does not import the private store, so a defect in a public
// handler cannot reach sleep records, medications, tasks, or profile labels.
// The private side pushes allowlisted snapshots in through PublishSnapshot;
// nothing flows the other way in this slice.
//
// The portal is disabled by default. See config.PortalConfig and the exposure
// gate in docs/portal-design.md section 12.
package portal

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/argon2"

	_ "modernc.org/sqlite"
)

var (
	// ErrLinkNotUsable is returned for unknown, expired, and revoked links
	// alike. Collapsing the three at the store boundary is deliberate: a
	// handler cannot leak which case occurred because it is never told.
	ErrLinkNotUsable = errors.New("portal link is not usable")

	ErrProfileNotFound  = errors.New("portal profile not found")
	ErrPasscodeRejected = errors.New("portal passcode rejected")
	ErrSessionInvalid   = errors.New("portal session invalid")
	ErrNoSnapshot       = errors.New("portal snapshot has not been materialized")
)

// Snapshot status values. These are the only statuses the public surface can
// describe, and they carry no estimator internals.
const (
	StatusAvailable        = "available"
	StatusRefused          = "refused"
	StatusInsufficientData = "insufficient_data"
)

const (
	// MaxLinkLifetime caps how long a share link may live (design section 4).
	MaxLinkLifetime = 90 * 24 * time.Hour

	// SessionLifetime caps an authenticated portal session. The effective
	// expiry is the earlier of this and link expiry.
	SessionLifetime = 24 * time.Hour

	// MinPasscodeLength is a usability floor, not a security claim. The
	// throttle in ratelimit.go is what makes short passcodes survivable.
	MinPasscodeLength = 6
	MaxPasscodeLength = 128
)

// Argon2id parameters. Stored per row so they can be raised later without
// invalidating existing passcodes.
const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// Grants are the portal-side copy of what a link is allowed to do. Only the
// fields the public handlers must enforce live here; the private display label
// stays in the owner's store.
type Grants struct {
	WakingWindows bool `json:"wakingWindows"`
	AllowRequests bool `json:"allowRequests"`
	AllowMessages bool `json:"allowMessages"`
}

// Profile is the portal-side projection of a share profile. It has no label,
// no owner identity, and no health fields by construction.
type Profile struct {
	ID        string
	Grants    Grants
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Window is one likely-awake interval. Half-open: [StartAt, EndAt).
type Window struct {
	StartAt time.Time `json:"startAt"`
	EndAt   time.Time `json:"endAt"`
	ZoneID  string    `json:"zoneId"`
}

// Snapshot is the entire set of facts the public surface may describe. Adding
// a field here widens the public boundary and requires the design review in
// docs/portal-design.md section 5.
type Snapshot struct {
	Version     int64
	Windows     []Window
	GeneratedAt time.Time
	HorizonEnd  time.Time
	Status      string
}

type Store struct {
	db      *sql.DB
	aead    cipher.AEAD
	csrfKey []byte
}

// Open creates or opens the portal database. rootKey is the daemon data key;
// the portal key is derived one-way from it so that reading the portal
// database never yields the private store's key.
func Open(path string, rootKey []byte) (*Store, error) {
	if len(rootKey) != 32 {
		return nil, errors.New("portal root key must be 32 bytes")
	}
	if path == "" {
		return nil, errors.New("portal database path is required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create portal data directory: %w", err)
		}
	}
	derived := sha256.Sum256(append([]byte("zeitboard-portal-store\x00"), rootKey...))
	block, err := aes.NewCipher(derived[:])
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
	csrfKey := sha256.Sum256(append([]byte("zeitboard-portal-csrf\x00"), derived[:]...))
	store := &Store{db: db, aead: aead, csrfKey: csrfKey[:]}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA secure_delete = ON`,
		`CREATE TABLE IF NOT EXISTS portal_profiles (
			profile_id TEXT PRIMARY KEY,
			token_hash BLOB NOT NULL UNIQUE,
			grant_windows INTEGER NOT NULL,
			grant_requests INTEGER NOT NULL,
			grant_messages INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			revoked_at TEXT NOT NULL DEFAULT '',
			passcode_hash BLOB NOT NULL,
			passcode_salt BLOB NOT NULL,
			passcode_time INTEGER NOT NULL,
			passcode_memory INTEGER NOT NULL,
			passcode_threads INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS portal_snapshots (
			profile_id TEXT PRIMARY KEY REFERENCES portal_profiles(profile_id) ON DELETE CASCADE,
			version INTEGER NOT NULL,
			generated_at TEXT NOT NULL,
			horizon_end TEXT NOT NULL,
			status TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS portal_sessions (
			session_hash BLOB PRIMARY KEY,
			profile_id TEXT NOT NULL REFERENCES portal_profiles(profile_id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_portal_sessions_profile
			ON portal_sessions(profile_id)`,
		`CREATE TABLE IF NOT EXISTS portal_rate_buckets (
			bucket_key TEXT PRIMARY KEY,
			window_start TEXT NOT NULL,
			count INTEGER NOT NULL,
			failures INTEGER NOT NULL DEFAULT 0,
			blocked_until TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS portal_access_audit (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			profile_id TEXT NOT NULL,
			event TEXT NOT NULL,
			source_hmac TEXT NOT NULL,
			occurred_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_portal_access_audit_profile
			ON portal_access_audit(profile_id, id)`,
		`CREATE TABLE IF NOT EXISTS portal_audit_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key BLOB NOT NULL,
			created_at TEXT NOT NULL
		)`,
		// Visitor requests. handle and message are private visitor text and
		// live only inside the encrypted blob; no column holds them in the
		// clear, so an index or a SELECT * cannot expose them by accident.
		`CREATE TABLE IF NOT EXISTS portal_requests (
			request_id TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL REFERENCES portal_profiles(profile_id) ON DELETE CASCADE,
			session_hash BLOB NOT NULL,
			secret_hash BLOB NOT NULL,
			window_start TEXT NOT NULL,
			window_end TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			duration_minutes INTEGER NOT NULL DEFAULT 0,
			beyond_horizon INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			decided_start TEXT NOT NULL DEFAULT '',
			decided_end TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_portal_requests_profile_status
			ON portal_requests(profile_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_portal_requests_session
			ON portal_requests(session_hash, created_at)`,
		// The transactional outbox. A row here means "durably accepted from
		// the visitor, not yet confirmed by the owner's queue".
		`CREATE TABLE IF NOT EXISTS portal_outbox (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL,
			request_id TEXT NOT NULL,
			idempotency_key TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS portal_request_sessions (
			session_hash BLOB PRIMARY KEY,
			request_id TEXT NOT NULL REFERENCES portal_requests(request_id) ON DELETE CASCADE,
			profile_id TEXT NOT NULL,
			expires_at TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("portal migrate: %w", err)
		}
	}
	return s.ensureAuditKey(ctx)
}

// CreateProfileInput carries a new link. The caller supplies the raw token and
// passcode; only derived values are persisted.
type CreateProfileInput struct {
	ProfileID string
	Token     string
	Passcode  string
	Grants    Grants
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (s *Store) CreateProfile(ctx context.Context, input CreateProfileInput) error {
	switch {
	case input.ProfileID == "":
		return errors.New("portal profile id is required")
	case input.Token == "":
		return errors.New("portal link token is required")
	case len(input.Passcode) < MinPasscodeLength:
		return fmt.Errorf("portal passcode must be at least %d characters", MinPasscodeLength)
	case len(input.Passcode) > MaxPasscodeLength:
		return fmt.Errorf("portal passcode must be at most %d characters", MaxPasscodeLength)
	case !input.ExpiresAt.After(input.CreatedAt):
		return errors.New("portal link expiry must be in the future")
	case input.ExpiresAt.Sub(input.CreatedAt) > MaxLinkLifetime:
		return fmt.Errorf("portal link lifetime must not exceed %d days", int(MaxLinkLifetime.Hours()/24))
	}

	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	hash := argon2.IDKey([]byte(input.Passcode), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	tokenHash := hashToken(input.Token)

	_, err := s.db.ExecContext(ctx, `INSERT INTO portal_profiles
		(profile_id, token_hash, grant_windows, grant_requests, grant_messages,
		 created_at, expires_at, revoked_at,
		 passcode_hash, passcode_salt, passcode_time, passcode_memory, passcode_threads)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?)`,
		input.ProfileID, tokenHash[:],
		boolToInt(input.Grants.WakingWindows), boolToInt(input.Grants.AllowRequests), boolToInt(input.Grants.AllowMessages),
		formatTime(input.CreatedAt), formatTime(input.ExpiresAt),
		hash, salt, int64(argonTime), int64(argonMemory), int64(argonThreads))
	if err != nil {
		return fmt.Errorf("create portal profile: %w", err)
	}
	return nil
}

// ResolveLink maps a raw link token to a usable profile. Unknown, expired, and
// revoked tokens all produce ErrLinkNotUsable.
func (s *Store) ResolveLink(ctx context.Context, token string, now time.Time) (Profile, error) {
	if token == "" {
		return Profile{}, ErrLinkNotUsable
	}
	tokenHash := hashToken(token)
	row := s.db.QueryRowContext(ctx, `SELECT profile_id, grant_windows, grant_requests, grant_messages,
		created_at, expires_at, revoked_at
		FROM portal_profiles WHERE token_hash = ?`, tokenHash[:])
	profile, expired, revoked, err := scanProfile(row, now)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Profile{}, ErrLinkNotUsable
	case err != nil:
		return Profile{}, err
	case expired || revoked:
		return Profile{}, ErrLinkNotUsable
	}
	return profile, nil
}

// LookupProfile is the owner-side read. Unlike ResolveLink it distinguishes
// missing from revoked, because the owner is entitled to that difference.
func (s *Store) LookupProfile(ctx context.Context, profileID string, now time.Time) (Profile, bool, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT profile_id, grant_windows, grant_requests, grant_messages,
		created_at, expires_at, revoked_at
		FROM portal_profiles WHERE profile_id = ?`, profileID)
	profile, expired, revoked, err := scanProfile(row, now)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, false, false, ErrProfileNotFound
	}
	if err != nil {
		return Profile{}, false, false, err
	}
	return profile, expired, revoked, nil
}

func scanProfile(row *sql.Row, now time.Time) (Profile, bool, bool, error) {
	var (
		profile                             Profile
		windows, requests, messages         int
		createdAtRaw, expiresAtRaw, revoked string
	)
	if err := row.Scan(&profile.ID, &windows, &requests, &messages, &createdAtRaw, &expiresAtRaw, &revoked); err != nil {
		return Profile{}, false, false, err
	}
	createdAt, err := parseTime(createdAtRaw)
	if err != nil {
		return Profile{}, false, false, err
	}
	expiresAt, err := parseTime(expiresAtRaw)
	if err != nil {
		return Profile{}, false, false, err
	}
	profile.CreatedAt = createdAt
	profile.ExpiresAt = expiresAt
	profile.Grants = Grants{
		WakingWindows: windows == 1,
		AllowRequests: requests == 1,
		AllowMessages: messages == 1,
	}
	return profile, !expiresAt.After(now), revoked != "", nil
}

// ListActiveProfiles returns profiles that are neither expired nor revoked.
// The materializer uses it to decide what to refresh; the owner UI uses
// ListProfiles, which also shows dead links.
func (s *Store) ListActiveProfiles(ctx context.Context, now time.Time) ([]Profile, error) {
	return s.listProfiles(ctx, now, true)
}

// ListProfiles returns every profile, including expired and revoked ones, so
// the owner can audit links they have already turned off.
func (s *Store) ListProfiles(ctx context.Context, now time.Time) ([]ProfileState, error) {
	profiles, err := s.listProfileStates(ctx, now)
	if err != nil {
		return nil, err
	}
	return profiles, nil
}

// ProfileState adds the owner-visible lifecycle that ResolveLink deliberately
// refuses to distinguish for the public.
type ProfileState struct {
	Profile
	Expired bool
	Revoked bool
}

func (s *Store) listProfiles(ctx context.Context, now time.Time, activeOnly bool) ([]Profile, error) {
	states, err := s.listProfileStates(ctx, now)
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(states))
	for _, state := range states {
		if activeOnly && (state.Expired || state.Revoked) {
			continue
		}
		profiles = append(profiles, state.Profile)
	}
	return profiles, nil
}

func (s *Store) listProfileStates(ctx context.Context, now time.Time) ([]ProfileState, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT profile_id, grant_windows, grant_requests, grant_messages,
		created_at, expires_at, revoked_at FROM portal_profiles ORDER BY created_at, profile_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var states []ProfileState
	for rows.Next() {
		var (
			profile                             Profile
			windows, requests, messages         int
			createdAtRaw, expiresAtRaw, revoked string
		)
		if err := rows.Scan(&profile.ID, &windows, &requests, &messages, &createdAtRaw, &expiresAtRaw, &revoked); err != nil {
			return nil, err
		}
		createdAt, err := parseTime(createdAtRaw)
		if err != nil {
			return nil, err
		}
		expiresAt, err := parseTime(expiresAtRaw)
		if err != nil {
			return nil, err
		}
		profile.CreatedAt = createdAt
		profile.ExpiresAt = expiresAt
		profile.Grants = Grants{
			WakingWindows: windows == 1,
			AllowRequests: requests == 1,
			AllowMessages: messages == 1,
		}
		states = append(states, ProfileState{
			Profile: profile,
			Expired: !expiresAt.After(now),
			Revoked: revoked != "",
		})
	}
	return states, rows.Err()
}

// RevokeProfile is the kill switch. It drops sessions immediately so that an
// already-authenticated visitor cannot keep reading through an open session,
// and it deletes the materialized snapshot so no availability data survives in
// the portal database.
func (s *Store) RevokeProfile(ctx context.Context, profileID string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx,
		`UPDATE portal_profiles SET revoked_at = ? WHERE profile_id = ? AND revoked_at = ''`,
		formatTime(now), profileID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM portal_profiles WHERE profile_id = ?`, profileID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrProfileNotFound
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM portal_sessions WHERE profile_id = ?`, profileID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM portal_snapshots WHERE profile_id = ?`, profileID); err != nil {
		return err
	}
	// Open requests are closed rather than deleted: the owner may still have a
	// pending proposal referencing one, and a request that silently vanished
	// would leave the visitor watching a status that never moves.
	if _, err := tx.ExecContext(ctx,
		`UPDATE portal_requests SET status = ?, updated_at = ? WHERE profile_id = ? AND status IN (?, ?)`,
		RequestClosed, formatTime(now), profileID, RequestQueued, RequestPending); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM portal_request_sessions WHERE profile_id = ?`, profileID); err != nil {
		return err
	}
	return tx.Commit()
}

// PublishSnapshot replaces the materialized availability for one profile. It
// is the only way availability data enters the portal database.
func (s *Store) PublishSnapshot(ctx context.Context, profileID string, snapshot Snapshot) error {
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	payload, err := json.Marshal(snapshot.Windows)
	if err != nil {
		return err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ciphertext := s.aead.Seal(nil, nonce, payload, snapshotAAD(profileID, snapshot.Version))

	result, err := s.db.ExecContext(ctx, `INSERT INTO portal_snapshots
		(profile_id, version, generated_at, horizon_end, status, nonce, ciphertext)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id) DO UPDATE SET
			version = excluded.version,
			generated_at = excluded.generated_at,
			horizon_end = excluded.horizon_end,
			status = excluded.status,
			nonce = excluded.nonce,
			ciphertext = excluded.ciphertext
		WHERE excluded.version >= portal_snapshots.version`,
		profileID, snapshot.Version, formatTime(snapshot.GeneratedAt), formatTime(snapshot.HorizonEnd),
		snapshot.Status, nonce, ciphertext)
	if err != nil {
		return fmt.Errorf("publish portal snapshot: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		// An older version lost the race with a newer one. That is the
		// intended outcome, not an error.
		return nil
	}
	return nil
}

func (s *Store) ReadSnapshot(ctx context.Context, profileID string) (Snapshot, error) {
	row := s.db.QueryRowContext(ctx, `SELECT version, generated_at, horizon_end, status, nonce, ciphertext
		FROM portal_snapshots WHERE profile_id = ?`, profileID)
	var (
		snapshot                 Snapshot
		generatedRaw, horizonRaw string
		nonce, ciphertext        []byte
	)
	if err := row.Scan(&snapshot.Version, &generatedRaw, &horizonRaw, &snapshot.Status, &nonce, &ciphertext); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNoSnapshot
		}
		return Snapshot{}, err
	}
	generatedAt, err := parseTime(generatedRaw)
	if err != nil {
		return Snapshot{}, err
	}
	horizonEnd, err := parseTime(horizonRaw)
	if err != nil {
		return Snapshot{}, err
	}
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, snapshotAAD(profileID, snapshot.Version))
	if err != nil {
		return Snapshot{}, fmt.Errorf("decrypt portal snapshot: %w", err)
	}
	var windows []Window
	if err := json.Unmarshal(plaintext, &windows); err != nil {
		return Snapshot{}, err
	}
	snapshot.GeneratedAt = generatedAt
	snapshot.HorizonEnd = horizonEnd
	snapshot.Windows = windows
	return snapshot, nil
}

func validateSnapshot(snapshot Snapshot) error {
	switch snapshot.Status {
	case StatusAvailable, StatusRefused, StatusInsufficientData:
	default:
		return fmt.Errorf("unsupported portal snapshot status %q", snapshot.Status)
	}
	if snapshot.Version <= 0 {
		return errors.New("portal snapshot version must be positive")
	}
	if snapshot.GeneratedAt.IsZero() {
		return errors.New("portal snapshot requires a generation time")
	}
	for _, window := range snapshot.Windows {
		if !window.EndAt.After(window.StartAt) {
			return errors.New("portal snapshot windows must be non-empty half-open intervals")
		}
		if window.ZoneID == "" {
			return errors.New("portal snapshot windows require a zone id")
		}
	}
	return nil
}

// VerifyPasscode reports whether the supplied passcode matches. It always runs
// the KDF, including for links that cannot be used, so a caller cannot
// distinguish "no such link" from "wrong passcode" by timing alone.
func (s *Store) VerifyPasscode(ctx context.Context, profileID, passcode string) error {
	row := s.db.QueryRowContext(ctx,
		`SELECT passcode_hash, passcode_salt, passcode_time, passcode_memory, passcode_threads
		 FROM portal_profiles WHERE profile_id = ?`, profileID)
	var (
		storedHash, salt              []byte
		timeCost, memoryCost, threads int64
	)
	if err := row.Scan(&storedHash, &salt, &timeCost, &memoryCost, &threads); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Burn comparable work before refusing so the absence of a
			// profile is not measurably cheaper than a wrong passcode.
			decoySalt := make([]byte, argonSaltLen)
			argon2.IDKey([]byte(passcode), decoySalt, argonTime, argonMemory, argonThreads, argonKeyLen)
			return ErrPasscodeRejected
		}
		return err
	}
	computed := argon2.IDKey([]byte(passcode), salt, uint32(timeCost), uint32(memoryCost), uint8(threads), uint32(len(storedHash)))
	if subtle.ConstantTimeCompare(computed, storedHash) != 1 {
		return ErrPasscodeRejected
	}
	return nil
}

// SessionToken is the pair handed to a visitor after a successful passcode.
type SessionToken struct {
	Session   string
	CSRF      string
	ExpiresAt time.Time
}

// CreateSession issues a portal session bound to one profile. The session
// expires at the earlier of SessionLifetime and link expiry, so revoking or
// letting a link lapse cannot be outlived by a cookie.
func (s *Store) CreateSession(ctx context.Context, profile Profile, now time.Time) (SessionToken, error) {
	sessionValue, err := randomToken()
	if err != nil {
		return SessionToken{}, err
	}
	expiresAt := now.Add(SessionLifetime)
	if profile.ExpiresAt.Before(expiresAt) {
		expiresAt = profile.ExpiresAt
	}
	sessionHash := hashToken(sessionValue)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO portal_sessions
		(session_hash, profile_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		sessionHash[:], profile.ID, formatTime(now), formatTime(expiresAt)); err != nil {
		return SessionToken{}, fmt.Errorf("create portal session: %w", err)
	}
	return SessionToken{Session: sessionValue, CSRF: s.CSRFToken(sessionValue), ExpiresAt: expiresAt}, nil
}

// CSRFToken derives the synchronizer token for a session rather than storing
// it. A server-rendered form has to embed the token on every page render, so
// the plaintext must be recoverable from the session cookie; storing only a
// hash would make it unrecoverable and the mechanism unusable. Deriving it
// under a key held by the server keeps it unguessable without persisting a
// second secret.
func (s *Store) CSRFToken(sessionValue string) string {
	if sessionValue == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.csrfKey)
	_, _ = mac.Write([]byte(sessionValue))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// MatchesCSRF compares a presented synchronizer token in constant time.
func (s *Store) MatchesCSRF(sessionValue, presented string) bool {
	if sessionValue == "" || presented == "" {
		return false
	}
	return constantTimeEqual([]byte(s.CSRFToken(sessionValue)), []byte(presented))
}

// Session is a resolved, unexpired portal session.
type Session struct {
	ProfileID string
	ExpiresAt time.Time
}

func (s *Store) ResolveSession(ctx context.Context, sessionValue string, now time.Time) (Session, error) {
	if sessionValue == "" {
		return Session{}, ErrSessionInvalid
	}
	sessionHash := hashToken(sessionValue)
	row := s.db.QueryRowContext(ctx,
		`SELECT profile_id, expires_at FROM portal_sessions WHERE session_hash = ?`, sessionHash[:])
	var (
		session      Session
		expiresAtRaw string
	)
	if err := row.Scan(&session.ProfileID, &expiresAtRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrSessionInvalid
		}
		return Session{}, err
	}
	expiresAt, err := parseTime(expiresAtRaw)
	if err != nil {
		return Session{}, err
	}
	if !expiresAt.After(now) {
		return Session{}, ErrSessionInvalid
	}
	session.ExpiresAt = expiresAt
	return session, nil
}

func constantTimeEqual(left, right []byte) bool {
	return subtle.ConstantTimeCompare(left, right) == 1
}

func (s *Store) DeleteSession(ctx context.Context, sessionValue string) error {
	sessionHash := hashToken(sessionValue)
	_, err := s.db.ExecContext(ctx, `DELETE FROM portal_sessions WHERE session_hash = ?`, sessionHash[:])
	return err
}

// AuditRetention bounds how long coarse access rows are kept. Retention that
// is only documented is not retention, so PurgeExpired enforces it and the
// daemon runs PurgeExpired on a timer.
const AuditRetention = 30 * 24 * time.Hour

// PurgeExpired removes expired sessions, stale rate buckets, and access-audit
// rows past AuditRetention. It is safe to call concurrently with request
// handling.
func (s *Store) PurgeExpired(ctx context.Context, now time.Time) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM portal_sessions WHERE expires_at <= ?`, formatTime(now)); err != nil {
		return err
	}
	bucketCutoff := formatTime(now.Add(-24 * time.Hour))
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM portal_rate_buckets WHERE window_start <= ? AND blocked_until <= ?`,
		bucketCutoff, formatTime(now)); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM portal_access_audit WHERE occurred_at <= ?`,
		formatTime(now.Add(-AuditRetention))); err != nil {
		return err
	}
	return nil
}

func hashToken(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// NewLinkToken mints a 256-bit link token. It is shown to the owner once and
// stored only as a hash.
func NewLinkToken() (string, error) { return randomToken() }

// NewProfileID mints an opaque profile identifier.
func NewProfileID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func snapshotAAD(profileID string, version int64) []byte {
	return []byte(fmt.Sprintf("portal-snapshot\x00%s\x00%d", profileID, version))
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}
