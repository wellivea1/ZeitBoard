package sleepv1

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestReviewResolvesConcurrentEditsWithoutMutatingSource(t *testing.T) {
	start := time.Date(2026, 9, 1, 4, 0, 0, 123, time.UTC)
	observation := testObservation("obs_review", start, start.Add(8*time.Hour), ClassificationPrincipal)
	firstStart, secondStart := start.Add(10*time.Minute), start.Add(20*time.Minute)
	first := Correction{CorrectionID: "cor_first", TargetObservationID: observation.ObservationID, CreatedAt: start.Add(10 * time.Hour), Reason: CorrectionUserEdit, Changes: CorrectionChanges{StartAt: &firstStart}}
	second := first
	second.CorrectionID, second.CreatedAt, second.Changes.StartAt = "cor_second", start.Add(11*time.Hour), &secondStart
	if _, err := Fold([]Observation{observation}, []Correction{first, second}); !errors.Is(err, ErrCorrectionReviewRequired) {
		t.Fatalf("concurrent heads did not withhold estimation: %v", err)
	}
	review, err := Review(observation, []Correction{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if !review.NeedsReview || !reflect.DeepEqual(review.CorrectionIDs(), []string{"cor_first", "cor_second"}) || !review.Effective.Intervals[0].Interval.Start.UTC.Equal(start) {
		t.Fatal("review lost disputed alternatives or presented one as confirmed")
	}
	resolved := second
	resolved.CorrectionID = "cor_resolved"
	resolved.SupersedesCorrectionIDs = review.CorrectionIDs()
	resolved.BasedOnSourceRevision = &review.SourceRevision
	resolved.CreatedAt = start.Add(12 * time.Hour)
	sessions, err := Fold([]Observation{observation}, []Correction{resolved, first, second})
	if err != nil {
		t.Fatal(err)
	}
	if !sessions[0].Intervals[0].Interval.Start.UTC.Equal(secondStart) || !observation.StartAt.Equal(start) {
		t.Fatal("resolution lost the chosen endpoint or changed original evidence")
	}
	resolved.SupersedesCorrectionIDs = []string{first.CorrectionID}
	if _, err := Fold([]Observation{observation}, []Correction{resolved, first, second}); !errors.Is(err, ErrCorrectionReviewRequired) {
		t.Fatal("an unseen concurrent edit was silently discarded")
	}
}

func TestReviewRejectsCrossTargetAndCyclicParents(t *testing.T) {
	at := time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC)
	excluded := true
	one := Correction{CorrectionID: "cor_one", TargetObservationID: "obs_one", CreatedAt: at, Reason: CorrectionUserEdit, Changes: CorrectionChanges{Excluded: &excluded}, SupersedesCorrectionIDs: []string{"cor_two"}}
	two := one
	two.CorrectionID, two.SupersedesCorrectionIDs = "cor_two", []string{"cor_one"}
	if err := validateCorrectionGraph(map[string]Correction{one.CorrectionID: one, two.CorrectionID: two}); err == nil {
		t.Fatal("cycle accepted")
	}
	two.SupersedesCorrectionIDs, two.TargetObservationID = nil, "obs_other"
	if err := validateCorrectionGraph(map[string]Correction{one.CorrectionID: one, two.CorrectionID: two}); err == nil {
		t.Fatal("cross-target parent accepted")
	}
}

func TestOlderProviderRevisionCannotUndoNewerOriginal(t *testing.T) {
	start := time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC)
	observation := testObservation("hc_newer", start, start.Add(8*time.Hour), ClassificationPrincipal)
	observation.Provenance.AcquisitionMethod = AcquisitionHealthConnect
	olderStart := start.Add(time.Hour)
	provider := Correction{CorrectionID: "cor_older", TargetObservationID: observation.ObservationID, CreatedAt: start.Add(8 * time.Hour), Reason: CorrectionSourceConflict, AcquisitionMethod: AcquisitionHealthConnect, Changes: CorrectionChanges{StartAt: &olderStart}}
	review, err := Review(observation, []Correction{provider})
	if err != nil {
		t.Fatal(err)
	}
	if review.NeedsReview || !review.SourceRevision.Equal(observation.Provenance.RecordedAt) || !review.Source.Intervals[0].Interval.Start.UTC.Equal(start) {
		t.Fatal("old source replay replaced newer evidence")
	}
}
