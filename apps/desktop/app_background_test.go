package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func waitBackground(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("background operation did not complete")
}

func TestDesktopBackgroundSyncRetriesWithoutAViewAndStops(t *testing.T) {
	app := newTestApp(t)
	seedOneSleepEntry(t, app)
	var requests, pushes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/v1/devices":
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_background", Token: "synthetic-token"})
		case "/v1/sync/push":
			if pushes.Add(1) == 1 {
				http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			var req syncPushRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
				return
			}
			_ = json.NewEncoder(w).Encode(syncPushResponse{SchemaVersion: "v1", Cursor: 1, Accepted: len(req.Records)})
		case "/v1/sync/pull":
			_ = json.NewEncoder(w).Encode(syncPullResponse{SchemaVersion: "v1", Cursor: 1, Records: []syncEnvelope{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	app.startDesktopBackground(context.Background(), 20*time.Millisecond)
	t.Cleanup(app.stopDesktopBackground)
	// No enrollment means no traffic, despite pending local records.
	time.Sleep(50 * time.Millisecond)
	if requests.Load() != 0 {
		t.Fatal("background sync contacted an unenrolled server")
	}
	configureBackendForTest(t, app, server.URL)
	waitBackground(t, func() bool {
		s, err := app.GetBackendSyncStatus()
		return err == nil && s.PendingPushCount == 0 && s.LastSyncLabel != "Not synced yet"
	})
	if pushes.Load() != 2 {
		t.Fatalf("retry did not retain then acknowledge outbox: %d", pushes.Load())
	}
	app.stopDesktopBackground()
	stopped := requests.Load()
	time.Sleep(50 * time.Millisecond)
	if requests.Load() != stopped {
		t.Fatal("requests continued after explicit quit")
	}
	app.startDesktopBackground(context.Background(), 20*time.Millisecond)
	waitBackground(t, func() bool { return requests.Load() > stopped })
	if pushes.Load() != 2 {
		t.Fatal("restart re-uploaded acknowledged records")
	}
}

func TestDisableBackendCancelsInflightSyncAndRejectsOldErrors(t *testing.T) {
	app := newTestApp(t)
	started, canceled := make(chan struct{}, 1), make(chan struct{}, 1)
	var pulls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_background", Token: "synthetic-token"})
		case "/v1/sync/pull":
			pulls.Add(1)
			select {
			case started <- struct{}{}:
			default:
			}
			<-r.Context().Done()
			select {
			case canceled <- struct{}{}:
			default:
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureBackendForTest(t, app, server.URL)
	previous, err := app.loadBackendSyncConfig()
	if err != nil {
		t.Fatal(err)
	}
	app.startDesktopBackground(context.Background(), 20*time.Millisecond)
	t.Cleanup(app.stopDesktopBackground)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("sync did not start")
	}
	status, err := app.DisableBackendSync()
	if err != nil || status.Enabled {
		t.Fatalf("disable: %#v %v", status, err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("disable did not cancel network request")
	}
	app.recordBackendSyncError(previous, errors.New("old projection failed"))
	status, err = app.GetBackendSyncStatus()
	if err != nil || status.Enabled || status.LastError != "" {
		t.Fatalf("late error revived old credentials: %#v %v", status, err)
	}
	time.Sleep(60 * time.Millisecond)
	if pulls.Load() != 1 {
		t.Fatal("background continued after disable")
	}
}

func TestBackendEnrollmentDoesNotForwardSecretsOnRedirect(t *testing.T) {
	app := newTestApp(t)
	var reached atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1) }))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	_, err := app.ConfigureBackendSync(BackendSyncInput{Enabled: true, BackendURL: redirect.URL, EnrollmentSecret: "synthetic-secret", InsecureSkipVerify: true})
	if err == nil || reached.Load() != 0 {
		t.Fatalf("forwarded enrollment to a redirect: requests=%d err=%v", reached.Load(), err)
	}
}
