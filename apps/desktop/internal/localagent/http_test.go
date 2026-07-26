package localagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const testToken = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakeCapability struct {
	mu        sync.Mutex
	proposals bool
	calls     []string
}

func (f *fakeCapability) ProposalsAvailable(context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.proposals
}

func (f *fakeCapability) CallTool(_ context.Context, name string, arguments json.RawMessage) (json.RawMessage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.mu.Unlock()
	return json.Marshal(map[string]any{"tool": name, "arguments": json.RawMessage(arguments)})
}

func (f *fakeCapability) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestHandlerSecurityAndExactPath(t *testing.T) {
	handler, err := NewHandler(testToken, &fakeCapability{})
	if err != nil {
		t.Fatal(err)
	}

	request := mcpRequest(http.MethodPost, "/other", initializeBody())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("wrong path status = %d", response.Code)
	}

	request = mcpRequest(http.MethodPost, "/mcp", initializeBody())
	request.Header.Del("Authorization")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status = %d", response.Code)
	}

	request = mcpRequest(http.MethodPost, "/mcp", initializeBody())
	request.Header.Set("Origin", "https://attacker.example")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("origin status = %d", response.Code)
	}

	request = mcpRequest(http.MethodPost, "/mcp", initializeBody())
	request.RemoteAddr = "192.0.2.4:1234"
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-loopback status = %d", response.Code)
	}
}

func TestHandlerLifecycleToolsAndNoApplySurface(t *testing.T) {
	capability := &fakeCapability{}
	handler, err := NewHandler(testToken, capability)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := initializeOnlySession(t, handler)

	response := performMCP(t, handler, sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	var beforeInitialized struct {
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &beforeInitialized); err != nil {
		t.Fatal(err)
	}
	if beforeInitialized.Error == nil || beforeInitialized.Error.Code != -32002 {
		t.Fatalf("pre-initialized tools/list did not fail closed: %s", response.Body.String())
	}

	notification := mcpRequest(http.MethodPost, "/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	setSessionHeaders(notification, sessionID)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, notification)
	if response.Code != http.StatusAccepted || response.Body.Len() != 0 {
		t.Fatalf("notification response = %d %q", response.Code, response.Body.String())
	}

	response = performMCP(t, handler, sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d: %s", response.Code, response.Body.String())
	}
	var listed struct {
		Result struct {
			Tools []ToolDefinition `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, tool := range listed.Result.Tools {
		names[tool.Name] = true
	}
	for _, required := range []string{"get_status", "get_overview", "get_rhythm_summary", "list_tasks", "get_medication_timing", "list_rhythm_markers", "get_appearance", "set_appearance", "ask_zeitboard_facts"} {
		if !names[required] {
			t.Errorf("missing tool %q", required)
		}
	}
	for _, forbidden := range []string{"approve", "apply", "decide_proposal", "delete_sleep", "propose_place_task"} {
		if names[forbidden] {
			t.Errorf("forbidden or unavailable tool exposed: %q", forbidden)
		}
	}

	call := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_status","arguments":{}}}`
	response = performMCP(t, handler, sessionID, call)
	var called struct {
		Result ToolResult `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &called); err != nil {
		t.Fatal(err)
	}
	if called.Result.IsError || len(called.Result.StructuredContent) == 0 || capability.callCount() != 1 {
		t.Fatalf("unexpected tool result: %+v calls=%d", called.Result, capability.callCount())
	}

	missingVersion := mcpRequest(http.MethodPost, "/mcp", call)
	missingVersion.Header.Set(SessionIDHeader, sessionID)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, missingVersion)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing protocol status = %d", response.Code)
	}

	deleteRequest := mcpRequest(http.MethodDelete, "/mcp", "")
	setSessionHeaders(deleteRequest, sessionID)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, deleteRequest)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", response.Code)
	}
	response = performMCP(t, handler, sessionID, call)
	if response.Code != http.StatusNotFound {
		t.Fatalf("deleted session status = %d", response.Code)
	}
}

func TestHandlerNegotiatesVersionAndRejectsMalformedTransport(t *testing.T) {
	handler, err := NewHandler(testToken, &fakeCapability{})
	if err != nil {
		t.Fatal(err)
	}

	badAccept := mcpRequest(http.MethodPost, "/mcp", initializeBody())
	badAccept.Header.Set("Accept", "application/jsonish, text/event-streaming")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, badAccept)
	if response.Code != http.StatusNotAcceptable {
		t.Fatalf("substring media types were accepted: %d", response.Code)
	}

	disabledAccept := mcpRequest(http.MethodPost, "/mcp", initializeBody())
	disabledAccept.Header.Set("Accept", "application/json; q=0, text/event-stream")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, disabledAccept)
	if response.Code != http.StatusNotAcceptable {
		t.Fatalf("q=0 media type was accepted: %d", response.Code)
	}
	invalidQuality := mcpRequest(http.MethodPost, "/mcp", initializeBody())
	invalidQuality.Header.Set("Accept", "application/json; q=2, text/event-stream")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, invalidQuality)
	if response.Code != http.StatusNotAcceptable {
		t.Fatalf("out-of-range media quality was accepted: %d", response.Code)
	}

	negotiated := mcpRequest(http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, negotiated)
	if response.Code != http.StatusOK || response.Header().Get(SessionIDHeader) == "" || !strings.Contains(response.Body.String(), `"protocolVersion":"`+ProtocolVersion+`"`) {
		t.Fatalf("protocol negotiation failed: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}

	invalidID := mcpRequest(http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":{"nested":true},"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, invalidID)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":-32600`) || response.Header().Get(SessionIDHeader) != "" {
		t.Fatalf("invalid request id was not rejected: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}

	invalidParams := mcpRequest(http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":3,"method":"initialize","params":{"protocolVersion":"","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, invalidParams)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":-32602`) || response.Header().Get(SessionIDHeader) != "" {
		t.Fatalf("invalid initialize params were not rejected: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

func TestHandlerProposalVisibilityAndBudgets(t *testing.T) {
	capability := &fakeCapability{proposals: true}
	handler, err := NewHandler(testToken, capability)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := initializeSession(t, handler)

	for _, tool := range ToolDefinitions(true) {
		if !IsProposeTool(tool.Name) {
			continue
		}
		properties, ok := tool.InputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("proposal schema properties are malformed: %#v", tool.InputSchema)
		}
		if _, exposed := properties["answer"]; exposed {
			t.Fatalf("proposal tool %q lets callers inject assistant text", tool.Name)
		}
	}

	handler.mu.Lock()
	session := handler.sessions[sessionID]
	session.TotalRemaining = 5
	session.ProposeRemaining = 2
	handler.mu.Unlock()

	const calls = 12
	var wait sync.WaitGroup
	wait.Add(calls)
	for i := 0; i < calls; i++ {
		go func() {
			defer wait.Done()
			response := performMCP(t, handler, sessionID, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_status","arguments":{}}}`)
			if response.Code != http.StatusOK {
				t.Errorf("concurrent call status = %d", response.Code)
			}
		}()
	}
	wait.Wait()
	if capability.callCount() != 5 {
		t.Fatalf("capability calls = %d, want 5", capability.callCount())
	}

	secondSession := initializeSession(t, handler)
	proposalCall := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"propose_place_task","arguments":{"target":{"task_id":"task_1"}}}}`
	for i := 0; i < DefaultProposeBudget+1; i++ {
		response := performMCP(t, handler, secondSession, proposalCall)
		var envelope struct {
			Result ToolResult `json:"result"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if i < DefaultProposeBudget && envelope.Result.IsError {
			t.Fatalf("proposal %d failed early: %+v", i, envelope.Result)
		}
		if i == DefaultProposeBudget && !envelope.Result.IsError {
			t.Fatal("proposal budget did not fail closed")
		}
	}
}

func initializeSession(t *testing.T, handler *Handler) string {
	t.Helper()
	sessionID := initializeOnlySession(t, handler)
	notification := mcpRequest(http.MethodPost, "/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	setSessionHeaders(notification, sessionID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, notification)
	if response.Code != http.StatusAccepted {
		t.Fatalf("initialized notification status = %d: %s", response.Code, response.Body.String())
	}
	return sessionID
}

func initializeOnlySession(t *testing.T, handler *Handler) string {
	t.Helper()
	request := mcpRequest(http.MethodPost, "/mcp", initializeBody())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("initialize status = %d: %s", response.Code, response.Body.String())
	}
	sessionID := response.Header().Get(SessionIDHeader)
	if sessionID == "" {
		t.Fatal("initialize did not return a session id")
	}
	return sessionID
}

func initializeBody() string {
	return `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
}

func performMCP(t *testing.T, handler *Handler, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := mcpRequest(http.MethodPost, "/mcp", body)
	setSessionHeaders(request, sessionID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func mcpRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:48210"
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	return request
}

func setSessionHeaders(request *http.Request, sessionID string) {
	request.Header.Set(SessionIDHeader, sessionID)
	request.Header.Set(ProtocolVersionHeader, ProtocolVersion)
}
