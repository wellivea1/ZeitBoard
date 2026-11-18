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
//
// # The forecast says when the next record is due
//
// The first rule counted hours from the newest record: caution after six,
// nothing after twenty-four, and nothing six hours after sleep was forecast to
// begin. A source that reports a night once it is over, as a watch does, made
// that most of every waking day and the second half of every sleep, and a
// longer cycle made it worse. Hours since the last record say little; what
// matters is whether a record that should have arrived has not.
//
// So the rule follows the forecast (ADR-0071). The records are current
// through the waking stretch and the sleep the forecast expects, whatever the
// cycle length. Once the latest likely wake of that sleep has passed with no
// record of it, the claim carries a caution for a grace period, and after
// that no claim is made. The fixed hours remain for the one case with no
// forecast to follow.
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

	// ReasonEvidenceAging: past the stale threshold but still usable. Only
	// where there is no forecast to follow.
	ReasonEvidenceAging Reason = "evidence_aging"

	// ReasonEvidenceStale: past the withhold threshold. Only where there is
	// no forecast to follow.
	ReasonEvidenceStale Reason = "evidence_stale"

	// ReasonSleepRecordDue: the latest likely wake of the forecast sleep has
	// passed and no record of that sleep has arrived yet. A watch that has
	// not synced looks like this, and so does a night nobody recorded, so the
	// claim stands with a caution for the grace period.
	ReasonSleepRecordDue Reason = "sleep_record_due"

	// ReasonExpectedSleepUnrecorded: the forecast sleep is over, the grace
	// has run out, and nothing recorded it. This is the case a pure age
	// threshold misses, and it is exactly how "Likely awake" survives for
	// days: the sleep that must have occurred since the last record is
	// simply absent.
	ReasonExpectedSleepUnrecorded Reason = "expected_sleep_unrecorded"

	// ReasonNoSourcesReporting: sources are configured but none is delivering.
	ReasonNoSourcesReporting Reason = "no_sources_reporting"
)

// Policy is the threshold set. The zero value is not usable; use Default.
type Policy struct {
	// RecordOverdueGrace is how long a claim may stand, with its caution,
	// after the latest likely wake of the forecast sleep has passed with no
	// record of that sleep. It absorbs ordinary forecast error and a source
	// that syncs late; ADR-0022 measured a 5.41 h P90 error, so a grace
	// shorter than that would withhold on correct behaviour.
	RecordOverdueGrace time.Duration

	// StaleAfter is how old the newest evidence may be before a claim must
	// carry its age, where there is no forecast to follow.
	StaleAfter time.Duration

	// WithholdAfter is how old the newest evidence may be before no
	// current-state claim may be made at all, where there is no forecast to
	// follow.
	WithholdAfter time.Duration
}

// Default is the owner's choice of 2026-11-17 (ADR-0071): follow the
// forecast, with six hours of caution once a record is overdue. The fixed
// hours are ADR-0031's, kept for evidence that has no forecast beside it.
func Default() Policy {
	return Policy{
		RecordOverdueGrace: 6 * time.Hour,
		StaleAfter:         6 * time.Hour,
		WithholdAfter:      24 * time.Hour,
	}
}

// Inputs describe what a surface knows when it wants to make a claim.
type Inputs struct {
	Now time.Time

	// NewestEvidence is the recorded-at time of the newest observation that
	// could change the current-state answer — a sleep episode or a correction
	// to one. It is deliberately not the time the estimate was computed.
	NewestEvidence time.Time

	// ExpectedSleepOnset and ExpectedSleepEnd are the first predicted sleep
	// window of the last analysis: its earliest likely onset and its latest
	// likely wake. That window is the first sleep after the newest recorded
	// one, so it is the sleep whose record is awaited. Both are zero when
	// the estimator refused or there is no forecast.
	ExpectedSleepOnset time.Time
	ExpectedSleepEnd   time.Time

	// LatestSleepEnd is the end of the newest recorded sleep. It is used to
	// tell "sleep already recorded since the prediction" from "sleep
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
	p = p.orDefault()
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
	due := p.deadlines(in)
	withheld := !now.Before(due.withheldAt)

	switch {
	case withheld && due.forecast:
		assessment.State = StateWithheld
		assessment.Reason = ReasonExpectedSleepUnrecorded
		assessment.Explanation = "A sleep was expected to be over by now and no record of it has arrived, so the current state is unknown."

	case withheld:
		assessment.State = StateWithheld
		assessment.Reason = ReasonEvidenceStale
		assessment.Explanation = fmt.Sprintf(
			"The newest sleep record is %s old, so the current state is not shown. An out-of-date answer is worse than none.",
			describeAge(age))

	case in.SourcesConfigured > 0 && in.SourcesReporting == 0:
		assessment.State = StateWithheld
		assessment.Reason = ReasonNoSourcesReporting
		assessment.Explanation = "No configured source is reporting, so the current state cannot be confirmed."

	case now.Before(due.cautionAt):
		assessment.State = StateCurrent
		assessment.Reason = ReasonFresh
		assessment.Explanation = "Based on recent records."

	case due.forecast:
		assessment.State = StateStale
		assessment.Reason = ReasonSleepRecordDue
		assessment.Explanation = "A sleep was expected to be over by now and no record of it has arrived yet, so treat this with extra caution."

	default:
		assessment.State = StateStale
		assessment.Reason = ReasonEvidenceAging
		assessment.Explanation = fmt.Sprintf(
			"The newest sleep record is %s old, so treat this with extra caution.",
			describeAge(age))
	}
	return assessment
}

func (p Policy) orDefault() Policy {
	if p.RecordOverdueGrace <= 0 || p.StaleAfter <= 0 || p.WithholdAfter <= 0 {
		return Default()
	}
	return p
}

// deadlines are the two instants the verdict turns on, for evidence that has
// already arrived: from cautionAt the claim carries its caution, and from
// withheldAt no claim is made. Both are inclusive.
type deadlines struct {
	cautionAt  time.Time
	withheldAt time.Time

	// forecast says the deadlines follow the forecast sleep, and not the age
	// of the evidence.
	forecast bool
}

func (p Policy) deadlines(in Inputs) deadlines {
	if awaited, ok := awaitedSleepEnd(in); ok {
		return deadlines{cautionAt: awaited, withheldAt: awaited.Add(p.RecordOverdueGrace), forecast: true}
	}
	evidence := in.NewestEvidence.UTC()
	return deadlines{cautionAt: evidence.Add(p.StaleAfter), withheldAt: evidence.Add(p.WithholdAfter)}
}

// awaitedSleepEnd is the latest likely wake of the forecast sleep, while no
// record of that sleep has arrived.
//
// Sleep recorded at or after the forecast onset discharges the expectation:
// the person slept, and the estimate has not caught up. Until it does there is
// no forecast to follow, and the age of the evidence decides.
func awaitedSleepEnd(in Inputs) (time.Time, bool) {
	if in.ExpectedSleepOnset.IsZero() || !in.ExpectedSleepEnd.After(in.ExpectedSleepOnset) {
		return time.Time{}, false
	}
	if !in.LatestSleepEnd.IsZero() && !in.LatestSleepEnd.UTC().Before(in.ExpectedSleepOnset.UTC()) {
		return time.Time{}, false
	}
	return in.ExpectedSleepEnd.UTC(), true
}

// Deadlines reports when the verdict on these inputs turns with no new
// evidence: from cautionAt a claim carries its caution, and from withheldAt
// none is made. A reader that cannot ask again, such as a published page,
// keeps them beside what it shows. Both are zero when there is no evidence,
// which never becomes fresh by waiting.
func (p Policy) Deadlines(in Inputs) (cautionAt, withheldAt time.Time) {
	if in.NewestEvidence.IsZero() {
		return time.Time{}, time.Time{}
	}
	found := p.orDefault().deadlines(in)
	return found.cautionAt, found.withheldAt
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
