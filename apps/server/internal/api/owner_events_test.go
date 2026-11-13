package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	syncmodel "non24.app/server/internal/sync"
)

func eventRevisionPayload(eventID string, revision int, title, endAt string) string {
	return fmt.Sprintf(
		`{"event_id":%q,"title":%q,"start_at":"2026-10-29T19:00:00Z","end_at":%q,"zone_id":"America/New_York","revision":%d,"created_at":"2026-10-27T12:00:00Z","updated_at":"2026-10-27T12:%02d:00Z"}`,
		eventID, title, endAt, revision, revision)
}

func eventRevisionRecord(eventID string, revision int, title string) string {
	return syncRecord(fmt.Sprintf("%s_r%d", eventID, revision), "event",
		eventRevisionPayload(eventID, revision, title, "2026-10-29T20:00:00Z"))
}

// pulledEvents reads one device's pull: the event revisions it holds, and how
// many tombstones name an event.
func pulledEvents(t *testing.T, h *testHarness, token string) ([]string, int) {
	t.Helper()
	status, body := h.request(t, http.MethodGet, "/v1/sync/pull?since=0", token, "")
	if status != http.StatusOK {
		t.Fatalf("pull status = %d body = %s", status, body)
	}
	var pull syncmodel.PullResponse
	if err := json.Unmarshal(body, &pull); err != nil {
		t.Fatal(err)
	}
	var revisions []string
	tombstones := 0
	for _, envelope := range pull.Records {
		switch envelope.Kind {
		case syncmodel.KindEvent:
			revisions = append(revisions, envelope.RecordID)
		case syncmodel.KindTombstone:
			var payload syncmodel.TombstonePayload
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.RecordKind == syncmodel.KindEvent {
				tombstones++
			}
		}
	}
	return revisions, tombstones
}

// An event the owner adds syncs as revisions every device can pull (ADR-0068),
// and erasing it erases every revision and refuses its return, as a task's.
func TestEventRevisionSyncLifecycle(t *testing.T) {
	h := newTestHarness(t)
	desktop := h.registerDevice(t, "desktop")
	phone := h.registerDevice(t, "phone")

	h.pushRecords(t, desktop, eventRevisionRecord("event_dentist_01", 1, "Synthetic dentist"))
	h.pushRecords(t, desktop, eventRevisionRecord("event_dentist_01", 2, "Synthetic dentist, moved"))
	if revisions, _ := pulledEvents(t, h, phone); len(revisions) != 2 ||
		revisions[0] != "event_dentist_01_r1" || revisions[1] != "event_dentist_01_r2" {
		t.Fatalf("the phone pulled %v", revisions)
	}

	for name, record := range map[string]string{
		"a record id naming another revision": syncRecord("event_dentist_01_r9", "event",
			eventRevisionPayload("event_dentist_01", 3, "Synthetic dentist", "2026-10-29T20:00:00Z")),
		"an event over a day": syncRecord("event_dentist_02_r1", "event",
			eventRevisionPayload("event_dentist_02", 1, "Synthetic dentist", "2026-10-31T19:00:00Z")),
		"an event ending as it starts": syncRecord("event_dentist_03_r1", "event",
			eventRevisionPayload("event_dentist_03", 1, "Synthetic dentist", "2026-10-29T19:00:00Z")),
		"a title with space around it": eventRevisionRecord("event_dentist_04", 1, " Synthetic dentist"),
	} {
		if status, _ := h.request(t, http.MethodPost, "/v1/sync/push", desktop, syncBatch(record)); status != http.StatusBadRequest {
			t.Errorf("%s was accepted: %d", name, status)
		}
	}

	status, body := h.request(t, http.MethodPost, "/v1/sync/erase", phone, `{"schema_version":"v1","record_ids":["event_dentist_01_r1"]}`)
	if status != http.StatusOK {
		t.Fatalf("erase status = %d body = %s", status, body)
	}
	var erase syncmodel.EraseResponse
	if err := json.Unmarshal(body, &erase); err != nil {
		t.Fatal(err)
	}
	if erase.Erased != 2 || erase.Tombstones != 2 {
		t.Fatalf("erase = %+v, want both revisions erased and tombstoned", erase)
	}
	// A device offline through the erasure pushes a later revision; it is
	// taken without complaint and kept nowhere.
	h.pushRecords(t, desktop, eventRevisionRecord("event_dentist_01", 3, "Offline stale edit"))
	revisions, tombstones := pulledEvents(t, h, desktop)
	if len(revisions) != 0 || tombstones != 2 {
		t.Fatalf("after erasure the desktop pulled %v and %d event tombstones", revisions, tombstones)
	}
}
