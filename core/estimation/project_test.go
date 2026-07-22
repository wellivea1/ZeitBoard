package estimation

import (
	"context"
	"errors"
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestProjectDerivesDriftFromTheEngine(t *testing.T) {
	// A clean free-running 25h rhythm: onset should drift +60 min per cycle.
	sessions := syntheticSessions(12, time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC), 25*time.Hour, 8*time.Hour, "UTC")
	projection, err := (RobustEstimator{}).Project(context.Background(), sessions, time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	if got := projection.SlopeLabel; got != "+60 min per cycle" {
		t.Fatalf("slope label = %q", got)
	}
	if len(projection.DriftPoints) != 12 {
		t.Fatalf("drift points = %d", len(projection.DriftPoints))
	}
	if len(projection.ObservedRows) != 12 {
		t.Fatalf("observed rows = %d", len(projection.ObservedRows))
	}

	// Newest row first (top-down read), matching the design.
	if projection.ObservedRows[0].Day != "Jan 12" {
		t.Fatalf("newest observed row day = %q", projection.ObservedRows[0].Day)
	}
	if projection.ObservedRows[0].CivilDate != "2026-01-12" || projection.ObservedRows[0].ZoneID != "UTC" {
		t.Fatalf("newest observed row civil anchor = %q %q", projection.ObservedRows[0].CivilDate, projection.ObservedRows[0].ZoneID)
	}
	if projection.DriftPoints[0].CivilDate != "2026-01-01" || projection.DriftPoints[0].ZoneID != "UTC" {
		t.Fatalf("first drift point civil anchor = %q %q", projection.DriftPoints[0].CivilDate, projection.DriftPoints[0].ZoneID)
	}
	if projection.Now.CivilDate != "2026-01-13" || projection.Now.ZoneID != "UTC" {
		t.Fatalf("now civil anchor = %q %q", projection.Now.CivilDate, projection.Now.ZoneID)
	}

	// Unwrapped fit must be monotone with a positive slope and stay continuous
	// across midnight (no 24h jumps), and the band must be non-degenerate.
	for i, point := range projection.DriftPoints {
		if point.BandHighHour <= point.BandLowHour {
			t.Fatalf("point %d band not positive: %v..%v", i, point.BandLowHour, point.BandHighHour)
		}
		if i > 0 {
			step := point.FitHour - projection.DriftPoints[i-1].FitHour
			if step <= 0 || step > 12 {
				t.Fatalf("fit not continuous/increasing at %d: step %v", i, step)
			}
		}
	}
	if projection.YMaxHour <= projection.YMinHour {
		t.Fatalf("y-range collapsed: %v..%v", projection.YMinHour, projection.YMaxHour)
	}

	// Forecast comes from the engine's widening windows.
	if len(projection.ForecastRows) != 7 {
		t.Fatalf("forecast rows = %d", len(projection.ForecastRows))
	}
	first := projection.ForecastRows[0].DurationHours
	last := projection.ForecastRows[len(projection.ForecastRows)-1].DurationHours
	if last < first {
		t.Fatalf("forecast windows should widen: first %v, last %v", first, last)
	}
}

func TestProjectReportsEarlierDriftSign(t *testing.T) {
	sessions := syntheticSessions(12, time.Date(2026, 2, 1, 6, 0, 0, 0, time.UTC), 23*time.Hour, 8*time.Hour, "UTC")
	projection, err := (RobustEstimator{}).Project(context.Background(), sessions, time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := projection.SlopeLabel; got != "-60 min per cycle" {
		t.Fatalf("slope label = %q", got)
	}
	if projection.DriftConfidence != "High" && projection.DriftConfidence != "Medium" && projection.DriftConfidence != "Low" {
		t.Fatalf("unexpected confidence label %q", projection.DriftConfidence)
	}
}

func TestProjectClassifiesEpisodeKindsFromEvidence(t *testing.T) {
	sessions := syntheticSessions(12, time.Date(2026, 3, 1, 2, 0, 0, 0, time.UTC), 25*time.Hour, 8*time.Hour, "UTC")
	sessions[3].Intervals[0].StartEvidence.Status = domain.StatusInferred
	sessions[5].Intervals[0].StartEvidence.CorrectionIDs = []domain.CorrectionID{"correction-1"}

	projection, err := (RobustEstimator{}).Project(context.Background(), sessions, time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	var sawInferred, sawCorrected bool
	for _, row := range projection.ObservedRows {
		switch row.Kind {
		case "inferred":
			sawInferred = true
			if row.Confidence != "Low" {
				t.Fatalf("inferred row confidence = %q", row.Confidence)
			}
		case "corrected":
			sawCorrected = true
			if row.Confidence != "Medium" {
				t.Fatalf("corrected row confidence = %q", row.Confidence)
			}
		}
	}
	if !sawInferred || !sawCorrected {
		t.Fatalf("expected inferred and corrected rows; inferred=%v corrected=%v", sawInferred, sawCorrected)
	}
}

func TestProjectPassesThroughRefusal(t *testing.T) {
	_, err := (RobustEstimator{}).Project(context.Background(), syntheticSessions(3, time.Now().UTC(), 24*time.Hour, 8*time.Hour, "UTC"), time.Now().UTC())
	var refusal *EstimationRefusal
	if !errors.As(err, &refusal) || refusal.Code != RefusalInsufficientData {
		t.Fatalf("error = %#v", err)
	}
}
