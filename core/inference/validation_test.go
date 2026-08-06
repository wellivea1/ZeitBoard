package inference_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/core/inference"
	"non24.app/core/simulate"
)

// The validation gate for shadow sleep inference.
//
// ADR-0031 states that an inferred episode may reach production planning only
// after a documented positive validation decision. This suite is where that
// decision is measured. It does not assert that the targets are met — it
// asserts the harness works and reports the numbers, because forcing a pass
// would defeat the purpose of having a gate.

type scenario struct {
	name     string
	params   simulate.Params
	activity func(int64) simulate.ActivityParams
}

func scenarios() []scenario {
	ordinary := func(seed int64) simulate.ActivityParams {
		return simulate.DefaultActivityParams(seed)
	}
	return []scenario{
		{"stable rhythm", simulate.Scenario1StableSleep(), ordinary},
		{"free-running tau 24.8", simulate.Scenario4Tau25(), ordinary},
		{"forced wake", simulate.Scenario7ForcedWake(), ordinary},
		{"fragmented sleep", simulate.Scenario10Fragmented(), ordinary},
		{"naps", simulate.Scenario9Naps(), ordinary},
		{
			"quiet wake", simulate.Scenario4Tau25(),
			func(seed int64) simulate.ActivityParams {
				// Wakes, then does not touch the machine for two hours. The
				// collector cannot see a wake that is not followed by use.
				params := simulate.DefaultActivityParams(seed)
				params.ResumeUsingAfter = 2 * time.Hour
				return params
			},
		},
		{
			"long wind-down", simulate.Scenario4Tau25(),
			func(seed int64) simulate.ActivityParams {
				// Reads for two hours before sleeping.
				params := simulate.DefaultActivityParams(seed)
				params.StopUsingBefore = 2 * time.Hour
				return params
			},
		},
		{
			"machine used mid-sleep", simulate.Scenario4Tau25(),
			func(seed int64) simulate.ActivityParams {
				params := simulate.DefaultActivityParams(seed)
				params.NightUse = map[int]time.Duration{2: 20 * time.Minute, 5: 30 * time.Minute}
				return params
			},
		},
		{
			"machine off some nights", simulate.Scenario4Tau25(),
			func(seed int64) simulate.ActivityParams {
				params := simulate.DefaultActivityParams(seed)
				params.MachineOff = map[int]bool{1: true, 4: true, 7: true}
				return params
			},
		},
		{
			"suspend instead of idle", simulate.Scenario4Tau25(),
			func(seed int64) simulate.ActivityParams {
				params := simulate.DefaultActivityParams(seed)
				params.SuspendInstead = map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true}
				return params
			},
		},
	}
}

func runScenario(t *testing.T, s scenario) inference.Score {
	t.Helper()
	result, err := simulate.Generate(s.params)
	if err != nil {
		t.Fatalf("%s: generate: %v", s.name, err)
	}
	transitions := simulate.GenerateActivity(result.Sessions, s.activity(s.params.Seed))
	intervals := inference.FromActivity(transitions)

	candidates, refusal := inference.Build(intervals, time.Now().UTC())
	if refusal != nil {
		// A refusal is a legitimate outcome and scores as zero coverage rather
		// than as a test failure.
		candidates = nil
	}
	return inference.ScoreCandidates(candidates, truthFrom(result.Sessions))
}

func truthFrom(sessions []domain.SleepSession) []inference.TruthInterval {
	spans := simulate.PrincipalIntervals(sessions)
	truth := make([]inference.TruthInterval, 0, len(spans))
	for _, span := range spans {
		truth = append(truth, inference.TruthInterval{StartAt: span.Start.UTC, EndAt: span.End.UTC})
	}
	return truth
}

// TestInferenceValidationGate measures every scenario and writes the table.
// It fails only if the harness itself is broken.
func TestInferenceValidationGate(t *testing.T) {
	targets := inference.PilotTargets()
	var report string
	report += "| Scenario | Truth | Cand | Cover | Onset med | Onset P90 | Wake med | Wake P90 | Onset bias | Wake bias | False |\n"
	report += "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n"

	passing := 0
	for _, s := range scenarios() {
		score := runScenario(t, s)
		if score.TruthEpisodes == 0 {
			t.Fatalf("%s: the generator produced no principal sleep", s.name)
		}
		ok, _ := score.Meets(targets)
		if ok {
			passing++
		}
		report += fmt.Sprintf("| %s | %d | %d | %.2f | %s | %s | %s | %s | %s | %s | %d |\n",
			s.name, score.TruthEpisodes, score.Candidates, score.Coverage,
			round(score.OnsetMedian), round(score.OnsetP90),
			round(score.WakeMedian), round(score.WakeP90),
			signed(score.OnsetBias), signed(score.WakeBias), score.FalsePositives)
	}

	t.Logf("shadow inference validation\n%s\nscenarios meeting every pilot target: %d of %d",
		report, passing, len(scenarios()))

	if path := os.Getenv("ZEITBOARD_INFERENCE_REPORT"); path != "" {
		if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
			t.Fatalf("write report: %v", err)
		}
	}
}

// TestOrdinaryUseIsBracketedNotMatched is the finding the gate exists to
// surface. Even with well-behaved usage, desktop inactivity brackets sleep: it
// starts before onset and ends after wake, so the boundaries are biased
// outward by construction rather than by a bug.
func TestOrdinaryUseIsBracketedNotMatched(t *testing.T) {
	score := runScenario(t, scenario{
		"ordinary", simulate.Scenario4Tau25(),
		func(seed int64) simulate.ActivityParams { return simulate.DefaultActivityParams(seed) },
	})
	if score.Matched == 0 {
		t.Fatal("no candidate matched any real episode; the harness is broken")
	}
	if score.OnsetBias >= 0 {
		t.Errorf("onset bias = %v; inactivity should begin before sleep onset", score.OnsetBias)
	}
	if score.WakeBias <= 0 {
		t.Errorf("wake bias = %v; use should resume after waking", score.WakeBias)
	}
}

// TestMidSleepUseOverSegments documents the failure mode most likely to produce
// a wrong answer in practice: a shared machine, or simply getting up in the
// night.
//
// The interesting result is that it does *not* reduce coverage. Splitting an
// eight-hour sleep leaves two four-hour halves, both above the three-hour
// minimum, so each becomes its own candidate. One real episode is reported as
// two. That is worse than a miss for a drift estimator that counts cycles: a
// missing night widens the uncertainty, an invented extra episode shifts the
// fit.
func TestMidSleepUseOverSegments(t *testing.T) {
	clean := runScenario(t, scenario{
		"clean", simulate.Scenario4Tau25(),
		func(seed int64) simulate.ActivityParams { return simulate.DefaultActivityParams(seed) },
	})
	shared := runScenario(t, scenario{
		"shared", simulate.Scenario4Tau25(),
		func(seed int64) simulate.ActivityParams {
			params := simulate.DefaultActivityParams(seed)
			params.NightUse = map[int]time.Duration{1: 30 * time.Minute, 3: 30 * time.Minute, 5: 30 * time.Minute}
			return params
		},
	})
	if clean.Candidates != clean.TruthEpisodes {
		t.Fatalf("the clean baseline is already over-segmented (%d candidates for %d episodes)",
			clean.Candidates, clean.TruthEpisodes)
	}
	if shared.Candidates <= shared.TruthEpisodes {
		t.Errorf("mid-sleep use produced %d candidates for %d episodes; the split is not being exercised",
			shared.Candidates, shared.TruthEpisodes)
	}
	// Over-segmentation must not read as a false positive: both halves really
	// do overlap a real episode. The count is what is wrong, not the location.
	if shared.FalsePositives > 0 {
		t.Errorf("a split episode was scored as %d invented episodes", shared.FalsePositives)
	}
}

// TestMissingEvidenceLowersCoverageWithoutInventing checks the honest failure:
// nights with no data produce no candidate rather than a guess.
func TestMissingEvidenceLowersCoverageWithoutInventing(t *testing.T) {
	score := runScenario(t, scenario{
		"machine off", simulate.Scenario4Tau25(),
		func(seed int64) simulate.ActivityParams {
			params := simulate.DefaultActivityParams(seed)
			params.MachineOff = map[int]bool{0: true, 1: true, 2: true, 3: true}
			return params
		},
	})
	if score.Coverage >= 1.0 {
		t.Errorf("coverage = %.2f with four nights of no data", score.Coverage)
	}
	if score.FalsePositives > 0 {
		t.Errorf("missing evidence produced %d invented episodes", score.FalsePositives)
	}
}

func round(value time.Duration) string {
	return value.Round(time.Minute).String()
}

func signed(value time.Duration) string {
	if value > 0 {
		return "+" + value.Round(time.Minute).String()
	}
	return value.Round(time.Minute).String()
}
