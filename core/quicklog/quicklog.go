// Package quicklog decides what one tap means.
//
// Recording a night currently costs a form: a start datetime, an end datetime,
// a zone and a classification, filled in by someone who has just woken up. The
// automaticity review named the one-tap "I am going to sleep" / "I woke up"
// pair the highest-value usability item in the project, and it is the direct
// answer to P7's remaining gap — the loop is automatic when a wearable or the
// desktop is watching, and a form when neither is.
//
// The whole difficulty is that a tap is not an observation. "I woke up" on its
// own establishes one boundary of an episode, and an estimator that needs both
// cannot be handed a guess about the other. This package holds the rules for
// when a pair of taps is enough to record a night, and when it has to stop and
// ask, so that the desktop and any later companion answer identically.
//
// Nothing here is medical. It compares two clock times against plausibility
// bounds the estimator already uses, and refuses to invent the ones it lacks.
package quicklog

import (
	"time"

	"non24.app/core/domain"
)

// The plausibility bounds are the estimator's own, so a night this package
// records is a night the estimator will accept. core/estimation skips episodes
// under three hours when fitting, and core/inference caps a candidate at
// fourteen; recording outside that range would produce a row that either does
// nothing or distorts the fit.
const (
	MinimumEpisode = 3 * time.Hour
	MaximumEpisode = 14 * time.Hour

	// StaleAfter is how long an unfinished sleep may sit before it stops being
	// something to close with one tap. Past it the person has been away long
	// enough that "now" is no longer evidence of when they woke.
	StaleAfter = 20 * time.Hour
)

// Pending is a recorded intent to sleep. It is deliberately not an observation:
// nothing has been observed to end, so nothing is appended until it has.
type Pending struct {
	StartedAt time.Time
	ZoneID    string
}

// Outcome is what a tap resolves to.
type Outcome string

const (
	// OutcomeRecord: both boundaries exist and are plausible. Append it.
	OutcomeRecord Outcome = "record"

	// OutcomeConfirmOnset: there is no pending sleep, so the onset is unknown.
	// The estimator's prediction may be offered as a starting point, clearly
	// labelled, but it is never recorded without a person agreeing to it.
	OutcomeConfirmOnset Outcome = "confirm_onset"

	// OutcomeConfirmShort: the pair is shorter than a principal sleep. It may
	// be a nap or a mistap, and guessing which would either lose a nap or
	// poison the fit with a fragment.
	OutcomeConfirmShort Outcome = "confirm_short"

	// OutcomeConfirmLong: the pair is longer than any plausible episode, which
	// almost always means the wake tap was missed rather than that the person
	// slept for a day.
	OutcomeConfirmLong Outcome = "confirm_long"

	// OutcomeConfirmStale: the pending sleep is old enough that `now` is not
	// evidence of the wake time.
	OutcomeConfirmStale Outcome = "confirm_stale"

	// OutcomeReject: the times cannot describe an episode at all.
	OutcomeReject Outcome = "reject"
)

// Wake is the answer to one "I woke up" tap.
type Wake struct {
	Outcome Outcome

	// Start and End are set when Outcome is OutcomeRecord.
	Start time.Time
	End   time.Time

	// SuggestedStart is a prefill for the cases that need a person, and is
	// never a value this package would record on its own. Zero when there is
	// nothing honest to suggest.
	SuggestedStart time.Time

	// SuggestionIsPrediction marks a SuggestedStart that came from the
	// estimator rather than from something the person did. A prefill the reader
	// believes is a record would be worse than an empty field.
	SuggestionIsPrediction bool

	// Duration is the pair's length, for wording the question.
	Duration time.Duration

	// Reason is a sentence for the person, not a log line.
	Reason string
}

// WakeInput is everything the decision needs.
type WakeInput struct {
	Now time.Time

	// Pending is the sleep the person marked, if any.
	Pending *Pending

	// PredictedOnset is the estimator's next predicted sleep onset, used only
	// as a labelled prefill when there is no pending sleep. Zero when the
	// estimator refused or has no history.
	PredictedOnset time.Time
}

// ResolveWake decides what "I woke up" means right now.
func ResolveWake(in WakeInput) Wake {
	now := in.Now.UTC()

	if in.Pending == nil {
		// Nothing was marked, so the onset is genuinely unknown. The estimator
		// may have an opinion; it is offered as a starting point and labelled
		// as one, because a forecast presented as a record is the exact
		// confusion this project exists to avoid.
		wake := Wake{
			Outcome: OutcomeConfirmOnset,
			Reason:  "There is no sleep to close, so the app does not know when you fell asleep. Enter it and the night is recorded.",
		}
		if !in.PredictedOnset.IsZero() && in.PredictedOnset.Before(now) {
			wake.SuggestedStart = in.PredictedOnset.UTC()
			wake.SuggestionIsPrediction = true
			wake.Reason = "There is no sleep to close. The time below is the app's prediction, not a record — change it if it is wrong."
		}
		return wake
	}

	start := in.Pending.StartedAt.UTC()
	if !now.After(start) {
		return Wake{
			Outcome: OutcomeReject,
			Reason:  "The sleep you marked starts later than now, so this cannot be recorded.",
			Start:   start,
		}
	}

	duration := now.Sub(start)
	wake := Wake{Start: start, End: now, Duration: duration, SuggestedStart: start}

	switch {
	case duration >= StaleAfter:
		wake.Outcome = OutcomeConfirmStale
		wake.Reason = "You marked sleep " + describe(duration) + " ago. Now is probably not when you woke, so enter the real times."
	case duration > MaximumEpisode:
		wake.Outcome = OutcomeConfirmLong
		wake.Reason = "That would be " + describe(duration) + " of sleep, which usually means the wake tap was missed. Check the times before recording."
	case duration < MinimumEpisode:
		wake.Outcome = OutcomeConfirmShort
		wake.Reason = "That is " + describe(duration) + ", short for a night. Record it as a nap, or correct the times."
	default:
		wake.Outcome = OutcomeRecord
		wake.Reason = "Recorded " + describe(duration) + " of sleep."
	}
	return wake
}

// SleepStart is the answer to one "I am going to sleep" tap.
type SleepStart struct {
	Pending Pending

	// Replaced is the pending sleep this tap overwrote, if any. Overwriting is
	// the right behaviour — the second tap is the current intent — but it must
	// be reported rather than done quietly, because the first tap's time is
	// gone afterwards.
	Replaced *Pending

	Reason string
}

// BeginSleep records the intent to sleep now.
func BeginSleep(now time.Time, zoneID string, existing *Pending) SleepStart {
	start := SleepStart{
		Pending: Pending{StartedAt: now.UTC(), ZoneID: zoneID},
		Reason:  "Sleep marked. Tap \"I woke up\" when you get up and the night is recorded.",
	}
	if existing != nil {
		previous := *existing
		start.Replaced = &previous
		start.Reason = "Sleep marked again, replacing the one from " + previous.StartedAt.UTC().Format("15:04") + " UTC. Only this one will be recorded."
	}
	return start
}

// Episode turns a resolved wake into the interval to append. It is only ever
// called for an outcome a person has accepted, so it takes the boundaries it is
// given rather than re-deciding them.
func Episode(start, end time.Time, zoneID string) (domain.TimeRange, error) {
	startInstant, err := domain.NewZonedInstant(start.UTC(), zoneID)
	if err != nil {
		return domain.TimeRange{}, err
	}
	endInstant, err := domain.NewZonedInstant(end.UTC(), zoneID)
	if err != nil {
		return domain.TimeRange{}, err
	}
	interval := domain.TimeRange{Start: startInstant, End: endInstant}
	if err := interval.Validate(); err != nil {
		return domain.TimeRange{}, err
	}
	return interval, nil
}

// describe renders a duration the way a person would say it out loud.
func describe(value time.Duration) string {
	minutes := int(value.Minutes())
	hours := minutes / 60
	minutes %= 60
	switch {
	case hours == 0:
		return itoa(minutes) + " minutes"
	case hours == 1 && minutes == 0:
		return "an hour"
	case minutes == 0:
		return itoa(hours) + " hours"
	default:
		return itoa(hours) + " hours " + itoa(minutes) + " minutes"
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}
