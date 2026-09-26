package portal

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Request threads (portal-design section 7, slice P5-c). A visitor who asked
// for a time and the owner can exchange short plain-text messages about that
// one request. A thread belongs to its request: only the request's author and
// the owner can read it, it closes when the request is decided, and its bodies
// are deleted fourteen days after that, or sooner if the owner erases them.
const (
	AuthorVisitor = "visitor"
	AuthorOwner   = "owner"

	MessagesPerThreadDay  = 20
	MessagesPerProfileDay = 100

	// ThreadRetention is how long a decided request's messages stay readable
	// so both sides can read the finished exchange.
	ThreadRetention = 14 * 24 * time.Hour
)

var (
	ErrMessageInvalid = errors.New("portal message is invalid")
	ErrMessageLimit   = errors.New("portal message limit reached")
	ErrThreadClosed   = errors.New("portal thread is closed")
)

// Message is one entry in a request's thread. Body is private text: encrypted
// at rest, never in a projection, an audit row or a notification title.
type Message struct {
	ID        string
	RequestID string
	Author    string
	Body      string
	CreatedAt time.Time
}

// ValidateMessage bounds and cleans a message body. Line breaks are kept,
// since a message is prose; every other control character is dropped, and
// escaping is left to the renderer.
func ValidateMessage(body string) (string, error) {
	var builder strings.Builder
	builder.Grow(len(body))
	for _, r := range strings.ReplaceAll(body, "\r\n", "\n") {
		switch {
		case r == '\n':
			builder.WriteRune(r)
		case r == '\t':
			builder.WriteRune(' ')
		case r < 0x20 || r == 0x7f:
		default:
			builder.WriteRune(r)
		}
	}
	cleaned := strings.TrimSpace(builder.String())
	for strings.Contains(cleaned, "\n\n\n") {
		cleaned = strings.ReplaceAll(cleaned, "\n\n\n", "\n\n")
	}
	if cleaned == "" {
		return "", fmt.Errorf("%w: write a message first", ErrMessageInvalid)
	}
	if utf8.RuneCountInString(cleaned) > MaxMessageRunes {
		return "", fmt.Errorf("%w: a message may be at most %d characters", ErrMessageInvalid, MaxMessageRunes)
	}
	return cleaned, nil
}

// AppendMessage adds a message to an open request's thread. The visitor's
// limits bound what one thread or one link can send in a day; the owner's
// replies are not limited.
func (s *Store) AppendMessage(ctx context.Context, profile Profile, requestID, author, body string, now time.Time) (Message, error) {
	if author != AuthorVisitor && author != AuthorOwner {
		return Message{}, fmt.Errorf("%w: unknown author", ErrMessageInvalid)
	}
	if !profile.Grants.AllowMessages {
		return Message{}, fmt.Errorf("%w: this link does not carry messages", ErrThreadClosed)
	}
	cleaned, err := ValidateMessage(body)
	if err != nil {
		return Message{}, err
	}
	messageID, err := randomToken()
	if err != nil {
		return Message{}, err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Message{}, err
	}
	ciphertext := s.aead.Seal(nil, nonce, []byte(cleaned), messageAAD(messageID, requestID, profile.ID, author))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM portal_requests WHERE request_id = ? AND profile_id = ?`,
		requestID, profile.ID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrRequestNotFound
	}
	if err != nil {
		return Message{}, err
	}
	if status != RequestQueued && status != RequestPending {
		return Message{}, fmt.Errorf("%w: the request has been answered", ErrThreadClosed)
	}

	if author == AuthorVisitor {
		since := formatTime(now.Add(-24 * time.Hour))
		var inThread, inProfile int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM portal_messages WHERE request_id = ? AND author = ? AND created_at >= ?`,
			requestID, AuthorVisitor, since).Scan(&inThread); err != nil {
			return Message{}, err
		}
		if inThread >= MessagesPerThreadDay {
			return Message{}, fmt.Errorf("%w: you have sent today's messages for this request", ErrMessageLimit)
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM portal_messages WHERE profile_id = ? AND author = ? AND created_at >= ?`,
			profile.ID, AuthorVisitor, since).Scan(&inProfile); err != nil {
			return Message{}, err
		}
		if inProfile >= MessagesPerProfileDay {
			return Message{}, fmt.Errorf("%w: this link has carried today's messages", ErrMessageLimit)
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO portal_messages
		(message_id, request_id, profile_id, author, created_at, nonce, ciphertext)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		messageID, requestID, profile.ID, author, formatTime(now), nonce, ciphertext); err != nil {
		return Message{}, fmt.Errorf("store portal message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	return Message{ID: messageID, RequestID: requestID, Author: author, Body: cleaned, CreatedAt: now}, nil
}

// ListMessages returns a request's thread, oldest first. Callers must already
// have proven the reader is the request's author or the owner.
func (s *Store) ListMessages(ctx context.Context, profileID, requestID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT message_id, author, created_at, nonce, ciphertext
		FROM portal_messages WHERE request_id = ? AND profile_id = ?
		ORDER BY created_at, message_id`, requestID, profileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	messages := []Message{}
	for rows.Next() {
		var (
			message           Message
			createdAt         string
			nonce, ciphertext []byte
		)
		if err := rows.Scan(&message.ID, &message.Author, &createdAt, &nonce, &ciphertext); err != nil {
			return nil, err
		}
		plaintext, err := s.aead.Open(nil, nonce, ciphertext, messageAAD(message.ID, requestID, profileID, message.Author))
		if err != nil {
			return nil, fmt.Errorf("decrypt portal message: %w", err)
		}
		if message.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		message.RequestID = requestID
		message.Body = string(plaintext)
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

// EraseThread deletes a request's messages now, which the owner may do at any
// time rather than wait for the retention window.
func (s *Store) EraseThread(ctx context.Context, profileID, requestID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM portal_messages WHERE request_id = ? AND profile_id = ?`, requestID, profileID)
	return err
}

// purgeThreads deletes the messages of requests answered or closed more than
// ThreadRetention ago. Retention that nothing enforces is not retention.
func (s *Store) purgeThreads(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM portal_messages WHERE request_id IN (
		SELECT request_id FROM portal_requests
		WHERE status NOT IN (?, ?) AND updated_at <= ?)`,
		RequestQueued, RequestPending, formatTime(now.Add(-ThreadRetention)))
	return err
}

func messageAAD(messageID, requestID, profileID, author string) []byte {
	return []byte(strings.Join([]string{"portal-message", messageID, requestID, profileID, author}, "\x00"))
}
