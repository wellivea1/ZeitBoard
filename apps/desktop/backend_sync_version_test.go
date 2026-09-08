package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSyncUnsupportedOrIncompleteAcknowledgmentRetainsPendingRecords(t *testing.T) {
	for _, body := range []string{`{"schema_version":"unsupported","cursor":1,"accepted":1}`, `{"schema_version":"v1","cursor":1}`, `{"schema_version":"v1","accepted":1}`, `{"schema_version":"v1","cursor":-1,"accepted":1}`, `{"schema_version":"v1","cursor":1,"accepted":1} {}`} {
		t.Run(body, func(t *testing.T) {
			app := newTestApp(t)
			seedOneSleepEntry(t, app)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/devices":
					_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "synthetic-version-token"})
				case "/v1/sync/push":
					_, _ = w.Write([]byte(body))
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
			if status.LastError == "" || status.PendingPushCount != 1 || status.PushedCount != 0 {
				t.Fatalf("invalid acknowledgment lost pending records: %#v", status)
			}
		})
	}
}

func TestSyncUnsupportedResponseCannotApplyRecords(t *testing.T) {
	app := newTestApp(t)
	observation := testSyncObservation("obs_version_rejected", time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "synthetic-version-token"})
		case "/v1/sync/pull":
			_ = json.NewEncoder(w).Encode(syncPullResponse{SchemaVersion: "unsupported", Cursor: 1, Records: []syncEnvelope{{Seq: 1, RecordID: observation.ObservationID, Kind: "observation", DeviceID: "device_other", CreatedAt: observation.Provenance.RecordedAt, Payload: mustJSON(t, observation)}}})
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
	if status.LastError == "" || status.Cursor != 0 || status.PulledCount != 0 {
		t.Fatal("unsupported pull version changed local state")
	}
}
