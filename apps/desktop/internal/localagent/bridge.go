package localagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Bridge carries one MCP client's stdio session to the running desktop app.
//
// The app listens on a new port with a new token each time it starts, and it
// drops a session after thirty idle minutes. A voice client stays connected
// for days, so the bridge outlives both: when the app refuses the session or
// cannot be reached, the bridge re-reads the app's descriptor, opens a new
// session the way the client opened the first, and retries the request once.
// It retries only a request that certainly never ran, so a proposal is never
// made twice.
type Bridge struct {
	descriptor Descriptor
	// load re-reads the running app's descriptor; nil for a fixed one.
	load      func() (Descriptor, error)
	client    *http.Client
	sessionID string
	// initialize is the client's initialize request, and initialized whether
	// the client confirmed it: a new session repeats both.
	initialize  []byte
	initialized bool
}

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// The failures a new session can cure. The request did not run.
var (
	errUnreachable  = errors.New("the ZeitBoard desktop app is not running")
	errUnauthorized = errors.New("the ZeitBoard desktop app refused the bridge's token")
	errSessionGone  = errors.New("the ZeitBoard desktop app no longer has this session")
)

func NewBridge(descriptor Descriptor) (*Bridge, error) {
	if err := validateDescriptor(descriptor); err != nil {
		return nil, err
	}
	return newBridge(descriptor, nil), nil
}

func newBridge(descriptor Descriptor, load func() (Descriptor, error)) *Bridge {
	transport := &http.Transport{
		Proxy:               nil,
		DisableCompression:  true,
		MaxIdleConns:        1,
		MaxIdleConnsPerHost: 1,
		IdleConnTimeout:     30 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	return &Bridge{
		descriptor: descriptor,
		load:       load,
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("local agent redirects are not allowed")
			},
		},
	}
}

// RunBridge serves one client on input and output until input ends or ctx is
// done. It may start before the app does, and it answers while the app is
// down; what went wrong goes to diagnostics, which the client logs.
func RunBridge(ctx context.Context, input io.Reader, output, diagnostics io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	path, err := DefaultDescriptorPath()
	if err != nil {
		return err
	}
	bridge := newBridge(Descriptor{}, func() (Descriptor, error) { return LoadDescriptor(path) })
	defer bridge.Close()

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRequestBytes)
	type scanResult struct {
		message []byte
		err     error
	}
	results := make(chan scanResult)
	go func() {
		defer close(results)
		for scanner.Scan() {
			// A Windows client writing through a UTF-8 console encoding
			// starts its input with a byte-order mark.
			message := append([]byte(nil), bytes.TrimSpace(bytes.TrimPrefix(scanner.Bytes(), utf8BOM))...)
			select {
			case results <- scanResult{message: message}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case results <- scanResult{err: err}:
			case <-ctx.Done():
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case scanned, ok := <-results:
			if !ok {
				return nil
			}
			if scanned.err != nil {
				return scanned.err
			}
			line := scanned.message
			if len(line) == 0 {
				continue
			}
			response, emit, err := bridge.Forward(ctx, line)
			if err != nil {
				// Answer, and keep serving: the app may be starting.
				if diagnostics != nil {
					_, _ = fmt.Fprintln(diagnostics, "zeitboard-local-mcp:", err)
				}
				if !bridgeRequestExpectsResponse(line) {
					continue
				}
				response, emit = bridgeErrorResponse(line), true
			}
			if emit {
				if _, err := output.Write(append(response, '\n')); err != nil {
					return err
				}
			}
		}
	}
}

func (b *Bridge) Forward(ctx context.Context, message []byte) ([]byte, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var parsed rpcRequest
	_ = json.Unmarshal(message, &parsed)
	switch parsed.Method {
	case "initialize":
		if b.sessionID != "" {
			if len(parsed.ID) == 0 {
				return nil, false, nil
			}
			if validRPCID(parsed.ID) {
				response, err := json.Marshal(rpcErrorResponse(parsed.ID, -32600, "MCP session is already initialized"))
				return response, true, err
			}
		}
		b.initialize = append([]byte(nil), message...)
	case "notifications/initialized":
		b.initialized = true
	}
	response, emit, err := b.exchange(ctx, message, parsed.Method)
	if !errors.Is(err, errUnreachable) && !errors.Is(err, errUnauthorized) && !errors.Is(err, errSessionGone) {
		return response, emit, err
	}
	if err := b.reopen(ctx, parsed.Method); err != nil {
		return nil, false, err
	}
	return b.exchange(ctx, message, parsed.Method)
}

// reopen finds the app again and, unless the retried request opens one
// itself, a new session.
func (b *Bridge) reopen(ctx context.Context, method string) error {
	if b.load != nil {
		descriptor, err := b.load()
		if err != nil {
			return fmt.Errorf("%w: %v", errUnreachable, err)
		}
		b.descriptor = descriptor
	}
	b.sessionID = ""
	if method == "initialize" || b.initialize == nil {
		return nil
	}
	if _, _, err := b.exchange(ctx, b.initialize, "initialize"); err != nil {
		return err
	}
	if b.sessionID == "" {
		return errSessionGone
	}
	if b.initialized {
		if _, _, err := b.exchange(ctx, []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`), "notifications/initialized"); err != nil {
			return err
		}
	}
	return nil
}

// exchange sends one message to the app and returns what the client should
// see. A request the app never received, or refused unread, fails with one of
// the curable errors.
func (b *Bridge) exchange(ctx context.Context, message []byte, method string) ([]byte, bool, error) {
	if b.descriptor.Endpoint == "" {
		if b.load == nil {
			return nil, false, errUnreachable
		}
		descriptor, err := b.load()
		if err != nil {
			return nil, false, fmt.Errorf("%w: %v", errUnreachable, err)
		}
		b.descriptor = descriptor
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.descriptor.Endpoint, bytes.NewReader(message))
	if err != nil {
		return nil, false, err
	}
	b.setHeaders(req)
	if method != "initialize" && b.sessionID != "" {
		req.Header.Set(SessionIDHeader, b.sessionID)
		req.Header.Set(ProtocolVersionHeader, ProtocolVersion)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		// Only a refused connection is certain not to have run the request.
		var dial *net.OpError
		if errors.As(err, &dial) && dial.Op == "dial" {
			return nil, false, fmt.Errorf("%w: %v", errUnreachable, err)
		}
		return nil, false, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusAccepted:
		return nil, false, nil
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, false, errUnauthorized
	case resp.StatusCode == http.StatusNotFound && method != "initialize":
		return nil, false, errSessionGone
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRequestBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(body) > maxRequestBytes {
		return nil, false, errors.New("desktop-local agent response exceeded the size limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("desktop-local agent returned HTTP %d", resp.StatusCode)
	}
	body = bytes.TrimSpace(body)
	if !json.Valid(body) {
		return nil, false, errors.New("desktop-local agent returned invalid JSON")
	}
	if method == "initialize" {
		var envelope struct {
			Error *rpcError `json:"error"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, false, errors.New("desktop-local agent returned an invalid initialize response")
		}
		if envelope.Error == nil {
			sessionID := strings.TrimSpace(resp.Header.Get(SessionIDHeader))
			if sessionID == "" {
				return nil, false, errors.New("desktop-local agent did not establish an MCP session")
			}
			b.sessionID = sessionID
		}
	}
	return body, true, nil
}

func (b *Bridge) Close() {
	if b == nil {
		return
	}
	defer b.client.CloseIdleConnections()
	if b.sessionID == "" || b.descriptor.Endpoint == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, b.descriptor.Endpoint, nil)
	if err != nil {
		return
	}
	b.setHeaders(req)
	req.Header.Set(SessionIDHeader, b.sessionID)
	req.Header.Set(ProtocolVersionHeader, ProtocolVersion)
	resp, err := b.client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

func (b *Bridge) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+b.descriptor.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
}

func bridgeErrorResponse(message []byte) []byte {
	var request rpcRequest
	id := json.RawMessage("null")
	if json.Unmarshal(message, &request) == nil && len(request.ID) > 0 {
		id = request.ID
	}
	encoded, err := json.Marshal(rpcErrorResponse(id, -32000, "ZeitBoard is not running. Start the desktop app and ask again."))
	if err != nil {
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32000,"message":"ZeitBoard desktop-local agent is unavailable."}}`)
	}
	return encoded
}

func bridgeRequestExpectsResponse(message []byte) bool {
	var request rpcRequest
	if json.Unmarshal(message, &request) != nil {
		return false
	}
	return len(request.ID) > 0
}
