package portal

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testOrigin   = "https://share.example.test"
	testPasscode = "open-sesame"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(delta time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(delta)
}

type harness struct {
	t       *testing.T
	store   *Store
	handler http.Handler
	clock   *clock
	token   string
	profile Profile
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	fixed := &clock{now: time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)}
	store, err := Open(filepath.Join(t.TempDir(), "portal.db"), make([]byte, 32))
	if err != nil {
		t.Fatalf("open portal store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	handler, err := NewHandler(HandlerConfig{
		Store:           store,
		Now:             fixed.Now,
		PublicOrigin:    testOrigin,
		ResolutionFloor: time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	h := &harness{t: t, store: store, handler: handler.Routes(), clock: fixed}
	h.token, h.profile = h.createProfile(Grants{WakingWindows: true})
	return h
}

func (h *harness) createProfile(grants Grants) (string, Profile) {
	h.t.Helper()
	profileID, err := NewProfileID()
	if err != nil {
		h.t.Fatalf("new profile id: %v", err)
	}
	token, err := NewLinkToken()
	if err != nil {
		h.t.Fatalf("new link token: %v", err)
	}
	now := h.clock.Now()
	if err := h.store.CreateProfile(context.Background(), CreateProfileInput{
		ProfileID: profileID,
		Token:     token,
		Passcode:  testPasscode,
		Grants:    grants,
		CreatedAt: now,
		ExpiresAt: now.Add(30 * 24 * time.Hour),
	}); err != nil {
		h.t.Fatalf("create profile: %v", err)
	}
	profile, err := h.store.ResolveLink(context.Background(), token, now)
	if err != nil {
		h.t.Fatalf("resolve new link: %v", err)
	}
	return token, profile
}

func (h *harness) publish(profileID string, snapshot Snapshot) {
	h.t.Helper()
	if err := h.store.PublishSnapshot(context.Background(), profileID, snapshot); err != nil {
		h.t.Fatalf("publish snapshot: %v", err)
	}
}

func (h *harness) sampleSnapshot() Snapshot {
	now := h.clock.Now()
	return Snapshot{
		Version:     now.UnixMilli(),
		GeneratedAt: now,
		Status:      StatusAvailable,
		HorizonEnd:  now.Add(30 * time.Hour),
		Windows: []Window{
			{StartAt: now.Add(-time.Hour), EndAt: now.Add(6 * time.Hour), ZoneID: "America/New_York"},
			{StartAt: now.Add(24 * time.Hour), EndAt: now.Add(30 * time.Hour), ZoneID: "America/New_York"},
		},
	}
}

func (h *harness) get(path string, cookie string) *httptest.ResponseRecorder {
	h.t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = "203.0.113.10:5555"
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

// login performs the passcode exchange using only an Origin header, which is
// the pre-Fetch-Metadata client shape.
func (h *harness) login(token, passcode, origin string) *httptest.ResponseRecorder {
	headers := map[string]string{}
	if origin != "" {
		headers["Origin"] = origin
	}
	return h.loginWithHeaders(token, passcode, headers)
}

func (h *harness) loginWithHeaders(token, passcode string, headers map[string]string) *httptest.ResponseRecorder {
	h.t.Helper()
	form := url.Values{"passcode": {passcode}}
	request := httptest.NewRequest(http.MethodPost, "/p/"+token+"/session", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.RemoteAddr = "203.0.113.10:5555"
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func (h *harness) sessionCookie(recorder *httptest.ResponseRecorder) string {
	h.t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie.Value
		}
	}
	h.t.Fatalf("no session cookie in response (status %d)", recorder.Code)
	return ""
}

// TestPortalPackageDoesNotReachPrivateData is the structural half of the
// boundary claim in docs/portal-design.md section 2. The public package must
// not be able to reach a sleep record by following an import, and a reviewer
// should not have to notice a new import to keep that true.
func TestPortalPackageDoesNotReachPrivateData(t *testing.T) {
	forbidden := []string{
		"non24.app/server/internal/store",
		"non24.app/server/internal/readmodel",
		"non24.app/server/internal/assistant",
		"non24.app/server/internal/api",
		"non24.app/server/internal/portalbridge",
		"non24.app/server/internal/provider",
		"non24.app/core/estimation",
		"non24.app/core/domain",
	}
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse portal package: %v", err)
	}
	for name, pkg := range packages {
		for path, file := range pkg.Files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			for _, imported := range file.Imports {
				value := strings.Trim(imported.Path.Value, `"`)
				for _, banned := range forbidden {
					if value == banned {
						t.Errorf("package %s file %s imports forbidden package %s", name, path, banned)
					}
				}
			}
		}
	}
}

// TestAvailabilityDTOAllowlist asserts the exact public JSON shape. Any new
// key is a widening of the public boundary and must fail here first.
func TestAvailabilityDTOAllowlist(t *testing.T) {
	h := newHarness(t)
	h.publish(h.profile.ID, h.sampleSnapshot())
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))

	recorder := h.get("/p/"+h.token+"/availability", cookie)
	if recorder.Code != http.StatusOK {
		t.Fatalf("availability status = %d, want 200", recorder.Code)
	}
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode availability: %v", err)
	}

	allowed := map[string]bool{"version": true, "windows": true, "generatedAt": true, "horizonEnd": true, "status": true}
	for key := range decoded {
		if !allowed[key] {
			t.Errorf("availability response has non-allowlisted key %q", key)
		}
	}
	for _, required := range []string{"version", "windows", "generatedAt", "horizonEnd", "status"} {
		if _, ok := decoded[required]; !ok {
			t.Errorf("availability response is missing %q", required)
		}
	}

	windowKeys := map[string]bool{"startAt": true, "endAt": true, "zoneId": true}
	windows, ok := decoded["windows"].([]any)
	if !ok || len(windows) == 0 {
		t.Fatalf("expected windows in availability response, got %v", decoded["windows"])
	}
	for _, entry := range windows {
		window, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("window is not an object: %v", entry)
		}
		for key := range window {
			if !windowKeys[key] {
				t.Errorf("window has non-allowlisted key %q", key)
			}
		}
	}
}

// TestUnknownExpiredRevokedAreIndistinguishable is the enumeration guard: all
// three must produce byte-identical responses.
func TestUnknownExpiredRevokedAreIndistinguishable(t *testing.T) {
	h := newHarness(t)

	expiredToken, expiredProfile := h.createProfile(Grants{WakingWindows: true})
	revokedToken, revokedProfile := h.createProfile(Grants{WakingWindows: true})
	if err := h.store.RevokeProfile(context.Background(), revokedProfile.ID, h.clock.Now()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_ = expiredProfile
	h.clock.Advance(31 * 24 * time.Hour)

	unknownToken, err := NewLinkToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}

	responses := make([]*httptest.ResponseRecorder, 0, 3)
	for _, token := range []string{unknownToken, expiredToken, revokedToken} {
		responses = append(responses, h.get("/p/"+token, ""))
	}
	for i, recorder := range responses {
		if recorder.Code != http.StatusGone {
			t.Errorf("response %d status = %d, want 410", i, recorder.Code)
		}
		if recorder.Body.String() != responses[0].Body.String() {
			t.Errorf("response %d body differs from the first; link states are distinguishable", i)
		}
	}
}

// TestRevocationIsImmediate covers the kill switch: an already-authenticated
// visitor must lose access at once, not when their cookie expires.
func TestRevocationIsImmediate(t *testing.T) {
	h := newHarness(t)
	h.publish(h.profile.ID, h.sampleSnapshot())
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))

	if recorder := h.get("/p/"+h.token, cookie); recorder.Code != http.StatusOK {
		t.Fatalf("pre-revocation status = %d, want 200", recorder.Code)
	}
	if err := h.store.RevokeProfile(context.Background(), h.profile.ID, h.clock.Now()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if recorder := h.get("/p/"+h.token, cookie); recorder.Code != http.StatusGone {
		t.Errorf("post-revocation status = %d, want 410", recorder.Code)
	}
	if _, err := h.store.ReadSnapshot(context.Background(), h.profile.ID); err == nil {
		t.Error("snapshot survived revocation; availability data must be deleted")
	}
}

func TestPasscodeGate(t *testing.T) {
	h := newHarness(t)
	h.publish(h.profile.ID, h.sampleSnapshot())

	page := h.get("/p/"+h.token, "")
	if page.Code != http.StatusOK {
		t.Fatalf("unauthenticated page status = %d, want 200", page.Code)
	}
	if !strings.Contains(page.Body.String(), "needs a passcode") {
		t.Error("unauthenticated visitor was not shown the passcode form")
	}
	if strings.Contains(page.Body.String(), "Likely awake") {
		t.Error("availability rendered before the passcode was accepted")
	}

	rejected := h.login(h.token, "wrong-passcode", testOrigin)
	if rejected.Code != http.StatusUnauthorized {
		t.Errorf("wrong passcode status = %d, want 401", rejected.Code)
	}

	// The failed attempt arms the backoff, so advance past it before the
	// legitimate visitor tries again.
	h.clock.Advance(2 * time.Second)
	accepted := h.login(h.token, testPasscode, testOrigin)
	if accepted.Code != http.StatusSeeOther {
		t.Fatalf("correct passcode status = %d, want 303", accepted.Code)
	}
	cookie := h.sessionCookie(accepted)
	dashboard := h.get("/p/"+h.token, cookie)
	if !strings.Contains(dashboard.Body.String(), "Likely awake right now") {
		t.Errorf("dashboard did not render the current state: %s", dashboard.Body.String())
	}
}

// TestPasscodeBackoffIsPerProfileAndSource proves progressive delay exists and
// that it does not become a global lockout an attacker could trigger.
func TestPasscodeBackoffIsPerProfileAndSource(t *testing.T) {
	h := newHarness(t)
	if recorder := h.login(h.token, "wrong", testOrigin); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("first failure status = %d, want 401", recorder.Code)
	}
	// Immediately retrying with the *correct* passcode is throttled, not
	// permanently refused.
	throttled := h.login(h.token, testPasscode, testOrigin)
	if throttled.Code != http.StatusTooManyRequests {
		t.Errorf("immediate retry status = %d, want 429", throttled.Code)
	}
	h.clock.Advance(5 * time.Second)
	if recorder := h.login(h.token, testPasscode, testOrigin); recorder.Code != http.StatusSeeOther {
		t.Errorf("retry after backoff status = %d, want 303", recorder.Code)
	}
}

func TestSessionIsBoundToOneProfile(t *testing.T) {
	h := newHarness(t)
	otherToken, otherProfile := h.createProfile(Grants{WakingWindows: true})
	h.publish(h.profile.ID, h.sampleSnapshot())
	h.publish(otherProfile.ID, h.sampleSnapshot())

	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))
	crossed := h.get("/p/"+otherToken+"/availability", cookie)
	if crossed.Code != http.StatusUnauthorized {
		t.Errorf("cross-link availability status = %d, want 401", crossed.Code)
	}
}

func TestSessionRequiresExactOrigin(t *testing.T) {
	h := newHarness(t)
	for name, origin := range map[string]string{
		"missing":     "",
		"foreign":     "https://evil.example",
		"subdomain":   "https://x.share.example.test",
		"scheme swap": "http://share.example.test",
	} {
		recorder := h.login(h.token, testPasscode, origin)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s origin status = %d, want 403", name, recorder.Code)
		}
	}
}

// TestSameOriginFormPostWithNullOrigin is the case a browser actually
// produces. Referrer-Policy: no-referrer makes a browser send `Origin: null`
// on a same-origin form submission, so gating on Origin alone would refuse
// every real login. Sec-Fetch-Site carries the truth.
func TestSameOriginFormPostWithNullOrigin(t *testing.T) {
	h := newHarness(t)
	recorder := h.loginWithHeaders(h.token, testPasscode, map[string]string{
		"Origin":         "null",
		"Sec-Fetch-Site": "same-origin",
		"Sec-Fetch-Mode": "navigate",
		"Sec-Fetch-Dest": "document",
	})
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("same-origin form post status = %d, want 303", recorder.Code)
	}
}

func TestFetchMetadataDecidesMutations(t *testing.T) {
	cases := map[string]struct {
		headers map[string]string
		want    int
	}{
		"same-origin without Origin": {
			map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusSeeOther,
		},
		"same-origin with matching Origin": {
			map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": testOrigin}, http.StatusSeeOther,
		},
		"cross-site even with a spoofed Origin": {
			map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": testOrigin}, http.StatusForbidden,
		},
		"same-site subdomain": {
			map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden,
		},
		"user-initiated navigation": {
			map[string]string{"Sec-Fetch-Site": "none"}, http.StatusForbidden,
		},
		"no metadata and no Origin": {
			map[string]string{}, http.StatusForbidden,
		},
		"no metadata with matching Origin": {
			map[string]string{"Origin": testOrigin}, http.StatusSeeOther,
		},
		"null Origin without metadata": {
			map[string]string{"Origin": "null"}, http.StatusForbidden,
		},
		"same-origin metadata but foreign Origin": {
			map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "https://evil.example"}, http.StatusForbidden,
		},
	}
	for name, testCase := range cases {
		h := newHarness(t)
		recorder := h.loginWithHeaders(h.token, testPasscode, testCase.headers)
		if recorder.Code != testCase.want {
			t.Errorf("%s: status = %d, want %d", name, recorder.Code, testCase.want)
		}
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	h := newHarness(t)
	h.publish(h.profile.ID, h.sampleSnapshot())
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))

	paths := []string{"/p/" + h.token, "/p/" + h.token + "/availability", "/p/assets/portal.css", "/p/nonexistent-token"}
	for _, path := range paths {
		recorder := h.get(path, cookie)
		header := recorder.Header()
		csp := header.Get("Content-Security-Policy")
		if !strings.Contains(csp, "default-src 'none'") {
			t.Errorf("%s missing default-src 'none': %q", path, csp)
		}
		if !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("%s missing frame-ancestors 'none'", path)
		}
		for _, directive := range []string{"style-src 'self'", "script-src 'self'", "connect-src 'self'", "img-src 'self'"} {
			if !strings.Contains(csp, directive) {
				t.Errorf("%s missing %q", path, directive)
			}
		}
		if got := header.Get("Cache-Control"); got != "no-store, max-age=0" {
			t.Errorf("%s Cache-Control = %q", path, got)
		}
		if got := header.Get("Referrer-Policy"); got != "no-referrer" {
			t.Errorf("%s Referrer-Policy = %q", path, got)
		}
		if got := header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s X-Content-Type-Options = %q", path, got)
		}
		if !strings.Contains(header.Get("X-Robots-Tag"), "noindex") {
			t.Errorf("%s is not marked noindex", path)
		}
	}
}

// TestPageHasNoInlineOrThirdPartyAssets keeps the CSP honest: a page with an
// inline style would render fine in a browser that ignores CSP and break in
// one that does not.
func TestPageHasNoInlineOrThirdPartyAssets(t *testing.T) {
	h := newHarness(t)
	h.publish(h.profile.ID, h.sampleSnapshot())
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))
	body := h.get("/p/"+h.token, cookie).Body.String()

	for _, forbidden := range []string{"<script", "style=\"", "<style", "http://", "https://"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("page contains %q, which the CSP forbids or which reaches a third party", forbidden)
		}
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	h := newHarness(t)
	recorder := h.login(h.token, testPasscode, testOrigin)
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name != sessionCookieName {
			continue
		}
		if !cookie.Secure {
			t.Error("session cookie is not Secure")
		}
		if !cookie.HttpOnly {
			t.Error("session cookie is not HttpOnly")
		}
		if cookie.SameSite != http.SameSiteStrictMode {
			t.Error("session cookie is not SameSite=Strict")
		}
		if cookie.Path != "/" {
			t.Errorf("session cookie Path = %q, want / for the __Host- prefix", cookie.Path)
		}
		if cookie.Domain != "" {
			t.Errorf("session cookie has Domain %q, which invalidates the __Host- prefix", cookie.Domain)
		}
		return
	}
	t.Fatal("no session cookie issued")
}

// TestSessionNeverOutlivesLink covers the case where link expiry is sooner
// than the 24-hour session lifetime.
func TestSessionNeverOutlivesLink(t *testing.T) {
	h := newHarness(t)
	now := h.clock.Now()
	profileID, _ := NewProfileID()
	token, _ := NewLinkToken()
	if err := h.store.CreateProfile(context.Background(), CreateProfileInput{
		ProfileID: profileID,
		Token:     token,
		Passcode:  testPasscode,
		Grants:    Grants{WakingWindows: true},
		CreatedAt: now,
		ExpiresAt: now.Add(2 * time.Hour),
	}); err != nil {
		t.Fatalf("create short link: %v", err)
	}
	profile, err := h.store.ResolveLink(context.Background(), token, now)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	session, err := h.store.CreateSession(context.Background(), profile, now)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.ExpiresAt.After(profile.ExpiresAt) {
		t.Errorf("session expires %v after link expiry %v", session.ExpiresAt, profile.ExpiresAt)
	}
}

func TestReadLimitThrottles(t *testing.T) {
	h := newHarness(t)
	h.publish(h.profile.ID, h.sampleSnapshot())

	for i := 0; i < ReadLimitPerHour; i++ {
		if recorder := h.get("/p/"+h.token, ""); recorder.Code == http.StatusTooManyRequests {
			t.Fatalf("throttled early at request %d", i)
		}
	}
	recorder := h.get("/p/"+h.token, "")
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("request past the limit status = %d, want 429", recorder.Code)
	}
	if recorder.Header().Get("Retry-After") == "" {
		t.Error("throttled response has no Retry-After")
	}

	// The window is fixed, so the budget returns after it rolls over.
	h.clock.Advance(ReadLimitWindow + time.Minute)
	if recorder := h.get("/p/"+h.token, ""); recorder.Code == http.StatusTooManyRequests {
		t.Error("still throttled after the window rolled over")
	}
}

func TestAccessAuditRejectsUnknownEvents(t *testing.T) {
	h := newHarness(t)
	err := h.store.RecordAccess(context.Background(), h.profile.ID, AccessEvent("visitor typed: hello"), "src", h.clock.Now())
	if err == nil {
		t.Fatal("RecordAccess accepted an event outside the closed set")
	}
}

// TestAuditKeyRotationDropsOldRows proves rotation is real rather than
// nominal: the identifiers produced under the old key are gone.
func TestAuditKeyRotationDropsOldRows(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	before, err := h.store.SourceID(ctx, "198.51.100.7:1234")
	if err != nil {
		t.Fatalf("source id: %v", err)
	}
	if err := h.store.RecordAccess(ctx, h.profile.ID, EventPageView, before, h.clock.Now()); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := h.store.RotateAuditKey(ctx, h.clock.Now(), 0); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	after, err := h.store.SourceID(ctx, "198.51.100.7:1234")
	if err != nil {
		t.Fatalf("source id after rotation: %v", err)
	}
	if before == after {
		t.Error("source identifier is unchanged after rotation")
	}
	summaries, err := h.store.SummarizeAccess(ctx, h.profile.ID, time.Time{})
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("audit rows survived rotation: %v", summaries)
	}
}

func TestSourceIDGroupsIPv6By64(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	first, err := h.store.SourceID(ctx, "[2001:db8:1:2::1]:443")
	if err != nil {
		t.Fatalf("source id: %v", err)
	}
	same, err := h.store.SourceID(ctx, "[2001:db8:1:2:ffff::9]:443")
	if err != nil {
		t.Fatalf("source id: %v", err)
	}
	other, err := h.store.SourceID(ctx, "[2001:db8:1:3::1]:443")
	if err != nil {
		t.Fatalf("source id: %v", err)
	}
	if first != same {
		t.Error("addresses in one /64 produced different identifiers, so rotating within a /64 defeats limits")
	}
	if first == other {
		t.Error("addresses in different /64s collapsed to one identifier")
	}
}

// TestResolutionFloorDefaultsToTheConstant stops a future change from
// silently disabling the enumeration timing floor by leaving the field unset.
func TestResolutionFloorDefaultsToTheConstant(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "portal.db"), make([]byte, 32))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for name, floor := range map[string]time.Duration{"unset": 0, "negative": -time.Second} {
		handler, err := NewHandler(HandlerConfig{Store: store, PublicOrigin: testOrigin, ResolutionFloor: floor})
		if err != nil {
			t.Fatalf("%s: new handler: %v", name, err)
		}
		if handler.resolutionFloor != resolutionFloor {
			t.Errorf("%s: resolutionFloor = %v, want %v", name, handler.resolutionFloor, resolutionFloor)
		}
	}
}

func TestHandlerRequiresOrigin(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "portal.db"), make([]byte, 32))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := NewHandler(HandlerConfig{Store: store}); err == nil {
		t.Error("handler was constructed without a public origin")
	}
}

func TestCreateProfileRejectsWeakInput(t *testing.T) {
	h := newHarness(t)
	now := h.clock.Now()
	cases := map[string]CreateProfileInput{
		"short passcode": {ProfileID: "a", Token: "t", Passcode: "abc", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		"no passcode":    {ProfileID: "b", Token: "t2", Passcode: "", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		"expiry in past": {ProfileID: "c", Token: "t3", Passcode: testPasscode, CreatedAt: now, ExpiresAt: now.Add(-time.Hour)},
		"over 90 days":   {ProfileID: "d", Token: "t4", Passcode: testPasscode, CreatedAt: now, ExpiresAt: now.Add(91 * 24 * time.Hour)},
	}
	for name, input := range cases {
		if err := h.store.CreateProfile(context.Background(), input); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// TestPurgeExpiredEnforcesRetention covers the maintenance sweep the daemon
// runs hourly. Retention that nothing enforces is not retention.
func TestPurgeExpiredEnforcesRetention(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	now := h.clock.Now()

	session, err := h.store.CreateSession(ctx, h.profile, now)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := h.store.RecordAccess(ctx, h.profile.ID, EventPageView, "src-old", now.Add(-AuditRetention-time.Hour)); err != nil {
		t.Fatalf("record old: %v", err)
	}
	if err := h.store.RecordAccess(ctx, h.profile.ID, EventPageView, "src-new", now); err != nil {
		t.Fatalf("record new: %v", err)
	}

	// Nothing is due yet.
	if err := h.store.PurgeExpired(ctx, now); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if _, err := h.store.ResolveSession(ctx, session.Session, now); err != nil {
		t.Errorf("a live session was purged: %v", err)
	}

	later := now.Add(SessionLifetime + time.Hour)
	if err := h.store.PurgeExpired(ctx, later); err != nil {
		t.Fatalf("purge later: %v", err)
	}
	if _, err := h.store.ResolveSession(ctx, session.Session, later); err == nil {
		t.Error("an expired session survived the sweep")
	}
	summaries, err := h.store.SummarizeAccess(ctx, h.profile.ID, time.Time{})
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	total := 0
	for _, summary := range summaries {
		total += summary.Count
	}
	if total != 1 {
		t.Errorf("audit row count after retention sweep = %d, want only the recent row", total)
	}
}

// TestSessionCSRFTokenRoundTrips guards the synchronizer token minted with
// every session. P5-a has no session-authenticated mutation to spend it on, so
// this keeps it from silently rotting before P5-b needs it.
func TestSessionCSRFTokenRoundTrips(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	now := h.clock.Now()

	issued, err := h.store.CreateSession(ctx, h.profile, now)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if issued.CSRF == "" || issued.CSRF == issued.Session {
		t.Fatal("session and CSRF tokens must both exist and differ")
	}
	resolved, err := h.store.ResolveSession(ctx, issued.Session, now)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !resolved.MatchesCSRF(issued.CSRF) {
		t.Error("the issued CSRF token does not match its session")
	}
	if resolved.MatchesCSRF("") || resolved.MatchesCSRF(issued.Session) {
		t.Error("CSRF comparison accepted a wrong value")
	}
}

func TestSnapshotRejectsUnknownStatus(t *testing.T) {
	h := newHarness(t)
	err := h.store.PublishSnapshot(context.Background(), h.profile.ID, Snapshot{
		Version:     1,
		GeneratedAt: h.clock.Now(),
		Status:      "probably_awake_ish",
	})
	if err == nil {
		t.Fatal("PublishSnapshot accepted a status outside the closed set")
	}
}
