package sqlite

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func testDoseProposal(id string, now time.Time) DoseProposal {
	return DoseProposal{
		ProposalID:   id,
		MedicationID: "med_local_01",
		Status:       MedicationEventTaken,
		DoseAt:       now.Add(-20 * time.Minute),
		ZoneID:       "America/New_York",
	}
}

// A proposed dose is no record at all until the owner records it; then it is
// the same dose a hand-logged one is, and it uploads like one.
func TestAProposedDoseBecomesARecordOnlyWhenTheOwnerRecordsIt(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	if err := store.CreateMedication(ctx, testMedicationRecord(now)); err != nil {
		t.Fatal(err)
	}
	proposal := testDoseProposal("dose_proposal_01", now)
	if err := store.ProposeDose(ctx, proposal, now); err != nil {
		t.Fatal(err)
	}
	if events, err := store.ListMedicationEvents(ctx); err != nil || len(events) != 0 {
		t.Fatalf("a proposal recorded a dose: %v %v", events, err)
	}
	listed, err := store.DoseProposals(ctx, now)
	if err != nil || len(listed) != 1 || listed[0].State != DoseProposalPending ||
		!listed[0].ExpiresAt.Equal(now.Add(DoseProposalLifetime)) || !listed[0].DoseAt.Equal(proposal.DoseAt) {
		t.Fatalf("waiting proposals = %+v, %v", listed, err)
	}

	recordedAt := now.Add(3 * time.Hour)
	record, err := store.RecordProposedDose(ctx, proposal.ProposalID, "dose_from_proposal", recordedAt)
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.ListMedicationEvents(ctx)
	if err != nil || len(events) != 1 {
		t.Fatalf("events = %v, %v", events, err)
	}
	event := events[0]
	if event.EventID != "dose_from_proposal" || event.MedicationID != proposal.MedicationID || event.Status != MedicationEventTaken ||
		!event.DoseAt.Equal(proposal.DoseAt) || event.ZoneID != proposal.ZoneID || event.Scheduled || event.Note != "" ||
		event.Provenance.AcquisitionMethod != ProvenanceAcquisitionManual ||
		event.Provenance.EvidenceStatus != ProvenanceEvidenceUserReported || !event.Provenance.RecordedAt.Equal(recordedAt) ||
		record.EventID != event.EventID {
		t.Fatalf("recorded dose = %+v", event)
	}
	listed, err = store.DoseProposals(ctx, recordedAt)
	if err != nil || listed[0].State != DoseProposalRecorded || listed[0].EventID != event.EventID || !listed[0].DecidedAt.Equal(recordedAt) {
		t.Fatalf("recorded proposal = %+v, %v", listed, err)
	}
	if _, err := store.RecordProposedDose(ctx, proposal.ProposalID, "dose_again", recordedAt); !errors.Is(err, ErrDoseProposalDecided) {
		t.Fatalf("recording twice: %v", err)
	}
	if err := store.DiscardProposedDose(ctx, proposal.ProposalID, recordedAt); !errors.Is(err, ErrDoseProposalDecided) {
		t.Fatalf("discarding a recorded dose: %v", err)
	}
	pending, err := store.PendingMedicationSyncRecords(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	uploads := false
	for _, record := range pending {
		uploads = uploads || (record.Kind == SyncKindMedicationEvent && record.RecordID == event.EventID)
	}
	if !uploads {
		t.Fatalf("the recorded dose does not upload: %+v", pending)
	}
}

func TestDoseProposalsAreDiscardedLapseAndStayBounded(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	if err := store.CreateMedication(ctx, testMedicationRecord(now)); err != nil {
		t.Fatal(err)
	}

	if err := store.ProposeDose(ctx, testDoseProposal("dose_proposal_discard", now), now); err != nil {
		t.Fatal(err)
	}
	if err := store.DiscardProposedDose(ctx, "dose_proposal_discard", now); err != nil {
		t.Fatal(err)
	}
	if err := store.ProposeDose(ctx, testDoseProposal("dose_proposal_lapse", now), now); err != nil {
		t.Fatal(err)
	}
	lapsed := now.Add(DoseProposalLifetime)
	if _, err := store.RecordProposedDose(ctx, "dose_proposal_lapse", "dose_too_late", lapsed); !errors.Is(err, ErrDoseProposalExpired) {
		t.Fatalf("recording a lapsed proposal: %v", err)
	}
	if _, err := store.RecordProposedDose(ctx, "dose_proposal_unknown", "dose_unknown", now); !errors.Is(err, ErrDoseProposalNotFound) {
		t.Fatalf("recording an unknown proposal: %v", err)
	}
	listed, err := store.DoseProposals(ctx, lapsed)
	if err != nil || len(listed) != 2 {
		t.Fatalf("proposals = %+v, %v", listed, err)
	}
	states := map[string]string{}
	for _, proposal := range listed {
		states[proposal.ProposalID] = proposal.State
	}
	if states["dose_proposal_discard"] != DoseProposalDiscarded || states["dose_proposal_lapse"] != DoseProposalExpired {
		t.Fatalf("states = %v", states)
	}
	if events, err := store.ListMedicationEvents(ctx); err != nil || len(events) != 0 {
		t.Fatalf("a discarded or lapsed proposal recorded a dose: %v %v", events, err)
	}

	unknown := testDoseProposal("dose_proposal_unknown_med", now)
	unknown.MedicationID = "med_not_here"
	if err := store.ProposeDose(ctx, unknown, now); !errors.Is(err, ErrMedicationNotFound) {
		t.Fatalf("unknown medication: %v", err)
	}
	for index := 0; index < MaxPendingDoseProposals; index++ {
		if err := store.ProposeDose(ctx, testDoseProposal(fmt.Sprintf("dose_proposal_many_%02d", index), lapsed), lapsed); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ProposeDose(ctx, testDoseProposal("dose_proposal_one_too_many", lapsed), lapsed); !errors.Is(err, ErrTooManyDoseProposals) {
		t.Fatalf("over the limit: %v", err)
	}
	// Once they lapse, agents may propose again, and a month on the history is
	// gone.
	month := lapsed.Add(doseProposalHistory + DoseProposalLifetime + time.Second)
	if err := store.ProposeDose(ctx, testDoseProposal("dose_proposal_next_month", month), month); err != nil {
		t.Fatal(err)
	}
	if listed, err := store.DoseProposals(ctx, month); err != nil || len(listed) != 1 || listed[0].ProposalID != "dose_proposal_next_month" {
		t.Fatalf("history was not pruned: %d proposals, %v", len(listed), err)
	}

	inactive := testMedicationRecord(now)
	inactive.MedicationID = "med_inactive"
	inactive.Active = false
	if err := store.CreateMedication(ctx, inactive); err != nil {
		t.Fatal(err)
	}
	stopped := testDoseProposal("dose_proposal_inactive", month)
	stopped.MedicationID = inactive.MedicationID
	if err := store.ProposeDose(ctx, stopped, month); !errors.Is(err, ErrMedicationNotActive) {
		t.Fatalf("inactive medication: %v", err)
	}
	malformed := testDoseProposal("dose_proposal_malformed", month)
	malformed.Status = "2 tablets"
	if err := store.ProposeDose(ctx, malformed, month); err == nil {
		t.Fatal("a malformed proposal was queued")
	}
}

func TestErasingAMedicationErasesItsDoseProposals(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	if err := store.CreateMedication(ctx, testMedicationRecord(now)); err != nil {
		t.Fatal(err)
	}
	if err := store.ProposeDose(ctx, testDoseProposal("dose_proposal_erased", now), now); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteMedication(ctx, "med_local_01"); err != nil {
		t.Fatal(err)
	}
	if listed, err := store.DoseProposals(ctx, now); err != nil || len(listed) != 0 {
		t.Fatalf("proposals outlived their medication: %+v, %v", listed, err)
	}
}
