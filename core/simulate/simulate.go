// Package simulate is the deterministic Non-24 sleep pattern generator from the
// validation plan ("Circadian Analysis Validation and Synthetic Data Plan" §3-4):
// a seeded generator that produces the sleep sessions the app would see while
// retaining the true latent state, so estimator output is scored against truth
// rather than against noisy observations. The generative-loop style follows the
// sleepdiary/Zeitdex docs simulator (add a diary entry per cycle with normal
// jitter); the scenario shapes and expected behaviors come from the plan.
//
// Only sleep-timing structure is generated here. Wearable/phone/desktop/light
// streams (plan scenarios 11-15, 18-19) wait until multi-source inference
// exists to consume them.
package simulate

import (
	"fmt"
	"math/rand"
	"time"

	"non24.app/core/domain"
)

// Segment is a stretch of cycles with one latent circadian period (tau).
// Multiple segments model change points such as temporary alignment.
type Segment struct {
	Cycles int
	Period time.Duration
}

// ForcedWake clamps wake to a fixed civil time for a range of cycles while the
// latent rhythm keeps drifting (plan scenario 7). Weekends optionally rebound.
type ForcedWake struct {
	FromCycle      int
	ToCycle        int
	WakeCivilHour  int
	WeekendRebound bool
}

// Params describes one generated history. Zero values disable a feature.
type Params struct {
	Seed           int64
	Start          time.Time // first latent sleep onset, UTC
	ZoneID         string
	Segments       []Segment
	Duration       time.Duration // mean sleep duration
	OnsetJitter    time.Duration // sigma of normal jitter on onset
	DurationJitter time.Duration // sigma of normal jitter on duration
	NapsPerCycle   int           // naps logged in each waking period
	NapDuration    time.Duration
	FragmentAfter  time.Duration // if >0, split main sleep after this much sleep
	FragmentGap    time.Duration // wake gap inside fragmented main sleep
	ForcedWake     *ForcedWake
	Unlogged       map[int]bool   // cycle -> episode not logged (missingness)
	Deprivation    map[int]bool   // cycle -> skipped sleep, short recovery next
	ZoneShifts     map[int]string // cycle -> zone in effect from that cycle on
}

// Truth is the latent generative state retained for scoring.
type Truth struct {
	LatentOnsets  []time.Time     // pre-jitter linear trajectory, one per cycle
	DriftPerCycle []time.Duration // latent tau-24h in effect at each cycle
}

// Result carries what the app would see plus the truth it should recover.
type Result struct {
	Sessions []domain.SleepSession
	Truth    Truth
}

// Generate builds the history. It is deterministic for a given Params value.
func Generate(params Params) (Result, error) {
	if len(params.Segments) == 0 {
		return Result{}, fmt.Errorf("simulate: at least one segment is required")
	}
	if params.Duration <= 0 {
		return Result{}, fmt.Errorf("simulate: mean sleep duration is required")
	}
	zone := params.ZoneID
	if zone == "" {
		zone = "UTC"
	}
	rng := rand.New(rand.NewSource(params.Seed))
	result := Result{}
	latent := params.Start.UTC()
	cycle := 0

	for _, segment := range params.Segments {
		if segment.Cycles <= 0 || segment.Period <= 0 {
			return Result{}, fmt.Errorf("simulate: segment cycles and period must be positive")
		}
		for c := 0; c < segment.Cycles; c++ {
			if next, ok := params.ZoneShifts[cycle]; ok {
				zone = next
			}
			result.Truth.LatentOnsets = append(result.Truth.LatentOnsets, latent)
			result.Truth.DriftPerCycle = append(result.Truth.DriftPerCycle, segment.Period-24*time.Hour)

			// Behavioral noise around the latent trajectory. Draw jitter values
			// unconditionally so feature flags do not shift the random sequence
			// of later cycles.
			onset := latent.Add(normalJitter(rng, params.OnsetJitter))
			duration := params.Duration + normalJitter(rng, params.DurationJitter)
			skipped := params.Deprivation[cycle]
			if params.Deprivation[cycle-1] {
				duration = params.Duration * 6 / 10 // short recovery sleep
			}
			if duration < 3*time.Hour+30*time.Minute {
				duration = 3*time.Hour + 30*time.Minute
			}
			wake := onset.Add(duration)
			if fw := params.ForcedWake; fw != nil && cycle >= fw.FromCycle && cycle <= fw.ToCycle {
				if clamped, apply := forcedWakeAt(onset, wake, zone, fw); apply {
					wake = clamped
				}
			}

			logged := !skipped && !params.Unlogged[cycle]
			if logged {
				session, err := buildSession(cycle, onset, wake, zone, params)
				if err != nil {
					return Result{}, err
				}
				result.Sessions = append(result.Sessions, session)
				for n := 0; n < params.NapsPerCycle; n++ {
					nap, err := buildNap(cycle, n, wake, latent.Add(segment.Period), zone, params, rng)
					if err != nil {
						return Result{}, err
					}
					result.Sessions = append(result.Sessions, nap)
				}
			}

			latent = latent.Add(segment.Period)
			cycle++
		}
	}
	return result, nil
}

// forcedWakeAt returns the first civil WakeCivilHour strictly after onset: the
// alarm fires regardless of how little sleep that leaves, which is exactly the
// harm the plan wants preserved — as latent onset drifts toward the alarm,
// logged sleep progressively shortens (and below the estimator's 3h floor it
// stops being usable evidence). Weekends rebound when enabled (plan scenario 7).
func forcedWakeAt(onset, wake time.Time, zone string, fw *ForcedWake) (time.Time, bool) {
	location, err := time.LoadLocation(zone)
	if err != nil {
		return wake, false
	}
	local := onset.In(location)
	clamped := time.Date(local.Year(), local.Month(), local.Day(), fw.WakeCivilHour, 0, 0, 0, location)
	for !clamped.After(local.Add(30 * time.Minute)) {
		clamped = clamped.AddDate(0, 0, 1)
	}
	if fw.WeekendRebound {
		day := clamped.Weekday()
		if day == time.Saturday || day == time.Sunday {
			return wake, false
		}
	}
	if clamped.UTC().After(wake) {
		return wake, false // the alarm is later than natural wake; sleep ends naturally
	}
	return clamped.UTC(), true
}

func buildSession(cycle int, onset, wake time.Time, zone string, params Params) (domain.SleepSession, error) {
	evidence := domain.Evidence{Acquisition: domain.AcquisitionManual, Status: domain.StatusUserConfirmed}
	intervals := []domain.SleepInterval{}
	if params.FragmentAfter > 0 && params.FragmentGap > 0 &&
		onset.Add(params.FragmentAfter+params.FragmentGap+time.Hour).Before(wake) {
		splitSleep := onset.Add(params.FragmentAfter)
		resume := splitSleep.Add(params.FragmentGap)
		first, err := interval(onset, splitSleep, zone, evidence)
		if err != nil {
			return domain.SleepSession{}, err
		}
		second, err := interval(resume, wake, zone, evidence)
		if err != nil {
			return domain.SleepSession{}, err
		}
		intervals = append(intervals, first, second)
	} else {
		whole, err := interval(onset, wake, zone, evidence)
		if err != nil {
			return domain.SleepSession{}, err
		}
		intervals = append(intervals, whole)
	}
	return domain.SleepSession{
		ID:          domain.SleepSessionID(fmt.Sprintf("sim_sleep_%03d", cycle)),
		Intervals:   intervals,
		SourceLabel: "simulated",
		CreatedAt:   wake,
	}, nil
}

func buildNap(cycle, n int, wake, nextOnset time.Time, zone string, params Params, rng *rand.Rand) (domain.SleepSession, error) {
	napDuration := params.NapDuration
	if napDuration <= 0 {
		napDuration = 40 * time.Minute
	}
	// Place naps inside the waking period, spread out with a little noise.
	wakeSpan := nextOnset.Sub(wake)
	offset := wakeSpan * time.Duration(n+1) / time.Duration(params.NapsPerCycle+2)
	start := wake.Add(offset + normalJitter(rng, 20*time.Minute))
	evidence := domain.Evidence{Acquisition: domain.AcquisitionManual, Status: domain.StatusUserConfirmed}
	span, err := interval(start, start.Add(napDuration), zone, evidence)
	if err != nil {
		return domain.SleepSession{}, err
	}
	return domain.SleepSession{
		ID:          domain.SleepSessionID(fmt.Sprintf("sim_nap_%03d_%d", cycle, n)),
		Intervals:   []domain.SleepInterval{span},
		IsNap:       true,
		SourceLabel: "simulated",
		CreatedAt:   start.Add(napDuration),
	}, nil
}

func interval(start, end time.Time, zone string, evidence domain.Evidence) (domain.SleepInterval, error) {
	zonedStart, err := domain.NewZonedInstant(start, zone)
	if err != nil {
		return domain.SleepInterval{}, err
	}
	zonedEnd, err := domain.NewZonedInstant(end, zone)
	if err != nil {
		return domain.SleepInterval{}, err
	}
	return domain.SleepInterval{
		Interval:      domain.TimeRange{Start: zonedStart, End: zonedEnd},
		StartEvidence: evidence,
		EndEvidence:   evidence,
	}, nil
}

func normalJitter(rng *rand.Rand, sigma time.Duration) time.Duration {
	value := rng.NormFloat64()
	if sigma <= 0 {
		return 0
	}
	return time.Duration(value * float64(sigma))
}
