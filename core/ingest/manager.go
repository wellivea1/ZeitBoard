package ingest

import (
	"context"
	"errors"
	"sync"
)

type Manager struct {
	collectors []Collector
	sink       ObservationSink

	mu     sync.Mutex
	cancel context.CancelFunc
	wait   sync.WaitGroup
	health ServiceHealth
}

func NewManager(sink ObservationSink, collectors ...Collector) *Manager {
	return &Manager{collectors: collectors, sink: sink}
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return nil
	}
	runContext, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.health = ServiceHealth{Running: true}
	for _, collector := range m.collectors {
		m.health.CollectorIDs = append(m.health.CollectorIDs, string(collector.ID()))
		m.wait.Add(1)
		go func(value Collector) {
			defer m.wait.Done()
			if err := value.Run(runContext, m.sink); err != nil && !errors.Is(err, context.Canceled) {
				m.mu.Lock()
				m.health.LastError = err.Error()
				m.mu.Unlock()
			}
		}(collector)
	}
	return nil
}

func (m *Manager) Stop(context.Context) error {
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.health.Running = false
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		m.wait.Wait()
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
