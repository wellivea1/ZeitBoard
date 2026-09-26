// Package markers defines the rhythm context marker of
// contracts/v1/rhythm-marker-set: an immutable, user-reported note that
// travel, illness or an obligation explains an unusual day. It is defined and
// validated once, here, for the desktop's store and the server's sync
// validation (ADR-0048). A marker is deliberately not an estimator input.
package markers

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"non24.app/core/domain"
	"non24.app/core/sleepv1"
)

const (
	Travel         = "travel"
	Illness        = "illness"
	Disruption     = "disruption"
	ForcedSchedule = "forced_schedule"
)

var (
	identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)
	zoneID     = regexp.MustCompile(`^(?:UTC|[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)+)$`)
)

type Record struct {
	MarkerID   string             `json:"marker_id"`
	Kind       string             `json:"kind"`
	StartAt    time.Time          `json:"start_at"`
	EndAt      *time.Time         `json:"end_at,omitempty"`
	ZoneID     string             `json:"zone_id"`
	Note       string             `json:"note,omitempty"`
	Provenance sleepv1.Provenance `json:"provenance"`
}

func ValidKind(value string) bool {
	switch value {
	case Travel, Illness, Disruption, ForcedSchedule:
		return true
	default:
		return false
	}
}

func (record Record) Validate() error {
	if !identifier.MatchString(record.MarkerID) {
		return errors.New("marker_id must match the v1 identifier format")
	}
	if !ValidKind(record.Kind) {
		return errors.New("kind must be travel, illness, disruption, or forced_schedule")
	}
	if record.StartAt.IsZero() {
		return errors.New("start_at is required")
	}
	if record.EndAt != nil && !record.EndAt.After(record.StartAt) {
		return errors.New("end_at must be after start_at")
	}
	if len(record.ZoneID) > 64 || !zoneID.MatchString(record.ZoneID) {
		return errors.New("zone_id must be an explicit IANA time-zone identifier")
	}
	if _, err := domain.NewZonedInstant(record.StartAt, record.ZoneID); err != nil {
		return fmt.Errorf("marker start: %w", err)
	}
	if strings.TrimSpace(record.Note) != record.Note || len(record.Note) > 500 {
		return errors.New("note must be canonical private text up to 500 characters")
	}
	if record.Provenance.AcquisitionMethod != sleepv1.AcquisitionManual || record.Provenance.EvidenceStatus != sleepv1.EvidenceUserReported {
		return errors.New("rhythm marker provenance must be manual and user_reported")
	}
	if record.Provenance.RecordedAt.IsZero() {
		return errors.New("provenance.recorded_at is required")
	}
	if record.Provenance.SourceRecordID != "" {
		return errors.New("manual rhythm markers cannot carry a source_record_id")
	}
	if record.StartAt.After(record.Provenance.RecordedAt) {
		return errors.New("start_at cannot be after provenance.recorded_at")
	}
	if record.EndAt != nil && record.EndAt.After(record.Provenance.RecordedAt) {
		return errors.New("end_at cannot be after provenance.recorded_at")
	}
	return nil
}
