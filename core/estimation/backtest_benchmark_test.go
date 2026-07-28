package estimation

import (
	"context"
	"fmt"
	"testing"
	"time"
)

var benchmarkBacktestReport BacktestReport

func BenchmarkBacktest(b *testing.B) {
	for _, historySize := range []int{100, 1_000, 10_000, 20_000} {
		b.Run(fmt.Sprintf("history_%d", historySize), func(b *testing.B) {
			sessions := syntheticSessions(
				historySize,
				time.Date(2023, 1, 1, 5, 0, 0, 0, time.UTC),
				24*time.Hour+45*time.Minute,
				8*time.Hour,
				"UTC",
			)
			estimator := RobustEstimator{}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				report, err := estimator.Backtest(context.Background(), sessions)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkBacktestReport = report
			}
		})
	}
}
