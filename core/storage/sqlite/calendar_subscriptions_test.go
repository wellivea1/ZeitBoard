package sqlite

import (
	"testing"
	"time"

	calendarcore "non24.app/core/calendar"
)

const syntheticCalendarAddress = "https://calendar.example.test/ical/synthetic/private-canary-secret/basic.ics"

// A subscribed calendar keeps its address beside its copy; a refresh replaces
// the copy, a failure keeps it, and removing the calendar erases the address.
func TestASubscribedCalendarKeepsItsAddressWithItsCopy(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	start := time.Date(2026, 1, 5, 14, 0, 0, 0, time.UTC)
	source := testCalendarSource(calendarcore.SourceICS)
	first := testImportedEvent("calendar_event_import_01", "uid-1/20260105T140000Z", "Dentist", start, true)
	if err := store.ReplaceSubscribedCalendar(ctx, source, []calendarcore.Event{first}, syntheticCalendarAddress); err != nil {
		t.Fatal(err)
	}
	subscriptions, err := store.CalendarSubscriptions(ctx)
	if err != nil || len(subscriptions) != 1 {
		t.Fatalf("subscriptions %+v, %v", subscriptions, err)
	}
	if got := subscriptions[0]; got.Address != syntheticCalendarAddress || got.Label != source.Label ||
		!got.RefreshedAt.Equal(source.LastImportedAt) || got.Failure != "" {
		t.Fatalf("subscription %+v", got)
	}
	// The address is not the source's endpoint, which a file source never has.
	sources, err := store.ListCalendarSources(ctx)
	if err != nil || len(sources) != 1 || sources[0].Endpoint != "" {
		t.Fatalf("sources %+v, %v", sources, err)
	}

	failedAt := source.LastImportedAt.Add(time.Hour)
	if err := store.RecordCalendarSubscriptionFailure(ctx, source.SourceID, failedAt, "the calendar's server returned HTTP 503"); err != nil {
		t.Fatal(err)
	}
	subscriptions, _ = store.CalendarSubscriptions(ctx)
	if got := subscriptions[0]; !got.AttemptedAt.Equal(failedAt) || !got.RefreshedAt.Equal(source.LastImportedAt) ||
		got.Failure == "" {
		t.Fatalf("after a failure %+v", got)
	}
	events, err := store.CalendarEvents(ctx, start.Add(-time.Hour), start.Add(2*time.Hour))
	if err != nil || len(events) != 1 {
		t.Fatalf("a failure lost the copy: %d events, %v", len(events), err)
	}

	refreshed := source
	refreshed.LastImportedAt = failedAt.Add(time.Hour)
	moved := testImportedEvent("calendar_event_import_02", "uid-2/20260105T160000Z", "Moved", start.Add(2*time.Hour), true)
	if err := store.ReplaceSubscribedCalendar(ctx, refreshed, []calendarcore.Event{moved}, syntheticCalendarAddress); err != nil {
		t.Fatal(err)
	}
	subscriptions, _ = store.CalendarSubscriptions(ctx)
	if got := subscriptions[0]; got.Failure != "" || !got.RefreshedAt.Equal(refreshed.LastImportedAt) {
		t.Fatalf("after a refresh %+v", got)
	}
	events, _ = store.CalendarEvents(ctx, start.Add(-time.Hour), start.Add(4*time.Hour))
	if len(events) != 1 || events[0].EventID != "calendar_event_import_02" {
		t.Fatalf("the refresh did not replace the copy: %+v", events)
	}

	if err := store.RemoveImportedCalendar(ctx, source.SourceID); err != nil {
		t.Fatal(err)
	}
	if subscriptions, err := store.CalendarSubscriptions(ctx); err != nil || len(subscriptions) != 0 {
		t.Fatalf("the address outlived its calendar: %+v, %v", subscriptions, err)
	}
	var kept int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_calendar_subscriptions`).Scan(&kept); err != nil || kept != 0 {
		t.Fatalf("%d addresses kept, %v", kept, err)
	}
}

func TestOnlyAnHTTPSAddressIsKept(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	source := testCalendarSource(calendarcore.SourceICS)
	for _, address := range []string{
		"http://calendar.example.test/basic.ics",
		"https://user:secret@calendar.example.test/basic.ics",
		"https://calendar.example.test/basic.ics#fragment",
		"calendar.example.test/basic.ics",
	} {
		if err := store.ReplaceSubscribedCalendar(ctx, source, nil, address); err == nil {
			t.Errorf("kept %q", address)
		}
	}
	if err := store.ReplaceSubscribedCalendar(ctx, testCalendarSource(calendarcore.SourceCalDAV), nil, syntheticCalendarAddress); err == nil {
		t.Error("kept a CalDAV source as a subscription")
	}
	if err := store.RecordCalendarSubscriptionFailure(ctx, "calendar_source_import_01", time.Now(), "failed"); err == nil {
		t.Error("recorded a failure for a calendar nobody subscribed to")
	}
}
