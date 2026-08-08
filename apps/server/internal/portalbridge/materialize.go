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

// Preparation is a computed but unpublished materialization: what would cross
// the boundary, who it would be published to, the sleep history behind it, and
// when it stops being trustworthy on its own.
//
// Computing and publishing are separate so the recompute orchestrator can stamp
// the result before it is written (ADR-0033). The stamp is what stops an
// unchanged projection from advertising itself as a fresh one.
type Preparation struct {
	Snapshot portal.Snapshot
	Profiles []portal.Profile
	Sessions []domain.SleepSession

	// ExpiresAt is the earliest instant the freshness verdict could change with
	// no new evidence at all. Zero means nothing further expires by itself.
	ExpiresAt time.Time
}

// Prepare computes the projection as of `now` without publishing it.
func (m Materializer) Prepare(ctx context.Context, now time.Time) (Preparation, error) {
	now = now.UTC()
	prep := Preparation{
		Snapshot: portal.Snapshot{
			Version:     now.UnixMilli(),
			GeneratedAt: now,
		},
	}

	// A materializer without a profile source has no consumers, which is a
	// legitimate configuration: Snapshot is used on its own to assert what
	// would cross the boundary, with nothing on the far side of it.
	if m.Profiles != nil {
		profiles, err := m.Profiles.ListActiveProfiles(ctx, now)
		if err != nil {
			return Preparation{}, fmt.Errorf("list portal profiles: %w", err)
		}
		prep.Profiles = profiles
	}

	sessions, err := m.Sleep.EffectiveSleepSessions(ctx)
	if err != nil {
		return Preparation{}, fmt.Errorf("read sleep sessions: %w", err)
	}
	prep.Sessions = sessions

	estimate, err := m.estimator().Estimate(ctx, sessions, now)
	if err != nil {
		var refusal *estimation.EstimationRefusal
		if errors.As(err, &refusal) {
			// A typed refusal is a legitimate outcome, not a failure. The
			// refusal code and message stay private: the public surface
			// learns only that no availability can be shown.
			prep.Snapshot.Status = portal.StatusRefused
			return prep, nil
		}
		return Preparation{}, fmt.Errorf("estimate availability: %w", err)
	}

	// The withholding decision is made here, on the age of the evidence,
	// rather than downstream on the age of this snapshot. `GeneratedAt` is
	// when the published content last changed, and materialization runs after
	// *any* accepted sync push — including a task push that says nothing about
	// sleep. Left to the reader, a stale rhythm would keep reading "updated
	// just now".
	policy := freshness.Default()
	inputs := freshnessInputs(estimate, sessions, now)
	prep.ExpiresAt = policy.NextChange(inputs)
	if !policy.Assess(inputs).MayClaimCurrentState() {
		prep.Snapshot.Status = portal.StatusInsufficientData
		return prep, nil
	}

	windows := wakingWindows(estimate)
	if len(windows) == 0 {
		prep.Snapshot.Status = portal.StatusInsufficientData
		return prep, nil
	}
	prep.Snapshot.Status = portal.StatusAvailable
	prep.Snapshot.Windows = windows
	prep.Snapshot.HorizonEnd = windows[len(windows)-1].EndAt
	return prep, nil
}

// Snapshot computes the allowlisted projection without publishing it, at the
// materializer's own clock. Publication is deliberately not part of it, so the
// boundary can be asserted directly on the value that would cross it.
func (m Materializer) Snapshot(ctx context.Context) (portal.Snapshot, error) {
	prep, err := m.Prepare(ctx, m.now())
	if err != nil {
		return portal.Snapshot{}, err
	}
	return prep.Snapshot, nil
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
//
// Nothing here depends on the current instant. Windows used to be filtered and
// clipped against `now` at this point, which made the stored snapshot a function
// of the moment it was written and so impossible to compare against the next
// one. Both rules now live where they belong — at render, in portal.BuildView
// and the availability DTO — where they keep working as time passes instead of
// being correct only at the instant of materialization.
func wakingWindows(estimate domain.PhaseEstimate) []portal.Window {
	windows := make([]portal.Window, 0, len(estimate.PredictedWakingWindows))
	for _, window := range estimate.PredictedWakingWindows {
		interval := window.Interval
		if !interval.End.UTC.After(interval.Start.UTC) {
			continue
		}
		zoneID := interval.Start.ZoneID
		if zoneID == "" {
			zoneID = "UTC"
		}
		windows = append(windows, portal.Window{
			StartAt: interval.Start.UTC.UTC(),
			EndAt:   interval.End.UTC.UTC(),
			ZoneID:  zoneID,
		})
	}
	return windows
}

// Publish writes a prepared projection to every profile it was prepared for.
// The snapshot must already carry the stamp it is to be published under, so
// that repeating a publication rewrites the same answer rather than announcing
// a new one.
func (m Materializer) Publish(ctx context.Context, prep Preparation) error {
	for _, profile := range prep.Profiles {
		published := prep.Snapshot
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
