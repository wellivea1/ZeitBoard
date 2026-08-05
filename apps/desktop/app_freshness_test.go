package main

import (
	"strings"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

// seedSleepEntriesEndingAt lays down a fittable rhythm whose newest episode
// ends at the supplied offset before now, so a test can control evidence age.
func seedSleepEntriesEndingAt(t *testing.T, app *App, count int, newestEndsAgo time.Duration) {
	t.Helper()
	location, _ := time.LoadLocation(defaultZoneID)
	lastEnd := time.Now().In(location).Add(-newestEndsAgo).Truncate(time.Minute)
	lastStart := lastEnd.Add(-8 * time.Hour)
	firstStart := lastStart.Add(-time.Duration(count-1) * 25 * time.Hour)
	for i := 0; i < count; i++ {
		start := firstStart.Add(time.Duration(i) * 25 * time.Hour)
		if _, err := app.AddSleepEntry(SleepEntryInput{
			StartLocal:     start.Format("2006-01-02T15:04"),
			EndLocal:       start.Add(8 * time.Hour).Format("2006-01-02T15:04"),
			ZoneID:         defaultZoneID,
			Classification: storage.SleepClassificationPrincipal,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestOverviewWithholdsAStaleCurrentState is the defect this closes. Before the
// shared policy, "Likely awake" was the default whenever the newest stored
// sleep interval did not contain now — so it survived for as long as the user
// went without recording anything, which is exactly when it is least true.
func TestOverviewWithholdsAStaleCurrentState(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 10, 4*24*time.Hour)

	overview, err := app.GetOverview()
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.Status != "estimated" {
		t.Skipf("estimator refused on this fixture (%s); freshness is not reachable", overview.Status)
	}
	if strings.Contains(overview.CurrentEstimatedState, "Likely") {
		t.Errorf("current state = %q after four days without a record; want it withheld",
			overview.CurrentEstimatedState)
	}
	if overview.Freshness.Trusted {
		t.Error("four-day-old evidence was marked trusted")
	}
	if overview.Freshness.State != "withheld" {
		t.Errorf("freshness state = %q, want withheld", overview.Freshness.State)
	}
	if overview.Freshness.Explanation == "" {
		t.Error("no explanation was given for withholding")
	}
}

// TestOverviewTrustsFreshEvidence keeps the policy from being so strict that
// ordinary use stops answering.
func TestOverviewTrustsFreshEvidence(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 10, 2*time.Hour)

	overview, err := app.GetOverview()
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.Status != "estimated" {
		t.Skipf("estimator refused on this fixture (%s)", overview.Status)
	}
	if !strings.Contains(overview.CurrentEstimatedState, "Likely") {
		t.Errorf("current state = %q with two-hour-old evidence; want a plain claim",
			overview.CurrentEstimatedState)
	}
	if !overview.Freshness.Trusted {
		t.Errorf("fresh evidence was not trusted: %+v", overview.Freshness)
	}
	if overview.Freshness.AgeLabel == "" {
		t.Error("no evidence age was reported")
	}
}

// TestOverviewUpdatedLabelDoesNotClaimFreshData guards the wording that used to
// say "just now" on every screen load, which described the recomputation rather
// than the data.
func TestOverviewUpdatedLabelDoesNotClaimFreshData(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 10, 3*24*time.Hour)

	overview, err := app.GetOverview()
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if strings.Contains(strings.ToLower(overview.UpdatedLabel), "just now") {
		t.Errorf("updated label still claims freshness it cannot know: %q", overview.UpdatedLabel)
	}
}
