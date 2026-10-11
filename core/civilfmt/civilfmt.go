// Package civilfmt writes civil times on the reader's clock (ADR-0057).
//
// A layout names its time of day the twelve-hour way, "3:04 PM", as Go's own
// reference time does; Format writes that part on the reader's clock and leaves
// the rest of the layout alone. Names of days and months stay English until
// there is a catalog to take them from.
package civilfmt

import (
	"strings"
	"time"
)

// Clock is how a reader writes the time of day.
type Clock uint8

const (
	// TwelveHour writes 3:04 PM. It is the zero value: it is what ZeitBoard
	// wrote before it knew its reader, and what a writer that cannot know its
	// reader still writes.
	TwelveHour Clock = iota

	// TwentyFourHour writes 15:04.
	TwentyFourHour
)

const twelveHourTime = "3:04 PM"

// Layout is the clock's time of day as a time.Format layout.
func (c Clock) Layout() string {
	if c == TwentyFourHour {
		return "15:04"
	}
	return twelveHourTime
}

// Time is the time of day: "3:04 PM" or "15:04".
func (c Clock) Time(t time.Time) string {
	return t.Format(c.Layout())
}

// Format writes t with layout, its "3:04 PM" written on the clock.
func (c Clock) Format(t time.Time, layout string) string {
	return t.Format(strings.Replace(layout, twelveHourTime, c.Layout(), 1))
}

// Hour is a whole hour of the day, for an axis: "6 PM" or "18:00".
func (c Clock) Hour(hour int) string {
	value := time.Date(2000, 1, 1, ((hour%24)+24)%24, 0, 0, 0, time.UTC)
	if c == TwentyFourHour {
		return value.Format("15:04")
	}
	return value.Format("3 PM")
}

// Civil writes an "HH:MM" civil time, as settings and schedules store it, on
// the clock. A value that is not one is returned as it is, so a reader sees
// what was stored rather than nothing.
func (c Clock) Civil(clock string) string {
	parsed, err := time.Parse("15:04", clock)
	if err != nil {
		return clock
	}
	return c.Time(parsed)
}
