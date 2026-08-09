package main

import (
	"strings"
	"testing"
	"time"
)

// Reaching hours describe the other party, not the user. Monday to Friday,
// nine to five, is a starting point and not a description of anybody's life.

func TestTheDefaultScheduleIsTheOneTheOutlookAlreadyUsed(t *testing.T) {
	app := newTestApp(t)
	envelope, err := app.GetReachingHours()
	if err != nil {
		t.Fatal(err)
	}
	if !envelope.State.Enabled {
		t.Error("reaching hours start switched off, so a new install shows nothing")
	}
	if envelope.State.StartLocal != "09:00" || envelope.State.EndLocal != "17:00" {
		t.Errorf("default hours are %s-%s", envelope.State.StartLocal, envelope.State.EndLocal)
	}
	if len(envelope.State.Days) != 5 {
		t.Errorf("%d default days, want the five weekdays", len(envelope.State.Days))
	}
	if envelope.Revision != 0 {
		t.Errorf("revision %d before anything was saved", envelope.Revision)
	}
}

func TestASavedScheduleReachesTheOutlook(t *testing.T) {
	app := newTestApp(t)
	saved, err := app.SaveReachingHours(ReachingHoursSaveInput{
		State: ReachingHoursDTO{
			Enabled:    true,
			Label:      "The all-night pharmacy",
			StartLocal: "20:00",
			EndLocal:   "08:00",
			Days:       []int{0, 1, 2, 3, 4, 5, 6},
			ZoneID:     defaultZoneID,
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Conflict {
		t.Fatal("a first save reported a conflict")
	}

	hours := app.currentReachingHours().officeHours(defaultZoneID)
	if hours.StartLocal != "20:00" || hours.EndLocal != "08:00" {
		t.Errorf("outlook received %s-%s", hours.StartLocal, hours.EndLocal)
	}
	if len(hours.Days) != 7 {
		t.Errorf("outlook received %d open days, want 7", len(hours.Days))
	}
}

// A schedule switched off must produce no windows rather than fall back to a
// default the person did not choose. Silence is the honest answer to "nobody to
// reach"; nine to five is a guess.
func TestTurningReachingHoursOffLeavesNoOpenDays(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.SaveReachingHours(ReachingHoursSaveInput{
		State: ReachingHoursDTO{
			Enabled:    false,
			StartLocal: "09:00",
			EndLocal:   "17:00",
			Days:       []int{1, 2, 3, 4, 5},
			ZoneID:     defaultZoneID,
		},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	hours := app.currentReachingHours().officeHours(defaultZoneID)
	if len(hours.Days) != 0 {
		t.Errorf("a disabled schedule still offers %d open days", len(hours.Days))
	}
	if hours.Days == nil {
		t.Error("nil days would let core fall back to its own weekday default")
	}
}

func TestAScheduleTheAppCannotHonourIsRefused(t *testing.T) {
	app := newTestApp(t)
	for _, testCase := range []struct {
		name  string
		state ReachingHoursDTO
		want  string
	}{
		{
			"an enabled schedule with no days",
			ReachingHoursDTO{Enabled: true, StartLocal: "09:00", EndLocal: "17:00", ZoneID: defaultZoneID},
			"at least one day",
		},
		{
			"a zone this computer does not know",
			ReachingHoursDTO{
				Enabled: true, StartLocal: "09:00", EndLocal: "17:00",
				Days: []int{1}, ZoneID: "Mars/Olympus_Mons",
			},
			"time zone",
		},
		{
			"a clock that is not a clock",
			ReachingHoursDTO{
				Enabled: true, StartLocal: "9am", EndLocal: "17:00",
				Days: []int{1}, ZoneID: defaultZoneID,
			},
			"civil time",
		},
		{
			"a weekday that does not exist",
			ReachingHoursDTO{
				Enabled: true, StartLocal: "09:00", EndLocal: "17:00",
				Days: []int{9}, ZoneID: defaultZoneID,
			},
			"Sunday through Saturday",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := app.SaveReachingHours(ReachingHoursSaveInput{State: testCase.state})
			if err == nil {
				t.Fatal("it was accepted")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("error %q does not say what is wrong", err)
			}
		})
	}
}

// The revision guard is what stops a second window, or the agent, quietly
// overwriting a schedule someone just edited.
func TestAStaleEditLosesToWhatIsStored(t *testing.T) {
	app := newTestApp(t)
	first, err := app.SaveReachingHours(ReachingHoursSaveInput{
		State: ReachingHoursDTO{
			Enabled: true, StartLocal: "08:00", EndLocal: "12:00",
			Days: []int{2}, ZoneID: defaultZoneID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := app.SaveReachingHours(ReachingHoursSaveInput{
		BaseRevision: first.Revision - 1,
		State: ReachingHoursDTO{
			Enabled: true, StartLocal: "23:00", EndLocal: "23:30",
			Days: []int{6}, ZoneID: defaultZoneID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Conflict {
		t.Fatal("the stale edit did not report a conflict")
	}
	if stale.State.StartLocal != "08:00" {
		t.Errorf("the stale edit overwrote the stored schedule: %+v", stale.State)
	}
}

// The summary is what the outlook prints. It has to say these are the person's
// own hours rather than assert them as a fact about the world, and it has to
// describe an overnight window as overnight.
func TestTheSummaryDescribesTheScheduleHonestly(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		state    ReachingHoursDTO
		contains []string
		absent   []string
	}{
		{
			name: "an ordinary working week",
			state: ReachingHoursDTO{
				Enabled: true, Label: "Typical office hours",
				StartLocal: "09:00", EndLocal: "17:00",
				Days: []int{1, 2, 3, 4, 5}, ZoneID: defaultZoneID,
			},
			contains: []string{"you set", "Monday to Friday", "9:00 AM to 5:00 PM"},
			absent:   []string{"next day"},
		},
		{
			name: "a night that crosses midnight",
			state: ReachingHoursDTO{
				Enabled: true, Label: "The night desk",
				StartLocal: "22:00", EndLocal: "06:00",
				Days: []int{5}, ZoneID: defaultZoneID,
			},
			contains: []string{"Friday", "10:00 PM to 6:00 AM the next day"},
		},
		{
			name: "a service that never closes",
			state: ReachingHoursDTO{
				Enabled: true, Label: "Crisis line",
				StartLocal: "00:00", EndLocal: "00:00",
				Days: []int{0, 1, 2, 3, 4, 5, 6}, ZoneID: defaultZoneID,
			},
			contains: []string{"every day", "all day"},
		},
		{
			name: "days that are not a run",
			state: ReachingHoursDTO{
				Enabled: true, Label: "The clinic",
				StartLocal: "09:00", EndLocal: "13:00",
				Days: []int{2, 4}, ZoneID: defaultZoneID,
			},
			contains: []string{"Tuesday and Thursday"},
			absent:   []string{"Tuesday to Thursday"},
		},
		{
			name:     "nothing set",
			state:    ReachingHoursDTO{Enabled: false},
			contains: []string{"No reaching hours are set"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			summary := reachingSummary(testCase.state)
			for _, want := range testCase.contains {
				if !strings.Contains(summary, want) {
					t.Errorf("summary %q is missing %q", summary, want)
				}
			}
			for _, unwanted := range testCase.absent {
				if strings.Contains(summary, unwanted) {
					t.Errorf("summary %q should not contain %q", summary, unwanted)
				}
			}
		})
	}
}

// Hours in another zone are the reason this is configurable at all, so the
// summary must name the zone rather than let the reader assume their own.
func TestForeignHoursNameTheirZone(t *testing.T) {
	local := localZoneID()
	foreign := "Europe/Berlin"
	if local == foreign {
		foreign = "Asia/Tokyo"
	}
	summary := reachingSummary(ReachingHoursDTO{
		Enabled: true, Label: "My sister",
		StartLocal: "18:00", EndLocal: "22:00",
		Days: []int{0, 6}, ZoneID: foreign,
	})
	if !strings.Contains(summary, foreign) {
		t.Errorf("summary %q does not say whose clock these hours are on", summary)
	}
	if strings.Contains(reachingSummary(ReachingHoursDTO{
		Enabled: true, Label: "Local", StartLocal: "09:00", EndLocal: "17:00",
		Days: []int{1}, ZoneID: local,
	}), local) {
		t.Error("the reader's own zone is repeated back at them")
	}
}

func TestASavedScheduleSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(t)
	app.configDir = dir
	if _, err := app.SaveReachingHours(ReachingHoursSaveInput{
		State: ReachingHoursDTO{
			Enabled: true, Label: "The clinic",
			StartLocal: "07:30", EndLocal: "11:30",
			Days: []int{2, 4}, ZoneID: defaultZoneID,
		},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	restarted := newTestApp(t)
	restarted.configDir = dir
	if err := restarted.loadReachingFromDisk(); err != nil {
		t.Fatalf("load: %v", err)
	}
	state := restarted.currentReachingHours()
	if state.Label != "The clinic" || state.StartLocal != "07:30" {
		t.Fatalf("restored %+v", state)
	}
	envelope, err := restarted.GetReachingHours()
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Revision == 0 {
		t.Error("the restored schedule has no revision, so the next edit cannot detect a conflict")
	}
}

// The outlook is the whole reason this setting exists, so the label it prints
// has to follow the setting rather than a constant.
func TestTheOutlookPrintsTheScheduleTheUserChose(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 14)
	if _, err := app.SaveReachingHours(ReachingHoursSaveInput{
		State: ReachingHoursDTO{
			Enabled: true, Label: "The all-night pharmacy",
			StartLocal: "00:00", EndLocal: "00:00",
			Days: []int{0, 1, 2, 3, 4, 5, 6}, ZoneID: defaultZoneID,
		},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if strings.Contains(view.OfficeHoursLabel, "Monday to Friday") {
		t.Errorf("the outlook still prints the old default: %q", view.OfficeHoursLabel)
	}
	if !strings.Contains(view.OfficeHoursLabel, "The all-night pharmacy") {
		t.Errorf("the outlook label %q ignores the saved schedule", view.OfficeHoursLabel)
	}
	if view.Status == string("available") && len(view.OfficeWindows) == 0 {
		t.Error("an always-open schedule produced no reaching windows")
	}
}

func TestTurningReachingHoursOffSilencesTheOutlook(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 14)
	if _, err := app.SaveReachingHours(ReachingHoursSaveInput{
		State: ReachingHoursDTO{
			Enabled: false, StartLocal: "09:00", EndLocal: "17:00",
			Days: []int{1, 2, 3, 4, 5}, ZoneID: defaultZoneID,
		},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if len(view.OfficeWindows) != 0 {
		t.Errorf("%d reaching windows with the schedule switched off", len(view.OfficeWindows))
	}
	if !strings.Contains(view.OfficeHoursLabel, "No reaching hours") {
		t.Errorf("label %q does not explain the empty section", view.OfficeHoursLabel)
	}
}

func TestTheStoredScheduleIsNotHealthData(t *testing.T) {
	// A settings file sits outside the encrypted-at-rest discussion for the
	// observation store, so it must not carry anything about the person's
	// sleep. This is a shape assertion, cheap and worth keeping.
	state := ReachingHoursDTO{
		Enabled: true, Label: "The clinic", StartLocal: "09:00", EndLocal: "17:00",
		Days: []int{2}, ZoneID: defaultZoneID,
	}
	if _, err := time.LoadLocation(state.ZoneID); err != nil {
		t.Fatalf("the test's own zone is unusable: %v", err)
	}
	if err := validateReachingHours(state); err != nil {
		t.Fatalf("a plain schedule was rejected: %v", err)
	}
}
