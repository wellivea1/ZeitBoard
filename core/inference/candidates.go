// Package inference builds candidate sleep episodes from more than one kind of
// evidence.
//
// It exists because the app's premise is that it follows the user's rhythm
// without being told, and until now it could only know about sleep the user
// typed in. A long stretch of desktop inactivity is weak evidence on its own —
// people read, watch things, and leave machines running — so a candidate here
// is a hypothesis with stated uncertainty and named supporting and conflicting
// sources, not a sleep record.
//
// Every candidate is shadow-only. Nothing in this package may reach planning,
// the estimator, or any projection until a documented validation decision says
// the boundary errors are acceptable, on the same measured-delta gate ADR-0022
// established for estimator changes. Candidates never overwrite or merge away a
// raw observation, and a user correction remains a separate append-only layer.
package inference

import (
	"fmt"
	"sort"
	"time"

	"non24.app/core/platform/activity"
)

// AlgorithmVersion identifies these rules so evidence produced under different
// ones can be told apart later.
const AlgorithmVersion = "sleep-candidates-v1"

// Defaults for what counts as a plausible principal sleep episode. They are
// starting points to be revised from measured pilot data, not claims.
const (
	// MinimumDuration keeps an evening on the sofa from becoming a sleep
	// episode.
	MinimumDuration = 3 * time.Hour

	// MaximumDuration bounds a candidate: a machine untouched for two days is
	// evidence of absence, not of a two-day sleep.
	MaximumDuration = 14 * time.Hour
)

// SourceKind names an evidence stream. It is a closed set so a candidate's
// provenance cannot become free text.
type SourceKind string

const (
	SourceDesktopActivity SourceKind = "desktop_activity"
	SourceWearableSleep   SourceKind = "wearable_sleep"
	SourceUserConfirmed   SourceKind = "user_confirmed"
)

// Interval is a half-open span of evidence from one source.
type Interval struct {
	Source  SourceKind
	StartAt time.Time
	EndAt   time.Time

	// Asleep reports what this source claims about the span. Desktop
	// inactivity claims nothing directly, so it is expressed as a quiet
	// interval and only becomes a candidate through the rules below.
	Asleep bool
}

// Candidate is a hypothesised sleep episode.
type Candidate struct {
	StartAt time.Time
	EndAt   time.Time

	// StartUncertainty and EndUncertainty bound each boundary. Desktop
	// evidence cannot see when someone actually fell asleep, only when they
	// stopped using the machine, so the uncertainty is part of the claim
	// rather than a footnote.
	StartUncertainty time.Duration
	EndUncertainty   time.Duration

	Supporting  []SourceKind
	Conflicting []SourceKind

	AlgorithmVersion string
}

// Refusal is a typed reason no candidate could be produced. The estimator's
// honesty about insufficient evidence applies here too.
type Refusal struct {
	Code    string
	Message string
}

func (r *Refusal) Error() string { return r.Message }

const (
	RefusalNoEvidence      = "no_evidence"
	RefusalNoQuietInterval = "no_quiet_interval"
)

// FromActivity turns a run of activity transitions into quiet intervals: spans
// during which the machine was not being used. It makes no claim about sleep.
func FromActivity(transitions []activity.Transition) []Interval {
	sorted := append([]activity.Transition(nil), transitions...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].At.Before(sorted[j].At) })

	var intervals []Interval
	var quietFrom time.Time
	quiet := false

	for _, transition := range sorted {
		switch transition.State {
		case activity.StateIdle, activity.StateLocked, activity.StateSuspended:
			if !quiet {
				quiet = true
				quietFrom = transition.At.UTC()
			}
		case activity.StateActive, activity.StateUnlocked, activity.StateResumed, activity.StateStartup:
			if quiet {
				intervals = append(intervals, Interval{
					Source:  SourceDesktopActivity,
					StartAt: quietFrom,
					EndAt:   transition.At.UTC(),
				})
				quiet = false
			}
		case activity.StateShutdown:
			// A shutdown ends the observable record rather than opening a
			// quiet interval: the machine being off says nothing about the
			// person, and treating it as evidence would turn every holiday
			// into a fortnight of sleep.
			if quiet {
				intervals = append(intervals, Interval{
					Source:  SourceDesktopActivity,
					StartAt: quietFrom,
					EndAt:   transition.At.UTC(),
				})
				quiet = false
			}
		}
	}
	return intervals
}

// Build produces candidates from the supplied evidence.
//
// A quiet desktop interval of plausible length becomes a candidate. A wearable
// or user-confirmed sleep interval overlapping it raises confidence and tightens
// the boundaries; one that contradicts it is recorded as conflicting rather than
// silently discarded, because a disagreement between sources is exactly what a
// person should be asked about later.
func Build(intervals []Interval, now time.Time) ([]Candidate, *Refusal) {
	if len(intervals) == 0 {
		return nil, &Refusal{Code: RefusalNoEvidence, Message: "no evidence was supplied"}
	}

	var quiet, asserted []Interval
	for _, interval := range intervals {
		if !interval.EndAt.After(interval.StartAt) {
			continue
		}
		if interval.Asleep {
			asserted = append(asserted, interval)
			continue
		}
		if interval.Source == SourceDesktopActivity {
			quiet = append(quiet, interval)
		}
	}

	candidates := make([]Candidate, 0, len(quiet)+len(asserted))

	// A source that directly asserts sleep is the stronger claim, so it forms
	// the candidate and desktop evidence corroborates or contradicts it.
	for _, sleep := range asserted {
		candidate := Candidate{
			StartAt:          sleep.StartAt.UTC(),
			EndAt:            sleep.EndAt.UTC(),
			StartUncertainty: assertedUncertainty(sleep.Source),
			EndUncertainty:   assertedUncertainty(sleep.Source),
			Supporting:       []SourceKind{sleep.Source},
			AlgorithmVersion: AlgorithmVersion,
		}
		for _, span := range quiet {
			if overlaps(span, sleep) {
				candidate.Supporting = appendSource(candidate.Supporting, span.Source)
			}
		}
		// Desktop use *during* an asserted sleep would be a genuine
		// contradiction, but FromActivity reports quiet intervals only, so
		// active spans are not available here. Detecting that conflict needs
		// the active side of the record and is deliberately left undone rather
		// than approximated.
		candidates = append(candidates, candidate)
	}

	for _, span := range quiet {
		duration := span.EndAt.Sub(span.StartAt)
		if duration < MinimumDuration || duration > MaximumDuration {
			continue
		}
		if coveredByAsserted(span, asserted) {
			continue
		}
		candidate := Candidate{
			StartAt: span.StartAt.UTC(),
			EndAt:   span.EndAt.UTC(),
			// Inactivity brackets sleep rather than marking it: someone stops
			// using the machine before falling asleep and uses it again some
			// time after waking. The uncertainty says so rather than implying
			// the boundaries are the sleep itself.
			StartUncertainty: 45 * time.Minute,
			EndUncertainty:   30 * time.Minute,
			Supporting:       []SourceKind{SourceDesktopActivity},
			AlgorithmVersion: AlgorithmVersion,
		}
		for _, sleep := range asserted {
			if overlaps(span, sleep) {
				candidate.Supporting = appendSource(candidate.Supporting, sleep.Source)
			} else if sameNight(span, sleep) {
				candidate.Conflicting = appendSource(candidate.Conflicting, sleep.Source)
			}
		}
		candidates = append(candidates, candidate)
	}

	if len(candidates) == 0 {
		return nil, &Refusal{
			Code:    RefusalNoQuietInterval,
			Message: "no evidence interval was long enough to suggest a principal sleep episode",
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].StartAt.Before(candidates[j].StartAt) })
	return candidates, nil
}

// assertedUncertainty reflects how well a source knows its own boundaries.
func assertedUncertainty(source SourceKind) time.Duration {
	switch source {
	case SourceUserConfirmed:
		return 15 * time.Minute
	case SourceWearableSleep:
		return 20 * time.Minute
	default:
		return 45 * time.Minute
	}
}

func overlaps(a, b Interval) bool {
	return a.StartAt.Before(b.EndAt) && b.StartAt.Before(a.EndAt)
}

// sameNight reports whether two intervals are close enough that disagreeing
// about them is a conflict rather than two unrelated events.
func sameNight(a, b Interval) bool {
	const window = 12 * time.Hour
	gap := a.StartAt.Sub(b.StartAt)
	if gap < 0 {
		gap = -gap
	}
	return gap <= window
}

func coveredByAsserted(span Interval, asserted []Interval) bool {
	for _, sleep := range asserted {
		if overlaps(span, sleep) {
			return true
		}
	}
	return false
}

func appendSource(sources []SourceKind, source SourceKind) []SourceKind {
	for _, existing := range sources {
		if existing == source {
			return sources
		}
	}
	return append(sources, source)
}

// Describe renders a candidate for a correction prompt. It states what
// disagreed rather than asking the user to adjudicate raw data.
func (c Candidate) Describe() string {
	if len(c.Conflicting) == 0 {
		return fmt.Sprintf("Possible sleep from %s to %s, from %s.",
			c.StartAt.Format("15:04"), c.EndAt.Format("15:04"), joinSources(c.Supporting))
	}
	return fmt.Sprintf("Possible sleep from %s to %s, from %s, but %s disagrees.",
		c.StartAt.Format("15:04"), c.EndAt.Format("15:04"),
		joinSources(c.Supporting), joinSources(c.Conflicting))
}

func joinSources(sources []SourceKind) string {
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		switch source {
		case SourceDesktopActivity:
			names = append(names, "this computer being unused")
		case SourceWearableSleep:
			names = append(names, "your wearable")
		case SourceUserConfirmed:
			names = append(names, "what you confirmed")
		}
	}
	switch len(names) {
	case 0:
		return "no source"
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		return fmt.Sprintf("%s, %s and %s", names[0], names[1], names[2])
	}
}
