package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"non24.app/core/agentpolicy"
	calendarcore "non24.app/core/calendar"
	storage "non24.app/core/storage/sqlite"
	"non24.app/desktop/internal/localagent"
)

func TestLocalAgentProjectionsExcludeRawAndPrivateRecords(t *testing.T) {
	app := newTestApp(t)
	now := time.Now().UTC().Truncate(time.Minute)
	app.nowFn = func() time.Time { return now }
	seedSleepEntries(t, app, 12)

	const taskTitle = "Call Dr. Private about confidential paperwork"
	tasks, err := app.AddTask(TaskInput{Title: taskTitle, DurationMinutes: 45})
	if err != nil {
		t.Fatal(err)
	}
	taskID := tasks.Tasks[0].TaskID

	const (
		medicationLabel = "Private medication label"
		medicationForm  = "private tablet form"
		strengthLabel   = "private strength 5 mg"
		clinicianRule   = "private clinician instruction"
		medicationNote  = "private medication event note"
		markerNote      = "private marker note"
	)
	medications, err := app.AddMedication(MedicationInput{
		Label: medicationLabel, Form: medicationForm, StrengthLabel: strengthLabel,
	})
	if err != nil {
		t.Fatal(err)
	}
	medication := medications.Medications[0]
	medications, err = app.UpdateMedicationSchedule(MedicationScheduleInput{
		MedicationID: medication.MedicationID, Revision: medication.Revision,
		Kind: storage.MedicationScheduleFixedClock, ZoneID: "UTC",
		CivilTimes: []string{"08:00", "20:00"}, ClinicianRule: clinicianRule,
	})
	if err != nil {
		t.Fatal(err)
	}
	medication = medications.Medications[0]
	doseLocal := now.Add(-2 * time.Hour).Format("2006-01-02T15:04")
	if _, err := app.LogMedicationEvent(MedicationEventInput{
		MedicationID: medication.MedicationID, DoseLocal: doseLocal, ZoneID: "UTC",
		Status: storage.MedicationEventTaken, Scheduled: true, Note: medicationNote,
	}); err != nil {
		t.Fatal(err)
	}

	markerLocation := locationOrUTC("America/New_York")
	markerStart := now.Add(-24 * time.Hour).In(markerLocation)
	markers, err := app.AddRhythmMarker(RhythmMarkerInput{
		Kind:       storage.RhythmMarkerTravel,
		StartLocal: markerStart.Format("2006-01-02T15:04"),
		EndLocal:   markerStart.Add(2 * time.Hour).Format("2006-01-02T15:04"),
		ZoneID:     "America/New_York", Note: markerNote,
	})
	if err != nil {
		t.Fatal(err)
	}
	marker := markers.Markers[0]

	capability := desktopLocalCapability{app: app}
	taskJSON := callLocalAgentTool(t, capability, "list_tasks", `{}`)
	medicationJSON := callLocalAgentTool(t, capability, "get_medication_timing", `{}`)
	markerJSON := callLocalAgentTool(t, capability, "list_rhythm_markers", `{}`)

	if !strings.Contains(taskJSON, taskID) || strings.Contains(taskJSON, taskTitle) || strings.Contains(taskJSON, `"title"`) {
		t.Fatalf("task projection did not preserve ids while redacting titles: %s", taskJSON)
	}
	for _, private := range []string{
		medicationLabel, medicationForm, strengthLabel, clinicianRule, medicationNote,
		doseLocal, markerNote, marker.StartAt, marker.EndAt,
	} {
		if private != "" && (strings.Contains(medicationJSON, private) || strings.Contains(markerJSON, private)) {
			t.Fatalf("local agent projection leaked private/raw value %q", private)
		}
	}
	for _, forbiddenKey := range []string{`"events"`, `"dose_local"`, `"started_at"`, `"coverage_ends_at"`} {
		if strings.Contains(medicationJSON, forbiddenKey) {
			t.Fatalf("medication projection exposed raw field %s: %s", forbiddenKey, medicationJSON)
		}
	}
	for _, forbiddenKey := range []string{`"start_at"`, `"end_at"`} {
		if strings.Contains(markerJSON, forbiddenKey) {
			t.Fatalf("marker projection exposed raw field %s: %s", forbiddenKey, markerJSON)
		}
	}
	if !strings.Contains(medicationJSON, medication.MedicationID) ||
		!strings.Contains(medicationJSON, `"logged_event_summary"`) ||
		!strings.Contains(medicationJSON, `"civil_times"`) {
		t.Fatalf("medication projection omitted allowlisted timing facts: %s", medicationJSON)
	}
	if !strings.Contains(markerJSON, marker.MarkerID) ||
		!strings.Contains(markerJSON, `"civil_start_date":"`+marker.CivilDate+`"`) {
		t.Fatalf("marker projection omitted coarse marker facts: %s", markerJSON)
	}
}

func TestLocalAgentMedicalRefusalIsExactAndFactsRemainAvailable(t *testing.T) {
	app := newTestApp(t)
	capability := desktopLocalCapability{app: app}

	refusalJSON := callLocalAgentTool(t, capability, "ask_zeitboard_facts", `{"message":"When should I increase my melatonin dose?"}`)
	var refusal localAgentFactsResult
	if err := json.Unmarshal([]byte(refusalJSON), &refusal); err != nil {
		t.Fatal(err)
	}
	if refusal.Result != "refused_medical" || refusal.Answer != agentpolicy.MedicalRefusal || len(refusal.Facts) != 0 {
		t.Fatalf("medical refusal = %#v", refusal)
	}

	factsJSON := callLocalAgentTool(t, capability, "ask_zeitboard_facts", `{"message":"Show my medication timing facts"}`)
	var facts localAgentFactsResult
	if err := json.Unmarshal([]byte(factsJSON), &facts); err != nil {
		t.Fatal(err)
	}
	if facts.Result != "facts" || facts.Facts["medication_timing"] == nil || facts.Answer == agentpolicy.MedicalRefusal {
		t.Fatalf("permitted medication facts = %#v", facts)
	}

	markerJSON := callLocalAgentTool(t, capability, "ask_zeitboard_facts", `{"message":"List my light therapy markers"}`)
	var markerFacts localAgentFactsResult
	if err := json.Unmarshal([]byte(markerJSON), &markerFacts); err != nil {
		t.Fatal(err)
	}
	if markerFacts.Facts["rhythm_markers"] == nil || markerFacts.Facts["medication_timing"] != nil {
		t.Fatalf("marker-only question crossed the medication boundary: %#v", markerFacts)
	}
}

func TestMedicationLogSummaryExcludesSuppressedEvidence(t *testing.T) {
	summaries := medicationLogSummaries([]MedicationLogDTO{
		{
			MedicationID: "med_test", Status: storage.MedicationEventSkipped,
			Scheduled: true, Excluded: true, CorrectionCount: 1,
		},
		{
			MedicationID: "med_test", Status: storage.MedicationEventTaken,
			Scheduled: true, WakeRelation: "after wake", SleepRelation: "before sleep",
		},
	})
	summary := summaries["med_test"]
	if summary.TakenCount != 1 || summary.SkippedCount != 0 || summary.ScheduledCount != 1 {
		t.Fatalf("suppressed evidence changed included aggregates: %#v", summary)
	}
	if summary.ExcludedCount != 1 || summary.CorrectedEventCount != 1 {
		t.Fatalf("suppression audit counts were lost: %#v", summary)
	}
	if summary.Latest == nil || summary.Latest.Status != storage.MedicationEventTaken || summary.Latest.Excluded {
		t.Fatalf("suppressed newest record became the latest fact: %#v", summary.Latest)
	}
}

func TestAppearanceRevisionConflictPersistenceAndBackupRecovery(t *testing.T) {
	app := newTestApp(t)
	local := defaultLocalAppearanceState()
	local.Theme = "light"
	loaded, err := app.LoadLocalAppearanceState(local)
	if err != nil || loaded.Revision != 1 || loaded.State.Theme != "light" {
		t.Fatalf("initial appearance = %#v, %v", loaded, err)
	}
	if _, err := app.applyAppearanceTool(json.RawMessage(`{"theme":"amber"}`)); err != nil {
		t.Fatal(err)
	}
	stale := local
	stale.Theme = "dark"
	conflict, err := app.SaveLocalAppearanceState(LocalAppearanceSaveInput{State: stale, BaseRevision: 1})
	if err != nil || !conflict.Conflict || conflict.Revision != 2 || conflict.State.Theme != "amber" {
		t.Fatalf("stale appearance save = %#v, %v", conflict, err)
	}
	fresh, err := app.SaveLocalAppearanceState(LocalAppearanceSaveInput{State: stale, BaseRevision: 2})
	if err != nil || fresh.Conflict || fresh.Revision != 3 || fresh.State.Theme != "dark" {
		t.Fatalf("fresh appearance save = %#v, %v", fresh, err)
	}

	reloaded := newAppWithStore(app.store, nil)
	reloaded.configDir = app.configDir
	if err := reloaded.loadAppearanceFromDisk(); err != nil {
		t.Fatal(err)
	}
	if reloaded.appearanceRevision != 3 || reloaded.currentAppearance().Theme != "dark" {
		t.Fatalf("reloaded appearance = rev %d %#v", reloaded.appearanceRevision, reloaded.currentAppearance())
	}

	path := filepath.Join(app.configDir, appearanceFileName)
	if err := os.WriteFile(path, []byte("corrupt primary"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered := newAppWithStore(app.store, nil)
	recovered.configDir = app.configDir
	if err := recovered.loadAppearanceFromDisk(); err != nil {
		t.Fatal(err)
	}
	if recovered.appearanceRevision != 2 || recovered.currentAppearance().Theme != "amber" {
		t.Fatalf("backup recovery = rev %d %#v", recovered.appearanceRevision, recovered.currentAppearance())
	}
	state, revision, found, fromBackup, err := readAppearanceFile(path)
	if err != nil || !found || fromBackup || revision != 2 || state.Theme != "amber" {
		t.Fatalf("repaired primary = %#v rev=%d found=%v backup=%v err=%v", state, revision, found, fromBackup, err)
	}

	if err := os.WriteFile(path, []byte("corrupt primary again"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("corrupt backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	repairing := newAppWithStore(app.store, nil)
	repairing.configDir = app.configDir
	if err := repairing.loadAppearanceFromDisk(); err == nil {
		t.Fatal("two corrupt appearance files were accepted")
	} else {
		repairing.appearanceErr = err.Error()
	}
	repairState := defaultLocalAppearanceState()
	repairState.Theme = "contrast"
	repaired, err := repairing.LoadLocalAppearanceState(repairState)
	if err != nil || repaired.Revision != 1 || repaired.State.Theme != "contrast" {
		t.Fatalf("appearance self-repair = %#v err=%v", repaired, err)
	}
	if repairing.appearanceErr != "" {
		t.Fatalf("appearance error remained latched after repair: %q", repairing.appearanceErr)
	}
	if _, _, found, fromBackup, err := readAppearanceFile(path); err != nil || !found || fromBackup {
		t.Fatalf("self-repaired appearance file is not a valid primary: found=%v backup=%v err=%v", found, fromBackup, err)
	}
}

func TestLocalMCPWorksWithBackendOffAndChangesAppearance(t *testing.T) {
	app := newTestApp(t)
	app.startLocalAgent(context.Background())
	t.Cleanup(func() { app.stopLocalAgent(context.Background()) })
	status := app.GetLocalAgentStatus()
	if !status.Running || status.BackendProposalsAvailable {
		t.Fatalf("local agent status with backend off = %#v", status)
	}
	descriptor, err := localagent.LoadDescriptor(filepath.Join(app.configDir, localagent.DescriptorFileName))
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := localagent.NewBridge(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bridge.Close)

	response, emit, err := bridge.Forward(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"zeitboard-test","version":"1"}}}`))
	if err != nil || !emit || !strings.Contains(string(response), `"protocolVersion":"2025-11-25"`) {
		t.Fatalf("initialize = %s emit=%v err=%v", response, emit, err)
	}
	response, emit, err = bridge.Forward(context.Background(), []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if err != nil || emit || len(response) != 0 {
		t.Fatalf("initialized notification = %s emit=%v err=%v", response, emit, err)
	}
	response, emit, err = bridge.Forward(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_status","arguments":{}}}`))
	if err != nil || !emit || !strings.Contains(string(response), `"backend_proposals_available":false`) {
		t.Fatalf("get_status = %s emit=%v err=%v", response, emit, err)
	}
	response, emit, err = bridge.Forward(context.Background(), []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"set_appearance","arguments":{"theme":"black"}}}`))
	if err != nil || !emit || !strings.Contains(string(response), `"theme":"black"`) {
		t.Fatalf("set_appearance = %s emit=%v err=%v", response, emit, err)
	}
	if app.currentAppearance().Theme != "black" {
		t.Fatalf("appearance command did not reach app: %#v", app.currentAppearance())
	}
}

func TestLocalAgentProposalSendsOnlyRedactedPlanningContext(t *testing.T) {
	app := newTestApp(t)
	now := time.Now().UTC().Truncate(time.Minute)
	app.nowFn = func() time.Time { return now }
	seedSleepEntries(t, app, 12)

	const privateTaskTitle = "Private appointment paperwork for Dr. Example"
	tasks, err := app.AddTask(TaskInput{Title: privateTaskTitle, DurationMinutes: 45})
	if err != nil {
		t.Fatal(err)
	}
	taskID := tasks.Tasks[0].TaskID

	medications, err := app.AddMedication(MedicationInput{
		Label: "Private medication label", Form: "private form", StrengthLabel: "private strength",
	})
	if err != nil {
		t.Fatal(err)
	}
	medication := medications.Medications[0]
	if _, err := app.UpdateMedicationSchedule(MedicationScheduleInput{
		MedicationID: medication.MedicationID, Revision: medication.Revision,
		Kind: storage.MedicationScheduleFixedClock, ZoneID: defaultZoneID,
		CivilTimes: []string{"08:00"}, ClinicianRule: "private clinician rule",
	}); err != nil {
		t.Fatal(err)
	}
	markerStart := now.Add(-24 * time.Hour).In(locationOrUTC(defaultZoneID))
	if _, err := app.AddRhythmMarker(RhythmMarkerInput{
		Kind: storage.RhythmMarkerTravel, StartLocal: markerStart.Format("2006-01-02T15:04"),
		ZoneID: defaultZoneID, Note: "private marker note",
	}); err != nil {
		t.Fatal(err)
	}

	state, err := app.localEstimate(context.Background(), now)
	if err != nil || state.Status != "estimated" {
		t.Fatalf("local estimate for proposal fixture = %#v err=%v", state, err)
	}
	windows := localPlanningAvailability(state, now)
	start, end, ok := planningSnapshotRange(windows)
	if !ok || len(windows) == 0 {
		t.Fatal("proposal fixture has no planning snapshot")
	}
	eventStart := windows[0].Interval.Start.UTC.Add(30 * time.Minute)
	eventEnd := eventStart.Add(30 * time.Minute)
	if !eventEnd.Before(windows[0].Interval.End.UTC) {
		t.Fatal("proposal fixture window is too short for a fixed event")
	}
	const (
		privateSourceLabel = "Private work calendar"
		privateEventTitle  = "Confidential specialist appointment"
		privateRecordID    = "private-user@example.test/event-secret"
		privateLocation    = "Private clinic location"
		privateEventNotes  = "Private calendar notes"
	)
	source := calendarcore.Source{
		SourceID: "calendar_source_private_01", Label: privateSourceLabel,
		Kind: calendarcore.SourceICS, ReadOnly: true,
		CoverageStartAt: start.Add(-time.Hour), CoverageEndAt: end.Add(time.Hour), LastImportedAt: now,
	}
	event := calendarcore.Event{
		EventID: "calendar_event_private_01", SourceID: source.SourceID,
		SourceRecordID: privateRecordID, Title: privateEventTitle,
		StartAt: eventStart, EndAt: eventEnd, ZoneID: defaultZoneID,
		Busy: true, Ownership: calendarcore.OwnershipImported, CreatedAt: now,
		Location: privateLocation, Notes: privateEventNotes,
	}
	if err := app.store.ReplaceImportedCalendar(context.Background(), source, []calendarcore.Event{event}, ""); err != nil {
		t.Fatal(err)
	}

	var paths []string
	var proposalBody []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "proposal-token"})
		case "/v1/proposals":
			proposalBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"schema_version":"v1","backend":{"configured":true,"provider":"disabled"},"result":"proposal_pending","action":"propose_place_task","answer":"PRIVATE BACKEND ANSWER","proposals":[{"proposalId":"proposal_srv_01","status":"pending","decisionToken":"private-decision-token","payload":{"private":"backend-payload-secret"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureBackendForTest(t, app, server.URL)

	data, err := desktopLocalCapability{app: app}.CallTool(context.Background(), "propose_place_task", json.RawMessage(`{"target":{"task_id":"`+taskID+`"}}`))
	if err != nil {
		t.Fatal(err)
	}
	outbound := string(proposalBody)
	if !strings.Contains(outbound, taskID) || !strings.Contains(outbound, `"fixed_events"`) || !strings.Contains(outbound, `"event_id":"event_001"`) {
		t.Fatalf("proposal request omitted required redacted planning fields: %s", outbound)
	}
	for _, private := range []string{
		privateTaskTitle, privateSourceLabel, privateEventTitle, privateRecordID,
		privateLocation, privateEventNotes, "Private medication label", "private form",
		"private strength", "private clinician rule", "private marker note",
		`"medication_facts"`, `"markers"`, `"observation`,
	} {
		if strings.Contains(outbound, private) {
			t.Fatalf("proposal request leaked private or out-of-scope value %q: %s", private, outbound)
		}
	}

	result := string(data)
	if !strings.Contains(result, `"answer":"A pending schedule proposal was created for human review."`) ||
		!strings.Contains(result, `"proposal_id":"proposal_srv_01"`) ||
		!strings.Contains(result, `"status":"pending"`) {
		t.Fatalf("sanitized local proposal result = %s", result)
	}
	for _, backendPrivate := range []string{"PRIVATE BACKEND ANSWER", "private-decision-token", "backend-payload-secret"} {
		if strings.Contains(result, backendPrivate) {
			t.Fatalf("local proposal result leaked backend field %q: %s", backendPrivate, result)
		}
	}
	for _, path := range paths {
		if path != "/v1/devices" && path != "/v1/proposals" {
			t.Fatalf("local proposal touched unexpected endpoint %q", path)
		}
	}
}

func callLocalAgentTool(t *testing.T, capability desktopLocalCapability, name, arguments string) string {
	t.Helper()
	data, err := capability.CallTool(context.Background(), name, json.RawMessage(arguments))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
