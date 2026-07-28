package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"non24.app/core/agentpolicy"
	"non24.app/desktop/internal/localagent"
)

type LocalAgentStatusDTO struct {
	SchemaVersion             string `json:"schemaVersion"`
	Mode                      string `json:"mode"`
	Running                   bool   `json:"running"`
	Endpoint                  string `json:"endpoint,omitempty"`
	Message                   string `json:"message"`
	BackendProposalsAvailable bool   `json:"backendProposalsAvailable"`
	LocalStoreAvailable       bool   `json:"localStoreAvailable"`
	AppearanceStatus          string `json:"appearanceStatus"`
}

type desktopLocalCapability struct {
	app *App
}

type localAgentQuestion struct {
	Message string `json:"message"`
}

type localAgentProposalTarget struct {
	TaskID                    string     `json:"task_id"`
	EarliestStartAt           *time.Time `json:"earliest_start_at,omitempty"`
	LatestFinishAt            *time.Time `json:"latest_finish_at,omitempty"`
	DurationMinutes           int        `json:"duration_minutes,omitempty"`
	PreferredAfterWakeMinutes *int       `json:"preferred_after_wake_minutes,omitempty"`
	ReminderID                string     `json:"reminder_id,omitempty"`
}

type localAgentProposalArguments struct {
	Target localAgentProposalTarget `json:"target"`
}

var localAgentIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,79}$`)

type localAgentProposalResult struct {
	SchemaVersion string                      `json:"schema_version"`
	Result        string                      `json:"result"`
	Action        string                      `json:"action"`
	Answer        string                      `json:"answer"`
	Proposals     []localAgentProposalSummary `json:"proposals"`
	Approval      string                      `json:"approval"`
}

type localAgentProposalSummary struct {
	ProposalID string `json:"proposal_id"`
	Status     string `json:"status"`
}

type localAgentFactsResult struct {
	SchemaVersion string         `json:"schema_version"`
	Result        string         `json:"result"`
	Answer        string         `json:"answer"`
	Facts         map[string]any `json:"facts"`
	Disclaimer    string         `json:"disclaimer"`
}

func (a *App) startLocalAgent(ctx context.Context) {
	a.localAgentMu.Lock()
	defer a.localAgentMu.Unlock()
	if a.localAgent != nil {
		return
	}
	dir := a.configDir
	if dir == "" {
		var err error
		dir, err = desktopDataDir()
		if err != nil {
			a.localAgentErr = "The desktop-local agent could not determine its configuration directory."
			return
		}
		a.configDir = dir
	}
	endpoint, err := localagent.Start(ctx, dir, desktopLocalCapability{app: a})
	if err != nil {
		if strings.Contains(err.Error(), "another ZeitBoard") {
			a.localAgentErr = err.Error()
		} else {
			a.localAgentErr = "The desktop-local agent could not start. Restart ZeitBoard and try again."
		}
		return
	}
	a.localAgent = endpoint
	a.localAgentErr = ""
}

func (a *App) stopLocalAgent(_ context.Context) {
	a.localAgentMu.RLock()
	endpoint := a.localAgent
	a.localAgentMu.RUnlock()
	if endpoint == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	closeErr := endpoint.Close(shutdownCtx)
	a.localAgentMu.Lock()
	if a.localAgent == endpoint {
		a.localAgent = nil
		if closeErr != nil {
			a.localAgentErr = "The desktop-local agent did not shut down cleanly."
		}
	}
	a.localAgentMu.Unlock()
}

func (a *App) GetLocalAgentStatus() LocalAgentStatusDTO {
	a.localAgentMu.RLock()
	endpoint := a.localAgent
	startupErr := a.localAgentErr
	a.localAgentMu.RUnlock()
	status := LocalAgentStatusDTO{
		SchemaVersion:             "v1",
		Mode:                      "desktop_local",
		BackendProposalsAvailable: desktopLocalCapability{app: a}.ProposalsAvailable(context.Background()),
		AppearanceStatus:          "ready",
	}
	if _, err := a.requireStore(); err == nil {
		status.LocalStoreAvailable = true
	}
	a.appearanceMu.RLock()
	if a.appearanceErr != "" {
		status.AppearanceStatus = "error"
	}
	a.appearanceMu.RUnlock()
	if endpoint == nil {
		status.Message = startupErr
		if status.Message == "" {
			status.Message = "The desktop-local agent endpoint is not running."
		}
		return status
	}
	endpointStatus := endpoint.Status()
	status.Running = endpointStatus.Running
	status.Endpoint = endpointStatus.Endpoint
	status.Message = endpointStatus.Message
	return status
}

func (c desktopLocalCapability) ProposalsAvailable(context.Context) bool {
	if c.app == nil {
		return false
	}
	_, _, err := c.app.requireBackendSync()
	return err == nil
}

func (c desktopLocalCapability) CallTool(ctx context.Context, name string, arguments json.RawMessage) (json.RawMessage, error) {
	if c.app == nil {
		return nil, errors.New("local capability is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var value any
	var err error
	switch name {
	case "get_status":
		err = requireEmptyToolArguments(arguments)
		value = c.app.agentStatusProjection()
	case "get_overview":
		err = requireEmptyToolArguments(arguments)
		if err == nil {
			value, err = c.app.agentOverviewProjection(ctx)
		}
	case "get_rhythm_summary":
		err = requireEmptyToolArguments(arguments)
		if err == nil {
			value, err = c.app.agentRhythmProjection(ctx)
		}
	case "list_tasks":
		err = requireEmptyToolArguments(arguments)
		if err == nil {
			value, err = c.app.agentTaskProjection(ctx)
		}
	case "get_medication_timing":
		err = requireEmptyToolArguments(arguments)
		if err == nil {
			value, err = c.app.agentMedicationProjection(ctx)
		}
	case "list_rhythm_markers":
		err = requireEmptyToolArguments(arguments)
		if err == nil {
			value, err = c.app.agentMarkerProjection(ctx)
		}
	case "get_appearance":
		err = requireEmptyToolArguments(arguments)
		value = projectAgentAppearance(c.app.currentAppearance())
	case "set_appearance":
		value, err = c.app.applyAppearanceTool(arguments)
	case "ask_zeitboard_facts":
		value, err = c.app.answerLocalFacts(ctx, arguments)
	case "propose_move_task", "propose_place_task", "propose_reminder_shift":
		value, err = c.app.createLocalAgentProposal(ctx, name, arguments)
	default:
		return nil, localagent.UserError("Unknown ZeitBoard tool.")
	}
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("encode local projection")
	}
	return encoded, nil
}

func requireEmptyToolArguments(arguments json.RawMessage) error {
	if len(bytes.TrimSpace(arguments)) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	var empty struct{}
	if err := decodeStrictJSON(arguments, &empty); err != nil {
		return localagent.UserError("This read tool does not accept arguments. No data was returned.")
	}
	return nil
}

func (a *App) answerLocalFacts(ctx context.Context, arguments json.RawMessage) (localAgentFactsResult, error) {
	var input localAgentQuestion
	if err := decodeStrictJSON(arguments, &input); err != nil {
		return localAgentFactsResult{}, localagent.UserError("A question of 1 to 2000 characters is required.")
	}
	message := strings.TrimSpace(input.Message)
	if message == "" || utf8.RuneCountInString(message) > 2000 {
		return localAgentFactsResult{}, localagent.UserError("A question of 1 to 2000 characters is required.")
	}
	if agentpolicy.IsMedicalDecisionPrompt(message) {
		return localAgentFactsResult{
			SchemaVersion: "v1",
			Result:        "refused_medical",
			Answer:        agentpolicy.MedicalRefusal,
			Facts:         map[string]any{},
			Disclaimer:    disclaimer,
		}, nil
	}

	lower := strings.ToLower(message)
	facts := make(map[string]any)
	if agentpolicy.ContainsMedicationFactSubject(message) {
		projection, err := a.agentMedicationProjection(ctx)
		if err != nil {
			return localAgentFactsResult{}, err
		}
		facts["medication_timing"] = projection
	}
	if agentpolicy.ContainsMarkerSubject(message) {
		projection, err := a.agentMarkerProjection(ctx)
		if err != nil {
			return localAgentFactsResult{}, err
		}
		facts["rhythm_markers"] = projection
	}
	if containsAny(lower, "task", "paperwork", "appointment", "window") {
		projection, err := a.agentTaskProjection(ctx)
		if err != nil {
			return localAgentFactsResult{}, err
		}
		facts["tasks"] = projection
	}
	if containsAny(lower, "appearance", "theme", "preset", "night mode", "reduced stimulation") {
		facts["appearance"] = projectAgentAppearance(a.currentAppearance())
	}
	if containsAny(lower, "rhythm", "sleep", "wake", "drift", "confidence") {
		overview, err := a.agentOverviewProjection(ctx)
		if err != nil {
			return localAgentFactsResult{}, err
		}
		rhythm, err := a.agentRhythmProjection(ctx)
		if err != nil {
			return localAgentFactsResult{}, err
		}
		facts["overview"] = overview
		facts["rhythm"] = rhythm
	}
	if len(facts) == 0 {
		overview, err := a.agentOverviewProjection(ctx)
		if err != nil {
			return localAgentFactsResult{}, err
		}
		facts["overview"] = overview
	}
	return localAgentFactsResult{
		SchemaVersion: "v1",
		Result:        "facts",
		Answer:        "ZeitBoard returned the requested allowlisted local facts. Timing descriptions are factual and not medical advice.",
		Facts:         facts,
		Disclaimer:    disclaimer,
	}, nil
}

func (a *App) createLocalAgentProposal(ctx context.Context, action string, arguments json.RawMessage) (localAgentProposalResult, error) {
	var input localAgentProposalArguments
	if err := decodeStrictJSON(arguments, &input); err != nil {
		return localAgentProposalResult{}, localagent.UserError("Proposal arguments are invalid. No proposal was created.")
	}
	input.Target.TaskID = strings.TrimSpace(input.Target.TaskID)
	input.Target.ReminderID = strings.TrimSpace(input.Target.ReminderID)
	if !localAgentIdentifierPattern.MatchString(input.Target.TaskID) || (input.Target.ReminderID != "" && !localAgentIdentifierPattern.MatchString(input.Target.ReminderID)) {
		return localAgentProposalResult{}, localagent.UserError("Proposal target fields are invalid. No proposal was created.")
	}
	if input.Target.DurationMinutes < 0 || input.Target.DurationMinutes > 1440 || (input.Target.PreferredAfterWakeMinutes != nil && (*input.Target.PreferredAfterWakeMinutes < 0 || *input.Target.PreferredAfterWakeMinutes > 1440)) {
		return localAgentProposalResult{}, localagent.UserError("Proposal timing fields are outside the allowed range. No proposal was created.")
	}
	if input.Target.EarliestStartAt != nil && input.Target.LatestFinishAt != nil && !input.Target.EarliestStartAt.Before(*input.Target.LatestFinishAt) {
		return localAgentProposalResult{}, localagent.UserError("The proposal finish must be after its start. No proposal was created.")
	}
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return localAgentProposalResult{}, localagent.UserError("Proposal tools need an enabled, enrolled self-hosted backend. No proposal was created.")
	}
	planning, err := a.assistantPlanningContext(ctx, assistantFactScope{})
	if err != nil {
		return localAgentProposalResult{}, err
	}
	request := struct {
		SchemaVersion     string                   `json:"schema_version"`
		RecommendedAction string                   `json:"recommended_action"`
		Target            localAgentProposalTarget `json:"target"`
		Context           assistantContextPayload  `json:"context"`
	}{"v1", action, input.Target, planning}
	var response assistantMessageResponse
	if err := a.newDesktopBackendClient(cfg, token).postJSON(ctx, "/v1/proposals", request, &response); err != nil {
		a.recordBackendSyncError(cfg, err)
		return localAgentProposalResult{}, errors.New("create backend proposal")
	}
	result := localAgentProposalResult{
		SchemaVersion: "v1",
		Result:        "no_action",
		Action:        action,
		Answer:        "No schedule proposal was created.",
		Proposals:     []localAgentProposalSummary{},
		Approval:      "Pending proposals require a human decision in ZeitBoard; this tool cannot approve or apply them.",
	}
	for _, proposal := range response.Proposals {
		if len(result.Proposals) >= 8 || !localAgentIdentifierPattern.MatchString(proposal.ProposalID) {
			continue
		}
		result.Proposals = append(result.Proposals, localAgentProposalSummary{ProposalID: proposal.ProposalID, Status: "pending"})
	}
	if len(result.Proposals) > 0 {
		result.Result = "proposed"
		result.Answer = "A pending schedule proposal was created for human review."
	}
	return result, nil
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func localAgentProjectionError(kind string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("build %s projection: %w", kind, err)
}
