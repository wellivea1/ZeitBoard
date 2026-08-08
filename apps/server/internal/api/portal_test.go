package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"non24.app/core/recompute"
	"non24.app/server/internal/analysis"
	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
	"non24.app/server/internal/readmodel"
	"non24.app/server/internal/store"
)

var portalTestNow = time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)

// TestSharingRoutesAbsentWithoutPortal keeps the owner surface off too. A
// daemon with the portal disabled exposes no sharing endpoints at all.
func TestSharingRoutesAbsentWithoutPortal(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "desktop")

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/portal/profiles"},
		{http.MethodPost, "/v1/portal/profiles"},
		{http.MethodPost, "/v1/portal/profiles/abc/revoke"},
		{http.MethodPost, "/v1/portal/profiles/abc/erase"},
	}
	for _, route := range routes {
		status, body := h.request(t, route.method, route.path, token, "{}")
		if status != http.StatusNotFound {
			t.Errorf("%s %s status = %d body = %s, want 404 when the portal is disabled",
				route.method, route.path, status, body)
		}
	}
}

func newPortalHarness(t *testing.T) (*testHarness, *portal.Store) {
	t.Helper()
	portalStore, err := portal.Open(filepath.Join(t.TempDir(), "portal.db"), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("open portal store: %v", err)
	}
	t.Cleanup(func() { _ = portalStore.Close() })

	harness := newTestHarness(t, func(s *Server) {
		materializer := portalbridge.Materializer{
			Sleep:    readmodel.SleepReader{Store: s.store},
			Profiles: portalStore,
			Sink:     portalStore,
			Now:      func() time.Time { return portalTestNow },
		}
		// A real orchestrator on a fixed clock rather than a stub: publishing
		// goes through exactly one path in production, and a test that skipped
		// it would not exercise the one that ships. The worker's loop is not
		// started, so every run here is the synchronous one the handler makes.
		WithPortal(PortalConfig{
			Store:        portalStore,
			PublicOrigin: "https://share.example.test",
			Materializer: materializer,
			Requests: &portalbridge.RequestBridge{
				Portal:  portalStore,
				Private: s.store,
				Now:     func() time.Time { return portalTestNow },
			},
			Recompute: &analysis.Worker{
				Orchestrator: recompute.Orchestrator{
					Analysis: analysis.Portal{Materializer: materializer},
					Journal:  store.RecomputeJournal{Store: s.store},
				},
				Now: func() time.Time { return portalTestNow },
			},
		})(s)
	})
	return harness, portalStore
}

func TestSharingLifecycle(t *testing.T) {
	h, portalStore := newPortalHarness(t)
	token := h.registerDevice(t, "desktop")

	status, data := h.request(t, http.MethodPost, "/v1/portal/profiles", token,
		`{"label":"Mum","passcode":"long-enough-passcode","expiresInDays":30,"grants":{"wakingWindows":true,"allowRequests":false,"allowMessages":false}}`)
	if status != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", status, data)
	}
	var created createPortalProfileResponse
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	if !strings.HasPrefix(created.LinkURL, "https://share.example.test/p/") {
		t.Errorf("link url = %q, want the configured public origin", created.LinkURL)
	}
	for _, phrase := range []string{"2 hours", "cannot be revoked"} {
		if !strings.Contains(created.Disclosure, phrase) {
			t.Errorf("disclosure does not mention %q: %q", phrase, created.Disclosure)
		}
	}

	linkToken := strings.TrimPrefix(created.LinkURL, "https://share.example.test/p/")
	if _, err := portalStore.ResolveLink(t.Context(), linkToken, portalTestNow); err != nil {
		t.Fatalf("issued link does not resolve: %v", err)
	}

	status, data = h.request(t, http.MethodGet, "/v1/portal/profiles", token, "")
	if status != http.StatusOK {
		t.Fatalf("list status = %d body = %s", status, data)
	}
	var list portalProfileListResponse
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Profiles) != 1 {
		t.Fatalf("profile count = %d, want 1", len(list.Profiles))
	}
	if list.Profiles[0].Label != "Mum" {
		t.Errorf("label = %q, want Mum", list.Profiles[0].Label)
	}
	if list.Profiles[0].State != "active" {
		t.Errorf("state = %q, want active", list.Profiles[0].State)
	}

	status, data = h.request(t, http.MethodPost, "/v1/portal/profiles/"+created.ProfileID+"/revoke", token, "{}")
	if status != http.StatusOK {
		t.Fatalf("revoke status = %d body = %s", status, data)
	}
	if _, err := portalStore.ResolveLink(t.Context(), linkToken, portalTestNow); err == nil {
		t.Error("link still resolves after revocation")
	}

	status, data = h.request(t, http.MethodPost, "/v1/portal/profiles/"+created.ProfileID+"/erase", token, "{}")
	if status != http.StatusOK {
		t.Fatalf("erase status = %d body = %s", status, data)
	}
	label, err := h.st.PortalLabel(t.Context(), created.ProfileID)
	if err != nil {
		t.Fatalf("read label: %v", err)
	}
	if label != "" {
		t.Errorf("private label survived erasure: %q", label)
	}
}

// TestSharingLabelStaysOutOfPortalStore is the split-store invariant at the
// owner boundary: the recipient's name is the owner's data, not the portal's.
func TestSharingLabelStaysOutOfPortalStore(t *testing.T) {
	h, portalStore := newPortalHarness(t)
	token := h.registerDevice(t, "desktop")

	status, data := h.request(t, http.MethodPost, "/v1/portal/profiles", token,
		`{"label":"Dr Alvarez neurology","passcode":"long-enough-passcode","expiresInDays":7,"grants":{"wakingWindows":true,"allowRequests":false,"allowMessages":false}}`)
	if status != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", status, data)
	}
	var created createPortalProfileResponse
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatal(err)
	}

	profiles, err := portalStore.ListProfiles(t.Context(), portalTestNow)
	if err != nil {
		t.Fatalf("list portal profiles: %v", err)
	}
	encoded, err := json.Marshal(profiles)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("Alvarez")) {
		t.Error("the private share label reached the portal store")
	}
	if created.ProfileID == "" {
		t.Error("no profile id was returned")
	}
}

func TestSharingRejectsWeakPasscode(t *testing.T) {
	h, _ := newPortalHarness(t)
	token := h.registerDevice(t, "desktop")

	status, _ := h.request(t, http.MethodPost, "/v1/portal/profiles", token,
		`{"label":"Short","passcode":"abc","expiresInDays":30,"grants":{"wakingWindows":true,"allowRequests":false,"allowMessages":false}}`)
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a short passcode", status)
	}
}

func TestSharingRejectsOverlongLifetime(t *testing.T) {
	h, _ := newPortalHarness(t)
	token := h.registerDevice(t, "desktop")

	status, _ := h.request(t, http.MethodPost, "/v1/portal/profiles", token,
		`{"label":"Forever","passcode":"long-enough-passcode","expiresInDays":400,"grants":{"wakingWindows":true,"allowRequests":false,"allowMessages":false}}`)
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a lifetime past the 90-day cap", status)
	}
}

func TestSharingRequiresDeviceAuth(t *testing.T) {
	h, _ := newPortalHarness(t)
	status, _ := h.request(t, http.MethodGet, "/v1/portal/profiles", "", "")
	if status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 without a device token", status)
	}
}

// visitorRequestFixture creates a link that accepts requests, submits one
// directly through the portal store, and pumps it into the owner's queue.
func visitorRequestFixture(t *testing.T, h *testHarness, portalStore *portal.Store) visitorRequestDTO {
	t.Helper()
	ctx := t.Context()
	profileID, _ := portal.NewProfileID()
	linkToken, _ := portal.NewLinkToken()
	if err := portalStore.CreateProfile(ctx, portal.CreateProfileInput{
		ProfileID: profileID,
		Token:     linkToken,
		Passcode:  "long-enough-passcode",
		Grants:    portal.Grants{WakingWindows: true, AllowRequests: true},
		CreatedAt: portalTestNow,
		ExpiresAt: portalTestNow.Add(30 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	profile, err := portalStore.ResolveLink(ctx, linkToken, portalTestNow)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	session, err := portalStore.CreateSession(ctx, profile, portalTestNow)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if _, err := portalStore.CreateRequest(ctx, profile, session.Session, portal.RequestInput{
		WindowStart:     portalTestNow.Add(30 * time.Hour),
		WindowEnd:       portalTestNow.Add(34 * time.Hour),
		ZoneID:          "UTC",
		DurationMinutes: 60,
		Handle:          "Sam",
		Message:         "coffee?",
	}, false, portalTestNow); err != nil {
		t.Fatalf("create request: %v", err)
	}

	// The device must exist before the pump: a visitor has no device of their
	// own, so the proposal is filed against an enrolled one.
	token := h.registerDevice(t, "desktop")
	bridge := portalbridge.RequestBridge{Portal: portalStore, Private: h.st, Now: func() time.Time { return portalTestNow }}
	if err := bridge.Pump(ctx); err != nil {
		t.Fatalf("pump: %v", err)
	}

	status, data := h.request(t, http.MethodGet, "/v1/portal/requests", token, "")
	if status != http.StatusOK {
		t.Fatalf("list status = %d body = %s", status, data)
	}
	var listed visitorRequestListResponse
	if err := json.Unmarshal(data, &listed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listed.Requests) != 1 {
		t.Fatalf("visitor request count = %d, want 1", len(listed.Requests))
	}
	return listed.Requests[0]
}

func TestVisitorRequestListCarriesOwnerContextAndDisclosure(t *testing.T) {
	h, portalStore := newPortalHarness(t)
	entry := visitorRequestFixture(t, h, portalStore)

	if entry.Handle != "Sam" || entry.Message != "coffee?" {
		t.Errorf("the owner cannot see what was asked: %+v", entry)
	}
	if entry.DecisionToken == "" {
		t.Error("no one-use decision token was issued")
	}
	// The owner must be told what approving reveals before choosing.
	for _, phrase := range []string{"exact time", "never why"} {
		if !strings.Contains(entry.Disclosure, phrase) {
			t.Errorf("disclosure does not mention %q: %q", phrase, entry.Disclosure)
		}
	}
}

func TestVisitorRequestCannotBeDecidedByTheGenericRoute(t *testing.T) {
	h, portalStore := newPortalHarness(t)
	entry := visitorRequestFixture(t, h, portalStore)
	token := h.registerDevice(t, "desktop2")

	body := fmt.Sprintf(`{"decision":"approved","token":%q}`, entry.DecisionToken)
	status, data := h.request(t, http.MethodPost, "/v1/proposals/"+entry.ProposalID+"/decision", token, body)
	if status != http.StatusConflict {
		t.Fatalf("status = %d body = %s, want 409 so the slot rules cannot be skipped", status, data)
	}
}

func TestVisitorRequestApprovalRoundTrip(t *testing.T) {
	h, portalStore := newPortalHarness(t)
	entry := visitorRequestFixture(t, h, portalStore)
	token := h.registerDevice(t, "desktop2")

	outside := fmt.Sprintf(`{"decision":"approved","token":%q,"startAt":%q,"endAt":%q}`,
		entry.DecisionToken,
		portalTestNow.Add(40*time.Hour).Format(time.RFC3339),
		portalTestNow.Add(41*time.Hour).Format(time.RFC3339))
	if status, _ := h.request(t, http.MethodPost, "/v1/portal/requests/"+entry.ProposalID+"/decision", token, outside); status != http.StatusBadRequest {
		t.Errorf("a slot outside the window returned %d, want 400", status)
	}

	inside := fmt.Sprintf(`{"decision":"approved","token":%q,"startAt":%q,"endAt":%q}`,
		entry.DecisionToken,
		portalTestNow.Add(31*time.Hour).Format(time.RFC3339),
		portalTestNow.Add(32*time.Hour).Format(time.RFC3339))
	status, data := h.request(t, http.MethodPost, "/v1/portal/requests/"+entry.ProposalID+"/decision", token, inside)
	if status != http.StatusOK {
		t.Fatalf("approval status = %d body = %s", status, data)
	}

	// The decision must already have reached the visitor's view.
	ids, err := portalStore.ListRequestsForProfile(t.Context(), entry.ProfileID)
	if err != nil || len(ids) != 1 {
		t.Fatalf("list requests: %v", err)
	}
	request, err := portalStore.ReadRequest(t.Context(), entry.ProfileID, ids[0])
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if request.Status != portal.RequestApproved {
		t.Errorf("visitor-visible status = %q, want approved", request.Status)
	}
	if !request.DecidedStart.Equal(portalTestNow.Add(31 * time.Hour).UTC()) {
		t.Errorf("the visitor was told %v, not the chosen block", request.DecidedStart)
	}

	// The token is one-use, on this route too.
	if status, _ := h.request(t, http.MethodPost, "/v1/portal/requests/"+entry.ProposalID+"/decision", token, inside); status != http.StatusConflict {
		t.Errorf("replayed decision returned %d, want 409", status)
	}
}
