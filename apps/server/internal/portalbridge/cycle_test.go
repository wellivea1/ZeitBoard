package portalbridge_test

import (
	"context"
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/simulate"
	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
)

type fixedSleep struct{ sessions []domain.SleepSession }

func (f fixedSleep) EffectiveSleepSessions(context.Context) ([]domain.SleepSession, error) {
	return f.sessions, nil
}

// A source that reports each night once it is over, on a 27-hour day. Under
// the fixed hours the share page showed nothing from about the middle of each
// sleep until the next record, and its own stamp called the forecast out of
// date a day after the last one. Following the forecast (ADR-0071), the page
// keeps its claim for as long as the day goes the way the forecast expects,
// carries a caution once the record is overdue, and drops the claim six hours
// later.
func TestA27HourDayIsSharedForAsLongAsItGoesAsForecast(t *testing.T) {
	history, err := simulate.Generate(simulate.Params{
		Seed:     7,
		Start:    time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC),
		ZoneID:   "UTC",
		Segments: []simulate.Segment{{Cycles: 16, Period: 27 * time.Hour}},
		Duration: 9 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	sessions := history.Sessions
	lastWake := sessions[len(sessions)-1].Intervals[0].Interval.End.UTC

	estimate, err := estimation.RobustEstimator{}.Estimate(context.Background(), sessions, lastWake)
	if err != nil {
		t.Fatal(err)
	}
	if cycle := estimate.ObservedCycleLength; cycle < 26*time.Hour+30*time.Minute || cycle > 27*time.Hour+30*time.Minute {
		t.Fatalf("the fixture's cycle was fitted as %s", cycle)
	}
	expected := estimate.PredictedSleepWindows[0].Interval
	earliestOnset, latestWake := expected.Start.UTC, expected.End.UTC
	if latestWake.Sub(lastWake) <= 24*time.Hour {
		t.Fatalf("the latest likely wake is %s after the record; the fixture does not outlast a day", latestWake.Sub(lastWake))
	}

	materializer := portalbridge.Materializer{Sleep: fixedSleep{sessions}}
	at := func(now time.Time) portal.Snapshot {
		t.Helper()
		prep, err := materializer.Prepare(context.Background(), now)
		if err != nil {
			t.Fatal(err)
		}
		return prep.Snapshot
	}
	awakeNow := func(snapshot portal.Snapshot, now time.Time) bool {
		for _, window := range snapshot.Windows {
			if !window.StartAt.After(now) && window.EndAt.After(now) {
				return true
			}
		}
		return false
	}

	withheldAt := latestWake.Add(6 * time.Hour)
	for now := lastWake.Add(time.Minute); now.Before(withheldAt); now = now.Add(20 * time.Minute) {
		snapshot := at(now)
		if snapshot.Status != portal.StatusAvailable {
			t.Fatalf("%s after the record the page shows %q", now.Sub(lastWake).Round(time.Minute), snapshot.Status)
		}
		if !snapshot.CautionAt.Equal(latestWake) || !snapshot.WithheldAt.Equal(withheldAt) {
			t.Fatalf("the page's deadlines are %s and %s, want %s and %s", snapshot.CautionAt, snapshot.WithheldAt, latestWake, withheldAt)
		}
		// The rendered page, at the hour it would be read, on a stamp as old
		// as the record: nothing is out of date, and the caution comes only
		// once the record is overdue.
		snapshot.GeneratedAt, snapshot.Version = lastWake, lastWake.UnixMilli()
		view := portal.BuildView(snapshot, now)
		overdue := !now.Before(latestWake)
		if view.Stale != overdue {
			t.Fatalf("%s after the record: caution = %v, overdue = %v", now.Sub(lastWake).Round(time.Minute), view.Stale, overdue)
		}
		if view.Headline == "This availability is out of date" {
			t.Fatalf("%s after the record the page calls itself out of date", now.Sub(lastWake).Round(time.Minute))
		}
		// Inside the hours a sleep may cover, nobody is told "awake".
		if !now.Before(earliestOnset) && now.Before(latestWake) && (awakeNow(snapshot, now) || view.LikelyAwake) {
			t.Fatalf("%s after the record, inside the forecast sleep, the page says awake", now.Sub(lastWake).Round(time.Minute))
		}
	}
	if !awakeNow(at(lastWake.Add(2*time.Hour)), lastWake.Add(2*time.Hour)) {
		t.Fatal("two hours after waking the page does not say awake")
	}

	gone := at(withheldAt)
	if gone.Status != portal.StatusInsufficientData || len(gone.Windows) != 0 {
		t.Fatalf("six hours past the latest likely wake the page still shows %q with %d windows", gone.Status, len(gone.Windows))
	}
}
