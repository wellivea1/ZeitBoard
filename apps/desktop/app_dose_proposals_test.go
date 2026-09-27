package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

// An agent can only ask: the dose it proposes is no record until the owner
// records it, and then it is the owner's dose like any other.
func TestAnAgentProposesADoseAndOnlyTheOwnerRecordsIt(t *testing.T) {
	app := newTestApp(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return now }
	const label = "Private evening tablet"
	medications, err := app.AddMedication(MedicationInput{Label: label})
	if err != nil {
		t.Fatal(err)
	}
	medicationID := medications.Medications[0].MedicationID
	capability := desktopLocalCapability{app: app}

	doseAt := now.Add(-20 * time.Minute)
	result := callLocalAgentTool(t, capability, "propose_log_dose",
		`{"target":{"medication_id":"`+medicationID+`","status":"taken","dose_at":"`+doseAt.Format(time.RFC3339)+`"}}`)
	var proposed localAgentProposalResult
	if err := json.Unmarshal([]byte(result), &proposed); err != nil {
		t.Fatal(err)
	}
	if proposed.Result != "proposed" || proposed.Action != "propose_log_dose" || len(proposed.Proposals) != 1 ||
		proposed.Proposals[0].Status != "pending" || strings.Contains(result, label) {
		t.Fatalf("proposal result = %s", result)
	}
	if doses, err := app.GetMedications(); err != nil || len(doses.Events) != 0 {
		t.Fatalf("a proposal recorded a dose: %+v %v", doses.Events, err)
	}
	snapshot, err := app.assistantSnapshot(context.Background(), now)
	if err != nil || snapshot.Medication.PendingDoseProposals != 1 {
		t.Fatalf("snapshot counts %d waiting doses, %v", snapshot.Medication.PendingDoseProposals, err)
	}

	queue, err := app.GetDoseProposals()
	if err != nil || len(queue.Pending) != 1 {
		t.Fatalf("queue = %+v, %v", queue, err)
	}
	waiting := queue.Pending[0]
	if waiting.ProposalID != proposed.Proposals[0].ProposalID || waiting.Title != "Record dose" ||
		waiting.MedicationLabel != label || waiting.Status != "taken" || waiting.DoseAt != doseAt.Format(time.RFC3339) ||
		waiting.State != storage.DoseProposalPending || waiting.ExpiresAt != queue.NextExpiryAt ||
		waiting.ExpiresAt != now.Add(storage.DoseProposalLifetime).Format(time.RFC3339) {
		t.Fatalf("waiting dose = %+v, next expiry %s", waiting, queue.NextExpiryAt)
	}

	later := now.Add(2 * time.Hour)
	app.nowFn = func() time.Time { return later }
	queue, err = app.DecideDoseProposal(DoseProposalDecisionInput{ProposalID: waiting.ProposalID, Decision: "record"})
	if err != nil || len(queue.Pending) != 0 || len(queue.History) != 1 || queue.History[0].State != storage.DoseProposalRecorded {
		t.Fatalf("after recording: %+v, %v", queue, err)
	}
	doses, err := app.GetMedications()
	if err != nil || len(doses.Events) != 1 {
		t.Fatalf("doses = %+v, %v", doses.Events, err)
	}
	if dose := doses.Events[0]; dose.MedicationID != medicationID || dose.Status != "taken" || dose.Scheduled ||
		dose.DoseAt != doseAt.Format(time.RFC3339) {
		t.Fatalf("recorded dose = %+v", dose)
	}
	if _, err := app.DecideDoseProposal(DoseProposalDecisionInput{ProposalID: waiting.ProposalID, Decision: "record"}); err == nil ||
		!strings.Contains(err.Error(), "already decided") {
		t.Fatalf("recording twice: %v", err)
	}
	snapshot, err = app.assistantSnapshot(context.Background(), later)
	if err != nil || snapshot.Medication.PendingDoseProposals != 0 {
		t.Fatalf("snapshot still counts %d waiting doses, %v", snapshot.Medication.PendingDoseProposals, err)
	}

	// Discarding records nothing.
	result = callLocalAgentTool(t, capability, "propose_log_dose", `{"target":{"medication_id":"`+medicationID+`","status":"skipped"}}`)
	if err := json.Unmarshal([]byte(result), &proposed); err != nil {
		t.Fatal(err)
	}
	queue, err = app.DecideDoseProposal(DoseProposalDecisionInput{ProposalID: proposed.Proposals[0].ProposalID, Decision: "discard"})
	if err != nil || len(queue.Pending) != 0 || queue.History[0].State != storage.DoseProposalDiscarded {
		t.Fatalf("after discarding: %+v, %v", queue, err)
	}
	if doses, err := app.GetMedications(); err != nil || len(doses.Events) != 1 {
		t.Fatalf("discarding recorded a dose: %+v %v", doses.Events, err)
	}
}

func TestAnAgentCannotProposeAnInvalidDose(t *testing.T) {
	app := newTestApp(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return now }
	medications, err := app.AddMedication(MedicationInput{Label: "Private evening tablet"})
	if err != nil {
		t.Fatal(err)
	}
	medicationID := medications.Medications[0].MedicationID
	capability := desktopLocalCapability{app: app}
	for name, arguments := range map[string]string{
		"a label for the id": `{"target":{"medication_id":"Private evening tablet","status":"taken"}}`,
		"an unknown id":      `{"target":{"medication_id":"med_not_here","status":"taken"}}`,
		"a dose amount":      `{"target":{"medication_id":"` + medicationID + `","status":"two tablets"}}`,
		"a future dose":      `{"target":{"medication_id":"` + medicationID + `","status":"taken","dose_at":"` + now.Add(time.Hour).Format(time.RFC3339) + `"}}`,
		"an old dose":        `{"target":{"medication_id":"` + medicationID + `","status":"taken","dose_at":"` + now.Add(-8*24*time.Hour).Format(time.RFC3339) + `"}}`,
		// Notes are private and typed by the owner; an agent cannot attach one.
		"a note":           `{"target":{"medication_id":"` + medicationID + `","status":"taken","note":"said so on the phone"}}`,
		"a scheduled mark": `{"target":{"medication_id":"` + medicationID + `","status":"taken","scheduled":true}}`,
		"no target":        `{}`,
	} {
		_, err := capability.CallTool(context.Background(), "propose_log_dose", json.RawMessage(arguments))
		if err == nil || !strings.Contains(err.Error(), "No dose was proposed") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if queue, err := app.GetDoseProposals(); err != nil || len(queue.Pending) != 0 {
		t.Fatalf("an invalid proposal was queued: %+v, %v", queue, err)
	}
}
