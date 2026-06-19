package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"non24.app/core/scheduling"
	storage "non24.app/core/storage/sqlite"
)

func TestEmptyLocalStoreReturnsHonestStates(t *testing.T) {
	app := newTestApp(t)

	overview, err := app.GetOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Status != "empty" || !overview.Empty || overview.FixtureMode {
		t.Fatalf("unexpected empty overview: %#v", overview)
	}
	if !strings.Contains(overview.CurrentEstimatedState, "No sleep entries") {
		t.Fatalf("empty overview should invite first entry: %#v", overview)
	}

	rhythm, err := app.GetRhythm()
	if err != nil {
		t.Fatal(err)
	}
	if rhythm.Status != "empty" || rhythm.FixtureMode || len(rhythm.ObservedRows) != 0 || len(rhythm.DriftPoints) != 0 {
		t.Fatalf("unexpected empty rhythm projection: %#v", rhythm)
	}

	proposals, err := app.GetProposals()
	if err != nil {
		t.Fatal(err)
	}
	if proposals.Status != "empty" || proposals.FixtureMode || len(proposals.Proposals) != 0 {
		t.Fatalf("unexpected empty proposals: %#v", proposals)
	}
	if len(proposals.Unplaced) == 0 || proposals.Unplaced[0].ReasonCode != string(scheduling.ReasonEstimateUnavailable) {
		t.Fatalf("empty proposals should be marked estimate unavailable: %#v", proposals.Unplaced)
	}
}

func TestBelowMinimumLocalDataReturnsTypedRefusal(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 2)

	overview, err := app.GetOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Status != "refused" || overview.Refusal == nil || overview.Refusal.Code != "insufficient_data" {
		t.Fatalf("expected insufficient data refusal, got %#v", overview)
	}
	if overview.FixtureMode {
		t.Fatal("refusal must not fall back to fixture mode")
	}

	proposals, err := app.GetProposals()
	if err != nil {
		t.Fatal(err)
	}
	if proposals.Status != "refused" || len(proposals.Proposals) != 0 {
		t.Fatalf("proposals should refuse without an estimate: %#v", proposals)
	}
}

func TestStoredEntriesDriveOverviewRhythmAndProposals(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 12)

	overview, err := app.GetOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Status != "estimated" || overview.FixtureMode {
		t.Fatalf("expected local estimated overview: %#v", overview)
	}
	if overview.PredictedNextSleepWindow == "" || overview.PredictedNextSleepWindow == "Not enough local data" {
		t.Fatalf("overview did not expose predicted window: %#v", overview)
	}
	if !strings.Contains(overview.DriftEstimate, "observed sleep cycle") {
		t.Fatalf("unsafe drift wording: %q", overview.DriftEstimate)
	}
	if len(overview.MedicationEvents) != 0 {
		t.Fatalf("desktop local slice should not fabricate medication records: %#v", overview.MedicationEvents)
	}

	rhythm, err := app.GetRhythm()
	if err != nil {
		t.Fatal(err)
	}
	if rhythm.Status != "estimated" || rhythm.FixtureMode || len(rhythm.ObservedRows) == 0 || len(rhythm.DriftPoints) == 0 {
		t.Fatalf("expected local rhythm projection: %#v", rhythm)
	}

	proposals, err := app.GetProposals()
	if err != nil {
		t.Fatal(err)
	}
	if proposals.Status != "estimated" || proposals.FixtureMode || len(proposals.Proposals) == 0 {
		t.Fatalf("expected local scheduler proposals: %#v", proposals)
	}
	validCodes := map[string]bool{
		scheduling.CodeWithinPredictedWakingWindow: true,
		scheduling.CodeAvoidsFixedEvent:            true,
		scheduling.CodeWithinTaskBounds:            true,
		scheduling.CodeUncertaintyBufferApplied:    true,
	}
	for _, proposal := range proposals.Proposals {
		if proposal.Origin != "scheduler" || proposal.To == "" || proposal.Confidence == "" {
			t.Fatalf("incomplete proposal: %#v", proposal)
		}
		if len(proposal.ExplanationCodes) == 0 {
			t.Fatalf("proposal %q has no explanation codes", proposal.ID)
		}
		for _, code := range proposal.ExplanationCodes {
			if !validCodes[code] {
				t.Fatalf("proposal %q has off-contract code %q", proposal.ID, code)
			}
			if code == scheduling.CodeUncertaintyBufferApplied {
				t.Fatalf("proposal %q claims an uncertainty buffer the engine does not apply", proposal.ID)
			}
		}
	}
}

func TestSleepEntryEditAndSuppressAreAppendOnly(t *testing.T) {
	app := newTestApp(t)
	location, _ := time.LoadLocation(defaultZoneID)
	start := time.Now().In(location).Add(-10 * time.Hour).Truncate(time.Minute)
	added, err := app.AddSleepEntry(SleepEntryInput{
		StartLocal:     start.Format("2006-01-02T15:04"),
		EndLocal:       start.Add(8 * time.Hour).Format("2006-01-02T15:04"),
		ZoneID:         defaultZoneID,
		Classification: storage.SleepClassificationPrincipal,
	})
	if err != nil {
		t.Fatal(err)
	}

	editedStart := start.Add(30 * time.Minute)
	editedEnd := start.Add(8*time.Hour + 15*time.Minute)
	edited, err := app.CorrectSleepEntry(SleepCorrectionInput{
		ObservationID:  added.ObservationID,
		StartLocal:     editedStart.Format("2006-01-02T15:04"),
		EndLocal:       editedEnd.Format("2006-01-02T15:04"),
		ZoneID:         defaultZoneID,
		Classification: storage.SleepClassificationNap,
	})
	if err != nil {
		t.Fatal(err)
	}
	if edited.StartLocal == edited.EffectiveStartLocal {
		t.Fatalf("raw start should remain immutable after correction: %#v", edited)
	}
	if edited.EffectiveClassification != storage.SleepClassificationNap || len(edited.History) != 1 {
		t.Fatalf("edit was not reflected with history: %#v", edited)
	}

	suppressed, err := app.SuppressSleepEntry(SleepSuppressInput{ObservationID: added.ObservationID})
	if err != nil {
		t.Fatal(err)
	}
	if !suppressed.Suppressed || len(suppressed.History) != 2 {
		t.Fatalf("suppression should append history and mark effective entry suppressed: %#v", suppressed)
	}
}

func TestSleepExportAndDeleteRequireExplicitErasure(t *testing.T) {
	app := newTestApp(t)
	location, _ := time.LoadLocation(defaultZoneID)
	start := time.Now().In(location).Add(-10 * time.Hour).Truncate(time.Minute)
	added, err := app.AddSleepEntry(SleepEntryInput{
		StartLocal:     start.Format("2006-01-02T15:04"),
		EndLocal:       start.Add(8 * time.Hour).Format("2006-01-02T15:04"),
		ZoneID:         defaultZoneID,
		Classification: storage.SleepClassificationPrincipal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SuppressSleepEntry(SleepSuppressInput{ObservationID: added.ObservationID}); err != nil {
		t.Fatal(err)
	}

	exported, err := app.ExportSleepData()
	if err != nil {
		t.Fatal(err)
	}
	if exported.ObservationCount != 1 || exported.CorrectionCount != 1 {
		t.Fatalf("suppressed entry should still export observation and correction: %#v", exported)
	}
	if !strings.Contains(exported.JSON, `"observation_set"`) || !strings.Contains(exported.JSON, added.ObservationID) {
		t.Fatalf("export is not the contract-shaped sleep data: %s", exported.JSON)
	}

	if _, err := app.DeleteSleepObservation(SleepDeleteInput{ObservationID: added.ObservationID, Confirmation: "suppress"}); err == nil {
		t.Fatal("delete should require the exact erasure confirmation")
	}
	entries, err := app.DeleteSleepObservation(SleepDeleteInput{ObservationID: added.ObservationID, Confirmation: deleteConfirm})
	if err != nil {
		t.Fatal(err)
	}
	if !entries.Empty || len(entries.Entries) != 0 {
		t.Fatalf("delete should hard-erase the local sleep entry: %#v", entries)
	}
	exported, err = app.ExportSleepData()
	if err != nil {
		t.Fatal(err)
	}
	if exported.ObservationCount != 0 || exported.CorrectionCount != 0 || strings.Contains(exported.JSON, added.ObservationID) {
		t.Fatalf("export still contains erased sleep data: %#v", exported)
	}
}

func TestAddSleepEntryValidatesCivilRange(t *testing.T) {
	app := newTestApp(t)
	_, err := app.AddSleepEntry(SleepEntryInput{
		StartLocal:     "2026-03-02T08:00",
		EndLocal:       "2026-03-02T07:00",
		ZoneID:         defaultZoneID,
		Classification: storage.SleepClassificationPrincipal,
	})
	if err == nil || !strings.Contains(err.Error(), "after sleep start") {
		t.Fatalf("expected end-after-start validation, got %v", err)
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "desktop.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return newAppWithStore(store, nil)
}

func seedSleepEntries(t *testing.T, app *App, count int) {
	t.Helper()
	location, _ := time.LoadLocation(defaultZoneID)
	lastStart := time.Now().In(location).Add(-12 * time.Hour).Truncate(time.Minute)
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
