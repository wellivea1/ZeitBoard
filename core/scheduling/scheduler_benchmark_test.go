package scheduling

import (
	"fmt"
	"testing"
	"time"

	"non24.app/core/domain"
)

var subtractEventsBenchmarkResult []domain.TimeRange

func BenchmarkSubtractEvents(b *testing.B) {
	for _, eventCount := range []int{100, 1_000, 10_000} {
		b.Run(fmt.Sprintf("events_%d", eventCount), func(b *testing.B) {
			start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			ranges := []domain.TimeRange{
				rangeAt(start, start.Add(time.Duration(2*eventCount+1)*time.Minute), "UTC"),
			}
			events := spacedEvents(start, eventCount)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				subtractEventsBenchmarkResult = subtractEvents(ranges, events)
			}
		})
	}
}
