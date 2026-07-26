package localagent

import (
	"context"
	"encoding/json"
	"errors"
)

const (
	ProtocolVersion      = "2025-11-25"
	DefaultTotalBudget   = 20
	DefaultProposeBudget = 5
)

type Capability interface {
	ProposalsAvailable(context.Context) bool
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

func ToolDefinitions(proposalsAvailable bool) []ToolDefinition {
	tools := []ToolDefinition{
		{Name: "get_status", Title: "Get Status", Description: "Read desktop-local agent and data availability status.", InputSchema: emptySchema()},
		{Name: "get_overview", Title: "Get Overview", Description: "Read a speakable overview projection. Raw sleep records are never returned.", InputSchema: emptySchema()},
		{Name: "get_rhythm_summary", Title: "Get Rhythm Summary", Description: "Read predicted sleep-wake timing, drift, confidence, and refusal state without raw records.", InputSchema: emptySchema()},
		{Name: "list_tasks", Title: "List Tasks", Description: "Read allowlisted task planning fields. Private task titles and notes are omitted.", InputSchema: emptySchema()},
		{Name: "get_medication_timing", Title: "Get Medication Timing Facts", Description: "Read neutral schedule, collision, and aggregate logged-event facts using opaque medication ids. Labels, notes, strengths, clinician text, exact logged timestamps, and event rows are omitted.", InputSchema: emptySchema()},
		{Name: "list_rhythm_markers", Title: "List Rhythm Markers", Description: "Read marker kind and coarse civil-date ranges. Private notes and exact record timestamps are omitted.", InputSchema: emptySchema()},
		{Name: "get_appearance", Title: "Get Appearance", Description: "Read the current appearance preset, reduced-stimulation state, and rhythm-linked night rule.", InputSchema: emptySchema()},
		{Name: "set_appearance", Title: "Set Appearance", Description: "Directly set reversible local display state under ADR-0021. This does not change health or schedule data.", InputSchema: appearanceSchema()},
		{Name: "ask_zeitboard_facts", Title: "Ask ZeitBoard Facts", Description: "Return allowlisted local facts for a question. Medical decisions are refused with the canonical ZeitBoard response.", InputSchema: questionSchema()},
	}
	if proposalsAvailable {
		tools = append(tools,
			ToolDefinition{Name: "propose_move_task", Title: "Propose Move Task", Description: "Create a pending move-task proposal on the configured self-hosted backend. Human approval is required.", InputSchema: proposalSchema()},
			ToolDefinition{Name: "propose_place_task", Title: "Propose Place Task", Description: "Create a pending place-task proposal on the configured self-hosted backend. Human approval is required.", InputSchema: proposalSchema()},
			ToolDefinition{Name: "propose_reminder_shift", Title: "Propose Reminder Shift", Description: "Create a pending reminder-shift proposal on the configured self-hosted backend. Human approval is required.", InputSchema: proposalSchema()},
		)
	}
	return tools
}

func KnownTool(name string, proposalsAvailable bool) bool {
	for _, tool := range ToolDefinitions(proposalsAvailable) {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func IsProposeTool(name string) bool {
	switch name {
	case "propose_move_task", "propose_place_task", "propose_reminder_shift":
		return true
	default:
		return false
	}
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

func proposalSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"target"},
		"properties": map[string]any{
			"target": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"task_id"},
				"properties": map[string]any{
					"task_id":                      map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9_-]{2,79}$`},
					"earliest_start_at":            map[string]any{"type": "string", "format": "date-time"},
					"latest_finish_at":             map[string]any{"type": "string", "format": "date-time"},
					"duration_minutes":             map[string]any{"type": "integer", "minimum": 1, "maximum": 1440},
					"preferred_after_wake_minutes": map[string]any{"type": "integer", "minimum": 0, "maximum": 1440},
					"reminder_id":                  map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9_-]{2,79}$`},
				},
			},
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
