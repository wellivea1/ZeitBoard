package outlook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/freshness"
	"non24.app/core/outlook"
	"non24.app/core/simulate"
)

// A rhythm that drifts fifty minutes a cycle, which is what this product is
// for: the waking hours walk right around the clock over a fortnight, so
// "when can I ring someone" has a different answer every day.
const (
	zone  = "America/New_York"
	cycle = 24*time.Hour + 50*time.Minute
)

// history builds a drifting sleep history whose newest episode ends at lastEnd.
func history(t *testing.T, lastEnd time.Time, count int) []domain.SleepSession {
	t.Helper()
	sessions := make([]domain.SleepSession, 0, count)
	for i := 0; i < count; i++ {
		end := lastEnd.Add(-time.Duration(count-1-i) * cycle)
		start := end.Add(-8 * time.Hour)
		startInstant, err := domain.NewZonedInstant(start, zone)
		if err != nil {
			t.Fatalf("zoned start: %v", err)
		}
		endInstant, err := domain.NewZonedInstant(end, zone)
		if err != nil {
			t.Fatalf("zoned end: %v", err)
		}
		sessions = append(sessions, domain.SleepSession{
			ID:             domain.SleepSessionID("episode-" + time.Duration(i).String()),
			Classification: domain.SleepClassificationPrincipal,
			CreatedAt:      end,
			Intervals: []domain.SleepInterval{{
				Interval:      domain.TimeRange{Start: startInstant, End: endInstant},
				StartEvidence: domain.Evidence{Status: domain.StatusObserved, RecordedAt: end},
				EndEvidence:   domain.Evidence{Status: domain.StatusObserved, RecordedAt: end},
			}},
		})
	}
	return sessions
}

type fixture struct {
	now      time.Time
	sessions []domain.SleepSession
	estimate domain.PhaseEstimate
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	// A Wednesday, so the default Monday-to-Friday office days are exercised
	// without the horizon landing entirely on a weekend.
	now := time.Date(2026, 8, 5, 18, 0, 0, 0, time.UTC)
	sessions := history(t, now.Add(-2*time.Hour), 12)
	estimate, err := (estimation.RobustEstimator{}).Estimate(context.Background(), sessions, now)
	if err != nil {
		t.Fatalf("the fixture history does not estimate: %v", err)
	}
	return fixture{now: now, sessions: sessions, estimate: estimate}
}

func (f fixture) input() outlook.Input {
	return outlook.Input{
		Now:      f.now,
		Estimate: f.estimate,
		Freshness: freshness.Default().Assess(freshness.Inputs{
			Now:            f.now,
			NewestEvidence: f.now.Add(-2 * time.Hour),
			LatestSleepEnd: f.now.Add(-2 * time.Hour),
		}),
		OfficeHours: outlook.DefaultOfficeHours(zone),
	}
}

func build(t *testing.T, in outlook.Input) outlook.Outlook {
	t.Helper()
	view, err := outlook.Build(in)
	if err != nil {
		t.Fatalf("build outlook: %v", err)
	}
	return view
}

// TestTheTimelineCoversTheHorizonWithoutGaps. Segments are what everything else
// is measured against; a hole in them would silently drop office overlap and
// task availability rather than report anything wrong.
func TestTheTimelineCoversTheHorizonWithoutGaps(t *testing.T) {
	f := newFixture(t)
	view := build(t, f.input())

	if view.Status != outlook.StatusAvailable {
		t.Fatalf("status = %q, want available", view.Status)
	}
	if len(view.Segments) == 0 {
		t.Fatal("the timeline is empty")
	}
	if !view.Segments[0].Interval.Start.UTC.Equal(f.now) {
		t.Errorf("timeline starts at %s, want now (%s)", view.Segments[0].Interval.Start.UTC, f.now)
	}
	last := view.Segments[len(view.Segments)-1]
	if !last.Interval.End.UTC.Equal(view.Horizon.End.UTC) {
		t.Errorf("timeline ends at %s, want the horizon end (%s)", last.Interval.End.UTC, view.Horizon.End.UTC)
	}
	for i := 1; i < len(view.Segments); i++ {
		if !view.Segments[i].Interval.Start.UTC.Equal(view.Segments[i-1].Interval.End.UTC) {
			t.Fatalf("gap between segment %d and %d: %s then %s",
				i-1, i, view.Segments[i-1].Interval.End.UTC, view.Segments[i].Interval.Start.UTC)
		}
		if view.Segments[i].Presence == view.Segments[i-1].Presence &&
			view.Segments[i].Observed == view.Segments[i-1].Observed {
			t.Errorf("segments %d and %d were not merged; both are %q", i-1, i, view.Segments[i].Presence)
		}
	}
}

// TestTheBoundaryIsUncertainRatherThanSharp is the honesty requirement.
//
// The estimator's sleep and waking envelopes overlap on purpose, and collapsing
// them into a two-state timetable would draw a confident line where the
// measured P90 onset error is 5.41 hours. Every wake and every onset inside the
// horizon must therefore have a stretch that says so.
func TestTheBoundaryIsUncertainRatherThanSharp(t *testing.T) {
	f := newFixture(t)
	view := build(t, f.input())

	var uncertain int
	for i, segment := range view.Segments {
		if segment.Presence != outlook.PresenceUncertain {
			continue
		}
		uncertain++
		if i == 0 || i+1 >= len(view.Segments) {
			continue
		}
		before, after := view.Segments[i-1].Presence, view.Segments[i+1].Presence
		if before == after {
			t.Errorf("uncertain stretch %d sits between two %q segments; it is not a boundary", i, before)
		}
	}
	if uncertain == 0 {
		t.Fatal("no uncertain stretch in three days of forecast; the boundaries are being drawn sharp")
	}
	if view.UncertainFor == 0 {
		t.Error("uncertain time totalled zero while uncertain segments exist")
	}
	if view.AwakeFor == 0 {
		t.Error("three days of forecast contain no confidently awake time at all")
	}
}

// TestTheCurrentPeriodIsNotUnknown.
//
// The estimator's first waking window is the one *after* the next sleep, so the
// period the reader is actually in has no window of its own. Left alone, the
// next several hours — the most useful part of the whole view — read "unknown",
// and the screen answers "when am I awake today" with a shrug.
func TestTheCurrentPeriodIsNotUnknown(t *testing.T) {
	f := newFixture(t)
	view := build(t, f.input())

	if view.Segments[0].Presence != outlook.PresenceAwake {
		t.Fatalf("the view opens with %q; the reader is awake and looking at it",
			view.Segments[0].Presence)
	}
	if view.Segments[0].Duration() < time.Hour {
		t.Errorf("the current waking stretch is only %s", view.Segments[0].Duration())
	}
	// It must end where the next sleep could start, not at some later point
	// that would claim knowledge about the onset.
	if view.NextSleep == nil {
		t.Fatal("no next sleep window inside the horizon")
	}
	if !view.Segments[0].Interval.End.UTC.Equal(view.NextSleep.Start.UTC) {
		t.Errorf("the current stretch ends at %s, want the earliest plausible onset (%s)",
			view.Segments[0].Interval.End.UTC, view.NextSleep.Start.UTC)
	}
	for _, segment := range view.Segments {
		if segment.Presence == outlook.PresenceUnknown &&
			!segment.Interval.End.UTC.Equal(view.Horizon.End.UTC) {
			t.Errorf("an unknown stretch at %s sits inside the forecast, not past its end",
				segment.Interval.Start.UTC)
		}
	}
}

// TestUncertaintyWidensWithDistance. A forecast that is as sure about Friday as
// about tonight is not reporting its own error.
func TestUncertaintyWidensWithDistance(t *testing.T) {
	f := newFixture(t)
	in := f.input()
	in.Horizon = outlook.MaximumHorizon
	view := build(t, in)

	// Only whole stretches: the horizon clips the last one, and a truncated
	// band says nothing about the model's confidence.
	var widths []time.Duration
	for i, segment := range view.Segments {
		if segment.Presence != outlook.PresenceUncertain {
			continue
		}
		if i == 0 || i+1 == len(view.Segments) {
			continue
		}
		widths = append(widths, segment.Duration())
	}
	if len(widths) < 2 {
		t.Fatalf("only %d whole uncertain stretches to compare", len(widths))
	}
	first, last := widths[0], widths[len(widths)-1]
	if last <= first {
		t.Errorf("uncertainty did not widen: first stretch %s, last %s", first, last)
	}
}

// TestARecordedSleepBeatsThePrediction. A stored episode is evidence; the
// forecast is a guess about the same hours.
func TestARecordedSleepBeatsThePrediction(t *testing.T) {
	f := newFixture(t)
	in := f.input()

	// An episode already under way, recorded, running two hours into the view.
	start, err := domain.NewZonedInstant(f.now.Add(-time.Hour), zone)
	if err != nil {
		t.Fatal(err)
	}
	end, err := domain.NewZonedInstant(f.now.Add(2*time.Hour), zone)
	if err != nil {
		t.Fatal(err)
	}
	in.RecordedSleep = []domain.TimeRange{{Start: start, End: end}}

	view := build(t, in)
	if view.Segments[0].Presence != outlook.PresenceAsleep {
		t.Errorf("first segment = %q, want asleep from the recorded episode", view.Segments[0].Presence)
	}
	if !view.Segments[0].Observed {
		t.Error("the recorded stretch is not marked as observed")
	}
	if !view.Segments[0].Interval.End.UTC.Equal(f.now.Add(2 * time.Hour)) {
		t.Errorf("the observed stretch ends at %s, want the record's end",
			view.Segments[0].Interval.End.UTC)
	}
	for _, segment := range view.Segments[1:] {
		if segment.Observed {
			t.Error("a forecast stretch was marked as observed")
		}
	}
}

// TestOfficeWindowsSeparateReachableFromPossible. The two are never added
// together: a window that is only "possible" rests on a boundary the model has
// not pinned down, and treating it as usable is how somebody sleeps through the
// one call they needed to make this week.
func TestOfficeWindowsSeparateReachableFromPossible(t *testing.T) {
	f := newFixture(t)
	view := build(t, f.input())

	if len(view.OfficeWindows) == 0 {
		t.Fatal("no office windows in three days")
	}
	for i, window := range view.OfficeWindows {
		if window.ReachableFor != durationOf(window.Reachable) {
			t.Errorf("window %d: reachableFor %s disagrees with its intervals", i, window.ReachableFor)
		}
		if window.PossibleFor != durationOf(window.Possible) {
			t.Errorf("window %d: possibleFor %s disagrees with its intervals", i, window.PossibleFor)
		}
		span := window.Interval.End.UTC.Sub(window.Interval.Start.UTC)
		if window.ReachableFor+window.PossibleFor > span {
			t.Errorf("window %d claims %s of a %s window", i, window.ReachableFor+window.PossibleFor, span)
		}
		if window.Usable() != (window.ReachableFor > 0) {
			t.Errorf("window %d: Usable disagrees with its own reachable time", i)
		}
		for _, reachable := range window.Reachable {
			if reachable.Start.UTC.Before(window.Interval.Start.UTC) ||
				reachable.End.UTC.After(window.Interval.End.UTC) {
				t.Errorf("window %d: reachable stretch escapes the office hours", i)
			}
		}
	}
}

// TestOfficeHoursSkipTheWeekend keeps the civil-clock arithmetic honest. It is
// the whole point of the office view that Saturday is not a business day
// however awake the user is.
func TestOfficeHoursSkipTheWeekend(t *testing.T) {
	// Friday evening: the 72-hour horizon runs across Saturday and Sunday and
	// into Monday morning.
	now := time.Date(2026, 8, 7, 22, 0, 0, 0, time.UTC)
	sessions := history(t, now.Add(-2*time.Hour), 12)
	estimate, err := (estimation.RobustEstimator{}).Estimate(context.Background(), sessions, now)
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	view := build(t, outlook.Input{
		Now:      now,
		Estimate: estimate,
		Freshness: freshness.Default().Assess(freshness.Inputs{
			Now: now, NewestEvidence: now.Add(-2 * time.Hour), LatestSleepEnd: now.Add(-2 * time.Hour),
		}),
		OfficeHours: outlook.DefaultOfficeHours(zone),
	})

	location, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range view.OfficeWindows {
		day := window.Interval.Start.UTC.In(location).Weekday()
		if day == time.Saturday || day == time.Sunday {
			t.Errorf("an office window landed on %s", day)
		}
	}
}

// TestACommitmentInsidePredictedSleepIsNamedAsSuch. For a drifting rhythm this
// is the ordinary case for anything booked more than a few days out, and it is
// the most useful thing this view can point at.
func TestACommitmentInsidePredictedSleepIsNamedAsSuch(t *testing.T) {
	f := newFixture(t)
	view := build(t, f.input())

	var asleep *domain.TimeRange
	for _, segment := range view.Segments {
		if segment.Presence == outlook.PresenceAsleep && segment.Duration() > 2*time.Hour {
			interval := segment.Interval
			asleep = &interval
			break
		}
	}
	if asleep == nil {
		t.Fatal("no predicted sleep stretch long enough to book an appointment inside")
	}

	start, err := domain.NewZonedInstant(asleep.Start.UTC.Add(30*time.Minute), zone)
	if err != nil {
		t.Fatal(err)
	}
	end, err := domain.NewZonedInstant(asleep.Start.UTC.Add(90*time.Minute), zone)
	if err != nil {
		t.Fatal(err)
	}

	in := f.input()
	in.Events = []domain.CalendarEvent{{
		ID:       "appointment-1",
		Title:    "Dentist",
		Interval: domain.TimeRange{Start: start, End: end},
		Fixed:    true,
	}}
	withEvent := build(t, in)

	if len(withEvent.Commitments) != 1 {
		t.Fatalf("commitment count = %d, want 1", len(withEvent.Commitments))
	}
	if withEvent.Commitments[0].Conflict != outlook.ConflictInsidePredictedSleep {
		t.Errorf("conflict = %q, want inside_predicted_sleep", withEvent.Commitments[0].Conflict)
	}
}

// TestCommitmentsCarryNoPrivateText. Imported event titles stay in the local
// trust zone (ADR-0023); a caller entitled to render them joins by id, and one
// that is not cannot obtain them by accident.
func TestCommitmentsCarryNoPrivateText(t *testing.T) {
	f := newFixture(t)
	in := f.input()
	start, err := domain.NewZonedInstant(f.now.Add(time.Hour), zone)
	if err != nil {
		t.Fatal(err)
	}
	end, err := domain.NewZonedInstant(f.now.Add(2*time.Hour), zone)
	if err != nil {
		t.Fatal(err)
	}
	in.Events = []domain.CalendarEvent{{
		ID:       "appointment-2",
		Title:    "Oncology follow-up",
		Location: "Ward 4",
		Notes:    "bring referral letter",
		Interval: domain.TimeRange{Start: start, End: end},
		Fixed:    true,
	}}

	view := build(t, in)
	if len(view.Commitments) != 1 {
		t.Fatalf("commitment count = %d, want 1", len(view.Commitments))
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal outlook: %v", err)
	}
	for _, secret := range []string{"Oncology", "Ward 4", "referral"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Errorf("private event text %q reached the outlook", secret)
		}
	}
	if view.Commitments[0].EventID != "appointment-2" {
		t.Errorf("event id = %q, want the join key", view.Commitments[0].EventID)
	}
}

// TestTasksArePlacedOnlyInConfidentlyAwakeTime. Availability is the confident
// core, not the full predicted waking envelope: suggesting a task for a stretch
// where the model does not know whether the person is up is how a suggestion
// lands mid-sleep.
func TestTasksArePlacedOnlyInConfidentlyAwakeTime(t *testing.T) {
	f := newFixture(t)
	in := f.input()
	in.Tasks = []domain.FlexibleTask{{
		ID:                "task-call",
		Title:             "Ring the pharmacy",
		EstimatedDuration: 20 * time.Minute,
		Constraint:        domain.TaskConstraint{MinimumConfidence: domain.ConfidenceLow, RequiresApproval: true},
	}}

	view := build(t, in)
	if len(view.Opportunities) != 1 {
		t.Fatalf("opportunity count = %d, want 1", len(view.Opportunities))
	}
	opportunity := view.Opportunities[0]
	if opportunity.Window == nil {
		t.Fatalf("the task was not placed: %q", opportunity.Unplaced)
	}
	if !opportunity.NeedsApproval {
		t.Error("a placement claimed it needs no approval")
	}

	inside := false
	for _, segment := range view.Segments {
		if segment.Presence != outlook.PresenceAwake {
			continue
		}
		if !opportunity.Window.Start.UTC.Before(segment.Interval.Start.UTC) &&
			!opportunity.Window.End.UTC.After(segment.Interval.End.UTC) {
			inside = true
			break
		}
	}
	if !inside {
		t.Errorf("the placement at %s is not inside a confidently awake stretch",
			opportunity.Window.Start.UTC)
	}
}

// TestAnUnplaceableTaskSaysWhy rather than disappearing from the list.
func TestAnUnplaceableTaskSaysWhy(t *testing.T) {
	f := newFixture(t)
	in := f.input()
	in.Tasks = []domain.FlexibleTask{{
		ID:                "task-marathon",
		Title:             "Something impossible",
		EstimatedDuration: 30 * time.Hour,
		Constraint:        domain.TaskConstraint{MinimumConfidence: domain.ConfidenceLow},
	}}

	view := build(t, in)
	if len(view.Opportunities) != 1 {
		t.Fatalf("opportunity count = %d, want 1", len(view.Opportunities))
	}
	if view.Opportunities[0].Window != nil {
		t.Fatal("a thirty-hour task was placed inside a waking window")
	}
	if view.Opportunities[0].Unplaced == "" {
		t.Error("the task vanished without a reason")
	}
}

// TestStaleEvidenceWithholdsTheWholeView.
//
// A forecast is anchored to where the person is now in their cycle. Without an
// anchor the next three days are a shape, not a plan, and drawing office
// windows over it would invite someone to book a call against arithmetic.
func TestStaleEvidenceWithholdsTheWholeView(t *testing.T) {
	f := newFixture(t)
	in := f.input()
	in.Freshness = freshness.Default().Assess(freshness.Inputs{
		Now:            f.now,
		NewestEvidence: f.now.Add(-3 * 24 * time.Hour),
		LatestSleepEnd: f.now.Add(-3 * 24 * time.Hour),
	})

	view := build(t, in)
	if view.Status != outlook.StatusWithheld {
		t.Fatalf("status = %q, want withheld", view.Status)
	}
	if len(view.Segments) != 0 || len(view.OfficeWindows) != 0 || len(view.Opportunities) != 0 {
		t.Error("a withheld view still published a timeline")
	}
	if view.Freshness.Reason == "" {
		t.Error("the withholding carries no reason for the reader")
	}
}

// TestARefusedEstimateKeepsItsOwnReason. "Not enough history yet" and "these
// records contradict each other" need different things from the reader.
func TestARefusedEstimateKeepsItsOwnReason(t *testing.T) {
	f := newFixture(t)
	_, err := (estimation.RobustEstimator{}).Estimate(context.Background(), nil, f.now)
	if err == nil {
		t.Fatal("an empty history estimated successfully")
	}

	in := f.input()
	in.Estimate = domain.PhaseEstimate{}
	in.EstimateError = err

	view := build(t, in)
	if view.Status != outlook.StatusRefused {
		t.Fatalf("status = %q, want refused", view.Status)
	}
	if view.Refusal == nil || view.Refusal.Code != string(estimation.RefusalInsufficientData) {
		t.Errorf("refusal = %+v, want the estimator's own code", view.Refusal)
	}
}

// TestNoEstimateIsRefusedRatherThanEmpty guards the path where a caller passes
// a zero estimate with no error: an empty timeline that claims to be available
// would read as "you are asleep for three days".
func TestNoEstimateIsRefusedRatherThanEmpty(t *testing.T) {
	view := build(t, outlook.Input{
		Now:         time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
		OfficeHours: outlook.DefaultOfficeHours(zone),
	})
	if view.Status != outlook.StatusRefused {
		t.Errorf("status = %q, want refused", view.Status)
	}
	if len(view.Segments) != 0 {
		t.Error("a refused view published a timeline")
	}
}

// TestTheHorizonStaysInsideItsBounds. Below two days the view stops answering
// "which day"; beyond four the bounds are wide enough that it answers nothing.
func TestTheHorizonStaysInsideItsBounds(t *testing.T) {
	f := newFixture(t)
	for _, testCase := range []struct {
		name    string
		horizon time.Duration
		want    time.Duration
	}{
		{"zero means the default", 0, outlook.DefaultHorizon},
		{"below the floor is raised", 6 * time.Hour, outlook.MinimumHorizon},
		{"above the ceiling is capped", 30 * 24 * time.Hour, outlook.MaximumHorizon},
		{"in range is honoured", 60 * time.Hour, 60 * time.Hour},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			in := f.input()
			in.Horizon = testCase.horizon
			view := build(t, in)
			got := view.Horizon.End.UTC.Sub(view.Horizon.Start.UTC)
			if got != testCase.want {
				t.Errorf("horizon = %s, want %s", got, testCase.want)
			}
		})
	}
}

// TestTheViewSurvivesDaylightSaving. The sweep runs on absolute instants; doing
// it on civil times would put the transition inside a night and quietly create
// or lose an hour of predicted sleep. The office day, by contrast, must move
// with the civil clock, because that is what an office does.
func TestTheViewSurvivesDaylightSaving(t *testing.T) {
	// US daylight saving ends on 1 November 2026.
	now := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	sessions := history(t, now.Add(-2*time.Hour), 12)
	estimate, err := (estimation.RobustEstimator{}).Estimate(context.Background(), sessions, now)
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	view := build(t, outlook.Input{
		Now:      now,
		Horizon:  outlook.MaximumHorizon,
		Estimate: estimate,
		Freshness: freshness.Default().Assess(freshness.Inputs{
			Now: now, NewestEvidence: now.Add(-2 * time.Hour), LatestSleepEnd: now.Add(-2 * time.Hour),
		}),
		OfficeHours: outlook.DefaultOfficeHours(zone),
	})

	location, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	for i, window := range view.OfficeWindows {
		local := window.Interval.Start.UTC.In(location)
		// Windows are clipped to now and to the horizon end, so only check the
		// ones that start at a real opening time.
		if local.Hour() == 9 && local.Minute() == 0 {
			closing := window.Interval.End.UTC.In(location)
			if closing.Hour() != 17 && !window.Interval.End.UTC.Equal(view.Horizon.End.UTC) {
				t.Errorf("office window %d closes at %s local, want 17:00", i, closing.Format("15:04"))
			}
		}
	}
	// The timeline must still be contiguous across the transition.
	for i := 1; i < len(view.Segments); i++ {
		if !view.Segments[i].Interval.Start.UTC.Equal(view.Segments[i-1].Interval.End.UTC) {
			t.Fatalf("daylight saving opened a gap at segment %d", i)
		}
	}
}

// TestADriftingRhythmLosesAndRegainsOfficeAccess is the product claim, checked.
//
// Someone whose day is fifty minutes long walks out of office hours and back
// into them over a fortnight. A view that reported the same reachable time
// every day would not be reporting anything.
func TestADriftingRhythmLosesAndRegainsOfficeAccess(t *testing.T) {
	generated, err := simulate.Generate(simulate.Params{
		Seed:           7,
		Start:          time.Date(2026, 6, 1, 2, 0, 0, 0, time.UTC),
		ZoneID:         zone,
		Segments:       []simulate.Segment{{Cycles: 40, Period: cycle}},
		Duration:       8 * time.Hour,
		OnsetJitter:    20 * time.Minute,
		DurationJitter: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(generated.Sessions) < 12 {
		t.Fatalf("generator produced %d sessions", len(generated.Sessions))
	}

	seen := map[bool]int{}
	for day := 0; day < 14; day++ {
		now := generated.Sessions[11].Intervals[0].Interval.End.UTC.Add(time.Duration(day) * 24 * time.Hour)
		var window []domain.SleepSession
		for _, session := range generated.Sessions {
			if session.Intervals[0].Interval.End.UTC.Before(now) {
				window = append(window, session)
			}
		}
		if len(window) < 8 {
			continue
		}
		estimate, err := (estimation.RobustEstimator{}).Estimate(context.Background(), window, now)
		if err != nil {
			continue
		}
		newest := window[len(window)-1].Intervals[0].Interval.End.UTC
		view, err := outlook.Build(outlook.Input{
			Now:      now,
			Estimate: estimate,
			Freshness: freshness.Default().Assess(freshness.Inputs{
				Now: now, NewestEvidence: now.Add(-time.Hour), LatestSleepEnd: newest,
			}),
			OfficeHours: outlook.DefaultOfficeHours(zone),
		})
		if err != nil {
			t.Fatalf("day %d: %v", day, err)
		}
		if view.Status != outlook.StatusAvailable {
			continue
		}
		reachable := time.Duration(0)
		for _, office := range view.OfficeWindows {
			reachable += office.ReachableFor
		}
		seen[reachable > 0]++
	}

	if seen[true] == 0 {
		t.Error("office hours were never reachable across a fortnight of drift")
	}
	if seen[false] == 0 {
		t.Error("office hours were reachable on every single day; the view is not tracking the drift")
	}
}

func durationOf(ranges []domain.TimeRange) time.Duration {
	var total time.Duration
	for _, interval := range ranges {
		total += interval.End.UTC.Sub(interval.Start.UTC)
	}
	return total
}
