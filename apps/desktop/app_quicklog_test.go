package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"non24.app/core/quicklog"
	storage "non24.app/core/storage/sqlite"
)

// TestTwoTapsReplaceTheForm is the feature in one test: mark sleep, mark wake,
// and a night is in the log without a four-field form.
func TestTwoTapsReplaceTheForm(t *testing.T) {
	app := newTestApp(t)

	begun, err := app.BeginQuickSleep()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if !begun.State.Pending {
		t.Fatal("no unfinished sleep after marking one")
	}
	if begun.Recorded {
		t.Error("marking sleep recorded an observation; nothing has been observed to end")
	}

	// Nothing is in the log yet — the first tap is an intent, not evidence.
	entries, err := app.ListSleepEntries()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries.Entries) != 0 {
		t.Fatalf("%d entries exist after only marking sleep", len(entries.Entries))
	}

	// Backdate the pending onset so the pair is a plausible night.
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.SetPendingSleep(ctx, storage.PendingSleepRecord{
		StartedAt: time.Now().UTC().Add(-8 * time.Hour),
		ZoneID:    defaultZoneID,
	}); err != nil {
		t.Fatal(err)
	}

	woke, err := app.CompleteQuickSleep()
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if woke.Outcome != string(quicklog.OutcomeRecord) || !woke.Recorded {
		t.Fatalf("outcome = %q recorded = %v (%s)", woke.Outcome, woke.Recorded, woke.Reason)
	}
	if woke.State.Pending {
		t.Error("the unfinished sleep survived being recorded")
	}

	entries, err = app.ListSleepEntries()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries.Entries) != 1 {
		t.Fatalf("%d entries after a completed pair, want 1", len(entries.Entries))
	}
	if entries.Entries[0].ObservationID != woke.Entry {
		t.Error("the recorded entry is not the one the tap reported")
	}
}

// TestWakingWithNothingMarkedAsksRatherThanGuesses. One tap gives one boundary,
// and the estimator needs two.
func TestWakingWithNothingMarkedAsksRatherThanGuesses(t *testing.T) {
	app := newTestApp(t)

	woke, err := app.CompleteQuickSleep()
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if woke.Outcome != string(quicklog.OutcomeConfirmOnset) {
		t.Fatalf("outcome = %q, want confirm_onset", woke.Outcome)
	}
	if woke.Recorded {
		t.Fatal("a night was recorded with no known onset")
	}
	if woke.SuggestedEndLocal == "" {
		t.Error("the wake time the person just reported was not carried into the question")
	}

	entries, err := app.ListSleepEntries()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries.Entries) != 0 {
		t.Error("something was appended anyway")
	}
}

// TestAnImplausiblePairIsNotRecorded. Each of these has a likely explanation
// and none of them is "record it and move on".
func TestAnImplausiblePairIsNotRecorded(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		offset time.Duration
		want   quicklog.Outcome
	}{
		{"a nap or a mistap", -30 * time.Minute, quicklog.OutcomeConfirmShort},
		{"a missed wake tap", -18 * time.Hour, quicklog.OutcomeConfirmLong},
		{"left running for a day", -26 * time.Hour, quicklog.OutcomeConfirmStale},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			app := newTestApp(t)
			store, err := app.requireStore()
			if err != nil {
				t.Fatal(err)
			}
			if err := store.SetPendingSleep(context.Background(), storage.PendingSleepRecord{
				StartedAt: time.Now().UTC().Add(testCase.offset),
				ZoneID:    defaultZoneID,
			}); err != nil {
				t.Fatal(err)
			}

			woke, err := app.CompleteQuickSleep()
			if err != nil {
				t.Fatalf("complete: %v", err)
			}
			if woke.Outcome != string(testCase.want) {
				t.Fatalf("outcome = %q, want %q", woke.Outcome, testCase.want)
			}
			if woke.Recorded {
				t.Fatal("it was recorded anyway")
			}
			// The unfinished sleep survives the question, so answering it does
			// not require remembering when you went to bed.
			if !woke.State.Pending {
				t.Error("the unfinished sleep was dropped while asking about it")
			}
			if woke.SuggestedStartLocal == "" {
				t.Error("the question starts from a blank field")
			}
			if woke.SuggestionIsPrediction {
				t.Error("a time the person marked was labelled as a prediction")
			}
		})
	}
}

// TestConfirmingRecordsWhatThePersonChose closes the loop after a question.
func TestConfirmingRecordsWhatThePersonChose(t *testing.T) {
	app := newTestApp(t)
	location, _ := time.LoadLocation(defaultZoneID)
	end := time.Now().In(location).Add(-time.Hour).Truncate(time.Minute)
	start := end.Add(-7 * time.Hour)

	result, err := app.ConfirmQuickSleep(ConfirmQuickSleepInput{
		StartLocal: start.Format(quickLogLocalLayout),
		EndLocal:   end.Format(quickLogLocalLayout),
		ZoneID:     defaultZoneID,
	})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !result.Recorded {
		t.Fatalf("not recorded: %s", result.Reason)
	}

	entries, err := app.ListSleepEntries()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries.Entries) != 1 {
		t.Fatalf("%d entries, want 1", len(entries.Entries))
	}
}

func TestConfirmingRefusesTimesThatAreNotASleep(t *testing.T) {
	app := newTestApp(t)
	location, _ := time.LoadLocation(defaultZoneID)
	at := time.Now().In(location).Add(-2 * time.Hour).Truncate(time.Minute)

	result, err := app.ConfirmQuickSleep(ConfirmQuickSleepInput{
		StartLocal: at.Format(quickLogLocalLayout),
		EndLocal:   at.Add(-time.Hour).Format(quickLogLocalLayout),
		ZoneID:     defaultZoneID,
	})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if result.Recorded {
		t.Fatal("a backwards sleep was recorded")
	}
	if !strings.Contains(result.Reason, "waking comes after") {
		t.Errorf("reason %q does not say what is wrong", result.Reason)
	}
}

// TestASecondSleepTapReplacesTheFirst. The newer tap is the current intent.
func TestASecondSleepTapReplacesTheFirst(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.BeginQuickSleep(); err != nil {
		t.Fatal(err)
	}
	second, err := app.BeginQuickSleep()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(second.Reason, "replacing") {
		t.Errorf("reason %q does not report the replacement", second.Reason)
	}
	if !second.State.Pending {
		t.Error("no unfinished sleep after the second tap")
	}
}

// TestDiscardingLeavesNothingBehind. A marked onset the person no longer
// believes is worse than none: left alone it attaches to the next wake tap.
func TestDiscardingLeavesNothingBehind(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.BeginQuickSleep(); err != nil {
		t.Fatal(err)
	}
	discarded, err := app.DiscardQuickSleep()
	if err != nil {
		t.Fatal(err)
	}
	if discarded.State.Pending {
		t.Error("the unfinished sleep survived being discarded")
	}
	if discarded.Recorded {
		t.Error("discarding recorded something")
	}
}

// TestErasingEverythingTakesTheUnfinishedSleepToo. Otherwise "delete all my
// sleep data" leaves a marked onset that writes a fresh row on the next tap.
func TestErasingEverythingTakesTheUnfinishedSleepToo(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.BeginQuickSleep(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DeleteAllSleepData(SleepDeleteAllInput{Confirmation: "DELETE"}); err != nil {
		t.Fatalf("delete all: %v", err)
	}
	state, err := app.GetQuickLogState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending {
		t.Error("an unfinished sleep survived erasure of all sleep data")
	}
}

// TestAStaleUnfinishedSleepIsFlagged so the button stops offering one-tap
// closure for something "now" says nothing about.
func TestAStaleUnfinishedSleepIsFlagged(t *testing.T) {
	app := newTestApp(t)
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetPendingSleep(context.Background(), storage.PendingSleepRecord{
		StartedAt: time.Now().UTC().Add(-30 * time.Hour),
		ZoneID:    defaultZoneID,
	}); err != nil {
		t.Fatal(err)
	}
	state, err := app.GetQuickLogState()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Pending || !state.PendingStale {
		t.Errorf("state = %+v, want a pending sleep flagged stale", state)
	}
}
