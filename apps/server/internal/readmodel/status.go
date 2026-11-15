package readmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"non24.app/core/presence"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

// StatusReader reads the statuses the owner reported (ADR-0070), as devices
// synced them. Which one stands is the caller's to work out, against its own
// clock.
type StatusReader struct {
	Store *store.Store
}

func (r StatusReader) ReportedStatuses(ctx context.Context) ([]presence.Report, error) {
	if r.Store == nil {
		return nil, errors.New("store is required")
	}
	highWater, err := r.Store.RecordHighWater(ctx)
	if err != nil {
		return nil, err
	}
	var reports []presence.Report
	if err := walkRecordsOfKind(ctx, r.Store, syncmodel.KindStatus, highWater, func(record syncmodel.Envelope) error {
		var report presence.Report
		if err := json.Unmarshal(record.Payload, &report); err != nil {
			return fmt.Errorf("record %s: %w", record.RecordID, err)
		}
		reports = append(reports, report)
		return nil
	}); err != nil {
		return nil, err
	}
	return reports, nil
}
