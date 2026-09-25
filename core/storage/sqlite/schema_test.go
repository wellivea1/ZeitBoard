package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestCurrentSchemaReopenPreservesTaskRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	created := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	task := TaskRecord{TaskID: "task_current", Title: "Synthetic task", DurationMinutes: 45,
		Status: TaskStatusOpen, CreatedAt: created, UpdatedAt: created, Revision: 1}
	if err := store.AddTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	task.Title = "Edited synthetic task"
	task.Revision = 2
	task.UpdatedAt = created.Add(time.Minute)
	if err := store.UpdateTask(ctx, task, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.GetTask(ctx, task.TaskID)
	if err != nil || got.Revision != 2 || got.Title != task.Title {
		t.Fatalf("reopen: %#v %v", got, err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'schema_migrations'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("current schema depends on historical migrations: %d %v", count, err)
	}
}

func TestUnsupportedDevelopmentSchemaFailsWithoutBackfill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unsupported.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE local_tasks(task_id TEXT PRIMARY KEY, status TEXT NOT NULL, created_at TEXT NOT NULL, payload_json BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO local_tasks VALUES('synthetic-old', 'open', '2026-09-08T12:00:00Z', '{"revision":4}')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if store, err := Open(path); err == nil {
		store.Close()
		t.Fatal("opened obsolete schema")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var data string
	if err := db.QueryRow(`SELECT payload_json FROM local_tasks WHERE task_id='synthetic-old'`).Scan(&data); err != nil || data != `{"revision":4}` {
		t.Fatalf("rewrote obsolete data: %q %v", data, err)
	}
}
