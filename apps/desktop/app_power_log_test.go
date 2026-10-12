package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"non24.app/core/platform/activity"
	storage "non24.app/core/storage/sqlite"
)

// syntheticPowerLog is a System log holding synthetic entries. It remembers
// what it was asked for.
type syntheticPowerLog struct {
	events []activity.PowerEvent
	asked  []time.Time
	err    error
}

func (l *syntheticPowerLog) After(since time.Time, limit int) ([]activity.PowerEvent, error) {
	l.asked = append(l.asked, since)
	if l.err != nil {
		return nil, l.err
	}
	var events []activity.PowerEvent
	for _, event := range l.events {
		if event.At.After(since) && len(events) < limit {
			events = append(events, event)
		}
	}
	return events, nil
}

// sleepsFrom logs a laptop closed at 23:00 and opened at 07:00, from day.
func sleepsFrom(day time.Time, nights int) []activity.PowerEvent {
	var events []activity.PowerEvent
	for night := range nights {
		at := day.AddDate(0, 0, night)
		events = append(events,
			activity.PowerEvent{At: at.Add(23 * time.Hour), State: activity.StateSuspended, RecordID: uint64(100 + 2*night)},
			activity.PowerEvent{At: at.Add(31 * time.Hour), State: activity.StateResumed, RecordID: uint64(101 + 2*night)})
	}
	return events
}

func turnOnCollection(t *testing.T, app *App) {
	t.Helper()
	if err := app.store.SetCollectionPreference(context.Background(), activity.SourceID,
		storage.CollectionPreference{Enabled: true, ZoneID: "UTC"}); err != nil {
		t.Fatal(err)
	}
}

func TestThePowerLogIsReadOnlyWhenChosen(t *testing.T) {
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) }
	log := &syntheticPowerLog{events: sleepsFrom(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 5)}
	app.powerLogSource = log

	if _, err := app.SetPowerLogReading(PowerLogInput{Enabled: true}); err == nil ||
		!strings.Contains(err.Error(), "Turn on desktop activity") {
		t.Fatalf("read without desktop activity: %v", err)
	}
	turnOnCollection(t, app)
	app.activityMu.Lock()
	app.readPowerLogLocked(context.Background())
	app.activityMu.Unlock()
	if len(log.asked) != 0 {
		t.Fatal("the log was read before it was chosen")
	}

	status, err := app.SetPowerLogReading(PowerLogInput{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !status.PowerLog.Enabled || !status.PowerLog.Supported || status.PowerLog.Entries != 10 ||
		status.PowerLog.SinceLabel == "" || status.RecordCount != 10 {
		t.Fatalf("status after the first read %+v", status)
	}
	// The first read goes as far back as the log does.
	if len(log.asked) != 1 || !log.asked[0].IsZero() {
		t.Fatalf("first read asked from %v", log.asked)
	}

	// The next pass asks again from an hour before the newest entry kept, and
	// keeps only what is new.
	log.events = append(log.events, sleepsFrom(time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), 1)...)
	app.activityMu.Lock()
	app.readPowerLogLocked(context.Background())
	app.activityMu.Unlock()
	if want := time.Date(2026, 10, 6, 6, 0, 0, 0, time.UTC); !log.asked[1].Equal(want) {
		t.Errorf("second read asked from %s, want %s", log.asked[1], want)
	}
	status, err = app.GetActivityCollection()
	if err != nil || status.PowerLog.Entries != 12 {
		t.Fatalf("after the second read %+v, %v", status.PowerLog, err)
	}

	// With desktop activity off, the choice stays but nothing is read.
	if err := app.store.SetCollectionPreference(context.Background(), activity.SourceID,
		storage.CollectionPreference{Enabled: false, ZoneID: "UTC"}); err != nil {
		t.Fatal(err)
	}
	app.activityMu.Lock()
	app.readPowerLogLocked(context.Background())
	app.activityMu.Unlock()
	if len(log.asked) != 2 {
		t.Fatal("the log was read with desktop activity off")
	}
}

func TestAFailedReadSaysSoAndKeepsTheChoice(t *testing.T) {
	app := newTestApp(t)
	app.powerLogSource = &syntheticPowerLog{err: context.DeadlineExceeded}
	turnOnCollection(t, app)
	status, err := app.SetPowerLogReading(PowerLogInput{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !status.PowerLog.Enabled || !strings.Contains(status.PowerLog.LastError, "could not be read") {
		t.Fatalf("status %+v", status.PowerLog)
	}
}

// Erasing desktop activity erases what was read from the log and turns the
// reading off, so starting over asks again.
func TestErasingActivityStopsReadingTheLog(t *testing.T) {
	app := newTestApp(t)
	app.powerLogSource = &syntheticPowerLog{events: sleepsFrom(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 2)}
	turnOnCollection(t, app)
	if _, err := app.SetPowerLogReading(PowerLogInput{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	status, err := app.DeleteActivityData(deleteConfirm)
	if err != nil {
		t.Fatal(err)
	}
	if status.PowerLog.Enabled || status.PowerLog.Entries != 0 || status.RecordCount != 0 {
		t.Fatalf("after erasing %+v", status)
	}
}

// What the computer's power log suggests sits beside the recorded nights, and
// the edges agree as the log says.
func TestThisComputerSuggestsNightsBesideRecordedOnes(t *testing.T) {
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }
	if nights, err := app.GetComputerNights(); err != nil || nights.Collected || nights.Figure != nil {
		t.Fatalf("nothing collected read as %+v, %v", nights, err)
	}
	app.powerLogSource = &syntheticPowerLog{events: sleepsFrom(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), 10)}
	turnOnCollection(t, app)
	if _, err := app.SetPowerLogReading(PowerLogInput{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// Three recorded nights fall inside what the log covers: asleep half an
	// hour after the laptop closed, awake twenty minutes before it opened.
	for _, day := range []int{3, 4, 5} {
		start := time.Date(2026, 9, day, 23, 30, 0, 0, time.UTC)
		if _, err := app.AddSleepEntry(SleepEntryInput{
			StartLocal: start.Format("2006-01-02T15:04"), EndLocal: start.Add(7*time.Hour + 10*time.Minute).Format("2006-01-02T15:04"),
			ZoneID: "UTC", Classification: storage.SleepClassificationPrincipal,
		}); err != nil {
			t.Fatal(err)
		}
	}
	nights, err := app.GetComputerNights()
	if err != nil {
		t.Fatal(err)
	}
	if !nights.Collected || nights.Figure == nil || nights.Figure.Nights != 10 {
		t.Fatalf("nights %+v", nights)
	}
	agreement := nights.Figure.Agreement
	if agreement == nil || agreement.Recorded != 3 || agreement.Matched != 3 {
		t.Fatalf("agreement %+v", agreement)
	}
	if want := "On those nights, suggestions typically began 30 minutes before your recorded sleep and ended 20 minutes after your recorded wake."; agreement.Summary != want {
		t.Errorf("summary %q", agreement.Summary)
	}
}
