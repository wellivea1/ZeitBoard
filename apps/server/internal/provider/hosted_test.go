package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// hostedStandIn plays a hosted provider on this machine: it records what it
// was asked and answers with a status and a body.
type hostedStandIn struct {
	server  *httptest.Server
	headers http.Header
	body    map[string]any
}

func newHostedStandIn(t *testing.T, status int, answer string) *hostedStandIn {
	t.Helper()
	stand := &hostedStandIn{}
	stand.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stand.headers = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&stand.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(stand.server.Close)
	return stand
}

func hosted(t *testing.T, name Name, stand *hostedStandIn) LLM {
	t.Helper()
	llm, status, err := New(Config{Name: name, Model: "synthetic-model", APIKey: "synthetic-key", Endpoint: stand.server.URL})
	if err != nil || !status.Configured || status.Provider != string(name) || status.Model != "synthetic-model" {
		t.Fatalf("%s: %+v, %v", name, status, err)
	}
	return llm
}

var hostedRequest = Request{System: "Synthetic system prompt.", User: "Synthetic question.",
	Context: json.RawMessage(`{"tasks":[]}`), Schema: json.RawMessage(`{"type":"object"}`)}

// OpenAI is asked through the Responses API to keep nothing, with the key as
// a bearer token, and its text is read from either place it may put it.
func TestOpenAIIsAskedToKeepNothing(t *testing.T) {
	for answer, want := range map[string]string{
		`{"output_text":"synthetic answer"}`:                                          "synthetic answer",
		`{"output":[{"content":[{"type":"output_text","text":"synthetic answer"}]}]}`: "synthetic answer",
	} {
		stand := newHostedStandIn(t, http.StatusOK, answer)
		response, err := hosted(t, OpenAI, stand).Complete(context.Background(), hostedRequest)
		if err != nil || response.Text != want {
			t.Fatalf("%s read as %+v, %v", answer, response, err)
		}
		if stand.headers.Get("Authorization") != "Bearer synthetic-key" || stand.body["store"] != false ||
			stand.body["model"] != "synthetic-model" {
			t.Errorf("asked with %v and %v", stand.headers, stand.body)
		}
		input, _ := stand.body["input"].([]any)
		if len(input) != 2 || input[0].(map[string]any)["role"] != "system" ||
			!strings.Contains(input[1].(map[string]any)["content"].(string), `{"tasks":[]}`) {
			t.Errorf("input %v", stand.body["input"])
		}
	}
	empty := newHostedStandIn(t, http.StatusOK, `{"output":[]}`)
	if _, err := hosted(t, OpenAI, empty).Complete(context.Background(), hostedRequest); err == nil {
		t.Error("an answer without text was read as one")
	}
}

// Anthropic is asked through the Messages API with its key and version
// headers, and its text is read from the first block that has some.
func TestAnthropicIsAskedWithItsHeaders(t *testing.T) {
	stand := newHostedStandIn(t, http.StatusOK, `{"content":[{"type":"thinking","text":""},{"type":"text","text":"synthetic answer"}]}`)
	response, err := hosted(t, Anthropic, stand).Complete(context.Background(), hostedRequest)
	if err != nil || response.Text != "synthetic answer" {
		t.Fatalf("read as %+v, %v", response, err)
	}
	if stand.headers.Get("x-api-key") != "synthetic-key" || stand.headers.Get("anthropic-version") == "" ||
		stand.headers.Get("Authorization") != "" {
		t.Errorf("headers %v", stand.headers)
	}
	messages, _ := stand.body["messages"].([]any)
	if stand.body["system"] != hostedRequest.System || len(messages) != 1 || stand.body["max_tokens"] == nil {
		t.Errorf("body %v", stand.body)
	}
	empty := newHostedStandIn(t, http.StatusOK, `{"content":[]}`)
	if _, err := hosted(t, Anthropic, empty).Complete(context.Background(), hostedRequest); err == nil {
		t.Error("an answer without text was read as one")
	}
}

// A hosted provider's refusals read as what the assistant acts on: a usage
// limit, a context too large, or a failure that names only the status.
func TestAHostedRefusalReadsAsItsKind(t *testing.T) {
	for _, name := range []Name{OpenAI, Anthropic} {
		for _, tc := range []struct {
			status int
			body   string
			want   error
		}{
			{http.StatusTooManyRequests, `{"error":{"message":"synthetic secret text"}}`, ErrUsageLimit},
			{http.StatusRequestEntityTooLarge, `{}`, ErrContextTooLarge},
			{http.StatusBadRequest, `{"error":{"message":"The context is too long for this model"}}`, ErrContextTooLarge},
		} {
			stand := newHostedStandIn(t, tc.status, tc.body)
			if _, err := hosted(t, name, stand).Complete(context.Background(), hostedRequest); !errors.Is(err, tc.want) {
				t.Errorf("%s answering %d read as %v", name, tc.status, err)
			}
		}
		stand := newHostedStandIn(t, http.StatusInternalServerError, `{"error":{"message":"synthetic secret text"}}`)
		_, err := hosted(t, name, stand).Complete(context.Background(), hostedRequest)
		if err == nil || strings.Contains(err.Error(), "synthetic secret text") || !strings.Contains(err.Error(), "500") {
			t.Errorf("%s answering 500 read as %v", name, err)
		}
	}
}
