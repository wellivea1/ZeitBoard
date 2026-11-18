package sleepv1

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func providerObservation(id string, start time.Time) Observation {
	observation := testObservation(id, start, start.Add(8*time.Hour), ClassificationPrincipal)
	observation.Provenance.AcquisitionMethod = AcquisitionHealthConnect
	return observation
}

func presence(id string, observation Observation, at time.Time, removed bool) Correction {
	reason := CorrectionSourceRestored
	if removed {
		reason = CorrectionSourceDeleted
	}
	return Correction{
		CorrectionID: id, TargetObservationID: observation.ObservationID, CreatedAt: at,
		Reason: reason, AcquisitionMethod: AcquisitionHealthConnect, Changes: CorrectionChanges{Excluded: &removed},
	}
}

// A sleep the provider deleted is kept and left out (ADR-0072): the record is
// still there with its times, marked as removed at the source, and the
// estimator does not get it.
func TestARecordTheProviderRemovedIsKeptAndLeftOut(t *testing.T) {
	start := time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC)
	observation := providerObservation("hc_removed", start)
	removed := presence("cor_removed", observation, start.Add(30*time.Hour), true)

	sessions, err := Fold([]Observation{observation}, []Correction{removed})
	if err != nil {
		t.Fatal(err)
	}
	night := sessions[0]
	if !night.RemovedAtSource || !night.Suppressed || night.IsPrincipalSleep() {
		t.Fatalf("removed = %v, left out = %v", night.RemovedAtSource, night.Suppressed)
	}
	if !night.Intervals[0].Interval.Start.UTC.Equal(start) || len(night.Intervals[0].StartEvidence.CorrectionIDs) != 0 {
		t.Fatal("removing a record changed its times or its evidence")
	}

	// Back at the source, it counts again. The later word stands whatever
	// order the two arrive in.
	restored := presence("cor_restored", observation, start.Add(31*time.Hour), false)
	for _, order := range [][]Correction{{removed, restored}, {restored, removed}} {
		sessions, err = Fold([]Observation{observation}, order)
		if err != nil {
			t.Fatal(err)
		}
		if sessions[0].RemovedAtSource || sessions[0].Suppressed {
			t.Fatal("a record back at the source is still left out")
		}
	}
	again := presence("cor_removed_again", observation, start.Add(32*time.Hour), true)
	sessions, err = Fold([]Observation{observation}, []Correction{again, removed, restored})
	if err != nil {
		t.Fatal(err)
	}
	if !sessions[0].RemovedAtSource {
		t.Fatal("the newest removal did not stand")
	}
}

// The owner's own correction stands on a record the provider removed: the
// night keeps their times and counts, it is only marked. Removal is no
// revision of the times, so their correction needs no second review, which a
// real revision does.
func TestTheOwnersCorrectionStandsOnARemovedRecord(t *testing.T) {
	start := time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC)
	observation := providerObservation("hc_kept", start)
	owned := start.Add(25 * time.Minute)
	mine := Correction{CorrectionID: "cor_mine", TargetObservationID: observation.ObservationID, CreatedAt: start.Add(9 * time.Hour),
		Reason: CorrectionUserEdit, AcquisitionMethod: AcquisitionManual, Changes: CorrectionChanges{StartAt: &owned}}
	removed := presence("cor_removed", observation, start.Add(30*time.Hour), true)

	sessions, err := Fold([]Observation{observation}, []Correction{mine, removed})
	if err != nil {
		t.Fatalf("a removal asked for the owner's correction to be reviewed again: %v", err)
	}
	night := sessions[0]
	if !night.RemovedAtSource || night.Suppressed || !night.IsPrincipalSleep() {
		t.Fatalf("removed = %v, left out = %v", night.RemovedAtSource, night.Suppressed)
	}
	if !night.Intervals[0].Interval.Start.UTC.Equal(owned) {
		t.Fatal("the owner's time was lost")
	}
	review, err := Review(observation, []Correction{mine, removed})
	if err != nil || review.NeedsReview || !review.SourceRevision.Equal(observation.Provenance.RecordedAt) {
		t.Fatalf("review = %+v, %v", review.NeedsReview, err)
	}
	if !review.Source.RemovedAtSource || !review.Effective.RemovedAtSource || review.Effective.Suppressed {
		t.Fatal("the review does not show the record as removed and still counted")
	}

	// A correction that leaves the night out is the owner's word too.
	out := true
	mine.Changes = CorrectionChanges{Excluded: &out}
	sessions, err = Fold([]Observation{observation}, []Correction{mine, removed})
	if err != nil || !sessions[0].Suppressed {
		t.Fatalf("the owner left the night out and it counts: %v", err)
	}

	// And one made afterwards puts a removed night back.
	in := false
	back := Correction{CorrectionID: "cor_back", TargetObservationID: observation.ObservationID, CreatedAt: start.Add(40 * time.Hour),
		Reason: CorrectionUserEdit, AcquisitionMethod: AcquisitionManual, Changes: CorrectionChanges{Excluded: &in}}
	sessions, err = Fold([]Observation{observation}, []Correction{removed, back})
	if err != nil || sessions[0].Suppressed || !sessions[0].RemovedAtSource {
		t.Fatalf("a correction did not put the removed night back: %v", err)
	}
}

func TestOnlyAProviderSaysItsRecordIsGoneAndSaysNothingElse(t *testing.T) {
	start := time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC)
	observation := providerObservation("hc_rules", start)
	valid := presence("cor_rules", observation, start.Add(30*time.Hour), true)
	if err := ValidateCorrection(valid); err != nil {
		t.Fatal(err)
	}
	yes, no, later := true, false, start.Add(time.Hour)
	nap := ClassificationNap
	for name, change := range map[string]func(*Correction){
		"a person saying it":            func(c *Correction) { c.AcquisitionMethod = AcquisitionManual },
		"nobody saying it":              func(c *Correction) { c.AcquisitionMethod = "" },
		"removed but counted":           func(c *Correction) { c.Changes.Excluded = &no },
		"removed with nothing changed":  func(c *Correction) { c.Changes.Excluded = nil; c.Changes.StartAt = &later },
		"removed with new times":        func(c *Correction) { c.Changes.StartAt = &later },
		"removed and reclassified":      func(c *Correction) { c.Changes.SleepClassification = &nap },
		"restored but left out":         func(c *Correction) { c.Reason = CorrectionSourceRestored; c.Changes.Excluded = &yes },
		"a revision that leaves it out": func(c *Correction) { c.Reason = CorrectionSourceConflict },
		"a removal claiming a review":   func(c *Correction) { c.BasedOnSourceRevision = &later },
		"another reason":                func(c *Correction) { c.Reason = CorrectionDuplicate },
	} {
		record := valid
		change(&record)
		if err := ValidateCorrection(record); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// It applies to the provider's own records only.
	manual := testObservation("manual_night", start, start.Add(8*time.Hour), ClassificationPrincipal)
	manual.Provenance.AcquisitionMethod = AcquisitionManual
	if _, err := Fold([]Observation{manual}, []Correction{presence("cor_other", manual, start.Add(30*time.Hour), true)}); err == nil {
		t.Error("a provider removed a record it did not write")
	}
}

// A night recorded on another clock carries that clock (ADR-0073). The
// instants and the zone the owner reads in are unchanged, so the estimator
// gets the night like any other.
func TestANightAwayCarriesTheClockItWasRecordedOn(t *testing.T) {
	// 4 September 2026, New York is on -04:00; the phone was in Berlin.
	start := time.Date(2026, 9, 4, 21, 0, 0, 0, time.UTC)
	observation := providerObservation("hc_away", start)
	observation.SourceOffset = &SourceOffset{StartSeconds: 7200, EndSeconds: 7200}
	if err := ValidateObservation(observation); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeObservation(encoded)
	if err != nil || decoded.SourceOffset == nil || *decoded.SourceOffset != *observation.SourceOffset {
		t.Fatalf("round trip = %+v, %v", decoded.SourceOffset, err)
	}
	sessions, err := Fold([]Observation{observation}, nil)
	if err != nil {
		t.Fatal(err)
	}
	night := sessions[0]
	if night.SourceClock == nil || night.SourceClock.StartOffsetSeconds != 7200 || night.SourceClock.EndOffsetSeconds != 7200 {
		t.Fatalf("clock = %+v", night.SourceClock)
	}
	if !night.IsPrincipalSleep() || night.Intervals[0].Interval.Start.ZoneID != "America/New_York" || !night.Intervals[0].Interval.Start.UTC.Equal(start) {
		t.Fatal("a night away changed the instants, the zone, or whether it counts")
	}

	// A night at home says nothing, and cannot say the same thing twice.
	home := providerObservation("hc_home", start)
	if sessions, err = Fold([]Observation{home}, nil); err != nil || sessions[0].SourceClock != nil {
		t.Fatalf("a night at home has a clock of its own: %v", err)
	}
	home.SourceOffset = &SourceOffset{StartSeconds: -4 * 3600, EndSeconds: -4 * 3600}
	if err := ValidateObservation(home); err == nil {
		t.Error("the zone's own offset was accepted as a clock of its own")
	}
	// One end is enough: a flight during the night.
	home.SourceOffset = &SourceOffset{StartSeconds: -4 * 3600, EndSeconds: 3600}
	if err := ValidateObservation(home); err != nil {
		t.Errorf("a night that changed clocks was refused: %v", err)
	}
	home.SourceOffset = &SourceOffset{StartSeconds: 19 * 3600, EndSeconds: 3600}
	if err := ValidateObservation(home); err == nil {
		t.Error("an offset no clock stands on was accepted")
	}
	// An end left out is not UTC.
	for _, payload := range []string{
		strings.Replace(string(encoded), `"start_seconds":7200,`, ``, 1),
		strings.Replace(string(encoded), `"end_seconds":7200`, `"end_seconds":null`, 1),
		strings.Replace(string(encoded), `"end_seconds":7200`, `"end_seconds":7200,"zone":"Europe/Berlin"`, 1),
	} {
		if payload == string(encoded) {
			t.Fatal("the fixture did not change")
		}
		if _, err := DecodeObservation([]byte(payload)); err == nil {
			t.Errorf("accepted %s", payload)
		}
	}
}
