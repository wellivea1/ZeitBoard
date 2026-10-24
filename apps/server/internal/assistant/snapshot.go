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

// SnapshotReachable is one window when someone the owner needs is available.
// Contact names that someone by place in the owner's list (ADR-0056); a
// snapshot from before there was a list has none.
type SnapshotReachable struct {
	Contact   string          `json:"contact,omitempty"`
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
	Contact     string   `json:"contact,omitempty"`
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

var (
	commitmentRefPattern = regexp.MustCompile(`^commitment_[0-9]{2,4}$`)
	contactRefPattern    = regexp.MustCompile(`^contact_[0-9]{2}$`)
)

const (
	maxSnapshotSegments    = 48
	maxSnapshotReachable   = 24
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
	keep := snapshotSanitizer{now: now, zoneID: zoneID, compact: compact}
	plans := input.Plans
	return &redactedSnapshot{
		Rhythm: keep.rhythm(input.Rhythm),
		NeedsYou: SnapshotNeedsYou{
			Suggestions:   boundedCount(input.NeedsYou.Suggestions),
			TaskConflicts: boundedCount(input.NeedsYou.TaskConflicts),
		},
		Timeline:         keep.timeline(plans.Segments),
		ReachableHours:   keep.reachableHours(plans.ReachableHours),
		Commitments:      keep.commitments(plans.Commitments),
		AwaitingDecision: keep.suggestions(plans.Suggestions),
		Unplaced:         keep.unplaced(plans.Unplaced),
	}
}

// snapshotSanitizer keeps each part of a snapshot that the model may read,
// near now and in the owner's zone, fewer of each when compact.
type snapshotSanitizer struct {
	now     time.Time
	zoneID  string
	compact bool
}

// limit is how many of a kind are kept: full, or small when compact.
func (k snapshotSanitizer) limit(full, small int) int {
	if k.compact {
		return small
	}
	return full
}

// window states a range in civil time, when it is a range at all and lies
// within two days before now and eight days after.
func (k snapshotSanitizer) window(value SnapshotRange) (string, bool) {
	if !value.End.After(value.Start) || value.Start.Before(k.now.Add(-48*time.Hour)) || value.End.After(k.now.Add(8*24*time.Hour)) {
		return "", false
	}
	return civilRange(value.Start, value.End, k.zoneID), true
}

func boundedValue(value *int, low, high int) *int {
	if value == nil || *value < low || *value > high {
		return nil
	}
	copied := *value
	return &copied
}

// rhythm keeps the rhythm's allowlisted states and bounded numbers. An
// unreadable value reads as the least trusting one, and what is happening
// now is said only on current evidence, as Home says it.
func (k snapshotSanitizer) rhythm(rhythm SnapshotRhythm) redactedRhythm {
	out := redactedRhythm{
		Status:               allowedValue(rhythm.Status, "estimated", "empty", "refused", "unavailable"),
		Presence:             allowedValue(rhythm.Presence, "awake", "asleep", "unknown"),
		Freshness:            allowedValue(rhythm.Freshness, "current", "stale", "withheld"),
		FreshnessReason:      allowedValue(rhythm.FreshnessReason, "no_evidence", "evidence_aging", "evidence_stale", "expected_sleep_unrecorded", "no_sources_reporting"),
		CycleMinutes:         boundedValue(rhythm.CycleMinutes, 60, 4320),
		DriftMinutesPerCycle: boundedValue(rhythm.DriftMinutesPerCycle, -1440, 1440),
		TypicalSleepMinutes:  boundedValue(rhythm.TypicalSleepMinutes, 1, 1440),
		FitRating:            allowedValue(rhythm.FitRating, "low", "medium", "high"),
	}
	if out.Status == "" {
		out.Status = "unavailable"
	}
	if out.Freshness == "" {
		out.Freshness = "withheld"
	}
	if out.Presence == "" || out.Freshness != "current" {
		out.Presence = "unknown"
	}
	if rhythm.SleepOnset != nil {
		out.SleepLikelyBegins, _ = k.window(*rhythm.SleepOnset)
	}
	if rhythm.Wake != nil {
		out.WakingLikely, _ = k.window(*rhythm.Wake)
	}
	return out
}

func (k snapshotSanitizer) timeline(segments []SnapshotSegment) []redactedSegment {
	var out []redactedSegment
	for _, segment := range segments {
		if len(out) == k.limit(maxSnapshotSegments, 6) {
			break
		}
		presence := allowedValue(segment.Presence, "awake", "asleep", "uncertain", "unknown")
		if span, ok := k.window(SnapshotRange{Start: segment.Start, End: segment.End}); ok && presence != "" {
			out = append(out, redactedSegment{Presence: presence, Window: span})
		}
	}
	return out
}

// reachableHours keeps each contact's hours by its opaque reference.
func (k snapshotSanitizer) reachableHours(hours []SnapshotReachable) []redactedReachable {
	var out []redactedReachable
	for _, reachable := range hours {
		if len(out) == k.limit(maxSnapshotReachable, 2) {
			break
		}
		status := allowedValue(reachable.Status, "reachable", "partial", "unreachable")
		span, ok := k.window(SnapshotRange{Start: reachable.Start, End: reachable.End})
		if !ok || status == "" || (reachable.Contact != "" && !contactRefPattern.MatchString(reachable.Contact)) {
			continue
		}
		entry := redactedReachable{Contact: reachable.Contact, Window: span, Status: status}
		entry.LikelyAwake = k.windows(reachable.Reachable)
		entry.Uncertain = k.windows(reachable.Possible)
		out = append(out, entry)
	}
	return out
}

// windows states each range that window keeps.
func (k snapshotSanitizer) windows(ranges []SnapshotRange) []string {
	var out []string
	for _, part := range ranges {
		if text, ok := k.window(part); ok {
			out = append(out, text)
		}
	}
	return out
}

// commitments keeps each commitment by its opaque reference: never a title.
func (k snapshotSanitizer) commitments(commitments []SnapshotCommitment) []redactedCommitment {
	var out []redactedCommitment
	for _, commitment := range commitments {
		if len(out) == k.limit(maxSnapshotCommitments, 3) {
			break
		}
		conflict := allowedValue(commitment.Conflict, "none", "inside_predicted_sleep", "overlaps_predicted_sleep", "near_uncertain_boundary")
		span, ok := k.window(SnapshotRange{Start: commitment.Start, End: commitment.End})
		if !ok || conflict == "" || !commitmentRefPattern.MatchString(commitment.Ref) ||
			(commitment.TaskID != "" && !contextIdentifierPattern.MatchString(commitment.TaskID)) {
			continue
		}
		out = append(out, redactedCommitment{
			Ref: commitment.Ref, Window: span, AllDay: commitment.AllDay, Conflict: conflict, TaskID: commitment.TaskID,
		})
	}
	return out
}

// suggestions keeps the suggested times awaiting the owner, by id.
func (k snapshotSanitizer) suggestions(suggestions []SnapshotSuggestion) []redactedSuggestion {
	var out []redactedSuggestion
	for _, suggestion := range suggestions {
		if len(out) == k.limit(maxSnapshotSuggestions, 2) {
			break
		}
		confidence := allowedValue(suggestion.Confidence, "low", "medium", "high")
		span, ok := k.window(SnapshotRange{Start: suggestion.Start, End: suggestion.End})
		if !ok || confidence == "" || !contextIdentifierPattern.MatchString(suggestion.ProposalID) ||
			!contextIdentifierPattern.MatchString(suggestion.TaskID) {
			continue
		}
		out = append(out, redactedSuggestion{
			ProposalID: suggestion.ProposalID, TaskID: suggestion.TaskID, Window: span, Confidence: confidence,
		})
	}
	return out
}

// unplaced keeps the tasks no time was found for, by id and reason.
func (k snapshotSanitizer) unplaced(unplaced []SnapshotUnplaced) []redactedUnplaced {
	var out []redactedUnplaced
	for _, item := range unplaced {
		if len(out) == k.limit(maxSnapshotUnplaced, 2) {
			break
		}
		reason := allowedValue(item.Reason, "no_available_interval", "outside_forecast_horizon", "estimate_unavailable", "invalid_constraints")
		if reason == "" || !contextIdentifierPattern.MatchString(item.TaskID) {
			continue
		}
		out = append(out, redactedUnplaced{TaskID: item.TaskID, Reason: reason})
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
