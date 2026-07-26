package config

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
