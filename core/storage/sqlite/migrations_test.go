package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestTaskRevisionAndSleepImportIndexMigrationsPreserveLegacyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	statements := []string{
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`CREATE TABLE local_tasks (
			task_id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			payload_json BLOB NOT NULL
		)`,
		`CREATE TABLE local_sleep_observations (
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
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	legacy := TaskRecord{
		TaskID: "task_legacy_revision", Title: "Legacy", DurationMinutes: 45,
		Status: TaskStatusOpen, CreatedAt: created, UpdatedAt: created, Revision: 4,
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO local_tasks(task_id, status, created_at, payload_json) VALUES(?, ?, ?, ?)`,
		legacy.TaskID, legacy.Status, formatSQLiteTime(created), payload); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"obs_legacy_duplicate_1", "obs_legacy_duplicate_2"} {
		if _, err := db.ExecContext(ctx, `INSERT INTO local_sleep_observations(
			observation_id, kind, start_at, end_at, zone_id, classification,
			acquisition_method, evidence_status, recorded_at, source_record_id, payload_json
		) VALUES(?, 'sleep_episode', ?, ?, 'UTC', 'principal', 'file_import',
			'directly_observed', ?, 'legacy-source', '{}')`,
			id, formatSQLiteTime(created), formatSQLiteTime(created.Add(8*time.Hour)), formatSQLiteTime(created)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open legacy store: %v", err)
	}
	defer store.Close()
	migrated, err := store.GetTask(ctx, legacy.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.Revision != 4 {
		t.Fatalf("migrated revision = %d, want 4", migrated.Revision)
	}

	rows, err := store.db.QueryContext(ctx, `PRAGMA index_list('local_sleep_observations')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			t.Fatal(err)
		}
		if name == "idx_local_sleep_import_source_record" {
			found = true
			if unique != 0 || partial != 1 {
				t.Fatalf("import index unique=%d partial=%d, want 0/1", unique, partial)
			}
		}
	}
	if !found {
		t.Fatal("sleep import source index was not created")
	}
	var applied int
	if err := store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version IN (9, 10)`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("new migration count = %d, want 2", applied)
	}
}
