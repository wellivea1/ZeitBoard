package localagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DescriptorFileName = "local-agent.json"
	DescriptorEnv      = "ZEITBOARD_LOCAL_MCP_DESCRIPTOR"
	startupClaimName   = ".local-agent-starting"
	startupClaimTTL    = 30 * time.Second
)

type Descriptor struct {
	SchemaVersion   string    `json:"schema_version"`
	Endpoint        string    `json:"endpoint"`
	Token           string    `json:"token"`
	PID             int       `json:"pid"`
	StartedAt       time.Time `json:"started_at"`
	ProtocolVersion string    `json:"protocol_version"`
}

type EndpointStatus struct {
	Running  bool   `json:"running"`
	Endpoint string `json:"endpoint,omitempty"`
	Message  string `json:"message"`
}

type Endpoint struct {
	server         *http.Server
	listener       net.Listener
	descriptor     Descriptor
	descriptorPath string

	mu       sync.RWMutex
	serveErr error
	closeErr error
	closed   bool
	close    sync.Once
	done     chan struct{}
}

func Start(ctx context.Context, configDir string, capability Capability) (*Endpoint, error) {
	if strings.TrimSpace(configDir) == "" {
		return nil, errors.New("local agent config directory is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	releaseClaim, err := acquireStartupClaim(configDir)
	if err != nil {
		return nil, err
	}
	defer releaseClaim()
	descriptorPath := filepath.Join(configDir, DescriptorFileName)
	live, err := descriptorEndpointLive(ctx, descriptorPath)
	if err != nil {
		return nil, err
	}
	if live {
		return nil, errors.New("another ZeitBoard desktop-local agent endpoint is already running")
	}
	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	handler, err := NewHandler(token, capability)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start local agent listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	descriptor := Descriptor{
		SchemaVersion:   "v1",
		Endpoint:        fmt.Sprintf("http://127.0.0.1:%d/mcp", port),
		Token:           token,
		PID:             os.Getpid(),
		StartedAt:       time.Now().UTC(),
		ProtocolVersion: ProtocolVersion,
	}
	if err := writeDescriptor(descriptorPath, descriptor); err != nil {
		_ = listener.Close()
		return nil, err
	}
	endpoint := &Endpoint{
		server: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    32 * 1024,
		},
		listener:       listener,
		descriptor:     descriptor,
		descriptorPath: descriptorPath,
		done:           make(chan struct{}),
	}
	go func() {
		if err := endpoint.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			endpoint.mu.Lock()
			endpoint.serveErr = err
			endpoint.mu.Unlock()
			_ = removeOwnedDescriptor(endpoint.descriptorPath, endpoint.descriptor.Token)
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = endpoint.Close(shutdownCtx)
		case <-endpoint.done:
		}
	}()
	return endpoint, nil
}

func (e *Endpoint) Status() EndpointStatus {
	if e == nil {
		return EndpointStatus{Message: "The desktop-local agent endpoint is not running."}
	}
	e.mu.RLock()
	err := e.serveErr
	closed := e.closed
	e.mu.RUnlock()
	if closed {
		return EndpointStatus{Endpoint: e.descriptor.Endpoint, Message: "The desktop-local agent endpoint is stopped."}
	}
	if err != nil {
		return EndpointStatus{Endpoint: e.descriptor.Endpoint, Message: "The desktop-local agent endpoint stopped unexpectedly."}
	}
	return EndpointStatus{Running: true, Endpoint: e.descriptor.Endpoint, Message: "Local agent ready; backend-independent read and appearance tools are available."}
}

func (e *Endpoint) Close(ctx context.Context) error {
	if e == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	e.close.Do(func() {
		e.mu.Lock()
		e.closed = true
		e.mu.Unlock()
		close(e.done)
		closeErr := e.server.Shutdown(ctx)
		if closeErr != nil {
			_ = e.server.Close()
		}
		if err := removeOwnedDescriptor(e.descriptorPath, e.descriptor.Token); err != nil && closeErr == nil {
			closeErr = err
		}
		e.mu.Lock()
		e.closeErr = closeErr
		e.mu.Unlock()
	})
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.closeErr
}

func DefaultDescriptorPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv(DescriptorEnv)); override != "" {
		return filepath.Clean(override), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ZeitBoard", DescriptorFileName), nil
}

func LoadDescriptor(path string) (Descriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Descriptor{}, fmt.Errorf("read desktop-local agent descriptor: %w", err)
	}
	var descriptor Descriptor
	if err := decodeOne(data, &descriptor); err != nil {
		return Descriptor{}, errors.New("desktop-local agent descriptor is invalid")
	}
	if err := validateDescriptor(descriptor); err != nil {
		return Descriptor{}, err
	}
	return descriptor, nil
}

func validateDescriptor(descriptor Descriptor) error {
	if descriptor.SchemaVersion != "v1" || descriptor.ProtocolVersion != ProtocolVersion || descriptor.PID <= 0 || len(descriptor.Token) != 64 || descriptor.StartedAt.IsZero() {
		return errors.New("desktop-local agent descriptor has unsupported or incomplete metadata")
	}
	if _, err := hex.DecodeString(descriptor.Token); err != nil {
		return errors.New("desktop-local agent descriptor token is invalid")
	}
	parsed, err := url.Parse(descriptor.Endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.Port() == "" || parsed.Path != "/mcp" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return errors.New("desktop-local agent descriptor endpoint must be loopback-only")
	}
	return nil
}

func writeDescriptor(path string, descriptor Descriptor) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create local agent config directory: %w", err)
	}
	data, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".local-agent-*.tmp")
	if err != nil {
		return fmt.Errorf("stage local agent descriptor: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	backupPath := ""
	if _, err := os.Stat(path); err == nil {
		suffix, tokenErr := randomToken(8)
		if tokenErr != nil {
			return tokenErr
		}
		backupPath = path + ".stale-" + suffix
		if err := os.Rename(path, backupPath); err != nil {
			return fmt.Errorf("stage previous local agent descriptor: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		if backupPath != "" {
			if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
				_ = os.Rename(backupPath, path)
			}
		}
		return fmt.Errorf("publish local agent descriptor: %w", err)
	}
	if backupPath != "" {
		_ = os.Remove(backupPath)
	}
	return nil
}

func acquireStartupClaim(configDir string) (func(), error) {
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, fmt.Errorf("create local agent config directory: %w", err)
	}
	path := filepath.Join(configDir, startupClaimName)
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, writeErr := fmt.Fprintf(file, "%d %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				_ = os.Remove(path)
				if writeErr != nil {
					return nil, writeErr
				}
				return nil, closeErr
			}
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("claim local agent startup: %w", err)
		}
		info, statErr := os.Stat(path)
		if statErr == nil && time.Since(info.ModTime()) <= startupClaimTTL {
			return nil, errors.New("another ZeitBoard desktop-local agent endpoint is starting")
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale local agent startup claim: %w", removeErr)
		}
	}
	return nil, errors.New("could not claim local agent startup")
}

func descriptorEndpointLive(ctx context.Context, path string) (bool, error) {
	descriptor, err := LoadDescriptor(path)
	if err != nil {
		return false, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, descriptor.Endpoint, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+descriptor.Token)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout: time.Second,
			}).DialContext,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("local agent redirects are not allowed")
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusMethodNotAllowed, nil
}

func removeOwnedDescriptor(path, token string) error {
	descriptor, err := LoadDescriptor(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return nil
	}
	if subtleTokenEqual(descriptor.Token, token) {
		return os.Remove(path)
	}
	return nil
}

func subtleTokenEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	var result byte
	for i := range left {
		result |= left[i] ^ right[i]
	}
	return result == 0
}

func randomToken(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}
