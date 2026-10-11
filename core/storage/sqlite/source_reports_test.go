package sqlite

import (
	"strings"
	"testing"
	"time"

	"non24.app/core/sources"
)

func testSourceReport(id, outcome string, at time.Time) sources.Report {
	report := sources.Report{ReportID: id, Source: sources.HealthConnect, Outcome: outcome, ReportedAt: at, BackgroundReads: true}
	if outcome == sources.Read {
		held := 1
		report.Covered = &sources.Window{StartAt: at.Add(-30 * 24 * time.Hour), EndAt: at}
		report.Held = &held
	}
	return report
}

func TestSourceReportsArriveOnceAndReadAsAStatus(t *testing.T) {
	h := newMedicationSyncHarness(t)
	at := time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)
	read := testSourceReport("srcrep-read-01", sources.Read, at)
	missing := testSourceReport("srcrep-missing-01", sources.PermissionMissing, at.Add(6*time.Hour))
	if result := h.pull(
		SyncPullSourceReport{DeviceID: "device_phone_01", Report: read},
		SyncPullSourceReport{DeviceID: "device_phone_01", Report: missing},
	); result.Applied != 2 {
		t.Fatalf("downloaded reports = %+v", result)
	}
	if result := h.pull(SyncPullSourceReport{DeviceID: "device_phone_01", Report: read}); result.Skipped != 1 {
		t.Fatalf("a second copy = %+v", result)
	}
	// A report is kept as sent: a different copy under the same id is refused.
	altered := read
	altered.BackgroundReads = false
	h.cursor++
	if _, err := h.store.ApplySyncPullPage(h.ctx, SyncPullPage{Cursor: h.cursor, Records: []SyncPullRecord{
		SyncPullSourceReport{DeviceID: "device_phone_01", Report: altered},
	}}); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("an altered copy was accepted: %v", err)
	}

	received, err := h.store.SourceReports(h.ctx)
	h.must(err)
	statuses := sources.Statuses(received)
	if len(statuses) != 1 || statuses[0].Outcome != sources.PermissionMissing || statuses[0].DeviceID != "device_phone_01" ||
		!statuses[0].ReadThrough.Equal(at) || statuses[0].Held != 1 {
		t.Fatalf("statuses = %+v", statuses)
	}

	// The server erasing one removes it here, and it is not kept again.
	if result := h.pull(SyncPullTombstone{RecordID: missing.ReportID, RecordKind: SyncKindSourceReport}); result.TombstonesApplied != 1 {
		t.Fatalf("tombstone = %+v", result)
	}
	if result := h.pull(SyncPullSourceReport{DeviceID: "device_phone_01", Report: missing}); result.Applied != 0 {
		t.Fatalf("an erased report came back: %+v", result)
	}
	if got := h.erasures(); len(got) != 0 {
		t.Fatalf("a remote erasure was queued again: %v", got)
	}
}

func TestAReportThatIsNotOneIsRefused(t *testing.T) {
	h := newMedicationSyncHarness(t)
	at := time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)
	for name, record := range map[string]SyncPullSourceReport{
		"no sender":       {Report: testSourceReport("srcrep-bad-01", sources.Unavailable, at)},
		"invalid outcome": {DeviceID: "device_phone_01", Report: testSourceReport("srcrep-bad-02", "maybe", at)},
	} {
		t.Run(name, func(t *testing.T) {
			h.cursor++
			if _, err := h.store.ApplySyncPullPage(h.ctx, SyncPullPage{Cursor: h.cursor, Records: []SyncPullRecord{record}}); err == nil {
				t.Fatal("an invalid report was kept")
			}
		})
	}
}

func TestDeletingAllSleepTakesTheSourceReportsWithIt(t *testing.T) {
	h := newMedicationSyncHarness(t)
	report := testSourceReport("srcrep-read-01", sources.Read, time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC))
	h.pull(SyncPullSourceReport{DeviceID: "device_phone_01", Report: report})

	h.must(h.store.DeleteAllSleepData(h.ctx))
	received, err := h.store.SourceReports(h.ctx)
	h.must(err)
	if len(received) != 0 {
		t.Fatalf("reports left after deleting all sleep: %+v", received)
	}
	// The server is asked to erase it too, and a copy arriving before that
	// is not kept again.
	if got := h.erasures(); len(got) != 1 || got[0] != report.ReportID {
		t.Fatalf("queued erasures = %v", got)
	}
	if result := h.pull(SyncPullSourceReport{DeviceID: "device_phone_01", Report: report}); result.Applied != 0 {
		t.Fatalf("a deleted report came back: %+v", result)
	}
}
