// Package activity records privacy-minimized desktop usage evidence.
//
// What it records is a small set of behavioural state transitions — active,
// idle, locked, suspended — and how long the previous state lasted. What it
// does not record has to stay explicit, because the temptation to widen it is
// permanent: no keystrokes, no typed content, no screenshots, no browser
// history, no application or window names, and no high-frequency input stream.
// docs/privacy.md carries that commitment and this package is bounded by it.
//
// The evidence exists so the app can tell that the machine went unused for six
// hours without the user typing it in. It is one input to sleep inference, not
// a sleep record on its own.
package activity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"time"

	"non24.app/core/domain"
	"non24.app/core/ingest"
)

const SourceID domain.DataSourceID = "desktop-activity"

// Source is one platform's view of machine state. Implementations poll; they
// do not subscribe, because a polling source cannot accidentally grow a
// callback that sees more than it should.
type Source interface {
	Capabilities() ingest.Capabilities
	Sample(now time.Time) (Sample, error)
}

// SafeCollector turns platform samples into contract-shaped observations.
type SafeCollector struct {
	ZoneID string
	Config Config

	// Source is the platform adapter. Nil selects the build's default, which
	// on unsupported platforms reports no capabilities and produces only the
	// startup and shutdown transitions.
	Source Source

	// Now and Sleep exist so tests can drive the loop deterministically.
	Now   func() time.Time
	Sleep func(context.Context, time.Duration) error
}

func (SafeCollector) ID() domain.DataSourceID { return SourceID }

func (collector SafeCollector) Capabilities(context.Context) (ingest.Capabilities, error) {
	return collector.source().Capabilities(), nil
}

func (collector SafeCollector) source() Source {
	if collector.Source != nil {
		return collector.Source
	}
	return platformSource()
}

func (collector SafeCollector) now() time.Time {
	if collector.Now != nil {
		return collector.Now().UTC()
	}
	return time.Now().UTC()
}

func (collector SafeCollector) sleep(ctx context.Context, d time.Duration) error {
	if collector.Sleep != nil {
		return collector.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run polls until the context ends, appending an observation per transition.
// A shutdown transition is recorded on the way out so a clean exit is
// distinguishable from the process vanishing.
func (collector SafeCollector) Run(ctx context.Context, sink ingest.ObservationSink) (runErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	session := hex.EncodeToString(random[:])
	var sequence uint64
	config := collector.Config.withDefaults()
	source := collector.source()
	machine := NewMachine(config)
	defer func() {
		if final := machine.Close(collector.now()); len(final) > 0 {
			flushContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if err := collector.append(flushContext, sink, final, session, &sequence); err != nil {
				runErr = errors.New("activity shutdown record could not be saved")
			}
		}
	}()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		sample, err := source.Sample(collector.now())
		if err != nil {
			// A source that cannot read the machine's state is not a reason to
			// stop collecting: the next poll may succeed, and dropping the
			// loop would silently end evidence collection for the session.
			sample = Sample{At: collector.now()}
		}
		if transitions := machine.Observe(sample); len(transitions) > 0 {
			if err := collector.append(ctx, sink, transitions, session, &sequence); err != nil {
				return err
			}
		}
		if err := collector.sleep(ctx, config.PollInterval); err != nil {
			return err
		}
	}
}

func (collector SafeCollector) append(ctx context.Context, sink ingest.ObservationSink, transitions []Transition, session string, sequence *uint64) error {
	zone := collector.ZoneID
	if zone == "" {
		zone = "UTC"
	}
	observations := make([]domain.SourceObservation, 0, len(transitions))
	for _, transition := range transitions {
		*sequence += 1
		at := transition.At.UTC()
		instant, err := domain.NewZonedInstant(at, zone)
		if err != nil {
			return err
		}
		body, err := transition.encode(runtime.GOOS)
		if err != nil {
			return err
		}
		status := domain.StatusObserved
		if transition.State == StateSuspended || transition.State == StateResumed {
			status = domain.StatusInferred
		}
		observations = append(observations, domain.SourceObservation{
			ID:         domain.ObservationID(fmt.Sprintf("activity-%s-%d", session, *sequence)),
			SourceID:   SourceID,
			Kind:       "activity",
			ObservedAt: instant,
			RecordedAt: collector.now(),
			Evidence: domain.Evidence{
				Acquisition: domain.AcquisitionCollected,
				Status:      status,
				Algorithm:   CollectorVersion,
				SourceIDs:   []domain.DataSourceID{SourceID},
				RecordedAt:  collector.now(),
			},
			Payload: body,
		})
	}
	return sink.Append(ctx, observations)
}
