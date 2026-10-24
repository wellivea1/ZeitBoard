// Package chatgpt lets the assistant use the owner's ChatGPT plan instead of
// an API key (ADR-0066), through OpenAI's Sign in with ChatGPT.
//
// The server holds the whole sign-in. The owner's computer opens the
// authorization address this package builds, catches the browser's return on
// a loopback address of its own, and passes the one-time code back; the code
// is of no use without the PKCE verifier, which never leaves this package.
// Tokens never reach a device, a log or a response. The refresh token is kept
// encrypted in the server's store, rotated on every refresh, and used by one
// refresh at a time.
package chatgpt

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	// UsageURL is where the owner sees and manages the plan's usage.
	UsageURL = "https://chatgpt.com/settings/usage"

	// appName is how ChatGPT names ZeitBoard when it asks for consent.
	appName = "ZeitBoard"

	// scope asks for identity, a refresh token, and use of the plan.
	scope     = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
	planScope = "chatgpt.tokens.use.direct"

	// dynamicClient stands for the client a first sign-in registers.
	dynamicClient = "dynamic_agent_client"

	// settingName is the sealed setting the connection is kept under.
	settingName = "assistant.chatgpt"

	signInTimeout  = 10 * time.Minute
	requestTimeout = 15 * time.Second
	revokeTimeout  = 10 * time.Second
	modelsTTL      = 10 * time.Minute
	// refreshMargin renews an access token this long before it lapses.
	refreshMargin  = time.Minute
	maxAnswerBytes = 1 << 20
)

// Endpoints are OpenAI's. They are fixed; tests point them at a local server.
type Endpoints struct {
	// Issuer signs the owner in and publishes the rest in its discovery
	// document.
	Issuer string
	// API is the resource the tokens are for, and serves the models.
	API string
}

// DefaultEndpoints are OpenAI's own.
func DefaultEndpoints() Endpoints {
	return Endpoints{Issuer: "https://auth.openai.com", API: "https://api.openai.com/v1"}
}

// Keeper keeps the connection. *store.Store is one: the setting is sealed
// whole, and a change the owner made is audited with the device that made it.
type Keeper interface {
	SealedSetting(ctx context.Context, name string) ([]byte, bool, error)
	PutSealedSetting(ctx context.Context, name string, value []byte, at time.Time, event, deviceID string) error
}

// Options configure an Account.
type Options struct {
	Endpoints Endpoints
	// DefaultModel is the model a new connection starts with, when the
	// server's configuration names one; otherwise the plan's first.
	DefaultModel string
	HTTP         *http.Client
	Now          func() time.Time
}

// State is where the connection stands.
type State string

const (
	Disconnected State = "disconnected"
	Connected    State = "connected"
	// Ended is a sign-in that stopped working. The account it was is kept, so
	// signing in again is to that account.
	Ended State = "sign_in_ended"
)

// Status is the connection as a device may see it: no token, no subject.
type Status struct {
	State State  `json:"state"`
	Email string `json:"email,omitempty"`
	Model string `json:"model,omitempty"`
	// Problem says why a sign-in ended, or why the last one was not kept.
	Problem  Problem `json:"problem,omitempty"`
	UsageURL string  `json:"usageUrl"`
}

// Model is one model the plan offers.
type Model struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
}

// saved is the connection as it is kept.
type saved struct {
	// HostID names this server to OpenAI: opaque, created before the first
	// sign-in, and kept for good.
	HostID string `json:"hostId"`
	// ClientID is the client the first sign-in registered, reused by every
	// later one.
	ClientID string `json:"clientId,omitempty"`
	Model    string `json:"model,omitempty"`
	Grant    *grant `json:"grant,omitempty"`
}

type grant struct {
	// RefreshToken is empty once the sign-in has ended.
	RefreshToken string    `json:"refreshToken,omitempty"`
	Subject      string    `json:"subject"`
	Email        string    `json:"email,omitempty"`
	Scopes       []string  `json:"scopes"`
	ConnectedAt  time.Time `json:"connectedAt"`
	EndedBy      Problem   `json:"endedBy,omitempty"`
}

// access is an access token, held in memory only.
type access struct {
	token   string
	expires time.Time
	// idToken, while set, is a refreshed token's ID token not yet verified,
	// as when the issuer's keys could not be read; the access token is not
	// used until it is.
	idToken    string
	receivedAt time.Time
}

// pending is one sign-in begun and not yet finished.
type pending struct {
	state, nonce, verifier, redirect string
	expires                          time.Time
}

// Account is the server's one ChatGPT connection.
type Account struct {
	keeper       Keeper
	endpoints    Endpoints
	defaultModel string
	client       *http.Client
	now          func() time.Time

	// turns lets one token request run at a time: a sign-in finishing, a
	// refresh, or a disconnection. Refresh tokens rotate, so two refreshes
	// with one token would end the sign-in.
	turns sync.Mutex

	mu        sync.Mutex
	saved     saved
	pending   *pending
	notice    Problem
	access    access
	models    []Model
	modelsAt  time.Time
	found     *discovery
	keys      map[string]*rsa.PublicKey
	keysFrom  string
	reconsent bool
}

// Open reads the kept connection.
func Open(ctx context.Context, keeper Keeper, options Options) (*Account, error) {
	if options.Endpoints == (Endpoints{}) {
		options.Endpoints = DefaultEndpoints()
	}
	if options.HTTP == nil {
		options.HTTP = &http.Client{}
	}
	client := *options.HTTP
	// A redirect could carry a token or the assistant's context elsewhere.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if options.Now == nil {
		options.Now = time.Now
	}
	account := &Account{
		keeper:       keeper,
		endpoints:    options.Endpoints,
		defaultModel: strings.TrimSpace(options.DefaultModel),
		client:       &client,
		now:          options.Now,
	}
	data, ok, err := keeper.SealedSetting(ctx, settingName)
	if err != nil {
		return nil, fmt.Errorf("read the ChatGPT connection: %w", err)
	}
	if ok {
		if err := json.Unmarshal(data, &account.saved); err != nil {
			return nil, fmt.Errorf("read the ChatGPT connection: %w", err)
		}
	}
	return account, nil
}

// Status is the connection now. It makes no network call.
func (a *Account) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	status := Status{State: Disconnected, Model: a.saved.Model, Problem: a.notice, UsageURL: UsageURL}
	if g := a.saved.Grant; g != nil {
		status.Email = g.Email
		status.State = Connected
		if g.RefreshToken == "" {
			status.State, status.Problem = Ended, g.EndedBy
		}
	}
	return status
}

// keep writes the connection, with an event when the owner made the change.
// Callers hold a.turns.
func (a *Account) keep(ctx context.Context, next saved, event, deviceID string) error {
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	// Kept even when the request that led here is cancelled: a refresh token
	// already rotated must not be lost with it.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()
	if err := a.keeper.PutSealedSetting(ctx, settingName, data, a.now(), event, deviceID); err != nil {
		return fmt.Errorf("keep the ChatGPT connection: %w", err)
	}
	a.mu.Lock()
	a.saved = next
	a.mu.Unlock()
	return nil
}

// current is a copy of the kept connection.
func (a *Account) current() saved {
	a.mu.Lock()
	defer a.mu.Unlock()
	copied := a.saved
	if a.saved.Grant != nil {
		g := *a.saved.Grant
		g.Scopes = slices.Clone(g.Scopes)
		copied.Grant = &g
	}
	return copied
}

// send performs a request and reads at most maxAnswerBytes of its answer.
// No error it returns holds a token or a body.
func (a *Account) send(request *http.Request) (int, []byte, error) {
	response, err := a.client.Do(request)
	if err != nil {
		// Cancelled is the caller's doing; anything else, a time limit
		// included, is ChatGPT not answering.
		if errors.Is(request.Context().Err(), context.Canceled) {
			return 0, nil, context.Canceled
		}
		return 0, nil, fail(Busy, "network_error")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes+1))
	if err != nil || len(body) > maxAnswerBytes {
		return response.StatusCode, nil, fail(Busy, "invalid_response")
	}
	return response.StatusCode, body, nil
}

func isControl(r rune) bool { return unicode.IsControl(r) }
