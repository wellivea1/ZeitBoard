package medication

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"non24.app/core/domain"
	"non24.app/core/sleepv1"
)

// The medication records of contracts/v2 (medication-set and
// medication-event-set). They are defined and validated once, here, and used
// by the desktop's store and by the server's sync validation (ADR-0048), so
// the two cannot disagree about what a valid record is.

const (
	EventTaken   = "taken"
	EventSkipped = "skipped"

	CorrectionUserEdit    = "user_edit"
	CorrectionDuplicate   = "duplicate"
	CorrectionInvalidTime = "invalid_time"
)

var contractIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)

// Record is one version of a medication definition: a mutable intention that
// syncs as immutable revisions.
type Record struct {
	MedicationID  string     `json:"medication_id"`
	Label         string     `json:"label"`
	Form          string     `json:"form,omitempty"`
	StrengthLabel string     `json:"strength_label,omitempty"`
	ClinicianRule string     `json:"clinician_rule,omitempty"`
	Active        bool       `json:"active"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	StartedZoneID string     `json:"started_zone_id,omitempty"`
	Schedule      *Schedule  `json:"schedule,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	Revision      int        `json:"revision"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// EventRecord is one recorded dose, taken or skipped: append-only evidence.
type EventRecord struct {
	EventID      string             `json:"event_id"`
	MedicationID string             `json:"medication_id"`
	DoseAt       time.Time          `json:"dose_at"`
	ZoneID       string             `json:"zone_id"`
	Status       string             `json:"status"`
	Scheduled    bool               `json:"scheduled"`
	Note         string             `json:"note,omitempty"`
	Provenance   sleepv1.Provenance `json:"provenance"`
}

// CorrectionRecord changes an event without rewriting it.
type CorrectionRecord struct {
	CorrectionID           string            `json:"correction_id"`
	TargetEventID          string            `json:"target_event_id"`
	SupersedesCorrectionID string            `json:"supersedes_correction_id,omitempty"`
	CreatedAt              time.Time         `json:"created_at"`
	Reason                 string            `json:"reason"`
	Changes                CorrectionChanges `json:"changes"`
}

type CorrectionChanges struct {
	DoseAt    *time.Time `json:"dose_at,omitempty"`
	ZoneID    *string    `json:"zone_id,omitempty"`
	Status    *string    `json:"status,omitempty"`
	Scheduled *bool      `json:"scheduled,omitempty"`
	Note      *string    `json:"note,omitempty"`
	Excluded  *bool      `json:"excluded,omitempty"`
}

// ValidIdentifier reports whether a value is a contract identifier.
func ValidIdentifier(value string) bool {
	return contractIdentifier.MatchString(value)
}

func (record Record) Validate() error {
	if !contractIdentifier.MatchString(record.MedicationID) {
		return errors.New("medication_id must match the v1 identifier format")
	}
	if err := canonicalPrivateText("label", record.Label, 120, true); err != nil {
		return err
	}
	if err := canonicalPrivateText("form", record.Form, 80, false); err != nil {
		return err
	}
	if err := canonicalPrivateText("strength_label", record.StrengthLabel, 80, false); err != nil {
		return err
	}
	if err := canonicalPrivateText("clinician_rule", record.ClinicianRule, 500, false); err != nil {
		return err
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return errors.New("created_at and updated_at are required")
	}
	if record.UpdatedAt.Before(record.CreatedAt) {
		return errors.New("updated_at must not precede created_at")
	}
	if record.Revision < 1 {
		return errors.New("revision must be at least 1")
	}
	if (record.StartedAt == nil) != (record.StartedZoneID == "") {
		return errors.New("started_at and started_zone_id must be set together")
	}
	if record.StartedAt != nil {
		if _, err := domain.NewZonedInstant(*record.StartedAt, record.StartedZoneID); err != nil {
			return fmt.Errorf("medication start: %w", err)
		}
	}
	if record.Schedule != nil {
		if err := record.Schedule.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (record EventRecord) Validate() error {
	if !contractIdentifier.MatchString(record.EventID) {
		return errors.New("event_id must match the v1 identifier format")
	}
	if !contractIdentifier.MatchString(record.MedicationID) {
		return errors.New("medication_id must match the v1 identifier format")
	}
	if record.Status != EventTaken && record.Status != EventSkipped {
		return errors.New("status must be taken or skipped")
	}
	if record.DoseAt.IsZero() {
		return errors.New("dose_at is required")
	}
	if _, err := domain.NewZonedInstant(record.DoseAt, record.ZoneID); err != nil {
		return err
	}
	if err := canonicalPrivateText("note", record.Note, 500, false); err != nil {
		return err
	}
	if !sleepv1.ValidAcquisition(record.Provenance.AcquisitionMethod) || !sleepv1.ValidEvidenceStatus(record.Provenance.EvidenceStatus) {
		return errors.New("medication event provenance is not supported")
	}
	if record.Provenance.RecordedAt.IsZero() {
		return errors.New("provenance.recorded_at is required")
	}
	if sourceRecordID := record.Provenance.SourceRecordID; sourceRecordID != "" {
		if strings.TrimSpace(sourceRecordID) != sourceRecordID || len(sourceRecordID) > 128 {
			return errors.New("provenance.source_record_id must be canonical text up to 128 characters")
		}
	}
	return nil
}

func (record CorrectionRecord) Validate() error {
	if !contractIdentifier.MatchString(record.CorrectionID) || !contractIdentifier.MatchString(record.TargetEventID) {
		return errors.New("correction_id and target_event_id must match the v1 identifier format")
	}
	if record.SupersedesCorrectionID != "" && !contractIdentifier.MatchString(record.SupersedesCorrectionID) {
		return errors.New("supersedes_correction_id must match the v1 identifier format")
	}
	if record.CreatedAt.IsZero() {
		return errors.New("created_at is required")
	}
	if record.Reason != CorrectionUserEdit && record.Reason != CorrectionDuplicate && record.Reason != CorrectionInvalidTime {
		return errors.New("medication correction reason is not supported")
	}
	changes := record.Changes
	count := 0
	if changes.DoseAt != nil {
		if changes.DoseAt.IsZero() {
			return errors.New("changes.dose_at is invalid")
		}
		count++
	}
	if changes.ZoneID != nil {
		if strings.TrimSpace(*changes.ZoneID) == "" {
			return errors.New("changes.zone_id is invalid")
		}
		if _, err := domain.NewZonedInstant(time.Unix(0, 0).UTC(), *changes.ZoneID); err != nil {
			return fmt.Errorf("changes.zone_id: %w", err)
		}
		count++
	}
	if changes.Status != nil {
		if *changes.Status != EventTaken && *changes.Status != EventSkipped {
			return errors.New("changes.status must be taken or skipped")
		}
		count++
	}
	if changes.Scheduled != nil {
		count++
	}
	if changes.Note != nil {
		if len(*changes.Note) > 500 || strings.TrimSpace(*changes.Note) != *changes.Note {
			return errors.New("changes.note must be canonical private text up to 500 characters")
		}
		count++
	}
	if changes.Excluded != nil {
		count++
	}
	if count == 0 {
		return errors.New("medication correction changes must not be empty")
	}
	return nil
}

func canonicalPrivateText(field, value string, maximum int, required bool) error {
	if strings.TrimSpace(value) != value || len(value) > maximum || (required && value == "") {
		return fmt.Errorf("%s must be canonical private text up to %d characters", field, maximum)
	}
	return nil
}
