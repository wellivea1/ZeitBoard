package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"non24.app/core/agentactions"
	storage "non24.app/core/storage/sqlite"
	"non24.app/desktop/internal/localagent"
)

// Doses an agent proposes (ADR-0051). The local endpoint queues them; only the
// owner records one, from Home or Plan, and recording appends the same
// medication event a hand-logged dose would.

// doseProposalsChanged tells the web views that the queue changed on its own:
// an agent proposed a dose.
const doseProposalsChanged = "zeitboard:dose-proposed"

// doseProposalHistoryShown is how many decided or lapsed proposals the
// decision history lists.
const doseProposalHistoryShown = 20

// proposeDose queues a dose an agent asked to record.
func (a *App) proposeDose(ctx context.Context, action string, arguments json.RawMessage) (localAgentProposalResult, error) {
	var input localAgentDoseArguments
	if err := decodeStrictJSON(arguments, &input); err != nil {
		return localAgentProposalResult{}, localagent.UserError("Proposal arguments are invalid. No dose was proposed.")
	}
	target := input.Target
	target.MedicationID = strings.TrimSpace(target.MedicationID)
	target.Status = strings.TrimSpace(target.Status)
	now := a.currentTime().UTC().Truncate(time.Second)
	if err := target.Validate(now); err != nil {
		return localAgentProposalResult{}, localagent.UserError("The proposal target is invalid: " + err.Error() + ". No dose was proposed.")
	}
	store, err := a.requireStore()
	if err != nil {
		return localAgentProposalResult{}, localagent.UserError("ZeitBoard's local records are unavailable. No dose was proposed.")
	}
	doseAt := now
	if target.DoseAt != nil {
		doseAt = target.DoseAt.UTC().Truncate(time.Second)
	}
	proposal := storage.DoseProposal{
		ProposalID:   newLocalID("dose_proposal"),
		MedicationID: target.MedicationID,
		Status:       target.Status,
		DoseAt:       doseAt,
		ZoneID:       localZoneID(),
	}
	switch err := store.ProposeDose(ctx, proposal, now); {
	case errors.Is(err, storage.ErrMedicationNotFound):
		return localAgentProposalResult{}, localagent.UserError("No medication has that id; get_snapshot and get_medication_timing list them. No dose was proposed.")
	case errors.Is(err, storage.ErrMedicationNotActive):
		return localAgentProposalResult{}, localagent.UserError("That medication is not active. No dose was proposed.")
	case errors.Is(err, storage.ErrTooManyDoseProposals):
		return localAgentProposalResult{}, localagent.UserError("Too many doses are already waiting for the owner. No dose was proposed.")
	case err != nil:
		return localAgentProposalResult{}, err
	}
	a.announce(doseProposalsChanged)
	return localAgentProposalResult{
		SchemaVersion: "v1",
		Result:        "proposed",
		Action:        action,
		Answer:        "The dose is waiting for the owner to record it in ZeitBoard. Nothing is recorded until they do, and it lapses after a day.",
		Proposals:     []localAgentProposalSummary{{ProposalID: proposal.ProposalID, Status: "pending"}},
		Approval:      "Pending proposals require a human decision in ZeitBoard; this tool cannot approve or apply them.",
	}, nil
}

// DoseProposalDTO is one proposed dose, with the medication's private label
// attached for this computer's screen only.
type DoseProposalDTO struct {
	ProposalID      string `json:"proposalId"`
	Title           string `json:"title"`
	MedicationID    string `json:"medicationId"`
	MedicationLabel string `json:"medicationLabel"`
	Status          string `json:"status"`
	// DoseAt is the instant; ZoneID is the zone it was proposed in.
	DoseAt    string `json:"doseAt"`
	ZoneID    string `json:"zoneId"`
	State     string `json:"state"`
	CreatedAt string `json:"createdAt"`
	ExpiresAt string `json:"expiresAt"`
	DecidedAt string `json:"decidedAt,omitempty"`
}

// DoseProposalsDTO is the queue: what waits for the owner, and what they or
// the clock decided lately.
type DoseProposalsDTO struct {
	SchemaVersion string            `json:"schemaVersion"`
	Pending       []DoseProposalDTO `json:"pending"`
	History       []DoseProposalDTO `json:"history"`
	NextExpiryAt  string            `json:"nextExpiryAt"`
}

// DoseProposalDecisionInput records or discards one proposed dose.
type DoseProposalDecisionInput struct {
	ProposalID string `json:"proposalId"`
	Decision   string `json:"decision"`
}

// GetDoseProposals reads the queue.
func (a *App) GetDoseProposals() (DoseProposalsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return DoseProposalsDTO{}, err
	}
	return a.doseProposals(context.Background(), store, a.currentTime())
}

// DecideDoseProposal records a proposed dose as the owner's own, or discards
// it, and returns the queue.
func (a *App) DecideDoseProposal(input DoseProposalDecisionInput) (DoseProposalsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return DoseProposalsDTO{}, err
	}
	ctx := context.Background()
	now := a.currentTime().UTC().Truncate(time.Second)
	proposalID := strings.TrimSpace(input.ProposalID)
	switch input.Decision {
	case "record":
		_, err = store.RecordProposedDose(ctx, proposalID, newLocalID("dose"), now)
	case "discard":
		err = store.DiscardProposedDose(ctx, proposalID, now)
	default:
		return DoseProposalsDTO{}, errors.New("a proposed dose is either recorded or discarded")
	}
	switch {
	case errors.Is(err, storage.ErrDoseProposalExpired):
		return DoseProposalsDTO{}, errors.New("this dose lapsed before it was decided, so nothing was recorded")
	case errors.Is(err, storage.ErrDoseProposalDecided):
		return DoseProposalsDTO{}, errors.New("this dose was already decided")
	case errors.Is(err, storage.ErrDoseProposalNotFound):
		return DoseProposalsDTO{}, errors.New("this dose is no longer waiting; its medication may have been deleted")
	case err != nil:
		return DoseProposalsDTO{}, err
	}
	return a.doseProposals(ctx, store, now)
}

func (a *App) doseProposals(ctx context.Context, store *storage.Store, now time.Time) (DoseProposalsDTO, error) {
	proposals, err := store.DoseProposals(ctx, now)
	if err != nil {
		return DoseProposalsDTO{}, err
	}
	medications, err := store.ListMedications(ctx)
	if err != nil {
		return DoseProposalsDTO{}, err
	}
	labels := make(map[string]string, len(medications))
	for _, medication := range medications {
		labels[medication.MedicationID] = medication.Label
	}
	result := DoseProposalsDTO{SchemaVersion: "v1", Pending: []DoseProposalDTO{}, History: []DoseProposalDTO{}}
	for _, proposal := range proposals {
		dto := doseProposalDTO(proposal, labels[proposal.MedicationID])
		if proposal.State != storage.DoseProposalPending {
			if len(result.History) < doseProposalHistoryShown {
				result.History = append(result.History, dto)
			}
			continue
		}
		result.Pending = append(result.Pending, dto)
		if result.NextExpiryAt == "" || proposal.ExpiresAt.Format(time.RFC3339) < result.NextExpiryAt {
			result.NextExpiryAt = proposal.ExpiresAt.UTC().Format(time.RFC3339)
		}
	}
	return result, nil
}

func doseProposalDTO(proposal storage.DoseProposal, label string) DoseProposalDTO {
	dto := DoseProposalDTO{
		ProposalID:      proposal.ProposalID,
		Title:           agentactions.CardTitle("propose_log_dose"),
		MedicationID:    proposal.MedicationID,
		MedicationLabel: label,
		Status:          proposal.Status,
		DoseAt:          proposal.DoseAt.UTC().Format(time.RFC3339),
		ZoneID:          proposal.ZoneID,
		State:           proposal.State,
		CreatedAt:       proposal.CreatedAt.UTC().Format(time.RFC3339),
		ExpiresAt:       proposal.ExpiresAt.UTC().Format(time.RFC3339),
	}
	if !proposal.DecidedAt.IsZero() {
		dto.DecidedAt = proposal.DecidedAt.UTC().Format(time.RFC3339)
	}
	return dto
}
