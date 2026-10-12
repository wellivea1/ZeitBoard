package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openAccountActivityTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "account-activity.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, context.Background()
}

func activityMinute(value string) int64 {
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return at.Unix() / 60
}

var accountActivityImportedAt = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func takeoutInput(minutes ...int64) AccountActivityInput {
	return AccountActivityInput{Source: AccountActivityGoogleTakeout, Files: 2, Entries: len(minutes) + 1, Unusable: 1, Minutes: minutes}
}

// A second export overlapping the first adds only the minutes not kept, and
// each import says what it read.
func TestAccountActivityImportsKeepEachMinuteOnce(t *testing.T) {
	store, ctx := openAccountActivityTestStore(t)
	first := takeoutInput(activityMinute("2026-03-01T22:14:00Z"), activityMinute("2026-03-02T07:40:00Z"))
	imported, err := store.ImportAccountActivity(ctx, first, accountActivityImportedAt)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Added != 2 || imported.Minutes != 2 || !imported.FirstAt.Equal(time.Date(2026, 3, 1, 22, 14, 0, 0, time.UTC)) {
		t.Fatalf("first import %+v", imported)
	}
	second := takeoutInput(activityMinute("2026-03-02T07:40:00Z"), activityMinute("2026-03-02T23:05:00Z"))
	imported, err = store.ImportAccountActivity(ctx, second, accountActivityImportedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if imported.Added != 1 || imported.Minutes != 2 {
		t.Fatalf("second import %+v", imported)
	}

	minutes, err := store.AccountActivityMinutes(ctx)
	if err != nil || len(minutes) != 3 {
		t.Fatalf("minutes %v, %v", minutes, err)
	}
	if !minutes[2].Equal(time.Date(2026, 3, 2, 23, 5, 0, 0, time.UTC)) {
		t.Errorf("latest minute %s", minutes[2])
	}
	imports, err := store.AccountActivityImports(ctx)
	if err != nil || len(imports) != 2 {
		t.Fatalf("imports %+v, %v", imports, err)
	}
	if latest := imports[0]; latest.Added != 1 || latest.Files != 2 || latest.Entries != 3 || latest.Unusable != 1 ||
		!latest.ImportedAt.Equal(accountActivityImportedAt.Add(time.Hour)) {
		t.Errorf("latest import read back as %+v", latest)
	}
}

// Many minutes go in across several statements in one transaction.
func TestALargeExportIsKeptWhole(t *testing.T) {
	store, ctx := openAccountActivityTestStore(t)
	start := activityMinute("2025-01-01T00:00:00Z")
	minutes := make([]int64, 3*accountActivityBatch+7)
	for i := range minutes {
		minutes[i] = start + int64(i)*7
	}
	imported, err := store.ImportAccountActivity(ctx, takeoutInput(minutes...), accountActivityImportedAt)
	if err != nil || imported.Added != len(minutes) {
		t.Fatalf("import %+v, %v", imported, err)
	}
	kept, err := store.AccountActivityMinutes(ctx)
	if err != nil || len(kept) != len(minutes) {
		t.Fatalf("kept %d minutes, %v", len(kept), err)
	}
}

func TestAccountActivityThatCannotBeRightIsRefused(t *testing.T) {
	store, ctx := openAccountActivityTestStore(t)
	march := activityMinute("2026-03-01T22:14:00Z")
	for name, input := range map[string]AccountActivityInput{
		"no minutes":        takeoutInput(),
		"unsorted":          takeoutInput(march+5, march),
		"repeated":          takeoutInput(march, march),
		"after import":      takeoutInput(march, activityMinute("2026-03-12T00:00:00Z")),
		"another service":   {Source: "other_service", Files: 1, Entries: 1, Minutes: []int64{march}},
		"more than entries": {Source: AccountActivityGoogleTakeout, Files: 1, Entries: 1, Minutes: []int64{march, march + 1}},
		"no files":          {Source: AccountActivityGoogleTakeout, Entries: 1, Minutes: []int64{march}},
	} {
		if _, err := store.ImportAccountActivity(ctx, input, accountActivityImportedAt); err == nil {
			t.Errorf("%s: imported", name)
		}
	}
	minutes, err := store.AccountActivityMinutes(ctx)
	if err != nil || len(minutes) != 0 {
		t.Fatalf("a refused import kept %v, %v", minutes, err)
	}
}

// One action removes every minute and what each import read, and leaves no
// free pages holding them.
func TestRemovingAccountActivityLeavesNothing(t *testing.T) {
	store, ctx := openAccountActivityTestStore(t)
	start := activityMinute("2026-01-01T00:00:00Z")
	minutes := make([]int64, 2000)
	for i := range minutes {
		minutes[i] = start + int64(i)*11
	}
	if _, err := store.ImportAccountActivity(ctx, takeoutInput(minutes...), accountActivityImportedAt); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveAccountActivity(ctx); err != nil {
		t.Fatal(err)
	}
	kept, err := store.AccountActivityMinutes(ctx)
	if err != nil || len(kept) != 0 {
		t.Fatalf("minutes left: %d, %v", len(kept), err)
	}
	imports, err := store.AccountActivityImports(ctx)
	if err != nil || len(imports) != 0 {
		t.Fatalf("imports left: %+v, %v", imports, err)
	}
	var free int
	if err := store.db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&free); err != nil || free != 0 {
		t.Errorf("%d free pages after removal, %v", free, err)
	}
}
