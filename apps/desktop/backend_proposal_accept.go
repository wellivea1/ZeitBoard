package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"non24.app/core/agentactions"
	"non24.app/core/domain"
	"non24.app/core/scheduling"
	storage "non24.app/core/storage/sqlite"
)

// An approved schedule proposal that the server holds becomes the task's
// accepted time on this computer (ADR-0067), as accepting the planner's own
// suggestion does: an app-owned block bound to the task's revision and to the
// evidence the time fits on, kept in the decision history, synced as a
// placement and undone the same way. A task keeps one accepted time, so a
// later approval takes the place of an earlier one.

// assistantDecision begins the id of a decision that accepted an assistant's
// proposal, which is how the decision history tells it from the planner's.
const assistantDecision = "decision_assistant"

// decisionOrigin is where an accepted or declined time came from.
func decisionOrigin(record storage.ProposalDecisionRecord) string {
	if strings.HasPrefix(record.DecisionID, assistantDecision+"_") {
		return "assistant"
	}
	return "scheduler"
}

// undoneHere names the proposals whose accepted time was undone here, which
// the server still records as approved.
func (a *App) undoneHere(ctx context.Context) map[string]bool {
	store, err := a.requireStore()
	if err != nil {
		return nil
	}
	undone, _ := store.UndoneProposals(ctx)
	return undone
}

// notAccepted says why a proposal's time cannot be accepted here.
type notAccepted struct{ why string }

func (n notAccepted) Error() string { return n.why }

// askedTime is what a schedule proposal asks for: a task, at a time.
type askedTime struct {
	TaskID string
	Start  time.Time
	End    time.Time
	ZoneID string
}

// acceptance checks that the time a proposal asks for still fits here, and
// gives the decision that writes it. Nothing is approved without one.
func (a *App) acceptance(ctx context.Context, client desktopBackendClient, proposalID string) (storage.ProposalDecisionItem, error) {
	asked, err := proposalAsks(ctx, client, proposalID)
	if err != nil {
		return storage.ProposalDecisionItem{}, err
	}
	return a.acceptedTime(ctx, proposalID, asked, a.currentTime().UTC().Truncate(time.Second))
}

// proposalAsks reads the time a proposal held by the server asks for.
func proposalAsks(ctx context.Context, client desktopBackendClient, proposalID string) (askedTime, error) {
	var read struct {
		SchemaVersion string                `json:"schema_version"`
		Proposal      backendProposalRecord `json:"proposal"`
	}
	if err := client.getJSON(ctx, "/v1/proposals/"+url.PathEscape(proposalID), &read); err != nil {
		return askedTime{}, err
	}
	var payload backendProposalPayload
	if !agentactions.IsScheduleProposal(read.Proposal.ActionID, agentactions.ChatAssistant|agentactions.AnyMCP) ||
		json.Unmarshal(read.Proposal.Payload, &payload) != nil || len(payload.ScheduleProposals.Proposals) != 1 {
		return askedTime{}, notAccepted{"That proposal names no single time."}
	}
	item := payload.ScheduleProposals.Proposals[0]
	return askedTime{TaskID: item.TaskID, Start: item.StartAt.UTC(), End: item.EndAt.UTC(), ZoneID: item.ZoneID}, nil
}

// acceptedTime plans the task again inside exactly the asked time, on the
// evidence the planner uses now, and gives the approval that writes it. The
// task's earlier accepted time, if it has one, gives way to it.
func (a *App) acceptedTime(ctx context.Context, proposalID string, asked askedTime, now time.Time) (storage.ProposalDecisionItem, error) {
	store, err := a.requireStore()
	if err != nil {
		return storage.ProposalDecisionItem{}, err
	}
	planningNow := now.Truncate(localProposalTTL)
	state, sleepFingerprint, err := a.localEstimateForPlanningSnapshot(ctx, planningNow)
	if err != nil {
		return storage.ProposalDecisionItem{}, err
	}
	if state.Status != "estimated" {
		return storage.ProposalDecisionItem{}, notAccepted{"There is no current estimate here to plan it against."}
	}
	zoneID := state.Estimate.AsOf.ZoneID
	tasks, records, err := store.OpenDomainTasks(ctx, zoneID)
	if err != nil {
		return storage.ProposalDecisionItem{}, err
	}
	index := slices.IndexFunc(records, func(record storage.TaskRecord) bool { return record.TaskID == asked.TaskID })
	if index < 0 {
		return storage.ProposalDecisionItem{}, notAccepted{"That task is no longer open on this computer."}
	}
	if err := askedFitsTask(asked, records[index], now); err != nil {
		return storage.ProposalDecisionItem{}, err
	}
	availability := localPlanningAvailability(state, planningNow)
	snapshotStart, snapshotEnd, ok := planningSnapshotRange(availability)
	if !ok {
		return storage.ProposalDecisionItem{}, notAccepted{"There is no predicted waking time here to plan it in."}
	}
	events, fingerprint, err := store.BusyDomainEvents(ctx, snapshotStart, snapshotEnd, zoneID)
	if err != nil {
		return storage.ProposalDecisionItem{}, err
	}
	active, err := store.ActiveProposalDecisions(ctx)
	if err != nil {
		return storage.ProposalDecisionItem{}, err
	}
	current := acceptedTimeOf(active, asked.TaskID)
	planned, err := planInside(tasks[index], asked, scheduling.Request{
		Availability: availability,
		Events:       withoutAcceptedTime(events, current),
		WakeAnchor:   planningWakeAnchor(state),
		Now:          now,
	})
	if err != nil {
		return storage.ProposalDecisionItem{}, err
	}
	item := localDecisionItem(proposalID, localProposalCandidate{
		proposal:          planned,
		task:              records[index],
		estimateID:        string(state.Estimate.ID),
		snapshotStartAt:   snapshotStart,
		snapshotEndAt:     snapshotEnd,
		eventSnapshotHash: fingerprint,
		sleepSnapshotHash: sleepFingerprint,
	}, storage.ProposalApproved, now)
	item.Input.DecisionID = newLocalID(assistantDecision)
	if current != nil {
		item.Replaces = &storage.ProposalUndo{DecisionID: newLocalID("decision"), ProposalID: current.ProposalID}
	}
	return item, nil
}

// askedFitsTask refuses a time that has begun, or that is not the task's
// length: an accepted time is the task's whole duration.
func askedFitsTask(asked askedTime, task storage.TaskRecord, now time.Time) error {
	if !asked.Start.After(now) {
		return notAccepted{"That time has already begun."}
	}
	length, takes := asked.End.Sub(asked.Start), time.Duration(task.DurationMinutes)*time.Minute
	if length != takes {
		return notAccepted{fmt.Sprintf("That time lasts %s, but %s takes %s.", formatDuration(length), task.Title, formatDuration(takes))}
	}
	return nil
}

// acceptedTimeOf is the task's accepted time: its approval still in force.
func acceptedTimeOf(active []storage.ProposalDecisionRecord, taskID string) *storage.ProposalDecisionRecord {
	for index := range active {
		if active[index].TaskID == taskID && active[index].Decision == storage.ProposalApproved {
			return &active[index]
		}
	}
	return nil
}

// withoutAcceptedTime leaves out the block of the accepted time being
// replaced, so a move may overlap where the task now sits.
func withoutAcceptedTime(events []domain.CalendarEvent, current *storage.ProposalDecisionRecord) []domain.CalendarEvent {
	if current == nil {
		return events
	}
	return slices.DeleteFunc(slices.Clone(events), func(event domain.CalendarEvent) bool {
		return string(event.ID) == current.EventID
	})
}

// planInside plans the task inside exactly the asked time, by the scheduler's
// own rules: in predicted waking time, clear of busy time, and as sure as the
// task asks. The time asked for takes the place of the task's own bounds,
// since the owner chose it.
func planInside(task domain.FlexibleTask, asked askedTime, request scheduling.Request) (scheduling.Proposal, error) {
	start, startErr := domain.NewZonedInstant(asked.Start, asked.ZoneID)
	end, endErr := domain.NewZonedInstant(asked.End, asked.ZoneID)
	if err := errors.Join(startErr, endErr); err != nil {
		return scheduling.Proposal{}, notAccepted{"That proposal's time cannot be read."}
	}
	task.Constraint.EarliestStart, task.Constraint.LatestFinish, task.Constraint.Deadline = &start, &end, nil
	request.Task = task
	planned, err := scheduling.Scheduler{}.Propose(request)
	if err != nil {
		return scheduling.Proposal{}, notAccepted{"That time no longer fits here. " + unplacedReasonLabel(scheduling.ClassifyUnplaced(err)) + "."}
	}
	if !planned.Window.Start.UTC.Equal(asked.Start) || !planned.Window.End.UTC.Equal(asked.End) {
		return scheduling.Proposal{}, notAccepted{"That time no longer fits here."}
	}
	return planned, nil
}

// planningWakeAnchor is the latest recorded waking, which the scheduler
// counts a task's after-wake preference from.
func planningWakeAnchor(state localEstimateState) *domain.WakeAnchor {
	latest, ok := latestPrincipalSession(state.Sessions)
	if !ok {
		return nil
	}
	return &domain.WakeAnchor{
		ID:         "latest-wake",
		At:         latest.Intervals[0].Interval.End,
		Confidence: state.Estimate.Confidence,
	}
}
