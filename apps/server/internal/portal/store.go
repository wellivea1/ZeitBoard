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
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
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

	// SkippedDoses shows when the owner logged a dose as skipped, by time
	// alone (ADR-0058). Off unless the owner turns it on for the link.
	SkippedDoses bool `json:"skippedDoses"`

	// ReportedStatus is how much of a status the owner reported this link
	// may show (ADR-0070): nothing, only that it is not a good time, or the
	// status itself with when it was said. Nothing unless the owner chooses.
	ReportedStatus string `json:"reportedStatus"`
}

// How much of a reported status a link shows.
const (
	// StatusGrantNone: the link shows the forecast and no report.
	StatusGrantNone = ""
	// StatusGrantContact: the link says only that now is not a good time,
	// while the owner is trying to sleep or has asked not to be disturbed.
	// It does not say which, or when it was said.
	StatusGrantContact = "contact"
	// StatusGrantFull: the link shows the status and when it was said.
	StatusGrantFull = "full"
)

// StatusUnavailable is all a contact-only link is told: not a good time.
const StatusUnavailable = "unavailable"

// ValidStatusGrant reports whether level is one a link can hold.
func ValidStatusGrant(level string) bool {
	return level == StatusGrantNone || level == StatusGrantContact || level == StatusGrantFull
}

// ReportedStatus is a status the owner reported, as far as one link may show
// it. On a contact-only link Status is StatusUnavailable and ReportedAt is
// unset.
type ReportedStatus struct {
	Status     string    `json:"status"`
	ReportedAt time.Time `json:"reportedAt,omitzero"`
	ExpiresAt  time.Time `json:"expiresAt"`
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

// SkippedDose is when the owner logged a dose as skipped: a time and the zone
// it was logged in, and nothing about the medication (ADR-0058).
type SkippedDose struct {
	At     time.Time `json:"at"`
	ZoneID string    `json:"zoneId"`
}

// SkippedDoseWindow is how far back a link shows doses logged as skipped:
// long enough for someone who looks once a day to see each one, short enough
// that the page is not a record of the owner's adherence.
const SkippedDoseWindow = 72 * time.Hour

// maxSkippedDoses bounds what one snapshot can carry. Even a dose every hour
// for three days fits.
const maxSkippedDoses = 96

// Snapshot is the entire set of facts the public surface may describe. Adding
// a field here widens the public boundary and requires the design review in
// docs/portal-design.md section 5.
type Snapshot struct {
	Version     int64
	Windows     []Window
	GeneratedAt time.Time
	HorizonEnd  time.Time
	Status      string

	// DosesShared is true for a link with the SkippedDoses grant, so the page
	// can tell "none logged as skipped" from "not shared". SkippedDoses is
	// empty on every other link.
	DosesShared  bool
	SkippedDoses []SkippedDose

	// Reported is the status the owner reported, cut down to what this link
	// may show, or nil. It is read against the clock when the page is drawn.
	Reported *ReportedStatus

	// CautionAt and WithheldAt are when this snapshot's forecast turns with
	// no new record, as the shared freshness policy judged it when the
	// snapshot was made (ADR-0071): from CautionAt the page adds its
	// caution, and from WithheldAt it shows no windows. The page applies
	// them itself, so a claim cannot outlive them if nothing republishes.
	// They stay on the server. No reader is sent them.
	CautionAt  time.Time
	WithheldAt time.Time
}

// deadlines are when the page adds its caution and when it stops showing the
// forecast. A snapshot published without them keeps the fixed hours from the
// time it last changed.
func (s Snapshot) deadlines() (cautionAt, withheldAt time.Time) {
	if !s.WithheldAt.IsZero() {
		return s.CautionAt, s.WithheldAt
	}
	if s.GeneratedAt.IsZero() {
		return time.Time{}, time.Time{}
	}
	return s.GeneratedAt.Add(StaleAfter), s.GeneratedAt.Add(UnavailableAfter)
}

// cautionDue reports whether the forecast is shown with its caution at now.
func (s Snapshot) cautionDue(now time.Time) bool {
	cautionAt, _ := s.deadlines()
	return !cautionAt.IsZero() && !now.Before(cautionAt)
}

// withheld reports whether the forecast is too old to show at now.
func (s Snapshot) withheld(now time.Time) bool {
	_, withheldAt := s.deadlines()
	return !withheldAt.IsZero() && !now.Before(withheldAt)
}

// snapshotPayload is what is sealed in a snapshot row. A row written before
// skipped doses existed holds a bare array of windows, which ReadSnapshot
// still reads.
type snapshotPayload struct {
	Windows      []Window        `json:"windows"`
	DosesShared  bool            `json:"dosesShared,omitempty"`
	SkippedDoses []SkippedDose   `json:"skippedDoses,omitempty"`
	Reported     *ReportedStatus `json:"reported,omitempty"`
	CautionAt    time.Time       `json:"cautionAt,omitzero"`
	WithheldAt   time.Time       `json:"withheldAt,omitzero"`
}

type Store struct {
	db      *sql.DB
	aead    cipher.AEAD
	csrfKey []byte

	// live tells open event streams that a link changed. One store is
	// shared by the publisher and the public handler, so a publish reaches
	// every open page of that link.
	live *liveHub
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
	store := &Store{db: db, aead: aead, csrfKey: csrfKey[:], live: newLiveHub()}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// CloseStreams ends every open event stream. The daemon calls it as it begins
// a graceful shutdown; pages reconnect to whatever serves them next.
func (s *Store) CloseStreams() {
	s.live.close()
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
		// Request threads (P5-c). Bodies are sealed like request text; the
		// author is in the clear because limits count by it, and it says
		// nothing a thread's two readers do not already know.
		`CREATE TABLE IF NOT EXISTS portal_messages (
			message_id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL REFERENCES portal_requests(request_id) ON DELETE CASCADE,
			profile_id TEXT NOT NULL,
			author TEXT NOT NULL CHECK (author IN ('visitor', 'owner')),
			created_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_portal_messages_request
			ON portal_messages(request_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_portal_messages_profile
			ON portal_messages(profile_id, author, created_at)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("portal migrate: %w", err)
		}
	}
	// Columns added after a table was first made are added here, for a new
	// database as for an old one, so both end with the same table. A link
	// from before ADR-0058 had no skipped-doses grant, which the default says.
	if err := s.ensureColumn(ctx, "portal_profiles", "grant_reported_status", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "portal_profiles", "grant_skipped_doses", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("portal migrate: %w", err)
	}
	return s.ensureAuditKey(ctx)
}

// ensureColumn adds a column to a table that predates it. SQLite has no ADD
// COLUMN IF NOT EXISTS, so the table is asked first.
func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
	return err
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
	case !ValidStatusGrant(input.Grants.ReportedStatus):
		return errors.New("portal link reported status must be empty, contact, or full")
	}

	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	hash := argon2.IDKey([]byte(input.Passcode), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	tokenHash := hashToken(input.Token)

	_, err := s.db.ExecContext(ctx, `INSERT INTO portal_profiles
		(profile_id, token_hash, grant_windows, grant_requests, grant_messages, grant_skipped_doses,
		 grant_reported_status, created_at, expires_at, revoked_at,
		 passcode_hash, passcode_salt, passcode_time, passcode_memory, passcode_threads)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?)`,
		input.ProfileID, tokenHash[:],
		boolToInt(input.Grants.WakingWindows), boolToInt(input.Grants.AllowRequests), boolToInt(input.Grants.AllowMessages),
		boolToInt(input.Grants.SkippedDoses), input.Grants.ReportedStatus,
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
	row := s.db.QueryRowContext(ctx, `SELECT `+profileColumns+`
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
	row := s.db.QueryRowContext(ctx, `SELECT `+profileColumns+`
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

// profileColumns are the columns every profile read selects, in the order
// scanProfile reads them.
const profileColumns = `profile_id, grant_windows, grant_requests, grant_messages, grant_skipped_doses, grant_reported_status,
		created_at, expires_at, revoked_at`

type rowScanner interface {
	Scan(dest ...any) error
}

// scanProfile reads one row of profileColumns, and says whether the link has
// expired at now or been revoked.
func scanProfile(row rowScanner, now time.Time) (Profile, bool, bool, error) {
	var (
		profile                                   Profile
		windows, requests, messages, skippedDoses int
		createdAtRaw, expiresAtRaw, revoked       string
	)
	var reportedStatus string
	if err := row.Scan(&profile.ID, &windows, &requests, &messages, &skippedDoses, &reportedStatus,
		&createdAtRaw, &expiresAtRaw, &revoked); err != nil {
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
		SkippedDoses:  skippedDoses == 1,
	}
	// A level this build does not know shows nothing.
	if ValidStatusGrant(reportedStatus) {
		profile.Grants.ReportedStatus = reportedStatus
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
	rows, err := s.db.QueryContext(ctx, `SELECT `+profileColumns+`
		FROM portal_profiles ORDER BY created_at, profile_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var states []ProfileState
	for rows.Next() {
		profile, expired, revoked, err := scanProfile(rows, now)
		if err != nil {
			return nil, err
		}
		states = append(states, ProfileState{Profile: profile, Expired: expired, Revoked: revoked})
	}
	return states, rows.Err()
}

// RevokeProfile is the kill switch. It drops sessions immediately so that an
// already-authenticated visitor cannot keep reading through an open session,
// and it deletes the materialized snapshot so no availability data survives in
// the portal database.
// DeleteProfile removes the profile row itself, and with it every dependent
// row: the snapshot, sessions, requests and their sessions all declare
// ON DELETE CASCADE on this key, and foreign keys are enforced.
//
// It exists because the owner-side erase route revoked the link, cleared its
// audit and dropped its private label but left the profile row behind, so a
// link the owner had "erased" was still listed — revoked and nameless. The
// handler's own comment said it erased the record that the link existed; this
// is what makes that true.
func (s *Store) DeleteProfile(ctx context.Context, profileID string) error {
	if profileID == "" {
		return errors.New("portal profile id is required")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM portal_profiles WHERE profile_id = ?`, profileID); err != nil {
		return err
	}
	s.live.notify(profileID)
	return nil
}

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
	if err := tx.Commit(); err != nil {
		return err
	}
	// Revocation is immediate at the link; an open page learns it now too.
	s.live.notify(profileID)
	return nil
}

// PublishSnapshot replaces the materialized availability for one profile. It
// is the only way availability data enters the portal database.
func (s *Store) PublishSnapshot(ctx context.Context, profileID string, snapshot Snapshot) error {
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	payload, err := json.Marshal(snapshotPayload{
		Windows: snapshot.Windows, DosesShared: snapshot.DosesShared, SkippedDoses: snapshot.SkippedDoses,
		Reported: snapshot.Reported, CautionAt: snapshot.CautionAt, WithheldAt: snapshot.WithheldAt,
	})
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
	s.live.notify(profileID)
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
	payload, err := decodeSnapshotPayload(plaintext)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.GeneratedAt = generatedAt
	snapshot.HorizonEnd = horizonEnd
	snapshot.Windows = payload.Windows
	snapshot.DosesShared = payload.DosesShared
	snapshot.SkippedDoses = payload.SkippedDoses
	snapshot.Reported = payload.Reported
	snapshot.CautionAt = payload.CautionAt
	snapshot.WithheldAt = payload.WithheldAt
	return snapshot, nil
}

// decodeSnapshotPayload reads a sealed snapshot, including one written as a
// bare array of windows before skipped doses existed.
func decodeSnapshotPayload(plaintext []byte) (snapshotPayload, error) {
	var payload snapshotPayload
	if trimmed := bytes.TrimSpace(plaintext); len(trimmed) > 0 && trimmed[0] == '[' {
		err := json.Unmarshal(trimmed, &payload.Windows)
		return payload, err
	}
	err := json.Unmarshal(plaintext, &payload)
	return payload, err
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
	if snapshot.WithheldAt.IsZero() != snapshot.CautionAt.IsZero() || snapshot.WithheldAt.Before(snapshot.CautionAt) {
		return errors.New("portal snapshot deadlines must both be set, the caution first")
	}
	for _, window := range snapshot.Windows {
		if !window.EndAt.After(window.StartAt) {
			return errors.New("portal snapshot windows must be non-empty half-open intervals")
		}
		if window.ZoneID == "" {
			return errors.New("portal snapshot windows require a zone id")
		}
	}
	if len(snapshot.SkippedDoses) > 0 && !snapshot.DosesShared {
		return errors.New("portal snapshot carries doses for a link that does not share them")
	}
	if len(snapshot.SkippedDoses) > maxSkippedDoses {
		return fmt.Errorf("portal snapshot carries more than %d skipped doses", maxSkippedDoses)
	}
	for _, dose := range snapshot.SkippedDoses {
		if dose.At.IsZero() || dose.ZoneID == "" {
			return errors.New("portal snapshot skipped doses require a time and a zone id")
		}
	}
	if reported := snapshot.Reported; reported != nil {
		switch reported.Status {
		case presenceAwake, presenceTryingToSleep, presenceDoNotDisturb:
			if reported.ReportedAt.IsZero() {
				return errors.New("portal snapshot reported status requires when it was said")
			}
		case StatusUnavailable:
			if !reported.ReportedAt.IsZero() {
				return errors.New("portal snapshot contact-only status says when it was said")
			}
		default:
			return fmt.Errorf("unsupported portal snapshot reported status %q", reported.Status)
		}
		if reported.ExpiresAt.IsZero() {
			return errors.New("portal snapshot reported status requires an expiry")
		}
	}
	return nil
}

// The statuses a full link may be told, as core/presence names them. They are
// repeated here because the portal package imports nothing of the private
// side, and a test holds them equal.
const (
	presenceAwake         = "awake"
	presenceTryingToSleep = "trying_to_sleep"
	presenceDoNotDisturb  = "do_not_disturb"
)

// UpdateGrants changes what an existing link shows: its waking windows, the
// doses logged as skipped, and how much of a reported status. Whether a link
// takes requests and messages is fixed when it is made, because requests and
// threads already exist under it.
func (s *Store) UpdateGrants(ctx context.Context, profileID string, grants Grants) error {
	if !ValidStatusGrant(grants.ReportedStatus) {
		return errors.New("portal link reported status must be empty, contact, or full")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE portal_profiles
		SET grant_windows = ?, grant_skipped_doses = ?, grant_reported_status = ?
		WHERE profile_id = ?`,
		boolToInt(grants.WakingWindows), boolToInt(grants.SkippedDoses), grants.ReportedStatus, profileID)
	if err != nil {
		return fmt.Errorf("update portal grants: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return ErrProfileNotFound
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
	// Key identifies the session without being its secret: the stored hash,
	// hex-encoded. It bounds streams per session and re-checks an open one.
	Key string
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
	session.Key = hex.EncodeToString(sessionHash[:])
	return session, nil
}

// ResolveSessionByKey re-reads a session an open stream belongs to, so that
// revocation, expiry or a new passcode ends the stream rather than leaving it
// running on a session that no longer exists.
func (s *Store) ResolveSessionByKey(ctx context.Context, key string, now time.Time) (Session, error) {
	sessionHash, err := hex.DecodeString(key)
	if err != nil || len(sessionHash) != sha256.Size {
		return Session{}, ErrSessionInvalid
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT profile_id, expires_at FROM portal_sessions WHERE session_hash = ?`, sessionHash)
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
	session.Key = key
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
	return s.purgeThreads(ctx, now)
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
