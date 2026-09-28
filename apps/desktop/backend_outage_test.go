package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Found reviewing finding #14: with the server's machine off, Home and Rhythm
// each waited out the request timeout before showing the local estimate, on
// every visit and every refresh.
func TestAnUnreachableServerIsNotWaitedOnAgain(t *testing.T) {
	app := newTestApp(t)
	var overviewRequests, rhythmRequests atomic.Int32
	var answering atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "outage-token"})
			return
		case "/v1/overview":
			overviewRequests.Add(1)
		case "/v1/rhythm":
			rhythmRequests.Add(1)
		default:
			http.NotFound(w, r)
			return
		}
		if !answering.Load() {
			// No answer at all, as from a machine that is off.
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		_ = json.NewEncoder(w).Encode(serverOverviewResponse{
			SchemaVersion: "v1", Status: "estimated", CurrentEstimatedState: "Likely awake from server",
			MedicationEvents: []MedicationEventDTO{}, Disclaimer: disclaimer,
		})
	}))
	defer server.Close()
	configureBackendForTest(t, app, server.URL)
	now := time.Now().UTC()
	app.nowFn = func() time.Time { return now }

	overview, err := app.GetOverview()
	if err != nil || overview.EstimateSource != "local" {
		t.Fatalf("an unreachable server: %+v, %v", overview, err)
	}
	asked := overviewRequests.Load()
	if asked == 0 {
		t.Fatal("the server was never asked")
	}
	// Known to be down: neither read waits on it again.
	if overview, err := app.GetOverview(); err != nil || overview.EstimateSource != "local" || overviewRequests.Load() != asked {
		t.Fatalf("asked again while down: %d requests, %+v, %v", overviewRequests.Load(), overview, err)
	}
	if rhythm, err := app.GetRhythm(); err != nil || rhythm.EstimateSource != "local" || rhythmRequests.Load() != 0 {
		t.Fatalf("rhythm asked while down: %d requests, %+v, %v", rhythmRequests.Load(), rhythm.EstimateSource, err)
	}

	// After the hold it asks again, and an answer ends the outage at once.
	now = now.Add(backendOutageHold + time.Second)
	answering.Store(true)
	if overview, err := app.GetOverview(); err != nil || overview.EstimateSource != "synced" {
		t.Fatalf("after the hold: %+v, %v", overview, err)
	}
	answering.Store(false)
	before := overviewRequests.Load()
	if overview, err := app.GetOverview(); err != nil || overview.EstimateSource != "local" || overviewRequests.Load() == before {
		t.Fatalf("an answer did not end the outage: %d requests, %+v, %v", overviewRequests.Load(), overview, err)
	}
}

func TestOnlyAnUnansweredRequestMarksAnOutage(t *testing.T) {
	var outage backendOutage
	now := time.Now()
	outage.note(errors.New("backend returned HTTP 500"), now)
	if outage.active(now) {
		t.Fatal("an error answer marked an outage")
	}
	outage.note(errBackendUnreachable, now)
	if !outage.active(now) || outage.active(now.Add(backendOutageHold)) {
		t.Fatal("an unanswered request should hold for exactly the outage hold")
	}
	outage.clear()
	if outage.active(now) {
		t.Fatal("clearing did not end the outage")
	}
}
