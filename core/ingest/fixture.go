package ingest

import (
	"context"
	"sync"

	"non24.app/core/domain"
)

type MemorySink struct {
	mu           sync.Mutex
	Observations []domain.SourceObservation
}

func (s *MemorySink) Append(_ context.Context, observations []domain.SourceObservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Observations = append(s.Observations, observations...)
	return nil
}

func (s *MemorySink) Snapshot() []domain.SourceObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.SourceObservation(nil), s.Observations...)
}
