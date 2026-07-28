package main

import (
	"context"
	"sort"
	"time"

	storage "non24.app/core/storage/sqlite"
)

const (
	maxAgentTasks                 = 100
	maxAgentMedications           = 32
	maxAgentScheduleOccurrences   = 32
	maxAgentScheduleGaps          = 16
	maxAgentMarkers               = 64
	maxAgentRhythmForecastWindows = 14
)

type agentStatusDTO struct {
	SchemaVersion             string   `json:"schema_version"`
	Mode                      string   `json:"mode"`
	Running                   bool     `json:"running"`
	BackendProposalsAvailable bool     `json:"backend_proposals_available"`
	LocalStoreAvailable       bool     `json:"local_store_available"`
	AppearanceStatus          string   `json:"appearance_status"`
	Capabilities              []string `json:"capabilities"`
	MedicalScope              string   `json:"medical_scope"`
	Disclaimer                string   `json:"disclaimer"`
}

type agentRefusalDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type agentOverviewDTO struct {
	SchemaVersion            string           `json:"schema_version"`
	EstimateSource           string           `json:"estimate_source"`
	Status                   string           `json:"status"`
	Empty                    bool             `json:"empty"`
	Refusal                  *agentRefusalDTO `json:"refusal,omitempty"`
	CurrentEstimatedState    string           `json:"current_estimated_state"`
	TimeSinceWake            string           `json:"time_since_wake"`
	PredictedNextSleepWindow string           `json:"predicted_next_sleep_window"`
	DriftEstimate            string           `json:"drift_estimate"`
	Confidence               string           `json:"confidence"`
	ConfidenceReasons        []string         `json:"confidence_reasons"`
	NextUsefulTaskWindow     string           `json:"next_useful_task_window"`
	Disclaimer               string           `json:"disclaimer"`
}

type agentRhythmDTO struct {
	SchemaVersion     string                   `json:"schema_version"`
	EstimateSource    string                   `json:"estimate_source"`
	Status            string                   `json:"status"`
	Refusal           *agentRefusalDTO         `json:"refusal,omitempty"`
	ActogramSummary   string                   `json:"actogram_summary"`
	DriftTitle        string                   `json:"drift_title"`
	SlopeLabel        string                   `json:"slope_label"`
	DriftConfidence   string                   `json:"drift_confidence"`
	DriftSummary      string                   `json:"drift_summary"`
	ForecastWindows   []agentRhythmForecastDTO `json:"forecast_windows"`
	ForecastTruncated bool                     `json:"forecast_truncated"`
	Disclaimer        string                   `json:"disclaimer"`
}

type agentRhythmForecastDTO struct {
	CivilDate     string  `json:"civil_date"`
	ZoneID        string  `json:"zone_id"`
	StartHour     float64 `json:"start_hour"`
	DurationHours float64 `json:"duration_hours"`
	StartLabel    string  `json:"start_label"`
	WakeLabel     string  `json:"wake_label"`
	Confidence    string  `json:"confidence"`
}

type agentTasksDTO struct {
	SchemaVersion string         `json:"schema_version"`
	Count         int            `json:"count"`
	Truncated     bool           `json:"truncated"`
	Tasks         []agentTaskDTO `json:"tasks"`
	PrivateFields string         `json:"private_fields"`
}

type agentTaskDTO struct {
	TaskID                    string `json:"task_id"`
	DurationMinutes           int    `json:"duration_minutes"`
	Status                    string `json:"status"`
	EarliestStartAt           string `json:"earliest_start_at,omitempty"`
	LatestFinishAt            string `json:"latest_finish_at,omitempty"`
	PreferredAfterWakeMinutes *int   `json:"preferred_after_wake_minutes,omitempty"`
	MinimumConfidence         string `json:"minimum_confidence,omitempty"`
}

type agentMedicationTimingDTO struct {
	SchemaVersion      string               `json:"schema_version"`
	Status             string               `json:"status"`
	EstimateStatus     string               `json:"estimate_status"`
	MedicationCount    int                  `json:"medication_count"`
	IncludedEventCount int                  `json:"included_event_count"`
	Truncated          bool                 `json:"truncated"`
	Medications        []agentMedicationDTO `json:"medications"`
	Disclaimer         string               `json:"disclaimer"`
	PrivateFields      string               `json:"private_fields"`
}

type agentMedicationDTO struct {
	MedicationID       string                       `json:"medication_id"`
	Active             bool                         `json:"active"`
	ScheduleKind       string                       `json:"schedule_kind"`
	IncludedEventCount int                          `json:"included_event_count"`
	LogSummary         agentMedicationLogSummaryDTO `json:"logged_event_summary"`
	Schedule           *agentMedicationScheduleDTO  `json:"schedule,omitempty"`
}

type agentMedicationLogSummaryDTO struct {
	TakenCount          int                           `json:"taken_count"`
	SkippedCount        int                           `json:"skipped_count"`
	ScheduledCount      int                           `json:"scheduled_count"`
	UnscheduledCount    int                           `json:"unscheduled_count"`
	ExcludedCount       int                           `json:"excluded_count"`
	CorrectedEventCount int                           `json:"corrected_event_count"`
	Latest              *agentMedicationLatestFactDTO `json:"latest,omitempty"`
}

type agentMedicationLatestFactDTO struct {
	Status            string `json:"status"`
	Scheduled         bool   `json:"scheduled"`
	WakeRelation      string `json:"wake_relation"`
	SleepRelation     string `json:"sleep_relation"`
	SleepRelationKind string `json:"sleep_relation_kind"`
	Confidence        string `json:"confidence"`
	Excluded          bool   `json:"excluded"`
}

type agentMedicationScheduleDTO struct {
	Kind            string                     `json:"kind"`
	ZoneID          string                     `json:"zone_id,omitempty"`
	CivilTimes      []string                   `json:"civil_times"`
	DaysOn          int                        `json:"days_on,omitempty"`
	DaysOff         int                        `json:"days_off,omitempty"`
	CycleStartedOn  string                     `json:"cycle_started_on,omitempty"`
	ReminderEnabled bool                       `json:"reminder_enabled"`
	Forecast        agentMedicationForecastDTO `json:"forecast"`
}

type agentMedicationForecastDTO struct {
	Status              string                         `json:"status"`
	CoveredCount        int                            `json:"covered_count"`
	CollisionCount      int                            `json:"collision_count"`
	OutsideHorizonCount int                            `json:"outside_horizon_count"`
	Occurrences         []agentMedicationOccurrenceDTO `json:"occurrences"`
	Gaps                []agentMedicationGapDTO        `json:"gaps"`
	Truncated           bool                           `json:"truncated"`
}

type agentMedicationOccurrenceDTO struct {
	CivilDate  string `json:"civil_date"`
	CivilTime  string `json:"civil_time"`
	Status     string `json:"status"`
	Context    string `json:"context"`
	Confidence string `json:"confidence"`
	Ambiguous  bool   `json:"ambiguous"`
	DSTNote    string `json:"dst_note,omitempty"`
}

type agentMedicationGapDTO struct {
	CivilDate string `json:"civil_date"`
	CivilTime string `json:"civil_time"`
}

type agentMarkersDTO struct {
	SchemaVersion string           `json:"schema_version"`
	Status        string           `json:"status"`
	Count         int              `json:"count"`
	Truncated     bool             `json:"truncated"`
	Markers       []agentMarkerDTO `json:"markers"`
	PrivateFields string           `json:"private_fields"`
	Disclaimer    string           `json:"disclaimer"`
}

type agentMarkerDTO struct {
	MarkerID       string `json:"marker_id"`
	Kind           string `json:"kind"`
	CivilStartDate string `json:"civil_start_date"`
	CivilEndDate   string `json:"civil_end_date,omitempty"`
	ZoneID         string `json:"zone_id"`
}

func (a *App) agentStatusProjection() agentStatusDTO {
	status := a.GetLocalAgentStatus()
	capabilities := []string{
		"read_overview",
		"read_rhythm_summary",
		"read_tasks_without_titles",
		"read_medication_timing_without_private_text",
		"read_markers_without_notes",
		"read_appearance",
		"set_reversible_appearance",
		"answer_local_facts",
	}
	if status.BackendProposalsAvailable {
		capabilities = append(capabilities, "create_pending_backend_proposals")
	}
	return agentStatusDTO{
		SchemaVersion:             "v1",
		Mode:                      status.Mode,
		Running:                   status.Running,
		BackendProposalsAvailable: status.BackendProposalsAvailable,
		LocalStoreAvailable:       status.LocalStoreAvailable,
		AppearanceStatus:          status.AppearanceStatus,
		Capabilities:              capabilities,
		MedicalScope:              "facts only; medical decisions are refused",
		Disclaimer:                disclaimer,
	}
}

func (a *App) agentOverviewProjection(ctx context.Context) (agentOverviewDTO, error) {
	overview, err := a.localOverview(ctx, a.currentTime().UTC().Truncate(time.Minute))
	if err != nil {
		return agentOverviewDTO{}, localAgentProjectionError("overview", err)
	}
	projection := agentOverviewDTO{
		SchemaVersion:            "v1",
		EstimateSource:           overview.EstimateSource,
		Status:                   overview.Status,
		Empty:                    overview.Empty,
		CurrentEstimatedState:    overview.CurrentEstimatedState,
		TimeSinceWake:            overview.TimeSinceWake,
		PredictedNextSleepWindow: overview.PredictedNextSleepWindow,
		DriftEstimate:            overview.DriftEstimate,
		Confidence:               overview.Confidence,
		ConfidenceReasons:        append([]string{}, overview.ConfidenceReasons...),
		NextUsefulTaskWindow:     overview.NextUsefulTaskWindow,
		Disclaimer:               overview.Disclaimer,
	}
	if overview.Refusal != nil {
		projection.Refusal = &agentRefusalDTO{Code: overview.Refusal.Code, Message: overview.Refusal.Message}
	}
	return projection, nil
}

func (a *App) agentRhythmProjection(ctx context.Context) (agentRhythmDTO, error) {
	rhythm, err := a.localRhythm(ctx, a.currentTime().UTC().Truncate(time.Minute))
	if err != nil {
		return agentRhythmDTO{}, localAgentProjectionError("rhythm", err)
	}
	projection := agentRhythmDTO{
		SchemaVersion:   "v1",
		EstimateSource:  rhythm.EstimateSource,
		Status:          rhythm.Status,
		ActogramSummary: rhythm.ActogramSummary,
		DriftTitle:      rhythm.DriftTitle,
		SlopeLabel:      rhythm.SlopeLabel,
		DriftConfidence: rhythm.DriftConfidence,
		DriftSummary:    rhythm.DriftSummary,
		ForecastWindows: []agentRhythmForecastDTO{},
		Disclaimer:      disclaimer,
	}
	if rhythm.Refusal != nil {
		projection.Refusal = &agentRefusalDTO{Code: string(rhythm.Refusal.Code), Message: rhythm.Refusal.Message}
	}
	limit := len(rhythm.ForecastRows)
	if limit > maxAgentRhythmForecastWindows {
		limit = maxAgentRhythmForecastWindows
		projection.ForecastTruncated = true
	}
	for _, row := range rhythm.ForecastRows[:limit] {
		projection.ForecastWindows = append(projection.ForecastWindows, agentRhythmForecastDTO{
			CivilDate: row.CivilDate, ZoneID: row.ZoneID, StartHour: row.StartHour,
			DurationHours: row.DurationHours, StartLabel: row.StartLabel,
			WakeLabel: row.WakeLabel, Confidence: row.Confidence,
		})
	}
	return projection, nil
}

func (a *App) agentTaskProjection(ctx context.Context) (agentTasksDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return agentTasksDTO{}, localAgentProjectionError("task", err)
	}
	records, err := store.ListTasks(ctx)
	if err != nil {
		return agentTasksDTO{}, localAgentProjectionError("task", err)
	}
	projection := agentTasksDTO{
		SchemaVersion: "v1",
		Count:         len(records),
		Tasks:         []agentTaskDTO{},
		PrivateFields: "task titles are intentionally omitted",
	}
	limit := len(records)
	if limit > maxAgentTasks {
		limit = maxAgentTasks
		projection.Truncated = true
	}
	for _, record := range records[:limit] {
		projection.Tasks = append(projection.Tasks, agentTaskDTO{
			TaskID: record.TaskID, DurationMinutes: record.DurationMinutes, Status: record.Status,
			EarliestStartAt:           formatOptionalAgentTime(record.EarliestStartAt),
			LatestFinishAt:            formatOptionalAgentTime(record.LatestFinishAt),
			PreferredAfterWakeMinutes: record.PreferredAfterWakeMinutes,
			MinimumConfidence:         record.MinimumConfidence,
		})
	}
	return projection, nil
}

func (a *App) agentMedicationProjection(ctx context.Context) (agentMedicationTimingDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return agentMedicationTimingDTO{}, localAgentProjectionError("medication timing", err)
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	records, err := store.ListMedications(ctx)
	if err != nil {
		return agentMedicationTimingDTO{}, localAgentProjectionError("medication timing", err)
	}
	events, err := store.EffectiveMedicationEvents(ctx)
	if err != nil {
		return agentMedicationTimingDTO{}, localAgentProjectionError("medication timing", err)
	}
	state, err := a.localEstimate(ctx, now)
	if err != nil {
		return agentMedicationTimingDTO{}, localAgentProjectionError("medication timing", err)
	}
	eventCounts := make(map[string]int, len(records))
	for _, event := range events {
		eventCounts[event.Event.MedicationID]++
	}
	medications := make([]MedicationDTO, 0, len(records))
	for _, record := range records {
		medication, err := medicationDTO(record, eventCounts[record.MedicationID], state, now)
		if err != nil {
			return agentMedicationTimingDTO{}, localAgentProjectionError("medication timing", err)
		}
		medications = append(medications, medication)
	}
	status := "ready"
	if len(medications) == 0 {
		status = "empty"
	}
	projection := agentMedicationTimingDTO{
		SchemaVersion: "v1", Status: status, EstimateStatus: state.Status,
		MedicationCount: len(medications),
		Medications:     []agentMedicationDTO{},
		Disclaimer:      "Medication timing shown here is user-entered or derived context, not medical advice.",
		PrivateFields:   "medication labels, form, strength, clinician text, notes, logged event rows, and exact logged timestamps are intentionally omitted",
	}
	sort.Slice(medications, func(i, j int) bool {
		return medications[i].MedicationID < medications[j].MedicationID
	})
	summaries := medicationLogSummaries(events, state)
	medicationLimit := len(medications)
	if medicationLimit > maxAgentMedications {
		medicationLimit = maxAgentMedications
		projection.Truncated = true
	}
	for _, medication := range medications[:medicationLimit] {
		summary := summaries[medication.MedicationID]
		includedEventCount := summary.TakenCount + summary.SkippedCount
		item := agentMedicationDTO{
			MedicationID: medication.MedicationID, Active: medication.Active,
			ScheduleKind: medication.ScheduleKind, IncludedEventCount: includedEventCount,
			LogSummary: summary,
		}
		projection.IncludedEventCount += includedEventCount
		if medication.Schedule != nil {
			schedule := medication.Schedule
			mapped := agentMedicationScheduleDTO{
				Kind: schedule.Kind, ZoneID: schedule.ZoneID,
				CivilTimes: append([]string{}, schedule.CivilTimes...), DaysOn: schedule.DaysOn,
				DaysOff: schedule.DaysOff, CycleStartedOn: schedule.CycleStartedOn,
				ReminderEnabled: schedule.ReminderEnabled,
				Forecast: agentMedicationForecastDTO{
					Status: schedule.Forecast.Status, CoveredCount: schedule.Forecast.CoveredCount,
					CollisionCount:      schedule.Forecast.CollisionCount,
					OutsideHorizonCount: schedule.Forecast.OutsideHorizonCount,
					Occurrences:         []agentMedicationOccurrenceDTO{}, Gaps: []agentMedicationGapDTO{},
				},
			}
			occurrenceLimit := len(schedule.Forecast.Occurrences)
			if occurrenceLimit > maxAgentScheduleOccurrences {
				occurrenceLimit = maxAgentScheduleOccurrences
				mapped.Forecast.Truncated = true
				projection.Truncated = true
			}
			for _, occurrence := range schedule.Forecast.Occurrences[:occurrenceLimit] {
				mapped.Forecast.Occurrences = append(mapped.Forecast.Occurrences, agentMedicationOccurrenceDTO{
					CivilDate: occurrence.CivilDate, CivilTime: occurrence.CivilTime,
					Status: occurrence.Status, Context: occurrence.Context,
					Confidence: occurrence.Confidence, Ambiguous: occurrence.Ambiguous, DSTNote: occurrence.DSTNote,
				})
			}
			gapLimit := len(schedule.Forecast.Gaps)
			if gapLimit > maxAgentScheduleGaps {
				gapLimit = maxAgentScheduleGaps
				mapped.Forecast.Truncated = true
				projection.Truncated = true
			}
			for _, gap := range schedule.Forecast.Gaps[:gapLimit] {
				mapped.Forecast.Gaps = append(mapped.Forecast.Gaps, agentMedicationGapDTO{CivilDate: gap.CivilDate, CivilTime: gap.CivilTime})
			}
			item.Schedule = &mapped
		}
		projection.Medications = append(projection.Medications, item)
	}
	return projection, nil
}

func medicationLogSummaries(events []storage.EffectiveMedicationEvent, state localEstimateState) map[string]agentMedicationLogSummaryDTO {
	summaries := make(map[string]agentMedicationLogSummaryDTO)
	latest := make(map[string]storage.EffectiveMedicationEvent)
	for _, event := range events {
		medicationID := event.Event.MedicationID
		summary := summaries[medicationID]
		if len(event.Corrections) > 0 {
			summary.CorrectedEventCount++
		}
		if event.Excluded {
			summary.ExcludedCount++
			summaries[medicationID] = summary
			continue
		}
		switch event.Event.Status {
		case "taken":
			summary.TakenCount++
		case "skipped":
			summary.SkippedCount++
		}
		if event.Event.Scheduled {
			summary.ScheduledCount++
		} else {
			summary.UnscheduledCount++
		}
		current, exists := latest[medicationID]
		if !exists || event.Event.DoseAt.After(current.Event.DoseAt) ||
			(event.Event.DoseAt.Equal(current.Event.DoseAt) && event.Event.EventID > current.Event.EventID) {
			latest[medicationID] = event
		}
		summaries[medicationID] = summary
	}
	sleepIndex := newMedicationSleepIndex(state.Sessions)
	anchors := medicationWakeAnchors(state.Sessions)
	latestWake := latestMedicationWake(anchors)
	for medicationID, event := range latest {
		projected := medicationEventDTO(event, "", state, sleepIndex, anchors, latestWake)
		summary := summaries[medicationID]
		summary.Latest = &agentMedicationLatestFactDTO{
			Status: projected.Status, Scheduled: projected.Scheduled,
			WakeRelation: projected.WakeRelation, SleepRelation: projected.SleepRelation,
			SleepRelationKind: projected.SleepRelationKind, Confidence: projected.Confidence,
		}
		summaries[medicationID] = summary
	}
	return summaries
}

func (a *App) agentMarkerProjection(ctx context.Context) (agentMarkersDTO, error) {
	data, err := a.rhythmMarkersAtContext(ctx, a.currentTime().UTC().Truncate(time.Second))
	if err != nil {
		return agentMarkersDTO{}, localAgentProjectionError("rhythm marker", err)
	}
	projection := agentMarkersDTO{
		SchemaVersion: "v1", Status: data.Status, Count: len(data.Markers),
		Markers: []agentMarkerDTO{}, PrivateFields: "marker notes and exact record timestamps are intentionally omitted",
		Disclaimer: "Markers are optional self-reports. They do not change the rhythm estimate or establish cause.",
	}
	markers := make([]agentMarkerDTO, 0, len(data.Markers))
	for _, marker := range data.Markers {
		endDate, err := markerEndCivilDate(marker)
		if err != nil {
			return agentMarkersDTO{}, localAgentProjectionError("rhythm marker", err)
		}
		markers = append(markers, agentMarkerDTO{
			MarkerID: marker.MarkerID, Kind: marker.Kind, CivilStartDate: marker.CivilDate,
			CivilEndDate: endDate, ZoneID: marker.ZoneID,
		})
	}
	sort.Slice(markers, func(i, j int) bool {
		if markers[i].CivilStartDate == markers[j].CivilStartDate {
			return markers[i].MarkerID < markers[j].MarkerID
		}
		return markers[i].CivilStartDate > markers[j].CivilStartDate
	})
	limit := len(markers)
	if limit > maxAgentMarkers {
		limit = maxAgentMarkers
		projection.Truncated = true
	}
	projection.Markers = append(projection.Markers, markers[:limit]...)
	return projection, nil
}

func markerEndCivilDate(marker RhythmMarkerDTO) (string, error) {
	if marker.EndAt == "" {
		return "", nil
	}
	end, err := time.Parse(time.RFC3339, marker.EndAt)
	if err != nil {
		return "", err
	}
	location, err := time.LoadLocation(marker.ZoneID)
	if err != nil {
		return "", err
	}
	return end.In(location).Format(time.DateOnly), nil
}

func formatOptionalAgentTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
