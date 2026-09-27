package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigurationDecidesTheProvider(t *testing.T) {
	// A hosted provider without a key is off, as before.
	for _, name := range []Name{OpenAI, Anthropic, OpenRouter, "local", ""} {
		if _, status, err := New(Config{Name: name, Model: "m"}); err != nil || status.Configured {
			t.Errorf("%q without a key: configured %v, %v", name, status.Configured, err)
		}
	}
	// A model the operator runs needs no key, but it needs an endpoint.
	_, status, err := New(Config{Name: "openai-compatible", Model: "local-model", Endpoint: "http://127.0.0.1:11434/v1/chat/completions"})
	if err != nil || !status.Configured || status.Provider != "openai_compatible" || status.Model != "local-model" {
		t.Fatalf("keyless local server: %+v, %v", status, err)
	}
	if _, _, err := New(Config{Name: OpenAICompatible, Model: "local-model"}); err == nil {
		t.Fatal("an OpenAI-compatible provider without an endpoint was accepted")
	}
	if _, _, err := New(Config{Name: OpenAICompatible, Endpoint: "http://127.0.0.1:1/v1"}); err == nil {
		t.Fatal("a provider without a model was accepted")
	}
}

// The assistant's context crosses the network only encrypted.
func TestAnEndpointIsHTTPSOrOnThisMachine(t *testing.T) {
	for _, endpoint := range []string{
		"https://models.example/v1/chat/completions",
		"http://127.0.0.1:11434/v1/chat/completions",
		"http://localhost:1234/v1/chat/completions",
		"http://[::1]:8080/v1/chat/completions",
	} {
		if _, _, err := New(Config{Name: OpenAICompatible, Model: "m", Endpoint: endpoint}); err != nil {
			t.Errorf("%s: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{
		"http://192.168.1.20:11434/v1/chat/completions",
		"http://models.example/v1/chat/completions",
		"ftp://127.0.0.1/v1",
		"https://user:secret@models.example/v1",
		"/v1/chat/completions",
	} {
		if _, _, err := New(Config{Name: OpenAICompatible, Model: "m", Endpoint: endpoint}); err == nil {
			t.Errorf("%s was accepted", endpoint)
		}
		if _, _, err := New(Config{Name: OpenAI, Model: "m", APIKey: "k", Endpoint: endpoint}); err == nil {
			t.Errorf("%s was accepted for a hosted provider", endpoint)
		}
	}
}

func TestAnOpenAICompatibleServerIsAskedForJSON(t *testing.T) {
	for _, key := range []string{"", "operator-key"} {
		var seen struct {
			authorization string
			body          map[string]any
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen.authorization = r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&seen.body)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"schema_version\":\"v1\"}"}}]}`))
		}))
		llm, _, err := New(Config{Name: OpenAICompatible, Model: "local-model", APIKey: key, Endpoint: server.URL + "/v1/chat/completions"})
		if err != nil {
			t.Fatal(err)
		}
		response, err := llm.Complete(context.Background(), Request{System: "system", User: "user"})
		server.Close()
		if err != nil || !strings.Contains(response.Text, "schema_version") {
			t.Fatalf("response = %+v, %v", response, err)
		}
		if want := map[string]string{"": "", "operator-key": "Bearer operator-key"}[key]; seen.authorization != want {
			t.Errorf("key %q sent authorization %q", key, seen.authorization)
		}
		format, _ := seen.body["response_format"].(map[string]any)
		if seen.body["model"] != "local-model" || format["type"] != "json_object" {
			t.Errorf("request body = %v", seen.body)
		}
	}
}
