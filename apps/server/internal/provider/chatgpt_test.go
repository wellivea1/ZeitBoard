package provider

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"non24.app/server/internal/chatgpt"
	"non24.app/server/internal/chatgpt/chatgpttest"
	"non24.app/server/internal/store"
)

func chatGPTAccount(t *testing.T, fake *chatgpttest.Server) *chatgpt.Account {
	t.Helper()
	st, err := store.Open(":memory:", bytes.Repeat([]byte("k"), 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	account, err := chatgpt.Open(context.Background(), st, chatgpt.Options{
		Endpoints: chatgpt.Endpoints{Issuer: fake.Issuer(), API: fake.API()},
	})
	if err != nil {
		t.Fatal(err)
	}
	return account
}

// The ChatGPT provider is configured while its account is connected with a
// model, and says what is missing otherwise; a failed answer is a reason.
func TestTheChatGPTProviderFollowsItsAccount(t *testing.T) {
	ctx := context.Background()
	fake := chatgpttest.New(t)
	account := chatGPTAccount(t, fake)
	llm := NewChatGPT(account)
	if status := llm.Status(); status.Configured || status.Provider != "chatgpt" || status.Auth != "chatgpt_plan" || status.Problem != ReasonSignInNeeded {
		t.Fatalf("before signing in: %+v", status)
	}
	if _, err := llm.Complete(ctx, Request{System: "s", User: "u"}); !isReason(err, ReasonSignInEnded) {
		t.Errorf("an account never signed in to answered %v", err)
	}

	begun, err := account.BeginSignIn(ctx, "http://127.0.0.1:53682/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	state, code, clientID := fake.Approve(begun.AuthorizationURL)
	if _, err := account.FinishSignIn(ctx, "d", state, code, clientID); err != nil {
		t.Fatal(err)
	}
	if status := llm.Status(); !status.Configured || status.Model != "synthetic-fast" || status.Problem != "" {
		t.Fatalf("after signing in: %+v", status)
	}
	if response, err := llm.Complete(ctx, Request{System: "s", User: "u"}); err != nil || response.Text != fake.Reply {
		t.Fatalf("answered %+v, %v", response, err)
	}

	fake.Lock()
	fake.Status, fake.Body = 429, `{"error":{"code":"subscription_sharing_usage_limit_exceeded"}}`
	fake.Unlock()
	_, err = llm.Complete(ctx, Request{System: "s", User: "u"})
	if !isReason(err, ReasonUsageLimit) || !errors.Is(err, ErrUsageLimit) {
		t.Errorf("a plan limit answered %v", err)
	}
}

func isReason(err error, reason Reason) bool {
	var unavailable Unavailable
	return errors.As(err, &unavailable) && unavailable.Reason == reason
}

// A sign-in fills the provider only when the configuration names none, or
// names ChatGPT.
func TestASignInNeverReplacesAConfiguredProvider(t *testing.T) {
	for name, offers := range map[Name]bool{
		"": true, "disabled": true, "local": true, "chatgpt": true, "ChatGPT": true,
		"openai": false, "anthropic": false, "openrouter": false, "openai-compatible": false,
	} {
		if OffersChatGPT(name) != offers {
			t.Errorf("%q offers a sign-in: %v", name, !offers)
		}
	}
	if _, status, err := New(Config{Name: "chatgpt", Model: "synthetic"}); err != nil || status.Configured {
		t.Errorf("a configuration naming ChatGPT built %+v, %v", status, err)
	}
}
