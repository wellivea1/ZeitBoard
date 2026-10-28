package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

// heldProposal is a server that holds one schedule proposal of the
// assistant's: the task, at a time. It records the decisions it is sent.
type heldProposal struct {
	mu        sync.Mutex
	taskID    string
	start     time.Time
	end       time.Time
	status    string
	decisions []string
}

func (h *heldProposal) serve(t *testing.T) *httptest.Server {
	t.Helper()
	record := func() map[string]any {
		h.mu.Lock()
		defer h.mu.Unlock()
		payload := mustJSON(t, map[string]any{
			"action_id": "propose_place_task",
			"schedule_proposals": map[string]any{"proposals": []map[string]any{{
				"task_id": h.taskID, "start_at": h.start, "end_at": h.end, "zone_id": "UTC",
				"confidence": map[string]any{"level": "high"}, "explanation_codes": []string{"within_task_bounds"},
			}}},
		})
		return map[string]any{
			"proposalId": "proposal_server_01", "actionId": "propose_place_task", "status": h.status,
			"createdAt": time.Now().UTC(), "expiresAt": time.Now().UTC().Add(15 * time.Minute),
			"payload": payload, "decisionToken": "one-use-token",
		}
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "device-token"})
		case r.URL.Path == "/v1/proposals/proposal_server_01" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": "v1", "proposal": record()})
		case r.URL.Path == "/v1/proposals/proposal_server_01/decision" && r.Method == http.MethodPost:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			h.mu.Lock()
			h.decisions = append(h.decisions, body["decision"])
			h.status = body["decision"]
			h.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": "v1"})
		case r.URL.Path == "/v1/proposals" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": "v1", "proposals": []any{record()}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func (h *heldProposal) decided() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.decisions...)
}

// plannedApp is a computer with an estimate and one hour-long task, and the
// time its planner would suggest for it.
func plannedApp(t *testing.T) (*App, localProposalCandidate, string) {
	t.Helper()
	app := newTestApp(t)
	now := time.Now().UTC().Truncate(localProposalTTL).Add(5 * time.Minute)
	app.nowFn = func() time.Time { return now }
	seedSleepEntries(t, app, 12)
	if _, err := app.AddTask(TaskInput{Title: "File paperwork", DurationMinutes: 60}); err != nil {
		t.Fatal(err)
	}
	built, err := app.buildLocalProposals(now)
	if err != nil {
		t.Fatal(err)
	}
	id, candidate := onlyPendingCandidate(t, built)
	return app, candidate, id
}

// acceptedTimes are the proposals whose blocks stand on this computer.
func acceptedTimes(t *testing.T, app *App) []storage.ProposalDecisionRecord {
	t.Helper()
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.ActiveProposalDecisions(app.applicationContext())
	if err != nil {
		t.Fatal(err)
	}
	var approved []storage.ProposalDecisionRecord
	for _, record := range active {
		if record.Decision == storage.ProposalApproved {
			approved = append(approved, record)
		}
	}
	events, err := store.OwnedCalendarEvents(app.applicationContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != len(approved) {
		t.Fatalf("%d blocks for %d accepted times", len(events), len(approved))
	}
	return approved
}

// Approving an assistant's proposal writes the task's accepted time here, as
// accepting the planner's suggestion does; the planner then suggests no other.
// On the first real run, approving wrote nothing anywhere.
func TestApprovingAnAssistantProposalWritesTheTasksTime(t *testing.T) {
	app, candidate, _ := plannedApp(t)
	held := &heldProposal{
		taskID: candidate.task.TaskID, status: "pending",
		start: candidate.proposal.Window.Start.UTC, end: candidate.proposal.Window.End.UTC,
	}
	configureBackendForTest(t, app, held.serve(t).URL)

	result, err := app.DecideBackendProposal(BackendProposalDecisionInput{
		ProposalID: "proposal_server_01", Decision: "approved", Token: "one-use-token",
	})
	if err != nil || result.Status != "ok" || !result.DecisionRecorded {
		t.Fatalf("decided %#v, %v", result, err)
	}
	if got := held.decided(); len(got) != 1 || got[0] != "approved" {
		t.Fatalf("the server was sent %v", got)
	}
	accepted := acceptedTimes(t, app)
	if len(accepted) != 1 || accepted[0].ProposalID != "proposal_server_01" ||
		!accepted[0].ProposalStartAt.Equal(held.start) || accepted[0].ProposalTitle != "File paperwork" {
		t.Fatalf("accepted times %#v", accepted)
	}
	rebuilt, err := app.buildLocalProposals(app.currentTime())
	if err != nil || len(rebuilt.pending) != 0 {
		t.Fatalf("the planner still suggests %d times, %v", len(rebuilt.pending), err)
	}

	// Undone here, the time goes; the server's record of the approval reads
	// as undone here rather than standing.
	if _, err := app.UndoLocalProposalDecision(LocalProposalUndoInput{ProposalID: "proposal_server_01"}); err != nil {
		t.Fatal(err)
	}
	if accepted := acceptedTimes(t, app); len(accepted) != 0 {
		t.Fatalf("accepted times after undo %#v", accepted)
	}
	list, err := app.GetBackendProposals()
	if err != nil || len(list.Proposals) != 1 || list.Proposals[0].Status != "approved" || !list.Proposals[0].UndoneHere {
		t.Fatalf("the server's record after undo %#v, %v", list, err)
	}
}

// A task keeps one accepted time: approving a new one undoes the old, even a
// time overlapping the one the task now has.
func TestAnApprovedProposalTakesThePlaceOfTheTasksTime(t *testing.T) {
	app, candidate, localID := plannedApp(t)
	if _, err := app.DecideLocalProposal(LocalProposalDecisionInput{ProposalID: localID, Decision: storage.ProposalApproved}); err != nil {
		t.Fatal(err)
	}
	held := &heldProposal{
		taskID: candidate.task.TaskID, status: "pending",
		start: candidate.proposal.Window.Start.UTC, end: candidate.proposal.Window.End.UTC,
	}
	configureBackendForTest(t, app, held.serve(t).URL)

	result, err := app.DecideBackendProposal(BackendProposalDecisionInput{
		ProposalID: "proposal_server_01", Decision: "approved", Token: "one-use-token",
	})
	if err != nil || result.Status != "ok" {
		t.Fatalf("decided %#v, %v", result, err)
	}
	if accepted := acceptedTimes(t, app); len(accepted) != 1 || accepted[0].ProposalID != "proposal_server_01" {
		t.Fatalf("accepted times %#v", accepted)
	}
}

// A time that does not fit here is not approved: the server is not asked,
// and the owner reads why.
func TestATimeThatDoesNotFitIsNotApproved(t *testing.T) {
	app, candidate, _ := plannedApp(t)
	start := candidate.proposal.Window.Start.UTC
	for name, test := range map[string]struct {
		start, end time.Time
		why        string
	}{
		"too short": {start, start.Add(30 * time.Minute), "That time lasts 30 minutes, but File paperwork takes 1 hour."},
		"begun":     {app.currentTime().Add(-time.Minute), app.currentTime().Add(59 * time.Minute), "That time has already begun."},
	} {
		t.Run(name, func(t *testing.T) {
			held := &heldProposal{taskID: candidate.task.TaskID, status: "pending", start: test.start, end: test.end}
			configureBackendForTest(t, app, held.serve(t).URL)
			result, err := app.DecideBackendProposal(BackendProposalDecisionInput{
				ProposalID: "proposal_server_01", Decision: "approved", Token: "one-use-token",
			})
			if err != nil || result.Status != "ok" || result.DecisionRecorded || result.DecisionNote != "Not approved. "+test.why {
				t.Fatalf("decided %#v, %v", result, err)
			}
			if got := held.decided(); len(got) != 0 {
				t.Errorf("the server was sent %v", got)
			}
			if accepted := acceptedTimes(t, app); len(accepted) != 0 {
				t.Errorf("accepted times %#v", accepted)
			}
		})
	}
	if !strings.HasPrefix(acceptanceProblem(storage.ErrStaleProposal), "Your records changed") {
		t.Error("a race reads as a race")
	}
}
