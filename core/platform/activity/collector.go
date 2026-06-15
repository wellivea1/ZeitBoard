package activity

import (
	"context"
	"encoding/json"
	"runtime"
	"time"

	"non24.app/core/domain"
	"non24.app/core/ingest"
)

const SourceID domain.DataSourceID = "desktop-activity"

type SafeCollector struct {
	ZoneID string
}

func (SafeCollector) ID() domain.DataSourceID { return SourceID }

func (SafeCollector) Capabilities(context.Context) (ingest.Capabilities, error) {
	return platformCapabilities(), nil
}

func (collector SafeCollector) Run(ctx context.Context, sink ingest.ObservationSink) error {
	zone := collector.ZoneID
	if zone == "" {
		zone = "UTC"
	}
	now := time.Now().UTC()
	instant, err := domain.NewZonedInstant(now, zone)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{
		"state": "startup",
		"os":    runtime.GOOS,
	})
	observation := domain.SourceObservation{
		ID:         domain.ObservationID("desktop-startup-" + now.Format("20060102T150405.000000000Z")),
		SourceID:   SourceID,
		Kind:       "activity",
		ObservedAt: instant,
		RecordedAt: now,
		Evidence: domain.Evidence{
			Acquisition: domain.AcquisitionCollected,
			Status:      domain.StatusObserved,
			SourceIDs:   []domain.DataSourceID{SourceID},
			RecordedAt:  now,
		},
		Payload: payload,
	}
	if err := sink.Append(ctx, []domain.SourceObservation{observation}); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}
