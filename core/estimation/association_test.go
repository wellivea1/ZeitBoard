package estimation

import (
	"strings"
	"testing"
	"time"
)

func TestTemporalAssociationUsesRobustBeforeAndAfterSlopesWithoutCausality(t *testing.T) {
	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	before := syntheticSessions(8, start.Add(-8*(24*time.Hour+50*time.Minute)), 24*time.Hour+50*time.Minute, 8*time.Hour, "UTC")
	after := syntheticSessions(8, start, 24*time.Hour+10*time.Minute, 8*time.Hour, "UTC")
	result := DescribeTemporalAssociation(append(before, after...), start)
	if result.Status != TemporalAssociationAvailable {
		t.Fatalf("association = %#v", result)
	}
	if got := result.Before.Drift.Round(time.Minute); got != 50*time.Minute {
		t.Fatalf("before drift = %v", got)
	}
	if got := result.After.Drift.Round(time.Minute); got != 10*time.Minute {
		t.Fatalf("after drift = %v", got)
	}
	if result.Before.EpisodeCount != 8 || result.After.EpisodeCount != 8 {
		t.Fatalf("segment counts = %d, %d", result.Before.EpisodeCount, result.After.EpisodeCount)
	}
	if !strings.Contains(result.Message, "does not establish cause") {
		t.Fatalf("safety message = %q", result.Message)
	}
}

func TestTemporalAssociationRefusesSparseAndAmbiguousSegments(t *testing.T) {
	start := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	sparse := append(
		syntheticSessions(4, start.Add(-4*25*time.Hour), 25*time.Hour, 8*time.Hour, "UTC"),
		syntheticSessions(5, start, 25*time.Hour, 8*time.Hour, "UTC")...,
	)
	if result := DescribeTemporalAssociation(sparse, start); result.Status != TemporalAssociationInsufficient {
		t.Fatalf("sparse association = %#v", result)
	}
	ambiguousBefore := syntheticSessions(5, start.Add(-130*time.Hour), 25*time.Hour, 8*time.Hour, "UTC")
	for index := 2; index < len(ambiguousBefore); index++ {
		ambiguousBefore[index].Intervals[0].Interval.Start.UTC = ambiguousBefore[index].Intervals[0].Interval.Start.UTC.Add(14 * time.Hour)
		ambiguousBefore[index].Intervals[0].Interval.End.UTC = ambiguousBefore[index].Intervals[0].Interval.End.UTC.Add(14 * time.Hour)
	}
	ambiguous := append(ambiguousBefore, syntheticSessions(5, start, 25*time.Hour, 8*time.Hour, "UTC")...)
	if result := DescribeTemporalAssociation(ambiguous, start); result.Status != TemporalAssociationAmbiguousCycles {
		t.Fatalf("ambiguous association = %#v", result)
	}
}
