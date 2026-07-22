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
		`DROP INDEX IF EXISTS idx_local_sleep_import_source_record`,
		`CREATE TRIGGER IF NOT EXISTS trg_local_sleep_import_source_record
			BEFORE INSERT ON local_sleep_observations
			WHEN NEW.acquisition_method = 'file_import'
				AND NEW.source_record_id <> ''
				AND EXISTS (
					SELECT 1 FROM local_sleep_observations
					WHERE acquisition_method = 'file_import'
						AND source_record_id = NEW.source_record_id
				)
			BEGIN
				SELECT RAISE(ABORT, 'duplicate imported source_record_id');
			END`,
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
		`CREATE TABLE IF NOT EXISTS local_sync_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS local_sleep_sync_records (
			record_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			payload_hash TEXT NOT NULL,
			pushed_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS local_sleep_erasures (
			record_id TEXT PRIMARY KEY,
			erased_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS local_tasks (
			task_id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			payload_json BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS local_task_sync_records (
			record_id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			pushed_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS local_medications (
			medication_id TEXT PRIMARY KEY,
			active INTEGER NOT NULL CHECK(active IN (0, 1)),
			revision INTEGER NOT NULL CHECK(revision >= 1),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			payload_json BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_medications_active
			ON local_medications(active, updated_at)`,
		`CREATE TABLE IF NOT EXISTS local_medication_events (
			event_id TEXT PRIMARY KEY,
			medication_id TEXT NOT NULL,
			dose_at TEXT NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('taken', 'skipped')),
			scheduled INTEGER NOT NULL CHECK(scheduled IN (0, 1)),
			recorded_at TEXT NOT NULL,
			payload_json BLOB NOT NULL,
			FOREIGN KEY(medication_id) REFERENCES local_medications(medication_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_medication_events_dose
			ON local_medication_events(dose_at, event_id)`,
		`CREATE TRIGGER IF NOT EXISTS trg_local_medication_events_immutable
			BEFORE UPDATE ON local_medication_events
			BEGIN
				SELECT RAISE(ABORT, 'medication events are immutable');
			END`,
		`CREATE TABLE IF NOT EXISTS local_medication_event_corrections (
			correction_id TEXT PRIMARY KEY,
			target_event_id TEXT NOT NULL,
			supersedes_correction_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			reason TEXT NOT NULL CHECK(reason IN ('user_edit', 'duplicate', 'invalid_time')),
			changes_json BLOB NOT NULL,
			payload_json BLOB NOT NULL,
			FOREIGN KEY(target_event_id) REFERENCES local_medication_events(event_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_medication_corrections_target
			ON local_medication_event_corrections(target_event_id, created_at, correction_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_local_medication_corrections_supersedes
			ON local_medication_event_corrections(supersedes_correction_id)
			WHERE supersedes_correction_id <> ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_local_medication_corrections_root
			ON local_medication_event_corrections(target_event_id)
			WHERE supersedes_correction_id = ''`,
		`CREATE TRIGGER IF NOT EXISTS trg_local_medication_corrections_immutable
			BEFORE UPDATE ON local_medication_event_corrections
			BEGIN
				SELECT RAISE(ABORT, 'medication event corrections are immutable');
			END`,
		`CREATE TABLE IF NOT EXISTS local_calendar_sources (
			source_id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			kind TEXT NOT NULL CHECK(kind IN ('ics', 'caldav', 'zeitboard')),
			read_only INTEGER NOT NULL CHECK(read_only IN (0, 1)),
			coverage_start_at TEXT NOT NULL,
			coverage_end_at TEXT NOT NULL,
			last_imported_at TEXT NOT NULL,
			endpoint TEXT NOT NULL DEFAULT '',
			CHECK(
				(kind IN ('ics', 'caldav') AND read_only = 1) OR
				(kind = 'zeitboard' AND read_only = 0)
			)
		)`,
		`CREATE TABLE IF NOT EXISTS local_calendar_events (
			event_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			source_record_id TEXT NOT NULL,
			title TEXT NOT NULL,
			start_at TEXT NOT NULL,
			end_at TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			all_day INTEGER NOT NULL CHECK(all_day IN (0, 1)),
			busy INTEGER NOT NULL CHECK(busy IN (0, 1)),
			ownership TEXT NOT NULL CHECK(ownership IN ('imported', 'app_owned')),
			created_at TEXT NOT NULL,
			location TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			task_revision INTEGER NOT NULL DEFAULT 0,
			proposal_id TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(source_id) REFERENCES local_calendar_sources(source_id) ON DELETE CASCADE,
			UNIQUE(source_id, source_record_id),
			CHECK(end_at >= start_at),
			CHECK(busy = 0 OR end_at > start_at),
			CHECK(
				(ownership = 'imported' AND task_id = '' AND task_revision = 0 AND proposal_id = '') OR
				(ownership = 'app_owned' AND task_id <> '' AND task_revision >= 1 AND proposal_id <> '')
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_calendar_events_interval
			ON local_calendar_events(start_at, end_at)`,
		`CREATE INDEX IF NOT EXISTS idx_local_calendar_events_busy
			ON local_calendar_events(busy, start_at, end_at)`,
		`CREATE TRIGGER IF NOT EXISTS trg_local_calendar_imported_immutable
			BEFORE UPDATE ON local_calendar_events
			WHEN OLD.ownership = 'imported'
			BEGIN
				SELECT RAISE(ABORT, 'imported calendar events are immutable');
			END`,
		`CREATE TABLE IF NOT EXISTS local_proposal_decisions (
			decision_id TEXT PRIMARY KEY,
			proposal_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			task_revision INTEGER NOT NULL,
			estimate_id TEXT NOT NULL,
			proposal_title TEXT NOT NULL,
			proposal_start_at TEXT NOT NULL,
			proposal_end_at TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			confidence TEXT NOT NULL CHECK(confidence IN ('low', 'medium', 'high')),
			explanation_codes_json BLOB NOT NULL,
			decision TEXT NOT NULL CHECK(decision IN ('approved', 'rejected', 'undone')),
			decided_at TEXT NOT NULL,
			supersedes_decision_id TEXT NOT NULL DEFAULT '',
			event_id TEXT NOT NULL DEFAULT '',
			snapshot_start_at TEXT NOT NULL,
			snapshot_end_at TEXT NOT NULL,
			event_snapshot_hash TEXT NOT NULL,
			sleep_snapshot_hash TEXT NOT NULL DEFAULT '',
			CHECK(snapshot_end_at > snapshot_start_at),
			CHECK(proposal_end_at > proposal_start_at),
			CHECK(
				(decision = 'approved' AND event_id <> '' AND supersedes_decision_id = '') OR
				(decision = 'rejected' AND event_id = '' AND supersedes_decision_id = '') OR
				(decision = 'undone' AND supersedes_decision_id <> '')
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_local_proposal_decisions_proposal
			ON local_proposal_decisions(proposal_id, decided_at, decision_id)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}
	hasSleepSnapshotHash, err := sqliteTableHasColumn(ctx, s.db, "local_proposal_decisions", "sleep_snapshot_hash")
	if err != nil {
		return fmt.Errorf("inspect proposal decision migration: %w", err)
	}
	if !hasSleepSnapshotHash {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE local_proposal_decisions
			ADD COLUMN sleep_snapshot_hash TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add proposal sleep snapshot hash: %w", err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, version := range []int{1, 2, 3, 4, 5, 6} {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, ?)`, version, now); err != nil {
			return err
		}
	}
	return nil
}

func sqliteTableHasColumn(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
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
	for _, table := range []string{"source_observations", "manual_corrections", "phase_estimates", "medication_events", "share_profiles", "local_sleep_corrections", "local_sleep_observations", "local_medication_event_corrections", "local_medication_events", "local_medications", "local_proposal_decisions", "local_calendar_events", "local_calendar_sources"} {
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
