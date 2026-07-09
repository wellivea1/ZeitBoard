package simulate

import "time"

// The scenario catalog implements the sleep-timing scenarios from the
// validation plan §4. Each constructor documents the plan scenario it encodes;
// the expected behaviors are asserted by the validation suite in
// scenarios_test.go. Scenarios 11-15 and 18-19 require multi-source streams
// (wearable/phone/desktop conflict, calendar) that no component consumes yet;
// scenario 20 (data corruption) is covered by construction in tests because
// corruption is rejected at the ingestion boundary, not generated as history.

func baseStart() time.Time {
	return time.Date(2026, 1, 5, 4, 30, 0, 0, time.UTC) // 23:30 EST on Jan 4
}

// Scenario1StableSleep: stable nocturnal sleep, 23:30 +/- 20 min, 24h period.
func Scenario1StableSleep() Params {
	return Params{
		Seed:        1,
		Start:       baseStart(),
		ZoneID:      "America/New_York",
		Segments:    []Segment{{Cycles: 21, Period: 24 * time.Hour}},
		Duration:    8*time.Hour + 30*time.Minute,
		OnsetJitter: 20 * time.Minute,
	}
}

// Scenario2SlightVariability: entrained rhythm with +/- 60 min onset noise.
func Scenario2SlightVariability() Params {
	return Params{
		Seed:        2,
		Start:       baseStart(),
		ZoneID:      "America/New_York",
		Segments:    []Segment{{Cycles: 21, Period: 24 * time.Hour}},
		Duration:    8 * time.Hour,
		OnsetJitter: 60 * time.Minute,
	}
}

// Scenario3Tau24_2: a subtle 0.2 h/cycle delay.
func Scenario3Tau24_2() Params {
	return Params{
		Seed:        3,
		Start:       baseStart(),
		ZoneID:      "America/New_York",
		Segments:    []Segment{{Cycles: 21, Period: 24*time.Hour + 12*time.Minute}},
		Duration:    8*time.Hour + 30*time.Minute,
		OnsetJitter: 20 * time.Minute,
	}
}

// Scenario4Tau25: classic 25-hour free-running rhythm.
func Scenario4Tau25() Params {
	return Params{
		Seed:        4,
		Start:       baseStart(),
		ZoneID:      "America/New_York",
		Segments:    []Segment{{Cycles: 18, Period: 25 * time.Hour}},
		Duration:    9 * time.Hour,
		OnsetJitter: 30 * time.Minute,
	}
}

// Scenario5Tau26: aggressive 26-hour rhythm crossing every civil boundary.
func Scenario5Tau26() Params {
	return Params{
		Seed:        5,
		Start:       baseStart(),
		ZoneID:      "America/New_York",
		Segments:    []Segment{{Cycles: 18, Period: 26 * time.Hour}},
		Duration:    9 * time.Hour,
		OnsetJitter: 30 * time.Minute,
	}
}

// Scenario6TemporaryAlignment: drift, a stable stretch, then renewed drift.
func Scenario6TemporaryAlignment() Params {
	return Params{
		Seed:   6,
		Start:  baseStart(),
		ZoneID: "America/New_York",
		Segments: []Segment{
			{Cycles: 10, Period: 25*time.Hour + 48*time.Minute},
			{Cycles: 10, Period: 24 * time.Hour},
			{Cycles: 10, Period: 25*time.Hour + 30*time.Minute},
		},
		Duration:    8*time.Hour + 30*time.Minute,
		OnsetJitter: 20 * time.Minute,
	}
}

// Scenario7ForcedWake: the latent rhythm keeps delaying while an alarm forces
// 08:00 wake on weekdays; weekends rebound.
func Scenario7ForcedWake() Params {
	return Params{
		Seed:        7,
		Start:       baseStart(),
		ZoneID:      "America/New_York",
		Segments:    []Segment{{Cycles: 18, Period: 25 * time.Hour}},
		Duration:    9 * time.Hour,
		OnsetJitter: 20 * time.Minute,
		ForcedWake: &ForcedWake{
			FromCycle:      4,
			ToCycle:        11,
			WakeCivilHour:  8,
			WeekendRebound: true,
		},
	}
}

// Scenario8Deprivation: one skipped night (an extended wake interval) with a
// short recovery sleep, inside an otherwise free-running rhythm.
func Scenario8Deprivation() Params {
	return Params{
		Seed:        8,
		Start:       baseStart(),
		ZoneID:      "America/New_York",
		Segments:    []Segment{{Cycles: 18, Period: 25 * time.Hour}},
		Duration:    8*time.Hour + 30*time.Minute,
		OnsetJitter: 25 * time.Minute,
		Deprivation: map[int]bool{9: true},
	}
}

// Scenario9Naps: main sleep plus two naps per waking period.
func Scenario9Naps() Params {
	return Params{
		Seed:         9,
		Start:        baseStart(),
		ZoneID:       "America/New_York",
		Segments:     []Segment{{Cycles: 18, Period: 24*time.Hour + 48*time.Minute}},
		Duration:     8 * time.Hour,
		OnsetJitter:  20 * time.Minute,
		NapsPerCycle: 2,
		NapDuration:  45 * time.Minute,
	}
}

// Scenario10Fragmented: main sleep split by a 45-minute wake gap.
func Scenario10Fragmented() Params {
	return Params{
		Seed:          10,
		Start:         baseStart(),
		ZoneID:        "America/New_York",
		Segments:      []Segment{{Cycles: 18, Period: 24*time.Hour + 48*time.Minute}},
		Duration:      8*time.Hour + 30*time.Minute,
		OnsetJitter:   20 * time.Minute,
		FragmentAfter: 4 * time.Hour,
		FragmentGap:   45 * time.Minute,
	}
}

// Scenario16Travel: a three-hour zone change mid-history; the absolute rhythm
// is unchanged, only civil rendering moves.
func Scenario16Travel() Params {
	return Params{
		Seed:        16,
		Start:       baseStart(),
		ZoneID:      "America/New_York",
		Segments:    []Segment{{Cycles: 18, Period: 24*time.Hour + 54*time.Minute}},
		Duration:    8*time.Hour + 30*time.Minute,
		OnsetJitter: 20 * time.Minute,
		ZoneShifts:  map[int]string{9: "America/Los_Angeles"},
	}
}

// Scenario17DaylightSaving: a free-running rhythm across the US spring-forward
// transition (2026-03-08 in America/New_York).
func Scenario17DaylightSaving() Params {
	return Params{
		Seed:        17,
		Start:       time.Date(2026, 3, 1, 4, 30, 0, 0, time.UTC),
		ZoneID:      "America/New_York",
		Segments:    []Segment{{Cycles: 18, Period: 24*time.Hour + 36*time.Minute}},
		Duration:    8*time.Hour + 30*time.Minute,
		OnsetJitter: 20 * time.Minute,
	}
}
