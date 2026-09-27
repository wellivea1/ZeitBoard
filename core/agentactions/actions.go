// Package agentactions is the one registry of what an agent may ask ZeitBoard
// to do (completion plan C4). The chat assistant's action schema and its
// validation, the server's and the desktop's MCP tool lists and dispatch, and
// the proposal cards all read it, so an action exists on every surface that
// offers it or on none. A test holds the contracts' action enums to it.
package agentactions

import (
	"errors"
	"regexp"
	"time"
)

// Version names the registry. Adding, removing or reshaping an action is a new
// version, and the contracts that list actions change with it.
const Version = "v1"

// Kind says what using an action does.
type Kind string

const (
	// Proposal creates a pending proposal. Nothing changes until a human
	// approves it, and no agent can approve its own proposal (ADR-0012).
	Proposal Kind = "proposal"
	// Direct changes local, reversible, non-health state at once (ADR-0021).
	Direct Kind = "direct"
)

// Surface is a place an action is offered.
type Surface uint8

const (
	// ChatAssistant is the chat assistant's model, which may recommend it.
	ChatAssistant Surface = 1 << iota
	// ServerMCP is the self-hosted server's MCP connector (ADR-0012).
	ServerMCP
	// LocalMCP is the desktop's loopback MCP endpoint (ADR-0028).
	LocalMCP

	// AnyMCP is either MCP endpoint. Both relay a proposal to the server's
	// direct proposal endpoint, which cannot tell them apart.
	AnyMCP = ServerMCP | LocalMCP
)

// Action is one registered action.
type Action struct {
	ID       string
	Kind     Kind
	Surfaces Surface
	// Title and Description are what a tool list shows.
	Title       string
	Description string
	// CardTitle names a pending proposal of this action on a card, before the
	// task it concerns; empty for a direct action.
	CardTitle string
}

// On reports whether the action is offered on a surface, or on any of a set.
func (a Action) On(surfaces Surface) bool { return a.Surfaces&surfaces != 0 }

const approvalRequired = " A human must approve it before anything changes."

var registry = []Action{
	{
		ID: "propose_move_task", Kind: Proposal, Surfaces: ChatAssistant | ServerMCP | LocalMCP,
		Title: "Propose Move Task", Description: "Create a pending proposal to move a task." + approvalRequired,
		CardTitle: "Move task",
	},
	{
		ID: "propose_place_task", Kind: Proposal, Surfaces: ChatAssistant | ServerMCP | LocalMCP,
		Title: "Propose Place Task", Description: "Create a pending proposal to place a task." + approvalRequired,
		CardTitle: "Place task",
	},
	{
		ID: "propose_reminder_shift", Kind: Proposal, Surfaces: ChatAssistant | ServerMCP | LocalMCP,
		Title: "Propose Reminder Shift", Description: "Create a pending proposal to shift a reminder." + approvalRequired,
		CardTitle: "Shift reminder",
	},
	{
		ID: "set_appearance", Kind: Direct, Surfaces: LocalMCP,
		Title:       "Set Appearance",
		Description: "Directly set reversible local display state under ADR-0021. This does not change health or schedule data.",
	},
}

// All returns every registered action, in registry order.
func All() []Action { return append([]Action(nil), registry...) }

// Lookup finds an action by id.
func Lookup(id string) (Action, bool) {
	for _, action := range registry {
		if action.ID == id {
			return action, true
		}
	}
	return Action{}, false
}

// Offered returns the actions of a kind a surface offers, in registry order.
func Offered(surface Surface, kind Kind) []Action {
	var offered []Action
	for _, action := range registry {
		if action.On(surface) && action.Kind == kind {
			offered = append(offered, action)
		}
	}
	return offered
}

// IDs returns the ids of Offered(surface, kind).
func IDs(surface Surface, kind Kind) []string {
	var ids []string
	for _, action := range Offered(surface, kind) {
		ids = append(ids, action.ID)
	}
	return ids
}

// IsProposal reports whether id names a registered proposal offered on a
// surface, or on any of a set.
func IsProposal(id string, surfaces Surface) bool {
	action, ok := Lookup(id)
	return ok && action.Kind == Proposal && action.On(surfaces)
}

// CardTitle names a pending proposal on a card: "Place task" before the task
// it concerns, or "Schedule change" for an action the registry no longer has.
func CardTitle(id string) string {
	if action, ok := Lookup(id); ok && action.CardTitle != "" {
		return action.CardTitle
	}
	return "Schedule change"
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,79}$`)

// Target is what every task proposal names: the task, optionally its bounds,
// duration, after-wake preference and the reminder it concerns.
type Target struct {
	TaskID                    string     `json:"task_id"`
	EarliestStartAt           *time.Time `json:"earliest_start_at,omitempty"`
	LatestFinishAt            *time.Time `json:"latest_finish_at,omitempty"`
	DurationMinutes           int        `json:"duration_minutes,omitempty"`
	PreferredAfterWakeMinutes *int       `json:"preferred_after_wake_minutes,omitempty"`
	ReminderID                string     `json:"reminder_id,omitempty"`
}

// Validate applies the target's rules, the same wherever a proposal arrives.
// Its errors complete the sentence "The proposal target is invalid: ...".
func (t Target) Validate() error {
	switch {
	case !identifier.MatchString(t.TaskID):
		return errors.New("the task id is missing or invalid")
	case t.ReminderID != "" && !identifier.MatchString(t.ReminderID):
		return errors.New("the reminder id is invalid")
	case t.DurationMinutes < 0 || t.DurationMinutes > 1440:
		// Zero is "not given": the task keeps its own duration.
		return errors.New("the duration must be 1 to 1440 minutes")
	case t.PreferredAfterWakeMinutes != nil && (*t.PreferredAfterWakeMinutes < 0 || *t.PreferredAfterWakeMinutes > 1440):
		return errors.New("the wake offset must be 0 to 1440 minutes")
	case (t.EarliestStartAt != nil && t.EarliestStartAt.IsZero()) || (t.LatestFinishAt != nil && t.LatestFinishAt.IsZero()):
		return errors.New("a timing bound is empty")
	case t.EarliestStartAt != nil && t.LatestFinishAt != nil && !t.EarliestStartAt.Before(*t.LatestFinishAt):
		return errors.New("the finish must be after the start")
	}
	return nil
}

// TargetSchema is the JSON Schema of a Target, for tool input schemas.
func TargetSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"task_id"},
		"properties": map[string]any{
			"task_id":                      map[string]any{"type": "string", "pattern": identifier.String()},
			"earliest_start_at":            map[string]any{"type": "string", "format": "date-time"},
			"latest_finish_at":             map[string]any{"type": "string", "format": "date-time"},
			"duration_minutes":             map[string]any{"type": "integer", "minimum": 1, "maximum": 1440},
			"preferred_after_wake_minutes": map[string]any{"type": "integer", "minimum": 0, "maximum": 1440},
			"reminder_id":                  map[string]any{"type": "string", "pattern": identifier.String()},
		},
	}
}
