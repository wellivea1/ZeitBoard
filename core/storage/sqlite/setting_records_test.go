package sqlite

import (
	"testing"
	"time"

	"non24.app/core/sharedsetting"
)

// ADR-0074: a choice is kept as made, waits to upload, and the newest one
// made on any device stands. One the server erases is gone here and is not
// kept again.
func TestChoicesAreKeptUploadedAndTheNewestStands(t *testing.T) {
	h := newMedicationSyncHarness(t)
	at := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	here := sharedsetting.Record{SettingID: "setting_here_01", Name: sharedsetting.Clock, Value: sharedsetting.Word(sharedsetting.Clock12), ChangedAt: at}
	h.must(h.store.AddSettingRecord(h.ctx, here))

	pending, err := h.store.PendingSettingSyncRecords(h.ctx, MaxSettingSyncPageSize)
	h.must(err)
	if len(pending) != 1 || pending[0].RecordID != here.SettingID || !pending[0].CreatedAt.Equal(at) {
		t.Fatalf("pending = %+v", pending)
	}
	h.must(h.store.MarkSettingSyncRecordsPushed(h.ctx, pending, at.Add(time.Minute)))
	if again, err := h.store.PendingSettingSyncRecords(h.ctx, MaxSettingSyncPageSize); err != nil || len(again) != 0 {
		t.Fatalf("an uploaded choice still waits: %+v, %v", again, err)
	}

	phone := sharedsetting.Record{SettingID: "setting-phone-01", Name: sharedsetting.Clock, Value: sharedsetting.Word(sharedsetting.Clock24), ChangedAt: at.Add(time.Hour)}
	share := sharedsetting.Record{SettingID: "setting-phone-02", Name: sharedsetting.ShareStatusByDefault, Value: sharedsetting.On(true), ChangedAt: at}
	if result := h.pull(SyncPullSetting{Record: phone}, SyncPullSetting{Record: share}); result.Applied != 2 {
		t.Fatalf("downloaded choices = %+v", result)
	}
	current, err := h.store.CurrentSettings(h.ctx)
	h.must(err)
	if current[sharedsetting.Clock].SettingID != phone.SettingID || current[sharedsetting.ShareStatusByDefault].SettingID != share.SettingID {
		t.Fatalf("current = %+v", current)
	}
	// A downloaded choice is not sent back.
	if again, err := h.store.PendingSettingSyncRecords(h.ctx, MaxSettingSyncPageSize); err != nil || len(again) != 0 {
		t.Fatalf("a downloaded choice waits to upload: %+v, %v", again, err)
	}

	// The server erasing the phone's choice puts this computer's back.
	if result := h.pull(SyncPullTombstone{RecordID: phone.SettingID, RecordKind: SyncKindSetting}); result.TombstonesApplied != 1 {
		t.Fatalf("tombstone = %+v", result)
	}
	if result := h.pull(SyncPullSetting{Record: phone}); result.Applied != 0 {
		t.Fatalf("an erased choice came back: %+v", result)
	}
	current, err = h.store.CurrentSettings(h.ctx)
	h.must(err)
	if current[sharedsetting.Clock].SettingID != here.SettingID {
		t.Fatalf("after the erasure the clock is %+v", current[sharedsetting.Clock])
	}

	// A choice outside the contract is refused before it is kept.
	if err := h.store.AddSettingRecord(h.ctx, sharedsetting.Record{SettingID: "setting_bad_01", Name: "theme", Value: sharedsetting.Word("dark"), ChangedAt: at}); err == nil {
		t.Fatal("a choice that stays on its device was kept for the others")
	}
	// Erasing everything here takes the choices too.
	h.must(h.store.DeleteAll(h.ctx))
	if current, err := h.store.CurrentSettings(h.ctx); err != nil || len(current) != 0 {
		t.Fatalf("after erasing everything: %+v, %v", current, err)
	}
}
