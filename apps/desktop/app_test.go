package main

import (
	"strings"
	"testing"

	"non24.app/core/scheduling"
)

func TestProposalsFlowFromTheRealScheduler(t *testing.T) {
	result, err := NewApp().GetProposals()
	if err != nil {
		t.Fatal(err)
	}
	if !result.FixtureMode {
		t.Fatal("fixture mode should be explicit")
	}
	if len(result.Proposals) == 0 {
		t.Fatal("expected at least one engine proposal")
	}

	validCodes := map[string]bool{
		scheduling.CodeWithinPredictedWakingWindow: true,
		scheduling.CodeAvoidsFixedEvent:            true,
		scheduling.CodeWithinTaskBounds:            true,
		scheduling.CodeUncertaintyBufferApplied:    true,
	}
	sawWakingWindow, sawAvoidsEvent := false, false
	for _, proposal := range result.Proposals {
		if len(proposal.ExplanationCodes) == 0 {
			t.Fatalf("proposal %q has no explanation codes", proposal.ID)
		}
		if len(proposal.ReasonLabels) != len(proposal.ExplanationCodes) {
			t.Fatalf("proposal %q reason labels do not match codes", proposal.ID)
		}
		for _, code := range proposal.ExplanationCodes {
			if !validCodes[code] {
				t.Fatalf("proposal %q has off-contract code %q", proposal.ID, code)
			}
			if code == scheduling.CodeUncertaintyBufferApplied {
				t.Fatalf("proposal %q claims an uncertainty buffer the engine does not apply", proposal.ID)
			}
			if code == scheduling.CodeWithinPredictedWakingWindow {
				sawWakingWindow = true
			}
			if code == scheduling.CodeAvoidsFixedEvent {
				sawAvoidsEvent = true
			}
		}
		if proposal.Origin != "scheduler" || proposal.To == "" || proposal.Confidence == "" {
			t.Fatalf("incomplete proposal: %#v", proposal)
		}
	}
	if !sawWakingWindow {
		t.Fatal("expected a placement justified by a predicted waking window")
	}
	if !sawAvoidsEvent {
		t.Fatal("expected a placement that avoids the fixed event")
	}

	// The deadline-too-soon task must surface as an honest unplaced entry.
	if len(result.Unplaced) == 0 {
		t.Fatal("expected the over-constrained task to be unplaced")
	}
	for _, unplaced := range result.Unplaced {
		if unplaced.ReasonCode != string(scheduling.ReasonNoAvailableInterval) {
			t.Fatalf("unexpected unplaced reason code %q", unplaced.ReasonCode)
		}
		if unplaced.Reason == "" || unplaced.NextAction == "" {
			t.Fatalf("incomplete unplaced entry: %#v", unplaced)
		}
	}
}

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
