package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"non24.app/server/internal/assistant"
	"non24.app/server/internal/chatgpt"
	"non24.app/server/internal/chatgpt/chatgpttest"
	"non24.app/server/internal/provider"
)

// withSyntheticChatGPT offers a sign-in to the synthetic OpenAI, as the
// daemon offers one to OpenAI's.
func withSyntheticChatGPT(t *testing.T, fake *chatgpttest.Server) Option {
	return func(s *Server) {
		account, err := chatgpt.Open(context.Background(), s.store, chatgpt.Options{
			Endpoints: chatgpt.Endpoints{Issuer: fake.Issuer(), API: fake.API()},
		})
		if err != nil {
			t.Fatal(err)
		}
		s.chatgpt = account
		s.assistant = assistant.New(provider.NewChatGPT(account), s.store)
	}
}

type chatGPTBody struct {
	ChatGPT struct {
		Offered bool   `json:"offered"`
		State   string `json:"state"`
		Email   string `json:"email"`
		Model   string `json:"model"`
		Problem string `json:"problem"`
	} `json:"chatgpt"`
	Revoked *bool `json:"revoked"`
}

func decodeChatGPT(t *testing.T, status int, body []byte) chatGPTBody {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	var decoded chatGPTBody
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

// Without a sign-in to offer, the status says so and nothing else answers.
func TestAServerNamingAProviderOffersNoSignIn(t *testing.T) {
	h := newTestHarness(t)
	token := h.registerDevice(t, "desktop")
	status, body := h.request(t, http.MethodGet, "/v1/assistant/chatgpt", token, "")
	if got := decodeChatGPT(t, status, body); got.ChatGPT.Offered {
		t.Errorf("offered %s", body)
	}
	if status, _ := h.request(t, http.MethodPost, "/v1/assistant/chatgpt/sign-in", token, `{"redirectUri":"http://127.0.0.1:53682/auth/callback"}`); status != http.StatusNotFound {
		t.Errorf("a sign-in not offered began with %d", status)
	}
	if status, _ := h.request(t, http.MethodGet, "/v1/assistant/chatgpt", "", ""); status != http.StatusUnauthorized {
		t.Errorf("a request without a device token answered %d", status)
	}
}

// A device signs in, chooses a model and disconnects through the server,
// and the assistant answers through the plan meanwhile. No answer carries a
// token, and no refusal reads as the device's own.
func TestADeviceSignsTheAssistantInToChatGPT(t *testing.T) {
	fake := chatgpttest.New(t)
	h := newTestHarness(t, withSyntheticChatGPT(t, fake))
	token := h.registerDevice(t, "desktop")
	get := func(path string) (int, []byte) { return h.request(t, http.MethodGet, path, token, "") }

	status, body := get("/v1/assistant/chatgpt")
	if got := decodeChatGPT(t, status, body); !got.ChatGPT.Offered || got.ChatGPT.State != "disconnected" {
		t.Fatalf("before signing in: %s", body)
	}
	status, body = h.request(t, http.MethodPost, "/v1/assistant/chatgpt/sign-in", token, `{"redirectUri":"http://127.0.0.1:53682/auth/callback"}`)
	if status != http.StatusOK {
		t.Fatalf("sign-in %d: %s", status, body)
	}
	var begun struct {
		SignIn chatgpt.SignIn `json:"signIn"`
	}
	if err := json.Unmarshal(body, &begun); err != nil {
		t.Fatal(err)
	}
	state, code, clientID := fake.Approve(begun.SignIn.AuthorizationURL)

	// An answer to a sign-in not waiting is a conflict, not a refusal of
	// the device.
	status, body = h.request(t, http.MethodPost, "/v1/assistant/chatgpt/finish", token,
		`{"state":"another","code":"`+code+`","clientId":"`+clientID+`"}`)
	if status != http.StatusConflict || !bytes.Contains(body, []byte(`"problem":"sign_in_expired"`)) {
		t.Errorf("a stray answer: %d %s", status, body)
	}
	finish, _ := json.Marshal(map[string]string{"state": state, "code": code, "clientId": clientID})
	status, body = h.request(t, http.MethodPost, "/v1/assistant/chatgpt/finish", token, string(finish))
	connected := decodeChatGPT(t, status, body)
	if connected.ChatGPT.State != "connected" || connected.ChatGPT.Email != "owner@example.com" || connected.ChatGPT.Model != "synthetic-fast" {
		t.Fatalf("after signing in: %s", body)
	}
	refresh := fake.RefreshToken()
	for _, path := range []string{"/v1/assistant/chatgpt", "/v1/status", "/v1/assistant/chatgpt/models"} {
		status, body := get(path)
		if status != http.StatusOK || bytes.Contains(body, []byte(refresh)) || bytes.Contains(body, []byte("access-")) ||
			bytes.Contains(body, []byte("user-synthetic-1")) {
			t.Errorf("%s: %d %s", path, status, body)
		}
	}
	status, body = get("/v1/assistant/chatgpt/models")
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"slug":"synthetic-deep"`)) || bytes.Contains(body, []byte("synthetic-hidden")) {
		t.Errorf("models: %d %s", status, body)
	}
	status, body = h.request(t, http.MethodPut, "/v1/assistant/chatgpt/model", token, `{"model":"synthetic-deep"}`)
	if got := decodeChatGPT(t, status, body); got.ChatGPT.Model != "synthetic-deep" {
		t.Errorf("after choosing a model: %s", body)
	}
	if status, body := h.request(t, http.MethodPut, "/v1/assistant/chatgpt/model", token, `{"model":"synthetic-hidden"}`); status != http.StatusConflict {
		t.Errorf("an unlisted model: %d %s", status, body)
	}

	status, body = h.request(t, http.MethodPost, "/v1/assistant/message", token, assistantRequestBody())
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"answer":"Synthetic answer."`)) ||
		!bytes.Contains(body, []byte(`"provider":"chatgpt"`)) || !bytes.Contains(body, []byte(`"auth":"chatgpt_plan"`)) {
		t.Fatalf("message: %d %s", status, body)
	}
	fake.Lock()
	fake.Status, fake.Body = http.StatusTooManyRequests, `{"error":{"code":"subscription_sharing_usage_limit_exceeded"}}`
	fake.Unlock()
	status, body = h.request(t, http.MethodPost, "/v1/assistant/message", token, assistantRequestBody())
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"reason":"usage_limit"`)) {
		t.Errorf("a plan limit: %d %s", status, body)
	}

	status, body = h.request(t, http.MethodDelete, "/v1/assistant/chatgpt", token, "")
	disconnected := decodeChatGPT(t, status, body)
	if disconnected.ChatGPT.State != "disconnected" || disconnected.Revoked == nil || !*disconnected.Revoked || disconnected.ChatGPT.Email != "" {
		t.Errorf("after disconnecting: %s", body)
	}
	status, body = h.request(t, http.MethodPost, "/v1/assistant/message", token, assistantRequestBody())
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"reason":"sign_in_needed"`)) || !strings.Contains(string(body), "Settings") {
		t.Errorf("a message after disconnecting: %d %s", status, body)
	}
}

// What a device sends is checked before it reaches the sign-in.
func TestASignInRefusesWhatIsNotItsOwn(t *testing.T) {
	fake := chatgpttest.New(t)
	h := newTestHarness(t, withSyntheticChatGPT(t, fake))
	token := h.registerDevice(t, "desktop")
	for name, body := range map[string]string{
		"another host":  `{"redirectUri":"http://192.168.1.2:53682/auth/callback"}`,
		"another path":  `{"redirectUri":"http://127.0.0.1:53682/"}`,
		"unknown field": `{"redirectUri":"http://127.0.0.1:53682/auth/callback","scope":"more"}`,
	} {
		if status, answer := h.request(t, http.MethodPost, "/v1/assistant/chatgpt/sign-in", token, body); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, answer)
		}
	}
	if status, answer := h.request(t, http.MethodGet, "/v1/assistant/chatgpt/models", token, ""); status != http.StatusConflict ||
		!bytes.Contains(answer, []byte(`"problem":"sign_in_ended"`)) {
		t.Errorf("models before signing in: %d %s", status, answer)
	}
}
