// Package chatgpttest is a synthetic OpenAI for tests: the issuer that signs
// the owner in, and the API the assistant asks. It grants made-up tokens to a
// made-up account and holds nobody's data.
package chatgpttest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// FullScope is everything a sign-in asks for.
const FullScope = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"

// Server is the synthetic OpenAI. Its fields set how it answers; lock it
// with Lock and Unlock to change them while requests run.
type Server struct {
	t      testing.TB
	issuer *httptest.Server
	api    *httptest.Server
	key    *rsa.PrivateKey

	sync.Mutex
	// Scope is what a sign-in grants; empty grants FullScope.
	Scope string
	// Subject and Email are the account signed in to.
	Subject, Email string
	// ExpiresIn is an access token's lifetime in seconds.
	ExpiresIn int
	// RefuseRefresh, when set, is the OAuth error a refresh answers with.
	RefuseRefresh string
	// RefreshScope, when set, is the scope a refresh states; otherwise a
	// refresh leaves it unsaid, as it may.
	RefreshScope string
	// KeepRefreshToken answers a refresh without a new refresh token, the
	// old one staying in force, as RFC 6749 section 6 allows.
	KeepRefreshToken bool
	// Claims, when set, changes an ID token's claims before it is signed.
	Claims func(map[string]any)
	// SignWith, when set, signs ID tokens with another key than the one
	// published; KeyID, when set, names another key.
	SignWith *rsa.PrivateKey
	KeyID    string
	// Models are the catalog's entries, as ChatGPT gives them.
	Models []map[string]any
	// Reply is the text a response streams.
	Reply string
	// Stream, when set, replaces a response's events with these payloads.
	Stream []string
	// Status and Body, when Status is set, answer a response request.
	Status int
	Body   string

	// What happened.
	Refreshes     int
	Revoked       []string
	ResponseCalls int
	LastResponse  map[string]any
	LastAccess    string

	clientID string
	codes    map[string]grantedCode
	refresh  map[string]bool // refresh tokens issued, and whether still live
	access   map[string]bool
}

type grantedCode struct {
	challenge, redirect, clientID, nonce string
	used                                 bool
}

// New starts the synthetic OpenAI.
func New(t testing.TB) *Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		t: t, key: key, Subject: "user-synthetic-1", Email: "owner@example.com", ExpiresIn: 3600,
		Models: []map[string]any{
			{"slug": "synthetic-fast", "display_name": "Synthetic Fast", "visibility": "list"},
			{"slug": "synthetic-deep", "display_name": "Synthetic Deep", "visibility": "list"},
			{"slug": "synthetic-hidden", "display_name": "Synthetic Hidden", "visibility": "hide"},
		},
		Reply:   `{"schema_version":"v1","recommended_action":"answer_only","answer":"Synthetic answer."}`,
		codes:   map[string]grantedCode{},
		refresh: map[string]bool{},
		access:  map[string]bool{},
	}
	s.issuer = httptest.NewServer(http.HandlerFunc(s.serveIssuer))
	s.api = httptest.NewServer(http.HandlerFunc(s.serveAPI))
	t.Cleanup(s.issuer.Close)
	t.Cleanup(s.api.Close)
	return s
}

// Issuer is the issuer's address; API is the API's, with its /v1.
func (s *Server) Issuer() string { return s.issuer.URL }
func (s *Server) API() string    { return s.api.URL + "/v1" }

// Approve does what the owner's browser does with an authorization address:
// it checks the request, consents, and gives back what the browser would
// bring to the redirect.
func (s *Server) Approve(address string) (state, code, clientID string) {
	s.t.Helper()
	parsed, err := url.Parse(address)
	if err != nil {
		s.t.Fatal(err)
	}
	query := parsed.Query()
	for name, want := range map[string]string{
		"response_type": "code", "scope": FullScope, "resource": s.API(), "code_challenge_method": "S256",
	} {
		if got := query.Get(name); got != want {
			s.t.Errorf("authorization %s is %q, not %q", name, got, want)
		}
	}
	s.Lock()
	defer s.Unlock()
	clientID = query.Get("client_id")
	if clientID == "dynamic_agent_client" {
		if s.clientID == "" {
			s.clientID = "client-synthetic-" + randomText(6)
		}
		clientID = s.clientID
	}
	code = "code-" + randomText(12)
	s.codes[code] = grantedCode{challenge: query.Get("code_challenge"), redirect: query.Get("redirect_uri"),
		clientID: clientID, nonce: query.Get("nonce")}
	return query.Get("state"), code, clientID
}

// RefreshToken is a refresh token still live, if one is.
func (s *Server) RefreshToken() string {
	s.Lock()
	defer s.Unlock()
	for token, live := range s.refresh {
		if live {
			return token
		}
	}
	return ""
}

// ExpireAccess makes every access token issued so far refused.
func (s *Server) ExpireAccess() {
	s.Lock()
	defer s.Unlock()
	clear(s.access)
}

func (s *Server) serveIssuer(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		writeJSON(w, http.StatusOK, map[string]string{
			"issuer":                 s.Issuer(),
			"authorization_endpoint": s.Issuer() + "/oauth/authorize",
			"token_endpoint":         s.Issuer() + "/oauth/token",
			"revocation_endpoint":    s.Issuer() + "/oauth/revoke",
			"jwks_uri":               s.Issuer() + "/.well-known/jwks.json",
		})
	case "/.well-known/jwks.json":
		writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "synthetic-key", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(s.key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.key.E)).Bytes()),
		}}})
	case "/oauth/token":
		s.serveToken(w, r)
	case "/oauth/revoke":
		if err := r.ParseForm(); err != nil || r.PostForm.Get("token_type_hint") != "refresh_token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		s.Lock()
		s.Revoked = append(s.Revoked, r.PostForm.Get("token"))
		s.refresh[r.PostForm.Get("token")] = false
		s.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) serveToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	form := r.PostForm
	s.Lock()
	defer s.Unlock()
	if form.Get("resource") != s.API() || form.Get("client_id") == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	nonce := ""
	switch form.Get("grant_type") {
	case "authorization_code":
		granted, ok := s.codes[form.Get("code")]
		sum := sha256.Sum256([]byte(form.Get("code_verifier")))
		if !ok || granted.used || granted.clientID != form.Get("client_id") || granted.redirect != form.Get("redirect_uri") ||
			base64.RawURLEncoding.EncodeToString(sum[:]) != granted.challenge {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
		}
		granted.used = true
		s.codes[form.Get("code")] = granted
		nonce = granted.nonce
	case "refresh_token":
		if form.Get("scope") != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_scope"})
			return
		}
		s.Refreshes++
		if s.RefuseRefresh != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": s.RefuseRefresh})
			return
		}
		live, known := s.refresh[form.Get("refresh_token")]
		if !known || !live {
			code := "invalid_grant"
			if known {
				code = "refresh_token_reused"
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": code})
			return
		}
		if s.KeepRefreshToken {
			access := "access-" + randomText(16)
			s.access[access] = true
			writeJSON(w, http.StatusOK, map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": s.ExpiresIn})
			return
		}
		s.refresh[form.Get("refresh_token")] = false
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	scope := s.Scope
	if scope == "" {
		scope = FullScope
	}
	access, refresh := "access-"+randomText(16), "refresh-"+randomText(16)
	s.access[access], s.refresh[refresh] = true, true
	answer := map[string]any{
		"access_token": access, "token_type": "Bearer", "expires_in": s.ExpiresIn,
		"refresh_token": refresh, "id_token": s.idToken(form.Get("client_id"), nonce),
	}
	// A refresh leaves the scope unsaid, as it may, unless told to state one.
	if form.Get("grant_type") == "authorization_code" {
		answer["scope"] = scope
	} else if s.RefreshScope != "" {
		answer["scope"] = s.RefreshScope
	}
	writeJSON(w, http.StatusOK, answer)
}

// idToken signs an ID token for the account. Callers hold the lock.
func (s *Server) idToken(clientID, nonce string) string {
	now := time.Now()
	claims := map[string]any{
		"iss": s.Issuer(), "aud": clientID, "sub": s.Subject, "email": s.Email,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	if s.Claims != nil {
		s.Claims(claims)
	}
	key, keyID := s.key, "synthetic-key"
	if s.SignWith != nil {
		key = s.SignWith
	}
	if s.KeyID != "" {
		keyID = s.KeyID
	}
	return s.Sign(map[string]any{"alg": "RS256", "kid": keyID}, claims, key)
}

// Sign makes a compact JWT with key.
func (s *Server) Sign(header, claims map[string]any, key *rsa.PrivateKey) string {
	h, _ := json.Marshal(header)
	c, _ := json.Marshal(claims)
	signed := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
	digest := sha256.Sum256([]byte(signed))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		s.t.Fatal(err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	s.Lock()
	live := s.access[token]
	s.LastAccess = token
	s.Unlock()
	if !live {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "invalid_token"}})
		return
	}
	switch r.URL.Path {
	case "/v1/models":
		s.Lock()
		models := s.Models
		s.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"models": models})
	case "/v1/responses":
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		s.Lock()
		s.ResponseCalls++
		s.LastResponse = body
		status, failure, events, reply := s.Status, s.Body, s.Stream, s.Reply
		s.Unlock()
		if status != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			fmt.Fprint(w, failure)
			return
		}
		if events == nil {
			events = []string{`{"type":"response.created"}`}
			half := len(reply) / 2
			for _, part := range []string{reply[:half], reply[half:]} {
				delta, _ := json.Marshal(map[string]string{"type": "response.output_text.delta", "delta": part})
				events = append(events, string(delta))
			}
			events = append(events, `{"type":"response.completed"}`)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			var parsed struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal([]byte(event), &parsed)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", parsed.Type, event)
		}
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func randomText(bytes int) string {
	buffer := make([]byte, bytes)
	_, _ = rand.Read(buffer)
	return base64.RawURLEncoding.EncodeToString(buffer)
}
