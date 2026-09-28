package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

func proposeThroughTheEndpoint(t *testing.T, capability desktopLocalCapability, tool, arguments string) localAgentProposalResult {
	t.Helper()
	var result localAgentProposalResult
	if err := json.Unmarshal([]byte(callLocalAgentTool(t, capability, tool, arguments)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Result != "proposed" || result.Action != tool || len(result.Proposals) != 1 || result.Proposals[0].Status != "pending" {
		t.Fatalf("%s result = %+v", tool, result)
	}
	return result
}

// An agent can only ask: the dose it proposes is no record until the owner
// accepts it, and then it is the owner's dose like any other.
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
	raw := callLocalAgentTool(t, capability, "propose_log_dose",
		`{"target":{"medication_id":"`+medicationID+`","status":"taken","dose_at":"`+doseAt.Format(time.RFC3339)+`"}}`)
	if strings.Contains(raw, label) {
		t.Fatalf("the result carried the private label: %s", raw)
	}
	proposed := proposeThroughTheEndpoint(t, capability, "propose_log_dose", `{"target":{"medication_id":"`+medicationID+`","status":"skipped"}}`)
	if doses := doseHistory(t, app); len(doses) != 0 {
		t.Fatalf("a proposal recorded a dose: %+v", doses)
	}
	snapshot, err := app.assistantSnapshot(context.Background(), now)
	if err != nil || snapshot.Medication.PendingDoseProposals != 2 || snapshot.Tasks.PendingTaskProposals != 0 {
		t.Fatalf("snapshot counts %d waiting doses, %d tasks, %v", snapshot.Medication.PendingDoseProposals, snapshot.Tasks.PendingTaskProposals, err)
	}

	queue, err := app.GetAgentProposals()
	if err != nil || len(queue.Pending) != 2 {
		t.Fatalf("queue = %+v, %v", queue, err)
	}
	var taken AgentProposalDTO
	for _, waiting := range queue.Pending {
		if waiting.Dose != nil && waiting.Dose.Status == "taken" {
			taken = waiting
		}
	}
	if taken.Title != "Record dose" || taken.ActionID != "propose_log_dose" || taken.Task != nil ||
		taken.Dose.MedicationLabel != label || taken.Dose.DoseAt != doseAt.Format(time.RFC3339) ||
		taken.State != storage.AgentProposalPending || taken.ExpiresAt != now.Add(storage.AgentProposalLifetime).Format(time.RFC3339) ||
		queue.NextExpiryAt != taken.ExpiresAt {
		t.Fatalf("waiting dose = %+v, next expiry %s", taken, queue.NextExpiryAt)
	}

	later := now.Add(2 * time.Hour)
	app.nowFn = func() time.Time { return later }
	queue, err = app.DecideAgentProposal(AgentProposalDecisionInput{ProposalID: taken.ProposalID, Decision: "approved"})
	if err != nil || len(queue.Pending) != 1 || len(queue.History) != 1 || queue.History[0].State != storage.AgentProposalApproved {
		t.Fatalf("after accepting: %+v, %v", queue, err)
	}
	doses := doseHistory(t, app)
	if len(doses) != 1 {
		t.Fatalf("doses = %+v", doses)
	}
	if dose := doses[0]; dose.MedicationID != medicationID || dose.Status != "taken" || dose.Scheduled ||
		dose.DoseAt != doseAt.Format(time.RFC3339) {
		t.Fatalf("recorded dose = %+v", dose)
	}
	if _, err := app.DecideAgentProposal(AgentProposalDecisionInput{ProposalID: taken.ProposalID, Decision: "approved"}); err == nil ||
		!strings.Contains(err.Error(), "already decided") {
		t.Fatalf("accepting twice: %v", err)
	}

	// Declining records nothing.
	queue, err = app.DecideAgentProposal(AgentProposalDecisionInput{ProposalID: proposed.Proposals[0].ProposalID, Decision: "rejected"})
	if err != nil || len(queue.Pending) != 0 || len(queue.History) != 2 {
		t.Fatalf("after declining: %+v, %v", queue, err)
	}
	for _, decided := range queue.History {
		want := storage.AgentProposalApproved
		if decided.ProposalID == proposed.Proposals[0].ProposalID {
			want = storage.AgentProposalRejected
		}
		if decided.State != want {
			t.Fatalf("%s is %s, want %s", decided.ProposalID, decided.State, want)
		}
	}
	if doses := doseHistory(t, app); len(doses) != 1 {
		t.Fatalf("declining recorded a dose: %+v", doses)
	}
	if snapshot, err := app.assistantSnapshot(context.Background(), later); err != nil || snapshot.Medication.PendingDoseProposals != 0 {
		t.Fatalf("snapshot still counts waiting doses: %v", err)
	}
}

// "Remind me to call the pharmacy, fifteen minutes, before Friday": the task
// waits for the owner, and once accepted it is theirs, planned like any other.
func TestAnAgentProposesATaskAndOnlyTheOwnerAddsIt(t *testing.T) {
	app := newTestApp(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return now }
	capability := desktopLocalCapability{app: app}
	finish := now.Add(72 * time.Hour)
	proposed := proposeThroughTheEndpoint(t, capability, "propose_add_task",
		`{"target":{"title":" Call the pharmacy ","duration_minutes":15,"latest_finish_at":"`+finish.Format(time.RFC3339)+`"}}`)
	if tasks, err := app.ListTasks(); err != nil || len(tasks.Tasks) != 0 {
		t.Fatalf("a proposal added a task: %+v %v", tasks.Tasks, err)
	}
	if snapshot, err := app.assistantSnapshot(context.Background(), now); err != nil || snapshot.Tasks.PendingTaskProposals != 1 {
		t.Fatalf("snapshot does not count the waiting task: %v", err)
	}
	queue, err := app.GetAgentProposals()
	if err != nil || len(queue.Pending) != 1 || queue.Pending[0].Task == nil || queue.Pending[0].Dose != nil {
		t.Fatalf("queue = %+v, %v", queue, err)
	}
	waiting := queue.Pending[0]
	if waiting.Title != "Add task" || waiting.Task.Title != "Call the pharmacy" || waiting.Task.DurationMinutes != 15 ||
		waiting.Task.LatestFinishAt != finish.Format(time.RFC3339) || waiting.Task.EarliestStartAt != "" {
		t.Fatalf("waiting task = %+v", waiting)
	}

	if _, err := app.DecideAgentProposal(AgentProposalDecisionInput{ProposalID: proposed.Proposals[0].ProposalID, Decision: "approved"}); err != nil {
		t.Fatal(err)
	}
	tasks, err := app.ListTasks()
	if err != nil || len(tasks.Tasks) != 1 {
		t.Fatalf("tasks = %+v, %v", tasks.Tasks, err)
	}
	if added := tasks.Tasks[0]; added.Title != "Call the pharmacy" || added.DurationMinutes != 15 || added.Status != storage.TaskStatusOpen {
		t.Fatalf("added task = %+v", added)
	}
}

func TestAnAgentCannotProposeAnInvalidDoseOrTask(t *testing.T) {
	app := newTestApp(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return now }
	medications, err := app.AddMedication(MedicationInput{Label: "Private evening tablet"})
	if err != nil {
		t.Fatal(err)
	}
	medicationID := medications.Medications[0].MedicationID
	capability := desktopLocalCapability{app: app}
	for name, call := range map[string][2]string{
		"a label for the id": {"propose_log_dose", `{"target":{"medication_id":"Private evening tablet","status":"taken"}}`},
		"an unknown id":      {"propose_log_dose", `{"target":{"medication_id":"med_not_here","status":"taken"}}`},
		"a dose amount":      {"propose_log_dose", `{"target":{"medication_id":"` + medicationID + `","status":"two tablets"}}`},
		"a future dose":      {"propose_log_dose", `{"target":{"medication_id":"` + medicationID + `","status":"taken","dose_at":"` + now.Add(time.Hour).Format(time.RFC3339) + `"}}`},
		"an old dose":        {"propose_log_dose", `{"target":{"medication_id":"` + medicationID + `","status":"taken","dose_at":"` + now.Add(-8*24*time.Hour).Format(time.RFC3339) + `"}}`},
		// Notes are private and typed by the owner; an agent cannot attach one.
		"a note":           {"propose_log_dose", `{"target":{"medication_id":"` + medicationID + `","status":"taken","note":"said so on the phone"}}`},
		"a scheduled mark": {"propose_log_dose", `{"target":{"medication_id":"` + medicationID + `","status":"taken","scheduled":true}}`},
		"no target":        {"propose_log_dose", `{}`},
		"no title":         {"propose_add_task", `{"target":{"duration_minutes":15}}`},
		"a two-line title": {"propose_add_task", `{"target":{"title":"Call\nthe pharmacy","duration_minutes":15}}`},
		"a long title":     {"propose_add_task", `{"target":{"title":"` + strings.Repeat("x", 121) + `","duration_minutes":15}}`},
		"a tiny task":      {"propose_add_task", `{"target":{"title":"Stretch","duration_minutes":2}}`},
		"a past finish":    {"propose_add_task", `{"target":{"title":"Taxes","duration_minutes":60,"latest_finish_at":"` + now.Add(-time.Hour).Format(time.RFC3339) + `"}}`},
		// A task an agent proposes is new; it cannot name or reshape one.
		"a task id":   {"propose_add_task", `{"target":{"task_id":"task_existing","title":"Taxes","duration_minutes":60}}`},
		"task notes":  {"propose_add_task", `{"target":{"title":"Taxes","duration_minutes":60,"notes":"bring receipts"}}`},
		"no duration": {"propose_add_task", `{"target":{"title":"Taxes"}}`},
	} {
		_, err := capability.CallTool(context.Background(), call[0], json.RawMessage(call[1]))
		if err == nil || !strings.Contains(err.Error(), "Nothing was proposed") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if queue, err := app.GetAgentProposals(); err != nil || len(queue.Pending) != 0 {
		t.Fatalf("an invalid proposal was queued: %+v, %v", queue, err)
	}
}
