//go:build !windows

package activity

import (
	"time"

	"non24.app/core/ingest"
)

// unsupportedSource is the adapter for platforms without an implementation.
//
// It reports no capabilities and returns samples that assert nothing, so the
// collector still records startup, suspend/resume from clock gaps, and
// shutdown. Returning fabricated idle values instead would let a Linux build
// silently contribute evidence it never measured.
type unsupportedSource struct{}

func platformSource() Source { return unsupportedSource{} }

func (unsupportedSource) Capabilities() ingest.Capabilities { return ingest.Capabilities{} }

func (unsupportedSource) Sample(now time.Time) (Sample, error) {
	return Sample{At: now}, nil
}
