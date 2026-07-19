package history

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"non24.app/core/domain"
	"non24.app/core/estimation"
)

type BacktestSummary struct {
	Name                 string
	UncertaintyScale     float64
	Evaluations          int
	Refusals             int
	Coverage             float64
	MedianAbsErrorHours  float64
	MeanAbsErrorHours    float64
	P90AbsErrorHours     float64
	HitRate              float64
	MeanWindowWidthHours float64
	Calibration          []estimation.CalibrationBucket
	RefusalCounts        map[estimation.RefusalCode]int
}

func RunBacktestMatrix(ctx context.Context, sessions []domain.SleepSession) ([]BacktestSummary, error) {
	var summaries []BacktestSummary
	for _, candidate := range []struct {
		name  string
		scale float64
	}{
		{name: "baseline", scale: 1},
		{name: "tighten-75", scale: 0.75},
		{name: "tighten-50", scale: 0.5},
	} {
		config := estimation.DefaultConfig()
		config.UncertaintyScale = candidate.scale
		report, err := (estimation.RobustEstimator{Config: config}).Backtest(ctx, sessions)
		if err != nil {
			return nil, fmt.Errorf("%s backtest: %w", candidate.name, err)
		}
		attempts := report.Evaluations + report.Refusals
		coverage := 0.0
		if attempts > 0 {
			coverage = float64(report.Evaluations) / float64(attempts)
		}
		windowTotal := 0.0
		windowCount := 0
		for _, point := range report.Points {
			if point.WindowWidthHours <= 0 {
				continue
			}
			windowTotal += point.WindowWidthHours
			windowCount++
		}
		if windowCount != report.Evaluations {
			return nil, fmt.Errorf("%s backtest window accounting mismatch: evaluations=%d windows=%d", candidate.name, report.Evaluations, windowCount)
		}
		meanWidth := 0.0
		if windowCount > 0 {
			meanWidth = windowTotal / float64(windowCount)
		}
		refusalCounts := make(map[estimation.RefusalCode]int)
		for _, refusal := range report.RefusalPoints {
			refusalCounts[refusal.Code]++
		}
		refusalTotal := 0
		for _, count := range refusalCounts {
			refusalTotal += count
		}
		if refusalTotal != report.Refusals {
			return nil, fmt.Errorf("%s backtest refusal accounting mismatch: total=%d details=%d", candidate.name, report.Refusals, refusalTotal)
		}
		summaries = append(summaries, BacktestSummary{
			Name:                 candidate.name,
			UncertaintyScale:     candidate.scale,
			Evaluations:          report.Evaluations,
			Refusals:             report.Refusals,
			Coverage:             coverage,
			MedianAbsErrorHours:  report.MedianAbsErrorHours,
			MeanAbsErrorHours:    report.MeanAbsErrorHours,
			P90AbsErrorHours:     report.P90AbsErrorHours,
			HitRate:              report.HitRate,
			MeanWindowWidthHours: meanWidth,
			Calibration:          append([]estimation.CalibrationBucket(nil), report.Calibration...),
			RefusalCounts:        refusalCounts,
		})
	}
	return summaries, nil
}

func FormatBacktestMarkdown(summaries []BacktestSummary) string {
	var output strings.Builder
	output.WriteString("| Candidate | Scale | Evaluations | Refusals | Coverage | Median error | Mean error | P90 error | Hit rate | Mean window |\n")
	output.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, summary := range summaries {
		fmt.Fprintf(&output, "| %s | %.2f | %d | %d | %.3f | %.2f h | %.2f h | %.2f h | %.3f | %.2f h |\n",
			summary.Name, summary.UncertaintyScale, summary.Evaluations, summary.Refusals, summary.Coverage,
			summary.MedianAbsErrorHours, summary.MeanAbsErrorHours, summary.P90AbsErrorHours,
			summary.HitRate, summary.MeanWindowWidthHours)
	}
	output.WriteString("\n| Candidate | Confidence | Count | Hit rate | Median error |\n")
	output.WriteString("|---|---|---:|---:|---:|\n")
	for _, summary := range summaries {
		for _, bucket := range summary.Calibration {
			fmt.Fprintf(&output, "| %s | %s | %d | %.3f | %.2f h |\n",
				summary.Name, bucket.Level, bucket.Count, bucket.HitRate, bucket.MedianAbsErrorHours)
		}
	}
	output.WriteString("\n| Candidate | Refusal code | Count |\n")
	output.WriteString("|---|---|---:|\n")
	for _, summary := range summaries {
		codes := make([]string, 0, len(summary.RefusalCounts))
		for code := range summary.RefusalCounts {
			codes = append(codes, string(code))
		}
		sort.Strings(codes)
		if len(codes) == 0 {
			fmt.Fprintf(&output, "| %s | none | 0 |\n", summary.Name)
			continue
		}
		for _, code := range codes {
			fmt.Fprintf(&output, "| %s | %s | %d |\n", summary.Name, code, summary.RefusalCounts[estimation.RefusalCode(code)])
		}
	}
	return output.String()
}
