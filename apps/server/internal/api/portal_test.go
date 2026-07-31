package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
	"non24.app/server/internal/readmodel"
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
		WithPortal(portalStore, "https://share.example.test", portalbridge.Materializer{
			Sleep:    readmodel.SleepReader{Store: s.store},
			Profiles: portalStore,
			Sink:     portalStore,
			Now:      func() time.Time { return portalTestNow },
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
