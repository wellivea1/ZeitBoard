package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func sleepReviewStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, context.Background()
}

func TestTheSleepLogIsReadAPageAtATimeNewestFirst(t *testing.T) {
	store, ctx := sleepReviewStore(t)
	first := time.Date(2026, 3, 1, 4, 0, 0, 0, time.UTC)
	for night := 0; night < 7; night++ {
		start := first.Add(time.Duration(night) * 25 * time.Hour)
		if err := store.AppendSleepObservation(ctx, testSleepObservation(fmt.Sprintf("obs_sleep_%02d", night), start, start.Add(8*time.Hour))); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(offset, limit int) (string, int) {
		t.Helper()
		reviews, total, err := store.ReadSleepReviewPage(ctx, offset, limit)
		if err != nil {
			t.Fatal(err)
		}
		listed := ""
		for _, review := range reviews {
			listed += review.ObservationID[len("obs_sleep_"):] + " "
		}
		return listed, total
	}
	if got, total := ids(0, 3); got != "06 05 04 " || total != 7 {
		t.Fatalf("first page = %q of %d", got, total)
	}
	if got, total := ids(6, 3); got != "00 " || total != 7 {
		t.Fatalf("last page = %q of %d", got, total)
	}
	if got, _ := ids(9, 3); got != "" {
		t.Fatalf("past the end = %q", got)
	}
	if _, _, err := store.ReadSleepReviewPage(ctx, 0, 0); err == nil {
		t.Fatal("an empty page size was accepted")
	}
}

// The Week board draws nights as corrected, so a range must find a night a
// correction moved into it and leave out one moved away.
func TestASleepRangeFollowsCorrections(t *testing.T) {
	store, ctx := sleepReviewStore(t)
	week := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	inside := week.Add(2*24*time.Hour + 4*time.Hour)
	before := week.Add(-10 * 24 * time.Hour)
	for id, start := range map[string]time.Time{
		"obs_sleep_inside":    inside,
		"obs_sleep_moved_in":  before,
		"obs_sleep_moved_out": inside.Add(24 * time.Hour),
		"obs_sleep_outside":   before.Add(24 * time.Hour),
	} {
		if err := store.AppendSleepObservation(ctx, testSleepObservation(id, start, start.Add(8*time.Hour))); err != nil {
			t.Fatal(err)
		}
	}
	move := func(id string, start time.Time) {
		t.Helper()
		end := start.Add(8 * time.Hour)
		if err := store.AppendSleepCorrection(ctx, SleepCorrectionRecord{
			CorrectionID: "corr_" + id, TargetObservationID: id, CreatedAt: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
			Reason: CorrectionReasonUserEdit, Changes: SleepCorrectionChanges{StartAt: &start, EndAt: &end},
		}); err != nil {
			t.Fatal(err)
		}
	}
	move("obs_sleep_moved_in", week.Add(4*24*time.Hour))
	move("obs_sleep_moved_out", week.Add(30*24*time.Hour))

	reviews, err := store.ReadSleepReviewsBetween(ctx, week, week.Add(7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, review := range reviews {
		found[review.ObservationID] = true
	}
	if len(reviews) != 2 || !found["obs_sleep_inside"] || !found["obs_sleep_moved_in"] {
		t.Fatalf("the week holds %v", found)
	}
	if _, err := store.ReadSleepReviewsBetween(ctx, week, week); err == nil {
		t.Fatal("an empty range was accepted")
	}
}
