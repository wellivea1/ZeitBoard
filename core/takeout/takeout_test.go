package takeout

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Every value that is not a time is a canary: it must never come back.
const searchActivity = `[
  {"header": "Search", "title": "Searched for canary-search-terms", "titleUrl": "https://www.google.com/search?q=canary", "time": "2026-03-01T22:14:09.123Z", "products": ["Search"]},
  {"header": "Search", "title": "Visited canary-page", "time": "2026-03-02T07:40:00Z", "products": ["Search"], "locationInfos": [{"name": "canary-place"}]},
  {"header": "Search", "title": "An entry without a time"}
]`

const youtubeActivity = `[
  {"header": "YouTube", "title": "Watched canary-video", "time": "2026-03-01T22:14:51Z", "subtitles": [{"name": "canary-channel"}]},
  {"header": "YouTube", "title": "Watched canary-from-the-future", "time": "2031-01-01T00:00:00Z"}
]`

// readAt is when the exports here are read: an entry dated long after it is
// damaged.
var readAt = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "takeout-20260310T120000Z-001.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, contents := range files {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFile(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func minute(value string) int64 {
	at, _ := time.Parse(time.RFC3339, value)
	return at.Unix() / 60
}

// A Takeout .zip gives up each minute that had activity, across products and
// whatever the folders are called, and nothing else.
func TestAnExportGivesUpItsMinutes(t *testing.T) {
	path := writeZip(t, map[string]string{
		"Takeout/My Activity/Search/MyActivity.json":              searchActivity,
		"Takeout/Meine Aktivitäten/YouTube/MeineAktivitäten.json": youtubeActivity,
		"Takeout/Chrome/BrowserHistory.json":                      `{"Browser History": [{"title": "canary-history", "time_usec": 1}]}`,
		"Takeout/archive_browser.html":                            "<html></html>",
	})
	minutes, summary, err := Read(path, readAt)
	if err != nil {
		t.Fatal(err)
	}
	// The two activities in the same minute are one minute.
	want := []int64{minute("2026-03-01T22:14:00Z"), minute("2026-03-02T07:40:00Z")}
	if len(minutes) != 2 || minutes[0] != want[0] || minutes[1] != want[1] {
		t.Fatalf("minutes %v, want %v", minutes, want)
	}
	if summary.Files != 2 || summary.Entries != 5 || summary.Unusable != 2 {
		t.Errorf("summary %+v", summary)
	}
	if !summary.First.Equal(time.Date(2026, 3, 1, 22, 14, 0, 0, time.UTC)) ||
		!summary.Last.Equal(time.Date(2026, 3, 2, 7, 40, 0, 0, time.UTC)) {
		t.Errorf("span %s to %s", summary.First, summary.Last)
	}
}

func TestOneMyActivityFileIsReadOnItsOwn(t *testing.T) {
	minutes, summary, err := Read(writeFile(t, "MyActivity.json", searchActivity), readAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(minutes) != 2 || summary.Files != 1 {
		t.Errorf("minutes %v, summary %+v", minutes, summary)
	}
}

// My Activity exported as web pages is refused with how to export it instead,
// whether as the .zip or as one of its pages.
func TestAnHTMLExportIsRefusedWithWhatToDo(t *testing.T) {
	zipped := writeZip(t, map[string]string{"Takeout/My Activity/Search/MyActivity.html": "<html>canary</html>"})
	if _, _, err := Read(zipped, readAt); !errors.Is(err, ErrHTMLExport) {
		t.Errorf(".zip: %v", err)
	}
	if _, _, err := Read(writeFile(t, "MyActivity.html", "<!DOCTYPE html><html></html>"), readAt); !errors.Is(err, ErrHTMLExport) {
		t.Errorf("page: %v", err)
	}
}

func TestAFileWithoutActivityIsRefused(t *testing.T) {
	for name, contents := range map[string]string{
		"history.json": `{"Browser History": []}`,
		"list.json":    `[{"title": "no time or header"}]`,
		"empty.json":   ``,
		"notes.txt":    `canary`,
	} {
		if _, _, err := Read(writeFile(t, name, contents), readAt); !errors.Is(err, ErrNoActivity) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestADamagedListIsAnError(t *testing.T) {
	_, _, err := Read(writeFile(t, "MyActivity.json", `[{"header": "Search", "time": "2026-03-01T22:14:09Z"}, {"header": `), readAt)
	if err == nil || errors.Is(err, ErrNoActivity) {
		t.Fatalf("a damaged list read as %v", err)
	}
}

// An entry far larger than any in My Activity is not held whole: the list
// reads as damaged instead.
func TestAnOversizedEntryIsNotHeld(t *testing.T) {
	huge := `[{"header": "Search", "time": "2026-03-01T22:14:09Z"}, {"header": "Search", "title": "` +
		strings.Repeat("x", 2*entryBudget) + `", "time": "2026-03-01T23:14:09Z"}]`
	_, _, err := Read(writeFile(t, "MyActivity.json", huge), readAt)
	if err == nil || errors.Is(err, ErrNoActivity) {
		t.Fatalf("an oversized entry read as %v", err)
	}
}

// Whatever a file holds, reading it never panics, and what comes back is only
// minutes, in order and once each.
func FuzzReadActivity(f *testing.F) {
	for _, seed := range []string{
		searchActivity,
		youtubeActivity,
		`[{"header": "Search", "time": "2026-03-01T22:14:09Z"}, {"header": `,
		`{"Browser History": []}`,
		`[]`,
		`[{"header": null, "time": "2026-03-01T22:14:09Z"}]`,
		`[{"header": "Search", "time": "1999-12-31T23:59:59Z"}, {"header": "x", "time": "2026-13-01T00:00:00Z"}]`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r := &reader{latest: readAt.Add(24 * time.Hour)}
		minutes, summary, err := r.result(r.readActivity(bytes.NewReader(data)))
		if err != nil {
			return
		}
		if summary.Files != 1 || len(minutes) > summary.Entries-summary.Unusable {
			t.Fatalf("summary %+v for %d minutes", summary, len(minutes))
		}
		for i := 1; i < len(minutes); i++ {
			if minutes[i] <= minutes[i-1] {
				t.Fatalf("minutes out of order: %v", minutes)
			}
		}
	})
}
