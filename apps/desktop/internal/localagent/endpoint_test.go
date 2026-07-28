package localagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEndpointDescriptorBridgeAndShutdown(t *testing.T) {
	configDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	endpoint, err := Start(ctx, configDir, &fakeCapability{})
	if err != nil {
		t.Fatal(err)
	}
	descriptorPath := filepath.Join(configDir, DescriptorFileName)
	descriptor, err := LoadDescriptor(descriptorPath)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Token == "" || strings.Contains(endpoint.Status().Endpoint, descriptor.Token) {
		t.Fatal("endpoint status must never expose the bearer token")
	}
	if !endpoint.Status().Running {
		t.Fatalf("endpoint not running: %+v", endpoint.Status())
	}

	duplicate, err := Start(ctx, configDir, &fakeCapability{})
	if err == nil {
		_ = duplicate.Close(context.Background())
		t.Fatal("a second endpoint must not replace a live descriptor")
	}
	stillOwned, err := LoadDescriptor(descriptorPath)
	if err != nil {
		t.Fatal(err)
	}
	if stillOwned.Token != descriptor.Token {
		t.Fatal("duplicate startup replaced the live endpoint descriptor")
	}

	invalidBridge, err := NewBridge(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	invalidResponse, invalidEmit, invalidErr := invalidBridge.Forward(nil, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
	if invalidErr != nil || !invalidEmit || invalidBridge.sessionID != "" || !strings.Contains(string(invalidResponse), `"code":-32602`) {
		t.Fatalf("initialize error forwarding: emit=%v session=%q err=%v body=%s", invalidEmit, invalidBridge.sessionID, invalidErr, invalidResponse)
	}
	invalidBridge.Close()

	bridge, err := NewBridge(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	response, emit, err := bridge.Forward(context.Background(), []byte(initializeBody()))
	if err != nil || !emit || !json.Valid(response) || bridge.sessionID == "" {
		t.Fatalf("initialize bridge response: emit=%v err=%v body=%s", emit, err, response)
	}
	response, emit, err = bridge.Forward(context.Background(), []byte(initializeBody()))
	if err != nil || !emit || !strings.Contains(string(response), `"code":-32600`) {
		t.Fatalf("duplicate initialize response: emit=%v err=%v body=%s", emit, err, response)
	}
	response, emit, err = bridge.Forward(context.Background(), []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if err != nil || emit || len(response) != 0 {
		t.Fatalf("notification bridge response: emit=%v err=%v body=%s", emit, err, response)
	}
	response, emit, err = bridge.Forward(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	if err != nil || !emit || !strings.Contains(string(response), "get_overview") {
		t.Fatalf("tools/list bridge response: emit=%v err=%v body=%s", emit, err, response)
	}
	bridge.Close()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := endpoint.Close(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	if err := endpoint.Close(shutdownCtx); err != nil {
		t.Fatalf("second close changed the endpoint result: %v", err)
	}
	if endpoint.Status().Running {
		t.Fatalf("closed endpoint still reports running: %+v", endpoint.Status())
	}
	if _, err := os.Stat(descriptorPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned descriptor remains after shutdown: %v", err)
	}
}

func TestEndpointCanceledShutdownStillClosesListenerAndDescriptor(t *testing.T) {
	configDir := t.TempDir()
	endpoint, err := Start(context.Background(), configDir, &fakeCapability{})
	if err != nil {
		t.Fatal(err)
	}
	descriptorPath := filepath.Join(configDir, DescriptorFileName)
	descriptor, err := LoadDescriptor(descriptorPath)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := endpoint.Close(canceled); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled shutdown error = %v", err)
	}
	if _, err := os.Stat(descriptorPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descriptor remains after canceled shutdown: %v", err)
	}
	bridge, err := NewBridge(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bridge.Forward(context.Background(), []byte(initializeBody())); err == nil {
		t.Fatal("listener still accepted requests after canceled shutdown")
	}
	bridge.Close()
}

func TestDescriptorValidationAndOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), DescriptorFileName)
	descriptor := Descriptor{
		SchemaVersion:   "v1",
		Endpoint:        "http://127.0.0.1:48731/mcp",
		Token:           testToken,
		PID:             42,
		StartedAt:       time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		ProtocolVersion: ProtocolVersion,
	}
	if err := writeDescriptor(path, descriptor); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDescriptor(path)
	if err != nil || loaded.Endpoint != descriptor.Endpoint {
		t.Fatalf("load descriptor: %+v %v", loaded, err)
	}
	if err := removeOwnedDescriptor(path, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("foreign descriptor was removed: %v", err)
	}
	if err := removeOwnedDescriptor(path, descriptor.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned descriptor was not removed: %v", err)
	}

	invalid := descriptor
	invalid.Endpoint = "http://localhost:48731/mcp"
	if err := validateDescriptor(invalid); err == nil {
		t.Fatal("localhost hostname must not replace the literal loopback address")
	}
	invalid = descriptor
	invalid.Endpoint = "http://127.0.0.1:48731/mcp?token=leak"
	if err := validateDescriptor(invalid); err == nil {
		t.Fatal("query-bearing endpoint must be rejected")
	}

	if err := writeDescriptor(path, descriptor); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString(`{"injected":true}`)
	_ = file.Close()
	if _, err := LoadDescriptor(path); err == nil {
		t.Fatal("trailing descriptor JSON must be rejected")
	}
}

func TestBridgeErrorResponsePreservesRequestIDWithoutDetails(t *testing.T) {
	response := bridgeErrorResponse([]byte(`{"jsonrpc":"2.0","id":"request-7","method":"tools/list"}`))
	if !json.Valid(response) || !strings.Contains(string(response), `"id":"request-7"`) {
		t.Fatalf("invalid bridge error response: %s", response)
	}
	if strings.Contains(string(response), "token") || strings.Contains(string(response), "descriptor") {
		t.Fatalf("bridge error leaked implementation details: %s", response)
	}
	if bridgeRequestExpectsResponse([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)) {
		t.Fatal("notification was classified as requiring a response")
	}
	if !bridgeRequestExpectsResponse([]byte(`{"jsonrpc":"2.0","id":0,"method":"ping"}`)) {
		t.Fatal("request id zero was classified as a notification")
	}
}

func TestRunBridgeReturnsWhenCanceledWhileInputIsIdle(t *testing.T) {
	configDir := t.TempDir()
	endpoint, err := Start(context.Background(), configDir, &fakeCapability{})
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close(context.Background())
	descriptorPath := filepath.Join(configDir, DescriptorFileName)
	t.Setenv(DescriptorEnv, descriptorPath)

	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunBridge(ctx, reader, io.Discard) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("canceled idle bridge returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled idle bridge did not return")
	}
}
