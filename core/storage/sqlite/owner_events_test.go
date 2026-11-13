package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"non24.app/core/ownerevents"
)

var ownerEventDay = time.Date(2026, 10, 29, 19, 0, 0, 0, time.UTC)

func ownerEvent(revision int, title string, start time.Time, updated time.Time) ownerevents.Record {
	return ownerevents.Record{
		EventID:   "event_dentist_01",
		Title:     title,
		StartAt:   start,
		EndAt:     start.Add(time.Hour),
		ZoneID:    "America/New_York",
		Revision:  revision,
		CreatedAt: ownerEventDay.Add(-48 * time.Hour),
		UpdatedAt: updated,
	}
}

// mirrored is what the calendar every reader reads holds of the owner's
// events: titles by start.
func mirrored(t *testing.T, store *Store, ctx context.Context) map[time.Time]string {
	t.Helper()
	events, err := store.CalendarEvents(ctx, ownerEventDay.Add(-72*time.Hour), ownerEventDay.Add(72*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	found := map[time.Time]string{}
	for _, event := range events {
		if event.SourceID == OwnerEventsCalendarSourceID {
			if !event.Busy {
				t.Errorf("%s is not busy time", event.Title)
			}
			found[event.StartAt] = event.Title
		}
	}
	return found
}

func pendingEventRecords(t *testing.T, store *Store, ctx context.Context) []string {
	t.Helper()
	pending, err := store.PendingOwnerEventSyncRecords(ctx, MaxOwnerEventSyncPageSize)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, record := range pending {
		ids = append(ids, record.RecordID)
	}
	return ids
}

// An event the owner adds is busy time every reader sees, and each change is
// a revision to send.
func TestAnOwnerEventIsAFixedCommitmentAndSyncsAsRevisions(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	added := ownerEvent(1, "Synthetic dentist", ownerEventDay, ownerEventDay.Add(-48*time.Hour))
	if err := store.AddOwnerEvent(ctx, added); err != nil {
		t.Fatal(err)
	}
	if got := mirrored(t, store, ctx); got[ownerEventDay] != "Synthetic dentist" || len(got) != 1 {
		t.Fatalf("mirrored %v", got)
	}
	busy, _, err := store.BusyDomainEvents(ctx, ownerEventDay.Add(-time.Hour), ownerEventDay.Add(2*time.Hour), "UTC")
	if err != nil || len(busy) != 1 {
		t.Fatalf("the planner sees %d busy events, %v", len(busy), err)
	}
	if got := pendingEventRecords(t, store, ctx); len(got) != 1 || got[0] != "event_dentist_01_r1" {
		t.Fatalf("pending %v", got)
	}
	pending, _ := store.PendingOwnerEventSyncRecords(ctx, 1)
	if err := store.MarkOwnerEventSyncRecordsPushed(ctx, pending, ownerEventDay); err != nil {
		t.Fatal(err)
	}
	if got := pendingEventRecords(t, store, ctx); len(got) != 0 {
		t.Fatalf("still pending %v", got)
	}

	moved := ownerEvent(2, "Synthetic dentist, moved", ownerEventDay.Add(2*time.Hour), ownerEventDay.Add(-24*time.Hour))
	if err := store.ChangeOwnerEvent(ctx, moved); err != nil {
		t.Fatal(err)
	}
	if got := mirrored(t, store, ctx); len(got) != 1 || got[ownerEventDay.Add(2*time.Hour)] != "Synthetic dentist, moved" {
		t.Fatalf("after the change the calendar holds %v", got)
	}
	if got := pendingEventRecords(t, store, ctx); len(got) != 1 || got[0] != "event_dentist_01_r2" {
		t.Fatalf("pending after the change %v", got)
	}
	stale := ownerEvent(2, "A change from an old view", ownerEventDay, ownerEventDay)
	if err := store.ChangeOwnerEvent(ctx, stale); !errors.Is(err, ErrOwnerEventChanged) {
		t.Fatalf("a change from an old view: %v", err)
	}
}

// Deleting an event erases every revision the server may hold, and the event
// does not come back.
func TestDeletingAnOwnerEventErasesItEverywhere(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	if err := store.AddOwnerEvent(ctx, ownerEvent(1, "Synthetic dentist", ownerEventDay, ownerEventDay.Add(-48*time.Hour))); err != nil {
		t.Fatal(err)
	}
	pending, _ := store.PendingOwnerEventSyncRecords(ctx, 1)
	if err := store.MarkOwnerEventSyncRecordsPushed(ctx, pending, ownerEventDay); err != nil {
		t.Fatal(err)
	}
	if err := store.ChangeOwnerEvent(ctx, ownerEvent(2, "Synthetic dentist, moved", ownerEventDay, ownerEventDay.Add(-24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteOwnerEvent(ctx, "event_dentist_01", 1); !errors.Is(err, ErrOwnerEventChanged) {
		t.Fatalf("deleting from an old view: %v", err)
	}
	if err := store.DeleteOwnerEvent(ctx, "event_dentist_01", 2); err != nil {
		t.Fatal(err)
	}
	erasures, err := store.PendingSyncErasures(ctx)
	if err != nil || len(erasures) != 2 {
		t.Fatalf("erasures %v, %v", erasures, err)
	}
	if _, err := store.OwnerEvent(ctx, "event_dentist_01"); !errors.Is(err, ErrOwnerEventNotFound) {
		t.Fatalf("read after deletion: %v", err)
	}
	if got := mirrored(t, store, ctx); len(got) != 0 {
		t.Fatalf("the calendar still holds %v", got)
	}
	if err := store.AddOwnerEvent(ctx, ownerEvent(1, "Synthetic dentist", ownerEventDay, ownerEventDay)); err == nil {
		t.Fatal("an erased event came back")
	}
}

func pullOwnerEvents(t *testing.T, store *Store, ctx context.Context, cursor int64, records ...SyncPullRecord) SyncPullPageResult {
	t.Helper()
	result, err := store.ApplySyncPullPage(ctx, SyncPullPage{Cursor: cursor, Records: records})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// Every device keeps the same revision: the higher one against what it sent,
// and the later edit against what it has not.
func TestEveryDeviceKeepsTheSameOwnerEvent(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	first := ownerEvent(1, "Synthetic dentist", ownerEventDay, ownerEventDay.Add(-48*time.Hour))
	if result := pullOwnerEvents(t, store, ctx, 1, SyncPullOwnerEvent{Event: first}); result.Applied != 1 {
		t.Fatalf("an unknown event was not taken: %+v", result)
	}
	if got := mirrored(t, store, ctx); got[ownerEventDay] != "Synthetic dentist" {
		t.Fatalf("mirrored %v", got)
	}
	if got := pendingEventRecords(t, store, ctx); len(got) != 0 {
		t.Fatalf("a received revision is sent back: %v", got)
	}

	// A higher revision replaces what this device holds and sent.
	second := ownerEvent(2, "Synthetic dentist, moved", ownerEventDay.Add(time.Hour), ownerEventDay.Add(-24*time.Hour))
	pullOwnerEvents(t, store, ctx, 2, SyncPullOwnerEvent{Event: second})
	if held, _ := store.OwnerEvent(ctx, "event_dentist_01"); held.Revision != 2 || held.Title != second.Title {
		t.Fatalf("held %+v", held)
	}
	pullOwnerEvents(t, store, ctx, 3, SyncPullOwnerEvent{Event: first})
	if held, _ := store.OwnerEvent(ctx, "event_dentist_01"); held.Revision != 2 {
		t.Fatalf("an older revision replaced a newer one: %+v", held)
	}

	// An edit here, not yet sent, that is later than another device's revision
	// of the same number is kept, renumbered past it.
	mine := ownerEvent(3, "Synthetic dentist, mine", ownerEventDay.Add(3*time.Hour), ownerEventDay.Add(-time.Hour))
	if err := store.ChangeOwnerEvent(ctx, mine); err != nil {
		t.Fatal(err)
	}
	theirs := ownerEvent(3, "Synthetic dentist, theirs", ownerEventDay.Add(4*time.Hour), ownerEventDay.Add(-2*time.Hour))
	pullOwnerEvents(t, store, ctx, 4, SyncPullOwnerEvent{Event: theirs})
	if held, _ := store.OwnerEvent(ctx, "event_dentist_01"); held.Revision != 4 || held.Title != mine.Title {
		t.Fatalf("the later local edit was lost: %+v", held)
	}
	if got := pendingEventRecords(t, store, ctx); len(got) != 1 || got[0] != "event_dentist_01_r4" {
		t.Fatalf("the renumbered edit is not pending: %v", got)
	}

	// One made later elsewhere replaces an earlier one not yet sent.
	later := ownerEvent(5, "Synthetic dentist, later", ownerEventDay.Add(5*time.Hour), ownerEventDay)
	pullOwnerEvents(t, store, ctx, 5, SyncPullOwnerEvent{Event: later})
	if held, _ := store.OwnerEvent(ctx, "event_dentist_01"); held.Revision != 5 || held.Title != later.Title {
		t.Fatalf("a later edit elsewhere did not win: %+v", held)
	}
	if got := mirrored(t, store, ctx); len(got) != 1 || got[ownerEventDay.Add(5*time.Hour)] != later.Title {
		t.Fatalf("mirrored %v", got)
	}

	// Erased elsewhere, it goes here and stays gone.
	result := pullOwnerEvents(t, store, ctx, 6, SyncPullTombstone{RecordID: "event_dentist_01_r5", RecordKind: SyncKindOwnerEvent})
	if result.TombstonesApplied != 1 {
		t.Fatalf("tombstone %+v", result)
	}
	pullOwnerEvents(t, store, ctx, 7, SyncPullOwnerEvent{Event: ownerEvent(6, "From an offline device", ownerEventDay, ownerEventDay)})
	if _, err := store.OwnerEvent(ctx, "event_dentist_01"); !errors.Is(err, ErrOwnerEventNotFound) {
		t.Fatalf("an erased event came back: %v", err)
	}
	if got := mirrored(t, store, ctx); len(got) != 0 {
		t.Fatalf("the calendar still holds %v", got)
	}
	if erasures, _ := store.PendingSyncErasures(ctx); len(erasures) != 0 {
		t.Fatalf("an erasure from elsewhere is sent back: %v", erasures)
	}
}
