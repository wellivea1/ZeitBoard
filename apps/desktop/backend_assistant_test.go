package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssistantOffMakesNoNetworkCalls(t *testing.T) {
	app := newTestApp(t)
	status, err := app.GetAssistantStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.Message == "" {
		t.Fatalf("assistant should be off with guidance: %#v", status)
	}
	reply, err := app.SendAssistantMessage(AssistantMessageInput{Message: "when am I likely awake tomorrow?"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Available || reply.Result != "unavailable" || reply.Answer == "" {
		t.Fatalf("assistant off should reply honestly: %#v", reply)
	}
}

func TestAssistantMessageSendsRedactedContextAndMapsProposals(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 12)
	const privateTitle = "Call Dr. Okafor about the private thing"
	list, err := app.AddTask(TaskInput{Title: privateTitle, DurationMinutes: 45})
	if err != nil {
		t.Fatal(err)
	}
	taskID := list.Tasks[0].TaskID

	var paths []string
	var assistantBody []byte
	payload := `{"proposal_id":"proposal_srv_01","action_id":"propose_place_task","schedule_proposals":{"schema_version":"v1","request_id":"req_01","generated_at":"2026-07-10T12:00:00Z","algorithm_version":"v1","proposals":[{"proposal_id":"sp_01","task_id":"` + taskID + `","start_at":"2026-07-10T15:00:00Z","end_at":"2026-07-10T15:45:00Z","zone_id":"UTC","confidence":{"level":"medium","reasons":[]},"explanation_codes":["within_predicted_waking_window"]}],"unplaced":[]},"created_at":"2026-07-10T12:00:00Z","expires_at":"2026-07-10T12:15:00Z"}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "assistant-token"})
		case "/v1/assistant/message":
			assistantBody, _ = io.ReadAll(r.Body)
			_, _ = w.Write([]byte(`{"schema_version":"v1","backend":{"configured":true,"provider":"anthropic","model":"claude-sonnet-5"},"result":"proposal_pending","action":"propose_place_task","answer":"I found a window inside your predicted waking time.","proposals":[{"proposalId":"proposal_srv_01","status":"pending","decisionToken":"one-use-token","payload":` + payload + `}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureBackendForTest(t, app, server.URL)

	reply, err := app.SendAssistantMessage(AssistantMessageInput{Message: "find 45 minutes for my call"})
	if err != nil {
		t.Fatal(err)
	}

	// Redaction: the request carries the task id and bounds, never the title
	// or any sleep record content.
	body := string(assistantBody)
	if strings.Contains(body, privateTitle) || strings.Contains(body, "Okafor") {
		t.Fatal("private task title leaked into the assistant request")
	}
	if strings.Contains(body, "\"title\"") || strings.Contains(body, "observation") {
		t.Fatalf("unexpected fields in assistant context: %s", body)
	}
	if !strings.Contains(body, taskID) || !strings.Contains(body, "\"availability\"") || !strings.Contains(body, "\"estimate_id\"") {
		t.Fatalf("assistant context missing redacted planning data: %s", body)
	}

	// Mapping: provider disclosed, proposal carries token and the LOCAL title.
	if !reply.Available || reply.Result != "proposal_pending" || reply.Provider != "anthropic" || !reply.Configured {
		t.Fatalf("reply = %#v", reply)
	}
	if len(reply.Proposals) != 1 {
		t.Fatalf("proposals = %#v", reply.Proposals)
	}
	proposal := reply.Proposals[0]
	if proposal.DecisionToken != "one-use-token" || proposal.Status != "pending" {
		t.Fatalf("proposal = %#v", proposal)
	}
	if !strings.Contains(proposal.Title, privateTitle) {
		t.Fatalf("rail should show the local title, got %q", proposal.Title)
	}
	if proposal.Window == "" || proposal.Window == "Window unavailable" {
		t.Fatalf("proposal window missing: %#v", proposal)
	}

	// No-mutation surface: the assistant flow touched only enrollment and the
	// message endpoint — there is no path that applies schedule changes.
	for _, path := range paths {
		if path != "/v1/devices" && path != "/v1/assistant/message" {
			t.Fatalf("unexpected endpoint touched by assistant flow: %s", path)
		}
	}
}

func TestAssistantMedicalRefusalPassesThroughWithoutProposal(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "refusal-token"})
		case "/v1/assistant/message":
			_, _ = w.Write([]byte(`{"schema_version":"v1","backend":{"configured":false,"provider":"none"},"result":"refused_medical","action":"answer_only","answer":"I can't help with medical decisions like medication or dosing.","proposals":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureBackendForTest(t, app, server.URL)

	reply, err := app.SendAssistantMessage(AssistantMessageInput{Message: "when should I take melatonin?"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Result != "refused_medical" || len(reply.Proposals) != 0 {
		t.Fatalf("medical refusal must create no proposal: %#v", reply)
	}
}
