package daemon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"non24.app/server/internal/config"
)

func TestServeSignalsReadyOnlyAfterStartupAndStops(t *testing.T) {
	configPath := writeDaemonTestConfig(t)
	stop := make(chan struct{})
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- serve(configPath, stop, ready)
	}()

	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("daemon exited before readiness: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("daemon did not become ready")
	}

	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graceful stop: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestServeDoesNotSignalReadyWhenConfigFails(t *testing.T) {
	ready := make(chan struct{})
	err := serve(filepath.Join(t.TempDir(), "missing.json"), nil, ready)
	if err == nil {
		t.Fatal("expected missing config to fail")
	}
	select {
	case <-ready:
		t.Fatal("readiness must not be signaled after startup failure")
	default:
	}
}

func writeDaemonTestConfig(t *testing.T) string {
	t.Helper()
	for _, name := range []string{
		config.EnvConfig,
		config.EnvListenAddress,
		config.EnvTLSCert,
		config.EnvTLSKey,
		config.EnvDataDir,
		config.EnvDataKey,
		config.EnvDataKeyFile,
		config.EnvEnrollmentSecret,
		config.EnvEnrollmentSecretFile,
		config.EnvLLMProvider,
		config.EnvLLMModel,
		config.EnvLLMAPIKey,
		config.EnvLLMAPIKeyFile,
		config.EnvLLMEndpoint,
		config.EnvPortalEnabled,
		config.EnvPortalOrigin,
	} {
		t.Setenv(name, "")
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "data-key.txt")
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(dir, "enrollment-secret.txt")
	if err := os.WriteFile(secretPath, []byte("enrollment-secret-123"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"listenAddress":        "127.0.0.1:0",
		"dataDir":              filepath.Join(dir, "data"),
		"dataKeyFile":          keyPath,
		"enrollmentSecretFile": secretPath,
		"assistant":            map[string]any{"provider": "disabled"},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}
