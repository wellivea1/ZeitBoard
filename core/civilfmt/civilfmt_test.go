package civilfmt

import (
	"testing"
	"time"
)

func TestTheClockWritesOnlyTheTimeOfDay(t *testing.T) {
	evening := time.Date(2026, 10, 11, 21, 5, 0, 0, time.UTC)
	morning := time.Date(2026, 10, 12, 9, 40, 0, 0, time.UTC)
	for _, testCase := range []struct {
		clock  Clock
		t      time.Time
		layout string
		want   string
	}{
		{TwelveHour, evening, "Jan 2, 3:04 PM", "Oct 11, 9:05 PM"},
		{TwentyFourHour, evening, "Jan 2, 3:04 PM", "Oct 11, 21:05"},
		{TwentyFourHour, morning, "Mon, Jan 2, 3:04 PM MST", "Mon, Oct 12, 09:40 UTC"},
		{TwentyFourHour, morning, "3:04 PM on Mon, Jan 2", "09:40 on Mon, Oct 12"},
		// A layout without a time of day is left alone.
		{TwentyFourHour, morning, "Jan 2, 2006", "Oct 12, 2026"},
	} {
		if got := testCase.clock.Format(testCase.t, testCase.layout); got != testCase.want {
			t.Errorf("%v.Format(%q) = %q, want %q", testCase.clock, testCase.layout, got, testCase.want)
		}
	}
	if got := TwentyFourHour.Time(evening); got != "21:05" {
		t.Errorf("Time = %q", got)
	}
}

// The zero clock is the twelve-hour one, so a writer that never learns its
// reader's clock writes what it always did.
func TestTheZeroClockIsTheTwelveHourOne(t *testing.T) {
	var clock Clock
	if clock != TwelveHour || clock.Time(time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)) != "12:30 AM" {
		t.Error("the zero clock is not the twelve-hour clock")
	}
}

func TestHoursAndCivilTimes(t *testing.T) {
	for _, testCase := range []struct {
		got, want string
	}{
		{TwelveHour.Hour(18), "6 PM"},
		{TwelveHour.Hour(0), "12 AM"},
		{TwentyFourHour.Hour(18), "18:00"},
		{TwentyFourHour.Hour(24), "00:00"},
		{TwentyFourHour.Hour(-6), "18:00"},
		{TwelveHour.Civil("17:00"), "5:00 PM"},
		{TwentyFourHour.Civil("07:30"), "07:30"},
		{TwentyFourHour.Civil("9am"), "9am"},
	} {
		if testCase.got != testCase.want {
			t.Errorf("got %q, want %q", testCase.got, testCase.want)
		}
	}
}
