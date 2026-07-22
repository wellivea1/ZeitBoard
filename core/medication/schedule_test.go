package medication

import (
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestFixedClockSchedulePreservesCivilTimeAcrossDST(t *testing.T) {
	schedule := Schedule{
		Kind:       ScheduleFixedClock,
		ZoneID:     "America/New_York",
		CivilTimes: []string{"09:00"},
	}
	expansion, err := ExpandSchedule(schedule,
		time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(expansion.Occurrences) != 4 {
		t.Fatalf("occurrences = %#v", expansion.Occurrences)
	}
	if got := expansion.Occurrences[0].At.UTC.Hour(); got != 14 {
		t.Fatalf("pre-DST UTC hour = %d, want 14", got)
	}
	if got := expansion.Occurrences[1].At.UTC.Hour(); got != 13 {
		t.Fatalf("post-DST UTC hour = %d, want 13", got)
	}
	for _, occurrence := range expansion.Occurrences {
		if occurrence.CivilTime != "09:00" {
			t.Fatalf("civil time drifted: %#v", occurrence)
		}
	}
}

func TestScheduleExpansionReportsNonexistentAndDeduplicatesRepeatedCivilTime(t *testing.T) {
	spring := Schedule{Kind: ScheduleFixedClock, ZoneID: "America/New_York", CivilTimes: []string{"02:30"}}
	expansion, err := ExpandSchedule(spring,
		time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(expansion.Occurrences) != 0 || len(expansion.Gaps) != 1 || expansion.Gaps[0].Reason != "nonexistent_civil_time" {
		t.Fatalf("spring expansion = %#v", expansion)
	}
	expansion, err = ExpandSchedule(spring,
		time.Date(2026, 3, 8, 8, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(expansion.Gaps) != 0 {
		t.Fatalf("gap before interval was reported: %#v", expansion.Gaps)
	}

	fall := Schedule{Kind: ScheduleFixedClock, ZoneID: "America/New_York", CivilTimes: []string{"01:30"}}
	expansion, err = ExpandSchedule(fall,
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(expansion.Occurrences) != 1 || !expansion.Occurrences[0].Ambiguous {
		t.Fatalf("fall expansion = %#v", expansion)
	}
	if got := expansion.Occurrences[0].At.UTC; !got.Equal(time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)) {
		t.Fatalf("fall occurrence = %v", got)
	}
}

func TestCyclingScheduleUsesCivilDates(t *testing.T) {
	schedule := Schedule{
		Kind:           ScheduleCycling,
		ZoneID:         "America/New_York",
		CivilTimes:     []string{"08:00"},
		DaysOn:         2,
		DaysOff:        1,
		CycleStartedOn: "2026-03-07",
	}
	expansion, err := ExpandSchedule(schedule,
		time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 13, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	dates := make([]string, 0, len(expansion.Occurrences))
	for _, occurrence := range expansion.Occurrences {
		dates = append(dates, occurrence.CivilDate)
	}
	want := []string{"2026-03-07", "2026-03-08", "2026-03-10", "2026-03-11"}
	if len(dates) != len(want) {
		t.Fatalf("cycling dates = %v", dates)
	}
	for index := range want {
		if dates[index] != want[index] {
			t.Fatalf("cycling dates = %v, want %v", dates, want)
		}
	}
}

func TestCollisionForecastSeparatesOverlapClearAndUnknown(t *testing.T) {
	occurrences := ScheduleExpansion{Occurrences: []ScheduleOccurrence{
		{At: domain.MustZonedInstant(time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC), "UTC"), CivilDate: "2026-07-22", CivilTime: "12:00"},
		{At: domain.MustZonedInstant(time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC), "UTC"), CivilDate: "2026-07-22", CivilTime: "18:00"},
		{At: domain.MustZonedInstant(time.Date(2026, 7, 23, 15, 0, 0, 0, time.UTC), "UTC"), CivilDate: "2026-07-23", CivilTime: "15:00"},
	}}
	confidence := domain.InferenceConfidence{Level: domain.ConfidenceMedium}
	estimate := &domain.PhaseEstimate{
		ID:         "estimate_medication_forecast",
		Confidence: confidence,
		PredictedSleepWindows: []domain.AvailabilityWindow{
			{
				ID:         "sleep_forecast_01",
				Kind:       domain.AvailabilityPredictedSleep,
				Interval:   domain.TimeRange{Start: domain.MustZonedInstant(time.Date(2026, 7, 22, 11, 0, 0, 0, time.UTC), "UTC"), End: domain.MustZonedInstant(time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC), "UTC")},
				Confidence: confidence,
			},
			{
				ID:         "sleep_forecast_02",
				Kind:       domain.AvailabilityPredictedSleep,
				Interval:   domain.TimeRange{Start: domain.MustZonedInstant(time.Date(2026, 7, 23, 11, 0, 0, 0, time.UTC), "UTC"), End: domain.MustZonedInstant(time.Date(2026, 7, 23, 15, 0, 0, 0, time.UTC), "UTC")},
				Confidence: confidence,
			},
		},
	}
	forecast, err := AnalyzeCollisions(occurrences, estimate, time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if forecast.Status != ForecastCollision || forecast.CoveredCount != 2 || forecast.CollisionCount != 1 || forecast.OutsideHorizonCount != 1 {
		t.Fatalf("forecast = %#v", forecast)
	}
	want := []string{OccurrenceInsidePredictedSleep, OccurrenceOutsidePredictedSleep, OccurrenceOutsideForecast}
	for index := range want {
		if forecast.Assessments[index].Status != want[index] {
			t.Fatalf("assessment %d = %#v", index, forecast.Assessments[index])
		}
	}
}

func TestAsNeededScheduleCannotEnableReminders(t *testing.T) {
	if err := (Schedule{Kind: ScheduleAsNeeded, ReminderEnabled: true}).Validate(); err == nil {
		t.Fatal("as-needed schedule enabled a timed reminder")
	}
	if err := (Schedule{Kind: ScheduleFixedClock, ZoneID: "Local", CivilTimes: []string{"09:00"}}).Validate(); err == nil {
		t.Fatal("machine-local pseudo-zone was accepted as an explicit schedule zone")
	}
	if err := (Schedule{Kind: ScheduleFixedClock, ZoneID: "GMT", CivilTimes: []string{"09:00"}}).Validate(); err == nil {
		t.Fatal("zone alias outside the v1 contract was accepted")
	}
	expansion, err := ExpandSchedule(Schedule{Kind: ScheduleAsNeeded}, time.Now(), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	forecast, err := AnalyzeCollisions(expansion, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if forecast.Status != ForecastNotApplicable {
		t.Fatalf("as-needed forecast = %#v", forecast)
	}

	noDoseExpansion, err := ExpandSchedule(Schedule{
		Kind:           ScheduleCycling,
		ZoneID:         "UTC",
		CivilTimes:     []string{"09:00"},
		DaysOn:         1,
		DaysOff:        1,
		CycleStartedOn: "2026-07-23",
	}, time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	forecast, err = AnalyzeCollisions(noDoseExpansion, nil, time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if forecast.Status != ForecastUnavailable {
		t.Fatalf("empty timed forecast = %#v", forecast)
	}
}
