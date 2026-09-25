package ingest

import (
	"context"
	"errors"
	"sync"
)

type collectorRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type Manager struct {
	collectors []Collector
	sink       ObservationSink
	lifecycle  sync.Mutex
	mu         sync.Mutex
	run        *collectorRun
	health     ServiceHealth
}

func NewManager(sink ObservationSink, collectors ...Collector) *Manager {
	return &Manager{collectors: collectors, sink: sink}
}

func (m *Manager) Start(ctx context.Context) error {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run != nil {
		select {
		case <-m.run.done:
		default:
			return nil
		}
	}
	runContext, cancel := context.WithCancel(ctx)
	run := &collectorRun{cancel: cancel, done: make(chan struct{})}
	m.run = run
	m.health = ServiceHealth{Running: len(m.collectors) > 0}
	var workers sync.WaitGroup
	for _, collector := range m.collectors {
		m.health.CollectorIDs = append(m.health.CollectorIDs, string(collector.ID()))
		workers.Add(1)
		go func(value Collector) {
			defer workers.Done()
			if err := value.Run(runContext, m.sink); err != nil && !errors.Is(err, context.Canceled) {
				m.mu.Lock()
				m.health.LastError = err.Error()
				m.mu.Unlock()
			}
		}(collector)
	}
	go func() {
		workers.Wait()
		cancel()
		m.mu.Lock()
		m.health.Running = false
		close(run.done)
		m.mu.Unlock()
	}()
	return nil
}

// Serialize lifecycle transitions, joining the old generation before starting
// another. Collector cancellation must include a bounded final flush.
func (m *Manager) Stop(context.Context) error {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	m.mu.Lock()
	run := m.run
	m.mu.Unlock()
	if run != nil {
		run.cancel()
		<-run.done
	}
	return nil
}

func (m *Manager) Health(context.Context) ServiceHealth {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := m.health
	result.CollectorIDs = append([]string(nil), m.health.CollectorIDs...)
	return result
}
