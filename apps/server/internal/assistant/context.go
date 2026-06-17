package assistant

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
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
	TaskID            string `json:"task_id"`
	DurationMinutes   int    `json:"duration_minutes"`
	Earliest          string `json:"earliest,omitempty"`
	Latest            string `json:"latest,omitempty"`
	MinimumConfidence string `json:"minimum_confidence,omitempty"`
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

func buildRedactedContext(input PlanningContext, compact bool) (json.RawMessage, error) {
	limitTasks := len(input.Tasks)
	limitAvailability := len(input.Availability)
	limitEvents := len(input.FixedEvents)
	if compact {
		limitTasks = min(limitTasks, 3)
		limitAvailability = min(limitAvailability, 3)
		limitEvents = min(limitEvents, 2)
	}
	ctx := redactedContext{
		Now:        civil(input.Now, input.ZoneID),
		ZoneID:     input.ZoneID,
		EstimateID: input.EstimateID,
		Policy: []string{
			"Return strict JSON only.",
			"Create proposals only; never apply schedule changes.",
			"Use civil-time ranges and confidence.",
			"No diagnosis, dosing, treatment timing, or exact phase claims.",
		},
	}
	for _, task := range input.Tasks[:limitTasks] {
		item := redactedTask{
			TaskID:            task.TaskID,
			DurationMinutes:   task.DurationMinutes,
			MinimumConfidence: task.MinimumConfidence,
		}
		if task.EarliestStartAt != nil {
			item.Earliest = civil(*task.EarliestStartAt, input.ZoneID)
		}
		if task.LatestFinishAt != nil {
			item.Latest = civil(*task.LatestFinishAt, input.ZoneID)
		}
		ctx.Tasks = append(ctx.Tasks, item)
	}
	for _, availability := range input.Availability[:limitAvailability] {
		zone := availability.ZoneID
		if zone == "" {
			zone = input.ZoneID
		}
		ctx.Availability = append(ctx.Availability, redactedAvailability{
			Kind:       availability.Kind,
			Window:     civilRange(availability.StartAt, availability.EndAt, zone),
			Confidence: availability.Confidence,
		})
	}
	for _, event := range input.FixedEvents[:limitEvents] {
		zone := event.ZoneID
		if zone == "" {
			zone = input.ZoneID
		}
		ctx.FixedEvents = append(ctx.FixedEvents, redactedFixedEvent{
			EventID: event.EventID,
			Window:  civilRange(event.StartAt, event.EndAt, zone),
		})
	}
	data, err := json.Marshal(ctx)
	if err != nil {
		return nil, err
	}
	return data, nil
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
