package calendar

import (
	"strings"
	"testing"
	"time"
)

func testParseOptions() ParseOptions {
	return ParseOptions{
		SourceID:      "calendar_source_test",
		SourceLabel:   "Test calendar",
		Kind:          SourceICS,
		ImportedAt:    time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		CoverageStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		CoverageEnd:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		DefaultZoneID: "America/New_York",
	}
}

func TestParseICSUnfoldsTextAndMapsWindowsZone(t *testing.T) {
	document := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:event-1@example.test",
		"DTSTART;TZID=Eastern Standard Time:20260308T013000",
		"DTEND;TZID=Eastern Standard Time:20260308T033000",
		"SUMMARY:Planning\\, review with a deliberately long",
		" continuation",
		"DESCRIPTION:Line one\\nLine two",
		"LOCATION:Desk\\; east",
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	set, err := ParseICS([]byte(document), testParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(set.Events))
	}
	event := set.Events[0]
	if event.ZoneID != "America/New_York" {
		t.Fatalf("zone = %q", event.ZoneID)
	}
	if got := event.StartAt.Format(time.RFC3339); got != "2026-03-08T06:30:00Z" {
		t.Fatalf("start = %s", got)
	}
	if got := event.EndAt.Format(time.RFC3339); got != "2026-03-08T07:30:00Z" {
		t.Fatalf("end = %s", got)
	}
	if event.Title != "Planning, review with a deliberately longcontinuation" {
		t.Fatalf("title = %q", event.Title)
	}
	if event.Notes != "Line one\nLine two" || event.Location != "Desk; east" {
		t.Fatalf("decoded text = %#v", event)
	}
}

func TestParseICSRejectsNonexistentCivilTime(t *testing.T) {
	document := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:event-1@example.test\r\n" +
		"DTSTART;TZID=America/New_York:20260308T023000\r\n" +
		"DTEND;TZID=America/New_York:20260308T033000\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	_, err := ParseICS([]byte(document), testParseOptions())
	if err == nil || !strings.Contains(err.Error(), "nonexistent civil time") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseICSRecurrencePreservesCivilTimeAndAppliesExceptions(t *testing.T) {
	document := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:weekly@example.test",
		"DTSTART;TZID=America/New_York:20260301T090000",
		"DTEND;TZID=America/New_York:20260301T100000",
		"RRULE:FREQ=WEEKLY;COUNT=4",
		"EXDATE;TZID=America/New_York:20260315T090000",
		"SUMMARY:Weekly check-in",
		"END:VEVENT",
		"BEGIN:VEVENT",
		"UID:weekly@example.test",
		"RECURRENCE-ID;TZID=America/New_York:20260322T090000",
		"DTSTART;TZID=America/New_York:20260322T110000",
		"SUMMARY:Moved check-in",
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	set, err := ParseICS([]byte(document), testParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Events) != 3 {
		t.Fatalf("events = %d, want 3: %#v", len(set.Events), set.Events)
	}
	wantStarts := []string{
		"2026-03-01T14:00:00Z",
		"2026-03-08T13:00:00Z",
		"2026-03-22T15:00:00Z",
	}
	for index, want := range wantStarts {
		if got := set.Events[index].StartAt.Format(time.RFC3339); got != want {
			t.Errorf("event %d start = %s, want %s", index, got, want)
		}
	}
	if set.Events[2].Title != "Moved check-in" {
		t.Fatalf("override title = %q", set.Events[2].Title)
	}
	if set.Events[2].EndAt.Sub(set.Events[2].StartAt) != time.Hour {
		t.Fatalf("override did not inherit one-hour duration")
	}
}

func TestParseICSAllDayCrossesDSTAsCivilDay(t *testing.T) {
	document := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:all-day@example.test",
		"DTSTART;VALUE=DATE:20260308",
		"DTEND;VALUE=DATE:20260309",
		"SUMMARY:All day",
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	set, err := ParseICS([]byte(document), testParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	event := set.Events[0]
	if !event.AllDay || event.EndAt.Sub(event.StartAt) != 23*time.Hour {
		t.Fatalf("all-day interval = %s to %s", event.StartAt, event.EndAt)
	}
}

func TestParseICSCancelledExceptionAndTransparentEventDoNotBlock(t *testing.T) {
	document := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:daily@example.test",
		"DTSTART:20260301T120000Z",
		"DTEND:20260301T130000Z",
		"RRULE:FREQ=DAILY;COUNT=2",
		"END:VEVENT",
		"BEGIN:VEVENT",
		"UID:daily@example.test",
		"RECURRENCE-ID:20260302T120000Z",
		"DTSTART:20260302T120000Z",
		"STATUS:CANCELLED",
		"END:VEVENT",
		"BEGIN:VEVENT",
		"UID:free@example.test",
		"DTSTART:20260303T120000Z",
		"DTEND:20260303T130000Z",
		"TRANSP:TRANSPARENT",
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	set, err := ParseICS([]byte(document), testParseOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Events) != 2 {
		t.Fatalf("events = %d, want master occurrence plus transparent event", len(set.Events))
	}
	if !set.Events[0].Busy || set.Events[1].Busy {
		t.Fatalf("busy flags = %t, %t", set.Events[0].Busy, set.Events[1].Busy)
	}
}

func TestParseICSRejectsUnboundedOrMalformedInputs(t *testing.T) {
	tests := map[string]string{
		"orphan fold": " continuation\r\n",
		"unknown zone": strings.Join([]string{
			"BEGIN:VCALENDAR", "VERSION:2.0", "BEGIN:VEVENT", "UID:a@example.test",
			"DTSTART;TZID=Made Up Zone:20260301T120000", "END:VEVENT", "END:VCALENDAR", "",
		}, "\r\n"),
		"minutely recurrence": strings.Join([]string{
			"BEGIN:VCALENDAR", "VERSION:2.0", "BEGIN:VEVENT", "UID:a@example.test",
			"DTSTART:20260301T120000Z", "DTEND:20260301T120100Z",
			"RRULE:FREQ=MINUTELY", "END:VEVENT", "END:VCALENDAR", "",
		}, "\r\n"),
		"duplicate uid": strings.Join([]string{
			"BEGIN:VCALENDAR", "VERSION:2.0", "BEGIN:VEVENT", "UID:a@example.test", "DTSTART:20260301T120000Z", "END:VEVENT",
			"BEGIN:VEVENT", "UID:a@example.test", "DTSTART:20260302T120000Z", "END:VEVENT", "END:VCALENDAR", "",
		}, "\r\n"),
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseICS([]byte(document), testParseOptions()); err == nil {
				t.Fatal("expected import to fail")
			}
		})
	}

	oversized := make([]byte, MaxDocumentBytes+1)
	if _, err := ParseICS(oversized, testParseOptions()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized error = %v", err)
	}
}
