package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"non24.app/core/domain"
)

const (
	RhythmMarkerTravel         = "travel"
	RhythmMarkerIllness        = "illness"
	RhythmMarkerDisruption     = "disruption"
	RhythmMarkerForcedSchedule = "forced_schedule"
)

var (
	ErrRhythmMarkerNotFound = errors.New("rhythm marker does not exist")
	rhythmMarkerZoneID      = regexp.MustCompile(`^(?:UTC|[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)+)$`)
)

// RhythmMarkerRecord is an immutable, user-reported context annotation. It is
// deliberately separate from estimator inputs so a marker cannot alter a
// rhythm estimate or be mistaken for diagnostic evidence.
type RhythmMarkerRecord struct {
	MarkerID   string                     `json:"marker_id"`
	Kind       string                     `json:"kind"`
	StartAt    time.Time                  `json:"start_at"`
	EndAt      *time.Time                 `json:"end_at,omitempty"`
	ZoneID     string                     `json:"zone_id"`
	Note       string                     `json:"note,omitempty"`
	Provenance SleepObservationProvenance `json:"provenance"`
}

type RhythmMarkerSet struct {
	SchemaVersion string               `json:"schema_version"`
	GeneratedAt   time.Time            `json:"generated_at"`
	Markers       []RhythmMarkerRecord `json:"markers"`
}

func (s *Store) CreateRhythmMarker(ctx context.Context, record RhythmMarkerRecord) error {
	record = normalizeRhythmMarker(record)
	if err := validateRhythmMarker(record); err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	endAt := ""
	if record.EndAt != nil {
		endAt = formatSQLiteTime(*record.EndAt)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO local_rhythm_markers(
		marker_id, kind, start_at, end_at, zone_id, recorded_at, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		record.MarkerID, record.Kind, formatSQLiteTime(record.StartAt), endAt,
		record.ZoneID, formatSQLiteTime(record.Provenance.RecordedAt), encoded,
	)
	return err
}

func (s *Store) ListRhythmMarkers(ctx context.Context) ([]RhythmMarkerRecord, error) {
	records := make([]RhythmMarkerRecord, 0)
	err := s.readJSONRows(ctx, `SELECT payload_json FROM local_rhythm_markers
		ORDER BY start_at, marker_id`, func(value []byte) error {
		var record RhythmMarkerRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	return records, err
}

func (s *Store) ExportRhythmMarkers(ctx context.Context, generatedAt time.Time) (RhythmMarkerSet, error) {
	if generatedAt.IsZero() {
		return RhythmMarkerSet{}, errors.New("generated_at is required")
	}
	markers, err := s.ListRhythmMarkers(ctx)
	if err != nil {
		return RhythmMarkerSet{}, err
	}
	return RhythmMarkerSet{
		SchemaVersion: "v1",
		GeneratedAt:   generatedAt.UTC(),
		Markers:       markers,
	}, nil
}

// DeleteRhythmMarker is permanent erasure, not append-only suppression. The
// post-delete compaction removes deleted private text from SQLite free pages
// and truncates the write-ahead log.
func (s *Store) DeleteRhythmMarker(ctx context.Context, markerID string) error {
	if !contractIdentifier.MatchString(markerID) {
		return errors.New("marker_id must match the v1 identifier format")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM local_rhythm_markers WHERE marker_id = ?`, markerID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrRhythmMarkerNotFound
	}
	return s.compactDeletedData(ctx)
}

func normalizeRhythmMarker(record RhythmMarkerRecord) RhythmMarkerRecord {
	record.StartAt = record.StartAt.UTC()
	if record.EndAt != nil {
		endAt := record.EndAt.UTC()
		record.EndAt = &endAt
	}
	record.Provenance.RecordedAt = record.Provenance.RecordedAt.UTC()
	return record
}

func validateRhythmMarker(record RhythmMarkerRecord) error {
	if !contractIdentifier.MatchString(record.MarkerID) {
		return errors.New("marker_id must match the v1 identifier format")
	}
	if !validRhythmMarkerKind(record.Kind) {
		return errors.New("kind must be travel, illness, disruption, or forced_schedule")
	}
	if record.StartAt.IsZero() {
		return errors.New("start_at is required")
	}
	if record.EndAt != nil && !record.EndAt.After(record.StartAt) {
		return errors.New("end_at must be after start_at")
	}
	if len(record.ZoneID) > 64 || !rhythmMarkerZoneID.MatchString(record.ZoneID) {
		return errors.New("zone_id must be an explicit IANA time-zone identifier")
	}
	if _, err := domain.NewZonedInstant(record.StartAt, record.ZoneID); err != nil {
		return fmt.Errorf("marker start: %w", err)
	}
	if strings.TrimSpace(record.Note) != record.Note || len(record.Note) > 500 {
		return errors.New("note must be canonical private text up to 500 characters")
	}
	if record.Provenance.AcquisitionMethod != ProvenanceAcquisitionManual || record.Provenance.EvidenceStatus != ProvenanceEvidenceUserReported {
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

func validRhythmMarkerKind(value string) bool {
	switch value {
	case RhythmMarkerTravel, RhythmMarkerIllness, RhythmMarkerDisruption, RhythmMarkerForcedSchedule:
		return true
	default:
		return false
	}
}
