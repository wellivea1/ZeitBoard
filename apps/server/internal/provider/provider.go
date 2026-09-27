package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Name string

const (
	Disabled    Name = "disabled"
	OpenAI      Name = "openai"
	Anthropic   Name = "anthropic"
	OpenRouter  Name = "openrouter"
	OpenCodeZen Name = "opencode_zen"
	// OpenAICompatible is any server speaking OpenAI's chat completions, such
	// as a model the operator runs themselves. It needs an endpoint; a key only
	// if that server asks for one.
	OpenAICompatible Name = "openai_compatible"
)

var (
	ErrDisabled        = errors.New("provider disabled")
	ErrUsageLimit      = errors.New("provider usage limit")
	ErrContextTooLarge = errors.New("provider context too large")
)

type Config struct {
	Name     Name
	Model    string
	APIKey   string
	Endpoint string
}

type Status struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	Model      string `json:"model,omitempty"`
}

type Request struct {
	System  string
	User    string
	Context json.RawMessage
	Schema  json.RawMessage
}

type Response struct {
	Text string
}

type LLM interface {
	Complete(context.Context, Request) (Response, error)
	Status() Status
}

type DisabledClient struct{}

func (DisabledClient) Complete(context.Context, Request) (Response, error) {
	return Response{}, ErrDisabled
}

func (DisabledClient) Status() Status {
	return Status{Provider: string(Disabled), Configured: false}
}

func New(cfg Config) (LLM, Status, error) {
	name := normalizeName(cfg.Name)
	if name == Disabled || (cfg.APIKey == "" && name != OpenAICompatible) {
		client := DisabledClient{}
		return client, client.Status(), nil
	}
	if cfg.Model == "" {
		return nil, Status{}, fmt.Errorf("model is required for provider %s", name)
	}
	if err := checkEndpoint(cfg.Endpoint); err != nil {
		return nil, Status{}, err
	}
	base := httpClient{
		name:     name,
		model:    cfg.Model,
		apiKey:   cfg.APIKey,
		endpoint: strings.TrimSpace(cfg.Endpoint),
		client:   &http.Client{Timeout: 30 * time.Second},
	}
	var llm LLM
	switch name {
	case OpenAI:
		llm = openAIClient{httpClient: base.withDefaultEndpoint("https://api.openai.com/v1/responses")}
	case Anthropic:
		llm = anthropicClient{httpClient: base.withDefaultEndpoint("https://api.anthropic.com/v1/messages")}
	case OpenRouter:
		llm = chatCompletionsClient{httpClient: base.withDefaultEndpoint("https://openrouter.ai/api/v1/chat/completions"), providerName: OpenRouter}
	case OpenCodeZen:
		if base.endpoint == "" {
			return nil, Status{}, errors.New("OpenCode Zen provider requires an endpoint")
		}
		llm = chatCompletionsClient{httpClient: base, providerName: OpenCodeZen}
	case OpenAICompatible:
		if base.endpoint == "" {
			return nil, Status{}, errors.New("an OpenAI-compatible provider requires an endpoint")
		}
		llm = chatCompletionsClient{httpClient: base, providerName: OpenAICompatible}
	default:
		return nil, Status{}, fmt.Errorf("unsupported provider %q", name)
	}
	return llm, llm.Status(), nil
}

func normalizeName(name Name) Name {
	switch strings.ToLower(strings.TrimSpace(string(name))) {
	case "", "disabled", "local":
		return Disabled
	case "openai":
		return OpenAI
	case "anthropic":
		return Anthropic
	case "openrouter":
		return OpenRouter
	case "opencode_zen", "opencode-zen", "zen":
		return OpenCodeZen
	case "openai_compatible", "openai-compatible":
		return OpenAICompatible
	default:
		return Name(strings.ToLower(strings.TrimSpace(string(name))))
	}
}

type httpClient struct {
	name     Name
	model    string
	apiKey   string
	endpoint string
	client   *http.Client
}

func (c httpClient) withDefaultEndpoint(endpoint string) httpClient {
	if c.endpoint == "" {
		c.endpoint = endpoint
	}
	return c
}

// Status of a constructed client: the disabled client stands in for any
// provider that is not configured.
func (c httpClient) Status() Status {
	return Status{Configured: true, Provider: string(c.name), Model: c.model}
}

// checkEndpoint keeps the assistant's context off the network in the clear:
// an endpoint is HTTPS, or plain HTTP only to this machine.
func checkEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return errors.New("assistant endpoint must be an absolute URL without credentials")
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		host := parsed.Hostname()
		if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return nil
		}
		return errors.New("assistant endpoint must use https unless it is on this machine")
	default:
		return errors.New("assistant endpoint must use https")
	}
}

func (c httpClient) doJSON(ctx context.Context, req *http.Request, target any) error {
	resp, err := c.client.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrUsageLimit
	}
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		return ErrContextTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if strings.Contains(strings.ToLower(string(body)), "context") && strings.Contains(strings.ToLower(string(body)), "too") {
			return ErrContextTooLarge
		}
		return fmt.Errorf("provider %s returned HTTP %d", c.name, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func jsonRequest(method, url string, body any) (*http.Request, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func prompt(req Request) string {
	return strings.Join([]string{
		req.User,
		"",
		"Redacted context JSON:",
		string(req.Context),
		"",
		"Return only JSON matching this schema:",
		string(req.Schema),
	}, "\n")
}
