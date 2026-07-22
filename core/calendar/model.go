// Package calendar imports private calendar snapshots and exports blocks owned
// by ZeitBoard. Calendar text must not cross into server projection types.
package calendar

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MaxDocumentBytes       = 8 << 20
	MaxEventComponents     = 20_000
	MaxMaterializedEvents  = 50_000
	DefaultHistoryDays     = 366
	DefaultFutureDays      = 732
	maxSourceLabelRunes    = 120
	maxTitleRunes          = 240
	maxLocationRunes       = 240
	maxNotesRunes          = 2_000
	maxSourceRecordIDRunes = 512
)

type SourceKind string

const (
	SourceICS       SourceKind = "ics"
	SourceCalDAV    SourceKind = "caldav"
	SourceZeitBoard SourceKind = "zeitboard"
)

type Ownership string

const (
	OwnershipImported Ownership = "imported"
	OwnershipAppOwned Ownership = "app_owned"
)

type Source struct {
	SourceID        string     `json:"source_id"`
	Label           string     `json:"label"`
	Kind            SourceKind `json:"kind"`
	ReadOnly        bool       `json:"read_only"`
	CoverageStartAt time.Time  `json:"coverage_start_at"`
	CoverageEndAt   time.Time  `json:"coverage_end_at"`
	LastImportedAt  time.Time  `json:"last_imported_at"`
}

type Event struct {
	EventID        string    `json:"event_id"`
	SourceID       string    `json:"source_id"`
	SourceRecordID string    `json:"source_record_id"`
	Title          string    `json:"title"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	ZoneID         string    `json:"zone_id"`
	AllDay         bool      `json:"all_day"`
	Busy           bool      `json:"busy"`
	Ownership      Ownership `json:"ownership"`
	CreatedAt      time.Time `json:"created_at"`
	Location       string    `json:"location,omitempty"`
	Notes          string    `json:"notes,omitempty"`
	TaskID         string    `json:"task_id,omitempty"`
	TaskRevision   int       `json:"task_revision,omitempty"`
	ProposalID     string    `json:"proposal_id,omitempty"`
}

type EventSet struct {
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Sources       []Source  `json:"sources"`
	Events        []Event   `json:"events"`
}

type ParseOptions struct {
	SourceID      string
	SourceLabel   string
	Kind          SourceKind
	ImportedAt    time.Time
	CoverageStart time.Time
	CoverageEnd   time.Time
	DefaultZoneID string
}

func CoverageAround(now time.Time) (time.Time, time.Time) {
	now = now.UTC()
	return now.AddDate(0, 0, -DefaultHistoryDays), now.AddDate(0, 0, DefaultFutureDays)
}

func (o ParseOptions) validate() error {
	if !validIdentifier(o.SourceID) {
		return fmt.Errorf("source id %q is not a v1 identifier", o.SourceID)
	}
	if err := boundedText("source label", o.SourceLabel, 1, maxSourceLabelRunes); err != nil {
		return err
	}
	if o.Kind != SourceICS && o.Kind != SourceCalDAV {
		return fmt.Errorf("calendar import source kind %q is not supported", o.Kind)
	}
	if o.ImportedAt.IsZero() {
		return errors.New("imported_at is required")
	}
	if o.CoverageStart.IsZero() || o.CoverageEnd.IsZero() || !o.CoverageStart.Before(o.CoverageEnd) {
		return errors.New("calendar coverage must be a non-empty interval")
	}
	if _, _, err := loadLocation(o.DefaultZoneID); err != nil {
		return fmt.Errorf("default zone: %w", err)
	}
	return nil
}

func (s Source) Validate() error {
	if !validIdentifier(s.SourceID) {
		return fmt.Errorf("source id %q is not a v1 identifier", s.SourceID)
	}
	if err := boundedText("source label", s.Label, 1, maxSourceLabelRunes); err != nil {
		return err
	}
	switch s.Kind {
	case SourceICS, SourceCalDAV:
		if !s.ReadOnly {
			return errors.New("imported calendar sources must be read-only")
		}
	case SourceZeitBoard:
		if s.ReadOnly {
			return errors.New("the ZeitBoard calendar source must be writable")
		}
	default:
		return fmt.Errorf("unknown calendar source kind %q", s.Kind)
	}
	if s.CoverageStartAt.IsZero() || s.CoverageEndAt.IsZero() || !s.CoverageStartAt.Before(s.CoverageEndAt) {
		return errors.New("calendar source coverage must be a non-empty interval")
	}
	if s.LastImportedAt.IsZero() {
		return errors.New("calendar source last_imported_at is required")
	}
	return nil
}

func (e Event) Validate() error {
	if !validIdentifier(e.EventID) || !validIdentifier(e.SourceID) {
		return errors.New("calendar event and source ids must be v1 identifiers")
	}
	if err := boundedText("source record id", e.SourceRecordID, 1, maxSourceRecordIDRunes); err != nil {
		return err
	}
	if err := boundedText("event title", e.Title, 1, maxTitleRunes); err != nil {
		return err
	}
	if e.StartAt.IsZero() || e.EndAt.IsZero() || e.EndAt.Before(e.StartAt) {
		return errors.New("calendar event interval is invalid")
	}
	if e.Busy && !e.StartAt.Before(e.EndAt) {
		return errors.New("a busy calendar event must have positive duration")
	}
	if _, _, err := loadLocation(e.ZoneID); err != nil {
		return fmt.Errorf("event zone: %w", err)
	}
	if e.CreatedAt.IsZero() {
		return errors.New("calendar event created_at is required")
	}
	if e.Location != "" {
		if err := boundedText("event location", e.Location, 1, maxLocationRunes); err != nil {
			return err
		}
	}
	if e.Notes != "" {
		if err := boundedText("event notes", e.Notes, 1, maxNotesRunes); err != nil {
			return err
		}
	}
	switch e.Ownership {
	case OwnershipImported:
		if e.TaskID != "" || e.TaskRevision != 0 || e.ProposalID != "" {
			return errors.New("imported calendar events cannot carry placement links")
		}
	case OwnershipAppOwned:
		if !validIdentifier(e.TaskID) || e.TaskRevision < 1 || !validIdentifier(e.ProposalID) {
			return errors.New("app-owned calendar events require task revision and proposal links")
		}
	default:
		return fmt.Errorf("unknown calendar ownership %q", e.Ownership)
	}
	return nil
}

func boundedText(name, value string, minRunes, maxRunes int) error {
	value = strings.TrimSpace(value)
	count := len([]rune(value))
	if count < minRunes || count > maxRunes {
		return fmt.Errorf("%s must contain %d to %d characters", name, minRunes, maxRunes)
	}
	return nil
}

func validIdentifier(value string) bool {
	if len(value) < 3 || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, r := range value[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}
