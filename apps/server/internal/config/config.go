package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultListenAddress = "127.0.0.1:8765"

	EnvConfig               = "ZEITBOARD_CONFIG"
	EnvListenAddress        = "ZEITBOARD_LISTEN_ADDR"
	EnvTLSCert              = "ZEITBOARD_TLS_CERT"
	EnvTLSKey               = "ZEITBOARD_TLS_KEY"
	EnvDataDir              = "ZEITBOARD_DATA_DIR"
	EnvDataKey              = "ZEITBOARD_DATA_KEY"
	EnvDataKeyFile          = "ZEITBOARD_DATA_KEY_FILE"
	EnvEnrollmentSecret     = "ZEITBOARD_ENROLLMENT_SECRET"
	EnvEnrollmentSecretFile = "ZEITBOARD_ENROLLMENT_SECRET_FILE"
	EnvLLMProvider          = "ZEITBOARD_LLM_PROVIDER"
	EnvLLMModel             = "ZEITBOARD_LLM_MODEL"
	EnvLLMAPIKey            = "ZEITBOARD_LLM_API_KEY"
	EnvLLMAPIKeyFile        = "ZEITBOARD_LLM_API_KEY_FILE"
	EnvLLMEndpoint          = "ZEITBOARD_LLM_ENDPOINT"
)

type Config struct {
	ListenAddress         string          `json:"listenAddress"`
	TLSCertPath           string          `json:"tlsCertPath"`
	TLSKeyPath            string          `json:"tlsKeyPath"`
	DataDir               string          `json:"dataDir"`
	DataKeyFile           string          `json:"dataKeyFile"`
	EnrollmentSecretFile  string          `json:"enrollmentSecretFile"`
	DataKey               []byte          `json:"-"`
	EnrollmentSecret      string          `json:"-"`
	Assistant             AssistantConfig `json:"assistant"`
	UsesSelfSignedDevCert bool            `json:"-"`
}

type AssistantConfig struct {
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	APIKeyFile string `json:"apiKeyFile"`
	Endpoint   string `json:"endpoint"`
	APIKey     string `json:"-"`
}

func Load(path string) (Config, error) {
	if path == "" {
		path = os.Getenv(EnvConfig)
	}
	cfg := Config{
		ListenAddress: defaultListenAddress,
		DataDir:       "data",
	}
	if path != "" {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return Config{}, fmt.Errorf("resolve config path: %w", err)
		}
		path = absolutePath
		if err := loadFile(path, &cfg); err != nil {
			return Config{}, err
		}
		resolveConfigFilePaths(&cfg, filepath.Dir(path))
	}

	overrideString(&cfg.ListenAddress, EnvListenAddress)
	overrideString(&cfg.TLSCertPath, EnvTLSCert)
	overrideString(&cfg.TLSKeyPath, EnvTLSKey)
	overrideString(&cfg.DataDir, EnvDataDir)
	overrideString(&cfg.DataKeyFile, EnvDataKeyFile)
	overrideString(&cfg.EnrollmentSecretFile, EnvEnrollmentSecretFile)
	overrideString(&cfg.Assistant.Provider, EnvLLMProvider)
	overrideString(&cfg.Assistant.Model, EnvLLMModel)
	overrideString(&cfg.Assistant.APIKeyFile, EnvLLMAPIKeyFile)
	overrideString(&cfg.Assistant.Endpoint, EnvLLMEndpoint)

	dataKey, err := loadDataKey(cfg.DataKeyFile)
	if err != nil {
		return Config{}, err
	}
	cfg.DataKey = dataKey

	secret, err := loadSecret(EnvEnrollmentSecret, cfg.EnrollmentSecretFile)
	if err != nil {
		return Config{}, err
	}
	if len([]byte(secret)) < 16 {
		return Config{}, errors.New("enrollment secret must be at least 16 bytes")
	}
	cfg.EnrollmentSecret = secret
	apiKey, err := loadOptionalSecret(EnvLLMAPIKey, cfg.Assistant.APIKeyFile)
	if err != nil {
		return Config{}, err
	}
	cfg.Assistant.APIKey = apiKey
	if cfg.Assistant.Provider == "" {
		cfg.Assistant.Provider = "disabled"
	}

	if cfg.ListenAddress == "" {
		cfg.ListenAddress = defaultListenAddress
	}
	if cfg.DataDir == "" {
		return Config{}, errors.New("data directory is required")
	}
	if (cfg.TLSCertPath == "") != (cfg.TLSKeyPath == "") {
		return Config{}, errors.New("TLS cert and key paths must be provided together")
	}
	if cfg.TLSCertPath == "" {
		if !isLocalAddress(cfg.ListenAddress) {
			return Config{}, errors.New("TLS cert/key paths are required for non-local listen addresses")
		}
		cfg.UsesSelfSignedDevCert = true
	}
	return cfg, nil
}

func (c Config) TLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.TLSCertPath != "" {
		cert, err := tls.LoadX509KeyPair(c.TLSCertPath, c.TLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load TLS certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
		return tlsConfig, nil
	}
	cert, err := selfSignedLocalhostCertificate()
	if err != nil {
		return nil, err
	}
	tlsConfig.Certificates = []tls.Certificate{cert}
	return tlsConfig, nil
}

func loadFile(path string, cfg *Config) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open config file: %w", err)
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return fmt.Errorf("decode config file: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("decode config file: expected exactly one JSON value")
		}
		return fmt.Errorf("decode config file trailing data: %w", err)
	}
	return nil
}

func resolveConfigFilePaths(cfg *Config, baseDir string) {
	cfg.TLSCertPath = resolveConfigFilePath(baseDir, cfg.TLSCertPath)
	cfg.TLSKeyPath = resolveConfigFilePath(baseDir, cfg.TLSKeyPath)
	cfg.DataDir = resolveConfigFilePath(baseDir, cfg.DataDir)
	cfg.DataKeyFile = resolveConfigFilePath(baseDir, cfg.DataKeyFile)
	cfg.EnrollmentSecretFile = resolveConfigFilePath(baseDir, cfg.EnrollmentSecretFile)
	cfg.Assistant.APIKeyFile = resolveConfigFilePath(baseDir, cfg.Assistant.APIKeyFile)
}

func resolveConfigFilePath(baseDir, value string) string {
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(baseDir, value)
}

func overrideString(target *string, envName string) {
	if value := os.Getenv(envName); value != "" {
		*target = value
	}
}

func loadDataKey(path string) ([]byte, error) {
	value := strings.TrimSpace(os.Getenv(EnvDataKey))
	if value == "" {
		if path == "" {
			return nil, errors.New("data key is required from ZEITBOARD_DATA_KEY or dataKeyFile")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read data key file: %w", err)
		}
		value = strings.TrimSpace(string(data))
	}
	key, err := DecodeDataKey(value)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func DecodeDataKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("data key is empty")
	}
	for _, decoder := range []struct {
		name string
		fn   func(string) ([]byte, error)
	}{
		{"base64", base64.StdEncoding.DecodeString},
		{"raw base64", base64.RawStdEncoding.DecodeString},
		{"url base64", base64.URLEncoding.DecodeString},
		{"raw url base64", base64.RawURLEncoding.DecodeString},
	} {
		if decoded, err := decoder.fn(value); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len([]byte(value)) == 32 {
		return []byte(value), nil
	}
	return nil, errors.New("data key must decode to exactly 32 bytes")
}

func loadSecret(envName, path string) (string, error) {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		return value, nil
	}
	if path == "" {
		return "", fmt.Errorf("%s or enrollmentSecretFile is required", envName)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read enrollment secret file: %w", err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", errors.New("enrollment secret is empty")
	}
	return value, nil
}

func loadOptionalSecret(envName, path string) (string, error) {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		return value, nil
	}
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read assistant API key file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func isLocalAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func selfSignedLocalhostCertificate() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate dev TLS key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate dev TLS serial: %w", err)
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create dev TLS cert: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal dev TLS key: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse dev TLS key pair: %w", err)
	}
	return cert, nil
}
