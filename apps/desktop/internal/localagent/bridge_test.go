package localagent

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// app is one run of the desktop app: an endpoint with its own port and token.
type app struct {
	handler    *Handler
	capability *fakeCapability
	server     *httptest.Server
	descriptor Descriptor
}

func startApp(t *testing.T, token string) *app {
	t.Helper()
	capability := &fakeCapability{}
	handler, err := NewHandler(token, capability)
	if err != nil {
		t.Fatal(err)
	}
	running := &app{handler: handler, capability: capability, server: httptest.NewServer(handler)}
	t.Cleanup(running.server.Close)
	running.descriptor = Descriptor{
		SchemaVersion: "v1", Endpoint: running.server.URL + "/mcp", Token: token,
		PID: 4242, StartedAt: time.Now().UTC(), ProtocolVersion: ProtocolVersion,
	}
	return running
}

func openedBridge(t *testing.T, load func() (Descriptor, error)) *Bridge {
	t.Helper()
	bridge := newBridge(Descriptor{}, load)
	t.Cleanup(bridge.Close)
	for _, message := range []string{initializeBody(), `{"jsonrpc":"2.0","method":"notifications/initialized"}`} {
		if _, _, err := bridge.Forward(context.Background(), []byte(message)); err != nil {
			t.Fatal(err)
		}
	}
	return bridge
}

const statusCall = `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_status","arguments":{}}}`

func TestABridgeOutlivesAnIdleSession(t *testing.T) {
	running := startApp(t, testToken)
	bridge := openedBridge(t, func() (Descriptor, error) { return running.descriptor, nil })
	first := bridge.sessionID
	// Thirty-one idle minutes later, the app has dropped the session.
	later := time.Now().UTC().Add(31 * time.Minute)
	running.handler.now = func() time.Time { return later }

	response, emit, err := bridge.Forward(context.Background(), []byte(statusCall))
	if err != nil || !emit || !strings.Contains(string(response), `"id":7`) || strings.Contains(string(response), `"error"`) {
		t.Fatalf("after the session lapsed: emit=%v err=%v body=%s", emit, err, response)
	}
	if bridge.sessionID == "" || bridge.sessionID == first {
		t.Fatal("the bridge did not open a new session")
	}
	if calls := running.capability.callCount(); calls != 1 {
		t.Fatalf("the call ran %d times; it should run once, in the new session", calls)
	}
}

func TestABridgeFollowsTheAppAcrossARestart(t *testing.T) {
	var mu sync.Mutex
	current := startApp(t, testToken)
	load := func() (Descriptor, error) {
		mu.Lock()
		defer mu.Unlock()
		return current.descriptor, nil
	}
	bridge := openedBridge(t, load)

	// The app restarts: a new port and a new token.
	current.server.Close()
	mu.Lock()
	current = startApp(t, strings.Repeat("b", 64))
	mu.Unlock()

	response, emit, err := bridge.Forward(context.Background(), []byte(statusCall))
	if err != nil || !emit || strings.Contains(string(response), `"error"`) {
		t.Fatalf("after a restart: emit=%v err=%v body=%s", emit, err, response)
	}
	if calls := current.capability.callCount(); calls != 1 || bridge.descriptor.Endpoint != current.descriptor.Endpoint {
		t.Fatalf("the call reached %s (%d calls), not the restarted app", bridge.descriptor.Endpoint, calls)
	}
}

// A request that may have run is never sent twice: only a refused connection,
// token or session is retried.
func TestABridgeNeverRetriesARequestThatMayHaveRun(t *testing.T) {
	running := startApp(t, testToken)
	bridge := openedBridge(t, func() (Descriptor, error) { return running.descriptor, nil })
	var served atomic.Int32
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served.Add(1)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer failing.Close()
	bridge.descriptor.Endpoint = failing.URL + "/mcp"
	if _, _, err := bridge.Forward(context.Background(), []byte(statusCall)); err == nil || served.Load() != 1 {
		t.Fatalf("a failed call was retried: served %d times, err %v", served.Load(), err)
	}
}

// A Windows client writing through a UTF-8 console encoding starts its input
// with a byte-order mark; PowerShell does.
func TestRunBridgeReadsInputThatStartsWithAByteOrderMark(t *testing.T) {
	configDir := t.TempDir()
	endpoint, err := Start(context.Background(), configDir, &fakeCapability{})
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close(context.Background())
	t.Setenv(DescriptorEnv, filepath.Join(configDir, DescriptorFileName))
	var output strings.Builder
	input := strings.NewReader(string(utf8BOM) + initializeBody() + "\n")
	if err := RunBridge(context.Background(), input, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"protocolVersion"`) {
		t.Fatalf("initialize after a byte-order mark: %s", output.String())
	}
}

// The client may start first, and the app may stop and start again: the bridge
// answers meanwhile and never exits.
func TestRunBridgeAnswersWhileTheAppIsDownAndCarriesOn(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv(DescriptorEnv, filepath.Join(configDir, DescriptorFileName))
	input, feed := io.Pipe()
	output, results := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunBridge(ctx, input, results, io.Discard) }()
	lines := bufio.NewScanner(output)
	ask := func(message string) string {
		t.Helper()
		if _, err := io.WriteString(feed, message+"\n"); err != nil {
			t.Fatal(err)
		}
		if !lines.Scan() {
			t.Fatalf("the bridge stopped answering: %v", <-done)
		}
		return lines.Text()
	}

	if answer := ask(initializeBody()); !strings.Contains(answer, "ZeitBoard is not running") {
		t.Fatalf("with the app down: %s", answer)
	}
	endpoint, err := Start(ctx, configDir, &fakeCapability{})
	if err != nil {
		t.Fatal(err)
	}
	if answer := ask(strings.Replace(initializeBody(), `"id":1`, `"id":2`, 1)); !strings.Contains(answer, `"protocolVersion"`) {
		t.Fatalf("once the app started: %s", answer)
	}
	if _, err := io.WriteString(feed, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	if answer := ask(statusCall); strings.Contains(answer, `"error"`) {
		t.Fatalf("a call once the app started: %s", answer)
	}
	if err := endpoint.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if answer := ask(strings.Replace(statusCall, `"id":7`, `"id":8`, 1)); !strings.Contains(answer, "ZeitBoard is not running") {
		t.Fatalf("after the app stopped: %s", answer)
	}
	restarted, err := Start(ctx, configDir, &fakeCapability{})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close(context.Background())
	if answer := ask(strings.Replace(statusCall, `"id":7`, `"id":9`, 1)); strings.Contains(answer, `"error"`) {
		t.Fatalf("after the app restarted: %s", answer)
	}
	feed.Close()
	if err := <-done; err != nil {
		t.Fatalf("the bridge ended with %v", err)
	}
}
