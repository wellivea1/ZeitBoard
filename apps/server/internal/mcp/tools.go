package mcp

import (
	"encoding/json"
	"errors"
)

type toolDefinition struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func toolDefinitions() []toolDefinition {
	empty := emptySchema()
	propose := proposeSchema()
	return []toolDefinition{
		{Name: "get_status", Title: "Get Status", Description: "Read backend and assistant provider status.", InputSchema: empty},
		{Name: "get_overview", Title: "Get Overview", Description: "Read the current server-computed overview projection.", InputSchema: empty},
		{Name: "get_rhythm", Title: "Get Rhythm", Description: "Read the server-computed rhythm projection with actogram and screen-reader row data.", InputSchema: empty},
		{Name: "get_accuracy", Title: "Get Accuracy", Description: "Read the estimator backtest projection.", InputSchema: empty},
		{Name: "list_proposals", Title: "List Proposals", Description: "Read pending and decided proposal summaries.", InputSchema: empty},
		{Name: "propose_move_task", Title: "Propose Move Task", Description: "Create a pending move-task proposal. A human must approve before anything changes.", InputSchema: propose},
		{Name: "propose_place_task", Title: "Propose Place Task", Description: "Create a pending place-task proposal. A human must approve before anything changes.", InputSchema: propose},
		{Name: "propose_reminder_shift", Title: "Propose Reminder Shift", Description: "Create a pending reminder-shift proposal. A human must approve before anything changes.", InputSchema: propose},
	}
}

func knownTool(name string) bool {
	if _, ok := readToolPath(name); ok {
		return true
	}
	return isProposeTool(name)
}

func isProposeTool(name string) bool {
	switch name {
	case "propose_move_task", "propose_place_task", "propose_reminder_shift":
		return true
	default:
		return false
	}
}

func readToolPath(name string) (string, bool) {
	switch name {
	case "get_status":
		return "/v1/status", true
	case "get_overview":
		return "/v1/overview", true
	case "get_rhythm":
		return "/v1/rhythm", true
	case "get_accuracy":
		return "/v1/accuracy", true
	case "list_proposals":
		return "/v1/proposals", true
	default:
		return "", false
	}
}

func emptySchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
	}
}

func proposeSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"context", "target"},
		"properties": map[string]any{
			"context": planningContextSchema(),
			"target":  actionTargetSchema(),
			"answer":  map[string]any{"type": "string", "maxLength": 240},
		},
	}
}

func planningContextSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"zone_id", "now", "tasks", "availability"},
		"properties": map[string]any{
			"zone_id":     map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
			"now":         map[string]any{"type": "string", "format": "date-time"},
			"estimate_id": map[string]any{"type": "string", "maxLength": 64},
			"tasks": map[string]any{
				"type":  "array",
				"items": taskSchema(),
			},
			"availability": map[string]any{
				"type":  "array",
				"items": availabilitySchema(),
			},
			"fixed_events": map[string]any{
				"type":  "array",
				"items": fixedEventSchema(),
			},
		},
	}
}

func taskSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"task_id", "duration_minutes"},
		"properties": map[string]any{
			"task_id":                      map[string]any{"type": "string", "minLength": 1, "maxLength": 80},
			"duration_minutes":             map[string]any{"type": "integer", "minimum": 1, "maximum": 1440},
			"earliest_start_at":            map[string]any{"type": "string", "format": "date-time"},
			"latest_finish_at":             map[string]any{"type": "string", "format": "date-time"},
			"preferred_after_wake_minutes": map[string]any{"type": "integer", "minimum": 0, "maximum": 1440},
			"minimum_confidence":           map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"business_hours":               map[string]any{"type": "boolean"},
			"business_start_local":         map[string]any{"type": "string", "maxLength": 8},
			"business_end_local":           map[string]any{"type": "string", "maxLength": 8},
		},
	}
}

func availabilitySchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"kind", "start_at", "end_at", "zone_id", "confidence"},
		"properties": map[string]any{
			"kind":       map[string]any{"type": "string", "enum": []string{"predicted_wake", "predicted_sleep", "functional", "free"}},
			"start_at":   map[string]any{"type": "string", "format": "date-time"},
			"end_at":     map[string]any{"type": "string", "format": "date-time"},
			"zone_id":    map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
			"confidence": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
		},
	}
}

func fixedEventSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"event_id", "start_at", "end_at", "zone_id"},
		"properties": map[string]any{
			"event_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 80},
			"start_at": map[string]any{"type": "string", "format": "date-time"},
			"end_at":   map[string]any{"type": "string", "format": "date-time"},
			"zone_id":  map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
		},
	}
}

func actionTargetSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"task_id"},
		"properties": map[string]any{
			"task_id":                      map[string]any{"type": "string", "minLength": 1, "maxLength": 80},
			"earliest_start_at":            map[string]any{"type": "string", "format": "date-time"},
			"latest_finish_at":             map[string]any{"type": "string", "format": "date-time"},
			"duration_minutes":             map[string]any{"type": "integer", "minimum": 1, "maximum": 1440},
			"preferred_after_wake_minutes": map[string]any{"type": "integer", "minimum": 0, "maximum": 1440},
			"reminder_id":                  map[string]any{"type": "string", "maxLength": 80},
		},
	}
}

type proposeArguments struct {
	Context json.RawMessage `json:"context"`
	Target  json.RawMessage `json:"target"`
	Answer  string          `json:"answer,omitempty"`
}

func (a proposeArguments) directProposalPayload(action string) (json.RawMessage, error) {
	if len(a.Context) == 0 || len(a.Target) == 0 {
		return nil, errors.New("Propose tools require context and target arguments.")
	}
	payload := map[string]any{
		"schema_version":     "v1",
		"recommended_action": action,
		"context":            json.RawMessage(a.Context),
		"target":             json.RawMessage(a.Target),
	}
	if a.Answer != "" {
		payload["answer"] = a.Answer
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
