package calendar

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestExportOwnedICSOmitsImportsFoldsAndRoundTrips(t *testing.T) {
	created := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	owned := Event{
		EventID:        "calendar_event_owned_test",
		SourceID:       "calendar_source_zeitboard",
		SourceRecordID: "proposal_test_01",
		Title:          "Write review, reconcile constraints; then document the result with a long Unicode suffix: cafe\u0301",
		StartAt:        time.Date(2026, 3, 16, 14, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 3, 16, 15, 0, 0, 0, time.UTC),
		ZoneID:         "America/New_York",
		Busy:           true,
		Ownership:      OwnershipAppOwned,
		CreatedAt:      created,
		Location:       "Desk, east; second floor",
		Notes:          "First line\nSecond line",
		TaskID:         "task_test_01",
		TaskRevision:   2,
		ProposalID:     "proposal_test_01",
	}
	imported := Event{
		EventID:        "calendar_event_imported_test",
		SourceID:       "calendar_source_imported",
		SourceRecordID: "private@example.test/20260316T120000Z",
		Title:          "PRIVATE IMPORTED TITLE",
		StartAt:        time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 3, 16, 13, 0, 0, 0, time.UTC),
		ZoneID:         "UTC",
		Busy:           true,
		Ownership:      OwnershipImported,
		CreatedAt:      created,
	}

	data, err := ExportOwnedICS([]Event{imported, owned}, created)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("PRIVATE IMPORTED TITLE")) {
		t.Fatal("export leaked an imported event")
	}
	if !bytes.HasSuffix(data, []byte("END:VCALENDAR\r\n")) {
		t.Fatalf("calendar does not use a final CRLF: %q", data[len(data)-20:])
	}
	for _, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\r\n")), []byte("\r\n")) {
		if len(line) > 75 {
			t.Fatalf("folded line has %d octets: %q", len(line), line)
		}
		if !utf8.Valid(line) {
			t.Fatalf("fold split UTF-8: %q", line)
		}
	}

	options := testParseOptions()
	options.SourceID = "calendar_source_roundtrip"
	set, err := ParseICS(data, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Events) != 1 {
		t.Fatalf("round-trip events = %d", len(set.Events))
	}
	got := set.Events[0]
	if got.Title != owned.Title || got.Location != owned.Location || got.Notes != owned.Notes {
		t.Fatalf("round-trip text = %#v", got)
	}
	if !got.StartAt.Equal(owned.StartAt) || !got.EndAt.Equal(owned.EndAt) {
		t.Fatalf("round-trip interval = %s to %s", got.StartAt, got.EndAt)
	}
}

func TestExportOwnedICSUsesCivilDatesForAllDayEvents(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	event := Event{
		EventID:        "calendar_event_all_day",
		SourceID:       "calendar_source_zeitboard",
		SourceRecordID: "proposal_all_day",
		Title:          "Reserved day",
		StartAt:        time.Date(2026, 3, 8, 0, 0, 0, 0, loc).UTC(),
		EndAt:          time.Date(2026, 3, 9, 0, 0, 0, 0, loc).UTC(),
		ZoneID:         "America/New_York",
		AllDay:         true,
		Busy:           true,
		Ownership:      OwnershipAppOwned,
		CreatedAt:      created,
		TaskID:         "task_all_day",
		TaskRevision:   1,
		ProposalID:     "proposal_all_day",
	}
	data, err := ExportOwnedICS([]Event{event}, created)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "DTSTART;VALUE=DATE:20260308\r\n") || !strings.Contains(text, "DTEND;VALUE=DATE:20260309\r\n") {
		t.Fatalf("all-day export uses wrong values:\n%s", text)
	}
}
