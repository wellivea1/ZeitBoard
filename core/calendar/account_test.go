package calendar

import (
	"strings"
	"testing"
	"time"
)

func accountOptions() ParseOptions {
	imported := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	start, end := CoverageAround(imported)
	return ParseOptions{
		SourceID: "calendar_source_account_work", SourceLabel: "Work", Kind: SourceICS,
		ImportedAt: imported, CoverageStart: start, CoverageEnd: end, DefaultZoneID: "America/New_York",
	}
}

// An account's events are kept like a file's: stable ids, untitled events as
// "Busy", free time not blocking, repeats and events outside the coverage
// left out, and the whole set in time order.
func TestAccountEventsAreKeptLikeAFilesEvents(t *testing.T) {
	at := time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC)
	events := []AccountEvent{
		{ID: "provider-2", Title: "Dentist", StartAt: at.Add(24 * time.Hour), EndAt: at.Add(25 * time.Hour), Busy: true, ZoneID: "America/Chicago"},
		{ID: "provider-1", Title: "  ", StartAt: at, EndAt: at.Add(time.Hour), Busy: true},
		{ID: "provider-1", Title: "Repeated", StartAt: at, EndAt: at.Add(time.Hour), Busy: true},
		{ID: "provider-3", Title: "Free lunch", StartAt: at.Add(2 * time.Hour), EndAt: at.Add(3 * time.Hour), Busy: false},
		{ID: "provider-old", Title: "Long ago", StartAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC), Busy: true},
		{ID: "provider-4", Title: strings.Repeat("x", 300), StartAt: at.Add(4 * time.Hour), EndAt: at.Add(5 * time.Hour), Busy: true},
	}
	set, err := AccountEventSet(accountOptions(), events)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Sources) != 1 || !set.Sources[0].ReadOnly || set.Sources[0].Label != "Work" {
		t.Fatalf("source %+v", set.Sources)
	}
	if len(set.Events) != 4 {
		t.Fatalf("kept %d events", len(set.Events))
	}
	first := set.Events[0]
	if first.Title != "Busy" || first.ZoneID != "America/New_York" || first.SourceRecordID != "provider-1" {
		t.Errorf("first %+v", first)
	}
	if set.Events[1].Busy || set.Events[1].Title != "Free lunch" {
		t.Errorf("free time %+v", set.Events[1])
	}
	if len([]rune(set.Events[2].Title)) != maxTitleRunes {
		t.Errorf("a long title kept %d characters", len([]rune(set.Events[2].Title)))
	}
	if set.Events[3].ZoneID != "America/Chicago" {
		t.Errorf("the event's own zone was not kept: %s", set.Events[3].ZoneID)
	}
	again, _ := AccountEventSet(accountOptions(), events)
	if again.Events[0].EventID != first.EventID {
		t.Fatal("the same event read twice has another id")
	}
	long := accountOptions()
	long.SourceLabel = strings.Repeat("Calendar ", 40)
	if named, err := AccountEventSet(long, nil); err != nil || len([]rune(named.Sources[0].Label)) > maxSourceLabelRunes {
		t.Fatalf("a long calendar name was not shortened: %v", err)
	}
}

// An all-day event is a day in the calendar's own zone, as a file's all-day
// dates are, however the provider stated it.
func TestAnAllDayAccountEventIsADayInTheCalendarsZone(t *testing.T) {
	day := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	set, err := AccountEventSet(accountOptions(), []AccountEvent{
		{ID: "holiday", Title: "Holiday", StartAt: day, EndAt: day.AddDate(0, 0, 1), AllDay: true, Busy: true, ZoneID: "UTC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	event := set.Events[0]
	newYork, _ := time.LoadLocation("America/New_York")
	if !event.AllDay || event.ZoneID != "America/New_York" ||
		!event.StartAt.Equal(time.Date(2026, 10, 14, 0, 0, 0, 0, newYork)) ||
		!event.EndAt.Equal(time.Date(2026, 10, 15, 0, 0, 0, 0, newYork)) {
		t.Fatalf("all-day event %+v", event)
	}
}

func TestAnAccountEventWithoutAnIDIsRefused(t *testing.T) {
	at := time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC)
	if _, err := AccountEventSet(accountOptions(), []AccountEvent{{Title: "No id", StartAt: at, EndAt: at.Add(time.Hour)}}); err == nil {
		t.Fatal("kept an event without an id")
	}
}
