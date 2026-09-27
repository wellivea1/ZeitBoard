// Package agentactions is the one registry of what an agent may ask ZeitBoard
// to do (completion plan C4, ADR-0050). The chat assistant's action schema and
// its validation, the server's and the desktop's MCP tool lists and dispatch,
// and the proposal cards all read it, so an action exists on every surface
// that offers it or on none. A test holds the contracts' action enums to it.
package agentactions

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
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

// Subject is what a proposal is about. It decides what the proposal names and
// where it waits for the owner.
type Subject string

const (
	// ScheduleSubject proposals change when an existing task happens. They
	// name a TaskTarget, and the server's scheduler resolves them into a
	// pending proposal wherever they arrive.
	ScheduleSubject Subject = "schedule"
	// DoseSubject proposals name a DoseTarget: a dose is a health record, so
	// only the owner makes one.
	DoseSubject Subject = "dose"
	// NewTaskSubject proposals name a NewTaskTarget, in the owner's words.
	NewTaskSubject Subject = "new_task"
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

	// AnyMCP is either MCP endpoint. Both relay a task proposal to the
	// server's direct proposal endpoint, which cannot tell them apart.
	AnyMCP = ServerMCP | LocalMCP
)

// Action is one registered action.
type Action struct {
	ID       string
	Kind     Kind
	Subject  Subject // proposals only
	Surfaces Surface
	// Title and Description are what a tool list shows.
	Title       string
	Description string
	// CardTitle names a pending proposal of this action on a card; empty for
	// a direct action.
	CardTitle string
}

// On reports whether the action is offered on a surface, or on any of a set.
func (a Action) On(surfaces Surface) bool { return a.Surfaces&surfaces != 0 }

// WaitsOnDesktop reports whether a proposal waits in the owner's computer's
// own queue until the owner accepts or declines it (ADR-0051), rather than
// being resolved by the server.
func (a Action) WaitsOnDesktop() bool { return a.Kind == Proposal && a.Subject != ScheduleSubject }

const approvalRequired = " A human must approve it before anything changes."

var registry = []Action{
	{
		ID: "propose_move_task", Kind: Proposal, Subject: ScheduleSubject, Surfaces: ChatAssistant | ServerMCP | LocalMCP,
		Title: "Propose Move Task", Description: "Create a pending proposal to move a task." + approvalRequired,
		CardTitle: "Move task",
	},
	{
		ID: "propose_place_task", Kind: Proposal, Subject: ScheduleSubject, Surfaces: ChatAssistant | ServerMCP | LocalMCP,
		Title: "Propose Place Task", Description: "Create a pending proposal to place a task." + approvalRequired,
		CardTitle: "Place task",
	},
	{
		ID: "propose_reminder_shift", Kind: Proposal, Subject: ScheduleSubject, Surfaces: ChatAssistant | ServerMCP | LocalMCP,
		Title: "Propose Reminder Shift", Description: "Create a pending proposal to shift a reminder." + approvalRequired,
		CardTitle: "Shift reminder",
	},
	{
		ID: "propose_log_dose", Kind: Proposal, Subject: DoseSubject, Surfaces: LocalMCP,
		Title: "Propose Log Dose",
		Description: "Create a pending dose for the owner to record: a medication by its opaque id from get_snapshot " +
			"or get_medication_timing, taken or skipped, and when, if not now. Nothing is recorded until the owner " +
			"records it on their computer, and it lapses after a day." + approvalRequired,
		CardTitle: "Record dose",
	},
	{
		ID: "propose_add_task", Kind: Proposal, Subject: NewTaskSubject, Surfaces: LocalMCP,
		Title: "Propose Add Task",
		Description: "Create a pending new task for the owner to add: a short title in the owner's own words, how many " +
			"minutes it takes, and optionally the earliest start and latest finish. Once added, ZeitBoard suggests a time " +
			"for it as for any task. It lapses after a day." + approvalRequired,
		CardTitle: "Add task",
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

// ProposalOn finds a registered proposal offered on a surface, or on any of a
// set.
func ProposalOn(id string, surfaces Surface) (Action, bool) {
	action, ok := Lookup(id)
	if !ok || action.Kind != Proposal || !action.On(surfaces) {
		return Action{}, false
	}
	return action, true
}

// IsProposal reports whether id names a registered proposal offered on a
// surface, or on any of a set.
func IsProposal(id string, surfaces Surface) bool {
	_, ok := ProposalOn(id, surfaces)
	return ok
}

// IsScheduleProposal reports whether id names a registered schedule proposal
// offered on a surface, or on any of a set: one the server's scheduler
// resolves.
func IsScheduleProposal(id string, surfaces Surface) bool {
	action, ok := ProposalOn(id, surfaces)
	return ok && action.Subject == ScheduleSubject
}

// TargetSchema is the JSON Schema of what a proposal about subject names, for
// tool input schemas.
func TargetSchema(subject Subject) map[string]any {
	switch subject {
	case DoseSubject:
		return DoseTargetSchema()
	case NewTaskSubject:
		return NewTaskTargetSchema()
	default:
		return TaskTargetSchema()
	}
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

// TaskTarget is what every schedule proposal names: the task, optionally its
// bounds, duration, after-wake preference and the reminder it concerns.
type TaskTarget struct {
	TaskID                    string     `json:"task_id"`
	EarliestStartAt           *time.Time `json:"earliest_start_at,omitempty"`
	LatestFinishAt            *time.Time `json:"latest_finish_at,omitempty"`
	DurationMinutes           int        `json:"duration_minutes,omitempty"`
	PreferredAfterWakeMinutes *int       `json:"preferred_after_wake_minutes,omitempty"`
	ReminderID                string     `json:"reminder_id,omitempty"`
}

// Validate applies the target's rules, the same wherever a proposal arrives.
// Its errors complete the sentence "The proposal target is invalid: ...".
func (t TaskTarget) Validate() error {
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

// TaskTargetSchema is the JSON Schema of a TaskTarget, for tool input schemas.
func TaskTargetSchema() map[string]any {
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

// Dose proposal limits. A proposed dose may be up to a week old, since an owner
// may catch up on a missed entry, but not in the future beyond clock skew, the
// same allowance as a dose logged by hand.
const (
	MaxProposedDoseAge = 7 * 24 * time.Hour
	DoseClockSkew      = 5 * time.Minute
)

// DoseTarget is what a dose proposal names: a medication by its opaque id,
// whether the dose was taken or skipped, and when, if not now. Never a label
// or a note: those stay on the owner's computer.
type DoseTarget struct {
	MedicationID string     `json:"medication_id"`
	Status       string     `json:"status"`
	DoseAt       *time.Time `json:"dose_at,omitempty"`
}

// Validate applies the target's rules at now. Its errors complete the sentence
// "The proposal target is invalid: ...".
func (t DoseTarget) Validate(now time.Time) error {
	switch {
	case !identifier.MatchString(t.MedicationID):
		return errors.New("the medication id is missing or invalid")
	case t.Status != "taken" && t.Status != "skipped":
		return errors.New("the status must be taken or skipped")
	case t.DoseAt == nil:
		return nil
	case t.DoseAt.IsZero():
		return errors.New("the dose time is empty")
	case t.DoseAt.After(now.Add(DoseClockSkew)):
		return errors.New("the dose time is in the future")
	case t.DoseAt.Before(now.Add(-MaxProposedDoseAge)):
		return errors.New("the dose time is more than a week ago")
	}
	return nil
}

// DoseTargetSchema is the JSON Schema of a DoseTarget, for tool input schemas.
func DoseTargetSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"medication_id", "status"},
		"properties": map[string]any{
			"medication_id": map[string]any{"type": "string", "pattern": identifier.String()},
			"status":        map[string]any{"type": "string", "enum": []string{"taken", "skipped"}},
			"dose_at": map[string]any{
				"type": "string", "format": "date-time",
				"description": "When the dose was taken or skipped; now if omitted. At most a week ago.",
			},
		},
	}
}

// NewTaskTarget is what a task-adding proposal names. The title is the owner's
// own words, which the agent heard; it is stored on the owner's computer and
// never read back to an agent. The limits are a task's own.
type NewTaskTarget struct {
	Title           string     `json:"title"`
	DurationMinutes int        `json:"duration_minutes"`
	EarliestStartAt *time.Time `json:"earliest_start_at,omitempty"`
	LatestFinishAt  *time.Time `json:"latest_finish_at,omitempty"`
}

// Validate applies the target's rules at now. Its errors complete the sentence
// "The proposal target is invalid: ...".
func (t NewTaskTarget) Validate(now time.Time) error {
	title := strings.TrimSpace(t.Title)
	switch {
	case title == "" || len(title) > 120:
		return errors.New("the title must be 1 to 120 characters")
	case strings.IndexFunc(title, unicode.IsControl) >= 0:
		return errors.New("the title must be one line of text")
	case t.DurationMinutes < 5 || t.DurationMinutes > 720:
		return errors.New("the duration must be 5 to 720 minutes")
	case (t.EarliestStartAt != nil && t.EarliestStartAt.IsZero()) || (t.LatestFinishAt != nil && t.LatestFinishAt.IsZero()):
		return errors.New("a timing bound is empty")
	case t.LatestFinishAt != nil && !t.LatestFinishAt.After(now):
		return errors.New("the latest finish has passed")
	case t.EarliestStartAt != nil && t.LatestFinishAt != nil &&
		t.LatestFinishAt.Sub(*t.EarliestStartAt) < time.Duration(t.DurationMinutes)*time.Minute:
		return errors.New("the task does not fit between its earliest start and latest finish")
	}
	return nil
}

// NewTaskTargetSchema is the JSON Schema of a NewTaskTarget, for tool input
// schemas.
func NewTaskTargetSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"title", "duration_minutes"},
		"properties": map[string]any{
			"title":             map[string]any{"type": "string", "minLength": 1, "maxLength": 120},
			"duration_minutes":  map[string]any{"type": "integer", "minimum": 5, "maximum": 720},
			"earliest_start_at": map[string]any{"type": "string", "format": "date-time"},
			"latest_finish_at":  map[string]any{"type": "string", "format": "date-time"},
		},
	}
}
