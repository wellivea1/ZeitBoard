package config

import (
	"bytes"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadResolvesFilePathsRelativeToConfig(t *testing.T) {
	clearConfigEnvironment(t)
	dir := t.TempDir()
	secretsDir := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dataKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	writeConfigTestFile(t, filepath.Join(secretsDir, "data-key.txt"), dataKey)
	writeConfigTestFile(t, filepath.Join(secretsDir, "enrollment.txt"), "enrollment-secret-123")
	writeConfigTestFile(t, filepath.Join(secretsDir, "provider.txt"), "provider-secret")

	configPath := filepath.Join(dir, "config.json")
	writeConfigTestFile(t, configPath, `{
  "listenAddress": "127.0.0.1:8765",
  "tlsCertPath": "tls/server.crt",
  "tlsKeyPath": "tls/server.key",
  "dataDir": "state",
  "dataKeyFile": "secrets/data-key.txt",
  "enrollmentSecretFile": "secrets/enrollment.txt",
  "assistant": {
    "provider": "disabled",
    "model": "",
    "apiKeyFile": "secrets/provider.txt",
    "endpoint": ""
  }
}`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		"TLSCertPath":          cfg.TLSCertPath,
		"TLSKeyPath":           cfg.TLSKeyPath,
		"DataDir":              cfg.DataDir,
		"DataKeyFile":          cfg.DataKeyFile,
		"EnrollmentSecretFile": cfg.EnrollmentSecretFile,
		"Assistant.APIKeyFile": cfg.Assistant.APIKeyFile,
	}
	wants := map[string]string{
		"TLSCertPath":          filepath.Join(dir, "tls", "server.crt"),
		"TLSKeyPath":           filepath.Join(dir, "tls", "server.key"),
		"DataDir":              filepath.Join(dir, "state"),
		"DataKeyFile":          filepath.Join(dir, "secrets", "data-key.txt"),
		"EnrollmentSecretFile": filepath.Join(dir, "secrets", "enrollment.txt"),
		"Assistant.APIKeyFile": filepath.Join(dir, "secrets", "provider.txt"),
	}
	for name, got := range checks {
		if got != wants[name] {
			t.Errorf("%s = %q, want %q", name, got, wants[name])
		}
	}
}

func TestLoadRejectsTrailingJSONValue(t *testing.T) {
	clearConfigEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigTestFile(t, path, "{}\n{}")
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "exactly one JSON value") {
		t.Fatalf("Load error = %v, want exactly-one-value rejection", err)
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		EnvConfig, EnvListenAddress, EnvTLSCert, EnvTLSKey, EnvDataDir,
		EnvDataKey, EnvDataKeyFile, EnvEnrollmentSecret,
		EnvEnrollmentSecretFile, EnvLLMProvider, EnvLLMModel,
		EnvLLMAPIKey, EnvLLMAPIKeyFile, EnvLLMEndpoint,
	} {
		t.Setenv(name, "")
	}
}

func writeConfigTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The daemon keeps the certificate it makes for localhost, so a client that
// trusts the file keeps trusting the daemon across restarts (ADR-0055).
func TestTheLocalhostCertificateIsKeptAndRenewedBeforeItExpires(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DataDir: dir}
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	first, err := keptLocalhostCertificate(cfg.LocalhostCertificatePath(), cfg.localhostKeyPath(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Leaf.VerifyHostname("localhost"); err != nil {
		t.Fatalf("the kept certificate is not for localhost: %v", err)
	}
	// Reused while it is good for more than a month.
	again, err := keptLocalhostCertificate(cfg.LocalhostCertificatePath(), cfg.localhostKeyPath(), now.Add(300*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !again.Leaf.Equal(first.Leaf) {
		t.Fatal("a valid kept certificate was replaced")
	}
	// Replaced within a month of its end, and the new one is kept.
	renewed, err := keptLocalhostCertificate(cfg.LocalhostCertificatePath(), cfg.localhostKeyPath(), now.Add(340*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if renewed.Leaf.Equal(first.Leaf) {
		t.Fatal("a certificate a month from its end was kept")
	}
	kept, err := os.ReadFile(cfg.LocalhostCertificatePath())
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(kept)
	if block == nil || !bytes.Equal(block.Bytes, renewed.Leaf.Raw) {
		t.Fatal("the file does not hold the certificate in use")
	}
	// A damaged file is replaced rather than served.
	if err := os.WriteFile(cfg.LocalhostCertificatePath(), []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := keptLocalhostCertificate(cfg.LocalhostCertificatePath(), cfg.localhostKeyPath(), now); err != nil {
		t.Fatalf("a damaged certificate was not replaced: %v", err)
	}
}
