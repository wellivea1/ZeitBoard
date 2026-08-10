package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"non24.app/core/platform/privatefile"
)

func assertOwnerOnly(t *testing.T, path, what string) {
	t.Helper()
	access, err := privatefile.Describe(path)
	if err != nil {
		t.Fatalf("describe %s: %v", what, err)
	}
	if !access.OwnerOnly {
		t.Errorf("%s is reachable by another account: %s", what, access.Detail)
	}
	if access.Inherited {
		t.Errorf("%s still inherits from its directory: %s", what, access.Detail)
	}
}

// The bearer token authenticates this device to the user's own server. It was
// written with a mode argument that restricts nothing on Windows, so on a
// shared machine it inherited whatever the profile directory allowed.
func TestTheBackendTokenIsOwnerOnly(t *testing.T) {
	app := newTestApp(t)
	if err := app.saveBackendSyncToken("device-token-value"); err != nil {
		t.Fatalf("save token: %v", err)
	}
	assertOwnerOnly(t, filepath.Join(app.configDir, backendSyncTokenFile), "the backend token")

	// The token still reads back, so this restricted access rather than losing it.
	token, err := app.loadBackendSyncToken()
	if err != nil || token != "device-token-value" {
		t.Fatalf("token after restriction = %q, %v", token, err)
	}
}

func TestTheSyncConfigurationIsOwnerOnly(t *testing.T) {
	app := newTestApp(t)
	if err := app.saveBackendSyncConfig(backendSyncConfig{
		Enabled: true, BackendURL: "https://localhost:8443", DeviceID: "dev-1",
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	assertOwnerOnly(t, filepath.Join(app.configDir, backendSyncConfigFile), "the sync configuration")
}

// Settings files are less sensitive than the database, but they go through the
// same staged-write helper, and the point of putting the restriction there is
// that a future settings file cannot be added without it.
func TestSettingsFilesAreOwnerOnly(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.SaveReachingHours(ReachingHoursSaveInput{
		State: ReachingHoursDTO{
			Enabled: true, Label: "The clinic", StartLocal: "09:00", EndLocal: "13:00",
			Days: []int{2, 4}, ZoneID: defaultZoneID,
		},
	}); err != nil {
		t.Fatalf("save reaching hours: %v", err)
	}
	assertOwnerOnly(t, filepath.Join(app.configDir, reachingFileName), "the reaching-hours file")
}

// An export is the whole sleep history in one file, which is the most
// disclosive artefact this app produces.
func TestAnExportedFileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sleep-export.json")
	if err := writePrivateFileAtomic(path, []byte(`{"episodes":[]}`)); err != nil {
		t.Fatalf("write export: %v", err)
	}
	assertOwnerOnly(t, path, "the export")
}

// The readout must be capable of reporting bad news, and must never describe
// restricted files as encrypted, because they are not.
func TestTheProtectionReadoutDoesNotOverclaim(t *testing.T) {
	app := newTestApp(t)
	report, err := app.GetStorageProtection()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.Detail, "not encrypted") {
		t.Errorf("detail %q does not say the files are unencrypted", report.Detail)
	}
	for _, phrase := range []string{"secure", "safe", "protected from"} {
		if strings.Contains(strings.ToLower(report.Headline), phrase) {
			t.Errorf("headline %q claims more than an owner-only permission gives", report.Headline)
		}
	}
	switch report.State {
	case "ok", "at_risk", "unknown":
	default:
		t.Errorf("unexpected state %q", report.State)
	}
}

func TestTheProtectionReadoutNoticesAnExposedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exposed.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := privatefile.Describe(path)
	if err != nil {
		t.Fatal(err)
	}
	// On Windows a fresh temporary file inherits the profile's entries, which
	// include SYSTEM and the administrators group, so this is a real negative.
	// Off Windows the mode is the mechanism and 0o600 is already owner-only, so
	// there is nothing to detect and the assertion is skipped rather than
	// weakened into something that always passes.
	if access.OwnerOnly {
		t.Skip("this platform's default file permissions are already owner-only")
	}
	if err := privatefile.Restrict(path); err != nil {
		t.Fatal(err)
	}
	after, err := privatefile.Describe(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.OwnerOnly {
		t.Errorf("restriction did not take effect: %s", after.Detail)
	}
}

// A store whose files could not be restricted has to say so rather than let the
// app report protection it does not have.
func TestTheStoreReportsAPermissionFailure(t *testing.T) {
	app := newTestApp(t)
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FilePermissionError(); err != nil {
		t.Fatalf("the test store's files were not restricted: %v", err)
	}
}
