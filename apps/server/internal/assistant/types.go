package assistant

import (
	"encoding/json"
	"time"

	"non24.app/server/internal/provider"
	"non24.app/server/internal/store"
)

const SchemaVersion = "v1"

type MessageRequest struct {
	SchemaVersion string          `json:"schema_version"`
	Message       string          `json:"message"`
	Context       PlanningContext `json:"context"`
}

type PlanningContext struct {
	ZoneID          string                    `json:"zone_id"`
	Now             time.Time                 `json:"now"`
	EstimateID      string                    `json:"estimate_id,omitempty"`
	Tasks           []TaskContext             `json:"tasks,omitempty"`
	Availability    []AvailabilityContext     `json:"availability,omitempty"`
	FixedEvents     []FixedEventContext       `json:"fixed_events,omitempty"`
	MedicationFacts []MedicationFactContext   `json:"medication_facts,omitempty"`
	Markers         []RhythmMarkerFactContext `json:"markers,omitempty"`
}

type TaskContext struct {
	TaskID                    string     `json:"task_id"`
	DurationMinutes           int        `json:"duration_minutes"`
	EarliestStartAt           *time.Time `json:"earliest_start_at,omitempty"`
	LatestFinishAt            *time.Time `json:"latest_finish_at,omitempty"`
	PreferredAfterWakeMinutes *int       `json:"preferred_after_wake_minutes,omitempty"`
	MinimumConfidence         string     `json:"minimum_confidence,omitempty"`
	BusinessHours             bool       `json:"business_hours,omitempty"`
	BusinessStartLocal        string     `json:"business_start_local,omitempty"`
	BusinessEndLocal          string     `json:"business_end_local,omitempty"`
}

type AvailabilityContext struct {
	Kind       string    `json:"kind"`
	StartAt    time.Time `json:"start_at"`
	EndAt      time.Time `json:"end_at"`
	ZoneID     string    `json:"zone_id"`
	Confidence string    `json:"confidence"`
}

type FixedEventContext struct {
	EventID string    `json:"event_id"`
	StartAt time.Time `json:"start_at"`
	EndAt   time.Time `json:"end_at"`
	ZoneID  string    `json:"zone_id"`
}

type MedicationFactContext struct {
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

type RhythmMarkerFactContext struct {
	MarkerID       string `json:"marker_id"`
	Kind           string `json:"kind"`
	CivilStartDate string `json:"civil_start_date"`
	CivilEndDate   string `json:"civil_end_date,omitempty"`
}

type MessageResponse struct {
	SchemaVersion string            `json:"schema_version"`
	Backend       provider.Status   `json:"backend"`
	Result        string            `json:"result"`
	Action        string            `json:"action"`
	Answer        string            `json:"answer"`
	Proposals     []ProposalSummary `json:"proposals"`
}

type ProposalSummary struct {
	ProposalID    string               `json:"proposalId"`
	Status        store.ProposalStatus `json:"status"`
	DecisionToken string               `json:"decisionToken,omitempty"`
	Payload       json.RawMessage      `json:"payload"`
}

type DirectProposalRequest struct {
	SchemaVersion     string          `json:"schema_version"`
	RecommendedAction string          `json:"recommended_action"`
	Target            *ActionTarget   `json:"target,omitempty"`
	Context           PlanningContext `json:"context"`
}

type modelAction struct {
	SchemaVersion     string        `json:"schema_version"`
	RecommendedAction string        `json:"recommended_action"`
	Target            *actionTarget `json:"target,omitempty"`
	Answer            string        `json:"answer,omitempty"`
}

type actionTarget struct {
	TaskID                    string     `json:"task_id"`
	EarliestStartAt           *time.Time `json:"earliest_start_at,omitempty"`
	LatestFinishAt            *time.Time `json:"latest_finish_at,omitempty"`
	DurationMinutes           int        `json:"duration_minutes,omitempty"`
	PreferredAfterWakeMinutes *int       `json:"preferred_after_wake_minutes,omitempty"`
	ReminderID                string     `json:"reminder_id,omitempty"`
}

type ActionTarget = actionTarget

type storedProposalPayload struct {
	ProposalID        string                   `json:"proposal_id"`
	ActionID          string                   `json:"action_id"`
	ScheduleProposals scheduleProposalsPayload `json:"schedule_proposals"`
	Answer            string                   `json:"answer,omitempty"`
	CreatedAt         time.Time                `json:"created_at"`
	ExpiresAt         time.Time                `json:"expires_at"`
}

type scheduleProposalsPayload struct {
	SchemaVersion    string                    `json:"schema_version"`
	RequestID        string                    `json:"request_id"`
	GeneratedAt      time.Time                 `json:"generated_at"`
	AlgorithmVersion string                    `json:"algorithm_version"`
	Proposals        []scheduleProposalPayload `json:"proposals"`
	Unplaced         []unplacedPayload         `json:"unplaced"`
}

type scheduleProposalPayload struct {
	ProposalID       string            `json:"proposal_id"`
	TaskID           string            `json:"task_id"`
	StartAt          time.Time         `json:"start_at"`
	EndAt            time.Time         `json:"end_at"`
	ZoneID           string            `json:"zone_id"`
	Confidence       confidencePayload `json:"confidence"`
	ExplanationCodes []string          `json:"explanation_codes"`
}

type unplacedPayload struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type confidencePayload struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons"`
}
