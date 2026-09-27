package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	calendarcore "non24.app/core/calendar"
	"non24.app/core/domain"
	"non24.app/core/scheduling"
	storage "non24.app/core/storage/sqlite"
)

const localProposalTTL = 30 * time.Minute

type LocalProposalDecisionInput struct {
	ProposalID string `json:"proposalId"`
	Decision   string `json:"decision"`
}

type LocalProposalUndoInput struct {
	ProposalID string `json:"proposalId"`
}

type LocalProposalDecisionDTO struct {
	ProposalID string `json:"proposalId"`
	Decision   string `json:"decision"`
	EventID    string `json:"eventId,omitempty"`
	Message    string `json:"message"`
}

// LocalProposalsDecisionInput decides several suggestions the owner reviewed
// together, all the same way.
type LocalProposalsDecisionInput struct {
	ProposalIDs []string `json:"proposalIds"`
	Decision    string   `json:"decision"`
}

// LocalProposalsDecisionDTO is what one batch decision recorded.
type LocalProposalsDecisionDTO struct {
	Decisions []LocalProposalDecisionDTO `json:"decisions"`
	Message   string                     `json:"message"`
}

type localProposalCandidate struct {
	proposal          scheduling.Proposal
	task              storage.TaskRecord
	estimateID        string
	snapshotStartAt   time.Time
	snapshotEndAt     time.Time
	eventSnapshotHash string
	sleepSnapshotHash string
}

type localProposalBuild struct {
	dto     ProposalsDTO
	pending map[string]localProposalCandidate
}

func (a *App) GetProposals() (ProposalsDTO, error) {
	result, err := a.buildLocalProposals(a.currentTime().UTC())
	if err != nil {
		return ProposalsDTO{}, err
	}
	if err := a.attachTaskConflictReview(&result.dto); err != nil {
		return ProposalsDTO{}, err
	}
	return result.dto, nil
}

func (a *App) DecideLocalProposal(input LocalProposalDecisionInput) (LocalProposalDecisionDTO, error) {
	decided, err := a.decideLocalProposals([]string{input.ProposalID}, input.Decision)
	if err != nil {
		return LocalProposalDecisionDTO{}, err
	}
	return decided.Decisions[0], nil
}

// DecideLocalProposals decides the suggestions the owner reviewed together, in
// one transaction: each is checked against the plan it came from, and if any
// has changed since, none is decided (ADR-0052).
func (a *App) DecideLocalProposals(input LocalProposalsDecisionInput) (LocalProposalsDecisionDTO, error) {
	if len(input.ProposalIDs) == 0 || len(input.ProposalIDs) > maxLocalProposalBatch {
		return LocalProposalsDecisionDTO{}, fmt.Errorf("choose 1 to %d suggestions", maxLocalProposalBatch)
	}
	return a.decideLocalProposals(input.ProposalIDs, input.Decision)
}

// maxLocalProposalBatch bounds one reviewed batch; the planner offers fewer.
const maxLocalProposalBatch = 50

func (a *App) decideLocalProposals(proposalIDs []string, decisionKind string) (LocalProposalsDecisionDTO, error) {
	if decisionKind != storage.ProposalApproved && decisionKind != storage.ProposalRejected {
		return LocalProposalsDecisionDTO{}, errors.New("decision must be approved or rejected")
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	built, err := a.buildLocalProposals(now)
	if err != nil {
		return LocalProposalsDecisionDTO{}, err
	}
	items := make([]storage.ProposalDecisionItem, 0, len(proposalIDs))
	for _, proposalID := range proposalIDs {
		candidate, found := built.pending[proposalID]
		if !found {
			return LocalProposalsDecisionDTO{}, storage.ErrStaleProposal
		}
		// A block that has already begun is not a plan. The planner no longer
		// offers one, and this keeps a decision from recording one if it ever did.
		if decisionKind == storage.ProposalApproved && candidate.proposal.Window.Start.UTC.Before(now) {
			return LocalProposalsDecisionDTO{}, storage.ErrStaleProposal
		}
		items = append(items, localDecisionItem(proposalID, candidate, decisionKind, now))
	}
	store, err := a.requireStore()
	if err != nil {
		return LocalProposalsDecisionDTO{}, err
	}
	records, err := store.DecideProposals(context.Background(), items)
	if err != nil {
		return LocalProposalsDecisionDTO{}, err
	}
	a.calendarWriter.nudge()
	result := LocalProposalsDecisionDTO{Decisions: make([]LocalProposalDecisionDTO, 0, len(records))}
	for _, record := range records {
		message := "Proposal rejected; no calendar block was written."
		if record.Decision == storage.ProposalApproved {
			message = "Proposal approved and written to ZeitBoard placements."
		}
		result.Decisions = append(result.Decisions, LocalProposalDecisionDTO{
			ProposalID: record.ProposalID, Decision: record.Decision, EventID: record.EventID, Message: message,
		})
	}
	result.Message = result.Decisions[0].Message
	if len(records) > 1 {
		verb := "rejected"
		if decisionKind == storage.ProposalApproved {
			verb = "approved and written to ZeitBoard placements"
		}
		result.Message = fmt.Sprintf("%d proposals %s together.", len(records), verb)
	}
	return result, nil
}

// localDecisionItem is the decision on one suggestion, with the evidence it
// was planned on and, for an approval, the app-owned block it writes.
func localDecisionItem(proposalID string, candidate localProposalCandidate, decisionKind string, now time.Time) storage.ProposalDecisionItem {
	item := storage.ProposalDecisionItem{Input: storage.ProposalDecisionInput{
		DecisionID:        newLocalID("decision"),
		ProposalID:        proposalID,
		TaskID:            candidate.task.TaskID,
		TaskRevision:      effectiveTaskRevision(candidate.task),
		EstimateID:        candidate.estimateID,
		ProposalTitle:     candidate.task.Title,
		ProposalStartAt:   candidate.proposal.Window.Start.UTC,
		ProposalEndAt:     candidate.proposal.Window.End.UTC,
		ZoneID:            candidate.proposal.Window.Start.ZoneID,
		Confidence:        string(candidate.proposal.Confidence.Level),
		ExplanationCodes:  append([]string(nil), candidate.proposal.ExplanationCodes...),
		Decision:          decisionKind,
		DecidedAt:         now,
		SnapshotStartAt:   candidate.snapshotStartAt,
		SnapshotEndAt:     candidate.snapshotEndAt,
		EventSnapshotHash: candidate.eventSnapshotHash,
		SleepSnapshotHash: candidate.sleepSnapshotHash,
	}}
	if decisionKind == storage.ProposalApproved {
		item.OwnedEvent = &calendarcore.Event{
			EventID:        ownedCalendarEventID(proposalID),
			SourceID:       storage.ZeitBoardCalendarSourceID,
			SourceRecordID: proposalID,
			Title:          candidate.task.Title,
			StartAt:        candidate.proposal.Window.Start.UTC,
			EndAt:          candidate.proposal.Window.End.UTC,
			ZoneID:         candidate.proposal.Window.Start.ZoneID,
			Busy:           true,
			Ownership:      calendarcore.OwnershipAppOwned,
			CreatedAt:      now,
			TaskID:         candidate.task.TaskID,
			TaskRevision:   effectiveTaskRevision(candidate.task),
			ProposalID:     proposalID,
		}
	}
	return item
}

func (a *App) UndoLocalProposalDecision(input LocalProposalUndoInput) (LocalProposalDecisionDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return LocalProposalDecisionDTO{}, err
	}
	record, err := store.UndoProposalDecision(
		context.Background(), newLocalID("decision"), input.ProposalID, a.currentTime().UTC().Truncate(time.Second),
	)
	if err != nil {
		return LocalProposalDecisionDTO{}, err
	}
	a.calendarWriter.nudge()
	return LocalProposalDecisionDTO{
		ProposalID: record.ProposalID,
		Decision:   record.Decision,
		EventID:    record.EventID,
		Message:    "Decision undone; any linked ZeitBoard placement was removed.",
	}, nil
}

func (a *App) buildLocalProposals(now time.Time) (localProposalBuild, error) {
	ctx := context.Background()
	store, err := a.requireStore()
	if err != nil {
		return localProposalBuild{}, err
	}
	planningNow := now.UTC().Truncate(localProposalTTL)
	state, sleepFingerprint, err := a.localEstimateForPlanningSnapshot(ctx, planningNow)
	if err != nil {
		return localProposalBuild{}, err
	}
	active, err := store.ActiveProposalDecisions(ctx)
	if err != nil {
		return localProposalBuild{}, err
	}
	result := localProposalBuild{
		dto: ProposalsDTO{
			Status:      state.Status,
			Refusal:     refusalDTO(state.Refusal, state.Message),
			FixtureMode: false,
			Proposals:   make([]ProposalDTO, 0, len(active)),
			Unplaced:    []UnplacedDTO{},
		},
		pending: make(map[string]localProposalCandidate),
	}
	activeByID := make(map[string]storage.ProposalDecisionRecord, len(active))
	approvedTaskRevisions := make(map[string]int)
	for _, record := range active {
		activeByID[record.ProposalID] = record
		if record.Decision == storage.ProposalApproved {
			approvedTaskRevisions[record.TaskID] = record.TaskRevision
		}
		dto, err := decidedProposalDTO(record)
		if err != nil {
			return localProposalBuild{}, err
		}
		result.dto.Proposals = append(result.dto.Proposals, dto)
	}

	zoneID := localZoneID()
	if state.Status == "estimated" {
		zoneID = state.Estimate.AsOf.ZoneID
	}
	tasks, taskRecords, err := store.OpenDomainTasks(ctx, zoneID)
	if err != nil {
		return localProposalBuild{}, err
	}
	if state.Status != "estimated" {
		result.dto.Unplaced = unplacedForUnavailableEstimate(tasks)
		return result, nil
	}

	availability := localPlanningAvailability(state, planningNow)
	snapshotStart, snapshotEnd, ok := planningSnapshotRange(availability)
	if !ok {
		for _, task := range tasks {
			result.dto.Unplaced = append(result.dto.Unplaced, UnplacedDTO{
				TaskID:     string(task.ID),
				Title:      task.Title,
				ReasonCode: string(scheduling.ReasonNoAvailableInterval),
				Reason:     unplacedReasonLabel(scheduling.ReasonNoAvailableInterval),
				NextAction: "Wait for the next estimate refresh or add explicit task bounds.",
			})
		}
		return result, nil
	}
	fixedEvents, fingerprint, err := store.BusyDomainEvents(ctx, snapshotStart, snapshotEnd, zoneID)
	if err != nil {
		return localProposalBuild{}, err
	}
	latest, hasLatest := latestPrincipalSession(state.Sessions)
	var wakeAnchor *domain.WakeAnchor
	if hasLatest {
		wakeAnchor = &domain.WakeAnchor{
			ID:         "latest-wake",
			At:         latest.Intervals[0].Interval.End,
			Confidence: state.Estimate.Confidence,
		}
	}
	expiresAt := planningNow.Add(localProposalTTL)
	scheduler := scheduling.Scheduler{}
	// The planning snapshot is pinned to the start of a 30-minute bucket so a
	// proposal keeps its identity while someone reads it. Suggestions must not
	// start inside that bucket, though: planning from its start offered, and
	// accepted, a block that had begun eleven minutes earlier. They start when
	// the bucket ends, which is also when they are replaced.
	earliestStart := expiresAt
	// Each suggestion reserves its time for the ones after it, so the pending
	// set is a plan that can be accepted whole rather than several tasks
	// stacked on one minute.
	var reserved []domain.TimeRange
	for _, index := range planningOrder(tasks) {
		task := tasks[index]
		record := taskRecords[index]
		if approvedTaskRevisions[record.TaskID] == effectiveTaskRevision(record) {
			continue
		}
		proposal, proposalErr := scheduler.Propose(scheduling.Request{
			Task:         task,
			Availability: availability,
			Events:       fixedEvents,
			WakeAnchor:   wakeAnchor,
			Now:          earliestStart,
			Reserved:     reserved,
		})
		if proposalErr != nil {
			reason := scheduling.ClassifyUnplaced(proposalErr)
			result.dto.Unplaced = append(result.dto.Unplaced, UnplacedDTO{
				TaskID:     record.TaskID,
				Title:      task.Title,
				ReasonCode: string(reason),
				Reason:     unplacedReasonLabel(reason),
				NextAction: "Adjust task bounds or wait for the calendar or estimate to refresh.",
			})
			continue
		}
		proposalID := deterministicProposalID(record, state.Estimate.ID, proposal.Window, fingerprint, sleepFingerprint)
		if _, alreadyDecided := activeByID[proposalID]; alreadyDecided {
			continue
		}
		reserved = append(reserved, proposal.Window)
		result.dto.Proposals = append(result.dto.Proposals, ProposalDTO{
			ID:               proposalID,
			Origin:           "scheduler",
			Kind:             "Place",
			Title:            task.Title,
			To:               formatRange(proposal.Window),
			StartAt:          proposal.Window.Start.UTC.Format(time.RFC3339),
			EndAt:            proposal.Window.End.UTC.Format(time.RFC3339),
			RhythmContext:    rhythmContext(proposal, availability, wakeAnchor, planningNow),
			Confidence:       confidenceTitle(proposal.Confidence.Level),
			ExplanationCodes: append([]string(nil), proposal.ExplanationCodes...),
			ReasonLabels:     reasonLabels(proposal.ExplanationCodes),
			CreatedLabel:     "Proposed by Scheduler from local sleep entries and fixed calendar events",
			ExpiresLabel:     "Refreshes at " + expiresAt.In(locationOrUTC(zoneID)).Format("3:04 PM MST"),
			Decision:         "pending",
			CanUndo:          false,
		})
		result.pending[proposalID] = localProposalCandidate{
			proposal:          proposal,
			task:              record,
			estimateID:        string(state.Estimate.ID),
			snapshotStartAt:   snapshotStart,
			snapshotEndAt:     snapshotEnd,
			eventSnapshotHash: fingerprint,
			sleepSnapshotHash: sleepFingerprint,
		}
	}
	return result, nil
}

func (a *App) localEstimateForPlanningSnapshot(ctx context.Context, now time.Time) (localEstimateState, string, error) {
	store, err := a.requireStore()
	if err != nil {
		return localEstimateState{}, "", err
	}
	for attempt := 0; attempt < 2; attempt++ {
		before, err := store.SleepPlanningFingerprint(ctx)
		if err != nil {
			return localEstimateState{}, "", err
		}
		state, err := a.localEstimate(ctx, now)
		if err != nil {
			return localEstimateState{}, "", err
		}
		after, err := store.SleepPlanningFingerprint(ctx)
		if err != nil {
			return localEstimateState{}, "", err
		}
		if before == after {
			return state, before, nil
		}
	}
	return localEstimateState{}, "", storage.ErrStaleProposal
}

func decidedProposalDTO(record storage.ProposalDecisionRecord) (ProposalDTO, error) {
	start, err := domain.NewZonedInstant(record.ProposalStartAt, record.ZoneID)
	if err != nil {
		return ProposalDTO{}, err
	}
	end, err := domain.NewZonedInstant(record.ProposalEndAt, record.ZoneID)
	if err != nil {
		return ProposalDTO{}, err
	}
	decisionTitle := "Rejected"
	if record.Decision == storage.ProposalApproved {
		decisionTitle = "Approved"
	}
	return ProposalDTO{
		ID:               record.ProposalID,
		Origin:           "scheduler",
		Kind:             "Place",
		Title:            record.ProposalTitle,
		To:               formatRange(domain.TimeRange{Start: start, End: end}),
		StartAt:          start.UTC.Format(time.RFC3339),
		EndAt:            end.UTC.Format(time.RFC3339),
		RhythmContext:    "saved decision for this exact scheduler window",
		Confidence:       confidenceTitle(domain.ConfidenceLevel(record.Confidence)),
		ExplanationCodes: append([]string(nil), record.ExplanationCodes...),
		ReasonLabels:     reasonLabels(record.ExplanationCodes),
		CreatedLabel:     decisionTitle + " " + record.DecidedAt.In(locationOrUTC(record.ZoneID)).Format("Jan 2, 3:04 PM MST"),
		ExpiresLabel:     "Decision is retained until you undo it",
		Decision:         record.Decision,
		CanUndo:          true,
	}, nil
}

func planningSnapshotRange(availability []domain.AvailabilityWindow) (time.Time, time.Time, bool) {
	var start, end time.Time
	for _, window := range availability {
		if !window.Interval.End.UTC.After(window.Interval.Start.UTC) {
			continue
		}
		if start.IsZero() || window.Interval.Start.UTC.Before(start) {
			start = window.Interval.Start.UTC
		}
		if end.IsZero() || window.Interval.End.UTC.After(end) {
			end = window.Interval.End.UTC
		}
	}
	return start.UTC(), end.UTC(), !start.IsZero() && start.Before(end)
}

// planningOrder places the task with the nearest deadline first. Scheduling is
// greedy — each suggestion reserves its time before the next task is placed —
// so the order decides who gets the earliest free time. In stored order, a
// ninety-minute task with no deadline could take the only slot before another
// task's deadline and push that task off the plan entirely. Tasks without a
// deadline keep their stored order behind those that have one, and the order
// is deterministic, so rebuilding the plan to record a decision reproduces the
// same windows and the same proposal identities.
func planningOrder(tasks []domain.FlexibleTask) []int {
	order := make([]int, len(tasks))
	for index := range order {
		order[index] = index
	}
	deadline := func(task domain.FlexibleTask) (time.Time, bool) {
		var latest time.Time
		if task.Constraint.Deadline != nil {
			latest = task.Constraint.Deadline.UTC
		}
		if bound := task.Constraint.LatestFinish; bound != nil && (latest.IsZero() || bound.UTC.Before(latest)) {
			latest = bound.UTC
		}
		return latest, !latest.IsZero()
	}
	sort.SliceStable(order, func(i, j int) bool {
		left, leftBounded := deadline(tasks[order[i]])
		right, rightBounded := deadline(tasks[order[j]])
		if leftBounded != rightBounded {
			return leftBounded
		}
		return leftBounded && left.Before(right)
	})
	return order
}

func deterministicProposalID(task storage.TaskRecord, estimateID domain.PhaseEstimateID, window domain.TimeRange, eventSnapshotHash, sleepSnapshotHash string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s",
		task.TaskID, effectiveTaskRevision(task), estimateID,
		window.Start.UTC.Format(time.RFC3339Nano), window.End.UTC.Format(time.RFC3339Nano), eventSnapshotHash, sleepSnapshotHash,
	)))
	return "proposal_" + hex.EncodeToString(digest[:16])
}

func ownedCalendarEventID(proposalID string) string {
	digest := sha256.Sum256([]byte(proposalID))
	return "calendar_event_" + hex.EncodeToString(digest[:16])
}

func effectiveTaskRevision(task storage.TaskRecord) int {
	if task.Revision < 1 {
		return 1
	}
	return task.Revision
}
