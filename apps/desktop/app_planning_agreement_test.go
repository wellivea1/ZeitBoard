package main

import (
	"context"
	"strings"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

// Home, the outlook and the planner once read the same records three ways.
// These hold the planner to what the other two say.

// With the newest sleep 64 hours old, Home says nothing about the present and
// the outlook is withheld, while the planner offered a time "in a predicted
// waking window" at high confidence. It now says why it offers none.
func TestThePlannerWithholdsOnRecordsHomeWithholdsOn(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 12, 64*time.Hour)
	if _, err := app.AddTask(TaskInput{Title: "Paperwork", DurationMinutes: 90}); err != nil {
		t.Fatal(err)
	}

	overview, err := app.GetOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Status != "estimated" {
		t.Skipf("the estimator refused this history (%s); the planner's gate is not reachable", overview.Status)
	}
	if overview.Freshness.State != "withheld" {
		t.Fatalf("freshness = %q, want withheld for this test to mean anything", overview.Freshness.State)
	}

	built, err := app.buildLocalProposals(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(built.pending) != 0 {
		t.Fatalf("the planner offered %d suggestions on records Home withholds on", len(built.pending))
	}
	if len(built.dto.Unplaced) != 1 || !strings.Contains(built.dto.Unplaced[0].Reason, "too old") {
		t.Fatalf("unplaced = %+v, want the task listed with the age of the records as the reason", built.dto.Unplaced)
	}
	if !strings.Contains(built.dto.Unplaced[0].NextAction, "Record") {
		t.Errorf("next action = %q, want the step that brings suggestions back", built.dto.Unplaced[0].NextAction)
	}
}

// The estimator's sleep and waking windows are envelopes and overlap. A task
// offered where both apply is offered in hours the outlook draws as uncertain.
func TestSuggestionsStayOutsideEveryPredictedSleepWindow(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 12, 2*time.Hour)
	// Enough long tasks to fill the waking days and press against their edges.
	for _, title := range []string{"One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight"} {
		if _, err := app.AddTask(TaskInput{Title: title, DurationMinutes: 240}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	built, err := app.buildLocalProposals(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.pending) == 0 {
		t.Fatal("no suggestions on fresh records; the check below would pass on nothing")
	}
	state, _, err := app.localEstimateForPlanningSnapshot(context.Background(), now.Truncate(localProposalTTL))
	if err != nil {
		t.Fatal(err)
	}
	for id, candidate := range built.pending {
		window := candidate.proposal.Window
		for _, sleep := range state.Estimate.PredictedSleepWindows {
			if window.Start.UTC.Before(sleep.Interval.End.UTC) && sleep.Interval.Start.UTC.Before(window.End.UTC) {
				t.Errorf("suggestion %s, %s to %s, falls in the predicted sleep window %s to %s",
					id, window.Start.UTC, window.End.UTC, sleep.Interval.Start.UTC, sleep.Interval.End.UTC)
			}
		}
	}
}

// Excluding a bad recent record must not make an old forecast read as current:
// the estimator already ignores the record, and so must the age of the evidence.
func TestAnExcludedNightDoesNotMakeOldRecordsCurrent(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 12, 72*time.Hour)
	location, _ := time.LoadLocation(defaultZoneID)
	end := time.Now().In(location).Add(-time.Hour).Truncate(time.Minute)
	recent, err := app.AddSleepEntry(SleepEntryInput{
		StartLocal:     end.Add(-8 * time.Hour).Format("2006-01-02T15:04"),
		EndLocal:       end.Format("2006-01-02T15:04"),
		ZoneID:         defaultZoneID,
		Classification: storage.SleepClassificationPrincipal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SuppressSleepEntry(SleepSuppressInput{ObservationID: recent.ObservationID, ReviewToken: recent.ReviewToken}); err != nil {
		t.Fatal(err)
	}

	overview, err := app.GetOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Status != "estimated" {
		t.Skipf("the estimator refused this history (%s)", overview.Status)
	}
	if overview.Freshness.Trusted || overview.Freshness.State != "withheld" {
		t.Fatalf("freshness = %+v after excluding the only recent record; the older records are three days old", overview.Freshness)
	}
}
