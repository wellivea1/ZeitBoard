package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"time"

	"non24.app/core/domain"
)

type CollectionPreference struct {
	Enabled bool   `json:"enabled"`
	ZoneID  string `json:"zoneId"`
}

// Missing or unreadable consent never grants collection. Preferences share the
// database's transaction durability; there is no older backup grant to revive.
func (s *Store) CollectionPreference(ctx context.Context, source domain.DataSourceID) (CollectionPreference, error) {
	var result CollectionPreference
	err := s.db.QueryRowContext(ctx, `SELECT enabled, zone_id FROM source_collection_preferences WHERE source_id = ?`, source).Scan(&result.Enabled, &result.ZoneID)
	if errors.Is(err, sql.ErrNoRows) {
		return CollectionPreference{}, nil
	}
	return result, err
}

func (s *Store) SetCollectionPreference(ctx context.Context, source domain.DataSourceID, value CollectionPreference) error {
	if source == "" {
		return errors.New("collection source is required")
	}
	if _, err := time.LoadLocation(value.ZoneID); err != nil || value.ZoneID == "" || value.ZoneID == "Local" {
		return errors.New("select a valid IANA time zone for collection")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO source_collection_preferences(source_id, enabled, zone_id) VALUES(?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET enabled = excluded.enabled, zone_id = excluded.zone_id`, source, value.Enabled, value.ZoneID)
	return err
}

func (s *Store) SourceObservationCount(ctx context.Context, source domain.DataSourceID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM source_observations WHERE source_id = ?`, source).Scan(&count)
	return count, err
}

// The caller stops collection before erasure. Other evidence sources and saved
// consent are independent; disabling and erasing are distinct user actions.
func (s *Store) DeleteSourceObservations(ctx context.Context, source domain.DataSourceID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM source_observations WHERE source_id = ?`, source)
	if err != nil {
		return err
	}
	return s.compactDeletedData(ctx)
}

// WriteSourceObservations streams one database snapshot into an owner-selected
// export. Memory and renderer payload stay bounded regardless of history size.
func (s *Store) WriteSourceObservations(ctx context.Context, source domain.DataSourceID, out io.Writer) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, source_id, external_id, kind, observed_utc, zone_id, recorded_at, evidence_json, payload_json
		FROM source_observations WHERE source_id = ? ORDER BY rowid`, source)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if _, err := io.WriteString(out, "{\"schema_version\":\"v1\",\"observations\":[\n"); err != nil {
		return 0, err
	}
	count := 0
	for rows.Next() {
		var item domain.SourceObservation
		var observed, recorded string
		var evidence, payload []byte
		if err := rows.Scan(&item.ID, &item.SourceID, &item.ExternalID, &item.Kind, &observed, &item.ObservedAt.ZoneID, &recorded, &evidence, &payload); err != nil {
			return count, err
		}
		if item.ObservedAt.UTC, err = time.Parse(time.RFC3339Nano, observed); err != nil {
			return count, err
		}
		if item.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded); err != nil {
			return count, err
		}
		if err := json.Unmarshal(evidence, &item.Evidence); err != nil {
			return count, err
		}
		item.Payload = json.RawMessage(payload)
		if count > 0 {
			if _, err := io.WriteString(out, ",\n"); err != nil {
				return count, err
			}
		}
		if err := json.NewEncoder(out).Encode(item); err != nil {
			return count, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	_, err = io.WriteString(out, "]}\n")
	return count, err
}
