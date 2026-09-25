package ingest

import (
	"context"
	"errors"
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

type exitingCollector struct{ runs chan struct{} }

func (exitingCollector) ID() domain.DataSourceID { return "exiting" }
func (exitingCollector) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{}, nil
}
func (c exitingCollector) Run(context.Context, ObservationSink) error {
	c.runs <- struct{}{}
	return errors.New("synthetic disk failure")
}

func TestExitedCollectorReportsStoppedAndCanRestart(t *testing.T) {
	c := exitingCollector{runs: make(chan struct{}, 2)}
	m := NewManager(&MemorySink{}, c)
	for i := 0; i < 2; i++ {
		if err := m.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		select {
		case <-c.runs:
		case <-time.After(time.Second):
			t.Fatal("collector did not restart")
		}
		deadline := time.Now().Add(time.Second)
		for m.Health(context.Background()).Running && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		h := m.Health(context.Background())
		if h.Running || h.LastError != "synthetic disk failure" {
			t.Fatalf("failed collector health: %#v", h)
		}
	}
	_ = m.Stop(context.Background())
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
