package simulate

import (
	"math/rand"
	"sort"
	"time"

	"non24.app/core/domain"
	"non24.app/core/platform/activity"
)

// ActivityParams describes how the person uses a computer around their sleep.
//
// These are the behaviours that decide whether desktop inactivity is usable
// evidence at all. Someone who reads for two hours before sleeping and does not
// touch the machine for an hour after waking produces a quiet interval three
// hours longer than their actual sleep, and no amount of care in the inference
// rules recovers that. Generating them explicitly is how the validation gate
// measures the gap instead of assuming it away.
type ActivityParams struct {
	Seed int64

	// StopUsingBefore is how long before sleep onset the person typically puts
	// the machine down. Positive values mean the quiet interval starts early.
	StopUsingBefore time.Duration

	// ResumeUsingAfter is how long after waking they typically touch it again.
	// This is the "quiet wake" problem: the machine cannot see a wake that is
	// not followed by use.
	ResumeUsingAfter time.Duration

	// Jitter is the sigma applied to both offsets.
	Jitter time.Duration

	// NightUse, when set, models someone else using the machine — or the user
	// getting up — during sleep, which splits a quiet interval in two.
	NightUse map[int]time.Duration

	// MachineOff marks cycles where the machine was shut down rather than
	// merely idle, so no evidence exists at all.
	MachineOff map[int]bool

	// SuspendInstead marks cycles where the machine suspended rather than
	// sitting idle, which is a cleaner signal.
	SuspendInstead map[int]bool

	// IdleThreshold mirrors the collector's configuration so generated
	// transitions land where the real collector would place them.
	IdleThreshold time.Duration
}

// DefaultActivityParams models an ordinary user: winds down for a while before
// sleeping, checks the machine reasonably soon after waking.
func DefaultActivityParams(seed int64) ActivityParams {
	return ActivityParams{
		Seed:             seed,
		StopUsingBefore:  40 * time.Minute,
		ResumeUsingAfter: 25 * time.Minute,
		Jitter:           15 * time.Minute,
		IdleThreshold:    15 * time.Minute,
	}
}

// GenerateActivity produces the transitions a desktop collector would have
// recorded for the supplied sleep history.
//
// It works from the *observed* sessions rather than the latent onsets, because
// that is what the machine could have seen: activity brackets the sleep that
// actually happened, jitter included.
func GenerateActivity(sessions []domain.SleepSession, params ActivityParams) []activity.Transition {
	if params.IdleThreshold <= 0 {
		params.IdleThreshold = 15 * time.Minute
	}
	random := rand.New(rand.NewSource(params.Seed))

	principal := make([]domain.TimeRange, 0, len(sessions))
	for _, session := range sessions {
		if session.EffectiveClassification() != domain.SleepClassificationPrincipal {
			continue
		}
		if len(session.Intervals) == 0 {
			continue
		}
		principal = append(principal, session.Intervals[0].Interval)
	}
	sort.Slice(principal, func(i, j int) bool {
		return principal[i].Start.UTC.Before(principal[j].Start.UTC)
	})

	transitions := make([]activity.Transition, 0, len(principal)*2+2)
	if len(principal) == 0 {
		return transitions
	}
	transitions = append(transitions, activity.Transition{
		At:    principal[0].Start.UTC.Add(-12 * time.Hour),
		State: activity.StateStartup,
	})

	for index, sleep := range principal {
		if params.MachineOff[index] {
			// No evidence at all for this cycle. Silence is the honest
			// outcome; inventing a quiet interval would flatter the inference.
			continue
		}
		quietFrom := sleep.Start.UTC.Add(-jittered(random, params.StopUsingBefore, params.Jitter))
		quietTo := sleep.End.UTC.Add(jittered(random, params.ResumeUsingAfter, params.Jitter))
		if !quietTo.After(quietFrom) {
			continue
		}

		startState := activity.StateIdle
		endState := activity.StateActive
		if params.SuspendInstead[index] {
			startState = activity.StateSuspended
			endState = activity.StateResumed
		}

		if nightUse, ok := params.NightUse[index]; ok && nightUse > 0 {
			// Someone used the machine mid-sleep. The quiet interval splits,
			// and neither half is long enough to look like principal sleep —
			// which is exactly the failure the gate needs to see.
			middle := sleep.Start.UTC.Add(sleep.End.UTC.Sub(sleep.Start.UTC) / 2)
			transitions = append(transitions,
				activity.Transition{At: quietFrom, State: startState, PriorDuration: time.Hour},
				activity.Transition{At: middle, State: activity.StateActive, PriorDuration: middle.Sub(quietFrom)},
				activity.Transition{At: middle.Add(nightUse), State: startState, PriorDuration: nightUse},
				activity.Transition{At: quietTo, State: endState, PriorDuration: quietTo.Sub(middle.Add(nightUse))},
			)
			continue
		}

		transitions = append(transitions,
			activity.Transition{At: quietFrom, State: startState, PriorDuration: 2 * time.Hour},
			activity.Transition{At: quietTo, State: endState, PriorDuration: quietTo.Sub(quietFrom)},
		)
	}
	sort.Slice(transitions, func(i, j int) bool { return transitions[i].At.Before(transitions[j].At) })
	return transitions
}

// jittered applies normal noise to an offset and keeps it non-negative.
func jittered(random *rand.Rand, base, sigma time.Duration) time.Duration {
	if sigma <= 0 {
		return base
	}
	value := time.Duration(float64(base) + random.NormFloat64()*float64(sigma))
	if value < 0 {
		return 0
	}
	return value
}

// PrincipalIntervals returns the observed principal sleep spans, which the
// scorer compares candidates against.
func PrincipalIntervals(sessions []domain.SleepSession) []domain.TimeRange {
	out := make([]domain.TimeRange, 0, len(sessions))
	for _, session := range sessions {
		if session.EffectiveClassification() != domain.SleepClassificationPrincipal {
			continue
		}
		if len(session.Intervals) == 0 {
			continue
		}
		out = append(out, session.Intervals[0].Interval)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.UTC.Before(out[j].Start.UTC) })
	return out
}
