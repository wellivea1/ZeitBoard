package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"non24.app/core/civilfmt"
	"non24.app/core/sharedsetting"
	storage "non24.app/core/storage/sqlite"
)

// ADR-0074: a choice made on this computer is kept for the owner's other
// devices; a newer one made on another device is followed here, and an older
// one is not.
func TestSharingAndTheClockFollowTheNewestChoiceOnAnyDevice(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return at }

	status, err := app.GetReportedStatus()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveStatusSettings(StatusSettingsInput{BaseRevision: status.SettingsRevision, ShareByDefault: true}); err != nil {
		t.Fatal(err)
	}
	clock, err := app.GetClockSetting()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveClockSetting(ClockSettingInput{Preference: sharedsetting.Clock24, BaseRevision: clock.Revision}); err != nil {
		t.Fatal(err)
	}
	pending, err := app.store.PendingSettingSyncRecords(ctx, storage.MaxSettingSyncPageSize)
	if err != nil {
		t.Fatal(err)
	}
	// Two choices changed; the one left as it was is not sent.
	if len(pending) != 2 {
		t.Fatalf("%d choices wait to upload, want sharing and the clock", len(pending))
	}

	// The phone turns sharing off later, and its clock change is older than
	// this computer's.
	pull(t, app,
		sharedsetting.Record{SettingID: "setting-phone-share", Name: sharedsetting.ShareStatusByDefault, Value: sharedsetting.On(false), ChangedAt: at.Add(time.Minute)},
		sharedsetting.Record{SettingID: "setting-phone-clock", Name: sharedsetting.Clock, Value: sharedsetting.Word(sharedsetting.Clock12), ChangedAt: at.Add(-time.Hour)},
	)
	changed, err := app.followSharedSettings(ctx)
	if err != nil || !changed {
		t.Fatalf("followed: %v, %v", changed, err)
	}
	if status, _ := app.GetReportedStatus(); status.Settings.ShareByDefault {
		t.Error("sharing is still on after the phone turned it off later")
	}
	if clock, _ := app.GetClockSetting(); clock.Preference != sharedsetting.Clock24 || displayClock() != civilfmt.TwentyFourHour {
		t.Errorf("an older clock from the phone replaced the newer one here: %+v", clock)
	}
	// Following again changes nothing.
	if changed, _ := app.followSharedSettings(ctx); changed {
		t.Error("following the same choices changed something")
	}

	// Choosing again here dates the choice after the phone's, even with this
	// computer's clock an hour behind.
	app.nowFn = func() time.Time { return at.Add(-time.Hour) }
	status, _ = app.GetReportedStatus()
	if _, err := app.SaveStatusSettings(StatusSettingsInput{BaseRevision: status.SettingsRevision, ShareByDefault: true}); err != nil {
		t.Fatal(err)
	}
	current, err := app.store.CurrentSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if on, _ := current[sharedsetting.ShareStatusByDefault].Value.Bool(); !on {
		t.Fatal("the choice made here does not stand")
	}
	if !current[sharedsetting.ShareStatusByDefault].ChangedAt.After(at.Add(time.Minute)) {
		t.Errorf("the choice made here is dated %s, before the phone's", current[sharedsetting.ShareStatusByDefault].ChangedAt)
	}
}

// A choice that already stands is not recorded again, so saving the same
// screen twice sends nothing new.
func TestAChoiceAlreadyStandingIsNotRecordedAgain(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	for range 2 {
		clock, _ := app.GetClockSetting()
		if _, err := app.SaveClockSetting(ClockSettingInput{Preference: sharedsetting.Clock12, BaseRevision: clock.Revision}); err != nil {
			t.Fatal(err)
		}
	}
	pending, err := app.store.PendingSettingSyncRecords(ctx, storage.MaxSettingSyncPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("%d records for one choice made twice", len(pending))
	}
	var sent map[string]any
	if err := json.Unmarshal(pending[0].Payload, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["name"] != "clock" || sent["value"] != "12" {
		t.Fatalf("sent %v", sent)
	}
}

func pull(t *testing.T, app *App, records ...sharedsetting.Record) {
	t.Helper()
	page := syncPullResponse{SchemaVersion: "v1", Cursor: int64(len(records))}
	for index, record := range records {
		payload, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		page.Records = append(page.Records, syncEnvelope{
			Seq: int64(index + 1), RecordID: record.SettingID, Kind: storage.SyncKindSetting,
			DeviceID: "device_0a1b2c3d", CreatedAt: record.ChangedAt, Payload: payload,
		})
	}
	if err := validatePullEnvelopePage(page, 0); err != nil {
		t.Fatalf("a page of settings was refused: %v", err)
	}
	var decoded []storage.SyncPullRecord
	for _, envelope := range page.Records {
		record, err := decodePulledRecord(envelope)
		if err != nil {
			t.Fatal(err)
		}
		decoded = append(decoded, record)
	}
	if _, err := app.store.ApplySyncPullPage(context.Background(), storage.SyncPullPage{Cursor: page.Cursor, Records: decoded}); err != nil {
		t.Fatal(err)
	}
}
