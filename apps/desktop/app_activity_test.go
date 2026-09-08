package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"non24.app/core/domain"
	"non24.app/core/ingest"
	"non24.app/core/platform/activity"
	storage "non24.app/core/storage/sqlite"
)

type syntheticActivitySource struct{ samples atomic.Int32 }

func (*syntheticActivitySource) Capabilities() ingest.Capabilities {
	return ingest.Capabilities{ActiveIdle: true, SessionState: true}
}
func (s *syntheticActivitySource) Sample(now time.Time) (activity.Sample, error) {
	s.samples.Add(1)
	return activity.Sample{At: now, IdleKnown: true, LockedKnown: true}, nil
}

func waitActivityCount(t *testing.T, app *App, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := app.GetActivityCollection()
		if err == nil && got.RecordCount == count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	got, err := app.GetActivityCollection()
	t.Fatalf("activity count wanted %d, status %#v, error %v", count, got, err)
}

func TestActivityConsentDurabilityRestartAndErasure(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic-activity.db")
	source := &syntheticActivitySource{}
	// A repeated wall clock must not cause duplicate IDs after restart.
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	open := func() *App {
		s, err := storage.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		app := newAppWithStore(s, nil)
		app.activitySource = activity.SafeCollector{ZoneID: "UTC", Source: source, Now: func() time.Time { return now }}
		t.Cleanup(func() { app.stopActivityService(ctx); _ = s.Close() })
		return app
	}
	app := open()
	app.startActivityService(ctx)
	status, err := app.GetActivityCollection()
	if err != nil || status.Enabled || status.Running || source.samples.Load() != 0 {
		t.Fatalf("fresh install collected: %#v %v", status, err)
	}
	if _, err := app.SetActivityCollection(ActivityCollectionInput{Enabled: true, ZoneID: "UTC"}); err != nil {
		t.Fatal(err)
	}
	waitActivityCount(t, app, 1)
	app.startActivityService(ctx)
	if source.samples.Load() != 1 {
		t.Fatal("duplicate lifecycle start sampled again")
	}
	app.stopActivityService(ctx)
	waitActivityCount(t, app, 2)
	if stopped, err := app.GetActivityCollection(); err != nil || stopped.Running {
		t.Fatal("quit left collector running")
	}
	if _, err := app.SetActivityCollection(ActivityCollectionInput{Enabled: true, ZoneID: "UTC"}); err == nil {
		t.Fatal("enabled collection during quit")
	}
	if err := app.store.Close(); err != nil {
		t.Fatal(err)
	}

	app = open()
	app.startActivityService(ctx)
	waitActivityCount(t, app, 3)
	status, err = app.SetActivityCollection(ActivityCollectionInput{Enabled: false, ZoneID: "UTC"})
	if err != nil || status.Enabled || status.Running || status.RecordCount != 4 {
		t.Fatalf("disable failed: %#v %v", status, err)
	}
	app.stopActivityService(ctx)
	_ = app.store.Close()
	app = open()
	app.startActivityService(ctx)
	status, err = app.GetActivityCollection()
	if err != nil || status.Enabled || status.Running || source.samples.Load() != 2 {
		t.Fatalf("revoked consent revived: %#v %v", status, err)
	}

	previousDialog := saveActivityDataDialog
	t.Cleanup(func() { saveActivityDataDialog = previousDialog })
	exportPath := filepath.Join(t.TempDir(), "activity.json")
	saveActivityDataDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) { return exportPath, nil }
	exported, err := app.SaveActivityDataExport()
	if err != nil || !exported.Saved || exported.RecordCount != 4 {
		t.Fatalf("export: %#v %v", exported, err)
	}
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	var bundle struct {
		Observations []domain.SourceObservation `json:"observations"`
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatal(err)
	}
	seen := map[domain.ObservationID]bool{}
	for _, row := range bundle.Observations {
		if seen[row.ID] {
			t.Fatal("restart reused an activity identity")
		}
		seen[row.ID] = true
	}
	seedOneSleepEntry(t, app)
	if _, err := app.DeleteActivityData("wrong"); err == nil {
		t.Fatal("erase skipped confirmation")
	}
	status, err = app.DeleteActivityData("DELETE")
	if err != nil || status.RecordCount != 0 || status.Enabled {
		t.Fatalf("erase: %#v %v", status, err)
	}
	sleep, err := app.store.ListSleepObservations(ctx)
	if err != nil || len(sleep) != 1 {
		t.Fatalf("activity erasure affected sleep: %v", err)
	}
}

func TestActivityNoStoreCannotGrantConsent(t *testing.T) {
	app := newAppWithStore(nil, nil)
	source := &syntheticActivitySource{}
	app.activitySource = activity.SafeCollector{Source: source}
	app.startActivityService(context.Background())
	if _, err := app.SetActivityCollection(ActivityCollectionInput{Enabled: true, ZoneID: "UTC"}); err == nil {
		t.Fatal("granted without store")
	}
	if source.samples.Load() != 0 {
		t.Fatal("sampled without durable consent")
	}
}

func TestDesktopDataDirectoryCanIsolateQualificationProfile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "qualification")
	t.Setenv("ZEITBOARD_DATA_DIR", dir)
	store, err := openDesktopStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := os.Stat(filepath.Join(dir, desktopDatabaseFile)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZEITBOARD_DATA_DIR", "relative-path")
	if _, err := desktopDataDir(); err == nil {
		t.Fatal("accepted ambiguous qualification path")
	}
}
