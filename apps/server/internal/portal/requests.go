package portal

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Request lifecycle. `queued` is an honest state, not a transient one: it
// means the portal has durably stored the request but the bridge has not yet
// confirmed the owner's queue received it. Showing "sent" before that
// confirmation would be a lie the visitor cannot check.
const (
	RequestQueued   = "queued"
	RequestPending  = "pending"
	RequestApproved = "approved"
	RequestDeclined = "declined"
	RequestClosed   = "closed"
)

// Limits from docs/portal-design.md section 6 and 8.
const (
	MaxRequestWindow      = 8 * time.Hour
	MinRequestDuration    = 15
	MaxRequestDuration    = 480
	MaxHandleRunes        = 40
	MaxMessageRunes       = 500
	RequestsPerSessionDay = 5
	MaxPendingPerProfile  = 20

	// RequestSessionLifetime bounds a requester's cookie. It is longer than a
	// portal session because a visitor should be able to come back and read a
	// decision without re-entering anything.
	RequestSessionLifetime = 14 * 24 * time.Hour
)

var (
	ErrRequestInvalid    = errors.New("portal request is invalid")
	ErrRequestNotFound   = errors.New("portal request not found")
	ErrRequestLimit      = errors.New("portal request limit reached")
	ErrRequestNotPending = errors.New("portal request is no longer open")
)

// RequestInput is the validated visitor submission.
type RequestInput struct {
	WindowStart     time.Time
	WindowEnd       time.Time
	ZoneID          string
	DurationMinutes int
	Handle          string
	Message         string
}

// Request is a stored visitor request. Handle and Message are private visitor
// text: they are encrypted at rest, never enter a projection DTO, and never
// reach an access-audit row or a notification title.
type Request struct {
	ID              string
	ProfileID       string
	Status          string
	WindowStart     time.Time
	WindowEnd       time.Time
	ZoneID          string
	DurationMinutes int
	BeyondHorizon   bool
	Handle          string
	Message         string
	DecidedStart    time.Time
	DecidedEnd      time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type requestSecrets struct {
	Handle  string `json:"handle"`
	Message string `json:"message"`
}

// ValidateRequest normalizes and bounds a visitor submission. There is
// deliberately no upper date horizon (owner decision 3); a request past the
// forecast horizon is accepted and flagged so the UI can warn rather than
// refuse. What is bounded is the window's *length*, because an eight-hour ask
// is a scheduling request and a three-week ask is not.
func ValidateRequest(input RequestInput, now time.Time, horizonEnd time.Time) (RequestInput, bool, error) {
	if input.WindowStart.IsZero() || input.WindowEnd.IsZero() {
		return RequestInput{}, false, fmt.Errorf("%w: a start and end are required", ErrRequestInvalid)
	}
	input.WindowStart = input.WindowStart.UTC()
	input.WindowEnd = input.WindowEnd.UTC()

	if !input.WindowEnd.After(input.WindowStart) {
		return RequestInput{}, false, fmt.Errorf("%w: the end must be after the start", ErrRequestInvalid)
	}
	if input.WindowStart.Before(now) {
		return RequestInput{}, false, fmt.Errorf("%w: the window has already started", ErrRequestInvalid)
	}
	window := input.WindowEnd.Sub(input.WindowStart)
	if window > MaxRequestWindow {
		return RequestInput{}, false, fmt.Errorf("%w: the window may not exceed %d hours",
			ErrRequestInvalid, int(MaxRequestWindow.Hours()))
	}

	if input.DurationMinutes != 0 {
		if input.DurationMinutes < MinRequestDuration || input.DurationMinutes > MaxRequestDuration {
			return RequestInput{}, false, fmt.Errorf("%w: length must be between %d and %d minutes",
				ErrRequestInvalid, MinRequestDuration, MaxRequestDuration)
		}
		if time.Duration(input.DurationMinutes)*time.Minute > window {
			return RequestInput{}, false, fmt.Errorf("%w: length must fit inside the window", ErrRequestInvalid)
		}
	}

	if input.ZoneID == "" {
		input.ZoneID = "UTC"
	}
	if _, err := time.LoadLocation(input.ZoneID); err != nil {
		return RequestInput{}, false, fmt.Errorf("%w: unknown time zone", ErrRequestInvalid)
	}

	input.Handle = sanitizeVisitorText(input.Handle)
	input.Message = sanitizeVisitorText(input.Message)
	if utf8.RuneCountInString(input.Handle) > MaxHandleRunes {
		return RequestInput{}, false, fmt.Errorf("%w: name must be at most %d characters", ErrRequestInvalid, MaxHandleRunes)
	}
	if utf8.RuneCountInString(input.Message) > MaxMessageRunes {
		return RequestInput{}, false, fmt.Errorf("%w: message must be at most %d characters", ErrRequestInvalid, MaxMessageRunes)
	}

	beyondHorizon := horizonEnd.IsZero() || input.WindowStart.After(horizonEnd)
	return input, beyondHorizon, nil
}

// sanitizeVisitorText strips control characters. It does not escape anything:
// escaping belongs to the renderer, and pre-escaping here would double-encode
// and corrupt the owner's copy.
func sanitizeVisitorText(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case r == '\n' || r == '\t':
			builder.WriteRune(' ')
		case r < 0x20 || r == 0x7f:
			// drop
		default:
			builder.WriteRune(r)
		}
	}
	return strings.TrimSpace(builder.String())
}

// CreatedRequest carries the values shown to the visitor exactly once.
type CreatedRequest struct {
	Request Request

	// Secret is the requester's 256-bit proof of authorship. It is placed in
	// a URL fragment, never a path or query, so it cannot reach a server log
	// or a proxy access log.
	Secret string
}

// CreateRequest stores a validated request and its outbox row in one
// transaction. The outbox is what makes the cross-database handoff safe:
// nothing is lost if the bridge is down, and a retry cannot duplicate the
// owner's proposal.
func (s *Store) CreateRequest(ctx context.Context, profile Profile, sessionValue string, input RequestInput, beyondHorizon bool, now time.Time) (CreatedRequest, error) {
	if !profile.Grants.AllowRequests {
		return CreatedRequest{}, fmt.Errorf("%w: this link does not accept requests", ErrRequestInvalid)
	}
	requestID, err := randomToken()
	if err != nil {
		return CreatedRequest{}, err
	}
	secret, err := randomToken()
	if err != nil {
		return CreatedRequest{}, err
	}
	payload, err := json.Marshal(requestSecrets{Handle: input.Handle, Message: input.Message})
	if err != nil {
		return CreatedRequest{}, err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return CreatedRequest{}, err
	}
	ciphertext := s.aead.Seal(nil, nonce, payload, requestAAD(requestID, profile.ID))
	sessionHash := hashToken(sessionValue)
	secretHash := hashToken(secret)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CreatedRequest{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var pending int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM portal_requests WHERE profile_id = ? AND status IN (?, ?)`,
		profile.ID, RequestQueued, RequestPending).Scan(&pending); err != nil {
		return CreatedRequest{}, err
	}
	if pending >= MaxPendingPerProfile {
		return CreatedRequest{}, fmt.Errorf("%w: too many requests are already waiting", ErrRequestLimit)
	}

	var daily int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM portal_requests WHERE session_hash = ? AND created_at >= ?`,
		sessionHash[:], formatTime(now.Add(-24*time.Hour))).Scan(&daily); err != nil {
		return CreatedRequest{}, err
	}
	if daily >= RequestsPerSessionDay {
		return CreatedRequest{}, fmt.Errorf("%w: you have reached today's request limit", ErrRequestLimit)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO portal_requests
		(request_id, profile_id, session_hash, secret_hash, window_start, window_end, zone_id,
		 duration_minutes, beyond_horizon, status, created_at, updated_at, nonce, ciphertext)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		requestID, profile.ID, sessionHash[:], secretHash[:],
		formatTime(input.WindowStart), formatTime(input.WindowEnd), input.ZoneID,
		input.DurationMinutes, boolToInt(beyondHorizon), RequestQueued,
		formatTime(now), formatTime(now), nonce, ciphertext); err != nil {
		return CreatedRequest{}, fmt.Errorf("store portal request: %w", err)
	}
	// The idempotency key is the request id: the bridge may retry forever and
	// still create exactly one proposal.
	if _, err := tx.ExecContext(ctx, `INSERT INTO portal_outbox
		(kind, request_id, idempotency_key, created_at) VALUES (?, ?, ?, ?)`,
		outboxProposalSubmit, requestID, requestID, formatTime(now)); err != nil {
		return CreatedRequest{}, fmt.Errorf("store portal outbox row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CreatedRequest{}, err
	}

	return CreatedRequest{
		Secret: secret,
		Request: Request{
			ID:              requestID,
			ProfileID:       profile.ID,
			Status:          RequestQueued,
			WindowStart:     input.WindowStart,
			WindowEnd:       input.WindowEnd,
			ZoneID:          input.ZoneID,
			DurationMinutes: input.DurationMinutes,
			BeyondHorizon:   beyondHorizon,
			Handle:          input.Handle,
			Message:         input.Message,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
	}, nil
}

const outboxProposalSubmit = "proposal_submit"

// ReadRequest returns a request scoped to one profile. Callers must already
// have proven the caller may see it; see AuthorizeRequestSecret.
func (s *Store) ReadRequest(ctx context.Context, profileID, requestID string) (Request, error) {
	row := s.db.QueryRowContext(ctx, `SELECT request_id, profile_id, status,
		window_start, window_end, zone_id, duration_minutes, beyond_horizon,
		decided_start, decided_end, created_at, updated_at, nonce, ciphertext
		FROM portal_requests WHERE request_id = ? AND profile_id = ?`, requestID, profileID)
	return s.scanRequest(row)
}

func (s *Store) scanRequest(row *sql.Row) (Request, error) {
	var (
		request           Request
		beyondHorizon     int
		windowStart       string
		windowEnd         string
		decidedStart      string
		decidedEnd        string
		createdAt         string
		updatedAt         string
		nonce, ciphertext []byte
	)
	if err := row.Scan(&request.ID, &request.ProfileID, &request.Status,
		&windowStart, &windowEnd, &request.ZoneID, &request.DurationMinutes, &beyondHorizon,
		&decidedStart, &decidedEnd, &createdAt, &updatedAt, &nonce, &ciphertext); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Request{}, ErrRequestNotFound
		}
		return Request{}, err
	}
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, requestAAD(request.ID, request.ProfileID))
	if err != nil {
		return Request{}, fmt.Errorf("decrypt portal request: %w", err)
	}
	var secrets requestSecrets
	if err := json.Unmarshal(plaintext, &secrets); err != nil {
		return Request{}, err
	}
	for target, raw := range map[*time.Time]string{
		&request.WindowStart:  windowStart,
		&request.WindowEnd:    windowEnd,
		&request.DecidedStart: decidedStart,
		&request.DecidedEnd:   decidedEnd,
		&request.CreatedAt:    createdAt,
		&request.UpdatedAt:    updatedAt,
	} {
		parsed, err := parseTime(raw)
		if err != nil {
			return Request{}, err
		}
		*target = parsed
	}
	request.BeyondHorizon = beyondHorizon == 1
	request.Handle = secrets.Handle
	request.Message = secrets.Message
	return request, nil
}

// AuthorizeRequestSecret exchanges the one-time requester secret for a
// request-scoped session. Knowing the shared link and passcode is not enough
// to read someone else's request, so this is what separates two visitors who
// hold the same link.
func (s *Store) AuthorizeRequestSecret(ctx context.Context, profileID, requestID, secret string, now time.Time) (SessionToken, error) {
	var storedHash []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT secret_hash FROM portal_requests WHERE request_id = ? AND profile_id = ?`,
		requestID, profileID).Scan(&storedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionToken{}, ErrRequestNotFound
	}
	if err != nil {
		return SessionToken{}, err
	}
	presented := hashToken(secret)
	if !constantTimeEqual(presented[:], storedHash) {
		return SessionToken{}, ErrRequestNotFound
	}

	sessionValue, err := randomToken()
	if err != nil {
		return SessionToken{}, err
	}
	sessionHash := hashToken(sessionValue)
	expiresAt := now.Add(RequestSessionLifetime)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO portal_request_sessions (session_hash, request_id, profile_id, expires_at)
		 VALUES (?, ?, ?, ?)`,
		sessionHash[:], requestID, profileID, formatTime(expiresAt)); err != nil {
		return SessionToken{}, err
	}
	return SessionToken{Session: sessionValue, ExpiresAt: expiresAt}, nil
}

// AuthorizesRequest reports whether a request cookie proves authorship of one
// specific request under one specific profile. All three must agree, so a
// cookie for another visitor's request — or for the same request under a
// different link — grants nothing.
func (s *Store) AuthorizesRequest(ctx context.Context, sessionValue, profileID, requestID string, now time.Time) (bool, error) {
	if sessionValue == "" || profileID == "" || requestID == "" {
		return false, nil
	}
	sessionHash := hashToken(sessionValue)
	var (
		storedRequestID string
		storedProfileID string
		expiresAtRaw    string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT request_id, profile_id, expires_at FROM portal_request_sessions WHERE session_hash = ?`,
		sessionHash[:]).Scan(&storedRequestID, &storedProfileID, &expiresAtRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if storedRequestID != requestID || storedProfileID != profileID {
		return false, nil
	}
	expiresAt, err := parseTime(expiresAtRaw)
	if err != nil {
		return false, err
	}
	return expiresAt.After(now), nil
}

func requestAAD(requestID, profileID string) []byte {
	return []byte(strings.Join([]string{"portal-request", requestID, profileID}, "\x00"))
}
