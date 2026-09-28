package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An instance's database made by an earlier zeitboardd opens in this one with
// exactly the schema a new database gets. The store upgrades by creating what
// is missing, which is sound only while nothing that exists changes shape;
// this is the test that notices if something does.
//
// testdata/upgrade-from-2026-09-08.sql is the schema the store created on
// 2026-09-08 (commit 465c26e), the build the completion plan started from.
func TestADatabaseFromAnEarlierBuildUpgradesToTheCurrentSchema(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "upgrade-from-2026-09-08.sql"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	earlierPath := filepath.Join(dir, "earlier.db")
	earlier, err := sql.Open("sqlite", earlierPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := earlier.Exec(string(fixture)); err != nil {
		t.Fatal(err)
	}
	if err := earlier.Close(); err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("k", 32))
	upgraded, err := Open(earlierPath, key)
	if err != nil {
		t.Fatalf("an earlier build's database did not open: %v", err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	fresh, err := Open(filepath.Join(dir, "fresh.db"), key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	got, want := schemaOf(t, upgraded.db), schemaOf(t, fresh.db)
	for name, text := range want {
		if got[name] != text {
			t.Errorf("%s upgraded as\n  %s\nbut a new database has\n  %s", name, got[name], text)
		}
	}
	for name := range got {
		if _, found := want[name]; !found {
			t.Errorf("%s is left over from the earlier build", name)
		}
	}
}

// schemaOf is every table, index, trigger and view, by kind and name, with
// its SQL's whitespace folded.
func schemaOf(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `SELECT type, name, sql FROM sqlite_master WHERE sql IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	schema := map[string]string{}
	for rows.Next() {
		var kind, name, text string
		if err := rows.Scan(&kind, &name, &text); err != nil {
			t.Fatal(err)
		}
		schema[kind+" "+name] = strings.Join(strings.Fields(text), " ")
	}
	return schema
}
