package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	calendarcore "non24.app/core/calendar"
)

const testCollection = "https://calendar.example/dav/owner/zeitboard/"

func writeBackStore(t *testing.T) (*Store, context.Context, time.Time) {
	t.Helper()
	store, ctx := openCalendarTestStore(t)
	source := testCalendarSource(calendarcore.SourceCalDAV)
	if err := store.ReplaceImportedCalendar(ctx, source, nil, testCollection); err != nil {
		t.Fatal(err)
	}
	return store, ctx, time.Date(2026, 9, 27, 18, 0, 0, 0, time.UTC)
}

// acceptTime stands in for an accepted suggestion: an app-owned block.
func acceptTime(t *testing.T, store *Store, id string, start time.Time) {
	t.Helper()
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := ensureZeitBoardSource(context.Background(), tx, start.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := insertCalendarEvent(context.Background(), tx, calendarcore.Event{
		EventID: id, SourceID: ZeitBoardCalendarSourceID, SourceRecordID: "proposal_" + id,
		Title: "Accepted " + id, StartAt: start, EndAt: start.Add(30 * time.Minute), ZoneID: "America/New_York",
		Busy: true, Ownership: calendarcore.OwnershipAppOwned, CreatedAt: start.Add(-time.Hour),
		TaskID: "task_" + id, TaskRevision: 1, ProposalID: "proposal_" + id,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func undoTime(t *testing.T, store *Store, id string) {
	t.Helper()
	if _, err := store.db.Exec(`DELETE FROM local_calendar_events WHERE event_id = ?`, id); err != nil {
		t.Fatal(err)
	}
}

// targetOf is the calendar written to, as the writer reads it.
func targetOf(t *testing.T, store *Store) CalendarWriteTarget {
	t.Helper()
	target, on, err := store.CalendarWriteTarget(context.Background())
	if err != nil || !on {
		t.Fatalf("writing is not on: %v", err)
	}
	return target
}

func writeStates(t *testing.T, store *Store) map[string]string {
	t.Helper()
	writes, err := store.CalendarWrites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, write := range writes {
		states[write.EventID] = write.State
	}
	return states
}

func TestWritingIsOffUntilTurnedOnForACalDAVCalendar(t *testing.T) {
	store, ctx, now := writeBackStore(t)
	acceptTime(t, store, "calendar_event_ahead", now.Add(24*time.Hour))
	acceptTime(t, store, "calendar_event_past", now.Add(-24*time.Hour))
	if states := writeStates(t, store); len(states) != 0 {
		t.Fatalf("writes queued with writing off: %v", states)
	}

	ics := testCalendarSource(calendarcore.SourceICS)
	ics.SourceID = "calendar_source_ics_file"
	if err := store.ReplaceImportedCalendar(ctx, ics, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: ics.SourceID}, now); !errors.Is(err, ErrCalendarNotWritable) {
		t.Fatalf("an ICS file as the target: %v", err)
	}
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_missing"}, now); !errors.Is(err, ErrCalendarSourceNotFound) {
		t.Fatalf("a missing calendar as the target: %v", err)
	}

	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{
		SourceID: "calendar_source_import_01", Username: "owner", Password: "synthetic-app-password",
	}, now); err != nil {
		t.Fatal(err)
	}
	target, on, err := store.CalendarWriteTarget(ctx)
	if err != nil || !on || target.CollectionURL != testCollection || target.Label != "Imported commitments" ||
		target.Username != "owner" || target.Password != "synthetic-app-password" || !target.EnabledAt.Equal(now) {
		t.Fatalf("target = %+v, %v, %v", target, on, err)
	}
	// Only the time that has not ended is written.
	if states := writeStates(t, store); len(states) != 1 || states["calendar_event_ahead"] != CalendarWriteCreate {
		t.Fatalf("queued on turning on: %v", states)
	}
	// Refreshing the calendar's events keeps writing on.
	if err := store.ReplaceImportedCalendar(ctx, testCalendarSource(calendarcore.SourceCalDAV), nil, testCollection); err != nil {
		t.Fatal(err)
	}
	if _, on, _ := store.CalendarWriteTarget(ctx); !on || len(writeStates(t, store)) != 1 {
		t.Fatal("re-importing the calendar turned writing off")
	}
}

func TestAcceptingAndUndoingQueueTheirWrites(t *testing.T) {
	store, ctx, now := writeBackStore(t)
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01", Username: "owner"}, now); err != nil {
		t.Fatal(err)
	}
	// Undone before its create was confirmed: what that create may have
	// written is looked for, by the approval it was written under, and the
	// looking done, nothing is left.
	acceptTime(t, store, "calendar_event_quick_undo", now.Add(time.Hour))
	undoTime(t, store, "calendar_event_quick_undo")
	due, err := store.DueCalendarWrites(ctx, targetOf(t, store), now, 10)
	if err != nil || len(due) != 1 || due[0].State != CalendarWriteDelete || due[0].Href != "" {
		t.Fatalf("an unconfirmed undone time: %+v, %v", due, err)
	}
	unconfirmed, found, err := store.CalendarWriteUnconfirmed(ctx, "calendar_event_quick_undo")
	if err != nil || !found || unconfirmed.EventID != "calendar_event_quick_undo" || unconfirmed.Approval != "" ||
		!unconfirmed.StartAt.IsZero() {
		t.Fatalf("what it looks for: %+v, %v, %v", unconfirmed, found, err)
	}
	if err := store.MarkCalendarWriteRemoved(ctx, targetOf(t, store), "calendar_event_quick_undo"); err != nil {
		t.Fatal(err)
	}
	if states := writeStates(t, store); len(states) != 0 {
		t.Fatalf("after looking, an unwritten undone time left %v", states)
	}
	if _, found, _ := store.CalendarWriteUnconfirmed(ctx, "calendar_event_quick_undo"); found {
		t.Fatal("what to look for outlived its removal")
	}

	acceptTime(t, store, "calendar_event_kept", now.Add(2*time.Hour))
	due, err = store.DueCalendarWrites(ctx, targetOf(t, store), now, 10)
	if err != nil || len(due) != 1 || due[0].State != CalendarWriteCreate || due[0].Title != "Accepted calendar_event_kept" {
		t.Fatalf("due = %+v, %v", due, err)
	}
	kept, found, err := store.CalendarWriteEvent(ctx, "calendar_event_kept")
	if err != nil || !found || kept.Event.Ownership != calendarcore.OwnershipAppOwned {
		t.Fatalf("event to write = %+v, %v, %v", kept, found, err)
	}
	href := testCollection + "calendar_event_kept@zeitboard.local.ics"
	if err := store.MarkCalendarWritten(ctx, targetOf(t, store), kept, href, `"etag-1"`, now); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.DueCalendarWrites(ctx, targetOf(t, store), now, 10); len(due) != 0 {
		t.Fatalf("a written time is still due: %+v", due)
	}
	undoTime(t, store, "calendar_event_kept")
	due, err = store.DueCalendarWrites(ctx, targetOf(t, store), now, 10)
	if err != nil || len(due) != 1 || due[0].State != CalendarWriteDelete || due[0].Href != href || due[0].ETag != `"etag-1"` {
		t.Fatalf("undoing a written time: %+v, %v", due, err)
	}
	if err := store.MarkCalendarWriteRemoved(ctx, targetOf(t, store), "calendar_event_kept"); err != nil {
		t.Fatal(err)
	}
	if states := writeStates(t, store); len(states) != 0 {
		t.Fatalf("after removal: %v", states)
	}

	// Undone while being written: what was written is removed.
	acceptTime(t, store, "calendar_event_race", now.Add(3*time.Hour))
	race, _, _ := store.CalendarWriteEvent(ctx, "calendar_event_race")
	undoTime(t, store, "calendar_event_race")
	if err := store.MarkCalendarWritten(ctx, targetOf(t, store), race, testCollection+"race.ics", `"etag-race"`, now); err != nil {
		t.Fatal(err)
	}
	due, err = store.DueCalendarWrites(ctx, targetOf(t, store), now, 10)
	if err != nil || len(due) != 1 || due[0].State != CalendarWriteDelete || due[0].ETag != `"etag-race"` ||
		due[0].Title != "Accepted calendar_event_race" || !due[0].StartAt.Equal(now.Add(3*time.Hour)) {
		t.Fatalf("a time undone mid-write: %+v, %v", due, err)
	}

	// An import recognises the accepted times, and the one being removed.
	acceptTime(t, store, "calendar_event_current", now.Add(4*time.Hour))
	own, err := store.OwnCalendarEventIDs(ctx)
	if err != nil || len(own) != 2 || !own["calendar_event_current"] || !own["calendar_event_race"] {
		t.Fatalf("own events = %v, %v", own, err)
	}
}

// A time accepted again while its earlier acceptance is still being removed
// is written once that removal is done. If the owner keeps the changed event
// instead, that event stands for the time, at the version ZeitBoard wrote.
func TestATimeAcceptedAgainMidRemovalIsWrittenAfter(t *testing.T) {
	store, ctx, now := writeBackStore(t)
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01"}, now); err != nil {
		t.Fatal(err)
	}
	removing := func(id string) {
		t.Helper()
		acceptTime(t, store, id, now.Add(time.Hour))
		accepted, _, _ := store.CalendarWriteEvent(ctx, id)
		if err := store.MarkCalendarWritten(ctx, targetOf(t, store), accepted, testCollection+id+".ics", `"etag"`, now); err != nil {
			t.Fatal(err)
		}
		undoTime(t, store, id)
		acceptTime(t, store, id, now.Add(time.Hour))
	}

	removing("calendar_event_removed")
	if err := store.MarkCalendarWriteRemoved(ctx, targetOf(t, store), "calendar_event_removed"); err != nil {
		t.Fatal(err)
	}
	removing("calendar_event_kept_changed")
	if err := store.MarkCalendarWriteConflict(ctx, targetOf(t, store), CalendarWrite{EventID: "calendar_event_kept_changed", State: CalendarWriteDelete},
		"changed in the calendar", now); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveCalendarWriteConflict(ctx, "calendar_event_kept_changed", false, now); err != nil {
		t.Fatal(err)
	}
	// Undone again before the removal settled: nothing more to write.
	removing("calendar_event_undone_again")
	undoTime(t, store, "calendar_event_undone_again")
	if err := store.MarkCalendarWriteRemoved(ctx, targetOf(t, store), "calendar_event_undone_again"); err != nil {
		t.Fatal(err)
	}
	states := writeStates(t, store)
	if len(states) != 2 || states["calendar_event_removed"] != CalendarWriteCreate ||
		states["calendar_event_kept_changed"] != CalendarWriteWritten {
		t.Fatalf("after the removals settled: %v", states)
	}
	writes, _ := store.CalendarWrites(ctx)
	for _, write := range writes {
		if write.EventID == "calendar_event_kept_changed" &&
			(write.Href != testCollection+"calendar_event_kept_changed.ics" || write.ETag != `"etag"` || write.Detail != "") {
			t.Fatalf("the kept event as written: %+v", write)
		}
	}

	// A time left out of the calendar at the owner's word stays out.
	acceptTime(t, store, "calendar_event_left_out", now.Add(2*time.Hour))
	if err := store.MarkCalendarWriteConflict(ctx, targetOf(t, store), CalendarWrite{EventID: "calendar_event_left_out", State: CalendarWriteCreate},
		"a different event is there", now); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveCalendarWriteConflict(ctx, "calendar_event_left_out", false, now); err != nil {
		t.Fatal(err)
	}
	if _, queued := writeStates(t, store)["calendar_event_left_out"]; queued {
		t.Fatal("a time the owner left out was queued again")
	}
}

// A write that lands after its acceptance was undone, and the time accepted
// again, is removed first, where it landed; then the new acceptance is
// written.
func TestAWriteLandingForAnUndoneAcceptanceIsRemovedFirst(t *testing.T) {
	store, ctx, now := writeBackStore(t)
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01"}, now); err != nil {
		t.Fatal(err)
	}
	const id = "calendar_event_relanded"
	acceptTime(t, store, id, now.Add(time.Hour))
	due := func() CalendarWrite {
		t.Helper()
		writes, err := store.DueCalendarWrites(ctx, targetOf(t, store), now, 10)
		if err != nil || len(writes) != 1 {
			t.Fatalf("due %+v, %v", writes, err)
		}
		return writes[0]
	}

	// Undone and accepted again while the write was on its way: its undo
	// queued a removal of whatever it writes, ahead of the new acceptance.
	accepted, _, _ := store.CalendarWriteEvent(ctx, id)
	undoTime(t, store, id)
	acceptTime(t, store, id, now.Add(time.Hour))
	landed := accepted
	landed.Approval = "decision_undone_meanwhile"
	// A conflict or failure of the create carried for it lands on no other
	// write.
	carried := CalendarWrite{EventID: id, State: CalendarWriteCreate, Approval: landed.Approval}
	if err := store.MarkCalendarWriteConflict(ctx, targetOf(t, store), carried, "a different event is there", now); err != nil {
		t.Fatal(err)
	}
	if err := store.DeferCalendarWrite(ctx, targetOf(t, store), carried, "the calendar did not answer", now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if write := due(); write.State != CalendarWriteDelete || write.Href != "" || write.Attempts != 0 || write.Detail != "" {
		t.Fatalf("an outcome for the undone acceptance landed on another write: %+v", write)
	}
	if err := store.MarkCalendarWritten(ctx, targetOf(t, store), landed, testCollection+"relanded.ics", `"etag-landed"`, now); err != nil {
		t.Fatal(err)
	}
	if write := due(); write.State != CalendarWriteDelete || write.Href != testCollection+"relanded.ics" ||
		write.ETag != `"etag-landed"` {
		t.Fatalf("what landed for the undone acceptance: %+v", write)
	}
	if _, unconfirmed, _ := store.CalendarWriteUnconfirmed(ctx, id); unconfirmed {
		t.Fatal("a removal that knows its address still looks for it")
	}
	if err := store.MarkCalendarWriteRemoved(ctx, targetOf(t, store), id); err != nil {
		t.Fatal(err)
	}
	if write := due(); write.State != CalendarWriteCreate || write.Href != "" {
		t.Fatalf("the new acceptance after the removal: %+v", write)
	}
}

// What the create of a time undone before it was confirmed is found to have
// written waits for the owner, at the address and version found: removing it
// anyway removes it there, whatever its version.
func TestWhatAnUnconfirmedCreateWroteWaitsForTheOwner(t *testing.T) {
	store, ctx, now := writeBackStore(t)
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01"}, now); err != nil {
		t.Fatal(err)
	}
	const id = "calendar_event_found"
	acceptTime(t, store, id, now.Add(time.Hour))
	undoTime(t, store, id)
	if err := store.MarkCalendarWriteFound(ctx, targetOf(t, store), id, testCollection+"found.ics", `"etag-found"`,
		"it may have been added before the undo", now); err != nil {
		t.Fatal(err)
	}
	writes, _ := store.CalendarWrites(ctx)
	if len(writes) != 1 || writes[0].State != CalendarWriteConflict || writes[0].Href != testCollection+"found.ics" ||
		writes[0].ETag != `"etag-found"` || writes[0].Detail != "it may have been added before the undo" {
		t.Fatalf("what was found: %+v", writes)
	}
	if _, unconfirmed, _ := store.CalendarWriteUnconfirmed(ctx, id); unconfirmed {
		t.Fatal("what was found is still looked for")
	}
	if err := store.ResolveCalendarWriteConflict(ctx, id, true, now); err != nil {
		t.Fatal(err)
	}
	due, err := store.DueCalendarWrites(ctx, targetOf(t, store), now, 10)
	if err != nil || len(due) != 1 || due[0].State != CalendarWriteDelete || due[0].Href != testCollection+"found.ics" || due[0].ETag != "" {
		t.Fatalf("removing it anyway: %+v, %v", due, err)
	}
}

// A pass carried on past turning writing on again, or for another calendar,
// records nothing, and finds nothing more to carry.
func TestAPassOutlivingItsTimeOfWritingRecordsNothing(t *testing.T) {
	store, ctx, now := writeBackStore(t)
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01"}, now); err != nil {
		t.Fatal(err)
	}
	const id = "calendar_event_carried"
	acceptTime(t, store, id, now.Add(time.Hour))
	before := targetOf(t, store)
	accepted, _, _ := store.CalendarWriteEvent(ctx, id)
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01"}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	write := CalendarWrite{EventID: id, State: CalendarWriteCreate, Approval: accepted.Approval}
	if err := store.MarkCalendarWritten(ctx, before, accepted, testCollection+"before.ics", `"etag-before"`, now); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkCalendarWriteConflict(ctx, before, write, "a different event is there", now); err != nil {
		t.Fatal(err)
	}
	if err := store.DeferCalendarWrite(ctx, before, write, "the calendar did not answer", now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	due, err := store.DueCalendarWrites(ctx, targetOf(t, store), now, 10)
	if err != nil || len(due) != 1 || due[0].State != CalendarWriteCreate || due[0].Attempts != 0 || due[0].Href != "" {
		t.Fatalf("what the earlier time of writing carried landed: %+v, %v", due, err)
	}
	if stale, _ := store.DueCalendarWrites(ctx, before, now, 10); len(stale) != 0 {
		t.Fatalf("the earlier time of writing found more to carry: %+v", stale)
	}
	if still, err := store.WritingStill(ctx, before); err != nil || still {
		t.Fatalf("the earlier time of writing is still on: %v", err)
	}
	if still, err := store.WritingStill(ctx, targetOf(t, store)); err != nil || !still {
		t.Fatalf("this time of writing is not on: %v", err)
	}
}

// A create that met a conflict is settled by undoing its time, so the time
// accepted again is written afresh.
func TestUndoingATimeSettlesItsCreateConflict(t *testing.T) {
	store, ctx, now := writeBackStore(t)
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01"}, now); err != nil {
		t.Fatal(err)
	}
	const id = "calendar_event_conflicted"
	acceptTime(t, store, id, now.Add(time.Hour))
	if err := store.MarkCalendarWriteConflict(ctx, targetOf(t, store), CalendarWrite{EventID: id, State: CalendarWriteCreate},
		"a different event is there", now); err != nil {
		t.Fatal(err)
	}
	undoTime(t, store, id)
	if states := writeStates(t, store); len(states) != 0 {
		t.Fatalf("an undone time kept its conflict: %v", states)
	}
	acceptTime(t, store, id, now.Add(time.Hour))
	if states := writeStates(t, store); states[id] != CalendarWriteCreate {
		t.Fatalf("accepted again: %v", states)
	}
}

func TestAConflictWaitsForTheOwner(t *testing.T) {
	store, ctx, now := writeBackStore(t)
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01"}, now); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"calendar_event_remove", "calendar_event_keep"} {
		acceptTime(t, store, id, now.Add(time.Hour))
		accepted, _, _ := store.CalendarWriteEvent(ctx, id)
		if err := store.MarkCalendarWritten(ctx, targetOf(t, store), accepted, testCollection+id+".ics", `"etag"`, now); err != nil {
			t.Fatal(err)
		}
		undoTime(t, store, id)
		if err := store.MarkCalendarWriteConflict(ctx, targetOf(t, store), CalendarWrite{EventID: id, State: CalendarWriteDelete}, "changed in the calendar", now); err != nil {
			t.Fatal(err)
		}
	}
	if due, _ := store.DueCalendarWrites(ctx, targetOf(t, store), now, 10); len(due) != 0 {
		t.Fatalf("a conflict was retried on its own: %+v", due)
	}
	if err := store.ResolveCalendarWriteConflict(ctx, "calendar_event_remove", true, now); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveCalendarWriteConflict(ctx, "calendar_event_keep", false, now); err != nil {
		t.Fatal(err)
	}
	due, err := store.DueCalendarWrites(ctx, targetOf(t, store), now, 10)
	if err != nil || len(due) != 1 || due[0].EventID != "calendar_event_remove" || due[0].ETag != "" {
		t.Fatalf("after settling: %+v, %v", due, err)
	}
	if err := store.ResolveCalendarWriteConflict(ctx, "calendar_event_keep", false, now); !errors.Is(err, ErrCalendarWriteNotFound) {
		t.Fatalf("settling twice: %v", err)
	}
}

func TestFailedWritesWaitAndStoppingForgets(t *testing.T) {
	store, ctx, now := writeBackStore(t)
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01", Password: "synthetic"}, now); err != nil {
		t.Fatal(err)
	}
	acceptTime(t, store, "calendar_event_retry", now.Add(time.Hour))
	if err := store.DeferCalendarWrite(ctx, targetOf(t, store), CalendarWrite{EventID: "calendar_event_retry", State: CalendarWriteCreate},
		"the calendar did not answer", now.Add(5*time.Minute), now); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.DueCalendarWrites(ctx, targetOf(t, store), now.Add(4*time.Minute), 10); len(due) != 0 {
		t.Fatalf("retried early: %+v", due)
	}
	due, err := store.DueCalendarWrites(ctx, targetOf(t, store), now.Add(5*time.Minute), 10)
	if err != nil || len(due) != 1 || due[0].Attempts != 1 || due[0].Detail != "the calendar did not answer" {
		t.Fatalf("retry due = %+v, %v", due, err)
	}
	// Trying again now does not wait out the delay.
	if err := store.RetryCalendarWrites(ctx); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.DueCalendarWrites(ctx, targetOf(t, store), now, 10); len(due) != 1 {
		t.Fatalf("a retry asked for now is not due: %+v", due)
	}

	if err := store.ClearCalendarWriteTarget(ctx); err != nil {
		t.Fatal(err)
	}
	if _, on, _ := store.CalendarWriteTarget(ctx); on {
		t.Fatal("writing is still on")
	}
	if states := writeStates(t, store); len(states) != 0 {
		t.Fatalf("stopping kept %v", states)
	}

	// Removing the calendar, or erasing everything, stops writing too.
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01"}, now); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveImportedCalendar(ctx, "calendar_source_import_01"); err != nil {
		t.Fatal(err)
	}
	if _, on, _ := store.CalendarWriteTarget(ctx); on || len(writeStates(t, store)) != 0 {
		t.Fatal("writing outlived its calendar")
	}
}
