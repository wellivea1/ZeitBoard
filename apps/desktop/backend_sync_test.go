package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"non24.app/core/estimation"
	storage "non24.app/core/storage/sqlite"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBackendSyncOffMakesNoNetworkCalls(t *testing.T) {
	app := newTestApp(t)
	calls := 0
	previousClient := newBackendHTTPClient
	newBackendHTTPClient = func(bool) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, http.ErrServerClosed
			}),
		}
	}
	t.Cleanup(func() { newBackendHTTPClient = previousClient })

	overview, err := app.GetOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.EstimateSource != "local" || overview.Status != "empty" {
		t.Fatalf("sync-off overview should stay local empty, got %#v", overview)
	}
	status, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.Status != "off" {
		t.Fatalf("sync-off status = %#v", status)
	}
	if calls != 0 {
		t.Fatalf("sync off made %d network calls", calls)
	}
}

func TestConfigureBackendSyncStoresTokenOutsideConfig(t *testing.T) {
	const token = "device-token-secret"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices" || r.Method != http.MethodPost {
			t.Fatalf("unexpected enrollment request %s %s", r.Method, r.URL.Path)
		}
		var req registerDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.EnrollmentSecret != "enroll-secret" || req.Label != "desktop-test" {
			t.Fatalf("unexpected enrollment body: %#v", req)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(registerDeviceResponse{
			SchemaVersion: "v1",
			DeviceID:      "device_desktop",
			Token:         token,
		})
	}))
	defer server.Close()
	app := newTestApp(t)

	status, err := app.ConfigureBackendSync(BackendSyncInput{
		Enabled:            true,
		BackendURL:         server.URL,
		EnrollmentSecret:   "enroll-secret",
		DeviceLabel:        "desktop-test",
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Status != "connected" || status.DeviceID != "device_desktop" {
		t.Fatalf("unexpected configured status: %#v", status)
	}
	config, err := os.ReadFile(filepath.Join(app.configDir, backendSyncConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(config), token) {
		t.Fatal("backend token was written to sync config")
	}
	tokenData, err := os.ReadFile(filepath.Join(app.configDir, backendSyncTokenFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(tokenData)) != token {
		t.Fatal("backend token was not stored in the restricted token file")
	}
	if strings.Contains(status.LastError, token) {
		t.Fatal("backend token leaked into sync status")
	}
}

func TestConfigureBackendSyncWrongSecretFailsClosed(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid enrollment secret", http.StatusForbidden)
	}))
	defer server.Close()
	app := newTestApp(t)

	status, err := app.ConfigureBackendSync(BackendSyncInput{
		Enabled:            true,
		BackendURL:         server.URL,
		EnrollmentSecret:   "wrong-secret",
		InsecureSkipVerify: true,
	})
	if err == nil {
		t.Fatal("wrong enrollment secret should fail")
	}
	if status.Enabled || status.Status != "off" {
		t.Fatalf("failed enrollment should not enable sync: %#v", status)
	}
	if _, err := os.Stat(filepath.Join(app.configDir, backendSyncTokenFile)); !os.IsNotExist(err) {
		t.Fatalf("failed enrollment should not store token, stat err = %v", err)
	}
}

func TestConfigureBackendSyncRejectsInsecureRemoteTLS(t *testing.T) {
	app := newTestApp(t)

	_, err := app.ConfigureBackendSync(BackendSyncInput{
		Enabled:            true,
		BackendURL:         "https://example.com",
		EnrollmentSecret:   "enroll-secret",
		InsecureSkipVerify: true,
	})
	if err == nil || !strings.Contains(err.Error(), "localhost development") {
		t.Fatalf("expected localhost-only TLS bypass error, got %v", err)
	}
}

func TestSyncNowPushesLocalRecordsOnceAndUsesBearerToken(t *testing.T) {
	const token = "push-token-secret"
	app := newTestApp(t)
	seedOneSleepEntry(t, app)
	pushes := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: token})
		case "/v1/sync/push":
			pushes++
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Fatalf("missing bearer token: %q", r.Header.Get("Authorization"))
			}
			var req syncPushRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(req)
			if strings.Contains(string(encoded), token) {
				t.Fatal("token leaked into sync push body")
			}
			if len(req.Records) != 1 || req.Records[0].Kind != storage.SleepSyncKindObservation {
				t.Fatalf("unexpected pushed records: %#v", req.Records)
			}
			_ = json.NewEncoder(w).Encode(syncPushResponse{SchemaVersion: "v1", Cursor: int64(pushes), Accepted: len(req.Records)})
		case "/v1/sync/pull":
			_ = json.NewEncoder(w).Encode(syncPullResponse{SchemaVersion: "v1", Cursor: 1, Records: []syncEnvelope{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configureBackendForTest(t, app, server.URL)
	first, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "connected" || first.PushedCount != 1 || first.PendingPushCount != 0 {
		t.Fatalf("unexpected first sync status: %#v", first)
	}
	second, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "connected" || second.PushedCount != 0 || pushes != 1 {
		t.Fatalf("second sync should be idempotent, status=%#v pushes=%d", second, pushes)
	}
}

func TestSyncNowSurfacesPushConflictWithoutCrashing(t *testing.T) {
	app := newTestApp(t)
	seedOneSleepEntry(t, app)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "conflict-token"})
		case "/v1/sync/push":
			http.Error(w, "record id conflict", http.StatusConflict)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configureBackendForTest(t, app, server.URL)
	status, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "error" || !strings.Contains(status.LastError, "HTTP 409") || status.PendingPushCount != 1 {
		t.Fatalf("conflict should be surfaced as sync status error: %#v", status)
	}
}

func TestSyncNowPullsRemoteRecordsAndDedupesOwnRecords(t *testing.T) {
	app := newTestApp(t)
	start := time.Date(2026, 3, 10, 5, 0, 0, 0, time.UTC)
	remoteObservation := testSyncObservation("obs_sleep_10", start)
	ownObservation := testSyncObservation("obs_sleep_11", start.Add(25*time.Hour))
	remoteCorrection := storage.SleepCorrectionRecord{
		CorrectionID:        "corr_sleep_10",
		TargetObservationID: remoteObservation.ObservationID,
		CreatedAt:           start.Add(9 * time.Hour),
		Reason:              storage.CorrectionReasonUserEdit,
		Changes:             storage.SleepCorrectionChanges{EndAt: timePtr(start.Add(8*time.Hour + 20*time.Minute))},
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "pull-token"})
		case "/v1/sync/pull":
			_ = json.NewEncoder(w).Encode(syncPullResponse{
				SchemaVersion: "v1",
				Cursor:        10,
				Records: []syncEnvelope{
					{Seq: 1, RecordID: remoteCorrection.CorrectionID, Kind: storage.SleepSyncKindCorrection, DeviceID: "phone_device", CreatedAt: remoteCorrection.CreatedAt, Payload: mustJSON(t, remoteCorrection)},
					{Seq: 2, RecordID: remoteObservation.ObservationID, Kind: storage.SleepSyncKindObservation, DeviceID: "phone_device", CreatedAt: remoteObservation.Provenance.RecordedAt, Payload: mustJSON(t, remoteObservation)},
					{Seq: 3, RecordID: ownObservation.ObservationID, Kind: storage.SleepSyncKindObservation, DeviceID: "device_desktop", CreatedAt: ownObservation.Provenance.RecordedAt, Payload: mustJSON(t, ownObservation)},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configureBackendForTest(t, app, server.URL)
	first, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "connected" || first.PulledCount != 2 || first.Cursor != 10 {
		t.Fatalf("unexpected first pull status: %#v", first)
	}
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	observations, err := store.ListSleepObservations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	corrections, err := store.ListSleepCorrections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || observations[0].ObservationID != remoteObservation.ObservationID || len(corrections) != 1 {
		t.Fatalf("pulled records not inserted/deduped as expected: observations=%#v corrections=%#v", observations, corrections)
	}
	second, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if second.PulledCount != 0 {
		t.Fatalf("second pull should dedupe existing remote records: %#v", second)
	}
}

func TestPullSkipsOrphanCorrectionWithoutWedgingCursor(t *testing.T) {
	app := newTestApp(t)
	start := time.Date(2026, 3, 12, 4, 0, 0, 0, time.UTC)
	// A correction whose target observation is absent everywhere (e.g. the user
	// hard-erased the observation locally): a permanent orphan for this device.
	orphanCorrection := storage.SleepCorrectionRecord{
		CorrectionID:        "corr_orphan_01",
		TargetObservationID: "obs_erased_01",
		CreatedAt:           start.Add(10 * time.Hour),
		Reason:              storage.CorrectionReasonUserEdit,
		Changes:             storage.SleepCorrectionChanges{EndAt: timePtr(start.Add(8 * time.Hour))},
	}
	liveObservation := testSyncObservation("obs_live_01", start)
	pulls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "orphan-token"})
		case "/v1/sync/pull":
			pulls++
			if r.URL.Query().Get("since") != "0" && pulls > 1 {
				_ = json.NewEncoder(w).Encode(syncPullResponse{SchemaVersion: "v1", Cursor: 7, Records: []syncEnvelope{}})
				return
			}
			_ = json.NewEncoder(w).Encode(syncPullResponse{
				SchemaVersion: "v1",
				Cursor:        7,
				Records: []syncEnvelope{
					{Seq: 6, RecordID: liveObservation.ObservationID, Kind: storage.SleepSyncKindObservation, DeviceID: "phone_device", CreatedAt: liveObservation.Provenance.RecordedAt, Payload: mustJSON(t, liveObservation)},
					{Seq: 7, RecordID: orphanCorrection.CorrectionID, Kind: storage.SleepSyncKindCorrection, DeviceID: "phone_device", CreatedAt: orphanCorrection.CreatedAt, Payload: mustJSON(t, orphanCorrection)},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configureBackendForTest(t, app, server.URL)
	status, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "connected" || status.LastError != "" {
		t.Fatalf("orphan correction must not fail the sync: %#v", status)
	}
	if status.PulledCount != 1 || status.SkippedCount != 1 {
		t.Fatalf("expected 1 pulled + 1 skipped, got %#v", status)
	}
	if status.Cursor != 7 {
		t.Fatalf("cursor must advance past the orphan, got %d", status.Cursor)
	}
	// A second sync starts after the orphan and stays healthy (not wedged).
	second, err := app.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "connected" || second.SkippedCount != 0 || second.Cursor != 7 {
		t.Fatalf("second sync should be clean: %#v", second)
	}
}

func TestBackendProposalsListAndCrossDeviceDecision(t *testing.T) {
	app := newTestApp(t)
	payload := mustJSON(t, map[string]any{
		"proposal_id": "proposal_abc123def456",
		"action_id":   "propose_place_task",
		"schedule_proposals": map[string]any{
			"schema_version":    "v1",
			"request_id":        "proposal_abc123def456",
			"generated_at":      "2026-03-12T10:00:00Z",
			"algorithm_version": "assistant-scheduler-v1",
			"proposals": []map[string]any{{
				"proposal_id": "proposal_abc123def456",
				"task_id":     "taxes",
				"start_at":    "2026-03-12T15:00:00Z",
				"end_at":      "2026-03-12T16:30:00Z",
				"zone_id":     "America/New_York",
				"confidence":  map[string]any{"level": "medium", "reasons": []string{"scheduler"}},
				"explanation_codes": []string{
					"within_predicted_waking_window",
				},
			}},
			"unplaced": []any{},
		},
		"answer": "Queued for approval.",
	})
	var decisions []map[string]string
	decided := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "proposal-token"})
		case r.URL.Path == "/v1/proposals" && r.Method == http.MethodGet:
			record := map[string]any{
				"proposalId": "proposal_abc123def456",
				"actionId":   "propose_place_task",
				"deviceId":   "device_agent",
				"status":     "pending",
				"createdAt":  "2026-03-12T10:00:00Z",
				"updatedAt":  "2026-03-12T10:00:00Z",
				"expiresAt":  "2026-03-12T10:15:00Z",
				"payload":    json.RawMessage(payload),
			}
			if decided {
				record["status"] = "approved"
			} else {
				record["decisionToken"] = "one-use-token"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": "v1", "proposals": []any{record}})
		case strings.HasSuffix(r.URL.Path, "/decision") && r.Method == http.MethodPost:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			decisions = append(decisions, body)
			decided = true
			_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": "v1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configureBackendForTest(t, app, server.URL)
	list, err := app.GetBackendProposals()
	if err != nil {
		t.Fatal(err)
	}
	if list.Status != "ok" || len(list.Proposals) != 1 {
		t.Fatalf("unexpected proposal list: %#v", list)
	}
	proposal := list.Proposals[0]
	if proposal.Status != "pending" || proposal.DecisionToken != "one-use-token" {
		t.Fatalf("pending proposal should carry the decision token: %#v", proposal)
	}
	if !strings.Contains(proposal.Title, "taxes") || proposal.Confidence != "Medium" {
		t.Fatalf("proposal should be humanized from the payload: %#v", proposal)
	}
	if len(proposal.ReasonLabels) == 0 {
		t.Fatalf("proposal should carry reason labels: %#v", proposal)
	}

	refreshed, err := app.DecideBackendProposal(BackendProposalDecisionInput{
		ProposalID: proposal.ProposalID,
		Decision:   "approved",
		Token:      proposal.DecisionToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0]["decision"] != "approved" || decisions[0]["token"] != "one-use-token" {
		t.Fatalf("decision endpoint payload = %#v", decisions)
	}
	if refreshed.Status != "ok" || len(refreshed.Proposals) != 1 || refreshed.Proposals[0].Status != "approved" {
		t.Fatalf("refreshed list should show the decision: %#v", refreshed)
	}

	if _, err := app.DecideBackendProposal(BackendProposalDecisionInput{ProposalID: "x", Decision: "applied", Token: "t"}); err == nil {
		t.Fatal("invalid decision verbs must be rejected")
	}
}

func TestBackendProposalsOffWhenSyncDisabled(t *testing.T) {
	app := newTestApp(t)
	calls := 0
	previousClient := newBackendHTTPClient
	newBackendHTTPClient = func(bool) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, http.ErrServerClosed
			}),
		}
	}
	t.Cleanup(func() { newBackendHTTPClient = previousClient })

	list, err := app.GetBackendProposals()
	if err != nil {
		t.Fatal(err)
	}
	if list.Status != "off" || len(list.Proposals) != 0 || calls != 0 {
		t.Fatalf("sync-off proposals should be an offline no-op: %#v calls=%d", list, calls)
	}
}

func TestSyncedOverviewAndRhythmUseServerEstimateAndFallbackLocal(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "projection-token"})
		case "/v1/overview":
			_ = json.NewEncoder(w).Encode(serverOverviewResponse{
				SchemaVersion:            "v1",
				Status:                   "estimated",
				CurrentEstimatedState:    "Likely awake from server",
				TimeSinceWake:            "5 hours",
				PredictedNextSleepWindow: "Tonight",
				DriftEstimate:            "+45 minutes per observed sleep cycle",
				Confidence:               "medium",
				ConfidenceReasons:        []string{"server estimate"},
				NextUsefulTaskWindow:     "Later today",
				SharingStatus:            "Server projection only",
				MedicationEvents:         []MedicationEventDTO{},
				FixtureMode:              false,
				Disclaimer:               disclaimer,
			})
		case "/v1/rhythm":
			_ = json.NewEncoder(w).Encode(serverRhythmResponse{
				SchemaVersion: "v1",
				Status:        "estimated",
				Projection: serverRhythmProjection(
					"Synced server actogram",
				),
			})
		default:
			http.NotFound(w, r)
		}
	}))

	configureBackendForTest(t, app, server.URL)
	overview, err := app.GetOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.EstimateSource != "synced" || overview.CurrentEstimatedState != "Likely awake from server" {
		t.Fatalf("overview should come from server estimate: %#v", overview)
	}
	rhythm, err := app.GetRhythm()
	if err != nil {
		t.Fatal(err)
	}
	if rhythm.EstimateSource != "synced" || rhythm.ActogramSummary != "Synced server actogram" {
		t.Fatalf("rhythm should come from server estimate: %#v", rhythm)
	}

	server.Close()
	fallback, err := app.GetOverview()
	if err != nil {
		t.Fatal(err)
	}
	if fallback.EstimateSource != "local" || fallback.FixtureMode {
		t.Fatalf("unavailable backend should fall back to local estimate state: %#v", fallback)
	}
}

func configureBackendForTest(t *testing.T, app *App, backendURL string) BackendSyncStatusDTO {
	t.Helper()
	status, err := app.ConfigureBackendSync(BackendSyncInput{
		Enabled:            true,
		BackendURL:         backendURL,
		EnrollmentSecret:   "enroll-secret",
		DeviceLabel:        "desktop-test",
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "connected" {
		t.Fatalf("backend sync did not connect: %#v", status)
	}
	return status
}

func seedOneSleepEntry(t *testing.T, app *App) {
	t.Helper()
	location, _ := time.LoadLocation(defaultZoneID)
	start := time.Date(2026, 3, 7, 22, 0, 0, 0, location)
	if _, err := app.AddSleepEntry(SleepEntryInput{
		StartLocal:     start.Format("2006-01-02T15:04"),
		EndLocal:       start.Add(8 * time.Hour).Format("2006-01-02T15:04"),
		ZoneID:         defaultZoneID,
		Classification: storage.SleepClassificationPrincipal,
	}); err != nil {
		t.Fatal(err)
	}
}

func testSyncObservation(id string, start time.Time) storage.SleepObservationRecord {
	return storage.SleepObservationRecord{
		ObservationID: id,
		Kind:          storage.SleepKindEpisode,
		StartAt:       start,
		EndAt:         start.Add(8 * time.Hour),
		ZoneID:        defaultZoneID,
		Sleep:         storage.SleepObservationDetails{Classification: storage.SleepClassificationPrincipal},
		Provenance: storage.SleepObservationProvenance{
			AcquisitionMethod: storage.ProvenanceAcquisitionManual,
			EvidenceStatus:    storage.ProvenanceEvidenceUserReported,
			RecordedAt:        start.Add(8*time.Hour + 5*time.Minute),
			SourceRecordID:    "remote-manual",
		},
	}
}

func serverRhythmProjection(summary string) *estimation.RhythmProjection {
	return &estimation.RhythmProjection{
		FixtureMode:     false,
		Status:          "estimated",
		ActogramSummary: summary,
		ObservedRows:    []estimation.RhythmBand{},
		ForecastRows:    []estimation.RhythmBand{},
		Now:             estimation.RhythmNow{Label: "now", Day: "Mar 7", Hour: 12},
		DriftTitle:      "Sleep-onset drift",
		SlopeLabel:      "+45 min per cycle",
		DriftConfidence: "Low",
		DriftSummary:    "Server estimate.",
		YMinHour:        0,
		YMaxHour:        24,
		DriftPoints:     []estimation.RhythmDriftPoint{},
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func timePtr(value time.Time) *time.Time {
	return &value
}
