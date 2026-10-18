package main

import (
	"runtime"
	"strings"
	"testing"

	"non24.app/core/civilfmt"
)

// The window's clock belongs to the process, so a test that changes it puts
// the twelve-hour clock back for the tests after it.
func restoreClockAfter(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { windowClock.Store(uint32(civilfmt.TwelveHour)) })
}

func saveClock(t *testing.T, app *App, preference string) ClockSettingDTO {
	t.Helper()
	current, err := app.GetClockSetting()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := app.SaveClockSetting(ClockSettingInput{Preference: preference, BaseRevision: current.Revision})
	if err != nil {
		t.Fatalf("save %s: %v", preference, err)
	}
	return saved
}

func TestTheClockFollowsTheComputerUntilChosen(t *testing.T) {
	restoreClockAfter(t)
	app := newTestApp(t)
	app.systemClock = civilfmt.TwentyFourHour
	if err := app.loadRegionalFromDisk(); err != nil {
		t.Fatal(err)
	}
	setting, err := app.GetClockSetting()
	if err != nil {
		t.Fatal(err)
	}
	if setting.Preference != clockSystem || setting.Clock != clock24 || setting.System != clock24 ||
		setting.Revision != 0 || setting.Platform != platformName(runtime.GOOS) {
		t.Fatalf("setting %+v", setting)
	}
	if displayClock() != civilfmt.TwentyFourHour {
		t.Error("the computer's own clock is not in use")
	}

	if chosen := saveClock(t, app, clock12); chosen.Clock != clock12 || chosen.System != clock24 {
		t.Fatalf("chosen %+v", chosen)
	}
	if displayClock() != civilfmt.TwelveHour {
		t.Error("the chosen clock is not in use")
	}
}

// Every label the desktop writes moves with the clock: here, Home's reaching
// row, which reads stored "HH:MM" hours.
func TestLabelsAreWrittenOnTheChosenClock(t *testing.T) {
	restoreClockAfter(t)
	app := newTestApp(t)
	contact := ReachingContactDTO{Label: "The clinic", ZoneID: defaultZoneID, Hours: weekdayHours("09:00", "13:00", 2)}
	if summary := reachingSummary(contact); !strings.Contains(summary, "9:00 AM to 1:00 PM") {
		t.Fatalf("twelve-hour summary %q", summary)
	}
	saveClock(t, app, clock24)
	if summary := reachingSummary(contact); !strings.Contains(summary, "09:00 to 13:00") || strings.Contains(summary, "AM") {
		t.Errorf("twenty-four-hour summary %q", summary)
	}
}

func TestAChosenClockSurvivesARestart(t *testing.T) {
	restoreClockAfter(t)
	app := newTestApp(t)
	saveClock(t, app, clock24)
	windowClock.Store(uint32(civilfmt.TwelveHour))

	restarted := newTestApp(t)
	restarted.configDir = app.configDir
	if err := restarted.loadRegionalFromDisk(); err != nil {
		t.Fatal(err)
	}
	setting, err := restarted.GetClockSetting()
	if err != nil {
		t.Fatal(err)
	}
	if setting.Preference != clock24 || setting.Revision == 0 || displayClock() != civilfmt.TwentyFourHour {
		t.Fatalf("restored %+v with the window on %v", setting, displayClock())
	}
}

func TestAClockTheAppDoesNotKnowIsRefused(t *testing.T) {
	restoreClockAfter(t)
	app := newTestApp(t)
	if _, err := app.SaveClockSetting(ClockSettingInput{Preference: "13"}); err == nil ||
		!strings.Contains(err.Error(), "12-hour or 24-hour") {
		t.Fatalf("an unknown clock was accepted: %v", err)
	}
	if displayClock() != civilfmt.TwelveHour {
		t.Error("a refused clock was put in use")
	}
}

func TestAStaleClockEditLosesToWhatIsStored(t *testing.T) {
	restoreClockAfter(t)
	app := newTestApp(t)
	first := saveClock(t, app, clock24)
	stale, err := app.SaveClockSetting(ClockSettingInput{Preference: clock12, BaseRevision: first.Revision - 1})
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Conflict || stale.Preference != clock24 || displayClock() != civilfmt.TwentyFourHour {
		t.Fatalf("the stale edit was applied: %+v", stale)
	}
}

// A stored clock that cannot be read leaves the computer's own in use, named
// as the clock setting names it on each system.
func TestAnUnreadableClockSaysWhichIsInUse(t *testing.T) {
	if message := clockUnreadable("windows"); !strings.Contains(message, "Windows' own") {
		t.Errorf("windows is told %q", message)
	}
	for _, goos := range []string{"darwin", "linux"} {
		if message := clockUnreadable(goos); strings.Contains(message, "Windows") || !strings.Contains(message, "ZeitBoard's default") {
			t.Errorf("%s is told %q", goos, message)
		}
	}
}
