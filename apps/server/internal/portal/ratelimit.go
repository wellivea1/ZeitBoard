package portal

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

// AccessEvent is a closed set. RecordAccess rejects anything else, which is
// what keeps visitor-supplied text, paths, and link tokens out of the audit
// table by construction rather than by reviewer vigilance.
type AccessEvent string

const (
	EventPageView         AccessEvent = "page_view"
	EventAvailabilityRead AccessEvent = "availability_read"
	EventPasscodeAccepted AccessEvent = "passcode_accepted"
	EventPasscodeRejected AccessEvent = "passcode_rejected"
	EventThrottled        AccessEvent = "throttled"
	EventLinkRejected     AccessEvent = "link_rejected"
)

func (event AccessEvent) valid() bool {
	switch event {
	case EventPageView, EventAvailabilityRead, EventPasscodeAccepted,
		EventPasscodeRejected, EventThrottled, EventLinkRejected:
		return true
	default:
		return false
	}
}

// Persisted limits from docs/portal-design.md section 8.
const (
	ReadLimitPerHour   = 120
	ReadLimitWindow    = time.Hour
	maxPasscodeBackoff = 15 * time.Minute

	// unknownProfile is the audit subject for a request whose link never
	// resolved. There is no profile to attribute it to, and inventing one
	// would imply the attacker guessed a real link.
	unknownProfile = "-"
)

func (s *Store) ensureAuditKey(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM portal_audit_keys`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO portal_audit_keys (key, created_at) VALUES (?, ?)`,
		key, formatTime(time.Now()))
	return err
}

func (s *Store) currentAuditKey(ctx context.Context) ([]byte, error) {
	var key []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT key FROM portal_audit_keys ORDER BY id DESC LIMIT 1`).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("portal audit key is missing")
	}
	return key, err
}

// SourceID converts a remote address into a pseudonymous identifier. The raw
// address is never stored. This is not anonymity: NAT groups distinct people
// under one identifier and a changing network splits one person across
// several. docs/privacy.md states that limitation.
func (s *Store) SourceID(ctx context.Context, remoteAddr string) (string, error) {
	key, err := s.currentAuditKey(ctx)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(normalizeRemoteAddr(remoteAddr)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16]), nil
}

// RotateAuditKey makes every previously issued source identifier unlinkable to
// future ones, and drops audit rows older than retain so the old key's outputs
// do not linger. Rotation without that deletion would be a rotation in name
// only.
func (s *Store) RotateAuditKey(ctx context.Context, now time.Time, retain time.Duration) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO portal_audit_keys (key, created_at) VALUES (?, ?)`, key, formatTime(now)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM portal_access_audit WHERE occurred_at <= ?`, formatTime(now.Add(-retain))); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM portal_audit_keys WHERE id NOT IN (SELECT id FROM portal_audit_keys ORDER BY id DESC LIMIT 1)`); err != nil {
		return err
	}
	return tx.Commit()
}

// normalizeRemoteAddr strips the port and collapses an IPv6 address to its /64
// prefix, because a single host is routinely given a whole /64 and would
// otherwise defeat per-source limits by picking a new address per request.
func normalizeRemoteAddr(remoteAddr string) string {
	host := strings.TrimSpace(remoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	addr = addr.Unmap()
	if addr.Is6() {
		if prefix, err := addr.Prefix(64); err == nil {
			return prefix.String()
		}
	}
	return addr.String()
}

// Allow applies a persisted fixed-window counter. It returns the wait before
// the caller may retry when the window is exhausted.
func (s *Store) Allow(ctx context.Context, bucketKey string, limit int, window time.Duration, now time.Time) (bool, time.Duration, error) {
	if limit <= 0 || window <= 0 {
		return false, 0, errors.New("portal rate limit requires a positive limit and window")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		windowStartRaw string
		count          int
	)
	err = tx.QueryRowContext(ctx,
		`SELECT window_start, count FROM portal_rate_buckets WHERE bucket_key = ?`, bucketKey).
		Scan(&windowStartRaw, &count)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO portal_rate_buckets (bucket_key, window_start, count) VALUES (?, ?, 1)`,
			bucketKey, formatTime(now)); err != nil {
			return false, 0, err
		}
		return true, 0, tx.Commit()
	case err != nil:
		return false, 0, err
	}

	windowStart, err := parseTime(windowStartRaw)
	if err != nil {
		return false, 0, err
	}
	if !now.Before(windowStart.Add(window)) {
		if _, err := tx.ExecContext(ctx,
			`UPDATE portal_rate_buckets SET window_start = ?, count = 1 WHERE bucket_key = ?`,
			formatTime(now), bucketKey); err != nil {
			return false, 0, err
		}
		return true, 0, tx.Commit()
	}
	if count >= limit {
		retryAfter := windowStart.Add(window).Sub(now)
		return false, retryAfter, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE portal_rate_buckets SET count = count + 1 WHERE bucket_key = ?`, bucketKey); err != nil {
		return false, 0, err
	}
	return true, 0, tx.Commit()
}

// PasscodeDelay reports how long the caller must wait before another passcode
// attempt is accepted. The backoff is per profile-and-source: there is
// deliberately no global lockout, because an attacker could otherwise use it
// to deny access to every legitimate visitor (design section 4).
func (s *Store) PasscodeDelay(ctx context.Context, bucketKey string, now time.Time) (time.Duration, error) {
	var blockedUntilRaw string
	err := s.db.QueryRowContext(ctx,
		`SELECT blocked_until FROM portal_rate_buckets WHERE bucket_key = ?`, bucketKey).Scan(&blockedUntilRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	blockedUntil, err := parseTime(blockedUntilRaw)
	if err != nil {
		return 0, err
	}
	if blockedUntil.After(now) {
		return blockedUntil.Sub(now), nil
	}
	return 0, nil
}

// NotePasscodeFailure records one rejected attempt and returns the new wait.
func (s *Store) NotePasscodeFailure(ctx context.Context, bucketKey string, now time.Time) (time.Duration, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var failures int
	err = tx.QueryRowContext(ctx,
		`SELECT failures FROM portal_rate_buckets WHERE bucket_key = ?`, bucketKey).Scan(&failures)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	failures++
	delay := passcodeBackoff(failures)
	blockedUntil := formatTime(now.Add(delay))

	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO portal_rate_buckets (bucket_key, window_start, count, failures, blocked_until)
			 VALUES (?, ?, 0, ?, ?)`,
			bucketKey, formatTime(now), failures, blockedUntil); err != nil {
			return 0, err
		}
		return delay, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE portal_rate_buckets SET failures = ?, blocked_until = ? WHERE bucket_key = ?`,
		failures, blockedUntil, bucketKey); err != nil {
		return 0, err
	}
	return delay, tx.Commit()
}

func (s *Store) ClearPasscodeFailures(ctx context.Context, bucketKey string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE portal_rate_buckets SET failures = 0, blocked_until = '' WHERE bucket_key = ?`, bucketKey)
	return err
}

func passcodeBackoff(failures int) time.Duration {
	if failures <= 1 {
		return time.Second
	}
	if failures > 20 {
		return maxPasscodeBackoff
	}
	delay := time.Second << uint(failures-1)
	if delay > maxPasscodeBackoff || delay <= 0 {
		return maxPasscodeBackoff
	}
	return delay
}

// RecordAccess appends one coarse audit row. It stores an event from the
// closed enum, a pseudonymous source, and a time. There is no field for a URL,
// a user agent, or any visitor text.
func (s *Store) RecordAccess(ctx context.Context, profileID string, event AccessEvent, sourceID string, now time.Time) error {
	if !event.valid() {
		return fmt.Errorf("unsupported portal access event %q", string(event))
	}
	if profileID == "" {
		profileID = unknownProfile
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO portal_access_audit (profile_id, event, source_hmac, occurred_at) VALUES (?, ?, ?, ?)`,
		profileID, string(event), sourceID, formatTime(now))
	return err
}

// AccessSummary is the coarse view the owner's Sharing screen renders. It
// counts events rather than listing them so the owner cannot reconstruct a
// visitor's browsing sequence.
type AccessSummary struct {
	ProfileID  string
	Event      AccessEvent
	Count      int
	LastAccess time.Time
}

func (s *Store) SummarizeAccess(ctx context.Context, profileID string, since time.Time) ([]AccessSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT event, COUNT(1), MAX(occurred_at) FROM portal_access_audit
		 WHERE profile_id = ? AND occurred_at >= ?
		 GROUP BY event ORDER BY event`, profileID, formatTime(since))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var summaries []AccessSummary
	for rows.Next() {
		var (
			event   string
			count   int
			lastRaw string
		)
		if err := rows.Scan(&event, &count, &lastRaw); err != nil {
			return nil, err
		}
		last, err := parseTime(lastRaw)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, AccessSummary{
			ProfileID:  profileID,
			Event:      AccessEvent(event),
			Count:      count,
			LastAccess: last,
		})
	}
	return summaries, rows.Err()
}

// EraseAudit drops a profile's audit rows. It exists so audit data is subject
// to the same erasure right as everything else (ADR-0014/0017).
func (s *Store) EraseAudit(ctx context.Context, profileID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM portal_access_audit WHERE profile_id = ?`, profileID)
	return err
}
