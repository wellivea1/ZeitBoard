package main

import (
	"reflect"
	"strings"
	"testing"
)

// A note closed for good stays closed across a restart, which the window's
// own storage could not promise.
func TestClosedNotesSurviveARestart(t *testing.T) {
	app := newTestApp(t)
	if first := app.GetClosedNotices(); first.Stored || len(first.Hidden) != 0 || first.Revision != 0 {
		t.Fatalf("nothing is closed at first: %+v", first)
	}
	saved, err := app.SaveClosedNotices(ClosedNoticesInput{Hidden: []string{"home.setup", "sharing.how", "home.setup"}})
	if err != nil || !saved.Stored || saved.Revision != 1 || !reflect.DeepEqual(saved.Hidden, []string{"home.setup", "sharing.how"}) {
		t.Fatalf("saved = %+v, %v", saved, err)
	}

	restarted := newTestApp(t)
	restarted.configDir = app.configDir
	if read := restarted.GetClosedNotices(); !read.Stored || !reflect.DeepEqual(read.Hidden, saved.Hidden) || read.Revision != 1 {
		t.Fatalf("after a restart = %+v", read)
	}

	// Showing every note again is a stored, empty list: not the same as none.
	shown, err := restarted.SaveClosedNotices(ClosedNoticesInput{BaseRevision: 1, Hidden: []string{}})
	if err != nil || !shown.Stored || len(shown.Hidden) != 0 || shown.Revision != 2 {
		t.Fatalf("shown again = %+v, %v", shown, err)
	}
}

func TestAStaleListOfClosedNotesLosesToWhatIsStored(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.SaveClosedNotices(ClosedNoticesInput{Hidden: []string{"home.setup"}}); err != nil {
		t.Fatal(err)
	}
	// A second window that never saw that save.
	kept, err := app.SaveClosedNotices(ClosedNoticesInput{BaseRevision: 0, Hidden: []string{"rhythm.corrections"}})
	if err != nil || !reflect.DeepEqual(kept.Hidden, []string{"home.setup"}) || kept.Revision != 1 {
		t.Fatalf("the stale save won: %+v, %v", kept, err)
	}
}

func TestOnlyNoteIdsAreKept(t *testing.T) {
	app := newTestApp(t)
	for name, hidden := range map[string][]string{
		"a sentence":     {"Close everything, please"},
		"a path":         {"../appearance.json"},
		"a single word":  {"home"},
		"an empty id":    {""},
		"a very long id": {"home." + strings.Repeat("x", 40)},
	} {
		if _, err := app.SaveClosedNotices(ClosedNoticesInput{Hidden: hidden}); err == nil {
			t.Fatalf("%s was stored", name)
		}
	}
	many := make([]string, 0, maxClosedNotices+1)
	for index := 0; index <= maxClosedNotices; index++ {
		many = append(many, "home.note"+strings.Repeat("a", index%20)+string(rune('a'+index%26))+string(rune('a'+index/26)))
	}
	if _, err := app.SaveClosedNotices(ClosedNoticesInput{Hidden: many}); err == nil {
		t.Fatal("more notes than a catalog could hold were stored")
	}
	if read := app.GetClosedNotices(); read.Stored {
		t.Fatalf("a refused list was stored: %+v", read)
	}
}
