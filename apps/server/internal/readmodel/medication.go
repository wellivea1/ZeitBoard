package readmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"non24.app/core/medication"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

// MedicationReader reads the doses devices have synced (ADR-0048) as they now
// stand, with each correction applied the way the desktop applies it
// (ADR-0058). Medication definitions are not read: nothing on the server needs
// a name, and the reader that would hand one out does not exist.
type MedicationReader struct {
	Store *store.Store
}

func (r MedicationReader) EffectiveDoses(ctx context.Context) ([]medication.EffectiveEvent, error) {
	if r.Store == nil {
		return nil, errors.New("store is required")
	}
	highWater, err := r.Store.RecordHighWater(ctx)
	if err != nil {
		return nil, err
	}
	var events []medication.EventRecord
	if err := walkRecordsOfKind(ctx, r.Store, syncmodel.KindMedicationEvent, highWater, func(record syncmodel.Envelope) error {
		var event medication.EventRecord
		if err := json.Unmarshal(record.Payload, &event); err != nil {
			return fmt.Errorf("record %s: %w", record.RecordID, err)
		}
		events = append(events, event)
		return nil
	}); err != nil {
		return nil, err
	}
	var corrections []medication.CorrectionRecord
	if err := walkRecordsOfKind(ctx, r.Store, syncmodel.KindMedicationCorrection, highWater, func(record syncmodel.Envelope) error {
		var correction medication.CorrectionRecord
		if err := json.Unmarshal(record.Payload, &correction); err != nil {
			return fmt.Errorf("record %s: %w", record.RecordID, err)
		}
		corrections = append(corrections, correction)
		return nil
	}); err != nil {
		return nil, err
	}
	return medication.Effective(events, corrections)
}
