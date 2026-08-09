package quicklog_test

import (
	"strings"
	"testing"
	"time"

	"non24.app/core/quicklog"
)

var now = time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)

func pendingAt(offset time.Duration) *quicklog.Pending {
	return &quicklog.Pending{StartedAt: now.Add(offset), ZoneID: "America/New_York"}
}

// TestAPairOfTapsRecordsANight is the point of the feature: two taps and the
// form is gone.
func TestAPairOfTapsRecordsANight(t *testing.T) {
	wake := quicklog.ResolveWake(quicklog.WakeInput{Now: now, Pending: pendingAt(-8 * time.Hour)})

	if wake.Outcome != quicklog.OutcomeRecord {
		t.Fatalf("outcome = %q, want record", wake.Outcome)
	}
	if !wake.Start.Equal(now.Add(-8*time.Hour)) || !wake.End.Equal(now) {
		t.Errorf("boundaries = %s..%s", wake.Start, wake.End)
	}
	if !strings.Contains(wake.Reason, "8 hours") {
		t.Errorf("reason %q does not say how much was recorded", wake.Reason)
	}
}

// TestNoPendingSleepIsNotGuessedAt. One tap gives one boundary. An estimator
// that needs both must not be handed an invention for the other.
func TestNoPendingSleepIsNotGuessedAt(t *testing.T) {
	wake := quicklog.ResolveWake(quicklog.WakeInput{Now: now})

	if wake.Outcome != quicklog.OutcomeConfirmOnset {
		t.Fatalf("outcome = %q, want confirm_onset", wake.Outcome)
	}
	if !wake.SuggestedStart.IsZero() {
		t.Error("a start was suggested with nothing to base it on")
	}
	if wake.SuggestionIsPrediction {
		t.Error("a non-existent suggestion was marked as a prediction")
	}
}

// TestAPredictedOnsetIsOfferedAndLabelled. The estimator's opinion is useful as
// a starting point and dangerous as a record, so it is offered as one and
// marked as the other.
func TestAPredictedOnsetIsOfferedAndLabelled(t *testing.T) {
	predicted := now.Add(-7 * time.Hour)
	wake := quicklog.ResolveWake(quicklog.WakeInput{Now: now, PredictedOnset: predicted})

	if wake.Outcome != quicklog.OutcomeConfirmOnset {
		t.Fatalf("outcome = %q", wake.Outcome)
	}
	if !wake.SuggestedStart.Equal(predicted) {
		t.Errorf("suggested start = %s, want the prediction", wake.SuggestedStart)
	}
	if !wake.SuggestionIsPrediction {
		t.Fatal("the prediction is not marked as one; a reader would take it for a record")
	}
	if !strings.Contains(wake.Reason, "prediction, not a record") {
		t.Errorf("reason %q does not say the prefill is a guess", wake.Reason)
	}
}

// A prediction in the future is not a plausible onset for a sleep that has
// already ended, so it is not offered at all.
func TestAFuturePredictionIsNotOffered(t *testing.T) {
	wake := quicklog.ResolveWake(quicklog.WakeInput{Now: now, PredictedOnset: now.Add(3 * time.Hour)})
	if !wake.SuggestedStart.IsZero() {
		t.Errorf("suggested a start in the future: %s", wake.SuggestedStart)
	}
}

// TestImplausiblePairsStopAndAsk. Each of these has a likely explanation, and
// each would damage something different if recorded silently.
func TestImplausiblePairsStopAndAsk(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		offset time.Duration
		want   quicklog.Outcome
		says   string
	}{
		// Under the estimator's own three-hour floor: a nap, or a mistap. Filed
		// as a principal sleep it would distort the fit; discarded it would
		// lose a nap.
		{"shorter than a night", -90 * time.Minute, quicklog.OutcomeConfirmShort, "nap"},
		// Past the fourteen-hour ceiling core/inference uses. Almost always a
		// missed wake tap rather than a very long night.
		{"longer than any episode", -16 * time.Hour, quicklog.OutcomeConfirmLong, "wake tap was missed"},
		// Old enough that "now" says nothing about when they woke.
		{"left running for a day", -30 * time.Hour, quicklog.OutcomeConfirmStale, "probably not when you woke"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			wake := quicklog.ResolveWake(quicklog.WakeInput{Now: now, Pending: pendingAt(testCase.offset)})
			if wake.Outcome != testCase.want {
				t.Fatalf("outcome = %q, want %q", wake.Outcome, testCase.want)
			}
			if !strings.Contains(wake.Reason, testCase.says) {
				t.Errorf("reason %q does not explain what probably happened", wake.Reason)
			}
			// The times are still carried, so the question can be pre-filled
			// rather than starting from a blank form.
			if wake.Start.IsZero() || wake.End.IsZero() {
				t.Error("the boundaries were dropped, so the question starts empty")
			}
			if wake.SuggestionIsPrediction {
				t.Error("a real marked time was labelled as a prediction")
			}
		})
	}
}

// TestTheBoundsAreTheEstimatorsOwn. If these drift from core/estimation and
// core/inference, this feature starts recording nights the estimator discards.
func TestTheBoundsAreTheEstimatorsOwn(t *testing.T) {
	if quicklog.MinimumEpisode != 3*time.Hour {
		t.Errorf("minimum = %s, want the estimator's three-hour floor", quicklog.MinimumEpisode)
	}
	if quicklog.MaximumEpisode != 14*time.Hour {
		t.Errorf("maximum = %s, want core/inference's fourteen-hour ceiling", quicklog.MaximumEpisode)
	}
	// The boundary values themselves must be accepted, not refused.
	for _, offset := range []time.Duration{-quicklog.MinimumEpisode, -quicklog.MaximumEpisode} {
		wake := quicklog.ResolveWake(quicklog.WakeInput{Now: now, Pending: pendingAt(offset)})
		if wake.Outcome != quicklog.OutcomeRecord {
			t.Errorf("a %s episode was refused as %q", -offset, wake.Outcome)
		}
	}
}

func TestAWakeBeforeItsOwnSleepIsRefused(t *testing.T) {
	wake := quicklog.ResolveWake(quicklog.WakeInput{Now: now, Pending: pendingAt(time.Hour)})
	if wake.Outcome != quicklog.OutcomeReject {
		t.Errorf("outcome = %q, want reject", wake.Outcome)
	}
}

// TestASecondSleepTapReplacesAndSaysSo. The newer tap is the current intent,
// but the older time is gone afterwards and the person should hear that.
func TestASecondSleepTapReplacesAndSaysSo(t *testing.T) {
	first := quicklog.BeginSleep(now, "America/New_York", nil)
	if first.Replaced != nil {
		t.Error("the first tap reported replacing something")
	}
	if !strings.Contains(first.Reason, "I woke up") {
		t.Errorf("reason %q does not say what to do next", first.Reason)
	}

	second := quicklog.BeginSleep(now.Add(20*time.Minute), "America/New_York", &first.Pending)
	if second.Replaced == nil {
		t.Fatal("replacing a pending sleep was not reported")
	}
	if !second.Replaced.StartedAt.Equal(now) {
		t.Errorf("replaced time = %s, want the first tap", second.Replaced.StartedAt)
	}
	if !strings.Contains(second.Reason, "replacing") {
		t.Errorf("reason %q does not mention the replacement", second.Reason)
	}
	if !second.Pending.StartedAt.Equal(now.Add(20 * time.Minute)) {
		t.Error("the pending sleep did not move to the newer tap")
	}
}

func TestEpisodeRefusesAnEmptyOrBackwardsInterval(t *testing.T) {
	if _, err := quicklog.Episode(now, now, "UTC"); err == nil {
		t.Error("a zero-length episode was accepted")
	}
	if _, err := quicklog.Episode(now, now.Add(-time.Hour), "UTC"); err == nil {
		t.Error("a backwards episode was accepted")
	}
	interval, err := quicklog.Episode(now.Add(-8*time.Hour), now, "America/New_York")
	if err != nil {
		t.Fatalf("a valid episode was refused: %v", err)
	}
	if interval.Start.ZoneID != "America/New_York" {
		t.Errorf("zone = %q", interval.Start.ZoneID)
	}
}
