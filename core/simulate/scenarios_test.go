package simulate

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
)

// The scenario validation suite: each test asserts the validation plan's
// "expected behavior" as a measurable claim against the real estimator.
// Thresholds are honest regression floors calibrated to current behavior —
// they document what the estimator achieves today, not aspirations.

type outcome struct {
	estimate domain.PhaseEstimate
	report   estimation.BacktestReport
}

func run(t *testing.T, params Params) (Result, outcome) {
	t.Helper()
	result, err := Generate(params)
	if err != nil {
		t.Fatal(err)
	}
	last := result.Sessions[len(result.Sessions)-1]
	asOf := last.Intervals[len(last.Intervals)-1].Interval.End.UTC
	estimator := estimation.RobustEstimator{}
	estimate, err := estimator.Estimate(context.Background(), result.Sessions, asOf)
	if err != nil {
		t.Fatalf("estimate refused: %v", err)
	}
	report, err := estimator.Backtest(context.Background(), result.Sessions)
	if err != nil {
		t.Fatalf("backtest refused: %v", err)
	}
	return result, outcome{estimate: estimate, report: report}
}

func driftErrorMinutes(estimate domain.PhaseEstimate, latent time.Duration) float64 {
	return math.Abs(estimate.ObservedDriftPerCycle.Minutes() - latent.Minutes())
}

func TestScenario1StableSleepReadsAsEntrained(t *testing.T) {
	_, got := run(t, Scenario1StableSleep())
	if err := driftErrorMinutes(got.estimate, 0); err > 5 {
		t.Fatalf("stable sleeper misread as drifting by %.1f min/cycle", err)
	}
	if got.report.MedianAbsErrorHours > 0.75 {
		t.Fatalf("stable rhythm median error %.2f h", got.report.MedianAbsErrorHours)
	}
}

func TestScenario2SlightVariabilityIsNotCalledFreeRunning(t *testing.T) {
	_, got := run(t, Scenario2SlightVariability())
	if err := driftErrorMinutes(got.estimate, 0); err > 20 {
		t.Fatalf("noisy entrained sleeper misread as drifting by %.1f min/cycle", err)
	}
}

func TestScenario3SubtleTauIsRecovered(t *testing.T) {
	_, got := run(t, Scenario3Tau24_2())
	if err := driftErrorMinutes(got.estimate, 12*time.Minute); err > 6 {
		t.Fatalf("0.2h/cycle delay missed by %.1f min/cycle", err)
	}
}

func TestScenario4Tau25FreeRunningRecovered(t *testing.T) {
	_, got := run(t, Scenario4Tau25())
	if err := driftErrorMinutes(got.estimate, time.Hour); err > 8 {
		t.Fatalf("25h rhythm drift missed by %.1f min/cycle", err)
	}
	if got.report.HitRate < 0.5 {
		t.Fatalf("25h rhythm forecast hit-rate %.2f", got.report.HitRate)
	}
	if got.report.MedianAbsErrorHours > 1 {
		t.Fatalf("25h rhythm median error %.2f h", got.report.MedianAbsErrorHours)
	}
}

func TestScenario5Tau26UnwrapsAcrossCivilBoundaries(t *testing.T) {
	_, got := run(t, Scenario5Tau26())
	if err := driftErrorMinutes(got.estimate, 2*time.Hour); err > 10 {
		t.Fatalf("26h rhythm drift missed by %.1f min/cycle", err)
	}
	if got.report.MedianAbsErrorHours > 1.25 {
		t.Fatalf("26h rhythm median error %.2f h — phase unwrap suspect", got.report.MedianAbsErrorHours)
	}
}

func TestScenario6TemporaryAlignmentDegradesHonestly(t *testing.T) {
	// The estimator is deliberately a single robust linear fit (change-point
	// classification is deferred work). The requirement here is honesty: the
	// backtest must SHOW the misfit — clearly worse than a clean rhythm —
	// rather than the estimate quietly claiming clean-rhythm accuracy.
	_, clean := run(t, Scenario4Tau25())
	_, changed := run(t, Scenario6TemporaryAlignment())
	if changed.report.P90AbsErrorHours < 1 {
		t.Fatalf("change points should leave a visible error tail, p90 %.2f h", changed.report.P90AbsErrorHours)
	}
	if changed.report.MedianAbsErrorHours <= clean.report.MedianAbsErrorHours {
		t.Fatalf("change-point history (%.2f h) should score worse than clean (%.2f h)",
			changed.report.MedianAbsErrorHours, clean.report.MedianAbsErrorHours)
	}
	// The estimator self-reports the misfit: it must not claim high confidence
	// on a history its linear model cannot describe (observed: low).
	if changed.estimate.Confidence.Level == domain.ConfidenceHigh {
		t.Fatal("change-point history must not be reported with high confidence")
	}
}

func TestScenario7ForcedWakeDoesNotFakeEntrainment(t *testing.T) {
	result, got := run(t, Scenario7ForcedWake())
	// The latent rhythm keeps drifting under the alarm; onset drift must still
	// be recovered — a fixed wake time alone is not entrainment.
	if err := driftErrorMinutes(got.estimate, time.Hour); err > 10 {
		t.Fatalf("forced wake corrupted the drift estimate by %.1f min/cycle", err)
	}
	// The generated evidence of harm is present: clamped nights are short.
	shortest := time.Duration(math.MaxInt64)
	for _, session := range result.Sessions {
		if d := session.Intervals[0].Interval.Duration(); d < shortest {
			shortest = d
		}
	}
	if shortest > 6*time.Hour {
		t.Fatalf("expected clamped short nights in the forced range, shortest %v", shortest)
	}
}

func TestScenario8DeprivationSurvivesRobustFit(t *testing.T) {
	_, got := run(t, Scenario8Deprivation())
	// One skipped night and a short recovery must not hijack the fit: the
	// Theil-Sen slope is the robustness mechanism the plan asks for.
	if err := driftErrorMinutes(got.estimate, time.Hour); err > 12 {
		t.Fatalf("deprivation event hijacked the drift estimate by %.1f min/cycle", err)
	}
	if got.report.Evaluations == 0 {
		t.Fatal("backtest must remain evaluable across the deprivation gap")
	}
}

func TestScenario9NapsAreNotPhaseMarkers(t *testing.T) {
	result, got := run(t, Scenario9Naps())
	naps := 0
	for _, session := range result.Sessions {
		if session.IsNap {
			naps++
		}
	}
	if naps != 36 {
		t.Fatalf("expected 36 naps in the log, got %d", naps)
	}
	if err := driftErrorMinutes(got.estimate, 48*time.Minute); err > 8 {
		t.Fatalf("naps corrupted the drift estimate by %.1f min/cycle", err)
	}
}

func TestScenario10FragmentationPreservedAndHarmless(t *testing.T) {
	result, got := run(t, Scenario10Fragmented())
	fragmented := 0
	for _, session := range result.Sessions {
		if len(session.Intervals) == 2 {
			fragmented++
		}
	}
	if fragmented == 0 {
		t.Fatal("expected fragmented main sleep sessions")
	}
	if err := driftErrorMinutes(got.estimate, 48*time.Minute); err > 8 {
		t.Fatalf("fragmentation corrupted the drift estimate by %.1f min/cycle", err)
	}
}

func TestScenario16TravelDoesNotFakeAPhaseShift(t *testing.T) {
	result, got := run(t, Scenario16Travel())
	if err := driftErrorMinutes(got.estimate, 54*time.Minute); err > 8 {
		t.Fatalf("zone change misread as phase shift: drift off by %.1f min/cycle", err)
	}
	last := result.Sessions[len(result.Sessions)-1]
	if last.Intervals[0].Interval.Start.ZoneID != "America/Los_Angeles" {
		t.Fatalf("civil rendering should follow the traveller, got %s",
			last.Intervals[0].Interval.Start.ZoneID)
	}
}

func TestScenario17DaylightSavingAddsNoArtificialDrift(t *testing.T) {
	_, got := run(t, Scenario17DaylightSaving())
	if err := driftErrorMinutes(got.estimate, 36*time.Minute); err > 8 {
		t.Fatalf("DST transition distorted drift by %.1f min/cycle", err)
	}
}

// TestScenarioBenchmarkTable logs the accuracy table for every scenario (run
// with -v). It asserts nothing new; it exists so a reader can see the actual
// margins behind the regression floors above.
func TestScenarioBenchmarkTable(t *testing.T) {
	rows := []struct {
		name   string
		latent time.Duration
		params Params
	}{
		{"S1 stable", 0, Scenario1StableSleep()},
		{"S2 noisy entrained", 0, Scenario2SlightVariability()},
		{"S3 tau 24.2", 12 * time.Minute, Scenario3Tau24_2()},
		{"S4 tau 25", time.Hour, Scenario4Tau25()},
		{"S5 tau 26", 2 * time.Hour, Scenario5Tau26()},
		{"S6 temp alignment", 0, Scenario6TemporaryAlignment()},
		{"S7 forced wake", time.Hour, Scenario7ForcedWake()},
		{"S8 deprivation", time.Hour, Scenario8Deprivation()},
		{"S9 naps", 48 * time.Minute, Scenario9Naps()},
		{"S10 fragmented", 48 * time.Minute, Scenario10Fragmented()},
		{"S16 travel", 54 * time.Minute, Scenario16Travel()},
		{"S17 DST", 36 * time.Minute, Scenario17DaylightSaving()},
	}
	t.Logf("%-20s %10s %10s %8s %8s %8s", "scenario", "drift err", "median", "p90", "hit", "conf")
	for _, row := range rows {
		_, got := run(t, row.params)
		label := "n/a"
		if row.name != "S6 temp alignment" {
			label = time.Duration(driftErrorMinutes(got.estimate, row.latent) * float64(time.Minute)).Round(time.Second).String()
		}
		t.Logf("%-20s %10s %9.2fh %7.2fh %8.2f %8s",
			row.name, label,
			got.report.MedianAbsErrorHours, got.report.P90AbsErrorHours,
			got.report.HitRate, got.estimate.Confidence.Level)
	}
}

func TestScenario20CorruptRecordsAreRefusedNotEstimated(t *testing.T) {
	// Corruption is rejected at the boundary rather than generated as history:
	// non-increasing onsets and impossible intervals must produce typed
	// refusals, never estimates.
	base, err := Generate(Scenario1StableSleep())
	if err != nil {
		t.Fatal(err)
	}
	sessions := base.Sessions
	duplicated := append(append([]domain.SleepSession{}, sessions...), sessions[3])
	estimator := estimation.RobustEstimator{}
	asOf := sessions[len(sessions)-1].Intervals[0].Interval.End.UTC
	if _, err := estimator.Estimate(context.Background(), duplicated, asOf); err == nil {
		t.Fatal("duplicate import must refuse, not estimate")
	} else {
		var refusal *estimation.EstimationRefusal
		if !errors.As(err, &refusal) {
			t.Fatalf("expected a typed refusal, got %v", err)
		}
	}

	negative := append([]domain.SleepSession{}, sessions...)
	bad := negative[5]
	badInterval := bad.Intervals[0]
	badInterval.Interval.Start, badInterval.Interval.End = badInterval.Interval.End, badInterval.Interval.Start
	bad.Intervals = []domain.SleepInterval{badInterval}
	negative[5] = bad
	if _, err := estimator.Estimate(context.Background(), negative, asOf); err == nil {
		t.Fatal("negative-duration sleep must refuse, not estimate")
	}
}
