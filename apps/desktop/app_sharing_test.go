package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestShareLinkLimitsMatchTheServer pins the two numbers this file duplicates.
// The desktop cannot import the server module, so a divergence would show up as
// a form that accepts what the instance then refuses.
func TestShareLinkLimitsMatchTheServer(t *testing.T) {
	// portal.MinPasscodeLength and portal.MaxLinkLifetime.
	if MinShareLinkPasscode != 6 {
		t.Errorf("MinShareLinkPasscode = %d, want the server's 6", MinShareLinkPasscode)
	}
	if MaxShareLinkDays != 90 {
		t.Errorf("MaxShareLinkDays = %d, want the server's 90-day ceiling", MaxShareLinkDays)
	}
}

// TestShareLinkValidationRefusesBeforeTheNetwork keeps the obvious mistakes off
// the wire and, more usefully, gives them a sentence a person can act on.
func TestShareLinkValidationRefusesBeforeTheNetwork(t *testing.T) {
	valid := CreateShareLinkInput{
		Label:         "Mum",
		Passcode:      "long-enough",
		ExpiresInDays: 14,
		Grants:        ShareGrantsDTO{WakingWindows: true},
	}
	if err := validateShareLinkInput(valid); err != nil {
		t.Fatalf("a valid link was refused: %v", err)
	}

	for _, testCase := range []struct {
		name  string
		input CreateShareLinkInput
		want  string
	}{
		{"no name", func() CreateShareLinkInput { c := valid; c.Label = "  "; return c }(), "name"},
		{"short passcode", func() CreateShareLinkInput { c := valid; c.Passcode = "abc"; return c }(), "passcode"},
		{"no expiry", func() CreateShareLinkInput { c := valid; c.ExpiresInDays = 0; return c }(), "expiry"},
		{"past the ceiling", func() CreateShareLinkInput { c := valid; c.ExpiresInDays = 365; return c }(), "expiry"},
		// A link that grants nothing is not a safe default, it is a pointless
		// one: it still exists, still has a token, and shows an empty page.
		{"grants nothing", func() CreateShareLinkInput { c := valid; c.Grants = ShareGrantsDTO{}; return c }(), "shows nothing"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateShareLinkInput(testCase.input)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(strings.ToLower(err.Error()), testCase.want) {
				t.Errorf("message %q does not mention %q", err.Error(), testCase.want)
			}
		})
	}
}

// TestSharingIsOffRatherThanBrokenWithoutSync. Sharing runs on the user's own
// server; a desktop with sync off has no links because there is nowhere to keep
// them, which is a different statement from "sharing is not built" — the one
// the screen used to make.
func TestSharingIsOffRatherThanBrokenWithoutSync(t *testing.T) {
	app := newTestApp(t)

	links, err := app.GetBackendShareLinks()
	if err != nil {
		t.Fatalf("share links: %v", err)
	}
	if links.Status != "off" {
		t.Errorf("status = %q, want off", links.Status)
	}
	if !strings.Contains(links.Message, "your own server") {
		t.Errorf("message %q does not say where sharing runs", links.Message)
	}
	if links.Links == nil {
		t.Error("links is nil; the UI expects an empty list")
	}
	if links.MinPasscodeLength != MinShareLinkPasscode || links.MaxDays != MaxShareLinkDays {
		t.Error("the form limits are missing from the off state")
	}
}

// TestErasureNeedsTheLinkIdTypedBack. Revoking stops access; erasing removes
// the record that the link existed. They must not be one click apart.
func TestErasureNeedsTheLinkIdTypedBack(t *testing.T) {
	app := newTestApp(t)

	result, err := app.EraseBackendShareLink(ShareLinkActionInput{
		ProfileID:    "profile-1",
		Confirmation: "profile-2",
	})
	if err != nil {
		t.Fatalf("erase: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("status = %q, want error on a mismatched confirmation", result.Status)
	}
	if !strings.Contains(result.Message, "Revoking is enough") {
		t.Errorf("message %q does not offer the gentler action", result.Message)
	}
}

// TestShareLinkStatesReadAsPlainFacts. "active" is a database word; the screen
// has to say what it means for the person holding the link.
func TestShareLinkStatesReadAsPlainFacts(t *testing.T) {
	created := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	expires := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

	record := backendShareProfileRecord{
		ProfileID: "abc123",
		Label:     "Mum",
		State:     "active",
		CreatedAt: created,
		ExpiresAt: expires,
	}
	record.Grants.WakingWindows = true
	record.Grants.AllowRequests = true

	dto := shareLinkDTO(record)
	if dto.StateLabel != "Working now" {
		t.Errorf("state label = %q", dto.StateLabel)
	}
	if !strings.HasPrefix(dto.ExpiresLabel, "Expires ") {
		t.Errorf("expiry label = %q", dto.ExpiresLabel)
	}
	if !strings.Contains(dto.GrantSummary, "likely awake") ||
		!strings.Contains(dto.GrantSummary, "ask for a time") {
		t.Errorf("grant summary %q does not describe both grants", dto.GrantSummary)
	}

	record.State = "revoked"
	if revoked := shareLinkDTO(record); revoked.StateLabel != "Revoked" ||
		!strings.HasPrefix(revoked.ExpiresLabel, "Expired ") {
		t.Errorf("revoked link reads as %+v", revoked)
	}

	// A link with nothing granted must not read as though it shares something.
	record.Grants.WakingWindows = false
	record.Grants.AllowRequests = false
	if none := shareLinkDTO(record); none.GrantSummary != "Shows nothing yet" {
		t.Errorf("empty grants summarised as %q", none.GrantSummary)
	}
}

// TestUnnamedLinkStillReads keeps a missing private label from rendering as a
// blank row the owner cannot tell apart.
func TestUnnamedLinkStillReads(t *testing.T) {
	dto := shareLinkDTO(backendShareProfileRecord{ProfileID: "abc", State: "active"})
	if dto.Label != "Unnamed link" {
		t.Errorf("label = %q", dto.Label)
	}
}

// TestShareLinkPreviewCarriesTheInstancesPage reads the recipient's page from
// the owner route and refuses a preview that belongs to another link.
func TestShareLinkPreviewCarriesTheInstancesPage(t *testing.T) {
	app := newTestApp(t)
	previewedFor := "prof_mum"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "synthetic-token"})
		case "/v1/portal/profiles/prof_mum/preview":
			if r.Method != http.MethodGet {
				t.Errorf("preview method = %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"schema_version": "v1",
				"profileId":      previewedFor,
				"state":          "active",
				"html":           `<div class="page"><header class="masthead">Availability</header><main></main></div>`,
				"stylesheet":     ":root, :host { --ink: #221f1a; }",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureBackendForTest(t, app, server.URL)

	preview, err := app.PreviewBackendShareLink(ShareLinkActionInput{ProfileID: "prof_mum"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != "ok" || !strings.Contains(preview.HTML, "masthead") || preview.State != "active" {
		t.Fatalf("preview = %+v", preview)
	}

	previewedFor = "prof_someone_else"
	preview, err = app.PreviewBackendShareLink(ShareLinkActionInput{ProfileID: "prof_mum"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != "error" || preview.HTML != "" {
		t.Fatalf("a preview for another link was shown: %+v", preview)
	}
}

func TestShareLinkPreviewIsOffWithoutSync(t *testing.T) {
	app := newTestApp(t)
	preview, err := app.PreviewBackendShareLink(ShareLinkActionInput{ProfileID: "prof_mum"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != "off" || preview.HTML != "" {
		t.Fatalf("preview without sync = %+v", preview)
	}
}

// TestShareAccessLabelsCoverTheServerEvents pins the event names this file
// mirrors from portal.AccessEvent. They had drifted: the desktop knew
// "passcode_failure" while the instance recorded "passcode_rejected", so the
// owner saw raw codes such as "page_view".
func TestShareAccessLabelsCoverTheServerEvents(t *testing.T) {
	for _, event := range []string{"page_view", "availability_read", "passcode_accepted",
		"passcode_rejected", "throttled", "link_rejected"} {
		label := shareAccessLabel(event)
		if label == "Other activity" || strings.Contains(label, "_") {
			t.Errorf("event %q has no label (%q)", event, label)
		}
	}
	if label := shareAccessLabel("something_new"); label != "Other activity" {
		t.Errorf("an unknown event is shown as %q", label)
	}
}
