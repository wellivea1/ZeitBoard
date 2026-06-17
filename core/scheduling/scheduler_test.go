package scheduling

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestBusinessHoursSchedulingSubtractsFixedEventWithoutMovingIt(t *testing.T) {
	zone := "America/New_York"
	location, _ := time.LoadLocation(zone)
	day := time.Date(2026, 6, 15, 0, 0, 0, 0, location)
	availability := window(day.Add(8*time.Hour), day.Add(18*time.Hour), zone, domain.ConfidenceHigh)
	event := domain.CalendarEvent{
		ID: "fixed-1", Title: "Appointment", Fixed: true,
		Interval: rangeAt(day.Add(9*time.Hour), day.Add(10*time.Hour+30*time.Minute), zone),
	}
	original := event
	task := domain.FlexibleTask{
		ID: "task-1", Title: "Paperwork", EstimatedDuration: 90 * time.Minute,
		Constraint: domain.TaskConstraint{
			BusinessHours: true, BusinessStartLocal: "09:00", BusinessEndLocal: "17:00",
			MinimumConfidence: domain.ConfidenceMedium, RequiresApproval: true,
		},
	}
	proposal, err := (Scheduler{}).Propose(Request{Task: task, Availability: []domain.AvailabilityWindow{availability}, Events: []domain.CalendarEvent{event}, Now: day})
	if err != nil {
		t.Fatal(err)
	}
	wantStart := day.Add(10*time.Hour + 30*time.Minute).UTC()
	if !proposal.Window.Start.UTC.Equal(wantStart) {
		t.Fatalf("start = %v, want %v", proposal.Window.Start.UTC, wantStart)
	}
	if !reflect.DeepEqual(event, original) {
		t.Fatal("fixed event was mutated")
	}
}

func TestLowConfidenceAvailabilityProducesNoProposal(t *testing.T) {
	now := time.Now().UTC()
	task := domain.FlexibleTask{
		ID: "task", EstimatedDuration: time.Hour,
		Constraint: domain.TaskConstraint{MinimumConfidence: domain.ConfidenceHigh},
	}
	_, err := (Scheduler{}).Propose(Request{
		Task:         task,
		Availability: []domain.AvailabilityWindow{window(now, now.Add(4*time.Hour), "UTC", domain.ConfidenceLow)},
		Now:          now,
	})
	if !errors.Is(err, ErrNoReliableWindow) {
		t.Fatalf("error = %v", err)
	}
	if got := ClassifyUnplaced(err); got != ReasonNoAvailableInterval {
		t.Fatalf("unplaced reason = %q", got)
	}
}

func TestProposalReportsEnforcedExplanationCodes(t *testing.T) {
	zone := "America/New_York"
	location, _ := time.LoadLocation(zone)
	day := time.Date(2026, 6, 15, 0, 0, 0, 0, location)
	availability := window(day.Add(8*time.Hour), day.Add(18*time.Hour), zone, domain.ConfidenceHigh)
	event := domain.CalendarEvent{
		ID: "fixed-1", Title: "Appointment", Fixed: true,
		Interval: rangeAt(day.Add(9*time.Hour), day.Add(10*time.Hour+30*time.Minute), zone),
	}
	task := domain.FlexibleTask{
		ID: "task-1", Title: "Paperwork", EstimatedDuration: 90 * time.Minute,
		Constraint: domain.TaskConstraint{
			BusinessHours: true, BusinessStartLocal: "09:00", BusinessEndLocal: "17:00",
			MinimumConfidence: domain.ConfidenceMedium, RequiresApproval: true,
		},
	}
	proposal, err := (Scheduler{}).Propose(Request{Task: task, Availability: []domain.AvailabilityWindow{availability}, Events: []domain.CalendarEvent{event}, Now: day})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{CodeWithinPredictedWakingWindow, CodeAvoidsFixedEvent, CodeWithinTaskBounds}
	if !reflect.DeepEqual(proposal.ExplanationCodes, want) {
		t.Fatalf("explanation codes = %v, want %v", proposal.ExplanationCodes, want)
	}
	for _, code := range proposal.ExplanationCodes {
		if code == CodeUncertaintyBufferApplied {
			t.Fatal("engine must not claim an uncertainty buffer it does not apply")
		}
	}
}

func TestProposalWithoutBoundsStillReportsAtLeastOneCode(t *testing.T) {
	now := time.Now().UTC()
	task := domain.FlexibleTask{ID: "t", EstimatedDuration: time.Hour, Constraint: domain.TaskConstraint{MinimumConfidence: domain.ConfidenceLow}}
	free := domain.AvailabilityWindow{
		ID: "free", Kind: domain.AvailabilityFree,
		Interval:   rangeAt(now, now.Add(4*time.Hour), "UTC"),
		Confidence: domain.InferenceConfidence{Level: domain.ConfidenceMedium},
	}
	proposal, err := (Scheduler{}).Propose(Request{Task: task, Availability: []domain.AvailabilityWindow{free}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.ExplanationCodes) < 1 {
		t.Fatal("contract requires at least one explanation code")
	}
}

func window(start, end time.Time, zone string, confidence domain.ConfidenceLevel) domain.AvailabilityWindow {
	return domain.AvailabilityWindow{
		ID: "window", Kind: domain.AvailabilityPredictedWake,
		Interval:   rangeAt(start, end, zone),
		Confidence: domain.InferenceConfidence{Level: confidence},
		EstimateID: "estimate",
	}
}

func rangeAt(start, end time.Time, zone string) domain.TimeRange {
	return domain.TimeRange{Start: domain.MustZonedInstant(start, zone), End: domain.MustZonedInstant(end, zone)}
}
