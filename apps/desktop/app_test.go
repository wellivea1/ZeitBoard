package main

import (
	"strings"
	"testing"
)

func TestFixtureOverviewFlowsThroughApplicationService(t *testing.T) {
	overview, err := NewApp().GetOverview()
	if err != nil {
		t.Fatal(err)
	}
	if !overview.FixtureMode {
		t.Fatal("fixture mode should be explicit")
	}
	if overview.CurrentEstimatedState == "" || overview.PredictedNextSleepWindow == "" || overview.NextUsefulTaskWindow == "" {
		t.Fatalf("incomplete overview: %#v", overview)
	}
	if !strings.Contains(overview.DriftEstimate, "observed sleep cycle") {
		t.Fatalf("unsafe drift wording: %q", overview.DriftEstimate)
	}
	if len(overview.MedicationEvents) != 1 || !strings.Contains(overview.MedicationEvents[0].RelativeToWake, "confirmed wake") {
		t.Fatalf("medication timing missing: %#v", overview.MedicationEvents)
	}
}
