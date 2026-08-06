// Package analysis binds the shared recompute orchestrator to the work this
// daemon actually has: recomputing the rhythm and republishing the availability
// projection.
//
// It is the only place that knows both, which keeps core/recompute free of the
// portal and the portal free of any scheduling opinion.
package analysis

import (
	"context"
	"strconv"
	"time"

	"non24.app/core/recompute"
	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
)

// Portal recomputes the availability projection.
type Portal struct {
	Materializer portalbridge.Materializer
}

var _ recompute.Analysis = Portal{}

// Prepare computes the projection and reports both fingerprints.
//
// The two answer different questions. The input fingerprint says whether the
// analysis would read the same thing, and is what makes a restart able to tell
// that a run is unnecessary. The content fingerprint says whether it would
// publish the same thing, and is what decides the stamp: an unchanged
// projection keeps the time it last changed, so a page cannot report itself as
// freshly updated on evidence that has not moved in a week.
func (p Portal) Prepare(ctx context.Context, now time.Time) (recompute.Prepared, error) {
	prep, err := p.Materializer.Prepare(ctx, now)
	if err != nil {
		return recompute.Prepared{}, err
	}

	inputs := recompute.Inputs{
		Sleep:     prep.Sessions,
		Consumers: consumerKeys(prep.Profiles),
	}

	return recompute.Prepared{
		Inputs:     inputs.Fingerprint(),
		Content:    contentFingerprint(prep.Snapshot),
		ValidUntil: prep.ExpiresAt,
		Apply: func(ctx context.Context, stamp recompute.Stamp) error {
			stamped := prep
			stamped.Snapshot.GeneratedAt = stamp.At
			stamped.Snapshot.Version = stamp.At.UnixMilli()
			return p.Materializer.Publish(ctx, stamped)
		},
	}, nil
}

// consumerKeys renders the profiles a projection is published to. They are part
// of the *input* fingerprint because a new link needs a row written even when
// the rhythm has not moved, and deliberately not part of the content
// fingerprint: creating a link for one person must not restamp everybody else's
// page as freshly updated.
//
// Only the opaque profile id and the grant flags appear. The owner's private
// label for a link lives in the private database and has no business here.
func consumerKeys(profiles []portal.Profile) []string {
	keys := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		keys = append(keys, profile.ID+"\x1f"+strconv.FormatBool(profile.Grants.WakingWindows))
	}
	return keys
}

// contentFingerprint digests what would cross the boundary. Version and
// GeneratedAt are excluded because they are the stamp itself; including them
// would make every run look like a change and defeat the whole mechanism.
func contentFingerprint(snapshot portal.Snapshot) recompute.Fingerprint {
	parts := make([]string, 0, len(snapshot.Windows)+2)
	parts = append(parts, "status\x1f"+snapshot.Status)
	parts = append(parts, "horizon\x1f"+timeKey(snapshot.HorizonEnd))
	for i, window := range snapshot.Windows {
		parts = append(parts, "window\x1f"+strconv.Itoa(i)+"\x1f"+
			timeKey(window.StartAt)+"\x1f"+
			timeKey(window.EndAt)+"\x1f"+
			window.ZoneID)
	}
	return recompute.Digest(parts)
}

func timeKey(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return strconv.FormatInt(value.UTC().UnixNano(), 10)
}
