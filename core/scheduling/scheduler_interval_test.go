package scheduling

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestSubtractEventsMatchesRepeatedSplittingReference(t *testing.T) {
	t.Parallel()

	zones := []string{"UTC", "America/New_York", "Europe/Berlin", "Asia/Kathmandu"}
	random := rand.New(rand.NewSource(20260728))
	base := time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)

	for iteration := 0; iteration < 500; iteration++ {
		ranges := make([]domain.TimeRange, random.Intn(8))
		for i := range ranges {
			start := base.Add(time.Duration(random.Intn(14*24*60)) * time.Minute)
			end := start.Add(time.Duration(1+random.Intn(36*60)) * time.Minute)
			startZone := zones[random.Intn(len(zones))]
			endZone := zones[random.Intn(len(zones))]
			ranges[i] = domain.TimeRange{
				Start: domain.MustZonedInstant(start, startZone),
				End:   domain.MustZonedInstant(end, endZone),
			}
		}

		events := make([]domain.CalendarEvent, random.Intn(40))
		for i := range events {
			start := base.Add(time.Duration(random.Intn(16*24*60)-24*60) * time.Minute)
			end := start.Add(time.Duration(1+random.Intn(12*60)) * time.Minute)
			events[i] = domain.CalendarEvent{
				ID:       domain.CalendarEventID(fmt.Sprintf("event-%d", i)),
				Interval: rangeAt(start, end, zones[random.Intn(len(zones))]),
			}
		}
		random.Shuffle(len(events), func(i, j int) {
			events[i], events[j] = events[j], events[i]
		})

		got := subtractEvents(ranges, events)
		want := subtractEventsRepeatedSplitting(ranges, events)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("iteration %d:\ngot  %#v\nwant %#v", iteration, got, want)
		}
	}
}

func TestSubtractEventsPreservesCivilTimeSemanticsAcrossDST(t *testing.T) {
	t.Parallel()

	zone := "America/New_York"
	location, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 3, 7, 22, 0, 0, 0, location)
	end := time.Date(2026, 3, 8, 8, 0, 0, 0, location)
	events := []domain.CalendarEvent{
		{Interval: rangeAt(
			time.Date(2026, 3, 8, 1, 30, 0, 0, location),
			time.Date(2026, 3, 8, 3, 30, 0, 0, location),
			zone,
		)},
		{Interval: rangeAt(
			time.Date(2026, 3, 8, 5, 0, 0, 0, location),
			time.Date(2026, 3, 8, 6, 0, 0, 0, location),
			zone,
		)},
	}
	ranges := []domain.TimeRange{rangeAt(start, end, zone)}

	got := subtractEvents(ranges, events)
	want := subtractEventsRepeatedSplitting(ranges, events)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i, interval := range got {
		if interval.Start.ZoneID != zone || interval.End.ZoneID != zone {
			t.Fatalf("interval %d zones = %q to %q", i, interval.Start.ZoneID, interval.End.ZoneID)
		}
	}
}

func TestSubtractEventsHandlesLargeInput(t *testing.T) {
	t.Parallel()

	const eventCount = 10_000
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := spacedEvents(start, eventCount)
	ranges := []domain.TimeRange{
		rangeAt(start, start.Add(time.Duration(2*eventCount+1)*time.Minute), "UTC"),
	}

	got := subtractEvents(ranges, events)
	if len(got) != eventCount+1 {
		t.Fatalf("free intervals = %d, want %d", len(got), eventCount+1)
	}
	for i, interval := range got {
		wantStart := start.Add(time.Duration(2*i) * time.Minute)
		wantEnd := wantStart.Add(time.Minute)
		if !interval.Start.UTC.Equal(wantStart) || !interval.End.UTC.Equal(wantEnd) {
			t.Fatalf("interval %d = %s to %s, want %s to %s", i, interval.Start.UTC, interval.End.UTC, wantStart, wantEnd)
		}
	}
}

func subtractEventsRepeatedSplitting(ranges []domain.TimeRange, events []domain.CalendarEvent) []domain.TimeRange {
	result := append([]domain.TimeRange(nil), ranges...)
	for _, event := range events {
		var next []domain.TimeRange
		for _, interval := range result {
			if !interval.Overlaps(event.Interval) {
				next = append(next, interval)
				continue
			}
			if event.Interval.Start.UTC.After(interval.Start.UTC) {
				end, _ := domain.NewZonedInstant(event.Interval.Start.UTC, interval.Start.ZoneID)
				next = append(next, domain.TimeRange{Start: interval.Start, End: end})
			}
			if event.Interval.End.UTC.Before(interval.End.UTC) {
				start, _ := domain.NewZonedInstant(event.Interval.End.UTC, interval.Start.ZoneID)
				next = append(next, domain.TimeRange{Start: start, End: interval.End})
			}
		}
		result = next
	}
	return result
}

func spacedEvents(start time.Time, count int) []domain.CalendarEvent {
	events := make([]domain.CalendarEvent, count)
	for i := range events {
		eventStart := start.Add(time.Duration(2*i+1) * time.Minute)
		events[i] = domain.CalendarEvent{
			ID:       domain.CalendarEventID(fmt.Sprintf("event-%d", i)),
			Interval: rangeAt(eventStart, eventStart.Add(time.Minute), "UTC"),
		}
	}
	return events
}
