package ingest

import (
	"context"

	"non24.app/core/domain"
)

type Capabilities struct {
	ActiveIdle   bool `json:"activeIdle"`
	SessionState bool `json:"sessionState"`
	PowerEvents  bool `json:"powerEvents"`
	ScreenState  bool `json:"screenState"`
}

type ObservationSink interface {
	Append(context.Context, []domain.SourceObservation) error
}

type Collector interface {
	ID() domain.DataSourceID
	Capabilities(context.Context) (Capabilities, error)
	Run(context.Context, ObservationSink) error
}

type CollectorService interface {
	Start(context.Context) error
	Stop(context.Context) error
	Health(context.Context) ServiceHealth
}

type ServiceHealth struct {
	Running      bool     `json:"running"`
	CollectorIDs []string `json:"collectorIds"`
	LastError    string   `json:"lastError,omitempty"`
}
