package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type BackendClient struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func (c *BackendClient) Available(ctx context.Context) error {
	_, err := c.get(ctx, "/v1/status")
	return err
}

func (c *BackendClient) Get(ctx context.Context, path string) (json.RawMessage, error) {
	return c.get(ctx, path)
}

func (c *BackendClient) Post(ctx context.Context, path string, payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	return c.do(ctx, http.MethodPost, path, payload)
}

func (c *BackendClient) get(ctx context.Context, path string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *BackendClient) do(ctx context.Context, method, path string, payload json.RawMessage) (json.RawMessage, error) {
	if c == nil || c.BaseURL == "" || c.Token == "" {
		return nil, errors.New("backend is not configured")
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	var body io.Reader
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backend request failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read backend response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(data))
		if len(message) > 160 {
			message = message[:160]
		}
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("backend returned HTTP %d: %s", resp.StatusCode, message)
	}
	if len(data) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New("backend returned non-JSON response")
	}
	compact := bytes.Buffer{}
	if err := json.Compact(&compact, data); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), compact.Bytes()...), nil
}
