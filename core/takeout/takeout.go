// Package takeout reads the times out of a Google Takeout export's My Activity
// (ADR-0059). It keeps when each activity happened, to the minute, and passes
// over everything else as it reads: what was searched, watched or visited,
// links, products and places never leave the decoder.
//
// My Activity is recognised by its shape, a list of entries each with a header
// and a time, rather than by its folder's name, which Takeout translates into
// the account's language.
package takeout

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

// MaxEntries bounds one export. A heavy user's decade of activity is a few
// million entries; past this the file is not one person's My Activity.
const MaxEntries = 10_000_000

// entryBudget bounds the bytes read for one entry. The decoder holds an entry
// whole while it decodes it, and a real one is a few kilobytes, so a file with
// an entry past this is damaged or not My Activity, and is not held.
const entryBudget = 1 << 20

var (
	// ErrNoActivity is a file with nothing that reads as My Activity.
	ErrNoActivity = errors.New("no Google My Activity was found in this file")

	// ErrHTMLExport is My Activity exported as web pages, whose times are
	// written for reading rather than for a program.
	ErrHTMLExport = errors.New("this export holds My Activity as web pages; export it again with My Activity set to JSON")

	// ErrTooLarge is an export with more entries than MaxEntries.
	ErrTooLarge = fmt.Errorf("this export has more than %d activity entries", MaxEntries)
)

// earliest is before any Google service kept activity; an entry dated earlier
// is damaged rather than old. So is one dated more than a day after it was
// read.
var earliest = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// Summary says what was read without saying anything that was in it.
type Summary struct {
	// Files are the My Activity files read.
	Files int

	// Entries are the activity entries read, and Unusable those among them
	// without a time that could be used.
	Entries  int
	Unusable int

	// First and Last are the earliest and latest minutes kept.
	First time.Time
	Last  time.Time
}

// Read reads the export at path: a Takeout .zip, or one My Activity .json file
// taken out of one. It returns each minute in which there was activity, as
// minutes since the Unix epoch, sorted and without repeats. now bounds the
// times that can be right.
func Read(path string, now time.Time) ([]int64, Summary, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, Summary{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, Summary{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, Summary{}, errors.New("the export must be a file")
	}

	reader := &reader{latest: now.Add(24 * time.Hour)}
	head := make([]byte, 4)
	n, _ := io.ReadFull(file, head)
	if bytes.Equal(head[:n], []byte("PK\x03\x04")) {
		archive, err := zip.NewReader(file, info.Size())
		if err != nil {
			return nil, Summary{}, fmt.Errorf("the .zip could not be opened: %w", err)
		}
		err = reader.readArchive(archive)
		return reader.result(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, Summary{}, err
	}
	err = reader.readFile(file)
	return reader.result(err)
}

type reader struct {
	latest  time.Time
	minutes []int64
	summary Summary
	sawHTML bool
}

func (r *reader) readArchive(archive *zip.Reader) error {
	for _, entry := range archive.File {
		name := strings.ToLower(entry.Name)
		switch {
		case strings.HasSuffix(name, ".json"):
			file, err := entry.Open()
			if err != nil {
				return fmt.Errorf("a file in the .zip could not be opened: %w", err)
			}
			err = r.readActivity(file)
			_ = file.Close()
			if err != nil {
				return err
			}
		case strings.HasSuffix(name, ".html") && strings.Contains(name, "activ"):
			// Takeout's HTML export puts an activity page per product. The
			// word differs by language but nearly always contains "activ".
			r.sawHTML = true
		}
	}
	return nil
}

func (r *reader) readFile(file io.Reader) error {
	buffered := bufio.NewReaderSize(file, 1<<16)
	if start, err := buffered.Peek(1); err == nil && start[0] == '<' {
		r.sawHTML = true
		return nil
	}
	return r.readActivity(buffered)
}

// present records that a field was there without copying its value, so a
// header naming a product, or anything after it, is never held.
type present bool

func (p *present) UnmarshalJSON([]byte) error {
	*p = true
	return nil
}

// activityEntry is everything this package reads of an entry. Every other
// field, the title of a search among them, is skipped by the decoder.
type activityEntry struct {
	Header present `json:"header"`
	Time   string  `json:"time"`
}

// readActivity reads one JSON document. A document that is not a list whose
// first entry has a header and a time is not My Activity, and is left unread.
func (r *reader) readActivity(source io.Reader) error {
	budget := &budgetReader{source: source, left: entryBudget}
	decoder := json.NewDecoder(budget)
	token, err := decoder.Token()
	if err != nil {
		return nil
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return nil
	}
	first := true
	for decoder.More() {
		budget.left = entryBudget
		var entry activityEntry
		if err := decoder.Decode(&entry); err != nil {
			if first {
				return nil
			}
			return errors.New("a My Activity file is cut short or damaged")
		}
		if first {
			if !bool(entry.Header) || entry.Time == "" {
				return nil
			}
			first = false
			r.summary.Files++
		}
		r.summary.Entries++
		if r.summary.Entries > MaxEntries {
			return ErrTooLarge
		}
		at, err := time.Parse(time.RFC3339Nano, entry.Time)
		if err != nil || at.Before(earliest) || at.After(r.latest) {
			r.summary.Unusable++
			continue
		}
		r.minutes = append(r.minutes, at.Unix()/60)
	}
	return nil
}

// budgetReader stops reading once an entry has used its budget, which the
// decoder then reports as a damaged list.
type budgetReader struct {
	source io.Reader
	left   int
}

func (b *budgetReader) Read(p []byte) (int, error) {
	if b.left <= 0 {
		return 0, errors.New("an activity entry is larger than My Activity's")
	}
	if len(p) > b.left {
		p = p[:b.left]
	}
	n, err := b.source.Read(p)
	b.left -= n
	return n, err
}

func (r *reader) result(err error) ([]int64, Summary, error) {
	if err != nil {
		return nil, Summary{}, err
	}
	if r.summary.Files == 0 {
		if r.sawHTML {
			return nil, Summary{}, ErrHTMLExport
		}
		return nil, Summary{}, ErrNoActivity
	}
	slices.Sort(r.minutes)
	r.minutes = slices.Compact(r.minutes)
	if len(r.minutes) > 0 {
		r.summary.First = time.Unix(r.minutes[0]*60, 0).UTC()
		r.summary.Last = time.Unix(r.minutes[len(r.minutes)-1]*60, 0).UTC()
	}
	return r.minutes, r.summary, nil
}
