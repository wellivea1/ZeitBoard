package simulate

import (
	"testing"
	"time"
)

func TestGenerateIsDeterministicPerSeed(t *testing.T) {
	first, err := Generate(Scenario4Tau25())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(Scenario4Tau25())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Sessions) != len(second.Sessions) {
		t.Fatalf("session counts differ: %d vs %d", len(first.Sessions), len(second.Sessions))
	}
	for i := range first.Sessions {
		a := first.Sessions[i].Intervals[0].Interval
		b := second.Sessions[i].Intervals[0].Interval
		if !a.Start.UTC.Equal(b.Start.UTC) || !a.End.UTC.Equal(b.End.UTC) {
			t.Fatalf("session %d differs between identical runs", i)
		}
	}

	reseeded := Scenario4Tau25()
	reseeded.Seed = 999
	third, err := Generate(reseeded)
	if err != nil {
		t.Fatal(err)
	}
	same := true
	for i := range first.Sessions {
		if !first.Sessions[i].Intervals[0].Interval.Start.UTC.Equal(
			third.Sessions[i].Intervals[0].Interval.Start.UTC) {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different seeds produced identical jitter")
	}
}

func TestGenerateRetainsLatentTruth(t *testing.T) {
	params := Scenario6TemporaryAlignment()
	result, err := Generate(params)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, segment := range params.Segments {
		total += segment.Cycles
	}
	if len(result.Truth.LatentOnsets) != total || len(result.Truth.DriftPerCycle) != total {
		t.Fatalf("truth incomplete: %d onsets, %d drifts, want %d",
			len(result.Truth.LatentOnsets), len(result.Truth.DriftPerCycle), total)
	}
	// Drift-per-cycle truth tracks the segment in effect.
	if result.Truth.DriftPerCycle[0] != time.Hour+48*time.Minute {
		t.Fatalf("segment 1 drift truth = %v", result.Truth.DriftPerCycle[0])
	}
	if result.Truth.DriftPerCycle[10] != 0 {
		t.Fatalf("segment 2 drift truth = %v", result.Truth.DriftPerCycle[10])
	}
	if result.Truth.DriftPerCycle[20] != time.Hour+30*time.Minute {
		t.Fatalf("segment 3 drift truth = %v", result.Truth.DriftPerCycle[20])
	}
	// The latent trajectory is exactly linear inside each segment.
	gap := result.Truth.LatentOnsets[5].Sub(result.Truth.LatentOnsets[4])
	if gap != 25*time.Hour+48*time.Minute {
		t.Fatalf("latent step = %v", gap)
	}
}

func TestUnloggedCyclesLeaveGapsInSessionsButNotTruth(t *testing.T) {
	params := Scenario4Tau25()
	params.Unlogged = map[int]bool{5: true, 6: true}
	result, err := Generate(params)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sessions) != 16 {
		t.Fatalf("expected 16 logged sessions, got %d", len(result.Sessions))
	}
	if len(result.Truth.LatentOnsets) != 18 {
		t.Fatalf("truth must keep all 18 cycles, got %d", len(result.Truth.LatentOnsets))
	}
}

func TestForcedWakeShortensLoggedSleepWithoutMovingOnsets(t *testing.T) {
	forced, err := Generate(Scenario7ForcedWake())
	if err != nil {
		t.Fatal(err)
	}
	free := Scenario7ForcedWake()
	free.ForcedWake = nil
	natural, err := Generate(free)
	if err != nil {
		t.Fatal(err)
	}
	shortened := 0
	for i := range forced.Sessions {
		fi := forced.Sessions[i].Intervals[0].Interval
		ni := natural.Sessions[i].Intervals[0].Interval
		if !fi.Start.UTC.Equal(ni.Start.UTC) {
			t.Fatalf("forced wake must not move sleep onset (cycle %d)", i)
		}
		if fi.Duration() < ni.Duration() {
			shortened++
		}
	}
	if shortened < 3 {
		t.Fatalf("expected several clamped nights, got %d", shortened)
	}
}
