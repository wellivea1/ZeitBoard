package main

import (
	"fmt"
	"time"

	"non24.app/core/domain"
	storage "non24.app/core/storage/sqlite"
)

// The dose history is read a page at a time (architecture review #12): each
// dose shown is placed against the rhythm, and only the doses on the page are.
// Every medication carries its newest dose for the quick taps, so nothing
// needs the whole history in the window.

const medicationHistoryPageSize = 50

type MedicationHistoryPageInput struct {
	// Page is zero-based. A page past the end reads as the last one.
	Page int `json:"page"`
}

type MedicationHistoryPageDTO struct {
	Status   string             `json:"status"`
	Message  string             `json:"message"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
	Events   []MedicationLogDTO `json:"events"`
}

// GetMedicationHistoryPage reads one page of the dose history, newest first.
func (a *App) GetMedicationHistoryPage(input MedicationHistoryPageInput) (MedicationHistoryPageDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return MedicationHistoryPageDTO{}, err
	}
	ctx := a.applicationContext()
	medications, err := store.ListMedications(ctx)
	if err != nil {
		return MedicationHistoryPageDTO{}, err
	}
	effective, err := store.EffectiveMedicationEvents(ctx)
	if err != nil {
		return MedicationHistoryPageDTO{}, err
	}
	total := len(effective)
	page := max(input.Page, 0)
	if total > 0 && page*medicationHistoryPageSize >= total {
		// Past the end, as after deleting the only dose on the last page.
		page = (total - 1) / medicationHistoryPageSize
	}
	dto := MedicationHistoryPageDTO{
		Status: "ready", Total: total, Page: page, PageSize: medicationHistoryPageSize,
		Message: fmt.Sprintf("%d recorded %s.", total, plural(total, "dose", "doses")),
		Events:  []MedicationLogDTO{},
	}
	if total == 0 {
		dto.Status, dto.Message = "empty", "No doses recorded yet."
		return dto, nil
	}
	state, err := a.localEstimate(ctx, a.currentTime().UTC().Truncate(time.Second))
	if err != nil {
		return MedicationHistoryPageDTO{}, err
	}
	projection := newMedicationProjection(medications, state)
	// Oldest first in the store: the page counts back from the end.
	newest := total - 1 - page*medicationHistoryPageSize
	for index := newest; index >= 0 && index > newest-medicationHistoryPageSize; index-- {
		dose, err := projection.dose(effective[index])
		if err != nil {
			return MedicationHistoryPageDTO{}, err
		}
		dto.Events = append(dto.Events, dose)
	}
	return dto, nil
}

// medicationProjection places doses against the rhythm. What that needs is
// worked out once, for however many doses are shown.
type medicationProjection struct {
	labels     map[string]string
	state      localEstimateState
	sleepIndex medicationSleepIndex
	anchors    []domain.WakeAnchor
	latestWake *domain.WakeAnchor
}

func newMedicationProjection(medications []storage.MedicationRecord, state localEstimateState) medicationProjection {
	labels := make(map[string]string, len(medications))
	for _, record := range medications {
		labels[record.MedicationID] = record.Label
	}
	anchors := medicationWakeAnchors(state.Sessions)
	return medicationProjection{
		labels: labels, state: state, sleepIndex: newMedicationSleepIndex(state.Sessions),
		anchors: anchors, latestWake: latestMedicationWake(anchors),
	}
}

func (p medicationProjection) dose(item storage.EffectiveMedicationEvent) (MedicationLogDTO, error) {
	label, exists := p.labels[item.Event.MedicationID]
	if !exists {
		return MedicationLogDTO{}, fmt.Errorf("medication event %s has no definition", item.Event.EventID)
	}
	return medicationEventDTO(item, label, p.state, p.sleepIndex, p.anchors, p.latestWake), nil
}
