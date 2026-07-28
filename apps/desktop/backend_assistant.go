package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"non24.app/core/agentpolicy"
	storage "non24.app/core/storage/sqlite"
)

const (
	maxBackendAssistantTasks        = 100
	maxBackendAssistantAvailability = 128
	maxBackendAssistantFixedEvents  = 256
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
	ZoneID          string                       `json:"zone_id"`
	Now             time.Time                    `json:"now"`
	EstimateID      string                       `json:"estimate_id,omitempty"`
	Tasks           []assistantTaskContext       `json:"tasks,omitempty"`
	Availability    []assistantAvailabilityEntry `json:"availability,omitempty"`
	FixedEvents     []assistantFixedEventContext `json:"fixed_events,omitempty"`
	MedicationFacts []assistantMedicationFact    `json:"medication_facts,omitempty"`
	Markers         []assistantMarkerFact        `json:"markers,omitempty"`
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

type assistantFixedEventContext struct {
	EventID string    `json:"event_id"`
	StartAt time.Time `json:"start_at"`
	EndAt   time.Time `json:"end_at"`
	ZoneID  string    `json:"zone_id"`
}

type assistantMedicationFact struct {
	MedicationID             string `json:"medication_id"`
	Active                   bool   `json:"active"`
	ScheduleKind             string `json:"schedule_kind"`
	ScheduledOccurrenceCount int    `json:"scheduled_occurrence_count"`
	CollisionCount           int    `json:"collision_count"`
	NextScheduledCivilDate   string `json:"next_scheduled_civil_date,omitempty"`
	NextScheduledCivilTime   string `json:"next_scheduled_civil_time,omitempty"`
	ScheduleZoneID           string `json:"schedule_zone_id,omitempty"`
	LoggedEventCount         int    `json:"logged_event_count"`
	LastLoggedStatus         string `json:"last_logged_status,omitempty"`
	LastWakeRelation         string `json:"last_wake_relation,omitempty"`
	LastSleepRelation        string `json:"last_sleep_relation,omitempty"`
	Confidence               string `json:"confidence,omitempty"`
}

type assistantMarkerFact struct {
	MarkerID       string `json:"marker_id"`
	Kind           string `json:"kind"`
	CivilStartDate string `json:"civil_start_date"`
	CivilEndDate   string `json:"civil_end_date,omitempty"`
}

type assistantFactScope struct {
	Medication bool
	Markers    bool
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
	client := a.newDesktopBackendClient(cfg, token)
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
	if utf8.RuneCountInString(message) > 2000 {
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
	scope := assistantFactScope{}
	// A prompt that will be refused does not need health facts to cross even
	// the owner-controlled backend boundary.
	if !agentpolicy.IsMedicalDecisionPrompt(message) {
		scope.Medication = agentpolicy.ContainsMedicationFactSubject(message)
		scope.Markers = agentpolicy.ContainsMarkerSubject(message)
	}
	planning, err := a.assistantPlanningContext(ctx, scope)
	if err != nil {
		return AssistantReplyDTO{}, err
	}
	client := a.newDesktopBackendClient(cfg, token)
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
func (a *App) assistantPlanningContext(ctx context.Context, scope assistantFactScope) (assistantContextPayload, error) {
	now := a.currentTime().UTC().Truncate(time.Minute)
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
		if zoneID := strings.TrimSpace(state.Estimate.AsOf.ZoneID); zoneID != "" {
			payload.ZoneID = zoneID
		}
		payload.EstimateID = string(state.Estimate.ID)
		availability := localPlanningAvailability(state, now)
		if len(availability) > maxBackendAssistantAvailability {
			availability = availability[:maxBackendAssistantAvailability]
		}
		for _, window := range availability {
			zoneID := strings.TrimSpace(window.Interval.Start.ZoneID)
			if zoneID == "" {
				zoneID = payload.ZoneID
			}
			payload.Availability = append(payload.Availability, assistantAvailabilityEntry{
				Kind:       string(window.Kind),
				StartAt:    window.Interval.Start.UTC,
				EndAt:      window.Interval.End.UTC,
				ZoneID:     zoneID,
				Confidence: string(window.Confidence.Level),
			})
		}
		if start, end, ok := planningSnapshotRange(availability); ok {
			fixedEvents, _, eventErr := store.BusyDomainEvents(ctx, start, end, payload.ZoneID)
			if eventErr != nil {
				return payload, eventErr
			}
			if len(fixedEvents) > maxBackendAssistantFixedEvents {
				fixedEvents = fixedEvents[:maxBackendAssistantFixedEvents]
			}
			for index, event := range fixedEvents {
				zoneID := strings.TrimSpace(event.Interval.Start.ZoneID)
				if zoneID == "" {
					zoneID = payload.ZoneID
				}
				payload.FixedEvents = append(payload.FixedEvents, assistantFixedEventContext{
					EventID: fmt.Sprintf("event_%03d", index+1),
					StartAt: event.Interval.Start.UTC,
					EndAt:   event.Interval.End.UTC,
					ZoneID:  zoneID,
				})
			}
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
		if len(payload.Tasks) >= maxBackendAssistantTasks {
			break
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

	if !scope.Medication && !scope.Markers {
		return payload, nil
	}

	if scope.Medication {
		medicationProjection, err := a.agentMedicationProjection(ctx)
		if err != nil {
			return payload, err
		}
		medications := append([]agentMedicationDTO{}, medicationProjection.Medications...)
		sort.Slice(medications, func(i, j int) bool {
			return medications[i].MedicationID < medications[j].MedicationID
		})
		if len(medications) > 8 {
			medications = medications[:8]
		}
		for _, medication := range medications {
			fact := assistantMedicationFact{
				MedicationID:     medication.MedicationID,
				Active:           medication.Active,
				ScheduleKind:     medication.ScheduleKind,
				LoggedEventCount: medication.IncludedEventCount,
			}
			if medication.Schedule != nil {
				fact.ScheduledOccurrenceCount = medication.Schedule.Forecast.CoveredCount
				fact.CollisionCount = medication.Schedule.Forecast.CollisionCount
				fact.ScheduleZoneID = medication.Schedule.ZoneID
				if len(medication.Schedule.Forecast.Occurrences) > 0 {
					next := medication.Schedule.Forecast.Occurrences[0]
					fact.NextScheduledCivilDate = next.CivilDate
					fact.NextScheduledCivilTime = next.CivilTime
				}
			}
			if latest := medication.LogSummary.Latest; latest != nil {
				fact.LastLoggedStatus = latest.Status
				fact.LastWakeRelation = latest.WakeRelation
				fact.LastSleepRelation = latest.SleepRelation
				fact.Confidence = latest.Confidence
			}
			payload.MedicationFacts = append(payload.MedicationFacts, fact)
		}
	}

	if scope.Markers {
		markerProjection, err := a.agentMarkerProjection(ctx)
		if err != nil {
			return payload, err
		}
		markers := append([]agentMarkerDTO{}, markerProjection.Markers...)
		if len(markers) > 12 {
			markers = markers[:12]
		}
		for _, marker := range markers {
			payload.Markers = append(payload.Markers, assistantMarkerFact{
				MarkerID:       marker.MarkerID,
				Kind:           marker.Kind,
				CivilStartDate: marker.CivilStartDate,
				CivilEndDate:   marker.CivilEndDate,
			})
		}
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
	now := a.currentTime().UTC()
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
