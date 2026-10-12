package medication

import (
	"fmt"
	"sort"
)

// EffectiveEvent is a dose as it now stands: its record, with every
// correction to it applied in the order they were made. The record itself is
// never rewritten (ADR-0024).
type EffectiveEvent struct {
	Event       EventRecord
	Excluded    bool
	Corrections []CorrectionRecord
}

// Effective applies each event's corrections, oldest first, and returns the
// events in dose order. The desktop and the server both read doses through it
// (ADR-0058), so a correction means the same thing wherever it is read.
func Effective(events []EventRecord, corrections []CorrectionRecord) ([]EffectiveEvent, error) {
	ordered := append([]CorrectionRecord(nil), corrections...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CorrectionID < ordered[j].CorrectionID
		}
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})
	byEvent := make(map[string][]CorrectionRecord)
	for _, correction := range ordered {
		byEvent[correction.TargetEventID] = append(byEvent[correction.TargetEventID], correction)
	}

	result := make([]EffectiveEvent, 0, len(events))
	for _, event := range events {
		item := EffectiveEvent{Event: event, Corrections: byEvent[event.EventID]}
		for _, correction := range item.Corrections {
			item.apply(correction.Changes)
		}
		if err := item.Event.Validate(); err != nil {
			return nil, fmt.Errorf("effective medication event %s: %w", event.EventID, err)
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Event.DoseAt.Equal(result[j].Event.DoseAt) {
			return result[i].Event.EventID < result[j].Event.EventID
		}
		return result[i].Event.DoseAt.Before(result[j].Event.DoseAt)
	})
	return result, nil
}

func (item *EffectiveEvent) apply(changes CorrectionChanges) {
	if changes.DoseAt != nil {
		item.Event.DoseAt = changes.DoseAt.UTC()
	}
	if changes.ZoneID != nil {
		item.Event.ZoneID = *changes.ZoneID
	}
	if changes.Status != nil {
		item.Event.Status = *changes.Status
	}
	if changes.Scheduled != nil {
		item.Event.Scheduled = *changes.Scheduled
	}
	if changes.Note != nil {
		item.Event.Note = *changes.Note
	}
	if changes.Excluded != nil {
		item.Excluded = *changes.Excluded
	}
}
