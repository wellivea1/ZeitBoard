package ownerevents

import (
	"strings"
	"testing"
	"time"
)

func synthetic() Record {
	at := time.Date(2026, 10, 29, 19, 0, 0, 0, time.UTC)
	return Record{
		EventID:   "event_dentist_01",
		Title:     "Synthetic dentist",
		StartAt:   at,
		EndAt:     at.Add(time.Hour),
		ZoneID:    "America/New_York",
		Revision:  1,
		CreatedAt: at.Add(-48 * time.Hour),
		UpdatedAt: at.Add(-48 * time.Hour),
	}
}

func TestAnEventIsValidatedAsItSyncs(t *testing.T) {
	if err := synthetic().Validate(); err != nil {
		t.Fatalf("a good event was refused: %v", err)
	}
	for name, change := range map[string]func(*Record){
		"no id":             func(r *Record) { r.EventID = "" },
		"empty title":       func(r *Record) { r.Title = "" },
		"spaced title":      func(r *Record) { r.Title = " Dentist" },
		"two-line title":    func(r *Record) { r.Title = "Dentist\nthen lunch" },
		"long title":        func(r *Record) { r.Title = strings.Repeat("a", MaxTitleRunes+1) },
		"ends as it starts": func(r *Record) { r.EndAt = r.StartAt },
		"ends before":       func(r *Record) { r.EndAt = r.StartAt.Add(-time.Minute) },
		"over a day":        func(r *Record) { r.EndAt = r.StartAt.Add(MaxLength + time.Minute) },
		"no zone":           func(r *Record) { r.ZoneID = "" },
		"unknown zone":      func(r *Record) { r.ZoneID = "Not/AZone" },
		"no revision":       func(r *Record) { r.Revision = 0 },
		"updated first":     func(r *Record) { r.UpdatedAt = r.CreatedAt.Add(-time.Second) },
		"no creation":       func(r *Record) { r.CreatedAt = time.Time{} },
		"id too long":       func(r *Record) { r.EventID = "e" + strings.Repeat("x", 61) },
	} {
		record := synthetic()
		change(&record)
		if record.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	long := synthetic()
	long.Title = strings.Repeat("é", MaxTitleRunes)
	long.EndAt = long.StartAt.Add(MaxLength)
	if err := long.Validate(); err != nil {
		t.Errorf("a full-length title and a day-long event were refused: %v", err)
	}
}

func TestARevisionRecordNamesItsEvent(t *testing.T) {
	if RecordID("event_dentist_01", 3) != "event_dentist_01_r3" {
		t.Fatalf("record id %q", RecordID("event_dentist_01", 3))
	}
	if id, revision, ok := EventOf("event_dentist_01_r3"); !ok || id != "event_dentist_01" || revision != 3 {
		t.Errorf("read %q, %d, %v", id, revision, ok)
	}
	for _, bad := range []string{"event_dentist_01", "event_dentist_01_r0", "event_dentist_01_rx", "_r1"} {
		if _, _, ok := EventOf(bad); ok {
			t.Errorf("%q read as a revision", bad)
		}
	}
}

// Two devices that change one event apart make two revisions of one number.
// Every device keeps the same one, whichever arrives first.
func TestEveryDeviceSettlesOnOneRevision(t *testing.T) {
	first := synthetic()
	later := first
	later.Revision = 2
	later.UpdatedAt = first.UpdatedAt.Add(time.Hour)
	if !Wins(later, []byte("b"), first, []byte("a")) || Wins(first, []byte("a"), later, []byte("b")) {
		t.Error("the higher revision did not win")
	}

	here, there := later, later
	there.Title = "Synthetic dentist, moved"
	there.UpdatedAt = here.UpdatedAt.Add(time.Minute)
	if !Wins(there, []byte("x"), here, []byte("y")) || Wins(here, []byte("y"), there, []byte("x")) {
		t.Error("the later update of one revision did not win")
	}

	same := here
	if Wins(same, []byte("a"), here, []byte("b")) == Wins(here, []byte("b"), same, []byte("a")) {
		t.Error("two revisions alike in number and time did not settle one way")
	}
	if Wins(here, []byte("a"), here, []byte("a")) {
		t.Error("a revision replaced itself")
	}
}
