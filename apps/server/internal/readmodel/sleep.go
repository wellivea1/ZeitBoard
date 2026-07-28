package readmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"non24.app/core/domain"
	"non24.app/core/sleepv1"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

type SleepReader struct {
	Store *store.Store
}

func (r SleepReader) EffectiveSleepSessions(ctx context.Context) ([]domain.SleepSession, error) {
	if r.Store == nil {
		return nil, errors.New("store is required")
	}
	highWater, err := r.Store.RecordHighWater(ctx)
	if err != nil {
		return nil, err
	}
	var observations []sleepv1.Observation
	var corrections []sleepv1.Correction
	if err := walkRecordsOfKind(ctx, r.Store, syncmodel.KindObservation, highWater, func(record syncmodel.Envelope) error {
		observation, ok, err := sleepObservationFromPayload(record.Payload)
		if err != nil {
			return fmt.Errorf("record %s: %w", record.RecordID, err)
		}
		if ok {
			observations = append(observations, observation)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := walkRecordsOfKind(ctx, r.Store, syncmodel.KindCorrection, highWater, func(record syncmodel.Envelope) error {
		correction, err := sleepv1.DecodeCorrection(record.Payload)
		if err != nil {
			return fmt.Errorf("record %s: %w", record.RecordID, err)
		}
		corrections = append(corrections, correction)
		return nil
	}); err != nil {
		return nil, err
	}
	effective, err := sleepv1.Fold(observations, corrections)
	if err != nil {
		return nil, err
	}
	return sleepv1.ResolveOverlaps(effective)
}

func sleepObservationFromPayload(payload json.RawMessage) (sleepv1.Observation, bool, error) {
	var envelope struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return sleepv1.Observation{}, false, err
	}
	if envelope.Kind != sleepv1.KindEpisode {
		return sleepv1.Observation{}, false, nil
	}
	observation, err := sleepv1.DecodeObservation(payload)
	return observation, true, err
}

func walkRecordsOfKind(
	ctx context.Context,
	st *store.Store,
	kind syncmodel.Kind,
	highWater int64,
	visit func(syncmodel.Envelope) error,
) error {
	since := int64(0)
	for {
		records, cursor, err := st.PullKindThrough(ctx, kind, since, highWater, syncmodel.MaxPullRecords)
		if err != nil {
			return err
		}
		for _, record := range records {
			if err := visit(record); err != nil {
				return err
			}
		}
		if len(records) < syncmodel.MaxPullRecords || cursor == since || cursor >= highWater {
			return nil
		}
		since = cursor
	}
}
