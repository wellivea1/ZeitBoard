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
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: ics.SourceID}, now); !errors.Is(err, ErrCalendarWriteTargetNotCalDAV) {
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
	// Undone before it was written: nothing to write or remove.
	acceptTime(t, store, "calendar_event_quick_undo", now.Add(time.Hour))
	undoTime(t, store, "calendar_event_quick_undo")
	if states := writeStates(t, store); len(states) != 0 {
		t.Fatalf("an unwritten undone time left %v", states)
	}

	acceptTime(t, store, "calendar_event_kept", now.Add(2*time.Hour))
	due, err := store.DueCalendarWrites(ctx, now, 10)
	if err != nil || len(due) != 1 || due[0].State != CalendarWriteCreate || due[0].Title != "Accepted calendar_event_kept" {
		t.Fatalf("due = %+v, %v", due, err)
	}
	kept, found, err := store.CalendarWriteEvent(ctx, "calendar_event_kept")
	if err != nil || !found || kept.Ownership != calendarcore.OwnershipAppOwned {
		t.Fatalf("event to write = %+v, %v, %v", kept, found, err)
	}
	href := testCollection + "calendar_event_kept@zeitboard.local.ics"
	if err := store.MarkCalendarWritten(ctx, kept, href, `"etag-1"`, now); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.DueCalendarWrites(ctx, now, 10); len(due) != 0 {
		t.Fatalf("a written time is still due: %+v", due)
	}
	undoTime(t, store, "calendar_event_kept")
	due, err = store.DueCalendarWrites(ctx, now, 10)
	if err != nil || len(due) != 1 || due[0].State != CalendarWriteDelete || due[0].Href != href || due[0].ETag != `"etag-1"` {
		t.Fatalf("undoing a written time: %+v, %v", due, err)
	}
	if err := store.MarkCalendarWriteRemoved(ctx, "calendar_event_kept"); err != nil {
		t.Fatal(err)
	}
	if states := writeStates(t, store); len(states) != 0 {
		t.Fatalf("after removal: %v", states)
	}

	// Undone while being written: what was written is removed.
	acceptTime(t, store, "calendar_event_race", now.Add(3*time.Hour))
	race, _, _ := store.CalendarWriteEvent(ctx, "calendar_event_race")
	undoTime(t, store, "calendar_event_race")
	if err := store.MarkCalendarWritten(ctx, race, testCollection+"race.ics", `"etag-race"`, now); err != nil {
		t.Fatal(err)
	}
	due, err = store.DueCalendarWrites(ctx, now, 10)
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

func TestAConflictWaitsForTheOwner(t *testing.T) {
	store, ctx, now := writeBackStore(t)
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{SourceID: "calendar_source_import_01"}, now); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"calendar_event_remove", "calendar_event_keep"} {
		acceptTime(t, store, id, now.Add(time.Hour))
		event, _, _ := store.CalendarWriteEvent(ctx, id)
		if err := store.MarkCalendarWritten(ctx, event, testCollection+id+".ics", `"etag"`, now); err != nil {
			t.Fatal(err)
		}
		undoTime(t, store, id)
		if err := store.MarkCalendarWriteConflict(ctx, id, "changed in the calendar", now); err != nil {
			t.Fatal(err)
		}
	}
	if due, _ := store.DueCalendarWrites(ctx, now, 10); len(due) != 0 {
		t.Fatalf("a conflict was retried on its own: %+v", due)
	}
	if err := store.ResolveCalendarWriteConflict(ctx, "calendar_event_remove", true, now); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveCalendarWriteConflict(ctx, "calendar_event_keep", false, now); err != nil {
		t.Fatal(err)
	}
	due, err := store.DueCalendarWrites(ctx, now, 10)
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
	if err := store.DeferCalendarWrite(ctx, "calendar_event_retry", "the calendar did not answer", now.Add(5*time.Minute), now); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.DueCalendarWrites(ctx, now.Add(4*time.Minute), 10); len(due) != 0 {
		t.Fatalf("retried early: %+v", due)
	}
	due, err := store.DueCalendarWrites(ctx, now.Add(5*time.Minute), 10)
	if err != nil || len(due) != 1 || due[0].Attempts != 1 || due[0].Detail != "the calendar did not answer" {
		t.Fatalf("retry due = %+v, %v", due, err)
	}
	// Trying again now does not wait out the delay.
	if err := store.RetryCalendarWrites(ctx); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.DueCalendarWrites(ctx, now, 10); len(due) != 1 {
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
