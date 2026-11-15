package main

import (
	"context"
	"testing"
	"time"

	"non24.app/core/presence"
	storage "non24.app/core/storage/sqlite"
)

func TestAReportedStatusStandsUntilItLapsesOrIsTakenBack(t *testing.T) {
	app := newTestApp(t)
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return now }

	if status, err := app.GetReportedStatus(); err != nil || status.Reported != "" || status.Status != "ok" {
		t.Fatalf("before any report: %+v, %v", status, err)
	}
	reported, err := app.ReportStatus(ReportStatusInput{Status: presence.TryingToSleep})
	if err != nil {
		t.Fatal(err)
	}
	if reported.Reported != presence.TryingToSleep || reported.ReportedLabel != "Trying to sleep" {
		t.Fatalf("reported = %+v", reported)
	}
	// Kept to the owner's own devices unless they choose otherwise.
	if reported.Shared {
		t.Error("a report was shared with links by default")
	}
	if reported.Delivery != "this_computer" {
		t.Errorf("delivery = %q with sync off", reported.Delivery)
	}

	// It lapses by itself at the time it was set to.
	now = now.Add(presence.DefaultLifetime(presence.TryingToSleep))
	if status, _ := app.GetReportedStatus(); status.Reported != "" {
		t.Fatalf("a lapsed status still stands: %+v", status)
	}

	// A new report replaces the last, and can be taken back.
	if _, err := app.ReportStatus(ReportStatusInput{Status: presence.DoNotDisturb, Minutes: 30}); err != nil {
		t.Fatal(err)
	}
	if status, _ := app.GetReportedStatus(); status.Reported != presence.DoNotDisturb {
		t.Fatalf("status = %+v", status)
	}
	cleared, err := app.ClearReportedStatus()
	if err != nil || cleared.Reported != "" {
		t.Fatalf("after clearing: %+v, %v", cleared, err)
	}

	for _, bad := range []ReportStatusInput{
		{Status: "asleep"},
		{Status: presence.Cleared},
		{Status: presence.Awake, Minutes: 2},
		{Status: presence.Awake, Minutes: 25 * 60},
	} {
		if _, err := app.ReportStatus(bad); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
}

func TestSharingAReportIsTheOwnersChoiceEachTimeAndByDefault(t *testing.T) {
	app := newTestApp(t)
	yes, no := true, false

	// One report shared, against the default.
	shared, err := app.ReportStatus(ReportStatusInput{Status: presence.Awake, Shared: &yes})
	if err != nil || !shared.Shared {
		t.Fatalf("an explicitly shared report: %+v, %v", shared, err)
	}
	// The default changed, and one report kept private against it.
	saved, err := app.SaveStatusSettings(StatusSettingsInput{BaseRevision: shared.SettingsRevision, ShareByDefault: true})
	if err != nil || !saved.Settings.ShareByDefault {
		t.Fatalf("settings = %+v, %v", saved.Settings, err)
	}
	if next, _ := app.ReportStatus(ReportStatusInput{Status: presence.Awake}); !next.Shared {
		t.Error("a report did not follow the owner's default")
	}
	if private, _ := app.ReportStatus(ReportStatusInput{Status: presence.DoNotDisturb, Shared: &no}); private.Shared {
		t.Error("a report kept private was shared")
	}
	// A stale save loses to what is stored.
	if stale, _ := app.SaveStatusSettings(StatusSettingsInput{BaseRevision: 0, ShareByDefault: false}); !stale.Settings.ShareByDefault {
		t.Error("a save from a stale revision overwrote the stored choice")
	}
}

func TestSleepTapsReportAStatusOnlyWhenTheOwnerAskedForIt(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.BeginQuickSleep(); err != nil {
		t.Fatal(err)
	}
	if status, _ := app.GetReportedStatus(); status.Reported != "" {
		t.Fatalf("a tap reported %q without being asked to", status.Reported)
	}

	current, _ := app.GetReportedStatus()
	if _, err := app.SaveStatusSettings(StatusSettingsInput{BaseRevision: current.SettingsRevision, FollowSleepTaps: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.BeginQuickSleep(); err != nil {
		t.Fatal(err)
	}
	status, _ := app.GetReportedStatus()
	if status.Reported != presence.TryingToSleep {
		t.Fatalf("with the choice on, the tap reported %q", status.Reported)
	}
	// The tap still records no sleep: trying to sleep is an intention.
	entries, err := app.GetSleepLogPage(SleepLogPageInput{Page: 0})
	if err != nil {
		t.Fatal(err)
	}
	if entries.Total != 0 {
		t.Fatalf("a going-to-sleep tap recorded %d nights", entries.Total)
	}
}

func TestReportsAreUploadedAndErasedWithTheSleepData(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	if _, err := app.ReportStatus(ReportStatusInput{Status: presence.Awake}); err != nil {
		t.Fatal(err)
	}
	pending, err := app.store.PendingStatusSyncRecords(ctx, storage.MaxStatusSyncPageSize)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %d, %v; want the one report waiting to upload", len(pending), err)
	}
	if err := app.store.MarkStatusSyncRecordsPushed(ctx, pending, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if again, _ := app.store.PendingStatusSyncRecords(ctx, storage.MaxStatusSyncPageSize); len(again) != 0 {
		t.Fatalf("an acknowledged report is still pending: %d", len(again))
	}

	if err := app.store.DeleteAllSleepData(ctx); err != nil {
		t.Fatal(err)
	}
	if reports, _ := app.store.StatusReports(ctx); len(reports) != 0 {
		t.Fatalf("erasing everything left %d status reports", len(reports))
	}
	// The uploaded copy is queued for erasure on the server.
	erasures, err := app.store.PendingSyncErasures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	queued := false
	for _, id := range erasures {
		if id == pending[0].RecordID {
			queued = true
		}
	}
	if !queued {
		t.Errorf("the uploaded report %s was not queued for erasure: %v", pending[0].RecordID, erasures)
	}
}

func TestOldReportsAreLetGo(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return now }
	if _, err := app.ReportStatus(ReportStatusInput{Status: presence.Awake}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(8 * 24 * time.Hour)
	if _, err := app.ReportStatus(ReportStatusInput{Status: presence.DoNotDisturb}); err != nil {
		t.Fatal(err)
	}
	reports, err := app.store.StatusReports(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Status != presence.DoNotDisturb {
		t.Fatalf("reports = %+v; a week-old report should be gone", reports)
	}
}
