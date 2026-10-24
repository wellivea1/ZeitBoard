package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"non24.app/desktop/internal/loopback"
)

// The assistant on the owner's ChatGPT plan (ADR-0066). The server holds the
// sign-in: this computer opens the address the server builds, catches the
// browser's return on a loopback address of its own, and passes the one-time
// code back. The code is of no use without the verifier the server keeps, and
// no token ever reaches this computer.

const (
	// chatGPTSignInTimeout is how long a sign-in waits for the browser.
	chatGPTSignInTimeout = 10 * time.Minute
	// chatGPTServerTimeout covers what the server asks of OpenAI for a
	// sign-in: the discovery document, the exchange, the keys, the models.
	chatGPTServerTimeout = 60 * time.Second
	// assistantMessageTimeout outlasts the 150 seconds the server allows an
	// assistant message, two model calls that may think before answering.
	assistantMessageTimeout = 160 * time.Second
)

// AssistantConnectionDTO is where the assistant's ChatGPT sign-in stands, as
// the server says: never a token.
type AssistantConnectionDTO struct {
	// Reachable is false when there is no server to ask; Message says why.
	Reachable bool `json:"reachable"`
	// Offered is false when the server's configuration names a provider of
	// its own, which a sign-in never replaces.
	Offered bool   `json:"offered"`
	State   string `json:"state,omitempty"`
	Email   string `json:"email,omitempty"`
	Model   string `json:"model,omitempty"`
	Problem string `json:"problem,omitempty"`
	Message string `json:"message,omitempty"`
	// Revoked answers a disconnection: false when ChatGPT did not confirm it
	// withdrew the grant.
	Revoked *bool `json:"revoked,omitempty"`
}

type AssistantModelDTO struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
}

type AssistantModelInput struct {
	Model string `json:"model"`
}

type chatGPTStatusResponse struct {
	SchemaVersion string `json:"schema_version"`
	ChatGPT       struct {
		Offered  bool   `json:"offered"`
		State    string `json:"state"`
		Email    string `json:"email,omitempty"`
		Model    string `json:"model,omitempty"`
		Problem  string `json:"problem,omitempty"`
		UsageURL string `json:"usageUrl"`
	} `json:"chatgpt"`
	Revoked *bool `json:"revoked,omitempty"`
}

func connectionDTO(response chatGPTStatusResponse) AssistantConnectionDTO {
	status := response.ChatGPT
	return AssistantConnectionDTO{
		Reachable: true, Offered: status.Offered, State: status.State, Email: status.Email,
		Model: status.Model, Problem: status.Problem, Revoked: response.Revoked,
	}
}

// GetAssistantConnection reads where the sign-in stands. It makes no call to
// OpenAI, and none at all while sync is off.
func (a *App) GetAssistantConnection() (AssistantConnectionDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		if !cfg.Enabled {
			return AssistantConnectionDTO{Message: "Connect your server above to set up the assistant."}, nil
		}
		return AssistantConnectionDTO{Message: sanitizeBackendError(err)}, nil
	}
	var response chatGPTStatusResponse
	err = a.newPassiveBackendClient(cfg, token).getJSON(a.applicationContext(), "/v1/assistant/chatgpt", &response)
	var refused backendHTTPError
	if errors.As(err, &refused) && refused.StatusCode == http.StatusNotFound {
		// A server from before ADR-0066 offers no sign-in.
		return AssistantConnectionDTO{Reachable: true}, nil
	}
	if err != nil {
		return AssistantConnectionDTO{Message: sanitizeBackendError(err)}, nil
	}
	return connectionDTO(response), nil
}

// ConnectChatGPT signs the assistant in to the owner's ChatGPT plan: it opens
// the server's sign-in in the browser, waits for the browser to come back, and
// passes the answer to the server.
func (a *App) ConnectChatGPT() (AssistantConnectionDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return AssistantConnectionDTO{}, errors.New("Connect your server under Settings › Sync first.")
	}
	ctx, done, err := a.beginChatGPTSignIn()
	if err != nil {
		return AssistantConnectionDTO{}, err
	}
	defer done()
	receiver, err := loopback.Listen("/auth/callback")
	if err != nil {
		return AssistantConnectionDTO{}, asSentence(err)
	}
	defer receiver.Close()

	client := a.newDesktopBackendClient(cfg, token).patient(chatGPTServerTimeout)
	var begun struct {
		SchemaVersion string `json:"schema_version"`
		SignIn        struct {
			AuthorizationURL string `json:"authorizationUrl"`
			State            string `json:"state"`
		} `json:"signIn"`
	}
	if err := client.postJSON(ctx, "/v1/assistant/chatgpt/sign-in", map[string]string{"redirectUri": receiver.Redirect()}, &begun); err != nil {
		return AssistantConnectionDTO{}, chatGPTFailure(err)
	}
	// Only a web address is opened: never a file or a program.
	address, err := url.Parse(begun.SignIn.AuthorizationURL)
	if err != nil || address.Scheme != "https" || address.Host == "" || begun.SignIn.State == "" {
		return AssistantConnectionDTO{}, errors.New("The server's sign-in address could not be used.")
	}
	openSignInPage(a.applicationContext(), address.String())
	waiting, stop := context.WithTimeout(ctx, chatGPTSignInTimeout)
	defer stop()
	answer, err := receiver.Wait(waiting, begun.SignIn.State)
	if err != nil {
		return AssistantConnectionDTO{}, chatGPTSignInStopped(err)
	}
	a.showDesktopWindow()
	var finished chatGPTStatusResponse
	if err := client.postJSON(ctx, "/v1/assistant/chatgpt/finish", map[string]string{
		"state": begun.SignIn.State, "code": answer.Get("code"), "clientId": answer.Get("client_id"),
	}, &finished); err != nil {
		return AssistantConnectionDTO{}, chatGPTFailure(err)
	}
	return connectionDTO(finished), nil
}

// assistantPages are the pages about the owner's ChatGPT plan the window may
// open in the browser, and no others: never an address a server sent.
var assistantPages = map[string]string{
	"usage": "https://chatgpt.com/settings/usage",
	"help":  "https://help.openai.com",
}

// openInBrowser opens a page in the owner's browser; tests see it opened.
var openInBrowser = runtime.BrowserOpenURL

type AssistantPageInput struct {
	Page string `json:"page"`
}

// OpenAssistantPage opens ChatGPT's usage settings, or OpenAI's help.
func (a *App) OpenAssistantPage(input AssistantPageInput) error {
	address, ok := assistantPages[input.Page]
	if !ok {
		return errors.New("That page is not one ZeitBoard opens.")
	}
	openInBrowser(a.applicationContext(), address)
	return nil
}

// CancelChatGPTSignIn stops the sign-in waiting for the browser.
func (a *App) CancelChatGPTSignIn() {
	a.chatGPTSignInMu.Lock()
	defer a.chatGPTSignInMu.Unlock()
	if a.chatGPTSignInCancel != nil {
		a.chatGPTSignInCancel()
	}
}

// ListAssistantModels lists the models the owner's plan offers.
func (a *App) ListAssistantModels() ([]AssistantModelDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return nil, errors.New("Connect your server under Settings › Sync first.")
	}
	var response struct {
		SchemaVersion string              `json:"schema_version"`
		Models        []AssistantModelDTO `json:"models"`
	}
	client := a.newDesktopBackendClient(cfg, token).patient(chatGPTServerTimeout)
	if err := client.getJSON(a.applicationContext(), "/v1/assistant/chatgpt/models", &response); err != nil {
		return nil, chatGPTFailure(err)
	}
	if response.Models == nil {
		response.Models = []AssistantModelDTO{}
	}
	return response.Models, nil
}

// ChooseAssistantModel sets the model the assistant asks.
func (a *App) ChooseAssistantModel(input AssistantModelInput) (AssistantConnectionDTO, error) {
	model := strings.TrimSpace(input.Model)
	if model == "" || len(model) > 200 {
		return AssistantConnectionDTO{}, errors.New("Choose one of your plan's models.")
	}
	return a.chatGPTChange(http.MethodPut, "/v1/assistant/chatgpt/model", map[string]string{"model": model})
}

// DisconnectChatGPT withdraws the assistant's ChatGPT sign-in.
func (a *App) DisconnectChatGPT() (AssistantConnectionDTO, error) {
	return a.chatGPTChange(http.MethodDelete, "/v1/assistant/chatgpt", nil)
}

func (a *App) chatGPTChange(method, path string, payload any) (AssistantConnectionDTO, error) {
	cfg, token, err := a.requireBackendSync()
	if err != nil {
		return AssistantConnectionDTO{}, errors.New("Connect your server under Settings › Sync first.")
	}
	var body []byte
	if payload != nil {
		if body, err = json.Marshal(payload); err != nil {
			return AssistantConnectionDTO{}, err
		}
	}
	var response chatGPTStatusResponse
	client := a.newDesktopBackendClient(cfg, token).patient(chatGPTServerTimeout)
	if err := client.doJSON(a.applicationContext(), method, path, body, &response); err != nil {
		return AssistantConnectionDTO{}, chatGPTFailure(err)
	}
	return connectionDTO(response), nil
}

// beginChatGPTSignIn holds the one sign-in that may wait at a time, until it
// is done or cancelled.
func (a *App) beginChatGPTSignIn() (context.Context, func(), error) {
	a.chatGPTSignInMu.Lock()
	defer a.chatGPTSignInMu.Unlock()
	if a.chatGPTSignInCancel != nil {
		return nil, nil, errors.New("ZeitBoard is already signing in to ChatGPT. Finish or cancel that first.")
	}
	ctx, cancel := context.WithCancel(a.applicationContext())
	a.chatGPTSignInCancel = cancel
	return ctx, func() {
		a.chatGPTSignInMu.Lock()
		a.chatGPTSignInCancel = nil
		a.chatGPTSignInMu.Unlock()
		cancel()
	}, nil
}

// chatGPTSignInStopped words why waiting for the browser stopped.
func chatGPTSignInStopped(err error) error {
	var refused loopback.Refused
	switch {
	case errors.As(err, &refused) && refused.Code == "access_denied":
		return errors.New("ChatGPT was not allowed in the browser, so nothing changed.")
	case errors.As(err, &refused):
		return fmt.Errorf("Signing in did not finish: %s.", refused.Code)
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("Signing in waited ten minutes for the browser and stopped. Try again when you are ready.")
	case errors.Is(err, context.Canceled):
		return errors.New("Signing in was cancelled.")
	default:
		return asSentence(err)
	}
}

// chatGPTProblems words what the server says went wrong with ChatGPT.
var chatGPTProblems = map[string]string{
	"usage_limit":       "You have reached your ChatGPT plan's limit for now.",
	"busy":              "ChatGPT is not answering right now. Try again in a few minutes.",
	"sign_in_ended":     "ChatGPT did not accept the sign-in. Sign in again.",
	"plan_not_shared":   "ChatGPT did not share your plan with ZeitBoard, so it was not connected. Sign in again and allow ZeitBoard to use your plan.",
	"not_eligible":      "This ChatGPT account cannot share its plan with apps. It needs Plus or Pro.",
	"not_enabled":       "OpenAI has not turned on plan usage for this sign-in. Your server can use an API key instead.",
	"model_unavailable": "Your plan does not offer that model. Choose another.",
	"account_mismatch":  "That is a different ChatGPT account from the one the assistant used. Sign in to that one, or disconnect first to switch.",
	"sign_in_expired":   "That sign-in waited too long, or another began since. Try again.",
	"refused":           "ChatGPT refused the sign-in. Try again.",
}

// chatGPTFailure words a failed request to the server's ChatGPT routes.
func chatGPTFailure(err error) error {
	var refused backendHTTPError
	if !errors.As(err, &refused) {
		if errors.Is(err, context.Canceled) {
			return errors.New("Signing in was cancelled.")
		}
		return asSentence(err)
	}
	var body struct {
		Problem string `json:"problem"`
	}
	if json.Unmarshal([]byte(refused.Message), &body) == nil {
		if sentence, ok := chatGPTProblems[body.Problem]; ok {
			return errors.New(sentence)
		}
	}
	if refused.StatusCode == http.StatusNotFound {
		return errors.New("Your server does not offer signing in to ChatGPT. Update it, or set an API key on it.")
	}
	return asSentence(err)
}
