package main

import (
	"os"
	"strings"
	"testing"
)

// A live round trip against a real zeitboardd, run only when
// ZEITBOARD_LIVE_BACKEND and ZEITBOARD_LIVE_TOKEN are set. It is skipped in CI
// and by default; it exists because the last three slices each found a defect
// that both sides' unit tests were blind to — the desktop asserted its own
// request shape and the server asserted its own, and nothing compared them.
//
//	ZEITBOARD_LIVE_BACKEND=https://127.0.0.1:8802 \
//	ZEITBOARD_LIVE_TOKEN=<device token> \
//	go test ./... -run TestShareLinkRoundTripAgainstARealInstance -count=1
func TestShareLinkRoundTripAgainstARealInstance(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("ZEITBOARD_LIVE_BACKEND"))
	secret := strings.TrimSpace(os.Getenv("ZEITBOARD_LIVE_SECRET"))
	if base == "" || secret == "" {
		t.Skip("set ZEITBOARD_LIVE_BACKEND and ZEITBOARD_LIVE_SECRET to run this")
	}

	// Enrol properly rather than hand-building a config: the exported methods
	// below all go through requireBackendSync, and a test that bypassed it
	// would exercise a path the app never takes. The first version of this test
	// did bypass it, and reported that revocation had not worked when in fact
	// the revoke call had never left the machine.
	app := newTestApp(t)
	status, err := app.ConfigureBackendSync(BackendSyncInput{
		Enabled:            true,
		BackendURL:         base,
		EnrollmentSecret:   secret,
		DeviceLabel:        "live-test",
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("configure sync: %v", err)
	}
	if !status.Enabled {
		t.Fatalf("sync did not enable: %+v", status)
	}
	cfg, token, err := app.requireBackendSync()
	if err != nil {
		t.Fatalf("sync config: %v", err)
	}
	ctx := app.applicationContext()
	client := app.newDesktopBackendClient(cfg, token)

	// Create.
	var created backendCreatedShareProfileResponse
	payload := map[string]any{
		"label":         "Live test link",
		"passcode":      "long-enough-passcode",
		"expiresInDays": 7,
		"grants":        map[string]bool{"wakingWindows": true, "allowRequests": true},
	}
	if err := client.postJSON(ctx, "/v1/portal/profiles", payload, &created); err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.LinkURL == "" || created.ProfileID == "" {
		t.Fatalf("create returned %+v", created)
	}
	if created.Disclosure == "" {
		t.Error("the instance returned no disclosure to show before sharing")
	}

	// List, through the same decoder the desktop uses.
	links := app.fetchBackendShareLinks(ctx, cfg, token)
	if links.Status != "ok" {
		t.Fatalf("list status = %q (%s)", links.Status, links.Message)
	}
	found := false
	for _, link := range links.Links {
		if link.ProfileID != created.ProfileID {
			continue
		}
		found = true
		if link.Label != "Live test link" {
			t.Errorf("label = %q; the private label is owner data and must round-trip", link.Label)
		}
		if link.State != "active" || link.StateLabel != "Working now" {
			t.Errorf("state = %q/%q", link.State, link.StateLabel)
		}
		if !link.Grants.WakingWindows || !link.Grants.AllowRequests {
			t.Errorf("grants did not round-trip: %+v", link.Grants)
		}
	}
	if !found {
		t.Fatalf("the created link is missing from the list of %d", len(links.Links))
	}

	// Revoke through the exported method, then confirm the state actually
	// changed rather than the call merely returning 200.
	revoked, err := app.RevokeBackendShareLink(ShareLinkActionInput{ProfileID: created.ProfileID})
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked.Status != "ok" {
		t.Fatalf("revoke returned %q: %s", revoked.Status, revoked.Message)
	}
	for _, link := range revoked.Links {
		if link.ProfileID == created.ProfileID && link.State == "active" {
			t.Error("the link still reports active after revocation")
		}
	}

	// Erasure needs the id typed back, and then the record is gone.
	erased, err := app.EraseBackendShareLink(ShareLinkActionInput{
		ProfileID:    created.ProfileID,
		Confirmation: created.ProfileID,
	})
	if err != nil {
		t.Fatalf("erase: %v", err)
	}
	if erased.Status != "ok" {
		t.Fatalf("erase returned %q: %s", erased.Status, erased.Message)
	}
	for _, link := range erased.Links {
		if link.ProfileID == created.ProfileID {
			t.Error("the erased link is still listed")
		}
	}
}
