package freshness

import "time"

// NextChange reports the earliest instant at which Assess could return a
// different State for the same inputs.
//
// It exists because this policy is a function of time as well as evidence, and
// nothing was re-evaluating it. ADR-0031 moved the withholding decision to the
// moment a projection is materialized, which is correct and incomplete: the
// decision then only happens when something else causes a materialization. A
// user who records nothing all day causes nothing, which is exactly the case the
// policy was written for. A caller that schedules a recompute at this instant
// closes that gap.
//
// The guarantee is one-sided on purpose: the returned time is never later than
// the real transition, and may be earlier. An early wake recomputes the same
// answer and publishes it idempotently, which costs a few milliseconds. A late
// one would leave a stale claim standing, which is the defect.
//
// A zero return means nothing further changes on its own — either there is no
// evidence at all, or the state is already withheld and no threshold remains.
func (p Policy) NextChange(in Inputs) time.Time {
	if p.StaleAfter <= 0 || p.WithholdAfter <= 0 {
		p = Default()
	}
	if in.NewestEvidence.IsZero() {
		// No evidence never becomes fresh by waiting. Only an arrival changes
		// this, and an arrival is an input change, not an expiry.
		return time.Time{}
	}
	now := in.Now.UTC()
	evidence := in.NewestEvidence.UTC()

	var next time.Time
	consider := func(candidate time.Time) {
		if candidate.IsZero() || !candidate.After(now) {
			return
		}
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}

	consider(evidence.Add(p.StaleAfter))
	consider(evidence.Add(p.WithholdAfter))

	// The unrecorded-sleep deadline only exists while the expectation is still
	// outstanding. Sleep recorded at or after the predicted onset discharges it
	// permanently, matching sleepIsOverdue.
	if !in.ExpectedSleepOnset.IsZero() {
		onset := in.ExpectedSleepOnset.UTC()
		if in.LatestSleepEnd.IsZero() || in.LatestSleepEnd.UTC().Before(onset) {
			// One tick past the deadline, not on it. The age thresholds are
			// inclusive (`age >= StaleAfter`) but sleepIsOverdue is strict, so
			// a run scheduled for the exact instant would find nothing changed
			// and schedule its next wake from the wrong state — leaving the
			// claim standing until some later threshold happened to catch it.
			consider(onset.Add(p.SleepOverdueGrace).Add(time.Nanosecond))
		}
	}

	return next
}
