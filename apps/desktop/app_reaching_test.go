package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Reaching hours describe other people, not the user. Monday to Friday, nine
// to five, is a starting point and not a description of anybody's life.

func weekdayHours(start, end string, days ...int) []ReachingDayHoursDTO {
	hours := make([]ReachingDayHoursDTO, 0, len(days))
	for _, day := range days {
		hours = append(hours, ReachingDayHoursDTO{Day: day, StartLocal: start, EndLocal: end})
	}
	return hours
}

func everyDay(start, end string) []ReachingDayHoursDTO {
	return weekdayHours(start, end, 0, 1, 2, 3, 4, 5, 6)
}

func saveReaching(t *testing.T, app *App, contacts ...ReachingContactDTO) ReachingHoursEnvelopeDTO {
	t.Helper()
	current, err := app.GetReachingHours()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := app.SaveReachingHours(ReachingHoursSaveInput{
		State: ReachingHoursDTO{Contacts: contacts}, BaseRevision: current.Revision,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Conflict {
		t.Fatal("a save from the current revision reported a conflict")
	}
	return saved
}

func TestTheDefaultListIsTheWorkingWeekTheOutlookAlwaysUsed(t *testing.T) {
	app := newTestApp(t)
	envelope, err := app.GetReachingHours()
	if err != nil {
		t.Fatal(err)
	}
	if len(envelope.State.Contacts) != 1 {
		t.Fatalf("a new install lists %d people, want one", len(envelope.State.Contacts))
	}
	contact := envelope.State.Contacts[0]
	if contact.Off {
		t.Error("the default starts switched off, so a new install shows nothing")
	}
	if !reachingContactIDPattern.MatchString(contact.ID) {
		t.Errorf("default id %q", contact.ID)
	}
	if len(contact.Hours) != 5 {
		t.Fatalf("%d default days, want the five weekdays", len(contact.Hours))
	}
	for _, day := range contact.Hours {
		if day.Day < 1 || day.Day > 5 || day.StartLocal != "09:00" || day.EndLocal != "17:00" {
			t.Errorf("default day %+v", day)
		}
	}
	if envelope.Revision != 0 {
		t.Errorf("revision %d before anything was saved", envelope.Revision)
	}
}

// Each person reaches the outlook as their own schedule, with their own hours
// on each day, and comes back as their own row in the order of the list.
func TestEachPersonReachesTheOutlookWithTheirOwnHours(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 14)
	saved := saveReaching(t, app,
		ReachingContactDTO{
			Label: "The clinic", ZoneID: defaultZoneID,
			Hours: append(weekdayHours("09:00", "13:00", 2), weekdayHours("13:00", "17:00", 4)...),
		},
		ReachingContactDTO{Label: "Crisis line", ZoneID: defaultZoneID, Hours: everyDay("00:00", "00:00")},
	)
	clinic, line := saved.State.Contacts[0], saved.State.Contacts[1]
	if clinic.ID == "" || line.ID == "" || clinic.ID == line.ID {
		t.Fatalf("ids %q and %q", clinic.ID, line.ID)
	}

	schedules := app.currentReachingHours().officeHours(defaultZoneID)
	if len(schedules) != 2 || schedules[0].ID != clinic.ID || schedules[1].ID != line.ID {
		t.Fatalf("outlook received %+v", schedules)
	}
	if open := schedules[0].Open; len(open) != 2 || open[0].StartLocal != "09:00" || open[1].StartLocal != "13:00" {
		t.Errorf("the clinic's days reached the outlook as %+v", open)
	}

	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if view.Status != "available" {
		t.Skipf("estimator refused on this fixture (%s)", view.Status)
	}
	if len(view.Reaching) != 2 || view.Reaching[0].Label != "The clinic" || view.Reaching[1].Label != "Crisis line" {
		t.Fatalf("rows %+v", view.Reaching)
	}
	if !strings.Contains(view.Reaching[0].HoursLabel, "Tuesday, 9:00 AM to 1:00 PM; Thursday, 1:00 PM to 5:00 PM") {
		t.Errorf("the clinic's row says %q", view.Reaching[0].HoursLabel)
	}
	if len(view.Reaching[1].Windows) == 0 {
		t.Error("a line that never closes has no window in three days")
	}
	for _, window := range view.Reaching[0].Windows {
		if !strings.HasPrefix(window.DayLabel, "Tue") && !strings.HasPrefix(window.DayLabel, "Thu") {
			t.Errorf("the clinic has a window on %s", window.DayLabel)
		}
	}
}

// Switching someone off keeps their hours but takes them off Home. Silence is
// the honest answer to "nobody to reach"; nine to five is a guess.
func TestSomeoneSwitchedOffKeepsTheirHoursButLeavesHome(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 14)
	saveReaching(t, app, ReachingContactDTO{
		Label: "The pharmacy", ZoneID: defaultZoneID, Off: true, Hours: everyDay("08:00", "20:00"),
	})
	if schedules := app.currentReachingHours().officeHours(defaultZoneID); len(schedules) != 0 {
		t.Errorf("someone switched off still reaches the outlook: %+v", schedules)
	}
	envelope, err := app.GetReachingHours()
	if err != nil {
		t.Fatal(err)
	}
	if len(envelope.State.Contacts) != 1 || len(envelope.State.Contacts[0].Hours) != 7 {
		t.Errorf("switching off lost the hours: %+v", envelope.State)
	}
	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if len(view.Reaching) != 0 {
		t.Errorf("Home still draws %d rows", len(view.Reaching))
	}
}

func TestAnEmptyListMeansNobodyToReach(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 14)
	saved := saveReaching(t, app)
	if saved.State.Contacts == nil {
		t.Error("an empty list would reach the screen as null")
	}
	view, err := app.GetOutlook()
	if err != nil {
		t.Fatalf("outlook: %v", err)
	}
	if view.Reaching == nil || len(view.Reaching) != 0 {
		t.Errorf("rows %+v with nobody on the list", view.Reaching)
	}
}

// An empty row says someone is not available in the days shown, where a
// missing row would leave the reader wondering whether the setting took.
func TestSomeoneWithNoWindowInTheHorizonKeepsTheirRow(t *testing.T) {
	rows := reachingRows(nil, []ReachingContactDTO{{ID: "contact-1", Label: "The clinic", Hours: weekdayHours("09:00", "13:00", 2)}},
		loadLocationOrUTC(defaultZoneID), time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC))
	if len(rows) != 1 || rows[0].Windows == nil || len(rows[0].Windows) != 0 {
		t.Fatalf("rows %+v", rows)
	}
}

func TestAListTheAppCannotHonourIsRefused(t *testing.T) {
	app := newTestApp(t)
	clinic := func(change func(*ReachingContactDTO)) ReachingContactDTO {
		contact := ReachingContactDTO{Label: "The clinic", ZoneID: defaultZoneID, Hours: weekdayHours("09:00", "17:00", 2)}
		change(&contact)
		return contact
	}
	unchanged := func(*ReachingContactDTO) {}
	six := make([]ReachingContactDTO, 0, 6)
	for _, name := range []string{"A", "B", "C", "D", "E", "F"} {
		six = append(six, clinic(func(contact *ReachingContactDTO) { contact.Label = name }))
	}
	for _, testCase := range []struct {
		name     string
		contacts []ReachingContactDTO
		want     string
	}{
		{"more people than Home has rows for", six, "room for 5"},
		{"someone with no name", []ReachingContactDTO{clinic(func(c *ReachingContactDTO) { c.Label = "  " })}, "name"},
		{
			"two people with one name",
			[]ReachingContactDTO{clinic(unchanged), clinic(func(c *ReachingContactDTO) { c.Label = "the CLINIC" })},
			"different names",
		},
		{"a zone this computer does not know", []ReachingContactDTO{clinic(func(c *ReachingContactDTO) { c.ZoneID = "Mars/Olympus_Mons" })}, "time zone"},
		{"a clock that is not a clock", []ReachingContactDTO{clinic(func(c *ReachingContactDTO) { c.Hours[0].StartLocal = "9am" })}, "like 09:00"},
		{"a weekday that does not exist", []ReachingContactDTO{clinic(func(c *ReachingContactDTO) { c.Hours[0].Day = 9 })}, "Sunday through Saturday"},
		{
			"a day listed twice",
			[]ReachingContactDTO{clinic(func(c *ReachingContactDTO) { c.Hours = append(c.Hours, c.Hours[0]) })},
			"Tuesday is listed twice",
		},
		{"someone shown with no day", []ReachingContactDTO{clinic(func(c *ReachingContactDTO) { c.Hours = nil })}, "at least one day"},
		{"an id that is not one", []ReachingContactDTO{clinic(func(c *ReachingContactDTO) { c.ID = "The Clinic!" })}, "id"},
		{
			"two people with one id",
			[]ReachingContactDTO{
				clinic(func(c *ReachingContactDTO) { c.ID = "contact-1" }),
				clinic(func(c *ReachingContactDTO) { c.ID, c.Label = "contact-1", "The pharmacy" }),
			},
			"id of their own",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := app.SaveReachingHours(ReachingHoursSaveInput{State: ReachingHoursDTO{Contacts: testCase.contacts}})
			if err == nil {
				t.Fatal("it was accepted")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("error %q does not say what is wrong", err)
			}
		})
	}
	// Someone switched off may keep no days: that is how a person is kept on
	// the list without hours.
	saveReaching(t, app, clinic(func(c *ReachingContactDTO) { c.Off, c.Hours = true, nil }))
}

// A save tidies what it is given rather than refusing it: names are trimmed,
// a missing zone is the owner's, days are put in order, and only a new person
// is given an id.
func TestASaveTidiesTheList(t *testing.T) {
	app := newTestApp(t)
	saved := saveReaching(t, app,
		ReachingContactDTO{ID: "contact-kept", Label: "  The clinic ", Hours: append(weekdayHours("09:00", "17:00", 4), weekdayHours("09:00", "13:00", 2)...)},
		ReachingContactDTO{Label: "My sister", ZoneID: "Europe/Berlin", Hours: weekdayHours("18:00", "22:00", 6)},
	)
	clinic, sister := saved.State.Contacts[0], saved.State.Contacts[1]
	if clinic.ID != "contact-kept" || clinic.Label != "The clinic" || clinic.ZoneID != localZoneID() {
		t.Errorf("clinic stored as %+v", clinic)
	}
	if clinic.Hours[0].Day != 2 || clinic.Hours[1].Day != 4 {
		t.Errorf("days stored out of order: %+v", clinic.Hours)
	}
	if !reachingContactIDPattern.MatchString(sister.ID) || sister.ZoneID != "Europe/Berlin" {
		t.Errorf("sister stored as %+v", sister)
	}
}

// The revision guard is what stops a second window, or the agent, quietly
// overwriting a list someone just edited.
func TestAStaleEditLosesToWhatIsStored(t *testing.T) {
	app := newTestApp(t)
	first := saveReaching(t, app, ReachingContactDTO{Label: "The clinic", ZoneID: defaultZoneID, Hours: weekdayHours("08:00", "12:00", 2)})
	stale, err := app.SaveReachingHours(ReachingHoursSaveInput{
		BaseRevision: first.Revision - 1,
		State: ReachingHoursDTO{Contacts: []ReachingContactDTO{
			{Label: "The night desk", ZoneID: defaultZoneID, Hours: weekdayHours("23:00", "23:30", 6)},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Conflict {
		t.Fatal("the stale edit did not report a conflict")
	}
	if len(stale.State.Contacts) != 1 || stale.State.Contacts[0].Label != "The clinic" {
		t.Errorf("the stale edit overwrote the stored list: %+v", stale.State)
	}
}

// The summary is what Home prints. It has to say these are hours the person
// set rather than assert them as a fact about the world, read the week in
// order, and describe an overnight window as overnight.
func TestTheSummaryReadsEachWeekHonestly(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		hours    []ReachingDayHoursDTO
		contains []string
		absent   []string
	}{
		{
			name:     "an ordinary working week",
			hours:    weekdayHours("09:00", "17:00", 1, 2, 3, 4, 5),
			contains: []string{"The clinic: you set Monday to Friday, 9:00 AM to 5:00 PM."},
			absent:   []string{"next day"},
		},
		{
			name:     "a longer Saturday morning",
			hours:    append(weekdayHours("09:00", "17:00", 1, 2, 3, 4, 5), weekdayHours("10:00", "13:00", 6)...),
			contains: []string{"Monday to Friday, 9:00 AM to 5:00 PM; Saturday, 10:00 AM to 1:00 PM"},
		},
		{
			name:     "the weekend reads Saturday first",
			hours:    weekdayHours("18:00", "22:00", 0, 6),
			contains: []string{"Saturday and Sunday"},
		},
		{
			name:     "the same hours on days that are not a run",
			hours:    weekdayHours("09:00", "13:00", 2, 4),
			contains: []string{"Tuesday and Thursday, 9:00 AM to 1:00 PM"},
			absent:   []string{"Tuesday to Thursday"},
		},
		{
			name:     "hours grouped in the order of the week",
			hours:    append(weekdayHours("09:00", "17:00", 1, 5), weekdayHours("12:00", "20:00", 3)...),
			contains: []string{"Monday and Friday, 9:00 AM to 5:00 PM; Wednesday, 12:00 PM to 8:00 PM"},
		},
		{
			name:     "a night that crosses midnight",
			hours:    weekdayHours("22:00", "06:00", 5),
			contains: []string{"Friday, 10:00 PM to 6:00 AM the next day"},
		},
		{
			name:     "a line that never closes",
			hours:    everyDay("00:00", "00:00"),
			contains: []string{"every day, all day"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			summary := reachingSummary(ReachingContactDTO{Label: "The clinic", ZoneID: localZoneID(), Hours: testCase.hours})
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

// Hours in another zone are much of the reason this is configurable at all, so
// the summary must name the zone rather than let the reader assume their own.
func TestForeignHoursNameTheirZone(t *testing.T) {
	local := localZoneID()
	foreign := "Europe/Berlin"
	if local == foreign {
		foreign = "Asia/Tokyo"
	}
	summary := reachingSummary(ReachingContactDTO{Label: "My sister", ZoneID: foreign, Hours: weekdayHours("18:00", "22:00", 0, 6)})
	if !strings.Contains(summary, "("+foreign+" time)") {
		t.Errorf("summary %q does not say whose clock these hours are on", summary)
	}
	if strings.Contains(reachingSummary(ReachingContactDTO{Label: "Local", ZoneID: local, Hours: weekdayHours("09:00", "17:00", 1)}), local) {
		t.Error("the reader's own zone is repeated back at them")
	}
}

func TestASavedListSurvivesARestart(t *testing.T) {
	app := newTestApp(t)
	saveReaching(t, app,
		ReachingContactDTO{Label: "The clinic", ZoneID: defaultZoneID, Hours: weekdayHours("07:30", "11:30", 2, 4)},
		ReachingContactDTO{Label: "My sister", ZoneID: "Europe/Berlin", Off: true, Hours: weekdayHours("18:00", "22:00", 6)},
	)

	restarted := newTestApp(t)
	restarted.configDir = app.configDir
	if err := restarted.loadReachingFromDisk(); err != nil {
		t.Fatalf("load: %v", err)
	}
	envelope, err := restarted.GetReachingHours()
	if err != nil {
		t.Fatal(err)
	}
	contacts := envelope.State.Contacts
	if len(contacts) != 2 || contacts[0].Label != "The clinic" || contacts[0].Hours[0].StartLocal != "07:30" ||
		!contacts[1].Off || contacts[1].ZoneID != "Europe/Berlin" {
		t.Fatalf("restored %+v", contacts)
	}
	if envelope.Revision == 0 {
		t.Error("the restored list has no revision, so the next edit cannot detect a conflict")
	}
}

// The single schedule kept before ADR-0056 becomes one person with the same
// hours on each of its days, and the old file goes once the list is written.
func TestTheOldScheduleCarriesOverAsOnePerson(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		legacy legacyReachingHours
		label  string
	}{
		{
			name: "a schedule that was on",
			legacy: legacyReachingHours{
				Enabled: true, Label: "The clinic", StartLocal: "22:00", EndLocal: "06:00",
				Days: []int{4, 2}, ZoneID: "Europe/Berlin",
			},
			label: "The clinic",
		},
		{
			name: "a schedule that was off, with no name",
			legacy: legacyReachingHours{
				StartLocal: "09:00", EndLocal: "17:00", Days: []int{1, 2, 3, 4, 5}, ZoneID: defaultZoneID,
			},
			label: "Reaching hours",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			app := newTestApp(t)
			legacyPath := filepath.Join(app.configDir, legacyReachingFileName)
			if err := legacyReachingStore(legacyPath).write(testCase.legacy, 3); err != nil {
				t.Fatal(err)
			}
			if err := app.loadReachingFromDisk(); err != nil {
				t.Fatalf("load: %v", err)
			}
			envelope, err := app.GetReachingHours()
			if err != nil {
				t.Fatal(err)
			}
			if len(envelope.State.Contacts) != 1 || envelope.Revision != 3 {
				t.Fatalf("carried over %+v at revision %d", envelope.State, envelope.Revision)
			}
			contact := envelope.State.Contacts[0]
			if contact.Label != testCase.label || contact.Off == testCase.legacy.Enabled ||
				contact.ZoneID != testCase.legacy.ZoneID || len(contact.Hours) != len(testCase.legacy.Days) {
				t.Fatalf("carried over as %+v", contact)
			}
			for index, day := range contact.Hours {
				if day.StartLocal != testCase.legacy.StartLocal || day.EndLocal != testCase.legacy.EndLocal ||
					(index > 0 && day.Day <= contact.Hours[index-1].Day) {
					t.Errorf("day %+v", day)
				}
			}
			if _, err := os.Stat(filepath.Join(app.configDir, reachingFileName)); err != nil {
				t.Errorf("the list was not written: %v", err)
			}
			if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the old file is still there: %v", err)
			}
		})
	}
}

// If the old file outlived the move, the list wins, and the old schedule is
// not carried over a second time.
func TestTheListWinsOverAnOldFileThatOutlivedTheMove(t *testing.T) {
	app := newTestApp(t)
	saveReaching(t, app, ReachingContactDTO{Label: "The pharmacy", ZoneID: defaultZoneID, Hours: everyDay("08:00", "20:00")})
	legacy := legacyReachingHours{Enabled: true, Label: "The clinic", StartLocal: "09:00", EndLocal: "17:00", Days: []int{1}, ZoneID: defaultZoneID}
	if err := legacyReachingStore(filepath.Join(app.configDir, legacyReachingFileName)).write(legacy, 9); err != nil {
		t.Fatal(err)
	}
	restarted := newTestApp(t)
	restarted.configDir = app.configDir
	if err := restarted.loadReachingFromDisk(); err != nil {
		t.Fatal(err)
	}
	if contacts := restarted.currentReachingHours().Contacts; len(contacts) != 1 || contacts[0].Label != "The pharmacy" {
		t.Fatalf("restored %+v", contacts)
	}
}
