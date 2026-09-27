package localagent

import (
	"context"
	"encoding/json"
	"errors"

	"non24.app/core/agentactions"
)

const (
	ProtocolVersion      = "2025-11-25"
	DefaultTotalBudget   = 20
	DefaultProposeBudget = 5
)

type Capability interface {
	// TaskProposalsAvailable reports whether a server can take schedule
	// proposals: its scheduler resolves them. Proposals that wait on this
	// computer, such as a dose or a new task, are always available.
	TaskProposalsAvailable(context.Context) bool
	CallTool(context.Context, string, json.RawMessage) (json.RawMessage, error)
}

type ToolError struct {
	Message string
}

func (e *ToolError) Error() string { return e.Message }

func UserError(message string) error {
	return &ToolError{Message: message}
}

func safeToolError(err error) string {
	var toolErr *ToolError
	if errors.As(err, &toolErr) && toolErr.Message != "" {
		return toolErr.Message
	}
	return "ZeitBoard could not complete that tool call. No change was made."
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func ToolDefinitions(taskProposals bool) []ToolDefinition {
	tools := []ToolDefinition{
		{Name: "get_status", Title: "Get Status", Description: "Read desktop-local agent and data availability status.", InputSchema: emptySchema()},
		{Name: "get_snapshot", Title: "Get Snapshot", Description: "Start here. Read everything ZeitBoard may tell an assistant in one versioned document " +
			"(assistant-snapshot v1): whether the rhythm estimate can be trusted now and what it says, recent sleep, the next three days " +
			"(when sleep and waking are likely, reachable hours, commitments, suggested times, tasks that could not be placed), what awaits " +
			"the owner's decision, tasks, medication timing and doses waiting to be recorded, context markers and sync state. The other " +
			"read tools are narrower views of the " +
			"same data. Titles, labels, notes and raw records are never included.", InputSchema: emptySchema()},
		{Name: "get_overview", Title: "Get Overview", Description: "Read a speakable overview projection. Raw sleep records are never returned.", InputSchema: emptySchema()},
		{Name: "get_rhythm_summary", Title: "Get Rhythm Summary", Description: "Read predicted sleep-wake timing, drift, confidence, and refusal state without raw records.", InputSchema: emptySchema()},
		{Name: "list_tasks", Title: "List Tasks", Description: "Read allowlisted task planning fields. Private task titles and notes are omitted.", InputSchema: emptySchema()},
		{Name: "get_medication_timing", Title: "Get Medication Timing Facts", Description: "Read neutral schedule, collision, and aggregate logged-event facts using opaque medication ids. Labels, notes, strengths, clinician text, exact logged timestamps, and event rows are omitted.", InputSchema: emptySchema()},
		{Name: "list_rhythm_markers", Title: "List Rhythm Markers", Description: "Read marker kind and coarse civil-date ranges. Private notes and exact record timestamps are omitted.", InputSchema: emptySchema()},
		{Name: "get_appearance", Title: "Get Appearance", Description: "Read the current appearance preset, reduced-stimulation state, and rhythm-linked night rule.", InputSchema: emptySchema()},
		actionTool("set_appearance", appearanceSchema()),
		{Name: "ask_zeitboard_facts", Title: "Ask ZeitBoard Facts", Description: "Return allowlisted local facts for a question. Medical decisions are refused with the canonical ZeitBoard response.", InputSchema: questionSchema()},
	}
	for _, action := range agentactions.Offered(agentactions.LocalMCP, agentactions.Proposal) {
		// A proposal that waits on this computer is always offered; a
		// schedule change needs a server to resolve it.
		if action.WaitsOnDesktop() || taskProposals {
			tools = append(tools, actionTool(action.ID, proposalSchema(agentactions.TargetSchema(action.Subject))))
		}
	}
	return tools
}

// actionTool lists a registered action under the registry's title and
// description.
func actionTool(id string, input map[string]any) ToolDefinition {
	action, ok := agentactions.Lookup(id)
	if !ok || !action.On(agentactions.LocalMCP) {
		panic("localagent: " + id + " is not a registered local action")
	}
	return ToolDefinition{Name: action.ID, Title: action.Title, Description: action.Description, InputSchema: input}
}

func KnownTool(name string, taskProposals bool) bool {
	for _, tool := range ToolDefinitions(taskProposals) {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func IsProposeTool(name string) bool {
	return agentactions.IsProposal(name, agentactions.LocalMCP)
}

func emptySchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false}
}

func questionSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"message"},
		"properties": map[string]any{
			"message": map[string]any{"type": "string", "minLength": 1, "maxLength": 2000},
		},
	}
}

func appearanceSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"minProperties":        1,
		"properties": map[string]any{
			"theme":               map[string]any{"type": "string", "enum": []string{"auto", "light", "dark", "black", "amber", "contrast"}},
			"reduced_stimulation": map[string]any{"type": "boolean"},
			"night_rule": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"enabled", "preset", "lead_hours", "fallback_start_local", "fallback_end_local"},
				"properties": map[string]any{
					"enabled":              map[string]any{"type": "boolean"},
					"preset":               map[string]any{"type": "string", "enum": []string{"amber", "black", "dark"}},
					"lead_hours":           map[string]any{"type": "number", "minimum": 0, "maximum": 12},
					"fallback_start_local": map[string]any{"type": "string", "pattern": `^$|^(?:[01]\d|2[0-3]):[0-5]\d$`},
					"fallback_end_local":   map[string]any{"type": "string", "pattern": `^$|^(?:[01]\d|2[0-3]):[0-5]\d$`},
				},
			},
		},
	}
}

func proposalSchema(target map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"target"},
		"properties": map[string]any{
			"target": target,
		},
	}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r rpcRequest) idOrNull() json.RawMessage {
	if len(r.ID) == 0 {
		return json.RawMessage("null")
	}
	return r.ID
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ToolResult struct {
	Content           []textContent   `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func rpcResult(id json.RawMessage, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func rpcErrorResponse(id json.RawMessage, code int, message string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

func jsonResult(data json.RawMessage) ToolResult {
	return ToolResult{Content: []textContent{{Type: "text", Text: string(data)}}, StructuredContent: data}
}

func textError(message string) ToolResult {
	if message == "" {
		message = "Tool call failed."
	}
	return ToolResult{Content: []textContent{{Type: "text", Text: message}}, IsError: true}
}
