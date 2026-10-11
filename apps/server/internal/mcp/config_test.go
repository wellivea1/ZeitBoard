package mcp

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configure sets the connector's environment for one test.
func configure(t *testing.T, backendURL, certificateFile string, skipVerify bool) Config {
	t.Helper()
	t.Setenv(EnvBackendURL, backendURL)
	t.Setenv(EnvDeviceToken, "synthetic-device-token")
	t.Setenv(EnvServerCertificateFile, certificateFile)
	value := "false"
	if skipVerify {
		value = "true"
	}
	t.Setenv(EnvInsecureSkipVerify, value)
	return LoadConfigFromEnv()
}

// The connector trusts the owner's server by its certificate, with
// verification on, rather than by skipping it (ADR-0055).
func TestTheConnectorTrustsTheServersOwnCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	certificate := filepath.Join(t.TempDir(), "server.pem")
	if err := os.WriteFile(certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}

	trusting := configure(t, server.URL, certificate, false)
	if !trusting.Configured {
		t.Fatalf("not configured: %s", trusting.UnavailableReason)
	}
	response, err := trusting.HTTPClient().Get(server.URL)
	if err != nil {
		t.Fatalf("the server's own certificate was not trusted: %v", err)
	}
	response.Body.Close()

	plain := configure(t, server.URL, "", false)
	if _, err := plain.HTTPClient().Get(server.URL); err == nil {
		t.Fatal("a self-signed server was trusted without its certificate")
	}
}

func TestSkippingVerificationIsForLocalhostOnly(t *testing.T) {
	if local := configure(t, "https://127.0.0.1:8765", "", true); !local.Configured {
		t.Fatalf("localhost development was refused: %s", local.UnavailableReason)
	}
	remote := configure(t, "https://zeitboard.example.test", "", true)
	if remote.Configured || !strings.Contains(remote.UnavailableReason, EnvServerCertificateFile) {
		t.Fatalf("skipping verification for a remote server = %+v", remote)
	}
}

func TestACertificateFileThatIsNotOneLeavesTheConnectorUnconfigured(t *testing.T) {
	dir := t.TempDir()
	notCertificate := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(notCertificate, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("synthetic")}), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"missing":   filepath.Join(dir, "missing.pem"),
		"a key":     notCertificate,
		"directory": dir,
	} {
		t.Run(name, func(t *testing.T) {
			if cfg := configure(t, "https://zeitboard.example.test", path, false); cfg.Configured || cfg.UnavailableReason == "" {
				t.Fatalf("configured from %s: %+v", name, cfg)
			}
		})
	}
}
