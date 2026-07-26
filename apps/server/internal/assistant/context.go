package assistant

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	contextIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,79}$`)
	contextCivilTimePattern  = regexp.MustCompile(`^(?:[01]\d|2[0-3]):[0-5]\d$`)
	contextZonePattern       = regexp.MustCompile(`^(?:UTC|[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)+)$`)
	contextDurationPattern   = regexp.MustCompile(`^[0-9]{1,6} (?:min|h(?: [0-9]{1,2} min)?)$`)
)

const (
	maxContextTasks        = 100
	maxContextAvailability = 128
	maxContextFixedEvents  = 256
	maxContextMedications  = 8
	maxContextMarkers      = 12
)

type redactedContext struct {
	Now          string                 `json:"now"`
	ZoneID       string                 `json:"zone_id"`
	EstimateID   string                 `json:"estimate_id,omitempty"`
	Tasks        []redactedTask         `json:"tasks,omitempty"`
	Availability []redactedAvailability `json:"availability,omitempty"`
	FixedEvents  []redactedFixedEvent   `json:"fixed_events,omitempty"`
	Policy       []string               `json:"policy"`
}

type redactedTask struct {
	TaskID                    string `json:"task_id"`
	DurationMinutes           int    `json:"duration_minutes"`
	Earliest                  string `json:"earliest,omitempty"`
	Latest                    string `json:"latest,omitempty"`
	MinimumConfidence         string `json:"minimum_confidence,omitempty"`
	PreferredAfterWakeMinutes *int   `json:"preferred_after_wake_minutes,omitempty"`
}

type redactedAvailability struct {
	Kind       string `json:"kind"`
	Window     string `json:"window"`
	Confidence string `json:"confidence"`
}

type redactedFixedEvent struct {
	EventID string `json:"event_id"`
	Window  string `json:"window"`
}

type redactedMedicationFact struct {
	MedicationID             string `json:"medication_id"`
	Active                   bool   `json:"active"`
	ScheduleKind             string `json:"schedule_kind"`
	ScheduledOccurrenceCount int    `json:"scheduled_occurrence_count"`
	CollisionCount           int    `json:"collision_count"`
	NextScheduled            string `json:"next_scheduled,omitempty"`
	LoggedEventCount         int    `json:"logged_event_count"`
	LastLoggedStatus         string `json:"last_logged_status,omitempty"`
	LastWakeRelation         string `json:"last_wake_relation,omitempty"`
	LastSleepRelation        string `json:"last_sleep_relation,omitempty"`
	Confidence               string `json:"confidence,omitempty"`
}

type redactedMarkerFact struct {
	MarkerID       string `json:"marker_id"`
	Kind           string `json:"kind"`
	CivilStartDate string `json:"civil_start_date"`
	CivilEndDate   string `json:"civil_end_date,omitempty"`
}

func buildRedactedContext(input PlanningContext, compact bool) (json.RawMessage, error) {
	limitTasks := maxContextTasks
	limitAvailability := maxContextAvailability
	limitEvents := maxContextFixedEvents
	if compact {
		limitTasks = 3
		limitAvailability = 3
		limitEvents = 2
	}
	zoneID := normalizedContextZone(input.ZoneID)
	ctx := redactedContext{
		Now:    civil(input.Now, zoneID),
		ZoneID: zoneID,
		Policy: []string{
			"Return strict JSON only.",
			"Create proposals only; never apply schedule changes.",
			"Use civil-time ranges and confidence.",
			"No diagnosis, dosing, treatment timing, or exact phase claims.",
		},
	}
	if input.EstimateID != "" && contextIdentifierPattern.MatchString(input.EstimateID) {
		ctx.EstimateID = input.EstimateID
	}
	for _, task := range input.Tasks {
		if len(ctx.Tasks) >= limitTasks {
			break
		}
		if item, ok := sanitizeTaskContext(task, zoneID); ok {
			ctx.Tasks = append(ctx.Tasks, item)
		}
	}
	for _, availability := range input.Availability {
		if len(ctx.Availability) >= limitAvailability {
			break
		}
		if item, ok := sanitizeAvailabilityContext(availability, zoneID); ok {
			ctx.Availability = append(ctx.Availability, item)
		}
	}
	for _, event := range input.FixedEvents {
		if len(ctx.FixedEvents) >= limitEvents {
			break
		}
		if item, ok := sanitizeFixedEventContext(event, zoneID); ok {
			ctx.FixedEvents = append(ctx.FixedEvents, item)
		}
	}
	data, err := json.Marshal(ctx)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func sanitizeTaskContext(input TaskContext, zoneID string) (redactedTask, bool) {
	if !validTaskContext(input) {
		return redactedTask{}, false
	}
	item := redactedTask{
		TaskID: input.TaskID, DurationMinutes: input.DurationMinutes,
		MinimumConfidence:         allowedValue(input.MinimumConfidence, "", "low", "medium", "high"),
		PreferredAfterWakeMinutes: input.PreferredAfterWakeMinutes,
	}
	if input.EarliestStartAt != nil {
		item.Earliest = civil(*input.EarliestStartAt, zoneID)
	}
	if input.LatestFinishAt != nil {
		item.Latest = civil(*input.LatestFinishAt, zoneID)
	}
	return item, true
}

func sanitizeAvailabilityContext(input AvailabilityContext, fallbackZone string) (redactedAvailability, bool) {
	zoneID := input.ZoneID
	if zoneID == "" {
		zoneID = fallbackZone
	}
	if !validAvailabilityContext(input, fallbackZone) {
		return redactedAvailability{}, false
	}
	return redactedAvailability{
		Kind: input.Kind, Window: civilRange(input.StartAt, input.EndAt, zoneID),
		Confidence: input.Confidence,
	}, true
}

func sanitizeFixedEventContext(input FixedEventContext, fallbackZone string) (redactedFixedEvent, bool) {
	zoneID := input.ZoneID
	if zoneID == "" {
		zoneID = fallbackZone
	}
	if !validFixedEventContext(input, fallbackZone) {
		return redactedFixedEvent{}, false
	}
	return redactedFixedEvent{EventID: input.EventID, Window: civilRange(input.StartAt, input.EndAt, zoneID)}, true
}

func validatePlanningContext(input PlanningContext) error {
	if !validContextZone(input.ZoneID) || input.Now.IsZero() {
		return fmt.Errorf("planning context requires a valid zone and current time")
	}
	if input.EstimateID != "" && !contextIdentifierPattern.MatchString(input.EstimateID) {
		return fmt.Errorf("planning context estimate id is invalid")
	}
	if len(input.Tasks) > maxContextTasks || len(input.Availability) > maxContextAvailability || len(input.FixedEvents) > maxContextFixedEvents {
		return fmt.Errorf("planning context exceeds the allowed item count")
	}
	for _, task := range input.Tasks {
		if !validTaskContext(task) {
			return fmt.Errorf("planning context contains an invalid task")
		}
	}
	for _, availability := range input.Availability {
		if !validAvailabilityContext(availability, input.ZoneID) {
			return fmt.Errorf("planning context contains an invalid availability window")
		}
	}
	for _, event := range input.FixedEvents {
		if !validFixedEventContext(event, input.ZoneID) {
			return fmt.Errorf("planning context contains an invalid fixed event")
		}
	}
	return nil
}

func validTaskContext(input TaskContext) bool {
	if !contextIdentifierPattern.MatchString(input.TaskID) || input.DurationMinutes < 1 || input.DurationMinutes > 1440 {
		return false
	}
	if input.PreferredAfterWakeMinutes != nil && (*input.PreferredAfterWakeMinutes < 0 || *input.PreferredAfterWakeMinutes > 1440) {
		return false
	}
	if allowedValue(input.MinimumConfidence, "", "low", "medium", "high") != input.MinimumConfidence {
		return false
	}
	if (input.EarliestStartAt != nil && input.EarliestStartAt.IsZero()) || (input.LatestFinishAt != nil && input.LatestFinishAt.IsZero()) {
		return false
	}
	if input.EarliestStartAt != nil && input.LatestFinishAt != nil && !input.EarliestStartAt.Before(*input.LatestFinishAt) {
		return false
	}
	if (input.BusinessStartLocal != "" && !contextCivilTimePattern.MatchString(input.BusinessStartLocal)) || (input.BusinessEndLocal != "" && !contextCivilTimePattern.MatchString(input.BusinessEndLocal)) {
		return false
	}
	return true
}

func validAvailabilityContext(input AvailabilityContext, fallbackZone string) bool {
	zoneID := input.ZoneID
	if zoneID == "" {
		zoneID = fallbackZone
	}
	return allowedValue(input.Kind, "predicted_wake", "predicted_sleep", "functional", "free") != "" &&
		allowedValue(input.Confidence, "low", "medium", "high") != "" &&
		validContextZone(zoneID) && !input.StartAt.IsZero() && input.StartAt.Before(input.EndAt)
}

func validFixedEventContext(input FixedEventContext, fallbackZone string) bool {
	zoneID := input.ZoneID
	if zoneID == "" {
		zoneID = fallbackZone
	}
	return contextIdentifierPattern.MatchString(input.EventID) && validContextZone(zoneID) &&
		!input.StartAt.IsZero() && input.StartAt.Before(input.EndAt)
}

func normalizedContextZone(value string) string {
	if validContextZone(value) {
		return value
	}
	return "UTC"
}

func sanitizedMedicationFacts(input []MedicationFactContext, limit int) []redactedMedicationFact {
	result := make([]redactedMedicationFact, 0, min(len(input), limit))
	for _, medication := range input {
		if len(result) >= limit {
			break
		}
		if fact, ok := sanitizeMedicationFact(medication); ok {
			result = append(result, fact)
		}
	}
	return result
}

func sanitizedMarkerFacts(input []RhythmMarkerFactContext, limit int) []redactedMarkerFact {
	result := make([]redactedMarkerFact, 0, min(len(input), limit))
	for _, marker := range input {
		if len(result) >= limit {
			break
		}
		if fact, ok := sanitizeMarkerFact(marker); ok {
			result = append(result, fact)
		}
	}
	return result
}

func civilRange(start, end time.Time, zoneID string) string {
	return fmt.Sprintf("%s to %s", civil(start, zoneID), civil(end, zoneID))
}

func civil(value time.Time, zoneID string) string {
	location := time.UTC
	if zoneID != "" {
		if loaded, err := time.LoadLocation(zoneID); err == nil {
			location = loaded
		}
	}
	return value.In(location).Format("Jan 2, 2006 3:04 PM MST")
}

func sanitizeMedicationFact(input MedicationFactContext) (redactedMedicationFact, bool) {
	if !contextIdentifierPattern.MatchString(input.MedicationID) || !allowedScheduleKind(input.ScheduleKind) {
		return redactedMedicationFact{}, false
	}
	return redactedMedicationFact{
		MedicationID:             input.MedicationID,
		Active:                   input.Active,
		ScheduleKind:             input.ScheduleKind,
		ScheduledOccurrenceCount: boundedFactCount(input.ScheduledOccurrenceCount),
		CollisionCount:           boundedFactCount(input.CollisionCount),
		NextScheduled:            sanitizedNextScheduled(input),
		LoggedEventCount:         boundedFactCount(input.LoggedEventCount),
		LastLoggedStatus:         allowedValue(input.LastLoggedStatus, "taken", "skipped"),
		LastWakeRelation:         sanitizedWakeRelation(input.LastWakeRelation),
		LastSleepRelation:        sanitizedSleepRelation(input.LastSleepRelation),
		Confidence:               allowedValue(input.Confidence, "Low", "Medium", "High", "Unknown"),
	}, true
}

func sanitizeMarkerFact(input RhythmMarkerFactContext) (redactedMarkerFact, bool) {
	if !contextIdentifierPattern.MatchString(input.MarkerID) ||
		allowedValue(input.Kind, "travel", "illness", "disruption", "forced_schedule") == "" ||
		!validCivilDate(input.CivilStartDate) ||
		(input.CivilEndDate != "" && !validCivilDate(input.CivilEndDate)) {
		return redactedMarkerFact{}, false
	}
	if input.CivilEndDate != "" && input.CivilEndDate < input.CivilStartDate {
		return redactedMarkerFact{}, false
	}
	return redactedMarkerFact{
		MarkerID: input.MarkerID, Kind: input.Kind,
		CivilStartDate: input.CivilStartDate, CivilEndDate: input.CivilEndDate,
	}, true
}

func sanitizedNextScheduled(input MedicationFactContext) string {
	if !validCivilDate(input.NextScheduledCivilDate) ||
		!contextCivilTimePattern.MatchString(input.NextScheduledCivilTime) ||
		!validContextZone(input.ScheduleZoneID) {
		return ""
	}
	return input.NextScheduledCivilDate + " " + input.NextScheduledCivilTime + " " + input.ScheduleZoneID
}

func validCivilDate(value string) bool {
	parsed, err := time.Parse(time.DateOnly, value)
	return err == nil && parsed.Format(time.DateOnly) == value
}

func validContextZone(value string) bool {
	if len(value) > 64 || !contextZonePattern.MatchString(value) {
		return false
	}
	_, err := time.LoadLocation(value)
	return err == nil
}

func allowedScheduleKind(value string) bool {
	return allowedValue(value, "none", "as_needed", "fixed_clock", "cycling") != ""
}

func allowedValue(value string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return ""
}

func boundedFactCount(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100000 {
		return 100000
	}
	return value
}

func sanitizedWakeRelation(value string) string {
	switch value {
	case "", "No prior recorded wake", "Inside a recorded sleep interval":
		return value
	}
	const suffix = " after recorded wake"
	if strings.HasSuffix(value, suffix) && contextDurationPattern.MatchString(strings.TrimSuffix(value, suffix)) {
		return value
	}
	return ""
}

func sanitizedSleepRelation(value string) string {
	switch value {
	case "", "No comparable sleep window", "Inside a recorded sleep interval", "Inside a predicted sleep window":
		return value
	}
	for _, suffix := range []string{" before next recorded sleep", " before predicted sleep"} {
		if strings.HasSuffix(value, suffix) && contextDurationPattern.MatchString(strings.TrimSuffix(value, suffix)) {
			return value
		}
	}
	return ""
}

func assistantSystemPrompt() string {
	return strings.Join([]string{
		"You are ZeitBoard's scheduling assistant.",
		"Return only the assistant action JSON.",
		"The server resolves proposals; you never apply changes.",
		"Use plain civil-time language with uncertainty.",
		"Do not provide diagnosis, dosing, treatment timing, or exact phase claims.",
	}, " ")
}

func actionSchemaPrompt() json.RawMessage {
	return json.RawMessage(`{"schema_version":"v1","recommended_action":"answer_only|propose_move_task|propose_place_task|propose_reminder_shift","target":{"task_id":"identifier","earliest_start_at":"optional RFC3339","latest_finish_at":"optional RFC3339","duration_minutes":"optional integer","preferred_after_wake_minutes":"optional integer","reminder_id":"optional identifier"},"answer":"optional plain answer"}`)
}

func min(left, right int) int {
	if right < left {
		return right
	}
	return left
}
