package sqlite

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Account activity (ADR-0059): the minutes in which an online account was
// used, read from an export the person chose. Only the minutes are kept, with
// counts of what each import read. They are never synced, shared or given to
// an assistant, and RemoveAccountActivity erases every one of them.

// AccountActivityGoogleTakeout is Google's My Activity, from a Takeout export.
const AccountActivityGoogleTakeout = "google_takeout"

// accountActivityBatch is how many minutes one INSERT carries, well inside
// SQLite's limit on bound values.
const accountActivityBatch = 500

// AccountActivityInput is one export as read: its minutes and counts of what
// was read, never anything that was in it.
type AccountActivityInput struct {
	Source   string
	Files    int
	Entries  int
	Unusable int

	// Minutes are the minutes with activity, as minutes since the Unix epoch,
	// sorted and without repeats.
	Minutes []int64
}

// AccountActivityImport is what one import read.
type AccountActivityImport struct {
	Source     string
	ImportedAt time.Time
	Files      int
	Entries    int
	Unusable   int

	// Minutes are the export's minutes with activity, and Added those among
	// them not already kept from an earlier import.
	Minutes int
	Added   int

	FirstAt time.Time
	LastAt  time.Time
}

func (s *Store) initializeAccountActivity(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS local_account_activity_imports (
			import_id INTEGER PRIMARY KEY,
			source TEXT NOT NULL CHECK(source IN ('google_takeout')),
			imported_at TEXT NOT NULL,
			files INTEGER NOT NULL CHECK(files >= 1),
			entries INTEGER NOT NULL CHECK(entries >= 0),
			unusable INTEGER NOT NULL CHECK(unusable >= 0),
			minutes INTEGER NOT NULL CHECK(minutes >= 1),
			added INTEGER NOT NULL CHECK(added >= 0 AND added <= minutes),
			first_at TEXT NOT NULL,
			last_at TEXT NOT NULL,
			CHECK(last_at >= first_at)
		)`,
		// A minute is its own rowid, so years of activity cost a few bytes a
		// minute and read back in order without an index.
		`CREATE TABLE IF NOT EXISTS local_account_activity_minutes (
			at_minute INTEGER PRIMARY KEY
		)`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate account activity: %w", err)
		}
	}
	return nil
}

// ImportAccountActivity keeps an export's minutes beside any kept before, and
// records what the import read. Minutes already kept are not counted twice.
func (s *Store) ImportAccountActivity(ctx context.Context, input AccountActivityInput, importedAt time.Time) (AccountActivityImport, error) {
	if err := validateAccountActivity(input, importedAt); err != nil {
		return AccountActivityImport{}, err
	}
	result := AccountActivityImport{
		Source: input.Source, ImportedAt: importedAt.UTC(),
		Files: input.Files, Entries: input.Entries, Unusable: input.Unusable,
		Minutes: len(input.Minutes),
		FirstAt: minuteTime(input.Minutes[0]),
		LastAt:  minuteTime(input.Minutes[len(input.Minutes)-1]),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountActivityImport{}, err
	}
	defer tx.Rollback()
	for batch := range slices.Chunk(input.Minutes, accountActivityBatch) {
		values := make([]any, len(batch))
		for i, minute := range batch {
			values[i] = minute
		}
		inserted, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO local_account_activity_minutes(at_minute) VALUES `+
			strings.TrimSuffix(strings.Repeat("(?),", len(batch)), ","), values...)
		if err != nil {
			return AccountActivityImport{}, err
		}
		added, err := inserted.RowsAffected()
		if err != nil {
			return AccountActivityImport{}, err
		}
		result.Added += int(added)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_account_activity_imports(
			source, imported_at, files, entries, unusable, minutes, added, first_at, last_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.Source, formatSQLiteTime(result.ImportedAt), result.Files, result.Entries, result.Unusable,
		result.Minutes, result.Added, formatSQLiteTime(result.FirstAt), formatSQLiteTime(result.LastAt)); err != nil {
		return AccountActivityImport{}, err
	}
	return result, tx.Commit()
}

func validateAccountActivity(input AccountActivityInput, importedAt time.Time) error {
	if input.Source != AccountActivityGoogleTakeout {
		return fmt.Errorf("account activity from %q is not read", input.Source)
	}
	if input.Files < 1 || input.Entries < len(input.Minutes) || input.Unusable < 0 || input.Unusable > input.Entries {
		return errors.New("account activity counts do not add up")
	}
	if len(input.Minutes) == 0 {
		return errors.New("the export has no activity with a usable time")
	}
	if importedAt.IsZero() {
		return errors.New("account activity needs the time it was imported")
	}
	latest := importedAt.Add(24*time.Hour).Unix() / 60
	for i, minute := range input.Minutes {
		if i > 0 && minute <= input.Minutes[i-1] {
			return errors.New("account activity minutes must be sorted without repeats")
		}
		if minute < 0 || minute > latest {
			return errors.New("account activity has a minute after it was imported")
		}
	}
	return nil
}

// AccountActivityImports lists what each import read, the latest first.
func (s *Store) AccountActivityImports(ctx context.Context) ([]AccountActivityImport, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source, imported_at, files, entries, unusable, minutes, added, first_at, last_at
		FROM local_account_activity_imports ORDER BY import_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var imports []AccountActivityImport
	for rows.Next() {
		var item AccountActivityImport
		var importedAt, firstAt, lastAt string
		if err := rows.Scan(&item.Source, &importedAt, &item.Files, &item.Entries, &item.Unusable,
			&item.Minutes, &item.Added, &firstAt, &lastAt); err != nil {
			return nil, err
		}
		for _, field := range []struct {
			text  string
			value *time.Time
		}{{importedAt, &item.ImportedAt}, {firstAt, &item.FirstAt}, {lastAt, &item.LastAt}} {
			if *field.value, err = time.Parse(time.RFC3339Nano, field.text); err != nil {
				return nil, fmt.Errorf("account activity import: %w", err)
			}
		}
		imports = append(imports, item)
	}
	return imports, rows.Err()
}

// AccountActivityMinutes reads every kept minute with activity, in order.
func (s *Store) AccountActivityMinutes(ctx context.Context) ([]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT at_minute FROM local_account_activity_minutes ORDER BY at_minute`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var minutes []time.Time
	for rows.Next() {
		var minute int64
		if err := rows.Scan(&minute); err != nil {
			return nil, err
		}
		minutes = append(minutes, minuteTime(minute))
	}
	return minutes, rows.Err()
}

// RemoveAccountActivity erases every imported minute and what the imports
// read, and compacts the database so the deleted pages are gone too.
func (s *Store) RemoveAccountActivity(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"local_account_activity_minutes", "local_account_activity_imports"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.compactDeletedData(ctx)
}

func minuteTime(minute int64) time.Time { return time.Unix(minute*60, 0).UTC() }
