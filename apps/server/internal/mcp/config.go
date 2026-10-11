package mcp

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"non24.app/core/tlstrust"
)

const (
	EnvBackendURL         = "ZEITBOARD_MCP_BACKEND_URL"
	EnvDeviceToken        = "ZEITBOARD_MCP_DEVICE_TOKEN"
	EnvDeviceTokenFile    = "ZEITBOARD_MCP_DEVICE_TOKEN_FILE"
	EnvTotalCallBudget    = "ZEITBOARD_MCP_TOTAL_CALL_BUDGET"
	EnvProposeCallBudget  = "ZEITBOARD_MCP_PROPOSE_CALL_BUDGET"
	EnvRequestTimeoutMS   = "ZEITBOARD_MCP_REQUEST_TIMEOUT_MS"
	EnvInsecureSkipVerify = "ZEITBOARD_MCP_INSECURE_SKIP_VERIFY"
	// EnvServerCertificateFile names a PEM file holding the backend's own
	// certificate, or the authority that issued it (ADR-0055).
	EnvServerCertificateFile = "ZEITBOARD_MCP_SERVER_CERTIFICATE_FILE"

	defaultTotalCallBudget   = 20
	defaultProposeCallBudget = 5
	defaultRequestTimeout    = 10 * time.Second
)

type Config struct {
	BackendURL         string
	DeviceToken        string
	TotalCallBudget    int
	ProposeCallBudget  int
	RequestTimeout     time.Duration
	InsecureSkipVerify bool
	// ServerCertificatePEM is trusted for the backend beside the system's
	// roots; verification stays on.
	ServerCertificatePEM string
	Configured           bool
	UnavailableReason    string
}

func LoadConfigFromEnv() Config {
	cfg := Config{
		BackendURL:        strings.TrimSpace(os.Getenv(EnvBackendURL)),
		DeviceToken:       strings.TrimSpace(os.Getenv(EnvDeviceToken)),
		TotalCallBudget:   intEnv(EnvTotalCallBudget, defaultTotalCallBudget),
		ProposeCallBudget: intEnv(EnvProposeCallBudget, defaultProposeCallBudget),
		RequestTimeout:    durationEnv(EnvRequestTimeoutMS, defaultRequestTimeout),
	}
	if cfg.DeviceToken == "" {
		cfg.DeviceToken = strings.TrimSpace(readOptionalSecretFile(os.Getenv(EnvDeviceTokenFile)))
	}
	cfg.InsecureSkipVerify = boolEnv(EnvInsecureSkipVerify)
	if path := strings.TrimSpace(os.Getenv(EnvServerCertificateFile)); path != "" {
		data, err := readBounded(path, tlstrust.MaxPEMBytes)
		if err != nil {
			cfg.UnavailableReason = "the server certificate file could not be read"
			return cfg
		}
		// PEM or a single DER certificate; anything else fails validation below.
		if converted, err := tlstrust.PEM(data); err == nil {
			data = []byte(converted)
		}
		cfg.ServerCertificatePEM = string(data)
	}
	if err := cfg.validate(); err != nil {
		cfg.Configured = false
		cfg.UnavailableReason = err.Error()
		return cfg
	}
	cfg.Configured = true
	return cfg
}

func (c Config) HTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// validate already refused a certificate file that is not one.
	if tlsConfig, err := tlstrust.Config(c.ServerCertificatePEM); err == nil {
		tlsConfig.InsecureSkipVerify = c.InsecureSkipVerify //nolint:gosec // Localhost development only; validate refuses it elsewhere.
		transport.TLSClientConfig = tlsConfig
	}
	timeout := c.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

func (c Config) BackendClient() *BackendClient {
	return &BackendClient{
		BaseURL: strings.TrimRight(c.BackendURL, "/"),
		Token:   c.DeviceToken,
		Client:  c.HTTPClient(),
	}
}

func (c Config) validate() error {
	if c.BackendURL == "" {
		return errors.New("backend URL is not configured")
	}
	parsed, err := url.Parse(c.BackendURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("backend URL must be an https URL")
	}
	if c.DeviceToken == "" {
		return errors.New("device token is not configured")
	}
	if c.ServerCertificatePEM != "" {
		if _, err := tlstrust.Certificates([]byte(c.ServerCertificatePEM)); err != nil {
			return fmt.Errorf("server certificate: %w", err)
		}
	}
	switch host := parsed.Hostname(); {
	case !c.InsecureSkipVerify, host == "localhost", host == "127.0.0.1", host == "::1":
	default:
		return errors.New("skipping TLS verification is for localhost development only; set " +
			EnvServerCertificateFile + " to the server's certificate instead")
	}
	if c.TotalCallBudget <= 0 {
		return errors.New("total call budget must be positive")
	}
	if c.ProposeCallBudget <= 0 {
		return errors.New("propose call budget must be positive")
	}
	return nil
}

func intEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func boolEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// readBounded reads at most limit bytes of a file, refusing a longer one.
func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("file is too large")
	}
	return data, nil
}

func readOptionalSecretFile(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func (c Config) StartupMessage() string {
	if c.Configured {
		return fmt.Sprintf("zeitboard MCP adapter configured for %s", c.BackendURL)
	}
	return "zeitboard MCP adapter exposing no tools: " + c.UnavailableReason
}
