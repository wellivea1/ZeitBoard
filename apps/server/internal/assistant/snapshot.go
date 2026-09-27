package assistant

import (
	"regexp"
	"time"
)

// The desktop's assistant snapshot (ADR-0049, contracts/v1/assistant-
// snapshot.schema.json) in the planning view the chat sends: what the owner is
// doing now, when sleep and waking are likely, the next three days, and what
// awaits their decision. These types hold only that view, and requests are
// decoded strictly, so a snapshot carrying a sleep, task, medication, marker
// or sync section is refused before anything reaches a model. Of what is
// accepted, only enums, counts, opaque ids and civil-time ranges reach the
// model; the snapshot's sentences (refusal messages, fit reasons, freshness
// explanations) stay out.

type SnapshotContext struct {
	SchemaVersion string           `json:"schema_version"`
	ZoneID        string           `json:"zone_id"`
	Now           time.Time        `json:"now"`
	Rhythm        SnapshotRhythm   `json:"rhythm"`
	Plans         SnapshotPlans    `json:"plans"`
	NeedsYou      SnapshotNeedsYou `json:"needs_you"`
	PrivateFields string           `json:"private_fields"`
	Disclaimer    string           `json:"disclaimer"`
}

type SnapshotRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type SnapshotRefusal struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SnapshotRhythm struct {
	Status               string           `json:"status"`
	Refusal              *SnapshotRefusal `json:"refusal,omitempty"`
	Freshness            string           `json:"freshness"`
	FreshnessReason      string           `json:"freshness_reason,omitempty"`
	FreshnessExplanation string           `json:"freshness_explanation,omitempty"`
	Presence             string           `json:"presence"`
	SleepOnset           *SnapshotRange   `json:"sleep_onset,omitempty"`
	Wake                 *SnapshotRange   `json:"wake,omitempty"`
	CycleMinutes         *int             `json:"cycle_minutes,omitempty"`
	DriftMinutesPerCycle *int             `json:"drift_minutes_per_cycle,omitempty"`
	TypicalSleepMinutes  *int             `json:"typical_sleep_minutes,omitempty"`
	FitRating            string           `json:"fit_rating,omitempty"`
	FitReasons           []string         `json:"fit_reasons"`
}

type SnapshotPlans struct {
	Status         string               `json:"status"`
	Horizon        *SnapshotRange       `json:"horizon,omitempty"`
	Segments       []SnapshotSegment    `json:"segments"`
	ReachableHours []SnapshotReachable  `json:"reachable_hours"`
	Commitments    []SnapshotCommitment `json:"commitments"`
	Suggestions    []SnapshotSuggestion `json:"suggestions"`
	Unplaced       []SnapshotUnplaced   `json:"unplaced"`
	Truncated      bool                 `json:"truncated"`
}

type SnapshotSegment struct {
	Presence string    `json:"presence"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Observed bool      `json:"observed"`
}

type SnapshotReachable struct {
	Start     time.Time       `json:"start"`
	End       time.Time       `json:"end"`
	Status    string          `json:"status"`
	Reachable []SnapshotRange `json:"reachable"`
	Possible  []SnapshotRange `json:"possible"`
}

type SnapshotCommitment struct {
	Ref      string    `json:"ref"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	AllDay   bool      `json:"all_day"`
	Conflict string    `json:"conflict"`
	TaskID   string    `json:"task_id,omitempty"`
}

type SnapshotSuggestion struct {
	ProposalID string    `json:"proposal_id"`
	TaskID     string    `json:"task_id"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Confidence string    `json:"confidence"`
	Reasons    []string  `json:"reasons"`
}

type SnapshotUnplaced struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type SnapshotNeedsYou struct {
	Suggestions   int `json:"suggestions"`
	TaskConflicts int `json:"task_conflicts"`
}

// What the model reads of the snapshot.

type redactedSnapshot struct {
	Rhythm           redactedRhythm       `json:"rhythm"`
	Timeline         []redactedSegment    `json:"timeline,omitempty"`
	ReachableHours   []redactedReachable  `json:"reachable_hours,omitempty"`
	Commitments      []redactedCommitment `json:"commitments,omitempty"`
	AwaitingDecision []redactedSuggestion `json:"awaiting_decision,omitempty"`
	Unplaced         []redactedUnplaced   `json:"unplaced,omitempty"`
	NeedsYou         SnapshotNeedsYou     `json:"needs_you"`
}

type redactedRhythm struct {
	Status               string `json:"status"`
	Presence             string `json:"presence"`
	Freshness            string `json:"freshness"`
	FreshnessReason      string `json:"freshness_reason,omitempty"`
	SleepLikelyBegins    string `json:"sleep_likely_begins,omitempty"`
	WakingLikely         string `json:"waking_likely,omitempty"`
	CycleMinutes         *int   `json:"cycle_minutes,omitempty"`
	DriftMinutesPerCycle *int   `json:"drift_minutes_per_cycle,omitempty"`
	TypicalSleepMinutes  *int   `json:"typical_sleep_minutes,omitempty"`
	FitRating            string `json:"fit_rating,omitempty"`
}

type redactedSegment struct {
	Presence string `json:"presence"`
	Window   string `json:"window"`
}

type redactedReachable struct {
	Window      string   `json:"window"`
	Status      string   `json:"status"`
	LikelyAwake []string `json:"likely_awake,omitempty"`
	Uncertain   []string `json:"uncertain,omitempty"`
}

type redactedCommitment struct {
	Ref      string `json:"ref"`
	Window   string `json:"window"`
	AllDay   bool   `json:"all_day,omitempty"`
	Conflict string `json:"conflict"`
	TaskID   string `json:"accepted_time_of_task,omitempty"`
}

type redactedSuggestion struct {
	ProposalID string `json:"proposal_id"`
	TaskID     string `json:"task_id"`
	Window     string `json:"window"`
	Confidence string `json:"confidence"`
}

type redactedUnplaced struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

var commitmentRefPattern = regexp.MustCompile(`^commitment_[0-9]{2,4}$`)

const (
	maxSnapshotSegments    = 48
	maxSnapshotReachable   = 16
	maxSnapshotCommitments = 64
	maxSnapshotSuggestions = 32
	maxSnapshotUnplaced    = 32
	maxSnapshotCount       = 1000
)

// sanitizeSnapshot keeps what the model may read: allowlisted values, ranges
// that make sense near now, and ids in the identifier format. Anything else is
// dropped rather than passed on.
func sanitizeSnapshot(input *SnapshotContext, now time.Time, zoneID string, compact bool) *redactedSnapshot {
	if input == nil {
		return nil
	}
	limit := func(full, small int) int {
		if compact {
			return small
		}
		return full
	}
	window := func(value SnapshotRange) (string, bool) {
		if !value.End.After(value.Start) || value.Start.Before(now.Add(-48*time.Hour)) || value.End.After(now.Add(8*24*time.Hour)) {
			return "", false
		}
		return civilRange(value.Start, value.End, zoneID), true
	}
	bounded := func(value *int, low, high int) *int {
		if value == nil || *value < low || *value > high {
			return nil
		}
		copied := *value
		return &copied
	}
	rhythm := input.Rhythm
	out := &redactedSnapshot{
		Rhythm: redactedRhythm{
			Status:               allowedValue(rhythm.Status, "estimated", "empty", "refused", "unavailable"),
			Presence:             allowedValue(rhythm.Presence, "awake", "asleep", "unknown"),
			Freshness:            allowedValue(rhythm.Freshness, "current", "stale", "withheld"),
			FreshnessReason:      allowedValue(rhythm.FreshnessReason, "no_evidence", "evidence_aging", "evidence_stale", "expected_sleep_unrecorded", "no_sources_reporting"),
			CycleMinutes:         bounded(rhythm.CycleMinutes, 60, 4320),
			DriftMinutesPerCycle: bounded(rhythm.DriftMinutesPerCycle, -1440, 1440),
			TypicalSleepMinutes:  bounded(rhythm.TypicalSleepMinutes, 1, 1440),
			FitRating:            allowedValue(rhythm.FitRating, "low", "medium", "high"),
		},
		NeedsYou: SnapshotNeedsYou{
			Suggestions:   boundedCount(input.NeedsYou.Suggestions),
			TaskConflicts: boundedCount(input.NeedsYou.TaskConflicts),
		},
	}
	// An unreadable value reads as the least trusting one.
	if out.Rhythm.Status == "" {
		out.Rhythm.Status = "unavailable"
	}
	if out.Rhythm.Freshness == "" {
		out.Rhythm.Freshness = "withheld"
	}
	// Home states what is happening now only on current evidence.
	if out.Rhythm.Presence == "" || out.Rhythm.Freshness != "current" {
		out.Rhythm.Presence = "unknown"
	}
	if rhythm.SleepOnset != nil {
		out.Rhythm.SleepLikelyBegins, _ = window(*rhythm.SleepOnset)
	}
	if rhythm.Wake != nil {
		out.Rhythm.WakingLikely, _ = window(*rhythm.Wake)
	}
	plans := input.Plans
	for _, segment := range plans.Segments {
		if len(out.Timeline) == limit(maxSnapshotSegments, 6) {
			break
		}
		presence := allowedValue(segment.Presence, "awake", "asleep", "uncertain", "unknown")
		if span, ok := window(SnapshotRange{Start: segment.Start, End: segment.End}); ok && presence != "" {
			out.Timeline = append(out.Timeline, redactedSegment{Presence: presence, Window: span})
		}
	}
	for _, reachable := range plans.ReachableHours {
		if len(out.ReachableHours) == limit(maxSnapshotReachable, 2) {
			break
		}
		status := allowedValue(reachable.Status, "reachable", "partial", "unreachable")
		span, ok := window(SnapshotRange{Start: reachable.Start, End: reachable.End})
		if !ok || status == "" {
			continue
		}
		entry := redactedReachable{Window: span, Status: status}
		for _, part := range reachable.Reachable {
			if text, ok := window(part); ok {
				entry.LikelyAwake = append(entry.LikelyAwake, text)
			}
		}
		for _, part := range reachable.Possible {
			if text, ok := window(part); ok {
				entry.Uncertain = append(entry.Uncertain, text)
			}
		}
		out.ReachableHours = append(out.ReachableHours, entry)
	}
	for _, commitment := range plans.Commitments {
		if len(out.Commitments) == limit(maxSnapshotCommitments, 3) {
			break
		}
		conflict := allowedValue(commitment.Conflict, "none", "inside_predicted_sleep", "overlaps_predicted_sleep", "near_uncertain_boundary")
		span, ok := window(SnapshotRange{Start: commitment.Start, End: commitment.End})
		if !ok || conflict == "" || !commitmentRefPattern.MatchString(commitment.Ref) ||
			(commitment.TaskID != "" && !contextIdentifierPattern.MatchString(commitment.TaskID)) {
			continue
		}
		out.Commitments = append(out.Commitments, redactedCommitment{
			Ref: commitment.Ref, Window: span, AllDay: commitment.AllDay, Conflict: conflict, TaskID: commitment.TaskID,
		})
	}
	for _, suggestion := range plans.Suggestions {
		if len(out.AwaitingDecision) == limit(maxSnapshotSuggestions, 2) {
			break
		}
		confidence := allowedValue(suggestion.Confidence, "low", "medium", "high")
		span, ok := window(SnapshotRange{Start: suggestion.Start, End: suggestion.End})
		if !ok || confidence == "" || !contextIdentifierPattern.MatchString(suggestion.ProposalID) ||
			!contextIdentifierPattern.MatchString(suggestion.TaskID) {
			continue
		}
		out.AwaitingDecision = append(out.AwaitingDecision, redactedSuggestion{
			ProposalID: suggestion.ProposalID, TaskID: suggestion.TaskID, Window: span, Confidence: confidence,
		})
	}
	for _, unplaced := range plans.Unplaced {
		if len(out.Unplaced) == limit(maxSnapshotUnplaced, 2) {
			break
		}
		reason := allowedValue(unplaced.Reason, "no_available_interval", "outside_forecast_horizon", "estimate_unavailable", "invalid_constraints")
		if reason == "" || !contextIdentifierPattern.MatchString(unplaced.TaskID) {
			continue
		}
		out.Unplaced = append(out.Unplaced, redactedUnplaced{TaskID: unplaced.TaskID, Reason: reason})
	}
	return out
}

func boundedCount(value int) int {
	switch {
	case value < 0:
		return 0
	case value > maxSnapshotCount:
		return maxSnapshotCount
	default:
		return value
	}
}
