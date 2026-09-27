package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	calendarcore "non24.app/core/calendar"
	"non24.app/core/domain"
	"non24.app/core/freshness"
	"non24.app/core/outlook"
	storage "non24.app/core/storage/sqlite"
)

// The assistant snapshot (ADR-0049, contracts/v1/assistant-snapshot.schema.json)
// coalesces what an assistant may know about the owner's day into one
// versioned document: the rhythm estimate and whether it may be trusted now,
// recent sleep, the next three days (when sleep and waking are likely,
// reachable hours, commitments, suggested times and what could not be placed),
// what awaits the owner, tasks, medication timing, context markers and sync
// state. It is assembled from the computations the Home screen draws — the
// local estimate and its freshness verdict, the operational outlook, the local
// planner, and the medication and marker projections — so an assistant and the
// screen cannot disagree.
//
// It carries no title, label, note, location, clinician text or raw record,
// and, like the other agent projections, no exact time of recorded evidence:
// sleep is stated as Home states it (awake for about so long, last night's
// date and length), doses and markers as the medication and marker
// projections state them. Task, proposal and medication ids are the app's
// opaque ids; calendar entries are numbered within the snapshot, because an
// imported entry's own id can carry its source's text. Instants are in the
// owner's zone, to the minute. Each surface takes the sections its policy
// allows: the local agent endpoint reads all of them, and the chat assistant
// sends its server only the planning view (forChat).

const (
	assistantSnapshotVersion = "v1"
	maxSnapshotSegments      = 48
	maxSnapshotReachable     = 16
	maxSnapshotCommitments   = 64
	maxSnapshotSuggestions   = 32
	maxSnapshotUnplaced      = 32

	snapshotPrivateFields = "Titles, labels, notes, locations, clinician text and raw records are never included. " +
		"Task, proposal and medication ids are opaque; calendar entries are numbered within this snapshot."
	snapshotFitNote = "The fit rating says how closely the model matches recent records; it has not ranked " +
		"forecasts reliably, so go by the ranges and the age of the records."
)

type assistantSnapshot struct {
	SchemaVersion string              `json:"schema_version"`
	ZoneID        string              `json:"zone_id"`
	Now           time.Time           `json:"now"`
	Rhythm        snapshotRhythm      `json:"rhythm"`
	Sleep         *snapshotSleep      `json:"sleep,omitempty"`
	Plans         snapshotPlans       `json:"plans"`
	NeedsYou      snapshotNeedsYou    `json:"needs_you"`
	Tasks         *snapshotTasks      `json:"tasks,omitempty"`
	Medication    *snapshotMedication `json:"medication,omitempty"`
	Markers       *snapshotMarkers    `json:"markers,omitempty"`
	Sync          *snapshotSync       `json:"sync,omitempty"`
	PrivateFields string              `json:"private_fields"`
	Disclaimer    string              `json:"disclaimer"`
}

type snapshotRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// snapshotRhythm is the estimate as Home states it: what is happening now,
// when sleep is likely to begin and waking likely to follow, and the fitted
// rhythm behind them.
type snapshotRhythm struct {
	Status               string           `json:"status"`
	Refusal              *agentRefusalDTO `json:"refusal,omitempty"`
	Freshness            string           `json:"freshness"`
	FreshnessReason      string           `json:"freshness_reason,omitempty"`
	FreshnessExplanation string           `json:"freshness_explanation,omitempty"`
	Presence             string           `json:"presence"`
	AwakeForMinutes      *int             `json:"awake_for_minutes,omitempty"`
	SleepOnset           *snapshotRange   `json:"sleep_onset,omitempty"`
	Wake                 *snapshotRange   `json:"wake,omitempty"`
	CycleMinutes         *int             `json:"cycle_minutes,omitempty"`
	DriftMinutesPerCycle *int             `json:"drift_minutes_per_cycle,omitempty"`
	TypicalSleepMinutes  *int             `json:"typical_sleep_minutes,omitempty"`
	FitRating            string           `json:"fit_rating,omitempty"`
	FitReasons           []string         `json:"fit_reasons"`
}

type snapshotSleep struct {
	NightsOnRecord int                   `json:"nights_on_record"`
	Last           *snapshotSleepEpisode `json:"last,omitempty"`
	// GoingToSleepMarked is a "going to sleep" tap still waiting for its wake.
	GoingToSleepMarked bool `json:"going_to_sleep_marked"`
}

// snapshotSleepEpisode is the latest main sleep as Home lists it: the civil
// date it ended on and its length, to five minutes.
type snapshotSleepEpisode struct {
	EndedOn         string `json:"ended_on"`
	DurationMinutes int    `json:"duration_minutes"`
	Source          string `json:"source"`
}

// snapshotPlans is the next three days. Segments, reachable hours and
// commitments come from the operational outlook; suggestions and unplaced
// tasks from the planner behind Home's "Needs you".
type snapshotPlans struct {
	Status         string               `json:"status"`
	Horizon        *snapshotRange       `json:"horizon,omitempty"`
	Segments       []snapshotSegment    `json:"segments"`
	ReachableHours []snapshotReachable  `json:"reachable_hours"`
	Commitments    []snapshotCommitment `json:"commitments"`
	Suggestions    []snapshotSuggestion `json:"suggestions"`
	Unplaced       []snapshotUnplaced   `json:"unplaced"`
	Truncated      bool                 `json:"truncated"`
}

type snapshotSegment struct {
	Presence string    `json:"presence"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Observed bool      `json:"observed"`
}

// snapshotReachable is one span of the owner's reaching hours: when the
// people they need are open, and the part of it the owner is likely awake for.
type snapshotReachable struct {
	Start     time.Time       `json:"start"`
	End       time.Time       `json:"end"`
	Status    string          `json:"status"`
	Reachable []snapshotRange `json:"reachable"`
	Possible  []snapshotRange `json:"possible"`
}

type snapshotCommitment struct {
	Ref      string    `json:"ref"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	AllDay   bool      `json:"all_day"`
	Conflict string    `json:"conflict"`
	// TaskID names the task when this block is its accepted time.
	TaskID string `json:"task_id,omitempty"`
}

type snapshotSuggestion struct {
	ProposalID string    `json:"proposal_id"`
	TaskID     string    `json:"task_id"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Confidence string    `json:"confidence"`
	Reasons    []string  `json:"reasons"`
}

type snapshotUnplaced struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type snapshotNeedsYou struct {
	Suggestions   int `json:"suggestions"`
	TaskConflicts int `json:"task_conflicts"`
}

type snapshotTasks struct {
	Count     int            `json:"count"`
	Truncated bool           `json:"truncated"`
	Items     []agentTaskDTO `json:"items"`
}

type snapshotMedication struct {
	Status    string `json:"status"`
	Count     int    `json:"count"`
	Truncated bool   `json:"truncated"`
	// PendingDoseProposals counts doses an agent proposed that wait for the
	// owner (ADR-0051).
	PendingDoseProposals int                  `json:"pending_dose_proposals"`
	Items                []agentMedicationDTO `json:"items"`
}

type snapshotMarkers struct {
	Status    string           `json:"status"`
	Count     int              `json:"count"`
	Truncated bool             `json:"truncated"`
	Items     []agentMarkerDTO `json:"items"`
}

type snapshotSync struct {
	Enabled         bool       `json:"enabled"`
	Status          string     `json:"status"`
	LastSyncAt      *time.Time `json:"last_sync_at,omitempty"`
	PendingUploads  int        `json:"pending_uploads"`
	PendingErasures int        `json:"pending_erasures"`
	WaitingRecords  int        `json:"waiting_records"`
}

// assistantSnapshot builds the whole snapshot at now.
func (a *App) assistantSnapshot(ctx context.Context, now time.Time) (assistantSnapshot, error) {
	now = now.UTC().Truncate(time.Minute)
	store, err := a.requireStore()
	if err != nil {
		return assistantSnapshot{}, localAgentProjectionError("snapshot", err)
	}
	computed, err := a.computeOutlook(ctx, now)
	if err != nil {
		return assistantSnapshot{}, localAgentProjectionError("snapshot", err)
	}
	proposals, err := a.buildLocalProposals(now)
	if err != nil {
		return assistantSnapshot{}, localAgentProjectionError("snapshot", err)
	}
	tasks, err := a.agentTaskProjection(ctx)
	if err != nil {
		return assistantSnapshot{}, err
	}
	medication, err := a.agentMedicationProjection(ctx)
	if err != nil {
		return assistantSnapshot{}, err
	}
	markers, err := a.agentMarkerProjection(ctx)
	if err != nil {
		return assistantSnapshot{}, err
	}
	pending, err := store.PendingSleep(ctx)
	if err != nil {
		return assistantSnapshot{}, localAgentProjectionError("snapshot", err)
	}
	doseProposals, err := store.DoseProposals(ctx, now)
	if err != nil {
		return assistantSnapshot{}, localAgentProjectionError("snapshot", err)
	}
	waitingDoses := 0
	for _, proposal := range doseProposals {
		if proposal.State == storage.DoseProposalPending {
			waitingDoses++
		}
	}
	cfg, err := a.loadBackendSyncConfig()
	if err != nil {
		return assistantSnapshot{}, localAgentProjectionError("snapshot", err)
	}
	syncStatus := a.backendSyncStatusCounts(cfg, syncCounts{})

	clock := snapshotClock{location: loadLocationOrUTC(computed.zoneID)}
	snapshot := assistantSnapshot{
		SchemaVersion: assistantSnapshotVersion,
		ZoneID:        computed.zoneID,
		Now:           clock.at(now),
		Rhythm:        snapshotRhythmOf(computed, now, clock),
		Sleep:         snapshotSleepOf(computed.state, pending != nil, clock),
		Plans:         snapshotPlansOf(computed, proposals, clock),
		NeedsYou:      snapshotNeedsYou{Suggestions: len(proposals.pending), TaskConflicts: syncStatus.TaskConflictCount},
		Tasks:         &snapshotTasks{Count: tasks.Count, Truncated: tasks.Truncated, Items: tasks.Tasks},
		Medication: &snapshotMedication{
			Status: medication.Status, Count: medication.MedicationCount,
			Truncated: medication.Truncated, PendingDoseProposals: waitingDoses, Items: medication.Medications,
		},
		Markers: &snapshotMarkers{Status: markers.Status, Count: markers.Count, Truncated: markers.Truncated, Items: markers.Markers},
		Sync: &snapshotSync{
			Enabled: syncStatus.Enabled, Status: syncStatus.Status,
			PendingUploads: syncStatus.PendingPushCount, PendingErasures: syncStatus.PendingErasureCount,
			WaitingRecords: syncStatus.WaitingRecordCount,
		},
		PrivateFields: snapshotPrivateFields,
		Disclaimer:    disclaimer + " " + snapshotFitNote,
	}
	if !cfg.LastSyncAt.IsZero() {
		last := clock.at(cfg.LastSyncAt)
		snapshot.Sync.LastSyncAt = &last
	}
	return snapshot, nil
}

// forChat is the planning view the chat assistant sends its server, whose
// model must never see a sleep record (ADR-0010): no sleep section and no
// time since the recorded wake. Tasks already travel as the planner's inputs;
// medication and markers are answered from templates and never reach a
// model; sync state is no planning input.
func (s assistantSnapshot) forChat() assistantSnapshot {
	s.Sleep, s.Tasks, s.Medication, s.Markers, s.Sync = nil, nil, nil, nil, nil
	s.Rhythm.AwakeForMinutes = nil
	return s
}

// snapshotClock renders instants in the owner's zone, to the minute.
type snapshotClock struct{ location *time.Location }

func (c snapshotClock) at(value time.Time) time.Time {
	return value.Truncate(time.Minute).In(c.location)
}

func (c snapshotClock) span(value domain.TimeRange) snapshotRange {
	return snapshotRange{Start: c.at(value.Start.UTC), End: c.at(value.End.UTC)}
}

func snapshotRhythmOf(computed outlookComputation, now time.Time, clock snapshotClock) snapshotRhythm {
	state, assessment := computed.state, computed.view.Freshness
	rhythm := snapshotRhythm{
		Status:          state.Status,
		Freshness:       string(assessment.State),
		FreshnessReason: string(assessment.Reason),
		Presence:        presenceUnknown,
		FitReasons:      []string{},
	}
	// Home states what is happening now only on current evidence; otherwise
	// its lead gives this explanation instead, and so does the snapshot.
	trusted := assessment.State == freshness.StateCurrent
	if !trusted {
		rhythm.FreshnessExplanation = assessment.Explanation
	}
	if state.Status != "estimated" {
		if refusal := refusalDTO(state.Refusal, state.Message); refusal != nil {
			rhythm.Refusal = &agentRefusalDTO{Code: refusal.Code, Message: refusal.Message}
		}
		return rhythm
	}
	if latest, ok := latestPrincipalSession(state.Sessions); ok && trusted {
		rhythm.Presence = presenceNow(latest, assessment, now)
		if wake := latest.Intervals[0].Interval.End.UTC; rhythm.Presence == presenceAwake && now.After(wake) {
			awake := roundedMinutes(now.Sub(wake), 5)
			rhythm.AwakeForMinutes = &awake
		}
	}
	onset, wake := computed.view.SleepAhead()
	if onset != nil {
		value := clock.span(*onset)
		rhythm.SleepOnset = &value
	}
	if wake != nil {
		value := clock.span(*wake)
		rhythm.Wake = &value
	}
	estimate := state.Estimate
	minutes := func(value time.Duration) *int {
		rounded := int(math.Round(value.Minutes()))
		return &rounded
	}
	if estimate.ObservedCycleLength > 0 {
		rhythm.CycleMinutes = minutes(estimate.ObservedCycleLength)
		rhythm.DriftMinutesPerCycle = minutes(estimate.ObservedDriftPerCycle)
	}
	if estimate.TypicalSleepDuration > 0 {
		rhythm.TypicalSleepMinutes = minutes(estimate.TypicalSleepDuration)
	}
	rhythm.FitRating = string(estimate.Confidence.Level)
	rhythm.FitReasons = append(rhythm.FitReasons, estimate.Confidence.Reasons...)
	return rhythm
}

func snapshotSleepOf(state localEstimateState, goingToSleepMarked bool, clock snapshotClock) *snapshotSleep {
	sleep := &snapshotSleep{GoingToSleepMarked: goingToSleepMarked}
	for _, session := range state.Sessions {
		if session.IsPrincipalSleep() && len(session.Intervals) > 0 {
			sleep.NightsOnRecord++
		}
	}
	if latest, ok := latestPrincipalSession(state.Sessions); ok {
		interval := latest.Intervals[0].Interval
		sleep.Last = &snapshotSleepEpisode{
			EndedOn:         clock.at(interval.End.UTC).Format(time.DateOnly),
			DurationMinutes: roundedMinutes(interval.End.UTC.Sub(interval.Start.UTC), 5),
			Source:          latest.SourceLabel,
		}
	}
	return sleep
}

// roundedMinutes rounds a duration to the nearest step of minutes.
func roundedMinutes(value time.Duration, step int) int {
	return int(math.Round(value.Minutes()/float64(step))) * step
}

func snapshotPlansOf(computed outlookComputation, proposals localProposalBuild, clock snapshotClock) snapshotPlans {
	view := computed.view
	plans := snapshotPlans{
		Status:         string(view.Status),
		Segments:       []snapshotSegment{},
		ReachableHours: []snapshotReachable{},
		Commitments:    []snapshotCommitment{},
		Suggestions:    []snapshotSuggestion{},
		Unplaced:       []snapshotUnplaced{},
	}
	if view.Status == outlook.StatusAvailable {
		horizon := clock.span(view.Horizon)
		plans.Horizon = &horizon
		for _, segment := range view.Segments {
			if len(plans.Segments) == maxSnapshotSegments {
				plans.Truncated = true
				break
			}
			span := clock.span(segment.Interval)
			plans.Segments = append(plans.Segments, snapshotSegment{
				Presence: string(segment.Presence), Start: span.Start, End: span.End, Observed: segment.Observed,
			})
		}
		for _, window := range view.OfficeWindows {
			if len(plans.ReachableHours) == maxSnapshotReachable {
				plans.Truncated = true
				break
			}
			span := clock.span(window.Interval)
			entry := snapshotReachable{
				Start: span.Start, End: span.End, Status: officeStatus(window),
				Reachable: []snapshotRange{}, Possible: []snapshotRange{},
			}
			for _, part := range window.Reachable {
				entry.Reachable = append(entry.Reachable, clock.span(part))
			}
			for _, part := range window.Possible {
				entry.Possible = append(entry.Possible, clock.span(part))
			}
			plans.ReachableHours = append(plans.ReachableHours, entry)
		}
		for index, commitment := range view.Commitments {
			if index == maxSnapshotCommitments {
				plans.Truncated = true
				break
			}
			span := clock.span(commitment.Interval)
			record := computed.events[commitment.EventID]
			entry := snapshotCommitment{
				Ref: fmt.Sprintf("commitment_%02d", index+1), Start: span.Start, End: span.End,
				AllDay: record.AllDay, Conflict: string(commitment.Conflict),
			}
			if record.Ownership == calendarcore.OwnershipAppOwned {
				entry.TaskID = record.TaskID
			}
			plans.Commitments = append(plans.Commitments, entry)
		}
	}
	for proposalID, candidate := range proposals.pending {
		span := clock.span(candidate.proposal.Window)
		plans.Suggestions = append(plans.Suggestions, snapshotSuggestion{
			ProposalID: proposalID, TaskID: candidate.task.TaskID, Start: span.Start, End: span.End,
			Confidence: string(candidate.proposal.Confidence.Level),
			Reasons:    append([]string{}, candidate.proposal.ExplanationCodes...),
		})
	}
	sort.Slice(plans.Suggestions, func(i, j int) bool {
		left, right := plans.Suggestions[i], plans.Suggestions[j]
		if !left.Start.Equal(right.Start) {
			return left.Start.Before(right.Start)
		}
		return left.ProposalID < right.ProposalID
	})
	if len(plans.Suggestions) > maxSnapshotSuggestions {
		plans.Suggestions, plans.Truncated = plans.Suggestions[:maxSnapshotSuggestions], true
	}
	for _, unplaced := range proposals.dto.Unplaced {
		if len(plans.Unplaced) == maxSnapshotUnplaced {
			plans.Truncated = true
			break
		}
		plans.Unplaced = append(plans.Unplaced, snapshotUnplaced{TaskID: unplaced.TaskID, Reason: unplaced.ReasonCode})
	}
	return plans
}
