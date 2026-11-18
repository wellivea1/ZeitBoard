package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"non24.app/core/agentactions"
	"non24.app/core/presence"
	storage "non24.app/core/storage/sqlite"
	"non24.app/desktop/internal/localagent"
)

// Proposals an agent makes that wait on this computer (ADR-0051): a dose to
// record, a task to add, a status to report (ADR-0070). The local endpoint queues them; only the owner
// approves one, from Home or Plan, and approving makes the same record the
// owner's own entry would.

// agentProposed tells the web views that the queue changed on its own: an
// agent proposed something.
const agentProposed = "zeitboard:agent-proposed"

// agentProposalHistoryShown is how many decided or lapsed proposals the
// decision history lists.
const agentProposalHistoryShown = 20

type localAgentDoseArguments struct {
	Target agentactions.DoseTarget `json:"target"`
}

type localAgentNewTaskArguments struct {
	Target agentactions.NewTaskTarget `json:"target"`
}

// proposeDose queues a dose an agent asked to record.
func (a *App) proposeDose(ctx context.Context, action string, arguments json.RawMessage) (localAgentProposalResult, error) {
	var input localAgentDoseArguments
	if err := decodeStrictJSON(arguments, &input); err != nil {
		return localAgentProposalResult{}, localagent.UserError("Proposal arguments are invalid. Nothing was proposed.")
	}
	target := input.Target
	target.MedicationID = strings.TrimSpace(target.MedicationID)
	target.Status = strings.TrimSpace(target.Status)
	now := a.currentTime().UTC().Truncate(time.Second)
	if err := target.Validate(now); err != nil {
		return localAgentProposalResult{}, invalidTarget(err)
	}
	doseAt := now
	if target.DoseAt != nil {
		doseAt = target.DoseAt.UTC().Truncate(time.Second)
	}
	return a.queueAgentProposal(action, func(store *storage.Store, id string) error {
		return store.ProposeDose(ctx, id, storage.ProposedDose{
			MedicationID: target.MedicationID, Status: target.Status, DoseAt: doseAt, ZoneID: localZoneID(),
		}, now)
	}, "The dose is waiting for the owner to record it in ZeitBoard. Nothing is recorded until they accept it, and it lapses after a day.")
}

// proposeTask queues a task an agent asked to add, in the owner's words.
func (a *App) proposeTask(ctx context.Context, action string, arguments json.RawMessage) (localAgentProposalResult, error) {
	var input localAgentNewTaskArguments
	if err := decodeStrictJSON(arguments, &input); err != nil {
		return localAgentProposalResult{}, localagent.UserError("Proposal arguments are invalid. Nothing was proposed.")
	}
	target := input.Target
	target.Title = strings.TrimSpace(target.Title)
	now := a.currentTime().UTC().Truncate(time.Second)
	if err := target.Validate(now); err != nil {
		return localAgentProposalResult{}, invalidTarget(err)
	}
	return a.queueAgentProposal(action, func(store *storage.Store, id string) error {
		return store.ProposeTask(ctx, id, storage.ProposedTask{
			Title: target.Title, DurationMinutes: target.DurationMinutes,
			EarliestStartAt: utcPointer(target.EarliestStartAt), LatestFinishAt: utcPointer(target.LatestFinishAt),
		}, now)
	}, "The task is waiting for the owner to add it in ZeitBoard. Nothing is added until they accept it, and it lapses after a day.")
}

type localAgentStatusArguments struct {
	Target agentactions.StatusTarget `json:"target"`
}

// proposeStatus queues a status an agent asked the owner to report, or to
// take back. The proposal says nothing about who sees it: accepting it shares
// the report or not by the owner's own setting, as a tap on Home would.
func (a *App) proposeStatus(ctx context.Context, action string, arguments json.RawMessage) (localAgentProposalResult, error) {
	var input localAgentStatusArguments
	if err := decodeStrictJSON(arguments, &input); err != nil {
		return localAgentProposalResult{}, localagent.UserError("Proposal arguments are invalid. Nothing was proposed.")
	}
	target := input.Target
	target.Status = strings.TrimSpace(target.Status)
	if err := target.Validate(); err != nil {
		return localAgentProposalResult{}, invalidTarget(err)
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	return a.queueAgentProposal(action, func(store *storage.Store, id string) error {
		return store.ProposeStatus(ctx, id, storage.ProposedStatus{Status: target.Status, Minutes: target.Minutes}, now)
	}, "The status is waiting for the owner to report it in ZeitBoard. Nothing is reported until they accept it, and it lapses after a day.")
}

func invalidTarget(err error) error {
	return localagent.UserError("The proposal target is invalid: " + err.Error() + ". Nothing was proposed.")
}

// queueAgentProposal stores one proposal under a new id and tells the views.
func (a *App) queueAgentProposal(action string, propose func(*storage.Store, string) error, answer string) (localAgentProposalResult, error) {
	store, err := a.requireStore()
	if err != nil {
		return localAgentProposalResult{}, localagent.UserError("ZeitBoard's local records are unavailable. Nothing was proposed.")
	}
	id := newLocalID("agent_proposal")
	switch err := propose(store, id); {
	case errors.Is(err, storage.ErrMedicationNotFound):
		return localAgentProposalResult{}, localagent.UserError("No medication has that id; get_snapshot and get_medication_timing list them. Nothing was proposed.")
	case errors.Is(err, storage.ErrMedicationNotActive):
		return localAgentProposalResult{}, localagent.UserError("That medication is not active. Nothing was proposed.")
	case errors.Is(err, storage.ErrTooManyAgentProposals):
		return localAgentProposalResult{}, localagent.UserError("Too many proposals are already waiting for the owner. Nothing was proposed.")
	case err != nil:
		return localAgentProposalResult{}, err
	}
	a.announce(agentProposed)
	return localAgentProposalResult{
		SchemaVersion: "v1",
		Result:        "proposed",
		Action:        action,
		Answer:        answer,
		Proposals:     []localAgentProposalSummary{{ProposalID: id, Status: "pending"}},
		Approval:      "Pending proposals require a human decision in ZeitBoard; this tool cannot approve or apply them.",
	}, nil
}

func utcPointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC().Truncate(time.Second)
	return &utc
}

// AgentProposalDTO is one proposal waiting on this computer, with the private
// label or title shown on this computer's screen only.
type AgentProposalDTO struct {
	ProposalID string `json:"proposalId"`
	ActionID   string `json:"actionId"`
	// Title is the card's title, from the action registry: "Record dose".
	Title     string             `json:"title"`
	State     string             `json:"state"`
	CreatedAt string             `json:"createdAt"`
	ExpiresAt string             `json:"expiresAt"`
	DecidedAt string             `json:"decidedAt,omitempty"`
	Dose      *ProposedDoseDTO   `json:"dose,omitempty"`
	Task      *ProposedTaskDTO   `json:"task,omitempty"`
	Reported  *ProposedStatusDTO `json:"reported,omitempty"`
}

// ProposedStatusDTO is a proposed status. Shared is what accepting it now
// would do, by the owner's own setting; the proposal cannot ask for it.
type ProposedStatusDTO struct {
	Status  string `json:"status"`
	Label   string `json:"label"`
	Minutes int    `json:"minutes"`
	Shared  bool   `json:"shared"`
}

// ProposedDoseDTO is a proposed dose: the instant, and the zone it was
// proposed in.
type ProposedDoseDTO struct {
	MedicationID    string `json:"medicationId"`
	MedicationLabel string `json:"medicationLabel"`
	Status          string `json:"status"`
	DoseAt          string `json:"doseAt"`
	ZoneID          string `json:"zoneId"`
}

// ProposedTaskDTO is a proposed task, as the owner would have typed it.
type ProposedTaskDTO struct {
	Title           string `json:"title"`
	DurationMinutes int    `json:"durationMinutes"`
	EarliestStartAt string `json:"earliestStartAt,omitempty"`
	LatestFinishAt  string `json:"latestFinishAt,omitempty"`
}

// AgentProposalsDTO is the queue: what waits for the owner, and what they or
// the clock decided lately.
type AgentProposalsDTO struct {
	SchemaVersion string             `json:"schemaVersion"`
	Pending       []AgentProposalDTO `json:"pending"`
	History       []AgentProposalDTO `json:"history"`
	NextExpiryAt  string             `json:"nextExpiryAt"`
}

// AgentProposalDecisionInput approves or rejects one proposal.
type AgentProposalDecisionInput struct {
	ProposalID string `json:"proposalId"`
	Decision   string `json:"decision"`
}

// GetAgentProposals reads the queue.
func (a *App) GetAgentProposals() (AgentProposalsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return AgentProposalsDTO{}, err
	}
	return a.agentProposals(a.applicationContext(), store, a.currentTime())
}

// DecideAgentProposal approves a proposal, which makes the owner's own record
// of it, or rejects it, and returns the queue.
func (a *App) DecideAgentProposal(input AgentProposalDecisionInput) (AgentProposalsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return AgentProposalsDTO{}, err
	}
	ctx := a.applicationContext()
	now := a.currentTime().UTC().Truncate(time.Second)
	proposalID := strings.TrimSpace(input.ProposalID)
	switch input.Decision {
	case storage.AgentProposalApproved:
		if a.isStatusProposal(ctx, store, proposalID, now) {
			err = a.approveStatusProposal(ctx, store, proposalID)
			break
		}
		_, err = store.ApproveAgentProposal(ctx, proposalID, now, newLocalID)
	case storage.AgentProposalRejected:
		err = store.RejectAgentProposal(ctx, proposalID, now)
	default:
		return AgentProposalsDTO{}, errors.New("a proposal is either approved or rejected")
	}
	switch {
	case errors.Is(err, storage.ErrAgentProposalExpired):
		return AgentProposalsDTO{}, errors.New("this proposal lapsed before it was decided, so nothing was recorded")
	case errors.Is(err, storage.ErrAgentProposalDecided):
		return AgentProposalsDTO{}, errors.New("this proposal was already decided")
	case errors.Is(err, storage.ErrAgentProposalNotFound):
		return AgentProposalsDTO{}, errors.New("this proposal is no longer waiting; its medication may have been deleted")
	case err != nil:
		return AgentProposalsDTO{}, err
	}
	return a.agentProposals(ctx, store, now)
}

// isStatusProposal reports whether a listed proposal is a status to report.
// One that is not listed is left to the general approval, which says why.
func (a *App) isStatusProposal(ctx context.Context, store *storage.Store, proposalID string, now time.Time) bool {
	proposals, err := store.AgentProposals(ctx, now)
	if err != nil {
		return false
	}
	for _, proposal := range proposals {
		if proposal.ProposalID == proposalID {
			return proposal.Status != nil
		}
	}
	return false
}

// approveStatusProposal makes an accepted proposal the owner's report: the
// same report a tap on Home makes, shared or not by the owner's own setting,
// and in the same transaction as the decision.
func (a *App) approveStatusProposal(ctx context.Context, store *storage.Store, proposalID string) error {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	settings, _ := a.loadStatusSettings()
	// Read before the transaction opens: the store has one connection, and a
	// read inside the approval would wait on the approval itself.
	at, err := a.nextStatusTime(ctx, store)
	if err != nil {
		return err
	}
	_, err = store.ApproveStatusProposal(ctx, proposalID, at, func(asked storage.ProposedStatus) (presence.Report, error) {
		if asked.Status == presence.Cleared {
			return statusReportAt(at, presence.Cleared, 0, false), nil
		}
		lifetime := presence.DefaultLifetime(asked.Status)
		if asked.Minutes != 0 {
			lifetime = time.Duration(asked.Minutes) * time.Minute
		}
		return statusReportAt(at, asked.Status, lifetime, settings.ShareByDefault), nil
	})
	if err == nil {
		a.wakeDesktopBackground()
	}
	return err
}

func (a *App) agentProposals(ctx context.Context, store *storage.Store, now time.Time) (AgentProposalsDTO, error) {
	proposals, err := store.AgentProposals(ctx, now)
	if err != nil {
		return AgentProposalsDTO{}, err
	}
	medications, err := store.ListMedications(ctx)
	if err != nil {
		return AgentProposalsDTO{}, err
	}
	labels := make(map[string]string, len(medications))
	for _, medication := range medications {
		labels[medication.MedicationID] = medication.Label
	}
	result := AgentProposalsDTO{SchemaVersion: "v1", Pending: []AgentProposalDTO{}, History: []AgentProposalDTO{}}
	shareStatus, _ := a.loadStatusSettings()
	for _, proposal := range proposals {
		dto := agentProposalDTO(proposal, labels)
		if dto.Reported != nil && dto.Reported.Status != presence.Cleared {
			dto.Reported.Shared = shareStatus.ShareByDefault
		}
		if proposal.State != storage.AgentProposalPending {
			if len(result.History) < agentProposalHistoryShown {
				result.History = append(result.History, dto)
			}
			continue
		}
		result.Pending = append(result.Pending, dto)
		if expiry := proposal.ExpiresAt.UTC().Format(time.RFC3339); result.NextExpiryAt == "" || expiry < result.NextExpiryAt {
			result.NextExpiryAt = expiry
		}
	}
	return result, nil
}

func agentProposalDTO(proposal storage.AgentProposal, labels map[string]string) AgentProposalDTO {
	dto := AgentProposalDTO{
		ProposalID: proposal.ProposalID,
		ActionID:   proposal.ActionID,
		Title:      agentactions.CardTitle(proposal.ActionID),
		State:      proposal.State,
		CreatedAt:  proposal.CreatedAt.UTC().Format(time.RFC3339),
		ExpiresAt:  proposal.ExpiresAt.UTC().Format(time.RFC3339),
	}
	if !proposal.DecidedAt.IsZero() {
		dto.DecidedAt = proposal.DecidedAt.UTC().Format(time.RFC3339)
	}
	if dose := proposal.Dose; dose != nil {
		dto.Dose = &ProposedDoseDTO{
			MedicationID: dose.MedicationID, MedicationLabel: labels[dose.MedicationID],
			Status: dose.Status, DoseAt: dose.DoseAt.UTC().Format(time.RFC3339), ZoneID: dose.ZoneID,
		}
	}
	if task := proposal.Task; task != nil {
		dto.Task = &ProposedTaskDTO{Title: task.Title, DurationMinutes: task.DurationMinutes}
		if task.EarliestStartAt != nil {
			dto.Task.EarliestStartAt = task.EarliestStartAt.UTC().Format(time.RFC3339)
		}
		if task.LatestFinishAt != nil {
			dto.Task.LatestFinishAt = task.LatestFinishAt.UTC().Format(time.RFC3339)
		}
	}
	if status := proposal.Status; status != nil {
		minutes := status.Minutes
		if minutes == 0 && status.Status != presence.Cleared {
			minutes = int(presence.DefaultLifetime(status.Status) / time.Minute)
		}
		dto.Reported = &ProposedStatusDTO{Status: status.Status, Label: statusWords(status.Status), Minutes: minutes}
	}
	return dto
}
