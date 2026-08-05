// Package portalbridge is the owner-side half of the portal boundary. It is
// the only code that holds both a private read model and the portal store, and
// its single job is to narrow the former into the allowlisted snapshot defined
// in docs/portal-design.md section 5.
//
// The portal package itself never imports this one. That direction matters: a
// public handler cannot reach a sleep session by following an import.
package portalbridge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/freshness"
	"non24.app/server/internal/portal"
)

// SleepSource is the private read model, narrowed to the one call
// materialization needs.
type SleepSource interface {
	EffectiveSleepSessions(ctx context.Context) ([]domain.SleepSession, error)
}

// SnapshotSink is the portal store, narrowed to publication. Nothing here can
// read the portal database back, and nothing in the portal database can be
// used to reach a sleep session.
type SnapshotSink interface {
	PublishSnapshot(ctx context.Context, profileID string, snapshot portal.Snapshot) error
}

// ProfileSource lists the profiles that should hold a fresh snapshot.
type ProfileSource interface {
	ListActiveProfiles(ctx context.Context, now time.Time) ([]portal.Profile, error)
}

type Materializer struct {
	Sleep     SleepSource
	Profiles  ProfileSource
	Sink      SnapshotSink
	Estimator estimation.RobustEstimator
	Now       func() time.Time
}

func (m Materializer) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

func (m Materializer) estimator() estimation.RobustEstimator {
	if m.Estimator.Config.MinimumEpisodes == 0 {
		return estimation.RobustEstimator{}
	}
	return m.Estimator
}

// Snapshot computes the allowlisted projection without publishing it. It is
// separated from MaterializeAll so the boundary can be tested directly on the
// value that would cross it.
func (m Materializer) Snapshot(ctx context.Context) (portal.Snapshot, error) {
	now := m.now()
	snapshot := portal.Snapshot{
		Version:     now.UnixMilli(),
		GeneratedAt: now,
	}

	sessions, err := m.Sleep.EffectiveSleepSessions(ctx)
	if err != nil {
		return portal.Snapshot{}, fmt.Errorf("read sleep sessions: %w", err)
	}

	estimate, err := m.estimator().Estimate(ctx, sessions, now)
	if err != nil {
		var refusal *estimation.EstimationRefusal
		if errors.As(err, &refusal) {
			// A typed refusal is a legitimate outcome, not a failure. The
			// refusal code and message stay private: the public surface
			// learns only that no availability can be shown.
			snapshot.Status = portal.StatusRefused
			return snapshot, nil
		}
		return portal.Snapshot{}, fmt.Errorf("estimate availability: %w", err)
	}

	// The withholding decision is made here, on the age of the evidence,
	// rather than downstream on the age of this snapshot. `GeneratedAt` is
	// when the row was written, and MaterializeAll runs after *any* accepted
	// sync push — including a task push that says nothing about sleep. Left to
	// the reader, a stale rhythm would keep reading "updated just now".
	assessment := freshness.Default().Assess(freshnessInputs(estimate, sessions, now))
	if !assessment.MayClaimCurrentState() {
		snapshot.Status = portal.StatusInsufficientData
		return snapshot, nil
	}

	windows := wakingWindows(estimate, now)
	if len(windows) == 0 {
		snapshot.Status = portal.StatusInsufficientData
		return snapshot, nil
	}
	snapshot.Status = portal.StatusAvailable
	snapshot.Windows = windows
	snapshot.HorizonEnd = windows[len(windows)-1].EndAt
	return snapshot, nil
}

// freshnessInputs derives the shared policy's inputs from an estimate and the
// sessions behind it. The freshness reason itself never crosses to the portal;
// a visitor learns only that no availability is shown.
func freshnessInputs(estimate domain.PhaseEstimate, sessions []domain.SleepSession, now time.Time) freshness.Inputs {
	in := freshness.Inputs{Now: now}
	for _, session := range sessions {
		for _, interval := range session.Intervals {
			end := interval.Interval.End.UTC
			if end.After(in.LatestSleepEnd) {
				in.LatestSleepEnd = end
			}
			// Prefer when the evidence was recorded over when the sleep
			// occurred: a record entered today about last week's sleep is
			// fresh evidence, and a week-old record of last night's sleep is
			// not. Fall back to the interval itself when provenance is absent.
			recorded := interval.EndEvidence.RecordedAt
			if session.CreatedAt.After(recorded) {
				recorded = session.CreatedAt
			}
			if recorded.IsZero() {
				recorded = end
			}
			if recorded.After(in.NewestEvidence) {
				in.NewestEvidence = recorded
			}
		}
	}
	if len(estimate.PredictedSleepWindows) > 0 {
		in.ExpectedSleepOnset = estimate.PredictedSleepWindows[0].Interval.Start.UTC
	}
	return in
}

// wakingWindows narrows predicted waking availability to instants and a zone.
// Window IDs, estimate IDs, confidence levels, explanations, and every input
// session ID are dropped here; confidence in particular is withheld because
// ADR-0022 measured the buckets inverted (High 0.61 hit rate below Medium
// 0.81), so publishing a label would misinform a visitor.
func wakingWindows(estimate domain.PhaseEstimate, now time.Time) []portal.Window {
	windows := make([]portal.Window, 0, len(estimate.PredictedWakingWindows))
	for _, window := range estimate.PredictedWakingWindows {
		interval := window.Interval
		if !interval.End.UTC.After(now) {
			continue
		}
		start := interval.Start.UTC
		if start.Before(now) {
			// Clip a window already in progress so the public page never
			// implies knowledge about a past that the visitor cannot use.
			start = now
		}
		if !interval.End.UTC.After(start) {
			continue
		}
		zoneID := interval.Start.ZoneID
		if zoneID == "" {
			zoneID = "UTC"
		}
		windows = append(windows, portal.Window{
			StartAt: start.UTC(),
			EndAt:   interval.End.UTC.UTC(),
			ZoneID:  zoneID,
		})
	}
	return windows
}

// MaterializeAll refreshes every active profile. Callers invoke it after an
// accepted sync push, after an estimate-affecting edit or erasure, and on
// daemon start.
func (m Materializer) MaterializeAll(ctx context.Context) error {
	now := m.now()
	profiles, err := m.Profiles.ListActiveProfiles(ctx, now)
	if err != nil {
		return fmt.Errorf("list portal profiles: %w", err)
	}
	if len(profiles) == 0 {
		return nil
	}
	snapshot, err := m.Snapshot(ctx)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		published := snapshot
		if !profile.Grants.WakingWindows {
			// A link without the windows grant still gets a row so the page
			// can render an honest "not shared" state rather than 404.
			published.Windows = nil
			published.HorizonEnd = time.Time{}
			published.Status = portal.StatusInsufficientData
		}
		if err := m.Sink.PublishSnapshot(ctx, profile.ID, published); err != nil {
			return fmt.Errorf("publish portal snapshot: %w", err)
		}
	}
	return nil
}
