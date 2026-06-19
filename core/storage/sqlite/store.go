package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"non24.app/core/domain"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA secure_delete = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS source_observations (
			id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			external_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			observed_utc TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			recorded_at TEXT NOT NULL,
			evidence_json BLOB NOT NULL,
			payload_json BLOB NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_source_external
			ON source_observations(source_id, external_id) WHERE external_id <> ''`,
		`CREATE TABLE IF NOT EXISTS manual_corrections (
			id TEXT PRIMARY KEY,
			target_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			correction_json BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS phase_estimates (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			estimate_json BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS medication_events (
			id TEXT PRIMARY KEY,
			taken_at TEXT NOT NULL,
			event_json BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS share_profiles (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			profile_json BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS local_sleep_observations (
			observation_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			start_at TEXT NOT NULL,
			end_at TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			classification TEXT NOT NULL,
			acquisition_method TEXT NOT NULL,
			evidence_status TEXT NOT NULL,
			recorded_at TEXT NOT NULL,
			source_record_id TEXT NOT NULL DEFAULT '',
			payload_json BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_sleep_observations_start
			ON local_sleep_observations(start_at)`,
		`CREATE TABLE IF NOT EXISTS local_sleep_corrections (
			correction_id TEXT PRIMARY KEY,
			target_observation_id TEXT NOT NULL,
			supersedes_correction_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			reason TEXT NOT NULL,
			changes_json BLOB NOT NULL,
			payload_json BLOB NOT NULL,
			FOREIGN KEY(target_observation_id) REFERENCES local_sleep_observations(observation_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_sleep_corrections_target
			ON local_sleep_corrections(target_observation_id, created_at)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?)`, now); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(2, ?)`, now)
	return err
}

func (s *Store) AppendObservation(ctx context.Context, observation domain.SourceObservation) error {
	if observation.ID == "" || observation.SourceID == "" {
		return errors.New("observation ID and source ID are required")
	}
	if err := observation.ObservedAt.Validate(); err != nil {
		return err
	}
	evidence, err := json.Marshal(observation.Evidence)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO source_observations(
		id, source_id, external_id, kind, observed_utc, zone_id, recorded_at, evidence_json, payload_json
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observation.ID, observation.SourceID, observation.ExternalID, observation.Kind,
		observation.ObservedAt.UTC.Format(time.RFC3339Nano), observation.ObservedAt.ZoneID,
		observation.RecordedAt.UTC().Format(time.RFC3339Nano), evidence, []byte(observation.Payload),
	)
	return err
}

func (s *Store) AddCorrection(ctx context.Context, correction domain.ManualCorrection) error {
	encoded, err := json.Marshal(correction)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO manual_corrections(id, target_id, created_at, correction_json) VALUES(?, ?, ?, ?)`, correction.ID, correction.TargetID, correction.CreatedAt.UTC().Format(time.RFC3339Nano), encoded)
	return err
}

func (s *Store) SaveEstimate(ctx context.Context, estimate domain.PhaseEstimate) error {
	encoded, err := json.Marshal(estimate)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO phase_estimates(id, created_at, estimate_json) VALUES(?, ?, ?)`, estimate.ID, estimate.CreatedAt.UTC().Format(time.RFC3339Nano), encoded)
	return err
}

func (s *Store) SaveMedicationEvent(ctx context.Context, event domain.MedicationEvent) error {
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO medication_events(id, taken_at, event_json) VALUES(?, ?, ?)`, event.ID, event.TakenAt.UTC.Format(time.RFC3339Nano), encoded)
	return err
}

func (s *Store) SaveShareProfile(ctx context.Context, profile domain.ShareProfile) error {
	encoded, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO share_profiles(id, created_at, profile_json) VALUES(?, ?, ?)`, profile.ID, profile.CreatedAt.UTC().Format(time.RFC3339Nano), encoded)
	return err
}

type ExportBundle struct {
	SchemaVersion    int                        `json:"schemaVersion"`
	ExportedAt       time.Time                  `json:"exportedAt"`
	Observations     []domain.SourceObservation `json:"observations"`
	Corrections      []domain.ManualCorrection  `json:"corrections"`
	Estimates        []domain.PhaseEstimate     `json:"estimates"`
	MedicationEvents []domain.MedicationEvent   `json:"medicationEvents"`
	ShareProfiles    []domain.ShareProfile      `json:"shareProfiles"`
}

func (s *Store) Export(ctx context.Context) (ExportBundle, error) {
	bundle := ExportBundle{SchemaVersion: 1, ExportedAt: time.Now().UTC()}
	rows, err := s.db.QueryContext(ctx, `SELECT id, source_id, external_id, kind, observed_utc, zone_id, recorded_at, evidence_json, payload_json FROM source_observations ORDER BY observed_utc`)
	if err != nil {
		return bundle, err
	}
	for rows.Next() {
		var observation domain.SourceObservation
		var observedUTC, recordedAt string
		var evidence, payload []byte
		if err := rows.Scan(&observation.ID, &observation.SourceID, &observation.ExternalID, &observation.Kind, &observedUTC, &observation.ObservedAt.ZoneID, &recordedAt, &evidence, &payload); err != nil {
			_ = rows.Close()
			return bundle, err
		}
		observation.ObservedAt.UTC, _ = time.Parse(time.RFC3339Nano, observedUTC)
		observation.RecordedAt, _ = time.Parse(time.RFC3339Nano, recordedAt)
		if err := json.Unmarshal(evidence, &observation.Evidence); err != nil {
			_ = rows.Close()
			return bundle, err
		}
		observation.Payload = append([]byte(nil), payload...)
		bundle.Observations = append(bundle.Observations, observation)
	}
	if err := rows.Close(); err != nil {
		return bundle, err
	}
	if err := s.readJSONRows(ctx, `SELECT correction_json FROM manual_corrections ORDER BY created_at`, func(value []byte) error {
		var item domain.ManualCorrection
		if err := json.Unmarshal(value, &item); err != nil {
			return err
		}
		bundle.Corrections = append(bundle.Corrections, item)
		return nil
	}); err != nil {
		return bundle, err
	}
	if err := s.readJSONRows(ctx, `SELECT estimate_json FROM phase_estimates ORDER BY created_at`, func(value []byte) error {
		var item domain.PhaseEstimate
		if err := json.Unmarshal(value, &item); err != nil {
			return err
		}
		bundle.Estimates = append(bundle.Estimates, item)
		return nil
	}); err != nil {
		return bundle, err
	}
	if err := s.readJSONRows(ctx, `SELECT event_json FROM medication_events ORDER BY taken_at`, func(value []byte) error {
		var item domain.MedicationEvent
		if err := json.Unmarshal(value, &item); err != nil {
			return err
		}
		bundle.MedicationEvents = append(bundle.MedicationEvents, item)
		return nil
	}); err != nil {
		return bundle, err
	}
	if err := s.readJSONRows(ctx, `SELECT profile_json FROM share_profiles ORDER BY created_at`, func(value []byte) error {
		var item domain.ShareProfile
		if err := json.Unmarshal(value, &item); err != nil {
			return err
		}
		bundle.ShareProfiles = append(bundle.ShareProfiles, item)
		return nil
	}); err != nil {
		return bundle, err
	}
	return bundle, nil
}

func (s *Store) readJSONRows(ctx context.Context, query string, visit func([]byte) error) error {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value []byte
		if err := rows.Scan(&value); err != nil {
			return err
		}
		if err := visit(value); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) DeleteAll(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, table := range []string{"source_observations", "manual_corrections", "phase_estimates", "medication_events", "share_profiles", "local_sleep_corrections", "local_sleep_observations"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.compactDeletedData(ctx)
}

func (s *Store) compactDeletedData(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `VACUUM`)
	return err
}
