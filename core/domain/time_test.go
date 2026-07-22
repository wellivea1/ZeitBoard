package domain

import (
	"testing"
	"time"
)

func TestZonedInstantPreservesUTCThroughDSTTransition(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Date(2026, 3, 8, 1, 30, 0, 0, location)
	after := before.Add(2 * time.Hour)
	start := MustZonedInstant(before, location.String())
	end := MustZonedInstant(after, location.String())

	if got := end.UTC.Sub(start.UTC); got != 2*time.Hour {
		t.Fatalf("elapsed time = %v, want 2h", got)
	}
	if start.UTC.Location() != time.UTC || end.UTC.Location() != time.UTC {
		t.Fatal("stored instants must be UTC")
	}
}

func TestTimeRangeSpansMidnightWithoutCivilDayAssumption(t *testing.T) {
	location, _ := time.LoadLocation("America/New_York")
	start := MustZonedInstant(time.Date(2026, 6, 15, 23, 30, 0, 0, location), location.String())
	end := MustZonedInstant(time.Date(2026, 6, 16, 8, 0, 0, 0, location), location.String())
	rangeValue := TimeRange{Start: start, End: end}
	if err := rangeValue.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := rangeValue.Duration(); got != 8*time.Hour+30*time.Minute {
		t.Fatalf("duration = %v", got)
	}
}

func TestResolveCivilTimeRejectsGapAndUsesFirstRepeatedInstant(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCivilTime(location, 2026, time.March, 8, 2, 30, 0); err == nil {
		t.Fatal("nonexistent spring-forward wall time was accepted")
	}
	repeated, err := ResolveCivilTime(location, 2026, time.November, 1, 1, 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !repeated.Ambiguous {
		t.Fatal("fall-back wall time was not marked ambiguous")
	}
	if got := repeated.Time.UTC(); !got.Equal(time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)) {
		t.Fatalf("repeated wall time = %v, want first occurrence at 05:30Z", got)
	}
}
