package chatgpt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// maxTokenBytes bounds each token ChatGPT grants.
const maxTokenBytes = 16 * 1024

type discovery struct {
	Issuer        string `json:"issuer"`
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	Revocation    string `json:"revocation_endpoint"`
	JWKSURI       string `json:"jwks_uri"`
}

// discover reads the issuer's discovery document once. Every address it gives
// must be on the issuer's own origin, so a document that names another host
// cannot send a code or a token there.
func (a *Account) discover(ctx context.Context) (discovery, error) {
	a.mu.Lock()
	if a.found != nil {
		found := *a.found
		a.mu.Unlock()
		return found, nil
	}
	a.mu.Unlock()
	unavailable := fail(Busy, "discovery_failed")
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoints.Issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return discovery{}, unavailable
	}
	request.Header.Set("Accept", "application/json")
	status, body, err := a.send(request)
	if err != nil {
		return discovery{}, err
	}
	var found discovery
	if status != http.StatusOK || json.Unmarshal(body, &found) != nil || found.Issuer != a.endpoints.Issuer ||
		found.Authorization == "" || found.Token == "" || found.JWKSURI == "" {
		return discovery{}, unavailable
	}
	for _, endpoint := range []string{found.Authorization, found.Token, found.JWKSURI, found.Revocation} {
		if endpoint != "" && !sameOrigin(found.Issuer, endpoint) {
			return discovery{}, unavailable
		}
	}
	a.mu.Lock()
	a.found = &found
	a.mu.Unlock()
	return found, nil
}

func sameOrigin(base, other string) bool {
	b, err := url.Parse(base)
	if err != nil {
		return false
	}
	o, err := url.Parse(other)
	return err == nil && o.Scheme == b.Scheme && o.Host == b.Host && o.User == nil
}

// SignIn is a sign-in begun: the address to open in the browser, and the
// state its answer must carry back.
type SignIn struct {
	AuthorizationURL string `json:"authorizationUrl"`
	State            string `json:"state"`
}

// BeginSignIn prepares a sign-in whose answer the browser brings back to
// redirect, a loopback address on the owner's computer. One sign-in waits at
// a time; beginning another forgets the first.
func (a *Account) BeginSignIn(ctx context.Context, redirect string) (SignIn, error) {
	if !loopbackRedirect(redirect) {
		return SignIn{}, fail(Refused, "invalid_redirect")
	}
	found, err := a.discover(ctx)
	if err != nil {
		return SignIn{}, err
	}
	authorize, err := url.Parse(found.Authorization)
	if err != nil {
		return SignIn{}, fail(Busy, "discovery_failed")
	}
	a.turns.Lock()
	defer a.turns.Unlock()
	current := a.current()
	// The host ID exists before the first sign-in and is kept for good.
	if current.HostID == "" {
		hostID, err := newHostID()
		if err != nil {
			return SignIn{}, err
		}
		next := current
		next.HostID = hostID
		if err := a.keep(ctx, next, "", ""); err != nil {
			return SignIn{}, err
		}
		current = next
	}
	state, nonce, verifier := randomValue(), randomValue(), randomValue()
	clientID := current.ClientID
	query := authorize.Query()
	if clientID == "" {
		clientID = dynamicClient
		query.Set("agent_name_hint", appName)
	}
	challenge := sha256.Sum256([]byte(verifier))
	query.Set("client_id", clientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", redirect)
	query.Set("scope", scope)
	query.Set("resource", a.endpoints.API)
	query.Set("state", state)
	query.Set("nonce", nonce)
	query.Set("code_challenge_method", "S256")
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("ext_agent_host_id", current.HostID)
	// Signing in again after a sign-in ended is to the account it was.
	if g := current.Grant; g != nil && g.Email != "" {
		query.Set("login_hint", g.Email)
	}

	a.mu.Lock()
	if a.reconsent {
		query.Set("prompt", "consent")
	}
	a.pending = &pending{state: state, nonce: nonce, verifier: verifier, redirect: redirect,
		clientID: clientID, expires: a.now().Add(signInTimeout)}
	a.mu.Unlock()
	authorize.RawQuery = query.Encode()
	return SignIn{AuthorizationURL: authorize.String(), State: state}, nil
}

// loopbackRedirect accepts only http://127.0.0.1:<port>/auth/callback.
func loopbackRedirect(redirect string) bool {
	parsed, err := url.Parse(redirect)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.Path != "/auth/callback" {
		return false
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil || host != "127.0.0.1" {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535 && strconv.Itoa(number) == port
}

var clientIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,200}$`)

// FinishSignIn trades the code the browser brought back for tokens, verifies
// who signed in, and keeps the connection, but only one that shares the
// plan: a sign-in that proves only who the owner is gives the assistant
// nothing, so it is withdrawn at once. clientID is the client the browser's
// answer named, which a first sign-in registered.
func (a *Account) FinishSignIn(ctx context.Context, deviceID, state, code, clientID string) (Status, error) {
	a.turns.Lock()
	defer a.turns.Unlock()
	begun, err := a.takePending(state)
	if err != nil {
		return Status{}, err
	}
	code = strings.TrimSpace(code)
	if code == "" || len(code) > maxTokenBytes {
		return Status{}, fail(Refused, "invalid_code")
	}
	current, clientID, err := a.register(ctx, a.current(), clientID)
	if err != nil {
		return Status{}, err
	}
	found, err := a.discover(ctx)
	if err != nil {
		return Status{}, err
	}
	tokens, receivedAt, err := a.exchange(ctx, found, current, begun, clientID, code)
	if err != nil {
		return Status{}, err
	}
	who, err := a.admit(ctx, found, current, begun, clientID, tokens, receivedAt)
	if err != nil {
		a.revoke(ctx, found, clientID, tokens.refresh)
		return Status{}, err
	}

	previous := current.Grant
	next := current
	next.Grant = &grant{RefreshToken: tokens.refresh, Subject: who.Subject, Email: who.Email,
		Scopes: tokens.scopes, ConnectedAt: a.now().UTC()}
	next.Model = a.startingModel(ctx, next.Model, tokens.access)
	if err := a.keep(ctx, next, "assistant_chatgpt_connected", deviceID); err != nil {
		a.revoke(ctx, found, clientID, tokens.refresh)
		return Status{}, err
	}
	a.mu.Lock()
	a.access = access{token: tokens.access, expires: receivedAt.Add(tokens.lifetime)}
	a.notice, a.reconsent, a.models = "", false, nil
	a.mu.Unlock()
	// Signing in again to the same account leaves the earlier grant behind.
	if previous != nil && previous.RefreshToken != "" {
		a.revoke(ctx, found, clientID, previous.RefreshToken)
	}
	return a.Status(), nil
}

// takePending takes the sign-in waiting for state, once. An answer for
// another state leaves it waiting, so a stray request cannot spoil it.
func (a *Account) takePending(state string) (pending, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	begun := a.pending
	if begun == nil || subtle.ConstantTimeCompare([]byte(begun.state), []byte(state)) != 1 {
		return pending{}, fail(SignInExpired, "sign_in_not_found")
	}
	a.pending = nil
	if !a.now().Before(begun.expires) {
		return pending{}, fail(SignInExpired, "sign_in_expired")
	}
	return *begun, nil
}

// register settles the client the answer names: the one a first sign-in
// registered, which is kept before the exchange so an exchange that fails
// does not register a second, or the one kept already.
func (a *Account) register(ctx context.Context, current saved, clientID string) (saved, string, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = current.ClientID
	}
	if !clientIDPattern.MatchString(clientID) || clientID == dynamicClient ||
		(current.ClientID != "" && clientID != current.ClientID) {
		return saved{}, "", fail(Refused, "registration_incomplete")
	}
	if current.ClientID == "" {
		current.ClientID = clientID
		if err := a.keep(ctx, current, "", ""); err != nil {
			return saved{}, "", err
		}
	}
	return current, clientID, nil
}

// exchange trades the code for tokens. A client ChatGPT no longer knows is
// forgotten, so the next sign-in registers anew.
func (a *Account) exchange(ctx context.Context, found discovery, current saved, begun pending, clientID, code string) (tokens, time.Time, error) {
	answer, receivedAt, err := a.tokenRequest(ctx, found, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"code_verifier": {begun.verifier},
		"redirect_uri":  {begun.redirect},
		"resource":      {a.endpoints.API},
	})
	var refused *Error
	if errors.As(err, &refused) && refused.Code == "invalid_client" {
		current.ClientID = ""
		_ = a.keep(ctx, current, "", "")
	}
	if err != nil {
		return tokens{}, receivedAt, err
	}
	granted, err := readTokens(answer, strings.Fields(scope), false)
	return granted, receivedAt, err
}

// admit decides whether a sign-in's tokens may be kept: an ID token for this
// sign-in, from the account the assistant used if it used one, with the plan
// shared. A refusal is the caller's to withdraw.
func (a *Account) admit(ctx context.Context, found discovery, current saved, begun pending, clientID string, granted tokens, receivedAt time.Time) (identity, error) {
	if granted.idToken == "" {
		return identity{}, fail(SignInEnded, "invalid_id_token")
	}
	who, err := a.verifyIDToken(ctx, found, granted.idToken, clientID, begun.nonce, receivedAt)
	if err != nil {
		return identity{}, err
	}
	if current.Grant != nil && current.Grant.Subject != who.Subject {
		return identity{}, fail(AccountMismatch, "account_mismatch")
	}
	if !slices.Contains(granted.scopes, planScope) || granted.refresh == "" {
		a.mu.Lock()
		a.notice, a.reconsent = PlanNotShared, true
		a.mu.Unlock()
		return identity{}, fail(PlanNotShared, "")
	}
	return who, nil
}

// startingModel is the model a new connection asks: the one chosen before,
// the configuration's, or the plan's first.
func (a *Account) startingModel(ctx context.Context, chosen, access string) string {
	if chosen != "" {
		return chosen
	}
	if a.defaultModel != "" {
		return a.defaultModel
	}
	if models, err := a.readModels(ctx, access); err == nil && len(models) > 0 {
		return models[0].Slug
	}
	return ""
}

// token gives an access token for a request: the one held while it has more
// than a minute left, or a new one from the refresh token, which rotates.
func (a *Account) token(ctx context.Context) (string, error) {
	if held, ok := a.heldAccess(); ok {
		return held, nil
	}
	a.turns.Lock()
	defer a.turns.Unlock()
	// Another request may have refreshed while this one waited its turn.
	if held, ok := a.heldAccess(); ok {
		return held, nil
	}
	current := a.current()
	g := current.Grant
	if g == nil {
		return "", fail(SignInEnded, "not_connected")
	}
	if g.RefreshToken == "" {
		return "", fail(SignInEnded, string(g.EndedBy))
	}
	found, err := a.discover(ctx)
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	unverified := a.access
	a.mu.Unlock()
	if unverified.idToken != "" && a.now().Add(refreshMargin).Before(unverified.expires) {
		if err := a.confirm(ctx, found, current, unverified); err != nil {
			return "", err
		}
		return unverified.token, nil
	}

	// The refresh goes on when the request that asked for it is cancelled:
	// ChatGPT has spent the old token once it answers.
	refreshing, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()
	answer, receivedAt, err := a.tokenRequest(refreshing, found, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {current.ClientID},
		"refresh_token": {g.RefreshToken},
		"resource":      {a.endpoints.API},
	})
	var refused *Error
	if errors.As(err, &refused) && (endsSignIn(refused.Code) || refused.Code == "invalid_client") {
		if refused.Code == "invalid_client" {
			// The registration is gone: the next sign-in registers anew.
			current.ClientID = ""
		}
		a.end(refreshing, current, SignInEnded)
		return "", fail(SignInEnded, refused.Code)
	}
	if err != nil {
		return "", err
	}
	tokens, err := readTokens(answer, g.Scopes, true)
	if err != nil {
		a.end(refreshing, current, SignInEnded)
		return "", err
	}
	// Kept before anything else can fail: the old token is spent.
	next := current
	rotated := *g
	rotated.RefreshToken, rotated.Scopes = tokens.refresh, tokens.scopes
	next.Grant = &rotated
	if err := a.keep(refreshing, next, "", ""); err != nil {
		return "", err
	}
	if !slices.Contains(tokens.scopes, planScope) {
		a.end(refreshing, next, PlanNotShared)
		return "", fail(PlanNotShared, "")
	}
	held := access{token: tokens.access, expires: receivedAt.Add(tokens.lifetime), idToken: tokens.idToken, receivedAt: receivedAt}
	a.mu.Lock()
	a.access = held
	a.mu.Unlock()
	if held.idToken != "" {
		if err := a.confirm(ctx, found, next, held); err != nil {
			return "", err
		}
	}
	return held.token, nil
}

// heldAccess is the access token held, while it is verified and has more
// than a minute left.
func (a *Account) heldAccess() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.access.token == "" || a.access.idToken != "" || !a.now().Add(refreshMargin).Before(a.access.expires) {
		return "", false
	}
	return a.access.token, true
}

// confirm verifies the ID token a refresh brought before its access token is
// used. Keys that cannot be read leave it to confirm on the next request;
// any other failure, or another account, ends the sign-in.
func (a *Account) confirm(ctx context.Context, found discovery, current saved, held access) error {
	who, err := a.verifyIDToken(ctx, found, held.idToken, current.ClientID, "", held.receivedAt)
	var failed *Error
	if errors.As(err, &failed) && failed.Problem == Busy {
		return err
	}
	if err == nil && who.Subject != current.Grant.Subject {
		err = fail(SignInEnded, "account_mismatch")
	}
	if err != nil {
		a.end(ctx, current, SignInEnded)
		return err
	}
	a.mu.Lock()
	if a.access.token == held.token {
		a.access.idToken = ""
	}
	a.mu.Unlock()
	return nil
}

// end records that the sign-in stopped working. The account it was is kept,
// so signing in again is to that account. Callers hold a.turns.
func (a *Account) end(ctx context.Context, current saved, why Problem) {
	a.mu.Lock()
	a.access, a.models = access{}, nil
	a.mu.Unlock()
	if current.Grant == nil {
		return
	}
	ended := *current.Grant
	ended.RefreshToken, ended.EndedBy = "", why
	current.Grant = &ended
	_ = a.keep(ctx, current, "assistant_chatgpt_sign_in_ended", "")
}

// endUnlessRefreshed ends the sign-in a refused access token came from,
// unless another request has already refreshed it.
func (a *Account) endUnlessRefreshed(ctx context.Context, refusedToken string, why Problem) {
	a.turns.Lock()
	defer a.turns.Unlock()
	a.mu.Lock()
	refreshed := a.access.token != "" && a.access.token != refusedToken
	a.mu.Unlock()
	if !refreshed {
		a.end(ctx, a.current(), why)
	}
}

// forget drops a held access token ChatGPT refused, so the next request
// refreshes.
func (a *Account) forget(refusedToken string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.access.token == refusedToken {
		a.access = access{}
	}
}

// Disconnect withdraws the grant and forgets it, and the account it was, so
// the next sign-in may be to any account. The registration and host ID stay.
// revoked is false when ChatGPT did not confirm the withdrawal; the owner can
// then remove ZeitBoard in ChatGPT's settings.
func (a *Account) Disconnect(ctx context.Context, deviceID string) (revoked bool, err error) {
	a.turns.Lock()
	defer a.turns.Unlock()
	current := a.current()
	revoked = true
	if g := current.Grant; g != nil && g.RefreshToken != "" {
		revoked = false
		if found, err := a.discover(ctx); err == nil {
			revoked = a.revoke(ctx, found, current.ClientID, g.RefreshToken)
		}
	}
	next := current
	next.Grant = nil
	if err := a.keep(ctx, next, "assistant_chatgpt_disconnected", deviceID); err != nil {
		return false, err
	}
	a.mu.Lock()
	a.access, a.models, a.pending, a.notice, a.reconsent = access{}, nil, nil, "", false
	a.mu.Unlock()
	return revoked, nil
}

// revoke withdraws a refresh token, trying twice, and reports whether ChatGPT
// confirmed it.
func (a *Account) revoke(ctx context.Context, found discovery, clientID, refresh string) bool {
	if found.Revocation == "" || refresh == "" {
		return false
	}
	form := url.Values{"token": {refresh}, "token_type_hint": {"refresh_token"}, "client_id": {clientID}}.Encode()
	ctx = context.WithoutCancel(ctx)
	for attempt := range 2 {
		if attempt > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		attemptCtx, cancel := context.WithTimeout(ctx, revokeTimeout)
		request, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, found.Revocation, strings.NewReader(form))
		if err != nil {
			cancel()
			return false
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		status, _, err := a.send(request)
		cancel()
		if err == nil && status == http.StatusOK {
			return true
		}
		if err == nil && status < 500 {
			return false
		}
	}
	return false
}

type tokenAnswer struct {
	AccessToken  string  `json:"access_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    float64 `json:"expires_in"`
	RefreshToken string  `json:"refresh_token"`
	IDToken      string  `json:"id_token"`
	Scope        string  `json:"scope"`
}

// tokenRequest posts a form to the token endpoint, giving its answer and when
// it came.
func (a *Account) tokenRequest(ctx context.Context, found discovery, form url.Values) (tokenAnswer, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, found.Token, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenAnswer{}, time.Time{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	status, body, err := a.send(request)
	receivedAt := a.now()
	if err != nil {
		return tokenAnswer{}, receivedAt, err
	}
	if status != http.StatusOK {
		return tokenAnswer{}, receivedAt, answered(status, body)
	}
	var answer tokenAnswer
	if json.Unmarshal(body, &answer) != nil {
		return tokenAnswer{}, receivedAt, fail(SignInEnded, "invalid_token_response")
	}
	return answer, receivedAt, nil
}

type tokens struct {
	access, refresh, idToken string
	lifetime                 time.Duration
	scopes                   []string
}

// readTokens checks a token answer: a bearer token with a lifetime, the
// scopes granted (the earlier ones when a refresh does not say, and the ones
// asked for when an exchange does not), and a refresh token on every refresh
// and whenever offline access was granted.
func readTokens(answer tokenAnswer, unstated []string, refreshing bool) (tokens, error) {
	invalid := fail(SignInEnded, "invalid_token_response")
	if !strings.EqualFold(answer.TokenType, "bearer") || answer.AccessToken == "" || answer.ExpiresIn <= 0 ||
		len(answer.AccessToken) > maxTokenBytes || len(answer.RefreshToken) > maxTokenBytes || len(answer.IDToken) > maxTokenBytes {
		return tokens{}, invalid
	}
	scopes := strings.Fields(answer.Scope)
	if len(scopes) == 0 {
		scopes = slices.Clone(unstated)
	}
	if answer.RefreshToken == "" && (refreshing || slices.Contains(scopes, "offline_access")) {
		return tokens{}, invalid
	}
	return tokens{
		access:   answer.AccessToken,
		refresh:  answer.RefreshToken,
		idToken:  answer.IDToken,
		lifetime: time.Duration(answer.ExpiresIn * float64(time.Second)),
		scopes:   scopes,
	}, nil
}

// randomValue is 32 random bytes, base64url: a state, a nonce or a verifier.
func randomValue() string {
	buffer := make([]byte, 32)
	_, _ = rand.Read(buffer)
	return base64.RawURLEncoding.EncodeToString(buffer)
}

// newHostID is a random version 4 UUID as a URN, as the DevKit makes them.
func newHostID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	text := hex.EncodeToString(raw[:])
	return "urn:uuid:" + text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:], nil
}
