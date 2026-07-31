package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePortalConfig(t *testing.T, portal map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "data-key.txt")
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(dir, "enrollment-secret.txt")
	if err := os.WriteFile(secretPath, []byte("enrollment-secret-123"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"listenAddress":        "127.0.0.1:8765",
		"dataDir":              filepath.Join(dir, "data"),
		"dataKeyFile":          keyPath,
		"enrollmentSecretFile": secretPath,
	}
	if portal != nil {
		payload["portal"] = portal
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func clearPortalEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvPortalEnabled, "")
	t.Setenv(EnvPortalOrigin, "")
}

// TestPortalIsDisabledByDefault is exposure-gate item 1. A config that says
// nothing about the portal must not serve one.
func TestPortalIsDisabledByDefault(t *testing.T) {
	clearPortalEnv(t)
	cfg, err := Load(writePortalConfig(t, nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Portal.Enabled {
		t.Error("portal is enabled by a config that never mentions it")
	}
	if cfg.Portal.PublicOrigin != "" {
		t.Errorf("portal origin defaulted to %q", cfg.Portal.PublicOrigin)
	}
}

// TestPortalRequiresOriginWhenEnabled refuses a half-configured public
// surface: without an exact origin the CSRF check could not be enforced, so
// the daemon must fail to start rather than serve without it.
func TestPortalRequiresOriginWhenEnabled(t *testing.T) {
	clearPortalEnv(t)
	_, err := Load(writePortalConfig(t, map[string]any{"enabled": true}))
	if err == nil {
		t.Fatal("an enabled portal without a public origin was accepted")
	}
	if !strings.Contains(err.Error(), "publicOrigin") {
		t.Errorf("error does not name the missing field: %v", err)
	}
}

func TestPortalOriginRules(t *testing.T) {
	clearPortalEnv(t)
	cases := map[string]struct {
		origin  string
		wantErr bool
	}{
		"https":              {"https://share.example.com", false},
		"https with port":    {"https://share.example.com:8443", false},
		"loopback http":      {"http://localhost:8765", false},
		"remote http":        {"http://share.example.com", true},
		"with path":          {"https://share.example.com/portal", true},
		"with query":         {"https://share.example.com?a=b", true},
		"with credentials":   {"https://user:pass@share.example.com", true},
		"unsupported scheme": {"ftp://share.example.com", true},
		"bare host":          {"share.example.com", true},
	}
	for name, testCase := range cases {
		cfg, err := Load(writePortalConfig(t, map[string]any{
			"enabled":      true,
			"publicOrigin": testCase.origin,
		}))
		if testCase.wantErr {
			if err == nil {
				t.Errorf("%s: origin %q was accepted", name, testCase.origin)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: origin %q was rejected: %v", name, testCase.origin, err)
			continue
		}
		if cfg.Portal.PublicOrigin != testCase.origin {
			t.Errorf("%s: origin = %q, want %q", name, cfg.Portal.PublicOrigin, testCase.origin)
		}
	}
}

// TestPortalOriginTrailingSlashIsNormalized keeps the Origin comparison exact:
// browsers never send a trailing slash, so a configured one must not silently
// break every mutation.
func TestPortalOriginTrailingSlashIsNormalized(t *testing.T) {
	clearPortalEnv(t)
	cfg, err := Load(writePortalConfig(t, map[string]any{
		"enabled":      true,
		"publicOrigin": "https://share.example.com/",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Portal.PublicOrigin != "https://share.example.com" {
		t.Errorf("origin = %q, want the trailing slash removed", cfg.Portal.PublicOrigin)
	}
}

func TestPortalEnvOverrides(t *testing.T) {
	clearPortalEnv(t)
	path := writePortalConfig(t, nil)
	t.Setenv(EnvPortalEnabled, "true")
	t.Setenv(EnvPortalOrigin, "https://env.example.com")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.Portal.Enabled || cfg.Portal.PublicOrigin != "https://env.example.com" {
		t.Errorf("env overrides not applied: %+v", cfg.Portal)
	}

	t.Setenv(EnvPortalEnabled, "yes-please")
	if _, err := Load(path); err == nil {
		t.Error("a non-boolean portal toggle was accepted")
	}
}
