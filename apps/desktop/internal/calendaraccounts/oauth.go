// Package calendaraccounts reads calendars from a Google or Microsoft account
// through the owner's own app registration (ADR-0064), and writes accepted
// times to one the owner owns once they allow it (ADR-0065). The owner
// registers an app with the provider and gives ZeitBoard its client id;
// ZeitBoard ships no credentials of its own. Sign-in is OAuth 2.0 with PKCE and
// a one-time loopback redirect, as RFC 8252 describes for installed apps. It
// asks only to read, unless the owner has turned writing on.
package calendaraccounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Provider names an account's maker.
type Provider string

const (
	Google    Provider = "google"
	Microsoft Provider = "microsoft"
)

// Valid reports whether p is a provider ZeitBoard reads.
func (p Provider) Valid() bool { return p == Google || p == Microsoft }

// Endpoints are where a provider signs in and serves calendars. They are
// fixed per provider; tests point them at a local server, and nothing the
// owner types ever chooses them.
type Endpoints struct {
	Authorize string
	Token     string
	API       string

	// Revoke withdraws a grant; Microsoft offers no such address, and its
	// grants are withdrawn in the account's own settings.
	Revoke string
}

// DefaultEndpoints are the providers' own.
func DefaultEndpoints(provider Provider) Endpoints {
	if provider == Microsoft {
		return Endpoints{
			Authorize: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			Token:     "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			API:       "https://graph.microsoft.com/v1.0",
		}
	}
	return Endpoints{
		Authorize: "https://accounts.google.com/o/oauth2/v2/auth",
		Token:     "https://oauth2.googleapis.com/token",
		API:       "https://www.googleapis.com/calendar/v3",
		Revoke:    "https://oauth2.googleapis.com/revoke",
	}
}

// scope is everything asked for: reading calendars, and adding events when
// writing is on; and for Microsoft a refresh token, which Google gives for
// access_type=offline instead. Google's write scope reaches only calendars
// the owner owns.
func scope(provider Provider, write bool) string {
	switch {
	case provider == Microsoft && write:
		return "offline_access Calendars.ReadWrite"
	case provider == Microsoft:
		return "offline_access Calendars.Read"
	case write:
		return "https://www.googleapis.com/auth/calendar.readonly https://www.googleapis.com/auth/calendar.events.owned"
	default:
		return "https://www.googleapis.com/auth/calendar.readonly"
	}
}

// Writes reports whether a granted scope, as a provider states it, lets
// ZeitBoard add events. Google lets the owner grant part of what is asked.
func Writes(provider Provider, granted string) bool {
	for _, each := range strings.Fields(granted) {
		name := strings.ToLower(each[strings.LastIndex(each, "/")+1:])
		switch {
		case provider == Microsoft && name == "calendars.readwrite":
			return true
		case provider == Google && (name == "calendar.events.owned" || name == "calendar.events" || name == "calendar"):
			return true
		}
	}
	return false
}

// Doer sends a request; *http.Client is one.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client talks to one provider for one app registration.
type Client struct {
	Provider Provider
	ClientID string

	// ClientSecret is Google's "Desktop app" secret, which Google says is not
	// confidential for an installed app but still asks for. Microsoft's
	// public clients have none.
	ClientSecret string

	Endpoints Endpoints
	HTTP      Doer

	// Write asks to add events too; set once the owner turns writing on.
	Write bool
}

// Authorization is one sign-in begun: the address to open in the browser,
// and what proves the answer is to this request.
type Authorization struct {
	URL      string
	State    string
	Verifier string
	Redirect string
}

// Begin prepares a sign-in whose answer comes back to redirect.
func (c Client) Begin(redirect string) (Authorization, error) {
	if !c.Provider.Valid() || strings.TrimSpace(c.ClientID) == "" {
		return Authorization{}, errors.New("an account needs a provider and a client id")
	}
	verifier, err := randomToken(48)
	if err != nil {
		return Authorization{}, err
	}
	state, err := randomToken(24)
	if err != nil {
		return Authorization{}, err
	}
	query := url.Values{
		"client_id":             {c.ClientID},
		"redirect_uri":          {redirect},
		"response_type":         {"code"},
		"scope":                 {scope(c.Provider, c.Write)},
		"state":                 {state},
		"code_challenge":        {Challenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	if c.Provider == Google {
		// A refresh token comes only with offline access, and only on a
		// consent the owner has just given. Signing in again keeps what was
		// granted before, so reading anew never takes writing away.
		query.Set("access_type", "offline")
		query.Set("prompt", "consent")
		query.Set("include_granted_scopes", "true")
	}
	return Authorization{
		URL:      c.Endpoints.Authorize + "?" + query.Encode(),
		State:    state,
		Verifier: verifier,
		Redirect: redirect,
	}, nil
}

// Challenge is PKCE's S256 transform of a verifier (RFC 7636 §4.2).
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// Token is what the provider grants. Scope is what the owner consented to,
// as the provider states it; a provider that does not say leaves it empty.
type Token struct {
	Access    string
	Refresh   string
	ExpiresAt time.Time
	Scope     string
}

// Exchange trades the code the browser brought back for tokens.
func (c Client) Exchange(ctx context.Context, auth Authorization, code string) (Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {auth.Redirect},
		"code_verifier": {auth.Verifier},
	}
	token, err := c.token(ctx, form)
	if err != nil {
		return Token{}, err
	}
	if token.Refresh == "" {
		return Token{}, errors.New("the provider granted no lasting access; sign in again and allow offline access")
	}
	return token, nil
}

// ErrSignInEnded is a refresh the provider refused: the owner withdrew the
// grant, or it expired, as Google's do after seven days for an app still in
// testing. Only signing in again restores it.
var ErrSignInEnded = errors.New("the sign-in has ended")

// refusal is an OAuth error code a provider answered with (RFC 6749 §5.2).
type refusal string

func (r refusal) Error() string { return "the provider refused: " + string(r) }

// Refresh gets a new access token. A provider that rotates refresh tokens
// returns a new one, which replaces the old; one that does not keeps it.
func (c Client) Refresh(ctx context.Context, refresh string) (Token, error) {
	token, err := c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	if refused := refusal(""); errors.As(err, &refused) && refused == "invalid_grant" {
		return Token{}, ErrSignInEnded
	}
	if err != nil {
		return Token{}, err
	}
	if token.Refresh == "" {
		token.Refresh = refresh
	}
	return token, nil
}

func (c Client) token(ctx context.Context, form url.Values) (Token, error) {
	form.Set("client_id", c.ClientID)
	if c.ClientSecret != "" {
		form.Set("client_secret", c.ClientSecret)
	}
	if c.Provider == Microsoft {
		form.Set("scope", scope(c.Provider, c.Write))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoints.Token, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	var answer struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	status, err := c.send(request, &answer)
	if err != nil {
		return Token{}, err
	}
	if status != http.StatusOK || answer.AccessToken == "" {
		if answer.Error != "" {
			return Token{}, refusal(answer.Error)
		}
		return Token{}, fmt.Errorf("the provider answered HTTP %d", status)
	}
	expires := answer.ExpiresIn
	if expires <= 0 {
		expires = 3600
	}
	granted := strings.TrimSpace(answer.Scope)
	if granted == "" {
		// Left out, the scope is the one asked for (RFC 6749 §5.1).
		granted = scope(c.Provider, c.Write)
	}
	return Token{
		Access:    answer.AccessToken,
		Refresh:   answer.RefreshToken,
		ExpiresAt: time.Now().Add(time.Duration(expires) * time.Second),
		Scope:     granted,
	}, nil
}

// Revoke withdraws the grant a refresh token carries, where the provider
// offers that, and reports whether it confirmed it.
func (c Client) Revoke(ctx context.Context, refresh string) bool {
	if c.Endpoints.Revoke == "" {
		return false
	}
	form := url.Values{"token": {refresh}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoints.Revoke, strings.NewReader(form.Encode()))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	status, err := c.send(request, &struct{}{})
	return err == nil && status == http.StatusOK
}

// maxResponseBytes bounds what one answer may be.
const maxResponseBytes = 16 << 20

// send performs a request and decodes a JSON answer into value. No error it
// returns holds a token or a response body.
func (c Client) send(request *http.Request, value any) (int, error) {
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("%s could not be reached", request.URL.Hostname())
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return response.StatusCode, fmt.Errorf("%s's answer could not be read", request.URL.Hostname())
	}
	if len(data) > maxResponseBytes {
		return response.StatusCode, fmt.Errorf("%s's answer is larger than 16 MiB", request.URL.Hostname())
	}
	if len(data) > 0 && json.Unmarshal(data, value) != nil && response.StatusCode/100 == 2 {
		return response.StatusCode, fmt.Errorf("%s's answer could not be read", request.URL.Hostname())
	}
	return response.StatusCode, nil
}
