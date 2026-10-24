package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// chatGPTBackend plays the server's side of ADR-0066: it hands out a sign-in
// address and takes the browser's answer back, holding nothing of OpenAI's.
type chatGPTBackend struct {
	t        *testing.T
	server   *httptest.Server
	mu       sync.Mutex
	redirect string
	finished map[string]string
	// refuse, when set, is the problem a finish answers with.
	refuse string
	// address, when set, replaces the sign-in address handed out.
	address  string
	offered  bool
	revoked  bool
	missing  bool
	paths    []string
	statuses []string
}

func newChatGPTBackend(t *testing.T) *chatGPTBackend {
	b := &chatGPTBackend{t: t, offered: true, revoked: true}
	b.server = httptest.NewTLSServer(http.HandlerFunc(b.serve))
	t.Cleanup(b.server.Close)
	return b
}

const syntheticSignInState = "state-synthetic-1"

func (b *chatGPTBackend) status(state string) map[string]any {
	status := map[string]any{"offered": b.offered, "state": state, "usageUrl": "https://chatgpt.com/settings/usage"}
	if state == "connected" {
		status["email"], status["model"] = "owner@example.com", "synthetic-fast"
	}
	return map[string]any{"schema_version": "v1", "chatgpt": status}
}

func (b *chatGPTBackend) serve(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.paths = append(b.paths, r.Method+" "+r.URL.Path)
	write := func(status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if b.missing && strings.HasPrefix(r.URL.Path, "/v1/assistant/chatgpt") {
		http.NotFound(w, r)
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "POST /v1/devices":
		write(http.StatusCreated, registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "assistant-token"})
	case "GET /v1/assistant/chatgpt":
		write(http.StatusOK, b.status("disconnected"))
	case "POST /v1/assistant/chatgpt/sign-in":
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		b.redirect = body["redirectUri"]
		address := b.address
		if address == "" {
			address = "https://auth.example.test/oauth/authorize?" + url.Values{
				"redirect_uri": {b.redirect}, "state": {syntheticSignInState},
			}.Encode()
		}
		write(http.StatusOK, map[string]any{"schema_version": "v1",
			"signIn": map[string]string{"authorizationUrl": address, "state": syntheticSignInState}})
	case "POST /v1/assistant/chatgpt/finish":
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &b.finished)
		if b.refuse != "" {
			write(http.StatusConflict, map[string]string{"error": "ChatGPT: " + b.refuse, "problem": b.refuse})
			return
		}
		write(http.StatusOK, b.status("connected"))
	case "GET /v1/assistant/chatgpt/models":
		write(http.StatusOK, map[string]any{"schema_version": "v1", "models": []map[string]string{
			{"slug": "synthetic-fast", "displayName": "Synthetic Fast"}, {"slug": "synthetic-deep", "displayName": "Synthetic Deep"},
		}})
	case "PUT /v1/assistant/chatgpt/model":
		write(http.StatusOK, b.status("connected"))
	case "DELETE /v1/assistant/chatgpt":
		response := b.status("disconnected")
		response["revoked"] = b.revoked
		write(http.StatusOK, response)
	default:
		http.NotFound(w, r)
	}
}

// browser answers the sign-in as the owner would: it opens the address and
// comes back to ZeitBoard's loopback address with a code, or with refusal.
func (b *chatGPTBackend) browser(refusal string) func(context.Context, string) {
	return func(_ context.Context, address string) {
		page, err := url.Parse(address)
		if err != nil || page.Scheme != "https" {
			b.t.Errorf("opened %q", address)
			return
		}
		answer := url.Values{"state": {page.Query().Get("state")}}
		if refusal != "" {
			answer.Set("error", refusal)
		} else {
			answer.Set("code", "code-synthetic")
			answer.Set("client_id", "client-synthetic")
		}
		go func() {
			response, err := http.Get(page.Query().Get("redirect_uri") + "?" + answer.Encode())
			if err != nil {
				b.t.Error(err)
				return
			}
			response.Body.Close()
		}()
	}
}

func withBrowser(t *testing.T, browser func(context.Context, string)) {
	opened := openSignInPage
	openSignInPage = browser
	t.Cleanup(func() { openSignInPage = opened })
}

// The computer opens the server's sign-in, catches the browser's return on
// its own loopback address, and hands the server only what the browser
// brought.
func TestTheAssistantSignsInThroughTheServer(t *testing.T) {
	app := newTestApp(t)
	backend := newChatGPTBackend(t)
	withBrowser(t, backend.browser(""))
	configureBackendForTest(t, app, backend.server.URL)

	before, err := app.GetAssistantConnection()
	if err != nil || !before.Reachable || !before.Offered || before.State != "disconnected" {
		t.Fatalf("before signing in: %+v, %v", before, err)
	}
	connected, err := app.ConnectChatGPT()
	if err != nil {
		t.Fatal(err)
	}
	if connected.State != "connected" || connected.Email != "owner@example.com" || connected.Model != "synthetic-fast" {
		t.Fatalf("after signing in: %+v", connected)
	}
	backend.mu.Lock()
	redirect, finished := backend.redirect, backend.finished
	backend.mu.Unlock()
	if !regexp.MustCompile(`^http://127\.0\.0\.1:\d+/auth/callback$`).MatchString(redirect) {
		t.Errorf("redirect %q", redirect)
	}
	if finished["state"] != syntheticSignInState || finished["code"] != "code-synthetic" ||
		finished["clientId"] != "client-synthetic" || len(finished) != 3 {
		t.Errorf("handed the server %v", finished)
	}

	models, err := app.ListAssistantModels()
	if err != nil || len(models) != 2 || models[1].DisplayName != "Synthetic Deep" {
		t.Errorf("models %+v, %v", models, err)
	}
	if _, err := app.ChooseAssistantModel(AssistantModelInput{Model: " "}); err == nil {
		t.Error("an empty model was chosen")
	}
	if chosen, err := app.ChooseAssistantModel(AssistantModelInput{Model: "synthetic-deep"}); err != nil || chosen.State != "connected" {
		t.Errorf("chose %+v, %v", chosen, err)
	}
	disconnected, err := app.DisconnectChatGPT()
	if err != nil || disconnected.State != "disconnected" || disconnected.Revoked == nil || !*disconnected.Revoked {
		t.Errorf("disconnected %+v, %v", disconnected, err)
	}
}

// What can go wrong reads as what to do, and only a web address is opened.
func TestASignInThatDoesNotFinishSaysWhy(t *testing.T) {
	app := newTestApp(t)
	backend := newChatGPTBackend(t)
	configureBackendForTest(t, app, backend.server.URL)

	withBrowser(t, backend.browser("access_denied"))
	if _, err := app.ConnectChatGPT(); err == nil || err.Error() != "ChatGPT was not allowed in the browser, so nothing changed." {
		t.Errorf("a refusal read as %v", err)
	}
	backend.mu.Lock()
	finished := backend.finished
	backend.mu.Unlock()
	if finished != nil {
		t.Errorf("a refused sign-in reached the server: %v", finished)
	}

	withBrowser(t, backend.browser(""))
	backend.mu.Lock()
	backend.refuse = "plan_not_shared"
	backend.mu.Unlock()
	if _, err := app.ConnectChatGPT(); err == nil || !strings.HasPrefix(err.Error(), "ChatGPT did not share your plan with ZeitBoard") {
		t.Errorf("a plan not shared read as %v", err)
	}

	backend.mu.Lock()
	backend.refuse, backend.address = "", "file:///C:/Windows/System32/calc.exe"
	backend.mu.Unlock()
	withBrowser(t, func(context.Context, string) { t.Error("a file address was opened") })
	if _, err := app.ConnectChatGPT(); err == nil || err.Error() != "The server's sign-in address could not be used." {
		t.Errorf("a file address read as %v", err)
	}
}

// One sign-in waits at a time, and cancelling it stops the wait.
func TestASignInWaitingForTheBrowserCanBeCancelled(t *testing.T) {
	app := newTestApp(t)
	backend := newChatGPTBackend(t)
	configureBackendForTest(t, app, backend.server.URL)
	opened := make(chan struct{})
	withBrowser(t, func(context.Context, string) { close(opened) }) // the browser never comes back
	waiting := make(chan error, 1)
	go func() {
		_, err := app.ConnectChatGPT()
		waiting <- err
	}()
	<-opened
	if _, err := app.ConnectChatGPT(); err == nil || !strings.Contains(err.Error(), "already signing in") {
		t.Errorf("a second sign-in read as %v", err)
	}
	app.CancelChatGPTSignIn()
	select {
	case err := <-waiting:
		if err == nil || err.Error() != "Signing in was cancelled." {
			t.Errorf("a cancelled sign-in read as %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelling did not stop the wait")
	}
}

// Without sync, nothing is asked; a server from before the sign-in offers
// none; a disconnection ChatGPT did not confirm says so.
func TestTheConnectionSaysWhereItStands(t *testing.T) {
	app := newTestApp(t)
	if off, err := app.GetAssistantConnection(); err != nil || off.Reachable || off.Message == "" {
		t.Errorf("with sync off: %+v, %v", off, err)
	}
	if _, err := app.ConnectChatGPT(); err == nil || !strings.Contains(err.Error(), "Settings › Sync") {
		t.Errorf("signing in with sync off read as %v", err)
	}

	backend := newChatGPTBackend(t)
	configureBackendForTest(t, app, backend.server.URL)
	backend.mu.Lock()
	backend.missing = true
	backend.mu.Unlock()
	if older, err := app.GetAssistantConnection(); err != nil || !older.Reachable || older.Offered ||
		!strings.Contains(older.Message, "Update it") {
		t.Errorf("an older server: %+v, %v", older, err)
	}
	if _, err := app.ListAssistantModels(); err == nil || !strings.Contains(err.Error(), "does not offer signing in to ChatGPT") {
		t.Errorf("models from an older server read as %v", err)
	}

	backend.mu.Lock()
	backend.missing, backend.revoked = false, false
	backend.mu.Unlock()
	if disconnected, err := app.DisconnectChatGPT(); err != nil || disconnected.Revoked == nil || *disconnected.Revoked {
		t.Errorf("an unconfirmed disconnection: %+v, %v", disconnected, err)
	}
}

// The window opens only the plan's own pages, never an address it is given.
func TestOnlyThePlansOwnPagesOpen(t *testing.T) {
	app := newTestApp(t)
	var opened []string
	previous := openInBrowser
	openInBrowser = func(_ context.Context, address string) { opened = append(opened, address) }
	t.Cleanup(func() { openInBrowser = previous })
	for _, page := range []string{"usage", "help"} {
		if err := app.OpenAssistantPage(AssistantPageInput{Page: page}); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.OpenAssistantPage(AssistantPageInput{Page: "https://attacker.example"}); err == nil {
		t.Error("an address was opened")
	}
	if len(opened) != 2 || opened[0] != "https://chatgpt.com/settings/usage" || opened[1] != "https://help.openai.com" {
		t.Errorf("opened %v", opened)
	}
}

// The assistant's reply carries what pays for it and why it did not answer.
func TestAnAssistantReplySaysWhyItDidNotAnswer(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(registerDeviceResponse{SchemaVersion: "v1", DeviceID: "device_desktop", Token: "assistant-token"})
		case "/v1/status":
			_, _ = w.Write([]byte(`{"schema_version":"v1","assistant":{"configured":true,"provider":"chatgpt","model":"synthetic-fast","auth":"chatgpt_plan"}}`))
		case "/v1/assistant/message":
			_, _ = w.Write([]byte(`{"schema_version":"v1","backend":{"configured":true,"provider":"chatgpt","model":"synthetic-fast","auth":"chatgpt_plan"},"result":"answer_only","action":"answer_only","answer":"The assistant's service hit its usage limit.","reason":"usage_limit","proposals":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureBackendForTest(t, app, server.URL)

	status, err := app.GetAssistantStatus()
	if err != nil || !status.Enabled || status.Auth != "chatgpt_plan" || status.Provider != "chatgpt" {
		t.Fatalf("status %+v, %v", status, err)
	}
	reply, err := app.SendAssistantMessage(AssistantMessageInput{Message: "find 45 minutes for my call"})
	if err != nil || reply.Reason != "usage_limit" || reply.Auth != "chatgpt_plan" || len(reply.Proposals) != 0 {
		t.Fatalf("reply %+v, %v", reply, err)
	}
}

// Once the browser has answered, Cancel no longer stops the sign-in: the
// server is told, so a code it has spent is not left half used.
func TestCancellingAfterTheBrowserAnsweredStillFinishes(t *testing.T) {
	app := newTestApp(t)
	backend := newChatGPTBackend(t)
	configureBackendForTest(t, app, backend.server.URL)
	withBrowser(t, backend.browser(""))

	reached, release := make(chan struct{}), make(chan struct{})
	inner := backend.server.Config.Handler
	backend.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/assistant/chatgpt/finish" {
			close(reached)
			<-release
		}
		inner.ServeHTTP(w, r)
	})
	done := make(chan error, 1)
	go func() {
		connected, err := app.ConnectChatGPT()
		if err == nil && connected.State != "connected" {
			err = errors.New("not connected: " + connected.State)
		}
		done <- err
	}()
	<-reached
	app.CancelChatGPTSignIn()
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a sign-in cancelled after the browser answered ended as %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the sign-in did not finish")
	}
}

// No answer of the server's is shown as it came, and a store that cannot be
// read is not mistaken for sync turned off.
func TestFailuresReadAsSentences(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{backendHTTPError{StatusCode: http.StatusConflict, Message: `{"error":"ChatGPT: plan_not_shared","problem":"plan_not_shared"}`},
			chatGPTProblems["plan_not_shared"]},
		{backendHTTPError{StatusCode: http.StatusConflict, Message: `{"error":"ChatGPT: something_new","problem":"something_new"}`},
			"ChatGPT could not do that. Try again."},
		{backendHTTPError{StatusCode: http.StatusInternalServerError, Message: `{"error":"the ChatGPT connection could not be kept"}`},
			"Your server could not finish that. Try again."},
		{backendHTTPError{StatusCode: http.StatusNotFound, Message: "404 page not found"},
			"Your server does not offer signing in to ChatGPT. Set the assistant's provider to chatgpt in its configuration."},
	} {
		if got := chatGPTFailure(tc.err).Error(); got != tc.want || strings.Contains(got, "{") {
			t.Errorf("%v read as %q", tc.err, got)
		}
	}

	app := newTestApp(t)
	app.storeErr = errors.New("the local store could not be opened")
	if connection, err := app.GetAssistantConnection(); err != nil || connection.Reachable ||
		connection.Message != "The local store could not be opened." {
		t.Errorf("a store that cannot be read read as %+v, %v", connection, err)
	}
}
