package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestOutlookAnswersTheNextThreeDays is the slice's acceptance in one place: a
// person with a fitted rhythm opens the app and gets a timeline, office
// windows, and a horizon — without asking anything or opening anything else.
func TestOutlookAnswersTheNextThreeDays(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 10, 2*time.Hour)

	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if view.Status != "available" {
		t.Fatalf("status = %q (%s); want an available view on fresh evidence",
			view.Status, view.WithheldMessage)
	}
	if view.HorizonHours < 48 || view.HorizonHours > 96 {
		t.Errorf("horizon = %.0f hours, want the 48-96 band", view.HorizonHours)
	}
	if len(view.Segments) < 3 {
		t.Errorf("timeline has %d stretches; three days should hold more", len(view.Segments))
	}
	if len(view.OfficeWindows) == 0 {
		t.Error("no office windows over three days")
	}
	if view.NextSleepLabel == "" {
		t.Error("no next sleep range")
	}
	if view.AwakeLabel == "" || view.UncertainLabel == "" {
		t.Error("the awake/uncertain summary is missing")
	}
	if len(view.Days) == 0 {
		t.Error("no day marks for the timeline")
	}

	// Offsets must be usable as layout positions without further date maths.
	var previous float64
	for i, segment := range view.Segments {
		if segment.OffsetHours < previous-0.001 {
			t.Errorf("segment %d goes backwards: %.2f after %.2f", i, segment.OffsetHours, previous)
		}
		if segment.DurationHours <= 0 {
			t.Errorf("segment %d has no duration", i)
		}
		previous = segment.OffsetHours
	}
	last := view.Segments[len(view.Segments)-1]
	if total := last.OffsetHours + last.DurationHours; total < view.HorizonHours-0.001 || total > view.HorizonHours+0.001 {
		t.Errorf("the timeline covers %.2f hours of a %.2f-hour horizon", total, view.HorizonHours)
	}
}

// TestOutlookShowsUncertainBoundaries. The whole reason this view exists rather
// than a two-colour bar is that the measured P90 onset error is over five
// hours; a sharp line would be a lie with a number attached.
func TestOutlookShowsUncertainBoundaries(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 10, 2*time.Hour)

	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if view.Status != "available" {
		t.Skipf("estimator refused on this fixture (%s)", view.Status)
	}
	found := map[string]bool{}
	for _, segment := range view.Segments {
		found[segment.Presence] = true
	}
	for _, presence := range []string{"awake", "asleep", "uncertain"} {
		if !found[presence] {
			t.Errorf("no %q stretch in three days of forecast", presence)
		}
	}
}

// TestOutlookIsWithheldOnStaleEvidence. A forecast is anchored to where the
// person is now in their cycle; with no anchor, drawing office windows over it
// would invite someone to book a call against arithmetic.
func TestOutlookIsWithheldOnStaleEvidence(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 10, 4*24*time.Hour)

	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if view.Status != "withheld" {
		t.Fatalf("status = %q after four days without a record, want withheld", view.Status)
	}
	if len(view.Segments) != 0 || len(view.OfficeWindows) != 0 {
		t.Error("a withheld view still drew a timeline")
	}
	if view.WithheldMessage == "" {
		t.Error("nothing explains the withholding to the reader")
	}
	if view.Freshness.State != "withheld" {
		t.Errorf("freshness state = %q, want withheld", view.Freshness.State)
	}
}

// TestOutlookRefusesWithoutHistory rather than drawing an empty three days,
// which would read as "asleep until Friday".
func TestOutlookRefusesWithoutHistory(t *testing.T) {
	app := newTestApp(t)

	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if view.Status != "refused" {
		t.Fatalf("status = %q with no sleep history, want refused", view.Status)
	}
	if view.Refusal == nil || view.Refusal.Message == "" {
		t.Error("the refusal carries no message")
	}
	if len(view.Segments) != 0 {
		t.Error("a refused view drew a timeline")
	}
}

// TestOutlookOfficeWindowsAreLabelledHonestly. "Partial" must never be
// presented as reachable: it means the only overlap sits on a boundary the
// model has not pinned down.
func TestOutlookOfficeWindowsAreLabelledHonestly(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 10, 2*time.Hour)

	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if view.Status != "available" {
		t.Skipf("estimator refused on this fixture (%s)", view.Status)
	}
	for i, window := range view.OfficeWindows {
		switch window.Status {
		case "reachable":
			if window.ReachableLabel == "" {
				t.Errorf("office window %d is reachable but names no time", i)
			}
		case "partial":
			if window.ReachableLabel != "" {
				t.Errorf("office window %d is only possible but advertises a reachable time", i)
			}
			if !strings.Contains(window.Detail, "uncertain") {
				t.Errorf("office window %d does not say why it is only possible: %q", i, window.Detail)
			}
		case "unreachable":
			if window.ReachableLabel != "" {
				t.Errorf("office window %d is unreachable but names a time", i)
			}
		default:
			t.Errorf("office window %d has an unknown status %q", i, window.Status)
		}
		if window.Detail == "" {
			t.Errorf("office window %d has no explanation", i)
		}
	}
}

// TestOutlookCarriesNoUnrenderedPrivateText. The view is local, so event and
// task titles belong in it — but nothing else from those records does, and the
// core package it is built on never sees the text at all.
func TestOutlookCarriesNoUnrenderedPrivateText(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 10, 2*time.Hour)

	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Estimator internals have no business on a planning screen; the same
	// narrowing the portal applies to a visitor applies here to the reader.
	for _, leak := range []string{"algorithmVersion", "inputSessionIds", "characteristicSleepStart"} {
		if strings.Contains(string(encoded), leak) {
			t.Errorf("estimator internal %q reached the outlook DTO", leak)
		}
	}
}

// TestOutlookHorizonIsCivilTimeAware keeps the day marks on civil midnights,
// which is what a person means by "tomorrow".
func TestOutlookHorizonIsCivilTimeAware(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 10, 2*time.Hour)

	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if view.Status != "available" {
		t.Skipf("estimator refused on this fixture (%s)", view.Status)
	}
	if len(view.Days) < 2 {
		t.Fatalf("day marks = %d over three days, want at least 2", len(view.Days))
	}
	for i := 1; i < len(view.Days); i++ {
		gap := view.Days[i].OffsetHours - view.Days[i-1].OffsetHours
		// 23 or 25 on a daylight-saving boundary, 24 otherwise.
		if gap < 22.9 || gap > 25.1 {
			t.Errorf("day marks %d and %d are %.2f hours apart", i-1, i, gap)
		}
	}
}
