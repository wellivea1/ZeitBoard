package calendaraccounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// RFC 7636, Appendix B.
func TestTheChallengeIsRFC7636s(t *testing.T) {
	if got := Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("challenge %s", got)
	}
}

// fakeProvider is a provider's token endpoint and calendar API, holding
// synthetic calendars, that checks what it is sent.
type fakeProvider struct {
	t        *testing.T
	provider Provider
	server   *httptest.Server
	verifier string
	secret   string
	foreign  bool
}

func newFakeProvider(t *testing.T, provider Provider) *fakeProvider {
	t.Helper()
	fake := &fakeProvider{t: t, provider: provider}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeProvider) client() Client {
	return Client{
		Provider: f.provider, ClientID: "synthetic-client", ClientSecret: f.secret,
		Endpoints: Endpoints{
			Authorize: f.server.URL + "/authorize", Token: f.server.URL + "/token",
			API: f.server.URL + "/api", Revoke: f.server.URL + "/revoke",
		},
		HTTP: f.server.Client(),
	}
}

func (f *fakeProvider) serve(w http.ResponseWriter, r *http.Request) {
	write := func(value any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.URL.Path == "/revoke" {
		if r.ParseForm() != nil || r.PostForm.Get("token") != "refresh-1" {
			w.WriteHeader(http.StatusBadRequest)
			write(map[string]string{"error": "invalid_token"})
			return
		}
		write(map[string]string{})
		return
	}
	if r.URL.Path == "/token" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		form := r.PostForm
		if form.Get("client_id") != "synthetic-client" || form.Get("client_secret") != f.secret {
			write(map[string]string{"error": "invalid_client"})
			return
		}
		if f.provider == Microsoft && form.Get("scope") != "offline_access Calendars.Read" {
			write(map[string]string{"error": "invalid_scope"})
			return
		}
		switch form.Get("grant_type") {
		case "authorization_code":
			if form.Get("code") != "synthetic-code" || Challenge(form.Get("code_verifier")) != Challenge(f.verifier) {
				w.WriteHeader(http.StatusBadRequest)
				write(map[string]string{"error": "invalid_grant"})
				return
			}
			write(map[string]any{
				"access_token": "access-1", "refresh_token": "refresh-1", "expires_in": 3599, "scope": scope(f.provider),
			})
		case "refresh_token":
			if form.Get("refresh_token") != "refresh-1" {
				w.WriteHeader(http.StatusBadRequest)
				write(map[string]string{"error": "invalid_grant"})
				return
			}
			answer := map[string]any{"access_token": "access-2", "expires_in": 3599}
			if f.provider == Microsoft {
				answer["refresh_token"] = "refresh-2"
			}
			write(answer)
		}
		return
	}
	if r.Header.Get("Authorization") != "Bearer access-1" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/api/users/me/calendarList" && r.URL.Query().Get("pageToken") == "":
		write(map[string]any{"items": []map[string]any{{"id": "owner@example.test", "summary": "Owner", "primary": true}}, "nextPageToken": "p2"})
	case r.URL.Path == "/api/users/me/calendarList":
		write(map[string]any{"items": []map[string]any{{"id": "team@group.example.test", "summary": "Team"}}})
	case strings.HasPrefix(r.URL.Path, "/api/calendars/") && r.URL.Query().Get("singleEvents") == "true" && r.URL.Query().Get("pageToken") == "":
		write(map[string]any{"timeZone": "America/New_York", "items": []map[string]any{
			{"id": "g1", "summary": "Dentist", "start": map[string]string{"dateTime": "2026-10-14T15:00:00-04:00"}, "end": map[string]string{"dateTime": "2026-10-14T16:00:00-04:00"}},
			{"id": "g2", "status": "cancelled", "summary": "Called off"},
		}, "nextPageToken": "e2"})
	case strings.HasPrefix(r.URL.Path, "/api/calendars/"):
		write(map[string]any{"items": []map[string]any{
			{"id": "g3", "summary": "Holiday", "start": map[string]string{"date": "2026-10-16"}, "end": map[string]string{"date": "2026-10-17"}},
			{"id": "g4", "summary": "Lunch", "transparency": "transparent", "start": map[string]string{"dateTime": "2026-10-15T12:00:00Z"}, "end": map[string]string{"dateTime": "2026-10-15T13:00:00Z"}},
		}})
	case r.URL.Path == "/api/me/calendars":
		write(map[string]any{"value": []map[string]any{{"id": "m-default", "name": "Calendar", "isDefaultCalendar": true, "owner": map[string]string{"address": "owner@example.test"}}}})
	case strings.HasSuffix(r.URL.Path, "/calendarView") && r.URL.Query().Get("page") == "":
		if r.Header.Get("Prefer") != `outlook.timezone="UTC"` {
			http.Error(w, "times must come in UTC", http.StatusBadRequest)
			return
		}
		next := f.server.URL + r.URL.Path + "?page=2"
		if f.foreign {
			next = "https://elsewhere.example.test/page2"
		}
		write(map[string]any{"value": []map[string]any{
			{"id": "m1", "subject": "Standup", "start": map[string]string{"dateTime": "2026-10-14T13:00:00.0000000"}, "end": map[string]string{"dateTime": "2026-10-14T13:15:00.0000000"}, "showAs": "busy"},
			{"id": "m2", "subject": "Gone", "isCancelled": true, "start": map[string]string{"dateTime": "2026-10-14T14:00:00.0000000"}, "end": map[string]string{"dateTime": "2026-10-14T15:00:00.0000000"}},
		}, "@odata.nextLink": next})
	case strings.HasSuffix(r.URL.Path, "/calendarView"):
		write(map[string]any{"value": []map[string]any{
			{"id": "m3", "subject": "Focus", "start": map[string]string{"dateTime": "2026-10-15T09:00:00.0000000"}, "end": map[string]string{"dateTime": "2026-10-15T11:00:00.0000000"}, "showAs": "free"},
		}})
	default:
		http.NotFound(w, r)
	}
}

func TestGoogleSignsInAndReadsEveryPage(t *testing.T) {
	fake := newFakeProvider(t, Google)
	fake.secret = "synthetic-desktop-secret"
	client := fake.client()
	auth, err := client.Begin("http://127.0.0.1:5555")
	if err != nil {
		t.Fatal(err)
	}
	fake.verifier = auth.Verifier
	query := mustQuery(t, auth.URL)
	for name, want := range map[string]string{
		"client_id": "synthetic-client", "redirect_uri": "http://127.0.0.1:5555", "response_type": "code",
		"code_challenge_method": "S256", "access_type": "offline",
		"scope": "https://www.googleapis.com/auth/calendar.readonly",
	} {
		if query.Get(name) != want {
			t.Errorf("%s = %q, want %q", name, query.Get(name), want)
		}
	}
	if query.Get("code_challenge") != Challenge(auth.Verifier) || query.Get("state") != auth.State {
		t.Error("the authorization does not carry its challenge and state")
	}

	token, err := client.Exchange(context.Background(), auth, "synthetic-code")
	if err != nil || token.Access != "access-1" || token.Refresh != "refresh-1" || time.Until(token.ExpiresAt) < time.Hour-time.Minute ||
		token.Scope != "https://www.googleapis.com/auth/calendar.readonly" {
		t.Fatalf("token %+v, %v", token, err)
	}
	refreshed, err := client.Refresh(context.Background(), "refresh-1")
	if err != nil || refreshed.Access != "access-2" || refreshed.Refresh != "refresh-1" {
		t.Fatalf("a refresh that rotates nothing keeps the refresh token: %+v, %v", refreshed, err)
	}

	calendars, err := client.Calendars(context.Background(), "access-1")
	if err != nil || len(calendars) != 2 || !calendars[0].Primary || calendars[0].Owner != "owner@example.test" || calendars[1].Name != "Team" {
		t.Fatalf("calendars %+v, %v", calendars, err)
	}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	events, err := client.Events(context.Background(), "access-1", "owner@example.test", from, from.AddDate(0, 1, 0))
	if err != nil || len(events) != 3 {
		t.Fatalf("events %+v, %v", events, err)
	}
	if !events[0].StartAt.Equal(time.Date(2026, 10, 14, 19, 0, 0, 0, time.UTC)) || !events[0].Busy || events[0].ZoneID != "America/New_York" {
		t.Errorf("timed event %+v", events[0])
	}
	if !events[1].AllDay || events[1].StartAt.Format(time.RFC3339) != "2026-10-16T00:00:00-04:00" {
		t.Errorf("all-day event %+v", events[1])
	}
	if events[2].Busy {
		t.Errorf("free time blocks: %+v", events[2])
	}
}

func TestMicrosoftSignsInAndReadsEveryPageOnItsOwnHost(t *testing.T) {
	fake := newFakeProvider(t, Microsoft)
	client := fake.client()
	auth, err := client.Begin("http://127.0.0.1:5555")
	if err != nil {
		t.Fatal(err)
	}
	fake.verifier = auth.Verifier
	if query := mustQuery(t, auth.URL); query.Get("scope") != "offline_access Calendars.Read" || query.Get("access_type") != "" {
		t.Errorf("authorization %v", query)
	}
	if _, err := client.Exchange(context.Background(), auth, "synthetic-code"); err != nil {
		t.Fatal(err)
	}
	if refreshed, err := client.Refresh(context.Background(), "refresh-1"); err != nil || refreshed.Refresh != "refresh-2" {
		t.Fatalf("a rotated refresh token is not kept: %+v, %v", refreshed, err)
	}
	calendars, err := client.Calendars(context.Background(), "access-1")
	if err != nil || len(calendars) != 1 || calendars[0].Owner != "owner@example.test" {
		t.Fatalf("calendars %+v, %v", calendars, err)
	}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	events, err := client.Events(context.Background(), "access-1", "m-default", from, from.AddDate(0, 1, 0))
	if err != nil || len(events) != 2 || events[0].ZoneID != "UTC" || events[1].Busy {
		t.Fatalf("events %+v, %v", events, err)
	}
	if !events[0].StartAt.Equal(time.Date(2026, 10, 14, 13, 0, 0, 0, time.UTC)) {
		t.Errorf("start %s", events[0].StartAt)
	}

	fake.foreign = true
	if _, err := client.Events(context.Background(), "access-1", "m-default", from, from.AddDate(0, 1, 0)); err == nil ||
		!strings.Contains(err.Error(), "next page elsewhere") {
		t.Fatalf("followed a page to another host: %v", err)
	}
}

func TestARefusedSignInSaysWhyWithoutTheBody(t *testing.T) {
	fake := newFakeProvider(t, Google)
	client := fake.client()
	auth, _ := client.Begin("http://127.0.0.1:5555")
	fake.verifier = "a different verifier"
	if _, err := client.Exchange(context.Background(), auth, "synthetic-code"); err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("a wrong verifier read as %v", err)
	}
	if _, err := (Client{Provider: "other", ClientID: "x"}).Begin("http://127.0.0.1:1"); err == nil {
		t.Fatal("began a sign-in with an unknown provider")
	}
}

// A refresh the provider refuses ends the sign-in; revoking withdraws a grant
// where the provider can, and says whether it did.
func TestAnEndedSignInIsToldApartAndAGrantCanBeWithdrawn(t *testing.T) {
	fake := newFakeProvider(t, Google)
	client := fake.client()
	if _, err := client.Refresh(context.Background(), "withdrawn"); !errors.Is(err, ErrSignInEnded) {
		t.Fatalf("a refused refresh read as %v", err)
	}
	if !client.Revoke(context.Background(), "refresh-1") || client.Revoke(context.Background(), "unknown") {
		t.Fatal("revoking did not report what the provider answered")
	}
	client.Endpoints.Revoke = ""
	if client.Revoke(context.Background(), "refresh-1") {
		t.Fatal("revoked without an address to revoke at")
	}
}

// The receiver takes only the answer to its own sign-in.
func TestTheReceiverTakesOnlyItsOwnAnswer(t *testing.T) {
	receiver, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	redirect := receiver.Redirect(Google)
	if !strings.HasPrefix(redirect, "http://127.0.0.1:") {
		t.Fatalf("redirect %s", redirect)
	}
	if port := strings.TrimPrefix(redirect, "http://127.0.0.1:"); receiver.Redirect(Microsoft) != "http://localhost:"+port {
		t.Fatalf("Microsoft's redirect %s", receiver.Redirect(Microsoft))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan string, 1)
	go func() {
		code, err := receiver.Wait(ctx, "expected-state")
		result <- fmt.Sprint(code, err)
	}()
	wrong, err := http.Get(redirect + "/?code=stolen&state=other-state")
	if err == nil {
		wrong.Body.Close()
		if wrong.StatusCode != http.StatusBadRequest {
			t.Errorf("a wrong state answered %d", wrong.StatusCode)
		}
	}
	right, err := http.Get(redirect + "/?code=synthetic-code&state=expected-state")
	if err != nil {
		t.Fatal(err)
	}
	right.Body.Close()
	if got := <-result; got != "synthetic-code<nil>" {
		t.Fatalf("waited for %s", got)
	}
}

func TestARefusalInTheBrowserEndsTheWait(t *testing.T) {
	receiver, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	result := make(chan error, 1)
	go func() {
		_, err := receiver.Wait(context.Background(), "s")
		result <- err
	}()
	response, err := http.Get(receiver.Redirect(Google) + "/?error=access_denied&state=s")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if err := <-result; err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("a refusal read as %v", err)
	}

	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	second, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.Wait(stopped, "s"); !errors.Is(err, context.Canceled) {
		t.Fatalf("a stopped wait read as %v", err)
	}
}

func mustQuery(t *testing.T, address string) url.Values {
	t.Helper()
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Query()
}
