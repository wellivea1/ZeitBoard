package medication

import (
	"testing"
	"time"

	"non24.app/core/sleepv1"
)

func dose(id string, at time.Time, status string) EventRecord {
	return EventRecord{
		EventID: id, MedicationID: "med_synthetic", DoseAt: at, ZoneID: "UTC", Status: status, Scheduled: true,
		Provenance: sleepv1.Provenance{
			AcquisitionMethod: "manual", EvidenceStatus: "user_reported", RecordedAt: at,
		},
	}
}

func correction(id, target string, at time.Time, changes CorrectionChanges) CorrectionRecord {
	return CorrectionRecord{CorrectionID: id, TargetEventID: target, CreatedAt: at, Reason: CorrectionUserEdit, Changes: changes}
}

// A dose reads as its corrections leave it, applied in the order they were
// made whatever order they arrive in, and the record itself is untouched.
func TestADoseReadsAsItsCorrectionsLeaveIt(t *testing.T) {
	base := time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC)
	skipped, taken := EventSkipped, EventTaken
	excluded := true
	later := base.Add(10 * time.Minute)
	events := []EventRecord{dose("dose-b", base.Add(24*time.Hour), EventTaken), dose("dose-a", base, EventSkipped)}
	corrections := []CorrectionRecord{
		// Arrives first but was made last: the dose ends up taken.
		correction("fix-2", "dose-a", base.Add(2*time.Hour), CorrectionChanges{Status: &taken}),
		correction("fix-1", "dose-a", base.Add(time.Hour), CorrectionChanges{Status: &skipped, DoseAt: &later}),
		correction("fix-3", "dose-b", base.Add(25*time.Hour), CorrectionChanges{Excluded: &excluded}),
	}

	effective, err := Effective(events, corrections)
	if err != nil {
		t.Fatal(err)
	}
	if len(effective) != 2 || effective[0].Event.EventID != "dose-a" || effective[1].Event.EventID != "dose-b" {
		t.Fatalf("not in dose order: %+v", effective)
	}
	first := effective[0]
	if first.Event.Status != EventTaken || !first.Event.DoseAt.Equal(later) || len(first.Corrections) != 2 ||
		first.Corrections[0].CorrectionID != "fix-1" {
		t.Errorf("dose-a reads %+v", first)
	}
	if !effective[1].Excluded {
		t.Error("an exclusion was not applied")
	}
	if events[1].Status != EventSkipped {
		t.Error("the record itself was rewritten")
	}
}

func TestAnEffectiveDoseMustStillBeValid(t *testing.T) {
	zone := "Mars/Olympus_Mons"
	events := []EventRecord{dose("dose-a", time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC), EventTaken)}
	corrections := []CorrectionRecord{
		correction("fix-1", "dose-a", time.Date(2026, 10, 9, 22, 0, 0, 0, time.UTC), CorrectionChanges{ZoneID: &zone}),
	}
	if _, err := Effective(events, corrections); err == nil {
		t.Fatal("a correction that leaves the dose invalid was accepted")
	}
}
