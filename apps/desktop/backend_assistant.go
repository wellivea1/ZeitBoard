package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	storage "non24.app/core/storage/sqlite"
)

// Assistant bindings (roadmap slice 8): the desktop chat surface over the M2
// propose-only endpoints. The desktop builds a REDACTED planning context —
// task ids, durations, and bounds only, never titles or any sleep record —
// and the server resolves model output into pending proposals (ADR-0010).
// There is no apply path here: deciding a proposal goes through the same
// one-use-token queue endpoint as everywhere else.

type AssistantStatusDTO struct {
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Message    string `json:"message,omitempty"`
}

type AssistantMessageInput struct {
	Message string `json:"message"`
}

type AssistantReplyDTO struct {
	Available  bool                 `json:"available"`
	Result     string               `json:"result"`
	Answer     string               `json:"answer"`
	Configured bool                 `json:"configured"`
	Provider   string               `json:"provider,omitempty"`
	Model      string               `json:"model,omitempty"`
	Proposals  []BackendProposalDTO `json:"proposals"`
}

type assistantStatusResponse struct {
	SchemaVersion string `json:"schema_version"`
	Assistant     struct {
		Configured bool   `json:"configured"`
		Provider   string `json:"provider"`
		Model      string `json:"model,omitempty"`
	} `json:"assistant"`
}

type assistantMessageRequest struct {
	SchemaVersion string                  `json:"schema_version"`
	Message       string                  `json:"message"`
	Context       assistantContextPayload `json:"context"`
}

type assistantContextPayload struct {
	ZoneID       string                       `json:"zone_id"`
	Now          time.Time                    `json:"now"`
	EstimateID   string                       `json:"estimate_id,omitempty"`
	Tasks        []assistantTaskContext       `json:"tasks,omitempty"`
	Availability []assistantAvailabilityEntry `json:"availability,omitempty"`
}

type assistantTaskContext struct {
	TaskID                    string     `json:"task_id"`
	DurationMinutes           int        `json:"duration_minutes"`
	EarliestStartAt           *time.Time `json:"earliest_start_at,omitempty"`
	LatestFinishAt            *time.Time `json:"latest_finish_at,omitempty"`
	PreferredAfterWakeMinutes *int       `json:"preferred_after_wake_minutes,omitempty"`
	MinimumConfidence         string     `json:"minimum_confidence,omitempty"`
}

type assistantAvailabilityEntry struct {
	Kind       string    `json:"kind"`
	StartAt    time.Time `json:"start_at"`
	EndAt      time.Time `json:"end_at"`
	ZoneID     string    `json:"zone_id"`
	Confidence string    `json:"confidence"`
}

type assistantMessageResponse struct {
	SchemaVersion string `json:"schema_version"`
	Backend       struct {
		Configured bool   `json:"configured"`
		Provider   string `json:"provider"`
		Model      string `json:"model,omitempty"`
	} `json:"backend"`
	Result    string                     `json:"result"`
	Action    string                     `json:"action"`
	Answer    string                     `json:"answer"`
	Proposals []assistantProposalSummary `json:"proposals"`
}

type assistantProposalSummary struct {
	ProposalID    string          `json:"proposalId"`
	Status        string          `json:"status"`
	DecisionToken string          `json:"decisionToken,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

const assistantOffMessage = "The assistant runs on your self-hosted backend. Connect it under Settings → Backend sync to start chatting."

// GetAssistantStatus reports whether the assistant surface is usable and which
// provider the backend discloses. It makes no network call when sync is off.
func (a *App) GetAssistantStatus() (AssistantStatusDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		if !cfg.Enabled {
			return AssistantStatusDTO{Enabled: false, Message: assistantOffMessage}, nil
		}
		return AssistantStatusDTO{Enabled: false, Message: sanitizeBackendError(err)}, nil
	}
	client := newDesktopBackendClient(cfg, token)
	var response assistantStatusResponse
	if err := client.getJSON(context.Background(), "/v1/status", &response); err != nil {
		return AssistantStatusDTO{Enabled: false, Message: sanitizeBackendError(err)}, nil
	}
	return AssistantStatusDTO{
		Enabled:    true,
		Configured: response.Assistant.Configured,
		Provider:   response.Assistant.Provider,
		Model:      response.Assistant.Model,
	}, nil
}

// SendAssistantMessage forwards one chat message with redacted local context
// and maps the propose-only response for the rail.
func (a *App) SendAssistantMessage(input AssistantMessageInput) (AssistantReplyDTO, error) {
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return AssistantReplyDTO{}, errors.New("message is required")
	}
	if len(message) > 2000 {
		return AssistantReplyDTO{}, errors.New("message is too long (2000 characters max)")
	}
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		if !cfg.Enabled {
			return AssistantReplyDTO{Available: false, Result: "unavailable", Answer: assistantOffMessage, Proposals: []BackendProposalDTO{}}, nil
		}
		return AssistantReplyDTO{Available: false, Result: "unavailable", Answer: sanitizeBackendError(err), Proposals: []BackendProposalDTO{}}, nil
	}

	ctx := context.Background()
	planning, err := a.assistantPlanningContext(ctx)
	if err != nil {
		return AssistantReplyDTO{}, err
	}
	client := newDesktopBackendClient(cfg, token)
	var response assistantMessageResponse
	request := assistantMessageRequest{SchemaVersion: "v1", Message: message, Context: planning}
	if err := client.postJSON(ctx, "/v1/assistant/message", request, &response); err != nil {
		a.recordBackendSyncError(cfg, err)
		return AssistantReplyDTO{Available: false, Result: "unavailable", Answer: sanitizeBackendError(err), Proposals: []BackendProposalDTO{}}, nil
	}

	titles, _ := a.localTaskTitles(ctx)
	reply := AssistantReplyDTO{
		Available:  true,
		Result:     response.Result,
		Answer:     response.Answer,
		Configured: response.Backend.Configured,
		Provider:   response.Backend.Provider,
		Model:      response.Backend.Model,
		Proposals:  []BackendProposalDTO{},
	}
	for _, summary := range response.Proposals {
		reply.Proposals = append(reply.Proposals, assistantProposalDTO(summary, titles))
	}
	return reply, nil
}

// assistantPlanningContext builds the redacted context: zone, now, estimate
// id, availability windows, and task ids with bounds. Titles never leave the
// device; the rail re-attaches them locally for display.
func (a *App) assistantPlanningContext(ctx context.Context) (assistantContextPayload, error) {
	now := time.Now().UTC().Truncate(time.Minute)
	payload := assistantContextPayload{ZoneID: defaultZoneID, Now: now}
	store, err := a.requireStore()
	if err != nil {
		return payload, err
	}
	state, err := a.localEstimate(ctx, now)
	if err != nil {
		return payload, err
	}
	if state.Status == "estimated" {
		payload.ZoneID = state.Estimate.AsOf.ZoneID
		payload.EstimateID = string(state.Estimate.ID)
		for _, window := range localPlanningAvailability(state, now) {
			payload.Availability = append(payload.Availability, assistantAvailabilityEntry{
				Kind:       string(window.Kind),
				StartAt:    window.Interval.Start.UTC,
				EndAt:      window.Interval.End.UTC,
				ZoneID:     window.Interval.Start.ZoneID,
				Confidence: string(window.Confidence.Level),
			})
		}
	}
	records, err := store.ListTasks(ctx)
	if err != nil {
		return payload, err
	}
	for _, record := range records {
		if record.Status != storage.TaskStatusOpen {
			continue
		}
		payload.Tasks = append(payload.Tasks, assistantTaskContext{
			TaskID:                    record.TaskID,
			DurationMinutes:           record.DurationMinutes,
			EarliestStartAt:           record.EarliestStartAt,
			LatestFinishAt:            record.LatestFinishAt,
			PreferredAfterWakeMinutes: record.PreferredAfterWakeMinutes,
			MinimumConfidence:         record.MinimumConfidence,
		})
	}
	return payload, nil
}

func (a *App) localTaskTitles(ctx context.Context) (map[string]string, error) {
	store, err := a.requireStore()
	if err != nil {
		return nil, err
	}
	records, err := store.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	titles := make(map[string]string, len(records))
	for _, record := range records {
		titles[record.TaskID] = record.Title
	}
	return titles, nil
}

// assistantProposalDTO maps a message-response proposal summary through the
// shared backend-proposal mapping, then swaps the private local task title in
// for the task id (titles exist only on this device).
func assistantProposalDTO(summary assistantProposalSummary, titles map[string]string) BackendProposalDTO {
	var meta struct {
		ActionID          string    `json:"action_id"`
		CreatedAt         time.Time `json:"created_at"`
		ExpiresAt         time.Time `json:"expires_at"`
		ScheduleProposals struct {
			Proposals []struct {
				TaskID string `json:"task_id"`
			} `json:"proposals"`
		} `json:"schedule_proposals"`
	}
	_ = json.Unmarshal(summary.Payload, &meta)
	dto := backendProposalDTO(backendProposalRecord{
		ProposalID:    summary.ProposalID,
		ActionID:      meta.ActionID,
		Status:        summary.Status,
		CreatedAt:     meta.CreatedAt,
		ExpiresAt:     meta.ExpiresAt,
		Payload:       summary.Payload,
		DecisionToken: summary.DecisionToken,
	})
	if len(meta.ScheduleProposals.Proposals) > 0 {
		if title, ok := titles[meta.ScheduleProposals.Proposals[0].TaskID]; ok && title != "" {
			dto.Title = backendProposalTitle(meta.ActionID, title)
		}
	}
	return dto
}

// AppearanceClockDTO gives the appearance scheduler structured forecast
// times (ADR-0021): when predicted sleep begins and when the sleeper is
// predicted to wake. Display automation is a local, reversible action — it
// never goes through the approval queue.
type AppearanceClockDTO struct {
	Status       string `json:"status"`
	SleepStartAt string `json:"sleepStartAt,omitempty"`
	WakeAt       string `json:"wakeAt,omitempty"`
}

func (a *App) GetAppearanceClock() (AppearanceClockDTO, error) {
	now := time.Now().UTC()
	state, err := a.localEstimate(context.Background(), now)
	if err != nil {
		return AppearanceClockDTO{}, err
	}
	if state.Status != "estimated" || len(state.Estimate.PredictedSleepWindows) == 0 {
		return AppearanceClockDTO{Status: state.Status}, nil
	}
	window := state.Estimate.PredictedSleepWindows[0].Interval
	return AppearanceClockDTO{
		Status:       "estimated",
		SleepStartAt: window.Start.UTC.Format(time.RFC3339),
		WakeAt:       window.End.UTC.Format(time.RFC3339),
	}, nil
}
