package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

func TestPullMalformedRecordDoesNotPartiallyApplyOrAdvanceCursor(t *testing.T) {
	app := newTestApp(t)
	start := time.Date(2026, 7, 28, 4, 0, 0, 0, time.UTC)
	observation := testSyncObservation("obs_pull_decode_01", start)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{
				SchemaVersion: "v1",
				DeviceID:      "device_desktop",
				Token:         "pull-page-token",
			})
		case "/v1/sync/pull":
			_ = json.NewEncoder(w).Encode(syncPullResponse{
				SchemaVersion: "v1",
				Cursor:        2,
				Records: []syncEnvelope{
					{
						Seq:       1,
						RecordID:  observation.ObservationID,
						Kind:      storage.SleepSyncKindObservation,
						DeviceID:  "phone_device",
						CreatedAt: observation.Provenance.RecordedAt,
						Payload:   mustJSON(t, observation),
					},
					{
						Seq:       2,
						RecordID:  "obs_pull_decode_bad",
						Kind:      storage.SleepSyncKindObservation,
						DeviceID:  "phone_device",
						CreatedAt: observation.Provenance.RecordedAt.Add(time.Minute),
						Payload:   json.RawMessage(`"not-an-observation"`),
					},
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
	if status.LastError == "" || status.PulledCount != 0 || status.Cursor != 0 {
		t.Fatalf("malformed page status = %#v", status)
	}
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	observations, err := store.ListSleepObservations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 0 {
		t.Fatalf("malformed page partially applied observations: %#v", observations)
	}
}
