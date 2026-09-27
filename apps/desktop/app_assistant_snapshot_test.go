package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	storage "non24.app/core/storage/sqlite"
)

// snapshotContract compiles contracts/v1/assistant-snapshot.schema.json with
// the common definitions it refers to.
func snapshotContract(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	for _, name := range []string{"common.schema.json", "assistant-snapshot.schema.json"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "v1", name))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource("mem:///contracts/v1/"+name, doc); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("mem:///contracts/v1/assistant-snapshot.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func requireSnapshotContract(t *testing.T, schema *jsonschema.Schema, encoded string) {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("snapshot does not match its contract: %v\n%s", err, encoded)
	}
}

// A synthetic day with every kind of record the snapshot coalesces: a rhythm
// estimate, an imported appointment, one task with an accepted time and one
// awaiting a decision, a medication with a schedule and a dose, and a
// context marker. Every private string is a canary.
func TestAssistantSnapshotCoalescesTheDayWithoutPrivateText(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntriesEndingAt(t, app, 10, 2*time.Hour)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)

	const (
		eventTitle      = "CANARY dentist appointment"
		eventLocation   = "CANARY clinic address"
		acceptedTitle   = "CANARY accepted task title"
		pendingTitle    = "CANARY pending task title"
		medicationLabel = "CANARY medication label"
		medicationForm  = "CANARY form"
		clinicianRule   = "CANARY clinician rule"
		doseNote        = "CANARY dose note"
		markerNote      = "CANARY marker note"
	)
	start := now.Add(5 * time.Hour).Truncate(time.Hour)
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//ZeitBoard Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:canary-uid@private.example\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:" + start.Format("20060102T150405Z") + "\r\n" +
		"DTEND:" + start.Add(30*time.Minute).Format("20060102T150405Z") + "\r\n" +
		"SUMMARY:" + eventTitle + "\r\nLOCATION:" + eventLocation + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if _, err := app.ImportCalendarFile(CalendarFileInput{FileName: "private.ics", Contents: ics, ZoneID: defaultZoneID}); err != nil {
		t.Fatal(err)
	}
	accepted, err := app.AddTask(TaskInput{Title: acceptedTitle, DurationMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	acceptedID := accepted.Tasks[0].TaskID
	proposals, err := app.GetProposals()
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals.Proposals) != 1 || proposals.Proposals[0].Decision != "pending" {
		t.Fatalf("expected one suggestion to accept: %+v", proposals)
	}
	if _, err := app.DecideLocalProposal(LocalProposalDecisionInput{ProposalID: proposals.Proposals[0].ID, Decision: storage.ProposalApproved}); err != nil {
		t.Fatal(err)
	}
	pending, err := app.AddTask(TaskInput{Title: pendingTitle, DurationMinutes: 45})
	if err != nil {
		t.Fatal(err)
	}
	pendingID := ""
	for _, task := range pending.Tasks {
		if task.TaskID != acceptedID {
			pendingID = task.TaskID
		}
	}
	medications, err := app.AddMedication(MedicationInput{Label: medicationLabel, Form: medicationForm})
	if err != nil {
		t.Fatal(err)
	}
	medication := medications.Medications[0]
	if _, err := app.UpdateMedicationSchedule(MedicationScheduleInput{MedicationID: medication.MedicationID, Revision: medication.Revision,
		Kind: storage.MedicationScheduleFixedClock, ZoneID: "UTC", CivilTimes: []string{"08:00"}, ClinicianRule: clinicianRule}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.LogMedicationEvent(MedicationEventInput{MedicationID: medication.MedicationID, DoseLocal: now.Add(-time.Hour).Format("2006-01-02T15:04"),
		ZoneID: "UTC", Status: storage.MedicationEventTaken, Scheduled: true, Note: doseNote}); err != nil {
		t.Fatal(err)
	}
	markers, err := app.AddRhythmMarker(RhythmMarkerInput{Kind: storage.RhythmMarkerTravel, StartLocal: now.Add(-30 * time.Hour).Format("2006-01-02T15:04"),
		EndLocal: now.Add(-26 * time.Hour).Format("2006-01-02T15:04"), ZoneID: "UTC", Note: markerNote})
	if err != nil {
		t.Fatal(err)
	}

	encoded := callLocalAgentTool(t, desktopLocalCapability{app: app}, "get_snapshot", `{}`)
	requireSnapshotContract(t, snapshotContract(t), encoded)
	for _, private := range []string{eventTitle, eventLocation, "canary-uid", acceptedTitle, pendingTitle,
		medicationLabel, medicationForm, clinicianRule, doseNote, markerNote} {
		if strings.Contains(encoded, private) {
			t.Fatalf("snapshot leaked %q: %s", private, encoded)
		}
	}
	var snapshot assistantSnapshot
	if err := json.Unmarshal([]byte(encoded), &snapshot); err != nil {
		t.Fatal(err)
	}

	// The rhythm is what Home says about now and the night ahead.
	overview, err := app.localOverview(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	computed, err := app.computeOutlook(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	onset, wake := computed.view.SleepAhead()
	rhythm := snapshot.Rhythm
	if rhythm.Status != "estimated" || rhythm.Presence != presenceAwake || overview.CurrentEstimatedState != "Likely awake" ||
		rhythm.AwakeForMinutes == nil || *rhythm.AwakeForMinutes != 120 || rhythm.FitRating == "" {
		t.Fatalf("rhythm = %+v, overview says %q", rhythm, overview.CurrentEstimatedState)
	}
	if onset == nil || rhythm.SleepOnset == nil || !rhythm.SleepOnset.Start.Equal(onset.Start.UTC.Truncate(time.Minute)) ||
		wake == nil || rhythm.Wake == nil || !rhythm.Wake.End.Equal(wake.End.UTC.Truncate(time.Minute)) {
		t.Fatalf("sleep onset %+v and wake %+v differ from the outlook's %v and %v", rhythm.SleepOnset, rhythm.Wake, onset, wake)
	}
	if sleep := snapshot.Sleep; sleep == nil || sleep.NightsOnRecord != 10 || sleep.Last == nil || sleep.Last.DurationMinutes != 480 || sleep.GoingToSleepMarked {
		t.Fatalf("sleep = %+v", snapshot.Sleep)
	}

	// The next three days: the appointment, the accepted time, the suggestion.
	plans := snapshot.Plans
	if plans.Status != "available" || len(plans.Segments) == 0 || len(plans.ReachableHours) == 0 {
		t.Fatalf("plans = %+v", plans)
	}
	var appointment, acceptedTime bool
	for _, commitment := range plans.Commitments {
		switch {
		case commitment.TaskID == acceptedID:
			acceptedTime = true
		case commitment.TaskID == "" && commitment.Start.Equal(start):
			appointment = true
		}
	}
	if !appointment || !acceptedTime {
		t.Fatalf("commitments = %+v", plans.Commitments)
	}
	current, err := app.GetProposals()
	if err != nil {
		t.Fatal(err)
	}
	pendingProposal := ""
	for _, proposal := range current.Proposals {
		if proposal.Decision == "pending" {
			pendingProposal = proposal.ID
		}
	}
	if len(plans.Suggestions) != 1 || plans.Suggestions[0].ProposalID != pendingProposal || plans.Suggestions[0].TaskID != pendingID ||
		snapshot.NeedsYou.Suggestions != 1 {
		t.Fatalf("suggestions = %+v, needs you = %+v, pending proposal %q", plans.Suggestions, snapshot.NeedsYou, pendingProposal)
	}
	if snapshot.Tasks == nil || snapshot.Tasks.Count != 2 || snapshot.Medication == nil || len(snapshot.Medication.Items) != 1 ||
		snapshot.Medication.Items[0].MedicationID != medication.MedicationID || snapshot.Markers == nil ||
		len(snapshot.Markers.Items) != 1 || snapshot.Markers.Items[0].MarkerID != markers.Markers[0].MarkerID ||
		snapshot.Sync == nil || snapshot.Sync.Enabled {
		t.Fatalf("tasks %+v, medication %+v, markers %+v, sync %+v", snapshot.Tasks, snapshot.Medication, snapshot.Markers, snapshot.Sync)
	}

	// The chat view carries no sleep record and no health context.
	chat, err := json.Marshal(snapshot.forChat())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"sleep"`, `"awake_for_minutes"`, `"tasks"`, `"medication"`, `"markers"`, `"sync"`} {
		if strings.Contains(string(chat), field) {
			t.Fatalf("chat view carries %s: %s", field, chat)
		}
	}
	requireSnapshotContract(t, snapshotContract(t), string(chat))
}

// With nothing recorded the snapshot still says so, in the same shape.
func TestAssistantSnapshotStatesAnEmptyRecord(t *testing.T) {
	app := newTestApp(t)
	encoded := callLocalAgentTool(t, desktopLocalCapability{app: app}, "get_snapshot", `{}`)
	requireSnapshotContract(t, snapshotContract(t), encoded)
	var snapshot assistantSnapshot
	if err := json.Unmarshal([]byte(encoded), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Rhythm.Status == "estimated" || snapshot.Rhythm.Presence != presenceUnknown || snapshot.Plans.Status != "refused" ||
		snapshot.Sleep == nil || snapshot.Sleep.NightsOnRecord != 0 || snapshot.Sleep.Last != nil || snapshot.NeedsYou != (snapshotNeedsYou{}) {
		t.Fatalf("empty snapshot = %s", encoded)
	}
}
