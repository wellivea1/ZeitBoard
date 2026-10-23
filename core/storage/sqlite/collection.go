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

// AppendNew keeps the observations not already kept, by their external id, and
// reports how many were new. It is for a source that may hand over the same
// record twice, such as the operating system's log (ADR-0060).
func (s *Store) AppendNew(ctx context.Context, observations []domain.SourceObservation) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, observation := range observations {
		if observation.ID == "" || observation.SourceID == "" || observation.ExternalID == "" {
			return 0, errors.New("an observation kept once needs an ID, a source and an external ID")
		}
		if err := observation.ObservedAt.Validate(); err != nil {
			return 0, err
		}
		evidence, err := json.Marshal(observation.Evidence)
		if err != nil {
			return 0, err
		}
		result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO source_observations(
			id, source_id, external_id, kind, observed_utc, zone_id, recorded_at, evidence_json, payload_json
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			observation.ID, observation.SourceID, observation.ExternalID, observation.Kind,
			observation.ObservedAt.UTC.Format(time.RFC3339Nano), observation.ObservedAt.ZoneID,
			observation.RecordedAt.UTC().Format(time.RFC3339Nano), evidence, []byte(observation.Payload))
		if err != nil {
			return 0, err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		added += int(inserted)
	}
	return added, tx.Commit()
}

// SourceObservations reads one source's observations, oldest first.
func (s *Store) SourceObservations(ctx context.Context, source domain.DataSourceID) ([]domain.SourceObservation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, source_id, external_id, kind, observed_utc, zone_id, recorded_at, evidence_json, payload_json
		FROM source_observations WHERE source_id = ? ORDER BY observed_utc, rowid`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var observations []domain.SourceObservation
	for rows.Next() {
		item, err := scanSourceObservation(rows)
		if err != nil {
			return nil, err
		}
		observations = append(observations, item)
	}
	return observations, rows.Err()
}

// ObservationSpan is how many observations a source made and when the oldest
// and newest of them happened.
type ObservationSpan struct {
	Count          int
	Oldest, Latest time.Time
}

// SourceObservationSpan spans a source's observations, or only those made
// under algorithm when it is not empty.
func (s *Store) SourceObservationSpan(ctx context.Context, source domain.DataSourceID, algorithm string) (ObservationSpan, error) {
	var span ObservationSpan
	var oldest, latest sql.NullString
	query, args := spanQuery(source, algorithm)
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&span.Count, &oldest, &latest)
	if err != nil || span.Count == 0 {
		return span, err
	}
	if span.Oldest, err = time.Parse(time.RFC3339Nano, oldest.String); err != nil {
		return span, err
	}
	span.Latest, err = time.Parse(time.RFC3339Nano, latest.String)
	return span, err
}

// spanQuery writes the span's query in one form for each case, so that one
// algorithm's span reads idx_source_observations_algorithm alone. A single
// form that let an empty algorithm match them all would read and parse every
// observation of the source.
func spanQuery(source domain.DataSourceID, algorithm string) (string, []any) {
	query := `SELECT COUNT(*), MIN(observed_utc), MAX(observed_utc) FROM source_observations WHERE source_id = ?`
	if algorithm == "" {
		return query, []any{source}
	}
	return query + ` AND json_extract(evidence_json, '$.algorithm') = ?`, []any{source, algorithm}
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
		item, err := scanSourceObservation(rows)
		if err != nil {
			return count, err
		}
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

func scanSourceObservation(rows *sql.Rows) (domain.SourceObservation, error) {
	var item domain.SourceObservation
	var observed, recorded string
	var evidence, payload []byte
	if err := rows.Scan(&item.ID, &item.SourceID, &item.ExternalID, &item.Kind, &observed, &item.ObservedAt.ZoneID, &recorded, &evidence, &payload); err != nil {
		return item, err
	}
	var err error
	if item.ObservedAt.UTC, err = time.Parse(time.RFC3339Nano, observed); err != nil {
		return item, err
	}
	if item.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded); err != nil {
		return item, err
	}
	if err := json.Unmarshal(evidence, &item.Evidence); err != nil {
		return item, err
	}
	item.Payload = json.RawMessage(payload)
	return item, nil
}
