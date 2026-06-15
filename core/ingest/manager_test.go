package ingest

import (
	"context"
	"testing"
	"time"

	"non24.app/core/domain"
)

type testCollector struct{}

func (testCollector) ID() domain.DataSourceID                            { return "test" }
func (testCollector) Capabilities(context.Context) (Capabilities, error) { return Capabilities{}, nil }
func (testCollector) Run(ctx context.Context, sink ObservationSink) error {
	now := time.Now().UTC()
	if err := sink.Append(ctx, []domain.SourceObservation{{
		ID: "test-observation", SourceID: "test", Kind: "fixture",
		ObservedAt: domain.MustZonedInstant(now, "UTC"), RecordedAt: now,
	}}); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestManagerStartsAndStopsCollectors(t *testing.T) {
	sink := &MemorySink{}
	manager := NewManager(sink, testCollector{})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for len(sink.Snapshot()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := len(sink.Snapshot()); got != 1 {
		t.Fatalf("observations = %d", got)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.Health(context.Background()).Running {
		t.Fatal("manager still running")
	}
}
