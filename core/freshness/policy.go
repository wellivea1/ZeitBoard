// Package freshness decides whether a current-state claim about the user's
// rhythm may be made at all.
//
// It exists because every surface had invented its own answer, and two of them
// were wrong in the same way: they measured how recently the *analysis* ran
// rather than how recently the *evidence* arrived. An estimator that recomputes
// on every screen load always looks current, and a portal snapshot rewritten
// because an unrelated task synced also looks current, while the newest sleep
// observation underneath is days old.
//
// The rule is therefore keyed on evidence, not computation, and it is shared by
// the desktop, the server projection, the local agent, and the portal so the
// four cannot drift apart.
package freshness

import (
	"fmt"
	"time"
)

// State is the decision. Callers must not present a current-state claim in any
// state other than Current, and must show the qualification in Stale.
type State string

const (
	// StateCurrent means the claim may be made plainly.
	StateCurrent State = "current"

	// StateStale means the claim may be made with the age stated prominently.
	StateStale State = "stale"

	// StateWithheld means no current-state claim may be made. An honest
	// "unknown" is more useful than a confident answer built on old evidence.
	StateWithheld State = "withheld"
)

// Reason is a closed set so that a surface can explain a withholding without
// inventing wording, and so tests can assert on the cause rather than prose.
type Reason string

const (
	ReasonFresh Reason = ""

	// ReasonNoEvidence: nothing has ever been recorded.
	ReasonNoEvidence Reason = "no_evidence"

	// ReasonEvidenceAging: past the stale threshold but still usable.
	ReasonEvidenceAging Reason = "evidence_aging"

	// ReasonEvidenceStale: past the withhold threshold.
	ReasonEvidenceStale Reason = "evidence_stale"

	// ReasonExpectedSleepUnrecorded: the estimate said sleep should already
	// have happened and nothing recorded it. This is the case a pure age
	// threshold misses, and it is exactly how "Likely awake" survives for
	// days: the last wake is old but not old enough to trip a 24-hour rule,
	// while the sleep that must have occurred since is simply absent.
	ReasonExpectedSleepUnrecorded Reason = "expected_sleep_unrecorded"

	// ReasonNoSourcesReporting: sources are configured but none is delivering.
	ReasonNoSourcesReporting Reason = "no_sources_reporting"
)

// Policy is the threshold set. The zero value is not usable; use Default.
type Policy struct {
	// StaleAfter is how old the newest evidence may be before a claim must
	// carry its age.
	StaleAfter time.Duration

	// WithholdAfter is how old the newest evidence may be before no
	// current-state claim may be made at all.
	WithholdAfter time.Duration

	// SleepOverdueGrace is how far past a predicted sleep onset the user may
	// be before an unrecorded sleep withholds the claim. It absorbs ordinary
	// forecast error rather than firing on every late night; ADR-0022
	// measured a 1.71 h median and 5.41 h P90 onset error, so a grace shorter
	// than the P90 would withhold constantly on correct behaviour.
	SleepOverdueGrace time.Duration
}

// Default matches the thresholds the availability portal already publishes, so
// adopting the shared policy does not silently change what visitors were told.
func Default() Policy {
	return Policy{
		StaleAfter:        6 * time.Hour,
		WithholdAfter:     24 * time.Hour,
		SleepOverdueGrace: 6 * time.Hour,
	}
}

// Inputs describe what a surface knows when it wants to make a claim.
type Inputs struct {
	Now time.Time

	// NewestEvidence is the recorded-at time of the newest observation that
	// could change the current-state answer — a sleep episode or a correction
	// to one. It is deliberately not the time the estimate was computed.
	NewestEvidence time.Time

	// ExpectedSleepOnset is the start of the next predicted sleep window as of
	// the last analysis. Zero when the estimator refused or none exists.
	ExpectedSleepOnset time.Time

	// LatestSleepEnd is the end of the newest recorded principal sleep. It is
	// used to tell "sleep already recorded since the prediction" from "sleep
	// predicted and never recorded".
	LatestSleepEnd time.Time

	// SourcesConfigured and SourcesReporting describe evidence breadth. Zero
	// configured sources means the user has not set any up, which is not a
	// failure; configured-but-silent sources are.
	SourcesConfigured int
	SourcesReporting  int
}

// Assessment is the shared result. Every surface renders from this rather than
// from its own arithmetic.
type Assessment struct {
	State  State
	Reason Reason

	// EvidenceAge is the age of NewestEvidence, or zero when there is none.
	EvidenceAge time.Duration

	// HasEvidence is false when nothing has ever been recorded, which callers
	// must distinguish from "recorded long ago".
	HasEvidence bool

	// Explanation is plain language for the person reading it. It never names
	// a medication, a task, a calendar entry, or a diagnosis, so it is safe on
	// the public surface as well as the private ones.
	Explanation string
}

// MayClaimCurrentState reports whether a "likely awake / likely asleep"
// statement is permitted at all.
func (a Assessment) MayClaimCurrentState() bool {
	return a.State == StateCurrent || a.State == StateStale
}

// Assess applies the policy. It is pure: no clock, no I/O, no storage.
func (p Policy) Assess(in Inputs) Assessment {
	if p.StaleAfter <= 0 || p.WithholdAfter <= 0 {
		p = Default()
	}
	now := in.Now.UTC()

	if in.NewestEvidence.IsZero() {
		return Assessment{
			State:       StateWithheld,
			Reason:      ReasonNoEvidence,
			Explanation: "There is no sleep history recorded yet, so there is nothing to base a current state on.",
		}
	}

	age := now.Sub(in.NewestEvidence.UTC())
	if age < 0 {
		// Evidence recorded in the future is a clock problem, not freshness.
		// Treat it as current rather than inventing a negative age.
		age = 0
	}
	assessment := Assessment{EvidenceAge: age, HasEvidence: true}

	switch {
	case age >= p.WithholdAfter:
		assessment.State = StateWithheld
		assessment.Reason = ReasonEvidenceStale
		assessment.Explanation = fmt.Sprintf(
			"The newest sleep record is %s old, so the current state is not shown. An out-of-date answer is worse than none.",
			describeAge(age))
		return assessment

	case p.sleepIsOverdue(in, now):
		// Deliberately checked before the aging branch: an unrecorded sleep is
		// a stronger signal than an age threshold, and the reason is more
		// useful to the person reading it.
		assessment.State = StateWithheld
		assessment.Reason = ReasonExpectedSleepUnrecorded
		assessment.Explanation = "Sleep was expected by now and none has been recorded, so the current state is unknown rather than awake."
		return assessment

	case in.SourcesConfigured > 0 && in.SourcesReporting == 0:
		assessment.State = StateWithheld
		assessment.Reason = ReasonNoSourcesReporting
		assessment.Explanation = "No configured source is reporting, so the current state cannot be confirmed."
		return assessment

	case age >= p.StaleAfter:
		assessment.State = StateStale
		assessment.Reason = ReasonEvidenceAging
		assessment.Explanation = fmt.Sprintf(
			"The newest sleep record is %s old, so treat this with extra caution.",
			describeAge(age))
		return assessment
	}

	assessment.State = StateCurrent
	assessment.Reason = ReasonFresh
	assessment.Explanation = "Based on recent records."
	return assessment
}

// sleepIsOverdue reports whether the estimate predicted a sleep onset that has
// comfortably passed with no sleep recorded since.
func (p Policy) sleepIsOverdue(in Inputs, now time.Time) bool {
	if in.ExpectedSleepOnset.IsZero() {
		return false
	}
	deadline := in.ExpectedSleepOnset.UTC().Add(p.SleepOverdueGrace)
	if !now.After(deadline) {
		return false
	}
	// Sleep recorded at or after the predicted onset discharges the
	// expectation: the user slept, and the estimate simply has not caught up.
	return in.LatestSleepEnd.IsZero() || in.LatestSleepEnd.UTC().Before(in.ExpectedSleepOnset.UTC())
}

// describeAge renders a duration the way a person would say it.
func describeAge(age time.Duration) string {
	switch {
	case age < 2*time.Minute:
		return "under a minute"
	case age < time.Hour:
		return fmt.Sprintf("%d minutes", int(age.Minutes()))
	case age < 2*time.Hour:
		return "about an hour"
	case age < 48*time.Hour:
		return fmt.Sprintf("about %d hours", int(age.Hours()))
	default:
		return fmt.Sprintf("%d days", int(age.Hours()/24))
	}
}
