package sqlite

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// medicationSyncHarness applies downloaded pages with an advancing cursor and
// reads back what this device would upload or erase next.
type medicationSyncHarness struct {
	t      *testing.T
	store  *Store
	ctx    context.Context
	cursor int64
}

func newMedicationSyncHarness(t *testing.T) *medicationSyncHarness {
	store, ctx := newSyncPullTestStore(t)
	return &medicationSyncHarness{t: t, store: store, ctx: ctx}
}

func (h *medicationSyncHarness) pull(records ...SyncPullRecord) SyncPullPageResult {
	h.t.Helper()
	h.cursor++
	result, err := h.store.ApplySyncPullPage(h.ctx, SyncPullPage{Cursor: h.cursor, Records: records})
	if err != nil {
		h.t.Fatal(err)
	}
	return result
}

func (h *medicationSyncHarness) pending() []MedicationSyncRecord {
	h.t.Helper()
	records, err := h.store.PendingMedicationSyncRecords(h.ctx, MaxMedicationSyncPageSize)
	if err != nil {
		h.t.Fatal(err)
	}
	return records
}

func (h *medicationSyncHarness) pendingIDs() []string {
	h.t.Helper()
	ids := []string{}
	for _, record := range h.pending() {
		ids = append(ids, record.RecordID)
	}
	return ids
}

func (h *medicationSyncHarness) pushAll() {
	h.t.Helper()
	if err := h.store.MarkMedicationSyncRecordsPushed(h.ctx, h.pending(), time.Now().UTC()); err != nil {
		h.t.Fatal(err)
	}
}

func (h *medicationSyncHarness) erasures() []string {
	h.t.Helper()
	ids, err := h.store.PendingSyncErasures(h.ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	sort.Strings(ids)
	return ids
}

func (h *medicationSyncHarness) must(err error) {
	h.t.Helper()
	if err != nil {
		h.t.Fatal(err)
	}
}

func (h *medicationSyncHarness) medication(id string) MedicationRecord {
	h.t.Helper()
	record, err := h.store.MedicationByID(h.ctx, id)
	if err != nil {
		h.t.Fatal(err)
	}
	return record
}

func testDoseCorrection(id, eventID, supersedes string, createdAt time.Time, changes MedicationEventCorrectionChanges) MedicationEventCorrectionRecord {
	return MedicationEventCorrectionRecord{
		CorrectionID: id, TargetEventID: eventID, SupersedesCorrectionID: supersedes,
		CreatedAt: createdAt, Reason: MedicationCorrectionUserEdit, Changes: changes,
	}
}

func TestMedicationSyncUploadsDefinitionsBeforeDosesAndCorrections(t *testing.T) {
	h := newMedicationSyncHarness(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	medication := testMedicationRecord(now)
	event := testMedicationEvent(now)
	h.must(h.store.CreateMedication(h.ctx, medication))
	h.must(h.store.AppendMedicationEvent(h.ctx, event))
	h.must(h.store.AppendMedicationEventCorrection(h.ctx, testDoseCorrection("medcorr_local_01", event.EventID, "",
		now.Add(time.Minute), MedicationEventCorrectionChanges{Status: stringPointer(MedicationEventSkipped)})))

	if got, want := h.pendingIDs(), []string{"med_local_01_r1", "dose_local_01", "medcorr_local_01"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("upload order = %v, want %v", got, want)
	}
	pending := h.pending()
	var uploaded MedicationRecord
	h.must(json.Unmarshal(pending[0].Payload, &uploaded))
	if pending[0].Kind != SyncKindMedication || uploaded.Label != medication.Label || !pending[0].CreatedAt.Equal(medication.UpdatedAt) {
		t.Fatalf("definition upload = %+v", pending[0])
	}
	if first, err := h.store.PendingMedicationSyncRecords(h.ctx, 1); err != nil || len(first) != 1 || first[0].RecordID != "med_local_01_r1" {
		t.Fatalf("one-record page = %v, %v", first, err)
	}

	h.pushAll()
	if count, err := h.store.PendingMedicationSyncRecordCount(h.ctx); err != nil || count != 0 {
		t.Fatalf("pending after acknowledgment = %d, %v", count, err)
	}
	edited := medication
	edited.Label = "Renamed private medication"
	edited.Revision, edited.UpdatedAt = 2, now.Add(time.Hour)
	h.must(h.store.UpdateMedication(h.ctx, edited, 1))
	if got := h.pendingIDs(); !reflect.DeepEqual(got, []string{"med_local_01_r2"}) {
		t.Fatalf("an edit uploads only its new revision, got %v", got)
	}
}

func TestDownloadedMedicationRevisionsKeepTheHighestAndRebaseUnsentEdits(t *testing.T) {
	h := newMedicationSyncHarness(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	remote := testMedicationRecord(now)
	if result := h.pull(SyncPullMedication{Medication: remote}); result.Applied != 1 {
		t.Fatalf("new definition not applied: %+v", result)
	}
	if got := h.pendingIDs(); len(got) != 0 {
		t.Fatalf("a downloaded definition was queued for upload: %v", got)
	}

	renamed := remote
	renamed.Label, renamed.Revision, renamed.UpdatedAt = "Renamed elsewhere", 2, now.Add(time.Hour)
	if result := h.pull(SyncPullMedication{Medication: renamed}); result.Applied != 1 || h.medication(remote.MedicationID).Label != "Renamed elsewhere" {
		t.Fatalf("newer revision not applied: %+v", result)
	}
	if result := h.pull(SyncPullMedication{Medication: remote}); result.Skipped != 1 || h.medication(remote.MedicationID).Revision != 2 {
		t.Fatalf("older revision replaced a newer one: %+v", result)
	}

	// This device renames it while another changes the schedule.
	local := h.medication(remote.MedicationID)
	local.Label, local.Revision, local.UpdatedAt = "Renamed here", 3, now.Add(2*time.Hour)
	h.must(h.store.UpdateMedication(h.ctx, local, 2))
	elsewhere := renamed
	elsewhere.Schedule = &MedicationSchedule{Kind: MedicationScheduleFixedClock, ZoneID: "America/New_York", CivilTimes: []string{"21:00"}, ReminderEnabled: true}
	elsewhere.Revision, elsewhere.UpdatedAt = 3, now.Add(3*time.Hour)
	if result := h.pull(SyncPullMedication{Medication: elsewhere}); result.Applied != 1 {
		t.Fatalf("conflicting revision not rebased: %+v", result)
	}
	rebased := h.medication(remote.MedicationID)
	if rebased.Revision != 4 || rebased.Label != "Renamed here" || rebased.Schedule == nil ||
		rebased.Schedule.Kind != MedicationScheduleFixedClock || !rebased.UpdatedAt.Equal(elsewhere.UpdatedAt) {
		t.Fatalf("rebased definition = %+v", rebased)
	}
	if got := h.pendingIDs(); !reflect.DeepEqual(got, []string{"med_local_01_r4"}) {
		t.Fatalf("rebased revision not queued: %v", got)
	}

	// Its upload coming back changes nothing.
	h.pushAll()
	if result := h.pull(SyncPullMedication{Medication: rebased}); result.Applied != 0 || len(h.pendingIDs()) != 0 {
		t.Fatalf("own upload was applied again: %+v", result)
	}
	// Nor does one whose acknowledgment was lost.
	unacknowledged := rebased
	unacknowledged.Active, unacknowledged.Revision, unacknowledged.UpdatedAt = false, 5, now.Add(4*time.Hour)
	h.must(h.store.UpdateMedication(h.ctx, unacknowledged, 4))
	if result := h.pull(SyncPullMedication{Medication: h.medication(remote.MedicationID)}); result.Applied != 0 || len(h.pendingIDs()) != 0 {
		t.Fatalf("own upload without acknowledgment: %+v, pending %v", result, h.pendingIDs())
	}
}

func TestRebaseMedicationKeepsEachDevicesChanges(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	encode := func(record MedicationRecord) []byte {
		data, err := json.Marshal(normalizeMedicationRecord(record))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	base := testMedicationRecord(now)
	edit := func(change func(*MedicationRecord), minutes int) MedicationRecord {
		record := base
		change(&record)
		record.Revision, record.UpdatedAt = 2, now.Add(time.Duration(minutes)*time.Minute)
		return record
	}
	rebase := func(base []byte, local, remote MedicationRecord) MedicationRecord {
		t.Helper()
		got, err := rebaseMedication(base, local, encode(local), remote, encode(remote))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	t.Run("changes to different fields both survive", func(t *testing.T) {
		local := edit(func(record *MedicationRecord) { record.Label = "Local label" }, 1)
		remote := edit(func(record *MedicationRecord) { record.Form = "capsule" }, 2)
		got := rebase(encode(base), local, remote)
		if got.Label != "Local label" || got.Form != "capsule" || got.Revision != 3 || !got.UpdatedAt.Equal(remote.UpdatedAt) {
			t.Fatalf("rebased = %+v", got)
		}
	})
	t.Run("the same field keeps the local edit", func(t *testing.T) {
		local := edit(func(record *MedicationRecord) { record.Label = "Local label" }, 2)
		remote := edit(func(record *MedicationRecord) { record.Label = "Remote label" }, 1)
		if got := rebase(encode(base), local, remote); got.Label != "Local label" || !got.UpdatedAt.Equal(local.UpdatedAt) {
			t.Fatalf("rebased = %+v", got)
		}
	})
	t.Run("a field cleared here stays cleared", func(t *testing.T) {
		local := edit(func(record *MedicationRecord) { record.StrengthLabel = "" }, 1)
		remote := edit(func(record *MedicationRecord) { record.Label = "Remote label" }, 2)
		if got := rebase(encode(base), local, remote); got.StrengthLabel != "" || got.Label != "Remote label" {
			t.Fatalf("rebased = %+v", got)
		}
	})
	t.Run("an invalid combination keeps the local edit whole", func(t *testing.T) {
		started := now.Add(-24 * time.Hour)
		withStart := base
		withStart.StartedAt, withStart.StartedZoneID = &started, "America/New_York"
		moved := started.Add(time.Hour)
		local := withStart
		local.StartedAt, local.Label, local.Revision, local.UpdatedAt = &moved, "Local label", 2, now.Add(time.Minute)
		remote := withStart
		remote.StartedAt, remote.StartedZoneID, remote.Revision, remote.UpdatedAt = nil, "", 2, now.Add(2*time.Minute)
		got := rebase(encode(withStart), local, remote)
		if got.StartedAt == nil || !got.StartedAt.Equal(moved) || got.StartedZoneID != "America/New_York" || got.Label != "Local label" || got.Revision != 3 {
			t.Fatalf("rebased = %+v", got)
		}
	})
	t.Run("an edit the other device already made adopts its revision", func(t *testing.T) {
		local := edit(func(record *MedicationRecord) { record.Label = "Same label" }, 1)
		remote := edit(func(record *MedicationRecord) { record.Label = "Same label" }, 2)
		if got := rebase(encode(base), local, remote); !reflect.DeepEqual(got, remote) {
			t.Fatalf("rebased = %+v, want the remote revision", got)
		}
	})
	t.Run("without a base the local fields win and remote-only fields join", func(t *testing.T) {
		local := edit(func(record *MedicationRecord) { record.Label = "Local label" }, 1)
		remote := edit(func(record *MedicationRecord) {
			record.Label, record.ClinicianRule = "Remote label", "Remote clinician rule"
		}, 2)
		if got := rebase(nil, local, remote); got.Label != "Local label" || got.ClinicianRule != "Remote clinician rule" {
			t.Fatalf("rebased = %+v", got)
		}
	})
}

func TestDownloadedDosesWaitForTheirDefinition(t *testing.T) {
	h := newMedicationSyncHarness(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	medication, event := testMedicationRecord(now), testMedicationEvent(now)
	correction := testDoseCorrection("medcorr_remote_01", event.EventID, "", now.Add(time.Minute),
		MedicationEventCorrectionChanges{Status: stringPointer(MedicationEventSkipped)})

	// After a server restore a phone can upload doses before the computer
	// uploads their definition.
	if result := h.pull(SyncPullMedicationCorrection{Correction: correction}, SyncPullMedicationEvent{Event: event}); result.Applied != 0 || result.Skipped != 2 {
		t.Fatalf("orphans applied: %+v", result)
	}
	if waiting, err := h.store.DeferredSyncRecordCount(h.ctx); err != nil || waiting != 2 {
		t.Fatalf("waiting = %d, %v", waiting, err)
	}
	if result := h.pull(SyncPullMedication{Medication: medication}); result.Applied != 3 {
		t.Fatalf("definition did not release its dose and correction: %+v", result)
	}
	if waiting, err := h.store.DeferredSyncRecordCount(h.ctx); err != nil || waiting != 0 {
		t.Fatalf("still waiting = %d, %v", waiting, err)
	}
	effective, err := h.store.EffectiveMedicationEvents(h.ctx)
	if err != nil || len(effective) != 1 || effective[0].Event.Status != MedicationEventSkipped {
		t.Fatalf("effective doses = %+v, %v", effective, err)
	}
	if got := h.pendingIDs(); len(got) != 0 {
		t.Fatalf("downloaded records were queued for upload: %v", got)
	}
}

func TestCorrectionsOfOneDoseFromTwoDevicesBothApply(t *testing.T) {
	h := newMedicationSyncHarness(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	event := testMedicationEvent(now)
	h.must(h.store.CreateMedication(h.ctx, testMedicationRecord(now)))
	h.must(h.store.AppendMedicationEvent(h.ctx, event))
	h.must(h.store.AppendMedicationEventCorrection(h.ctx, testDoseCorrection("medcorr_local_01", event.EventID, "",
		now.Add(2*time.Minute), MedicationEventCorrectionChanges{Note: stringPointer("Corrected here")})))

	elsewhere := testDoseCorrection("medcorr_remote_01", event.EventID, "", now.Add(time.Minute),
		MedicationEventCorrectionChanges{Status: stringPointer(MedicationEventSkipped)})
	if result := h.pull(SyncPullMedicationCorrection{Correction: elsewhere}); result.Applied != 1 {
		t.Fatalf("second first correction refused: %+v", result)
	}
	effective, err := h.store.EffectiveMedicationEvents(h.ctx)
	if err != nil || len(effective) != 1 {
		t.Fatalf("effective doses = %+v, %v", effective, err)
	}
	dose := effective[0]
	if dose.Event.Status != MedicationEventSkipped || dose.Event.Note != "Corrected here" || len(dose.Corrections) != 2 ||
		dose.Corrections[0].CorrectionID != elsewhere.CorrectionID {
		t.Fatalf("both corrections should apply in creation order: %+v", dose)
	}
	latest, err := h.store.LatestMedicationEventCorrectionID(h.ctx, event.EventID)
	if err != nil || latest != "medcorr_local_01" {
		t.Fatalf("latest = %q, %v", latest, err)
	}
	excluded := true
	h.must(h.store.AppendMedicationEventCorrection(h.ctx, testDoseCorrection("medcorr_local_02", event.EventID, latest,
		now.Add(3*time.Minute), MedicationEventCorrectionChanges{Excluded: &excluded})))
	if err := h.store.AppendMedicationEventCorrection(h.ctx, testDoseCorrection("medcorr_local_03", event.EventID, latest,
		now.Add(4*time.Minute), MedicationEventCorrectionChanges{Excluded: &excluded})); err == nil {
		t.Fatal("a local correction that does not follow the latest one was accepted")
	}
}

func TestDeletingAMedicationErasesItEverywhereAndRefusesItsLaterRecords(t *testing.T) {
	h := newMedicationSyncHarness(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	medication, event := testMedicationRecord(now), testMedicationEvent(now)
	h.must(h.store.CreateMedication(h.ctx, medication))
	h.must(h.store.AppendMedicationEvent(h.ctx, event))
	h.pushAll()
	h.must(h.store.AppendMedicationEventCorrection(h.ctx, testDoseCorrection("medcorr_local_01", event.EventID, "",
		now.Add(time.Minute), MedicationEventCorrectionChanges{Status: stringPointer(MedicationEventSkipped)})))
	edited := medication
	edited.Label, edited.Revision, edited.UpdatedAt = "Renamed private medication", 2, now.Add(time.Hour)
	h.must(h.store.UpdateMedication(h.ctx, edited, 1))
	inFlight := h.pending()[0]

	h.must(h.store.DeleteMedication(h.ctx, medication.MedicationID))
	want := []string{"dose_local_01", "med_local_01_r1", "med_local_01_r2", "medcorr_local_01"}
	if got := h.erasures(); !reflect.DeepEqual(got, want) {
		t.Fatalf("queued erasures = %v, want %v", got, want)
	}
	if got := h.pendingIDs(); len(got) != 0 {
		t.Fatalf("a deleted medication still uploads: %v", got)
	}
	// The upload that was in flight is acknowledged after the deletion: it
	// stays queued for erasure and leaves no copy behind.
	h.must(h.store.MarkMedicationSyncRecordsPushed(h.ctx, []MedicationSyncRecord{inFlight}, now))
	var copies int
	h.must(h.store.db.QueryRowContext(h.ctx, `SELECT COUNT(*) FROM local_medication_sync_records`).Scan(&copies))
	if copies != 0 || !reflect.DeepEqual(h.erasures(), want) {
		t.Fatalf("in-flight revision left %d copies, erasures %v", copies, h.erasures())
	}

	// A device that has not heard of the deletion yet cannot bring it back.
	later := edited
	later.Label, later.Revision, later.UpdatedAt = "Edited elsewhere", 3, now.Add(2*time.Hour)
	laterDose := testMedicationEvent(now)
	laterDose.EventID = "dose_remote_02"
	laterCorrection := testDoseCorrection("medcorr_remote_02", event.EventID, "", now.Add(time.Hour),
		MedicationEventCorrectionChanges{Status: stringPointer(MedicationEventTaken)})
	result := h.pull(SyncPullMedication{Medication: later}, SyncPullMedicationEvent{Event: laterDose},
		SyncPullMedicationCorrection{Correction: laterCorrection})
	medications, err := h.store.ListMedications(h.ctx)
	h.must(err)
	events, err := h.store.ListMedicationEvents(h.ctx)
	h.must(err)
	waiting, err := h.store.DeferredSyncRecordCount(h.ctx)
	h.must(err)
	if result.Applied != 0 || len(medications) != 0 || len(events) != 0 || waiting != 0 {
		t.Fatalf("deleted medication came back: %+v, %d definitions, %d doses, %d waiting", result, len(medications), len(events), waiting)
	}
}

func TestTombstonesFromAnotherDeviceEraseWithoutQueueingAgain(t *testing.T) {
	h := newMedicationSyncHarness(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	first, second := testMedicationRecord(now), testMedicationRecord(now)
	second.MedicationID = "med_local_02"
	firstDose, secondDose := testMedicationEvent(now), testMedicationEvent(now)
	secondDose.EventID, secondDose.MedicationID = "dose_local_02", second.MedicationID
	firstCorrection := testDoseCorrection("medcorr_local_01", firstDose.EventID, "", now.Add(time.Minute),
		MedicationEventCorrectionChanges{Status: stringPointer(MedicationEventSkipped)})
	secondCorrection := testDoseCorrection("medcorr_local_02", secondDose.EventID, "", now.Add(time.Minute),
		MedicationEventCorrectionChanges{Status: stringPointer(MedicationEventSkipped)})
	marker := testRhythmMarker(now)
	h.pull(SyncPullMedication{Medication: first}, SyncPullMedication{Medication: second},
		SyncPullMedicationEvent{Event: firstDose}, SyncPullMedicationEvent{Event: secondDose},
		SyncPullMedicationCorrection{Correction: firstCorrection}, SyncPullMedicationCorrection{Correction: secondCorrection},
		SyncPullMarker{Marker: marker})

	// Another device deleted the first medication, one dose correction of
	// the second, and the marker (a kindless tombstone, routed by what this
	// device holds).
	result := h.pull(
		SyncPullTombstone{RecordID: "med_local_01_r1", RecordKind: SyncKindMedication},
		SyncPullTombstone{RecordID: "dose_local_01", RecordKind: SyncKindMedicationEvent},
		SyncPullTombstone{RecordID: secondCorrection.CorrectionID, RecordKind: SyncKindMedicationCorrection},
		SyncPullTombstone{RecordID: marker.MarkerID},
	)
	if result.TombstonesApplied != 3 {
		t.Fatalf("tombstones applied = %+v", result)
	}
	medications, err := h.store.ListMedications(h.ctx)
	h.must(err)
	effective, err := h.store.EffectiveMedicationEvents(h.ctx)
	h.must(err)
	markers, err := h.store.ListRhythmMarkers(h.ctx)
	h.must(err)
	if len(medications) != 1 || medications[0].MedicationID != second.MedicationID || len(effective) != 1 ||
		effective[0].Event.EventID != secondDose.EventID || len(effective[0].Corrections) != 0 || len(markers) != 0 {
		t.Fatalf("after tombstones: %+v, %+v, %+v", medications, effective, markers)
	}
	if got := h.erasures(); len(got) != 0 {
		t.Fatalf("remote deletions were queued again: %v", got)
	}
	if result := h.pull(SyncPullMedication{Medication: first}, SyncPullMedicationCorrection{Correction: secondCorrection},
		SyncPullMarker{Marker: marker}); result.Applied != 0 {
		t.Fatalf("erased records came back: %+v", result)
	}
}

func TestMarkerSyncUploadsDownloadsAndErases(t *testing.T) {
	h := newMedicationSyncHarness(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	local := testRhythmMarker(now)
	h.must(h.store.CreateRhythmMarker(h.ctx, local))
	pending, err := h.store.PendingMarkerSyncRecords(h.ctx, MaxMarkerSyncPageSize)
	h.must(err)
	var uploaded RhythmMarkerRecord
	if len(pending) != 1 || json.Unmarshal(pending[0].Payload, &uploaded) != nil || uploaded.Note != local.Note {
		t.Fatalf("pending markers = %+v", pending)
	}
	h.must(h.store.MarkMarkerSyncRecordsPushed(h.ctx, pending, now))
	if count, err := h.store.PendingMarkerSyncRecordCount(h.ctx); err != nil || count != 0 {
		t.Fatalf("pending after acknowledgment = %d, %v", count, err)
	}

	remote := testRhythmMarker(now)
	remote.MarkerID, remote.Kind = "marker_remote_01", RhythmMarkerIllness
	if result := h.pull(SyncPullMarker{Marker: remote}); result.Applied != 1 {
		t.Fatalf("downloaded marker = %+v", result)
	}
	if result := h.pull(SyncPullMarker{Marker: remote}); result.Skipped != 1 {
		t.Fatalf("second copy = %+v", result)
	}
	if count, err := h.store.PendingMarkerSyncRecordCount(h.ctx); err != nil || count != 0 {
		t.Fatalf("a downloaded marker was queued for upload: %d, %v", count, err)
	}
	conflicting := remote
	conflicting.Note = "Different private note"
	h.cursor++
	if _, err := h.store.ApplySyncPullPage(h.ctx, SyncPullPage{Cursor: h.cursor, Records: []SyncPullRecord{SyncPullMarker{Marker: conflicting}}}); err == nil ||
		!strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("conflicting copy = %v", err)
	}

	h.must(h.store.DeleteRhythmMarker(h.ctx, remote.MarkerID))
	if got := h.erasures(); !reflect.DeepEqual(got, []string{remote.MarkerID}) {
		t.Fatalf("queued erasures = %v", got)
	}
	if result := h.pull(SyncPullMarker{Marker: remote}); result.Applied != 0 {
		t.Fatalf("deleted marker came back: %+v", result)
	}
}

func TestANewServerReceivesTheMedicationRecordsItDoesNotHold(t *testing.T) {
	h := newMedicationSyncHarness(t)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	event := testMedicationEvent(now)
	h.must(h.store.CreateMedication(h.ctx, testMedicationRecord(now)))
	h.must(h.store.AppendMedicationEvent(h.ctx, event))
	h.must(h.store.CreateRhythmMarker(h.ctx, testRhythmMarker(now)))
	h.pushAll()
	markers, err := h.store.PendingMarkerSyncRecords(h.ctx, MaxMarkerSyncPageSize)
	h.must(err)
	h.must(h.store.MarkMarkerSyncRecordsPushed(h.ctx, markers, now))

	h.must(h.store.SaveSyncEnrollment(h.ctx, SyncConnection{Enabled: true, BackendURL: "https://127.0.0.1:8765", DeviceID: "device_new"}, "synthetic-token"))
	h.cursor = 0
	h.pull(SyncPullMedicationEvent{Event: event}) // the new server holds only the dose
	h.must(h.store.FinishSyncReconciliation(h.ctx))
	if got := h.pendingIDs(); !reflect.DeepEqual(got, []string{"med_local_01_r1"}) {
		t.Fatalf("pending after reconciliation = %v", got)
	}
	if count, err := h.store.PendingMarkerSyncRecordCount(h.ctx); err != nil || count != 1 {
		t.Fatalf("marker pending after reconciliation = %d, %v", count, err)
	}
}
