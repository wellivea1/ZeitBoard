package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"non24.app/core/markers"
)

const (
	RhythmMarkerTravel         = markers.Travel
	RhythmMarkerIllness        = markers.Illness
	RhythmMarkerDisruption     = markers.Disruption
	RhythmMarkerForcedSchedule = markers.ForcedSchedule
)

var (
	ErrRhythmMarkerNotFound = errors.New("rhythm marker does not exist")
	rhythmMarkerZoneID      = regexp.MustCompile(`^(?:UTC|[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)+)$`)
)

// RhythmMarkerRecord is defined and validated in core/markers, shared with the
// server's sync validation (ADR-0048).
type RhythmMarkerRecord = markers.Record

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
	_, err = s.db.ExecContext(ctx, `INSERT INTO local_rhythm_markers(`+rhythmMarkerColumns+`)
		VALUES(?, ?, ?, ?, ?, ?, ?)`, rhythmMarkerRow(record, encoded)...)
	return err
}

const rhythmMarkerColumns = `marker_id, kind, start_at, end_at, zone_id, recorded_at, payload_json`

func rhythmMarkerRow(record RhythmMarkerRecord, encoded []byte) []any {
	endAt := ""
	if record.EndAt != nil {
		endAt = formatSQLiteTime(*record.EndAt)
	}
	return []any{record.MarkerID, record.Kind, formatSQLiteTime(record.StartAt), endAt,
		record.ZoneID, formatSQLiteTime(record.Provenance.RecordedAt), encoded}
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

// DeleteRhythmMarker is permanent erasure, not append-only suppression. It
// also queues the erasure for the server, so the owner's other devices lose
// the marker too (ADR-0048). The post-delete compaction removes deleted
// private text from SQLite free pages and truncates the write-ahead log.
func (s *Store) DeleteRhythmMarker(ctx context.Context, markerID string) error {
	if !contractIdentifier.MatchString(markerID) {
		return errors.New("marker_id must match the v1 identifier format")
	}
	return s.eraseHere(ctx, markerID, eraseMarkerTx, ErrRhythmMarkerNotFound)
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

func validateRhythmMarker(record RhythmMarkerRecord) error { return record.Validate() }

func validRhythmMarkerKind(value string) bool { return markers.ValidKind(value) }
