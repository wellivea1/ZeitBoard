package inference

import (
	"sort"
	"time"
)

// Scoring answers the only question that decides whether inferred episodes may
// ever reach planning: how far are the boundaries from the truth, and how often
// does the inference invent an episode that did not happen.
//
// It deliberately reports coverage and false positives alongside the error
// quantiles. An inference that only fires on the easy nights can post excellent
// median error while being useless, and one that fires constantly can cover
// everything while being noise.

// TruthInterval is an actual sleep episode the candidates are scored against.
type TruthInterval struct {
	StartAt time.Time
	EndAt   time.Time
}

// Score is the measured result for one run.
type Score struct {
	TruthEpisodes int
	Candidates    int

	// Matched is the number of truth episodes a candidate overlapped.
	Matched int

	// Coverage is Matched / TruthEpisodes.
	Coverage float64

	// FalsePositives are candidates that overlapped no truth episode. These
	// are the expensive errors: an invented sleep episode corrupts the drift
	// estimate that everything else is built on.
	FalsePositives int

	OnsetMedian time.Duration
	OnsetP90    time.Duration
	WakeMedian  time.Duration
	WakeP90     time.Duration

	// OnsetBias and WakeBias are signed medians. They matter more than the
	// absolute error for a systematic problem: a consistently late-by-40-minutes
	// onset is correctable, while an unbiased error of the same size is not.
	OnsetBias time.Duration
	WakeBias  time.Duration
}

// Meets reports whether a score clears the supplied targets.
type Targets struct {
	OnsetMedian       time.Duration
	WakeMedian        time.Duration
	BoundaryP90       time.Duration
	MinimumCoverage   float64
	MaxFalsePositives int
}

// PilotTargets are the starting points from the automaticity review. They are
// explicitly to be revised from measured live data, and must never be used to
// justify publishing more confidence than the evidence supports.
func PilotTargets() Targets {
	return Targets{
		OnsetMedian:       45 * time.Minute,
		WakeMedian:        30 * time.Minute,
		BoundaryP90:       90 * time.Minute,
		MinimumCoverage:   0.85,
		MaxFalsePositives: 0,
	}
}

// Meets reports whether every target is satisfied, and which are not.
func (s Score) Meets(targets Targets) (bool, []string) {
	var failures []string
	if s.OnsetMedian > targets.OnsetMedian {
		failures = append(failures, "onset median")
	}
	if s.WakeMedian > targets.WakeMedian {
		failures = append(failures, "wake median")
	}
	if s.OnsetP90 > targets.BoundaryP90 || s.WakeP90 > targets.BoundaryP90 {
		failures = append(failures, "boundary P90")
	}
	if s.Coverage < targets.MinimumCoverage {
		failures = append(failures, "coverage")
	}
	if s.FalsePositives > targets.MaxFalsePositives {
		failures = append(failures, "false positives")
	}
	return len(failures) == 0, failures
}

// ScoreCandidates matches candidates to truth by overlap and measures error.
//
// Matching is by greatest overlap rather than nearest start, because a
// candidate that brackets a real episode should be scored against that episode
// even when its boundaries are poor — which is the case that matters.
func ScoreCandidates(candidates []Candidate, truth []TruthInterval) Score {
	score := Score{TruthEpisodes: len(truth), Candidates: len(candidates)}
	if len(truth) == 0 {
		score.FalsePositives = len(candidates)
		return score
	}

	var onsetErrors, wakeErrors []time.Duration
	var onsetSigned, wakeSigned []time.Duration
	matchedTruth := make(map[int]bool, len(truth))

	for _, candidate := range candidates {
		best, ok := bestOverlap(candidate, truth)
		if !ok {
			score.FalsePositives++
			continue
		}
		matchedTruth[best] = true
		onsetDelta := candidate.StartAt.Sub(truth[best].StartAt)
		wakeDelta := candidate.EndAt.Sub(truth[best].EndAt)
		onsetSigned = append(onsetSigned, onsetDelta)
		wakeSigned = append(wakeSigned, wakeDelta)
		onsetErrors = append(onsetErrors, absDuration(onsetDelta))
		wakeErrors = append(wakeErrors, absDuration(wakeDelta))
	}

	score.Matched = len(matchedTruth)
	score.Coverage = float64(score.Matched) / float64(len(truth))
	score.OnsetMedian = quantile(onsetErrors, 0.5)
	score.OnsetP90 = quantile(onsetErrors, 0.9)
	score.WakeMedian = quantile(wakeErrors, 0.5)
	score.WakeP90 = quantile(wakeErrors, 0.9)
	score.OnsetBias = quantile(onsetSigned, 0.5)
	score.WakeBias = quantile(wakeSigned, 0.5)
	return score
}

func bestOverlap(candidate Candidate, truth []TruthInterval) (int, bool) {
	bestIndex := -1
	var bestOverlap time.Duration
	for index, actual := range truth {
		start := candidate.StartAt
		if actual.StartAt.After(start) {
			start = actual.StartAt
		}
		end := candidate.EndAt
		if actual.EndAt.Before(end) {
			end = actual.EndAt
		}
		if !end.After(start) {
			continue
		}
		if overlap := end.Sub(start); overlap > bestOverlap {
			bestOverlap = overlap
			bestIndex = index
		}
	}
	return bestIndex, bestIndex >= 0
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

// quantile returns the linear-interpolation-free order statistic. Sample sizes
// here are small enough that a more elaborate estimator would imply precision
// the data does not have.
func quantile(values []time.Duration, q float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(q * float64(len(sorted)-1))
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
