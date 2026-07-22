package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	return result.dto, nil
}

func (a *App) DecideLocalProposal(input LocalProposalDecisionInput) (LocalProposalDecisionDTO, error) {
	if input.Decision != storage.ProposalApproved && input.Decision != storage.ProposalRejected {
		return LocalProposalDecisionDTO{}, errors.New("decision must be approved or rejected")
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	built, err := a.buildLocalProposals(now)
	if err != nil {
		return LocalProposalDecisionDTO{}, err
	}
	candidate, found := built.pending[input.ProposalID]
	if !found {
		return LocalProposalDecisionDTO{}, storage.ErrStaleProposal
	}
	decision := storage.ProposalDecisionInput{
		DecisionID:        newLocalID("decision"),
		ProposalID:        input.ProposalID,
		TaskID:            candidate.task.TaskID,
		TaskRevision:      effectiveTaskRevision(candidate.task),
		EstimateID:        candidate.estimateID,
		ProposalTitle:     candidate.task.Title,
		ProposalStartAt:   candidate.proposal.Window.Start.UTC,
		ProposalEndAt:     candidate.proposal.Window.End.UTC,
		ZoneID:            candidate.proposal.Window.Start.ZoneID,
		Confidence:        string(candidate.proposal.Confidence.Level),
		ExplanationCodes:  append([]string(nil), candidate.proposal.ExplanationCodes...),
		Decision:          input.Decision,
		DecidedAt:         now,
		SnapshotStartAt:   candidate.snapshotStartAt,
		SnapshotEndAt:     candidate.snapshotEndAt,
		EventSnapshotHash: candidate.eventSnapshotHash,
		SleepSnapshotHash: candidate.sleepSnapshotHash,
	}
	var owned *calendarcore.Event
	if input.Decision == storage.ProposalApproved {
		event := calendarcore.Event{
			EventID:        ownedCalendarEventID(input.ProposalID),
			SourceID:       storage.ZeitBoardCalendarSourceID,
			SourceRecordID: input.ProposalID,
			Title:          candidate.task.Title,
			StartAt:        candidate.proposal.Window.Start.UTC,
			EndAt:          candidate.proposal.Window.End.UTC,
			ZoneID:         candidate.proposal.Window.Start.ZoneID,
			Busy:           true,
			Ownership:      calendarcore.OwnershipAppOwned,
			CreatedAt:      now,
			TaskID:         candidate.task.TaskID,
			TaskRevision:   effectiveTaskRevision(candidate.task),
			ProposalID:     input.ProposalID,
		}
		owned = &event
	}
	store, err := a.requireStore()
	if err != nil {
		return LocalProposalDecisionDTO{}, err
	}
	record, err := store.DecideProposal(context.Background(), decision, owned)
	if err != nil {
		return LocalProposalDecisionDTO{}, err
	}
	message := "Proposal rejected; no calendar block was written."
	if record.Decision == storage.ProposalApproved {
		message = "Proposal approved and written to ZeitBoard placements."
	}
	return LocalProposalDecisionDTO{
		ProposalID: record.ProposalID,
		Decision:   record.Decision,
		EventID:    record.EventID,
		Message:    message,
	}, nil
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

	zoneID := defaultZoneID
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
	for index, task := range tasks {
		record := taskRecords[index]
		if approvedTaskRevisions[record.TaskID] == effectiveTaskRevision(record) {
			continue
		}
		proposal, proposalErr := scheduler.Propose(scheduling.Request{
			Task:         task,
			Availability: availability,
			Events:       fixedEvents,
			WakeAnchor:   wakeAnchor,
			Now:          planningNow,
		})
		if proposalErr != nil {
			reason := scheduling.ClassifyUnplaced(proposalErr)
			result.dto.Unplaced = append(result.dto.Unplaced, UnplacedDTO{
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
		result.dto.Proposals = append(result.dto.Proposals, ProposalDTO{
			ID:               proposalID,
			Origin:           "scheduler",
			Kind:             "Place",
			Title:            task.Title,
			To:               formatRange(proposal.Window),
			RhythmContext:    rhythmContext(proposal, availability),
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
