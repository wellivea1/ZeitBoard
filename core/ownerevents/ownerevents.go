// Package ownerevents defines the event the owner adds in ZeitBoard
// (ADR-0068): a title, a start and an end within a day of each other, and the
// zone it was entered in. Each change travels as an immutable revision,
// "<event_id>_r<revision>", and every device keeps the one that wins. It is
// defined and validated once, here, for the desktop's store and the server's
// sync validation.
package ownerevents

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"non24.app/core/domain"
)

const (
	// MaxTitleRunes bounds a title, as a task's is.
	MaxTitleRunes = 120
	// MaxLength bounds an event: a commitment, not a stretch of days.
	MaxLength = 24 * time.Hour
)

var (
	identifier   = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)
	zoneID       = regexp.MustCompile(`^(?:UTC|[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)+)$`)
	revisionPart = regexp.MustCompile(`^([a-z][a-z0-9_-]*)_r([1-9][0-9]*)$`)
)

// Record is one revision of an event, as it syncs.
type Record struct {
	EventID   string    `json:"event_id"`
	Title     string    `json:"title"`
	StartAt   time.Time `json:"start_at"`
	EndAt     time.Time `json:"end_at"`
	ZoneID    string    `json:"zone_id"`
	Revision  int       `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (record Record) Validate() error {
	if !identifier.MatchString(record.EventID) {
		return errors.New("event_id must match the v1 identifier format")
	}
	if err := ValidateTitle(record.Title); err != nil {
		return err
	}
	if record.StartAt.IsZero() || record.EndAt.IsZero() {
		return errors.New("an event needs a start and an end")
	}
	if !record.EndAt.After(record.StartAt) {
		return errors.New("an event must end after it starts")
	}
	if record.EndAt.Sub(record.StartAt) > MaxLength {
		return errors.New("an event can last at most a day")
	}
	if len(record.ZoneID) > 64 || !zoneID.MatchString(record.ZoneID) {
		return errors.New("zone_id must be an explicit IANA time-zone identifier")
	}
	if _, err := domain.NewZonedInstant(record.StartAt, record.ZoneID); err != nil {
		return fmt.Errorf("event start: %w", err)
	}
	if record.Revision < 1 {
		return errors.New("revision must be at least 1")
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.Before(record.CreatedAt) {
		return errors.New("an event is created before it is updated")
	}
	if len(RecordID(record.EventID, record.Revision)) > 64 {
		return errors.New("event_id is too long to carry a revision")
	}
	return nil
}

// ValidateTitle accepts a title as the owner wrote it: one line, trimmed,
// 1 to 120 characters.
func ValidateTitle(title string) error {
	count := utf8.RuneCountInString(title)
	if count == 0 || count > MaxTitleRunes || strings.TrimSpace(title) != title {
		return fmt.Errorf("a title is 1 to %d characters, with no space around it", MaxTitleRunes)
	}
	if strings.ContainsFunc(title, unicode.IsControl) {
		return errors.New("a title is one line of text")
	}
	return nil
}

// RecordID is the id of one revision's sync record.
func RecordID(eventID string, revision int) string {
	return eventID + "_r" + strconv.Itoa(revision)
}

// EventOf reads the event and revision a revision record id names.
func EventOf(recordID string) (eventID string, revision int, ok bool) {
	match := revisionPart.FindStringSubmatch(recordID)
	if match == nil {
		return "", 0, false
	}
	revision, err := strconv.Atoi(match[2])
	if err != nil {
		return "", 0, false
	}
	return match[1], revision, true
}

// Wins reports whether a revision replaces the one held: the higher revision,
// and of two with one number, as when two devices changed the event apart,
// the later edit. Every device so settles on the same revision whatever order
// they arrive in.
func Wins(candidate Record, candidatePayload []byte, held Record, heldPayload []byte) bool {
	if candidate.Revision != held.Revision {
		return candidate.Revision > held.Revision
	}
	return Later(candidate, candidatePayload, held, heldPayload)
}

// Later reports whether one edit was made after another: the later update,
// and of two made at one moment, the greater bytes. A device holding an edit
// it has not sent weighs another device's revision by this, whatever their
// numbers, so the edit made last is the one every device keeps.
func Later(candidate Record, candidatePayload []byte, held Record, heldPayload []byte) bool {
	if !candidate.UpdatedAt.Equal(held.UpdatedAt) {
		return candidate.UpdatedAt.After(held.UpdatedAt)
	}
	return bytes.Compare(candidatePayload, heldPayload) > 0
}
