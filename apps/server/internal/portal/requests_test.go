package portal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func (h *harness) requestHarness(t *testing.T) (string, Profile, string) {
	t.Helper()
	token, profile := h.createProfile(Grants{WakingWindows: true, AllowRequests: true})
	h.publish(profile.ID, h.sampleSnapshot())
	recorder := h.loginWithHeaders(token, testPasscode, map[string]string{"Sec-Fetch-Site": "same-origin"})
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d", recorder.Code)
	}
	return token, profile, h.sessionCookie(recorder)
}

func (h *harness) submitRequest(t *testing.T, token, cookie string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	form.Set("csrf", h.store.CSRFToken(cookie))
	request := httptest.NewRequest(http.MethodPost, "/p/"+token+"/requests", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.RemoteAddr = "203.0.113.10:5555"
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func validRequestForm(now time.Time) url.Values {
	start := now.Add(30 * time.Hour).UTC()
	return url.Values{
		"zone_id":      {"UTC"},
		"window_start": {start.Format("2006-01-02T15:04")},
		"window_end":   {start.Add(4 * time.Hour).Format("2006-01-02T15:04")},
		"handle":       {"Sam"},
		"message":      {"Coffee?"},
	}
}

func TestRequestRequiresTheGrant(t *testing.T) {
	h := newHarness(t)
	// The default fixture profile has WakingWindows but not AllowRequests.
	h.publish(h.profile.ID, h.sampleSnapshot())
	cookie := h.sessionCookie(h.loginWithHeaders(h.token, testPasscode, map[string]string{"Sec-Fetch-Site": "same-origin"}))

	if recorder := h.get("/p/"+h.token+"/requests", cookie); recorder.Code != http.StatusForbidden {
		t.Errorf("request form status = %d, want 403 without the grant", recorder.Code)
	}
	if recorder := h.submitRequest(t, h.token, cookie, validRequestForm(h.clock.Now())); recorder.Code != http.StatusForbidden {
		t.Errorf("request POST status = %d, want 403 without the grant", recorder.Code)
	}
}

func TestRequestCreationRequiresCSRF(t *testing.T) {
	h := newHarness(t)
	token, _, cookie := h.requestHarness(t)

	form := validRequestForm(h.clock.Now())
	form.Set("csrf", "not-the-token")
	request := httptest.NewRequest(http.MethodPost, "/p/"+token+"/requests", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.RemoteAddr = "203.0.113.10:5555"
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a wrong synchronizer token", recorder.Code)
	}
}

func TestRequestCreationShowsRecoveryCodeOnce(t *testing.T) {
	h := newHarness(t)
	token, profile, cookie := h.requestHarness(t)

	recorder := h.submitRequest(t, token, cookie, validRequestForm(h.clock.Now()))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "Keep this code") {
		t.Error("the one-time recovery code was not shown")
	}
	// A queued request must not claim it has been delivered.
	if !strings.Contains(body, "on its way") {
		t.Errorf("queued request did not describe itself honestly: %s", body)
	}

	ids, err := h.store.ListRequestsForProfile(context.Background(), profile.ID)
	if err != nil || len(ids) != 1 {
		t.Fatalf("stored request count = %d err = %v", len(ids), err)
	}
	stored, err := h.store.ReadRequest(context.Background(), profile.ID, ids[0])
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if stored.Status != RequestQueued {
		t.Errorf("status = %q, want %q before the bridge confirms", stored.Status, RequestQueued)
	}
	if stored.Handle != "Sam" || stored.Message != "Coffee?" {
		t.Errorf("visitor text was not preserved: %+v", stored)
	}
}

// TestRequestSecretIsNotInTheURLPath keeps the requester secret out of every
// place a server or proxy would log it.
func TestRequestSecretIsNotInTheURLPath(t *testing.T) {
	h := newHarness(t)
	token, _, cookie := h.requestHarness(t)
	recorder := h.submitRequest(t, token, cookie, validRequestForm(h.clock.Now()))
	body := recorder.Body.String()

	// The continue link must carry the secret only after a '#'.
	index := strings.Index(body, "/requests/")
	if index < 0 {
		t.Fatal("no request link was rendered")
	}
	link := body[index:]
	if end := strings.IndexAny(link, `"`); end >= 0 {
		link = link[:end]
	}
	fragment := strings.Index(link, "#")
	if fragment < 0 {
		t.Fatalf("continue link has no fragment: %q", link)
	}
	if strings.Contains(link[:fragment], "s=") || strings.Contains(link[:fragment], "secret") {
		t.Errorf("the requester secret appears before the fragment: %q", link)
	}
}

func TestRequestValidationRules(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	horizon := now.Add(7 * 24 * time.Hour)
	base := func() RequestInput {
		return RequestInput{
			WindowStart: now.Add(2 * time.Hour),
			WindowEnd:   now.Add(6 * time.Hour),
			ZoneID:      "UTC",
		}
	}
	cases := map[string]func(*RequestInput){
		"end before start":  func(i *RequestInput) { i.WindowEnd = i.WindowStart.Add(-time.Hour) },
		"window in past":    func(i *RequestInput) { i.WindowStart = now.Add(-time.Hour) },
		"window over 8h":    func(i *RequestInput) { i.WindowEnd = i.WindowStart.Add(9 * time.Hour) },
		"duration too low":  func(i *RequestInput) { i.DurationMinutes = 5 },
		"duration too high": func(i *RequestInput) { i.DurationMinutes = 600 },
		"duration > window": func(i *RequestInput) { i.DurationMinutes = 300 },
		"unknown zone":      func(i *RequestInput) { i.ZoneID = "Mars/Olympus" },
		"handle too long":   func(i *RequestInput) { i.Handle = strings.Repeat("x", MaxHandleRunes+1) },
		"message too long":  func(i *RequestInput) { i.Message = strings.Repeat("x", MaxMessageRunes+1) },
	}
	for name, mutate := range cases {
		input := base()
		mutate(&input)
		if _, _, err := ValidateRequest(input, now, horizon); err == nil {
			t.Errorf("%s was accepted", name)
		} else if !errors.Is(err, ErrRequestInvalid) {
			t.Errorf("%s returned an untyped error: %v", name, err)
		}
	}

	valid, beyond, err := ValidateRequest(base(), now, horizon)
	if err != nil {
		t.Fatalf("a valid request was rejected: %v", err)
	}
	if beyond {
		t.Error("a request inside the horizon was flagged beyond it")
	}
	if valid.ZoneID != "UTC" {
		t.Errorf("zone = %q", valid.ZoneID)
	}
}

// TestBeyondHorizonIsAcceptedWithAWarning is owner decision 3: no product cap
// on how far ahead someone may ask, but the infeasibility must be stated.
func TestBeyondHorizonIsAcceptedWithAWarning(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	horizon := now.Add(7 * 24 * time.Hour)
	input := RequestInput{
		WindowStart: now.Add(90 * 24 * time.Hour),
		WindowEnd:   now.Add(90*24*time.Hour + 3*time.Hour),
		ZoneID:      "UTC",
	}
	_, beyond, err := ValidateRequest(input, now, horizon)
	if err != nil {
		t.Fatalf("a far-future request was refused: %v", err)
	}
	if !beyond {
		t.Fatal("a request past the horizon was not flagged")
	}

	// And the flag must reach the visitor as an explicit warning.
	h := newHarness(t)
	token, _, cookie := h.requestHarness(t)
	form := validRequestForm(h.clock.Now())
	start := h.clock.Now().Add(120 * 24 * time.Hour).UTC()
	form.Set("window_start", start.Format("2006-01-02T15:04"))
	form.Set("window_end", start.Add(3*time.Hour).Format("2006-01-02T15:04"))

	body := h.submitRequest(t, token, cookie, form).Body.String()
	if !strings.Contains(body, "further ahead than this estimate can reach") {
		t.Errorf("no infeasibility warning was shown: %s", body)
	}
}

func TestRequestSanitizesControlCharacters(t *testing.T) {
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	input := RequestInput{
		WindowStart: now.Add(time.Hour),
		WindowEnd:   now.Add(4 * time.Hour),
		ZoneID:      "UTC",
		Handle:      "Sam\x00\x1b[31m",
		Message:     "line one\nline two\ttabbed",
	}
	validated, _, err := ValidateRequest(input, now, now.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if strings.ContainsAny(validated.Handle, "\x00\x1b") {
		t.Errorf("control characters survived: %q", validated.Handle)
	}
	if strings.ContainsAny(validated.Message, "\n\t") {
		t.Errorf("newlines and tabs were not folded: %q", validated.Message)
	}
	if !strings.Contains(validated.Message, "line one line two tabbed") {
		t.Errorf("message content was mangled: %q", validated.Message)
	}
}

func TestRequestPerSessionDailyCap(t *testing.T) {
	h := newHarness(t)
	token, _, cookie := h.requestHarness(t)

	for i := 0; i < RequestsPerSessionDay; i++ {
		if recorder := h.submitRequest(t, token, cookie, validRequestForm(h.clock.Now())); recorder.Code != http.StatusOK {
			t.Fatalf("request %d status = %d", i, recorder.Code)
		}
	}
	recorder := h.submitRequest(t, token, cookie, validRequestForm(h.clock.Now()))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status past the daily cap = %d, want 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "request limit") {
		t.Errorf("the cap was not explained: %s", recorder.Body.String())
	}
}

// TestRequestStatusRequiresProofOfAuthorship is the isolation property:
// holding the shared link and passcode is not enough to read someone else's
// request.
func TestRequestStatusRequiresProofOfAuthorship(t *testing.T) {
	h := newHarness(t)
	token, profile, cookie := h.requestHarness(t)
	h.submitRequest(t, token, cookie, validRequestForm(h.clock.Now()))
	ids, err := h.store.ListRequestsForProfile(context.Background(), profile.ID)
	if err != nil || len(ids) != 1 {
		t.Fatalf("setup: %v", err)
	}
	requestID := ids[0]

	// A second visitor with the same link but no requester cookie.
	page := h.get("/p/"+token+"/requests/"+requestID, cookie)
	if page.Code != http.StatusOK {
		t.Fatalf("status = %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "Enter the code") {
		t.Error("an unauthorized reader was not asked for the code")
	}
	for _, secret := range []string{"Sam", "Coffee?"} {
		if strings.Contains(body, secret) {
			t.Errorf("another visitor's request text %q was disclosed", secret)
		}
	}
}

func TestRequestSecretExchangeGrantsAccess(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	token, profile, cookie := h.requestHarness(t)

	created, err := h.store.CreateRequest(ctx, profile, cookie, RequestInput{
		WindowStart: h.clock.Now().Add(3 * time.Hour),
		WindowEnd:   h.clock.Now().Add(6 * time.Hour),
		ZoneID:      "UTC",
		Handle:      "Ada",
	}, false, h.clock.Now())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	form := url.Values{"secret": {created.Secret}}
	request := httptest.NewRequest(http.MethodPost,
		"/p/"+token+"/requests/"+created.Request.ID+"/session", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.RemoteAddr = "203.0.113.10:5555"
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("exchange status = %d", recorder.Code)
	}
	var requestCookie string
	for _, c := range recorder.Result().Cookies() {
		if c.Name == requestCookieName {
			requestCookie = c.Value
		}
	}
	if requestCookie == "" {
		t.Fatal("no request cookie was issued")
	}

	page := httptest.NewRequest(http.MethodGet, "/p/"+token+"/requests/"+created.Request.ID, nil)
	page.RemoteAddr = "203.0.113.10:5555"
	page.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	page.AddCookie(&http.Cookie{Name: requestCookieName, Value: requestCookie})
	pageRecorder := httptest.NewRecorder()
	h.handler.ServeHTTP(pageRecorder, page)

	if !strings.Contains(pageRecorder.Body.String(), "Ada") {
		t.Errorf("the author could not read their own request: %s", pageRecorder.Body.String())
	}
}

func TestWrongRequestSecretIsIndistinguishableFromUnknown(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	token, profile, cookie := h.requestHarness(t)
	created, err := h.store.CreateRequest(ctx, profile, cookie, RequestInput{
		WindowStart: h.clock.Now().Add(3 * time.Hour),
		WindowEnd:   h.clock.Now().Add(6 * time.Hour),
		ZoneID:      "UTC",
	}, false, h.clock.Now())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	exchange := func(requestID, secret string) *httptest.ResponseRecorder {
		form := url.Values{"secret": {secret}}
		request := httptest.NewRequest(http.MethodPost,
			"/p/"+token+"/requests/"+requestID+"/session", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.RemoteAddr = "203.0.113.10:5555"
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
		recorder := httptest.NewRecorder()
		h.handler.ServeHTTP(recorder, request)
		return recorder
	}

	wrongSecret := exchange(created.Request.ID, "definitely-not-the-secret")
	unknownRequest := exchange("no-such-request-id", "definitely-not-the-secret")
	if wrongSecret.Code != unknownRequest.Code {
		t.Errorf("statuses differ: %d vs %d", wrongSecret.Code, unknownRequest.Code)
	}
	if wrongSecret.Body.String() != unknownRequest.Body.String() {
		t.Error("bodies differ, so requests are enumerable")
	}
}

func TestRevocationClosesOpenRequests(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	token, profile, cookie := h.requestHarness(t)
	h.submitRequest(t, token, cookie, validRequestForm(h.clock.Now()))

	if err := h.store.RevokeProfile(ctx, profile.ID, h.clock.Now()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	ids, err := h.store.ListRequestsForProfile(ctx, profile.ID)
	if err != nil || len(ids) != 1 {
		t.Fatalf("list: %v", err)
	}
	request, err := h.store.ReadRequest(ctx, profile.ID, ids[0])
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if request.Status != RequestClosed {
		t.Errorf("status after revocation = %q, want %q", request.Status, RequestClosed)
	}
}

func TestDeclinedRequestRevealsNoReason(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, profile, cookie := h.requestHarness(t)
	created, err := h.store.CreateRequest(ctx, profile, cookie, RequestInput{
		WindowStart: h.clock.Now().Add(3 * time.Hour),
		WindowEnd:   h.clock.Now().Add(6 * time.Hour),
		ZoneID:      "UTC",
	}, false, h.clock.Now())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := h.store.ApplyDecision(ctx, created.Request.ID, RequestDeclined, time.Time{}, time.Time{}, h.clock.Now()); err != nil {
		t.Fatalf("decline: %v", err)
	}
	stored, err := h.store.ReadRequest(ctx, profile.ID, created.Request.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	view := h.handlerView(stored)
	lowered := strings.ToLower(view.StatusLabel + " " + view.StatusDetail)
	for _, forbidden := range []string{"asleep", "sleep", "busy", "calendar", "conflict", "medication", "because"} {
		if strings.Contains(lowered, forbidden) {
			t.Errorf("a declined request disclosed %q: %q", forbidden, view.StatusDetail)
		}
	}
}

// handlerView exposes requestView for assertions without exporting it.
func (h *harness) handlerView(request Request) RequestView {
	handler, err := NewHandler(HandlerConfig{
		Store:           h.store,
		Now:             h.clock.Now,
		PublicOrigin:    testOrigin,
		ResolutionFloor: time.Nanosecond,
	})
	if err != nil {
		h.t.Fatalf("handler: %v", err)
	}
	return handler.requestView(request)
}

func TestApplyDecisionIsIdempotent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, profile, cookie := h.requestHarness(t)
	created, err := h.store.CreateRequest(ctx, profile, cookie, RequestInput{
		WindowStart: h.clock.Now().Add(3 * time.Hour),
		WindowEnd:   h.clock.Now().Add(6 * time.Hour),
		ZoneID:      "UTC",
	}, false, h.clock.Now())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	slotStart := h.clock.Now().Add(4 * time.Hour)
	slotEnd := slotStart.Add(time.Hour)

	for i := 0; i < 3; i++ {
		if err := h.store.ApplyDecision(ctx, created.Request.ID, RequestApproved, slotStart, slotEnd, h.clock.Now()); err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}
	// A later contradicting decision must not overwrite a settled one.
	if err := h.store.ApplyDecision(ctx, created.Request.ID, RequestDeclined, time.Time{}, time.Time{}, h.clock.Now()); err != nil {
		t.Fatalf("second decision: %v", err)
	}
	stored, err := h.store.ReadRequest(ctx, profile.ID, created.Request.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if stored.Status != RequestApproved {
		t.Errorf("status = %q, want the first decision to stand", stored.Status)
	}
	if !stored.DecidedStart.Equal(slotStart.UTC()) {
		t.Errorf("decided start = %v, want %v", stored.DecidedStart, slotStart.UTC())
	}
}

func TestApprovalRequiresAChosenBlock(t *testing.T) {
	h := newHarness(t)
	err := h.store.ApplyDecision(context.Background(), "any", RequestApproved, time.Time{}, time.Time{}, h.clock.Now())
	if err == nil {
		t.Fatal("an approval without a chosen block was accepted")
	}
}
