package chatgpt_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"non24.app/server/internal/chatgpt"
	"non24.app/server/internal/chatgpt/chatgpttest"
	"non24.app/server/internal/store"
)

const redirect = "http://127.0.0.1:53682/auth/callback"

var ctx = context.Background()

func open(t *testing.T, path string, fake *chatgpttest.Server, options chatgpt.Options) (*chatgpt.Account, *store.Store) {
	t.Helper()
	st, err := store.Open(path, bytes.Repeat([]byte("k"), 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	options.Endpoints = chatgpt.Endpoints{Issuer: fake.Issuer(), API: fake.API()}
	account, err := chatgpt.Open(ctx, st, options)
	if err != nil {
		t.Fatal(err)
	}
	return account, st
}

func signIn(t *testing.T, account *chatgpt.Account, fake *chatgpttest.Server) (chatgpt.Status, url.Values, error) {
	t.Helper()
	begun, err := account.BeginSignIn(ctx, redirect)
	if err != nil {
		t.Fatal(err)
	}
	address, err := url.Parse(begun.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	state, code, clientID := fake.Approve(begun.AuthorizationURL)
	if state != begun.State {
		t.Fatalf("the address carries state %q, the sign-in %q", state, begun.State)
	}
	status, err := account.FinishSignIn(ctx, "device-synthetic", state, code, clientID)
	return status, address.Query(), err
}

func problemOf(err error) chatgpt.Problem {
	var failed *chatgpt.Error
	if errors.As(err, &failed) {
		return failed.Problem
	}
	return ""
}

// A first sign-in registers a client, names ZeitBoard and this server's host,
// and keeps a connection that shares the plan, with the plan's first model.
func TestASignInKeepsAConnectionThatSharesThePlan(t *testing.T) {
	fake := chatgpttest.New(t)
	path := filepath.Join(t.TempDir(), "zeitboardd.db")
	account, _ := open(t, path, fake, chatgpt.Options{})
	if status := account.Status(); status.State != chatgpt.Disconnected || status.UsageURL != chatgpt.UsageURL {
		t.Fatalf("before signing in: %+v", status)
	}

	status, query, err := signIn(t, account, fake)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != chatgpt.Connected || status.Email != "owner@example.com" || status.Model != "synthetic-fast" {
		t.Fatalf("after signing in: %+v", status)
	}
	if query.Get("client_id") != "dynamic_agent_client" || query.Get("agent_name_hint") != "ZeitBoard" ||
		query.Get("redirect_uri") != redirect || query.Get("prompt") != "" || query.Get("login_hint") != "" {
		t.Errorf("a first sign-in asked %v", query)
	}
	if !regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(query.Get("ext_agent_host_id")) {
		t.Errorf("host ID %q", query.Get("ext_agent_host_id"))
	}
	if len(query.Get("state")) < 43 || len(query.Get("nonce")) < 43 || len(query.Get("code_challenge")) != 43 {
		t.Errorf("state, nonce or challenge too short: %v", query)
	}

	// Nothing the sign-in granted is on disk as it was granted.
	refresh := fake.RefreshToken()
	for _, file := range []string{path, path + "-wal"} {
		data, err := os.ReadFile(file)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		for _, secret := range []string{refresh, "owner@example.com", "user-synthetic-1"} {
			if bytes.Contains(data, []byte(secret)) {
				t.Errorf("%s holds %q as it was granted", filepath.Base(file), secret)
			}
		}
	}

	// A later sign-in reuses the registration and the host, and the
	// connection survives the server starting again.
	host := query.Get("ext_agent_host_id")
	reopened, err := chatgpt.Open(ctx, mustStore(t, path), chatgpt.Options{Endpoints: chatgpt.Endpoints{Issuer: fake.Issuer(), API: fake.API()}})
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Status(); got != status {
		t.Errorf("reopened as %+v", got)
	}
	begun, err := reopened.BeginSignIn(ctx, redirect)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := url.Parse(begun.AuthorizationURL)
	if q := again.Query(); q.Get("client_id") == "dynamic_agent_client" || q.Get("agent_name_hint") != "" || q.Get("ext_agent_host_id") != host {
		t.Errorf("a later sign-in asked %v", q)
	}
}

func mustStore(t *testing.T, path string) *store.Store {
	t.Helper()
	st, err := store.Open(path, bytes.Repeat([]byte("k"), 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// The assistant's request streams, keeps nothing on OpenAI's side, and gives
// the instructions as instructions.
func TestRespondStreamsTheModelsText(t *testing.T) {
	fake := chatgpttest.New(t)
	account, _ := open(t, ":memory:", fake, chatgpt.Options{DefaultModel: "synthetic-deep"})
	if _, _, err := signIn(t, account, fake); err != nil {
		t.Fatal(err)
	}
	text, err := account.Respond(ctx, "Synthetic instructions.", "Synthetic question.")
	if err != nil || text != fake.Reply {
		t.Fatalf("answered %q, %v", text, err)
	}
	fake.Lock()
	sent := fake.LastResponse
	fake.Unlock()
	if sent["model"] != "synthetic-deep" || sent["instructions"] != "Synthetic instructions." || sent["store"] != false || sent["stream"] != true {
		t.Errorf("asked %v", sent)
	}
	input, _ := sent["input"].([]any)
	if len(input) != 1 || input[0].(map[string]any)["role"] != "user" || input[0].(map[string]any)["content"] != "Synthetic question." {
		t.Errorf("input %v", sent["input"])
	}
}

// Access tokens about to lapse are renewed by one refresh, however many
// requests want one, and the rotated refresh token is the one kept.
func TestARefreshRotatesTheTokenOnce(t *testing.T) {
	fake := chatgpttest.New(t)
	fake.ExpiresIn = 30 // inside the minute before expiry: every request refreshes
	account, _ := open(t, ":memory:", fake, chatgpt.Options{})
	if _, _, err := signIn(t, account, fake); err != nil {
		t.Fatal(err)
	}
	first := fake.RefreshToken()
	fake.Lock()
	fake.ExpiresIn = 3600
	fake.Unlock()

	var wait sync.WaitGroup
	failures := make(chan error, 8)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := account.Respond(ctx, "i", "q"); err != nil {
				failures <- err
			}
		}()
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	fake.Lock()
	refreshes := fake.Refreshes
	fake.Unlock()
	if refreshes != 1 {
		t.Errorf("%d refreshes for eight requests", refreshes)
	}
	if rotated := fake.RefreshToken(); rotated == first || rotated == "" {
		t.Errorf("kept %q after rotating %q", rotated, first)
	}
}

// A refresh token ChatGPT says is spent ends the sign-in, keeping the account
// it was: signing in again hints at it, and refuses another account.
func TestASpentRefreshTokenEndsTheSignIn(t *testing.T) {
	fake := chatgpttest.New(t)
	account, _ := open(t, ":memory:", fake, chatgpt.Options{})
	if _, _, err := signIn(t, account, fake); err != nil {
		t.Fatal(err)
	}
	fake.ExpireAccess()
	fake.Lock()
	fake.RefuseRefresh = "refresh_token_invalidated"
	fake.Unlock()
	if _, err := account.Respond(ctx, "i", "q"); problemOf(err) != chatgpt.SignInEnded {
		t.Fatalf("a spent token answered %v", err)
	}
	if status := account.Status(); status.State != chatgpt.Ended || status.Email != "owner@example.com" || status.Problem != chatgpt.SignInEnded {
		t.Fatalf("after the sign-in ended: %+v", status)
	}

	fake.Lock()
	fake.RefuseRefresh, fake.Subject = "", "user-synthetic-2"
	fake.Unlock()
	if _, query, err := signIn(t, account, fake); problemOf(err) != chatgpt.AccountMismatch || query.Get("login_hint") != "owner@example.com" {
		t.Fatalf("another account signing in: %v, hint %q", err, query.Get("login_hint"))
	}
	fake.Lock()
	fake.Subject = "user-synthetic-1"
	fake.Unlock()
	if status, _, err := signIn(t, account, fake); err != nil || status.State != chatgpt.Connected {
		t.Fatalf("the same account signing in again: %+v, %v", status, err)
	}
}

// A sign-in that does not share the plan gives the assistant nothing: it is
// withdrawn at once, and the next sign-in asks for consent anew.
func TestASignInWithoutThePlanIsWithdrawn(t *testing.T) {
	fake := chatgpttest.New(t)
	fake.Scope = "openid profile email offline_access"
	account, _ := open(t, ":memory:", fake, chatgpt.Options{})
	if _, _, err := signIn(t, account, fake); problemOf(err) != chatgpt.PlanNotShared {
		t.Fatalf("a sign-in without the plan answered %v", err)
	}
	fake.Lock()
	revoked := len(fake.Revoked)
	fake.Unlock()
	if revoked != 1 || fake.RefreshToken() != "" {
		t.Errorf("%d tokens revoked, %q still live", revoked, fake.RefreshToken())
	}
	if status := account.Status(); status.State != chatgpt.Disconnected || status.Problem != chatgpt.PlanNotShared {
		t.Errorf("after a sign-in without the plan: %+v", status)
	}
	begun, err := account.BeginSignIn(ctx, redirect)
	if err != nil {
		t.Fatal(err)
	}
	if address, _ := url.Parse(begun.AuthorizationURL); address.Query().Get("prompt") != "consent" {
		t.Errorf("signing in again asked %v", address.Query())
	}
}

// The browser's answer counts only for the sign-in waiting for it, once,
// within ten minutes, and from the client registered.
func TestOnlyTheWaitingSignInFinishes(t *testing.T) {
	fake := chatgpttest.New(t)
	now := time.Now()
	clock := func() time.Time { return now }
	account, _ := open(t, ":memory:", fake, chatgpt.Options{Now: clock})

	begun, err := account.BeginSignIn(ctx, redirect)
	if err != nil {
		t.Fatal(err)
	}
	state, code, clientID := fake.Approve(begun.AuthorizationURL)
	if _, err := account.FinishSignIn(ctx, "d", "another-state", code, clientID); problemOf(err) != chatgpt.SignInExpired {
		t.Errorf("another state finished as %v", err)
	}
	// A stray answer leaves the waiting sign-in to finish, once.
	if _, err := account.FinishSignIn(ctx, "d", state, code, clientID); err != nil {
		t.Fatalf("the waiting sign-in did not finish after a stray answer: %v", err)
	}
	if _, err := account.FinishSignIn(ctx, "d", state, code, clientID); problemOf(err) != chatgpt.SignInExpired {
		t.Errorf("a finished sign-in finished again as %v", err)
	}
	if _, err := account.Disconnect(ctx, "d"); err != nil {
		t.Fatal(err)
	}

	begun, _ = account.BeginSignIn(ctx, redirect)
	state, code, clientID = fake.Approve(begun.AuthorizationURL)
	now = now.Add(11 * time.Minute)
	if _, err := account.FinishSignIn(ctx, "d", state, code, clientID); problemOf(err) != chatgpt.SignInExpired {
		t.Errorf("a lapsed sign-in finished as %v", err)
	}

	for name, client := range map[string]string{"placeholder": "dynamic_agent_client", "malformed": "client id!"} {
		begun, _ = account.BeginSignIn(ctx, redirect)
		state, code, _ = fake.Approve(begun.AuthorizationURL)
		if _, err := account.FinishSignIn(ctx, "d", state, code, client); problemOf(err) != chatgpt.Refused {
			t.Errorf("a %s client finished as %v", name, err)
		}
	}
	if status := account.Status(); status.State != chatgpt.Disconnected {
		t.Errorf("after refused answers: %+v", status)
	}
}

// Only http://127.0.0.1:<port>/auth/callback is a redirect.
func TestASignInReturnsOnlyToTheLoopbackCallback(t *testing.T) {
	fake := chatgpttest.New(t)
	account, _ := open(t, ":memory:", fake, chatgpt.Options{})
	for _, bad := range []string{
		"https://127.0.0.1:53682/auth/callback", "http://localhost:53682/auth/callback",
		"http://127.0.0.1:53682/", "http://127.0.0.1/auth/callback", "http://127.0.0.1:053682/auth/callback",
		"http://127.0.0.1:53682/auth/callback?x=1", "http://user@127.0.0.1:53682/auth/callback",
		"http://192.168.1.2:53682/auth/callback", "http://[::1]:53682/auth/callback",
	} {
		if _, err := account.BeginSignIn(ctx, bad); problemOf(err) != chatgpt.Refused {
			t.Errorf("%s began as %v", bad, err)
		}
	}
}

// An ID token is believed only when the issuer signed it for this client and
// this sign-in, and it is current.
func TestAnIDTokenMustBeTheIssuersForThisSignIn(t *testing.T) {
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(map[string]any){
		"another issuer":   func(c map[string]any) { c["iss"] = "https://issuer.invalid" },
		"another audience": func(c map[string]any) { c["aud"] = "client-other" },
		"another party":    func(c map[string]any) { c["azp"] = "client-other" },
		"many audiences":   func(c map[string]any) { c["aud"] = []string{c["aud"].(string), "client-other"} },
		"another nonce":    func(c map[string]any) { c["nonce"] = "nonce-other" },
		"expired":          func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() },
		"from the future":  func(c map[string]any) { c["iat"] = time.Now().Add(time.Hour).Unix() },
		"no subject":       func(c map[string]any) { delete(c, "sub") },
	} {
		t.Run(name, func(t *testing.T) {
			fake := chatgpttest.New(t)
			fake.Claims = change
			account, _ := open(t, ":memory:", fake, chatgpt.Options{})
			if _, _, err := signIn(t, account, fake); problemOf(err) != chatgpt.SignInEnded {
				t.Fatalf("signed in as %v", err)
			}
			if account.Status().State != chatgpt.Disconnected || fake.RefreshToken() != "" {
				t.Errorf("kept %+v, live %q", account.Status(), fake.RefreshToken())
			}
		})
	}
	t.Run("another key", func(t *testing.T) {
		fake := chatgpttest.New(t)
		fake.SignWith = other
		account, _ := open(t, ":memory:", fake, chatgpt.Options{})
		if _, _, err := signIn(t, account, fake); problemOf(err) != chatgpt.SignInEnded {
			t.Fatalf("a token signed with another key signed in as %v", err)
		}
	})
	t.Run("a key the issuer does not publish", func(t *testing.T) {
		fake := chatgpttest.New(t)
		fake.KeyID = "synthetic-unpublished"
		account, _ := open(t, ":memory:", fake, chatgpt.Options{})
		if _, _, err := signIn(t, account, fake); problemOf(err) != chatgpt.SignInEnded {
			t.Fatalf("a token naming an unpublished key signed in as %v", err)
		}
	})
}

// Disconnecting withdraws the grant and forgets the account, keeping the
// registration; a token ChatGPT refuses is renewed once and the request sent
// again.
func TestDisconnectingWithdrawsTheGrant(t *testing.T) {
	fake := chatgpttest.New(t)
	account, _ := open(t, ":memory:", fake, chatgpt.Options{})
	if _, _, err := signIn(t, account, fake); err != nil {
		t.Fatal(err)
	}
	fake.ExpireAccess()
	if _, err := account.Respond(ctx, "i", "q"); err != nil {
		t.Fatalf("a refused access token was not renewed: %v", err)
	}
	refresh := fake.RefreshToken()
	revoked, err := account.Disconnect(ctx, "device-synthetic")
	if err != nil || !revoked {
		t.Fatalf("disconnected %v, %v", revoked, err)
	}
	fake.Lock()
	withdrawn := fake.Revoked
	fake.Unlock()
	if len(withdrawn) != 1 || withdrawn[0] != refresh {
		t.Errorf("withdrew %v, held %q", withdrawn, refresh)
	}
	if status := account.Status(); status.State != chatgpt.Disconnected || status.Email != "" {
		t.Errorf("after disconnecting: %+v", status)
	}
	if _, err := account.Respond(ctx, "i", "q"); problemOf(err) != chatgpt.SignInEnded {
		t.Errorf("a disconnected account answered %v", err)
	}
	begun, _ := account.BeginSignIn(ctx, redirect)
	if address, _ := url.Parse(begun.AuthorizationURL); address.Query().Get("client_id") == "dynamic_agent_client" || address.Query().Get("login_hint") != "" {
		t.Errorf("signing in after disconnecting asked %v", address.Query())
	}
}

// What a failed answer says reaches the owner as a problem, never as text.
func TestAFailedAnswerSaysWhy(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		stream []string
		want   chatgpt.Problem
	}{
		"plan limit":           {status: 429, body: `{"error":{"code":"subscription_sharing_usage_limit_exceeded","message":"secret text"}}`, want: chatgpt.UsageLimit},
		"v2 limit":             {status: 429, body: `{"error":{"code":"subscription_sharing_v2_usage_limit_exceeded"}}`, want: chatgpt.UsageLimit},
		"not eligible":         {status: 403, body: `{"detail":{"error":{"code":"subscription_sharing_user_not_eligible"}}}`, want: chatgpt.NotEligible},
		"client not enabled":   {status: 403, body: `{"error":{"code":"subscription_sharing_v2_client_not_enabled"}}`, want: chatgpt.NotEnabled},
		"unavailable":          {status: 503, body: `{"error":{"code":"subscription_sharing_usage_unavailable"}}`, want: chatgpt.Busy},
		"unsupported":          {status: 400, body: `{"error":{"code":"subscription_sharing_unsupported_capability","param":"text.format"}}`, want: chatgpt.Refused},
		"model gone":           {status: 404, body: `{"error":{"code":"model_not_found"}}`, want: chatgpt.ModelUnavailable},
		"server error":         {status: 500, body: `not json`, want: chatgpt.Busy},
		"failed in stream":     {stream: []string{`{"type":"response.failed","response":{"error":{"code":"subscription_sharing_usage_limit_exceeded"}}}`}, want: chatgpt.UsageLimit},
		"error event":          {stream: []string{`{"type":"error","code":"server_error","message":"secret text"}`}, want: chatgpt.Busy},
		"incomplete":           {stream: []string{`{"type":"response.incomplete"}`}, want: chatgpt.Refused},
		"cut short":            {stream: []string{`{"type":"response.output_text.delta","delta":"half"}`}, want: chatgpt.Busy},
		"plan scope withdrawn": {status: 403, body: `{"error":{"code":"chatpass_v2_scope_not_authorized"}}`, want: chatgpt.PlanNotShared},
	} {
		t.Run(name, func(t *testing.T) {
			fake := chatgpttest.New(t)
			account, _ := open(t, ":memory:", fake, chatgpt.Options{})
			if _, _, err := signIn(t, account, fake); err != nil {
				t.Fatal(err)
			}
			fake.Lock()
			fake.Status, fake.Body, fake.Stream = tc.status, tc.body, tc.stream
			fake.Unlock()
			text, err := account.Respond(ctx, "i", "q")
			if problemOf(err) != tc.want || text != "" {
				t.Fatalf("answered %q, %v", text, err)
			}
			if strings.Contains(err.Error(), "secret text") {
				t.Errorf("the error repeats ChatGPT's message: %v", err)
			}
		})
	}
}

// The catalog lists what ChatGPT's picker would, and only a listed model can
// be chosen.
func TestOnlyAListedModelCanBeChosen(t *testing.T) {
	fake := chatgpttest.New(t)
	fake.Models = append(fake.Models, map[string]any{"slug": "", "display_name": "No slug", "visibility": "list"},
		map[string]any{"slug": "synthetic-control\n", "display_name": "Control", "visibility": "list"})
	account, _ := open(t, ":memory:", fake, chatgpt.Options{})
	if _, _, err := signIn(t, account, fake); err != nil {
		t.Fatal(err)
	}
	models, err := account.Models(ctx)
	if err != nil || len(models) != 2 || models[0].Slug != "synthetic-fast" || models[1].DisplayName != "Synthetic Deep" {
		t.Fatalf("listed %+v, %v", models, err)
	}
	if status, err := account.ChooseModel(ctx, "d", "synthetic-deep"); err != nil || status.Model != "synthetic-deep" {
		t.Fatalf("chose %+v, %v", status, err)
	}
	for _, unlisted := range []string{"synthetic-hidden", "synthetic-unknown", ""} {
		if _, err := account.ChooseModel(ctx, "d", unlisted); problemOf(err) != chatgpt.ModelUnavailable {
			t.Errorf("%q chosen as %v", unlisted, err)
		}
	}
	if account.Status().Model != "synthetic-deep" {
		t.Errorf("model %q", account.Status().Model)
	}
}
