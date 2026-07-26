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

type Bridge struct {
	descriptor Descriptor
	client     *http.Client
	sessionID  string
}

func NewBridge(descriptor Descriptor) (*Bridge, error) {
	if err := validateDescriptor(descriptor); err != nil {
		return nil, err
	}
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
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("local agent redirects are not allowed")
			},
		},
	}, nil
}

func RunBridge(ctx context.Context, input io.Reader, output io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	path, err := DefaultDescriptorPath()
	if err != nil {
		return err
	}
	descriptor, err := LoadDescriptor(path)
	if err != nil {
		return fmt.Errorf("ZeitBoard desktop-local agent is unavailable; start the desktop app first: %w", err)
	}
	bridge, err := NewBridge(descriptor)
	if err != nil {
		return err
	}
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
			message := append([]byte(nil), bytes.TrimSpace(scanner.Bytes())...)
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
				if bridgeRequestExpectsResponse(line) {
					fallback := bridgeErrorResponse(line)
					if _, writeErr := output.Write(append(fallback, '\n')); writeErr != nil {
						return writeErr
					}
				}
				return err
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
	var parsed rpcRequest
	_ = json.Unmarshal(message, &parsed)
	if parsed.Method == "initialize" && b.sessionID != "" {
		if len(parsed.ID) == 0 {
			return nil, false, nil
		}
		if validRPCID(parsed.ID) {
			response, err := json.Marshal(rpcErrorResponse(parsed.ID, -32600, "MCP session is already initialized"))
			return response, true, err
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.descriptor.Endpoint, bytes.NewReader(message))
	if err != nil {
		return nil, false, err
	}
	b.setHeaders(req)
	if parsed.Method != "initialize" && b.sessionID != "" {
		req.Header.Set(SessionIDHeader, b.sessionID)
		req.Header.Set(ProtocolVersionHeader, ProtocolVersion)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		return nil, false, nil
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
	if parsed.Method == "initialize" {
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
	if b.sessionID == "" {
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
	encoded, err := json.Marshal(rpcErrorResponse(id, -32000, "ZeitBoard desktop-local agent is unavailable. Start the desktop app and retry."))
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
