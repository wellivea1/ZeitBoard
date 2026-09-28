package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Found reviewing finding #14, then walking the app: with the server's machine
// off, Home, Rhythm, the decision queue and Sharing each waited out the request
// timeout before showing what they could, on every visit and every refresh.
func TestAServerThatIsOffIsNotWaitedOnAgain(t *testing.T) {
	app := newTestApp(t)
	var answering atomic.Bool
	var mu sync.Mutex
	requests := map[string]int{}
	asked := func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return requests[path]
	}
	total := func() int {
		mu.Lock()
		defer mu.Unlock()
		sum := 0
		for _, count := range requests {
			sum += count
		}
		return sum
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/devices" {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "outage-token"})
			return
		}
		mu.Lock()
		requests[r.URL.Path]++
		mu.Unlock()
		if !answering.Load() {
			// No answer at all, as from a machine that is off.
			if connection, _, err := w.(http.Hijacker).Hijack(); err == nil {
				_ = connection.Close()
			}
			return
		}
		if r.URL.Path != "/v1/overview" {
			http.NotFound(w, r)
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
	if err != nil || overview.EstimateSource != "local" || asked("/v1/overview") == 0 {
		t.Fatalf("an unanswered server: %+v, %v, asked %d times", overview, err, asked("/v1/overview"))
	}
	overviewRequests := asked("/v1/overview")

	// Known to be down, the reads that happen on their own do not wait on it.
	if overview, err := app.GetOverview(); err != nil || overview.EstimateSource != "local" || asked("/v1/overview") != overviewRequests {
		t.Fatalf("overview asked again while down: %+v, %v", overview.EstimateSource, err)
	}
	if rhythm, err := app.GetRhythm(); err != nil || rhythm.EstimateSource != "local" || asked("/v1/rhythm") != 0 {
		t.Fatalf("rhythm asked while down: %v, %v", rhythm.EstimateSource, err)
	}
	if proposals, err := app.GetBackendProposals(); err != nil || proposals.Status != "error" || asked("/v1/proposals") != 0 {
		t.Fatalf("the queue asked while down: %+v, %v", proposals.Status, err)
	}
	if requestsDTO, err := app.GetBackendVisitorRequests(); err != nil || requestsDTO.Status != "error" || asked("/v1/portal/requests") != 0 {
		t.Fatalf("visitor requests asked while down: %+v, %v", requestsDTO.Status, err)
	}
	if links, err := app.GetBackendShareLinks(); err != nil || links.Status != "error" || asked("/v1/portal/profiles") != 0 {
		t.Fatalf("share links asked while down: %+v, %v", links.Status, err)
	}
	// Sync keeps trying: it is how the server's return is noticed.
	before := total()
	if _, err := app.SyncNow(); err != nil {
		t.Fatal(err)
	}
	if total() == before {
		t.Fatal("sync did not try the server while it was down")
	}

	// Any answer ends the outage at once, even before the hold is out.
	answering.Store(true)
	if _, err := app.SyncNow(); err != nil {
		t.Fatal(err)
	}
	if overview, err := app.GetOverview(); err != nil || overview.EstimateSource != "synced" {
		t.Fatalf("after the server answered: %+v, %v", overview.EstimateSource, err)
	}

	// And after the hold, a read asks again without waiting for sync.
	answering.Store(false)
	if overview, _ := app.GetOverview(); overview.EstimateSource != "local" {
		t.Fatalf("down again: %v", overview.EstimateSource)
	}
	now = now.Add(backendOutageHold + time.Second)
	answering.Store(true)
	if overview, err := app.GetOverview(); err != nil || overview.EstimateSource != "synced" {
		t.Fatalf("after the hold: %+v, %v", overview.EstimateSource, err)
	}
}

func TestTheOutageHoldsForItsTimeAndEndsOnAnAnswer(t *testing.T) {
	var outage backendOutage
	now := time.Now()
	if outage.active(now) {
		t.Fatal("a new outage record is active")
	}
	outage.start(now)
	if !outage.active(now) || !outage.active(now.Add(backendOutageHold-time.Second)) || outage.active(now.Add(backendOutageHold)) {
		t.Fatal("an unanswered request should hold for exactly the outage hold")
	}
	outage.clear()
	if outage.active(now) {
		t.Fatal("an answer did not end the outage")
	}
}
