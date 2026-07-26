package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"non24.app/server/internal/api"
	"non24.app/server/internal/auth"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

const testDeviceToken = "mcp-device-token"

func TestReadToolsReturnProjectedServerDTOs(t *testing.T) {
	srv, cleanup := newMCPIntegrationServer(t)
	defer cleanup()

	for _, name := range []string{"get_status", "get_overview", "get_rhythm", "get_accuracy", "list_proposals"} {
		result, err := srv.callTool(context.Background(), callParams(name, nil))
		if err != nil {
			t.Fatalf("%s call error: %v", name, err)
		}
		if result.IsError {
			t.Fatalf("%s returned tool error: %+v", name, result)
		}
		if len(result.StructuredContent) == 0 {
			t.Fatalf("%s missing structured content", name)
		}
		assertNoForbiddenToolFields(t, result.StructuredContent)
		if name == "get_overview" && !bytes.Contains(result.StructuredContent, []byte(`"status":"estimated"`)) {
			t.Fatalf("overview did not return estimated projection: %s", result.StructuredContent)
		}
		if name == "get_rhythm" && !bytes.Contains(result.StructuredContent, []byte(`"slopeLabel":"+50 min per cycle"`)) {
			t.Fatalf("rhythm did not return engine slope: %s", result.StructuredContent)
		}
	}
}

func TestProposeToolCreatesPendingProposalOnly(t *testing.T) {
	srv, cleanup := newMCPIntegrationServer(t)
	defer cleanup()

	result, err := srv.callTool(context.Background(), callParams("propose_place_task", proposeToolArguments()))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("propose tool returned error: %+v", result)
	}
	var response struct {
		Result    string `json:"result"`
		Action    string `json:"action"`
		Answer    string `json:"answer"`
		Proposals []struct {
			ProposalID    string               `json:"proposalId"`
			Status        store.ProposalStatus `json:"status"`
			DecisionToken string               `json:"decisionToken"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(result.StructuredContent, &response); err != nil {
		t.Fatal(err)
	}
	if response.Result != "proposal_pending" || response.Action != "propose_place_task" || len(response.Proposals) != 1 {
		t.Fatalf("propose response = %+v", response)
	}
	if response.Proposals[0].Status != store.ProposalPending || response.Proposals[0].DecisionToken == "" {
		t.Fatalf("proposal summary = %+v, want pending with approval token", response.Proposals[0])
	}
	if !strings.Contains(response.Answer, "human approval") {
		t.Fatalf("proposal answer should mention human approval: %q", response.Answer)
	}
}

func TestNoApprovalToolIsExposed(t *testing.T) {
	for _, tool := range toolDefinitions() {
		lower := strings.ToLower(tool.Name)
		for _, forbidden := range []string{"approve", "apply", "decision"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("MCP tool exposes forbidden approval/apply surface: %+v", tool)
			}
		}
	}
	properties, ok := proposeSchema()["properties"].(map[string]any)
	if !ok {
		t.Fatalf("proposal schema properties are malformed: %#v", proposeSchema())
	}
	if _, exposed := properties["answer"]; exposed {
		t.Fatal("proposal schema lets callers inject approval-card text")
	}
	if knownTool("approve_proposal") || knownTool("apply_proposal") || knownTool("decide_proposal") {
		t.Fatalf("approval/apply tool should not be known")
	}
	srv := &Server{Configured: true, Backend: okBackend(t), TotalRemaining: 5, ProposeRemaining: 2}
	if _, err := srv.callTool(context.Background(), callParams("approve_proposal", nil)); err == nil {
		t.Fatalf("approve_proposal unexpectedly callable")
	}
}

func TestCallBudgetsFailClosed(t *testing.T) {
	srv := &Server{Configured: true, Backend: okBackend(t), TotalRemaining: 1, ProposeRemaining: 1}
	first, err := srv.callTool(context.Background(), callParams("get_status", nil))
	if err != nil || first.IsError {
		t.Fatalf("first call = %+v err=%v", first, err)
	}
	second, err := srv.callTool(context.Background(), callParams("get_status", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !second.IsError || !strings.Contains(second.Content[0].Text, "budget") {
		t.Fatalf("second call should fail closed on budget: %+v", second)
	}

	proposeLimited := &Server{Configured: true, Backend: okBackend(t), TotalRemaining: 5, ProposeRemaining: 0}
	blocked, err := proposeLimited.callTool(context.Background(), callParams("propose_place_task", proposeToolArguments()))
	if err != nil {
		t.Fatal(err)
	}
	if !blocked.IsError || !strings.Contains(blocked.Content[0].Text, "Proposal call budget") {
		t.Fatalf("propose budget result = %+v", blocked)
	}
}

func TestUnconfiguredBackendExposesNoToolsAndDoesNotLeakToken(t *testing.T) {
	cfg := Config{
		BackendURL:        "https://127.0.0.1:1",
		DeviceToken:       "secret-token-value",
		TotalCallBudget:   1,
		Configured:        false,
		UnavailableReason: "backend URL is not configured",
	}
	if strings.Contains(cfg.StartupMessage(), cfg.DeviceToken) {
		t.Fatalf("startup message leaked device token")
	}
	srv := NewServer(cfg)
	if tools := srv.availableTools(context.Background()); len(tools) != 0 {
		t.Fatalf("unconfigured server exposed tools: %+v", tools)
	}
	result, err := srv.callTool(context.Background(), callParams("get_status", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || strings.Contains(result.Content[0].Text, cfg.DeviceToken) {
		t.Fatalf("unconfigured call result = %+v", result)
	}
}

func TestJSONRPCInitializeAndToolsList(t *testing.T) {
	srv, cleanup := newMCPIntegrationServer(t)
	defer cleanup()

	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n",
	)
	var output bytes.Buffer
	if err := srv.Serve(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSON-RPC response lines = %d output=%s", len(lines), output.String())
	}
	if !strings.Contains(lines[0], `"protocolVersion":"2025-11-25"`) {
		t.Fatalf("initialize response = %s", lines[0])
	}
	var listResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &listResp); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range listResp.Result.Tools {
		names = append(names, tool.Name)
		lower := strings.ToLower(tool.Name)
		if strings.Contains(lower, "approve") || strings.Contains(lower, "apply") || strings.Contains(lower, "decision") {
			t.Fatalf("tools/list exposed approval/apply tool: %s", lines[1])
		}
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "get_overview") || !strings.Contains(joined, "propose_place_task") {
		t.Fatalf("tools/list response = %s", lines[1])
	}
}

func newMCPIntegrationServer(t *testing.T) (*Server, func()) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "zeitboardd.db"), bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RegisterDevice(ctx, "dev_mcp_01", "mcp", auth.HashToken(testDeviceToken), time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	records := driftingSleepPushRecords(10, time.Date(2026, 2, 23, 1, 0, 0, 0, time.UTC), 24*time.Hour+50*time.Minute, 8*time.Hour)
	req := syncmodel.PushRequest{SchemaVersion: syncmodel.SchemaVersion, Records: records}
	if err := syncmodel.ValidatePushRequest(&req); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Append(ctx, "dev_mcp_01", req.Records); err != nil {
		t.Fatal(err)
	}
	apiServer := api.New(st, "enrollment-secret-123")
	apiServerTest := httptest.NewTLSServer(apiServer.Handler())
	srv := &Server{
		Backend:          &BackendClient{BaseURL: apiServerTest.URL, Token: testDeviceToken, Client: apiServerTest.Client()},
		Configured:       true,
		TotalRemaining:   20,
		ProposeRemaining: 5,
	}
	return srv, func() {
		apiServerTest.Close()
		_ = st.Close()
	}
}

func okBackend(t *testing.T) *BackendClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/status":
			_, _ = w.Write([]byte(`{"schema_version":"v1","assistant":{"configured":false,"provider":"disabled"}}`))
		case "/v1/proposals":
			_, _ = w.Write([]byte(`{"schema_version":"v1","result":"proposal_pending","action":"propose_place_task","answer":"awaits human approval","proposals":[]}`))
		default:
			_, _ = w.Write([]byte(`{"schema_version":"v1","status":"estimated"}`))
		}
	}))
	t.Cleanup(server.Close)
	return &BackendClient{BaseURL: server.URL, Token: "token", Client: server.Client()}
}

func callParams(name string, arguments json.RawMessage) json.RawMessage {
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	data, _ := json.Marshal(map[string]any{"name": name, "arguments": arguments})
	return data
}

func proposeToolArguments() json.RawMessage {
	return json.RawMessage(`{
  "target": {"task_id":"task_flexible_01"},
  "context": {
    "zone_id": "America/New_York",
    "now": "2026-03-05T12:00:00Z",
    "estimate_id": "estimate_synthetic_01",
    "tasks": [{
      "task_id": "task_flexible_01",
      "duration_minutes": 30,
      "earliest_start_at": "2026-03-05T15:00:00Z",
      "latest_finish_at": "2026-03-06T01:00:00Z",
      "minimum_confidence": "low"
    }],
    "availability": [{
      "kind": "predicted_wake",
      "start_at": "2026-03-05T14:30:00Z",
      "end_at": "2026-03-06T02:30:00Z",
      "zone_id": "America/New_York",
      "confidence": "medium"
    }],
    "fixed_events": [{
      "event_id": "event_fixed_01",
      "start_at": "2026-03-05T16:00:00Z",
      "end_at": "2026-03-05T17:00:00Z",
      "zone_id": "America/New_York"
    }]
  }
}`)
}

func driftingSleepPushRecords(count int, start time.Time, period, duration time.Duration) []syncmodel.PushRecord {
	records := make([]syncmodel.PushRecord, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("obs_sleep_%02d", i)
		sleepStart := start.Add(time.Duration(i) * period)
		records = append(records, syncmodel.PushRecord{
			RecordID:  id,
			Kind:      syncmodel.KindObservation,
			CreatedAt: sleepStart.Add(duration).Add(5 * time.Minute),
			Payload: json.RawMessage(fmt.Sprintf(
				`{"observation_id":%q,"kind":"sleep_episode","start_at":%q,"end_at":%q,"zone_id":"America/New_York","sleep":{"classification":"principal"},"provenance":{"acquisition_method":"synthetic","evidence_status":"directly_observed","recorded_at":%q,"source_record_id":%q}}`,
				id,
				sleepStart.UTC().Format(time.RFC3339),
				sleepStart.Add(duration).UTC().Format(time.RFC3339),
				sleepStart.Add(duration).Add(5*time.Minute).UTC().Format(time.RFC3339),
				fmt.Sprintf("mcp-source-%02d", i),
			)),
		})
	}
	return records
}

func assertNoForbiddenToolFields(t *testing.T, body []byte) {
	t.Helper()
	lower := strings.ToLower(string(body))
	for _, forbidden := range []string{
		`"payload"`,
		`"source_record_id"`,
		`"sourceids"`,
		`"observation_id"`,
		`"observationids"`,
		"obs_sleep_",
		"mcp-source",
		testDeviceToken,
		`"inputsessionids"`,
		`"algorithmversion"`,
		`"zoneid"`,
		`"utc"`,
		`"notes"`,
		`"diagnosis"`,
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("tool output leaked forbidden field/value %q in %s", forbidden, body)
		}
	}
}
