package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

// A stateful TLS peer exercises the desktop's real HTTP, durable store and
// enrollment lifecycle. It can roll history back without changing its URL.
type recoveryPeer struct {
	mu      sync.Mutex
	server  *httptest.Server
	records []syncEnvelope
	devices map[string]string
	seq     int64
	pushes  int
}

func newRecoveryPeer(t *testing.T) *recoveryPeer {
	t.Helper()
	p := &recoveryPeer{devices: map[string]string{}}
	p.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if r.URL.Path == "/v1/devices" {
			var req registerDeviceRequest
			if json.NewDecoder(r.Body).Decode(&req) != nil || req.EnrollmentSecret != "synthetic-enroll" {
				http.Error(w, "enrollment rejected", 403)
				return
			}
			id := fmt.Sprintf("device_%d", len(p.devices)+1)
			token := "synthetic-token-" + id
			p.devices[token] = id
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: id, Token: token})
			return
		}
		device := p.devices[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		if device == "" {
			http.Error(w, "not enrolled", 401)
			return
		}
		switch r.URL.Path {
		case "/v1/sync/pull":
			since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
			if since > p.seq {
				http.Error(w, "server restored", 409)
				return
			}
			page := syncPullResponse{SchemaVersion: "v1", Cursor: since, Records: []syncEnvelope{}}
			for _, record := range p.records {
				if record.Seq > since {
					page.Records = append(page.Records, record)
					page.Cursor = record.Seq
					if len(page.Records) == storage.MaxSyncPullPageSize {
						break
					}
				}
			}
			_ = json.NewEncoder(w).Encode(page)
		case "/v1/sync/push":
			var req syncPushRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			p.pushes++
			accepted := 0
			for _, record := range req.Records {
				found := false
				for _, existing := range p.records {
					if existing.RecordID == record.RecordID {
						found = true
						break
					}
				}
				if found {
					continue
				}
				p.seq++
				accepted++
				p.records = append(p.records, syncEnvelope{Seq: p.seq, RecordID: record.RecordID, Kind: record.Kind, DeviceID: device, CreatedAt: record.CreatedAt, Payload: record.Payload})
			}
			_ = json.NewEncoder(w).Encode(syncPushResponse{SchemaVersion: "v1", Cursor: p.seq, Accepted: accepted})
		case "/v1/sync/erase":
			var req syncEraseRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			for _, id := range req.RecordIDs {
				retained := p.records[:0]
				for _, record := range p.records {
					if record.RecordID != id {
						retained = append(retained, record)
					}
				}
				p.records = retained
				p.seq++
				payload, _ := json.Marshal(syncTombstonePayload{RecordID: id})
				p.records = append(p.records, syncEnvelope{Seq: p.seq, RecordID: id, Kind: "tombstone", DeviceID: device, CreatedAt: time.Now().UTC(), Payload: payload})
			}
			_ = json.NewEncoder(w).Encode(syncEraseResponse{SchemaVersion: "v1", Cursor: p.seq, Tombstones: len(req.RecordIDs)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)
	return p
}

func enrollRecoveryPeer(t *testing.T, a *App, p *recoveryPeer) {
	t.Helper()
	if _, err := a.ConfigureBackendSync(BackendSyncInput{Enabled: true, BackendURL: p.server.URL, EnrollmentSecret: "synthetic-enroll", InsecureSkipVerify: true}); err != nil {
		t.Fatal(err)
	}
}

func syncRecoveryPeer(t *testing.T, a *App) BackendSyncStatusDTO {
	t.Helper()
	status, err := a.SyncNow()
	if err != nil || status.LastError != "" {
		t.Fatalf("sync failed: %v; %s", err, status.LastError)
	}
	return status
}

func TestFailedServerSwitchRetainsEnrollmentAndSuccessfulSwitchReconciles(t *testing.T) {
	a := newTestApp(t)
	seedOneSleepEntry(t, a)
	first, second := newRecoveryPeer(t), newRecoveryPeer(t)
	enrollRecoveryPeer(t, a, first)
	if syncRecoveryPeer(t, a).PushedCount != 1 {
		t.Fatal("initial record not uploaded")
	}
	prior, priorToken, err := a.store.LoadSyncConnection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ConfigureBackendSync(BackendSyncInput{Enabled: true, BackendURL: second.server.URL, EnrollmentSecret: "incorrect", InsecureSkipVerify: true}); err == nil {
		t.Fatal("failed enrollment accepted")
	}
	current, currentToken, err := a.store.LoadSyncConnection(context.Background())
	if err != nil || current != prior || currentToken != priorToken {
		t.Fatal("failed switch destroyed previous enrollment")
	}
	if syncRecoveryPeer(t, a).PushedCount != 0 {
		t.Fatal("failed switch discarded acknowledgment")
	}
	enrollRecoveryPeer(t, a, second)
	if cursor, err := a.store.SleepSyncCursor(context.Background()); err != nil || cursor != 0 {
		t.Fatal("new server inherited previous cursor")
	}
	if syncRecoveryPeer(t, a).PushedCount != 1 {
		t.Fatal("new server did not receive retained local evidence")
	}
	enrollRecoveryPeer(t, a, first)
	if syncRecoveryPeer(t, a).PushedCount != 0 {
		t.Fatal("returning server unnecessarily received known records")
	}
}

func TestServerRestoreReconcilesMissingRecordsAndRetainsErasure(t *testing.T) {
	a := newTestApp(t)
	seedOneSleepEntry(t, a)
	peer := newRecoveryPeer(t)
	enrollRecoveryPeer(t, a, peer)
	syncRecoveryPeer(t, a)
	peer.mu.Lock()
	saved := append([]syncEnvelope(nil), peer.records...)
	peer.records = nil
	peer.seq = 0
	pushes := peer.pushes
	peer.mu.Unlock()
	status, err := a.SyncNow()
	if err != nil || !strings.Contains(status.LastError, "Re-enroll") {
		t.Fatal("server rollback was not surfaced")
	}
	peer.mu.Lock()
	unchanged := peer.pushes == pushes
	peer.mu.Unlock()
	if !unchanged {
		t.Fatal("desktop uploaded before learning the server was restored")
	}
	enrollRecoveryPeer(t, a, peer)
	if syncRecoveryPeer(t, a).PushedCount != 1 {
		t.Fatal("re-enrollment did not recover missing accepted upload")
	}
	if err := a.store.DeleteSleepObservation(context.Background(), saved[0].RecordID); err != nil {
		t.Fatal(err)
	}
	syncRecoveryPeer(t, a)
	// Restore the old server data after deletion was acknowledged. Durable
	// desktop suppression must survive the new enrollment and erase it again.
	peer.mu.Lock()
	peer.records = saved
	peer.seq = 1
	pushes = peer.pushes
	peer.mu.Unlock()
	enrollRecoveryPeer(t, a, peer)
	if got := syncRecoveryPeer(t, a); got.ErasuresPushed != 1 || got.PushedCount != 0 {
		t.Fatalf("restored erased record was not suppressed: %#v", got)
	}
	rows, err := a.store.ListSleepObservations(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("restore resurrected erased source")
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if peer.pushes != pushes || len(peer.records) != 1 || peer.records[0].Kind != "tombstone" {
		t.Fatal("remote restored payload survived erasure reconciliation")
	}
}

func TestPullDrainsMultiplePagesIncludingRecordsFromThisDevice(t *testing.T) {
	a := newTestApp(t)
	peer := newRecoveryPeer(t)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	peer.mu.Lock()
	for i := 0; i < 501; i++ {
		row := testSyncObservation(fmt.Sprintf("obs_paged_%04d", i), start.Add(time.Duration(i)*25*time.Hour))
		payload, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		peer.seq++
		peer.records = append(peer.records, syncEnvelope{Seq: peer.seq, RecordID: row.ObservationID, Kind: "observation", DeviceID: "device_1", CreatedAt: row.Provenance.RecordedAt, Payload: payload})
	}
	peer.mu.Unlock()
	enrollRecoveryPeer(t, a, peer)
	status := syncRecoveryPeer(t, a)
	if status.PulledCount != 501 || status.Cursor != 501 || status.PushedCount != 0 {
		t.Fatalf("download was incomplete or echoed uploads: %#v", status)
	}
}

func TestPullEnvelopeValidationRejectsIdentityAndCursorGaps(t *testing.T) {
	row := testSyncObservation("obs_valid_page", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	for name, change := range map[string]func(*syncPullResponse){
		"cursor gap":        func(p *syncPullResponse) { p.Cursor = 3 },
		"replayed sequence": func(p *syncPullResponse) { p.Records[0].Seq = 0 },
		"identity mismatch": func(p *syncPullResponse) { p.Records[0].RecordID = "obs_other" },
		"missing timestamp": func(p *syncPullResponse) { p.Records[0].CreatedAt = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			page := syncPullResponse{SchemaVersion: "v1", Cursor: 1, Records: []syncEnvelope{{Seq: 1, RecordID: row.ObservationID, Kind: "observation", DeviceID: "device_synthetic", CreatedAt: row.Provenance.RecordedAt, Payload: mustJSON(t, row)}}}
			change(&page)
			if err := validatePullEnvelopePage(page, 0); err == nil {
				t.Fatal("invalid envelope accepted")
			}
		})
	}
}
