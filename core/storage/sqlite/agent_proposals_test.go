package sqlite

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// fixedID names the record an approval makes, whatever its kind.
func fixedID(id string) func(string) string { return func(string) string { return id } }

func testProposedDose(now time.Time) ProposedDose {
	return ProposedDose{
		MedicationID: "med_local_01",
		Status:       MedicationEventTaken,
		DoseAt:       now.Add(-20 * time.Minute),
		ZoneID:       "America/New_York",
	}
}

// A proposed dose is no record at all until the owner approves it; then it is
// the same dose a hand-logged one is, and it uploads like one.
func TestAProposedDoseBecomesARecordOnlyWhenTheOwnerApprovesIt(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	if err := store.CreateMedication(ctx, testMedicationRecord(now)); err != nil {
		t.Fatal(err)
	}
	dose := testProposedDose(now)
	if err := store.ProposeDose(ctx, "agent_proposal_01", dose, now); err != nil {
		t.Fatal(err)
	}
	if events, err := store.ListMedicationEvents(ctx); err != nil || len(events) != 0 {
		t.Fatalf("a proposal recorded a dose: %v %v", events, err)
	}
	listed, err := store.AgentProposals(ctx, now)
	if err != nil || len(listed) != 1 || listed[0].State != AgentProposalPending || listed[0].Dose == nil || listed[0].Task != nil ||
		!listed[0].ExpiresAt.Equal(now.Add(AgentProposalLifetime)) || !listed[0].Dose.DoseAt.Equal(dose.DoseAt) {
		t.Fatalf("waiting proposals = %+v, %v", listed, err)
	}

	approvedAt := now.Add(3 * time.Hour)
	approved, err := store.ApproveAgentProposal(ctx, "agent_proposal_01", approvedAt, fixedID("dose_from_proposal"))
	if err != nil || approved.State != AgentProposalApproved || approved.ResultID != "dose_from_proposal" {
		t.Fatalf("approved = %+v, %v", approved, err)
	}
	events, err := store.ListMedicationEvents(ctx)
	if err != nil || len(events) != 1 {
		t.Fatalf("events = %v, %v", events, err)
	}
	event := events[0]
	if event.EventID != "dose_from_proposal" || event.MedicationID != dose.MedicationID || event.Status != MedicationEventTaken ||
		!event.DoseAt.Equal(dose.DoseAt) || event.ZoneID != dose.ZoneID || event.Scheduled || event.Note != "" ||
		event.Provenance.AcquisitionMethod != ProvenanceAcquisitionManual ||
		event.Provenance.EvidenceStatus != ProvenanceEvidenceUserReported || !event.Provenance.RecordedAt.Equal(approvedAt) {
		t.Fatalf("recorded dose = %+v", event)
	}
	listed, err = store.AgentProposals(ctx, approvedAt)
	if err != nil || listed[0].State != AgentProposalApproved || listed[0].ResultID != event.EventID || !listed[0].DecidedAt.Equal(approvedAt) {
		t.Fatalf("approved proposal = %+v, %v", listed, err)
	}
	if _, err := store.ApproveAgentProposal(ctx, "agent_proposal_01", approvedAt, fixedID("dose_again")); !errors.Is(err, ErrAgentProposalDecided) {
		t.Fatalf("approving twice: %v", err)
	}
	if err := store.RejectAgentProposal(ctx, "agent_proposal_01", approvedAt); !errors.Is(err, ErrAgentProposalDecided) {
		t.Fatalf("rejecting an approved proposal: %v", err)
	}
	pending, err := store.PendingMedicationSyncRecords(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	uploads := false
	for _, record := range pending {
		uploads = uploads || (record.Kind == SyncKindMedicationEvent && record.RecordID == event.EventID)
	}
	if !uploads {
		t.Fatalf("the recorded dose does not upload: %+v", pending)
	}
}

// A proposed task is the task the owner would have typed: added only on
// approval, open, at its first revision, with the proposal's bounds.
func TestAProposedTaskIsAddedOnlyWhenTheOwnerApprovesIt(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	finish := now.Add(48 * time.Hour)
	task := ProposedTask{Title: "  Call the pharmacy  ", DurationMinutes: 15, LatestFinishAt: &finish}
	if err := store.ProposeTask(ctx, "agent_proposal_task", task, now); err != nil {
		t.Fatal(err)
	}
	if tasks, err := store.ListTasks(ctx); err != nil || len(tasks) != 0 {
		t.Fatalf("a proposal added a task: %v %v", tasks, err)
	}
	listed, err := store.AgentProposals(ctx, now)
	if err != nil || len(listed) != 1 || listed[0].Task == nil || listed[0].Task.Title != "Call the pharmacy" {
		t.Fatalf("waiting proposals = %+v, %v", listed, err)
	}

	approvedAt := now.Add(time.Hour)
	if _, err := store.ApproveAgentProposal(ctx, "agent_proposal_task", approvedAt, fixedID("task_from_proposal")); err != nil {
		t.Fatal(err)
	}
	tasks, err := store.ListTasks(ctx)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks = %v, %v", tasks, err)
	}
	added := tasks[0]
	if added.TaskID != "task_from_proposal" || added.Title != "Call the pharmacy" || added.DurationMinutes != 15 ||
		added.Status != TaskStatusOpen || added.Revision != 1 || !added.CreatedAt.Equal(approvedAt) ||
		added.LatestFinishAt == nil || !added.LatestFinishAt.Equal(finish) || added.EarliestStartAt != nil {
		t.Fatalf("added task = %+v", added)
	}

	// A task the store would refuse is refused as a proposal.
	for name, bad := range map[string]ProposedTask{
		"no title":     {DurationMinutes: 15},
		"a long title": {Title: fmt.Sprintf("%0121d", 0), DurationMinutes: 15},
		"too short":    {Title: "Stretch", DurationMinutes: 4},
		"finish first": {Title: "Taxes", DurationMinutes: 30, EarliestStartAt: &finish, LatestFinishAt: &now},
	} {
		if err := store.ProposeTask(ctx, "agent_proposal_bad", bad, now); err == nil {
			t.Errorf("%s: queued", name)
		}
	}
}

func TestAgentProposalsAreRejectedLapseAndStayBounded(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	if err := store.CreateMedication(ctx, testMedicationRecord(now)); err != nil {
		t.Fatal(err)
	}

	if err := store.ProposeDose(ctx, "agent_proposal_reject", testProposedDose(now), now); err != nil {
		t.Fatal(err)
	}
	if err := store.RejectAgentProposal(ctx, "agent_proposal_reject", now); err != nil {
		t.Fatal(err)
	}
	if err := store.ProposeTask(ctx, "agent_proposal_lapse", ProposedTask{Title: "Taxes", DurationMinutes: 90}, now); err != nil {
		t.Fatal(err)
	}
	lapsed := now.Add(AgentProposalLifetime)
	if _, err := store.ApproveAgentProposal(ctx, "agent_proposal_lapse", lapsed, fixedID("task_too_late")); !errors.Is(err, ErrAgentProposalExpired) {
		t.Fatalf("approving a lapsed proposal: %v", err)
	}
	if _, err := store.ApproveAgentProposal(ctx, "agent_proposal_unknown", now, fixedID("dose_unknown")); !errors.Is(err, ErrAgentProposalNotFound) {
		t.Fatalf("approving an unknown proposal: %v", err)
	}
	listed, err := store.AgentProposals(ctx, lapsed)
	if err != nil || len(listed) != 2 {
		t.Fatalf("proposals = %+v, %v", listed, err)
	}
	states := map[string]string{}
	for _, proposal := range listed {
		states[proposal.ProposalID] = proposal.State
	}
	if states["agent_proposal_reject"] != AgentProposalRejected || states["agent_proposal_lapse"] != AgentProposalExpired {
		t.Fatalf("states = %v", states)
	}
	events, err := store.ListMedicationEvents(ctx)
	tasks, taskErr := store.ListTasks(ctx)
	if err != nil || taskErr != nil || len(events) != 0 || len(tasks) != 0 {
		t.Fatalf("a rejected or lapsed proposal made a record: %v %v", events, tasks)
	}

	unknown := testProposedDose(now)
	unknown.MedicationID = "med_not_here"
	if err := store.ProposeDose(ctx, "agent_proposal_unknown_med", unknown, now); !errors.Is(err, ErrMedicationNotFound) {
		t.Fatalf("unknown medication: %v", err)
	}
	// Doses and tasks share one bound.
	for index := 0; index < MaxPendingAgentProposals; index++ {
		id := fmt.Sprintf("agent_proposal_many_%02d", index)
		var err error
		if index%2 == 0 {
			err = store.ProposeDose(ctx, id, testProposedDose(lapsed), lapsed)
		} else {
			err = store.ProposeTask(ctx, id, ProposedTask{Title: "Errand", DurationMinutes: 30}, lapsed)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ProposeTask(ctx, "agent_proposal_one_too_many", ProposedTask{Title: "Errand", DurationMinutes: 30}, lapsed); !errors.Is(err, ErrTooManyAgentProposals) {
		t.Fatalf("over the limit: %v", err)
	}
	// Once they lapse, agents may propose again, and a month on the history is
	// gone.
	month := lapsed.Add(agentProposalHistory + AgentProposalLifetime + time.Second)
	if err := store.ProposeDose(ctx, "agent_proposal_next_month", testProposedDose(month), month); err != nil {
		t.Fatal(err)
	}
	if listed, err := store.AgentProposals(ctx, month); err != nil || len(listed) != 1 || listed[0].ProposalID != "agent_proposal_next_month" {
		t.Fatalf("history was not pruned: %d proposals, %v", len(listed), err)
	}

	inactive := testMedicationRecord(now)
	inactive.MedicationID = "med_inactive"
	inactive.Active = false
	if err := store.CreateMedication(ctx, inactive); err != nil {
		t.Fatal(err)
	}
	stopped := testProposedDose(month)
	stopped.MedicationID = inactive.MedicationID
	if err := store.ProposeDose(ctx, "agent_proposal_inactive", stopped, month); !errors.Is(err, ErrMedicationNotActive) {
		t.Fatalf("inactive medication: %v", err)
	}
	malformed := testProposedDose(month)
	malformed.Status = "2 tablets"
	if err := store.ProposeDose(ctx, "agent_proposal_malformed", malformed, month); err == nil {
		t.Fatal("a malformed proposal was queued")
	}
}

func TestErasingAMedicationErasesItsDoseProposalsButNotTasks(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	if err := store.CreateMedication(ctx, testMedicationRecord(now)); err != nil {
		t.Fatal(err)
	}
	if err := store.ProposeDose(ctx, "agent_proposal_erased", testProposedDose(now), now); err != nil {
		t.Fatal(err)
	}
	if err := store.ProposeTask(ctx, "agent_proposal_kept", ProposedTask{Title: "Errand", DurationMinutes: 30}, now); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteMedication(ctx, "med_local_01"); err != nil {
		t.Fatal(err)
	}
	if listed, err := store.AgentProposals(ctx, now); err != nil || len(listed) != 1 || listed[0].ProposalID != "agent_proposal_kept" {
		t.Fatalf("proposals after erasing the medication: %+v, %v", listed, err)
	}
}
