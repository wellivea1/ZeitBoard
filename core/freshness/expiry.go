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
// Both deadlines are inclusive, so a run scheduled for the returned instant
// finds the state changed. A zero return means nothing further changes on its
// own: either there is no evidence at all, or the state is already withheld.
func (p Policy) NextChange(in Inputs) time.Time {
	cautionAt, withheldAt := p.Deadlines(in)
	now := in.Now.UTC()
	switch {
	case cautionAt.After(now):
		return cautionAt
	case withheldAt.After(now):
		return withheldAt
	}
	return time.Time{}
}
