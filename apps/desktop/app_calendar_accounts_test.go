package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"non24.app/desktop/internal/calendaraccounts"
)

const (
	syntheticAccountClient = "synthetic-client.apps.example.test"
	syntheticAccountSecret = "synthetic-desktop-secret-canary"
	syntheticAccountOwner  = "owner@example.test"
)

// syntheticAccount is a provider holding one synthetic account: its token,
// revoke and calendar endpoints, and a browser that signs in to it. Every
// credential it issues says "canary", which must never reach the window.
type syntheticAccount struct {
	t        *testing.T
	provider calendaraccounts.Provider
	server   *httptest.Server

	mu        sync.Mutex
	challenge string
	refresh   string
	issued    int
	refreshes int
	ended     bool
	gained    bool
	revoked   []string

	// owner is whose account the browser signs in to. requested is the
	// scope the sign-in page asked for, and grant, when set, what the owner
	// allowed instead. scopes is what each access token was issued for.
	owner     string
	requested string
	grant     string
	scopes    map[string]string

	// empty leaves out the owner's own events, as in a calendar kept for
	// ZeitBoard, and gone names a calendar deleted at the provider. stored
	// holds the events ZeitBoard wrote, by ID, versioned.
	empty   bool
	gone    string
	stored  map[string]map[string]any
	version int
	posts   int

	// quitting, when set, is called as a refresh comes in, which then waits
	// for the app to give up on it, as when the app quits mid-refresh.
	// checking is the same for the check that a calendar takes events.
	// losing stores a Google create and answers as if it failed, as an
	// answer lost on the way back would.
	quitting func()
	checking func()
	losing   bool
}

func accountApp(t *testing.T, provider calendaraccounts.Provider) (*App, *syntheticAccount) {
	t.Helper()
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) }
	return app, serveAccount(t, app, provider)
}

// serveAccount points an app at a synthetic provider, with a browser that
// signs in to it.
func serveAccount(t *testing.T, app *App, provider calendaraccounts.Provider) *syntheticAccount {
	t.Helper()
	fake := &syntheticAccount{
		t: t, provider: provider, owner: syntheticAccountOwner,
		scopes: map[string]string{}, stored: map[string]map[string]any{},
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)
	app.calendarAccountHTTP = fake.server.Client()
	app.calendarAccountEndpoints = func(provider calendaraccounts.Provider) calendaraccounts.Endpoints {
		endpoints := calendaraccounts.Endpoints{
			Authorize: fake.server.URL + "/authorize", Token: fake.server.URL + "/token", API: fake.server.URL + "/api",
		}
		if provider == calendaraccounts.Google {
			endpoints.Revoke = fake.server.URL + "/revoke"
		}
		return endpoints
	}
	opened := openSignInPage
	openSignInPage = fake.browser("")
	t.Cleanup(func() { openSignInPage = opened })
	return fake
}

// browser signs in as the owner would: it reads the sign-in page's address
// and comes back to ZeitBoard with a code, or with refusal when one is given.
func (s *syntheticAccount) browser(refusal string) func(context.Context, string) {
	return func(_ context.Context, address string) {
		page, err := url.Parse(address)
		if err != nil {
			s.t.Error(err)
			return
		}
		query := page.Query()
		if query.Get("client_id") != syntheticAccountClient || query.Get("code_challenge_method") != "S256" ||
			(s.provider == calendaraccounts.Microsoft) != strings.HasPrefix(query.Get("redirect_uri"), "http://localhost:") {
			s.t.Errorf("sign-in page %s", address)
		}
		s.mu.Lock()
		s.challenge = query.Get("code_challenge")
		s.requested = query.Get("scope")
		s.mu.Unlock()
		back := url.Values{"state": {query.Get("state")}, "code": {"synthetic-code"}}
		if refusal != "" {
			back = url.Values{"state": {query.Get("state")}, "error": {refusal}}
		}
		go func() {
			if response, err := http.Get(query.Get("redirect_uri") + "/?" + back.Encode()); err == nil {
				response.Body.Close()
			}
		}()
	}
}

func (s *syntheticAccount) with(change func(*syntheticAccount)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	change(s)
}

func (s *syntheticAccount) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	write := func(status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	switch {
	case r.URL.Path == "/token" && r.ParseForm() == nil && s.quitting != nil && r.PostForm.Get("grant_type") == "refresh_token":
		s.quitting()
		<-r.Context().Done()
	case r.URL.Path == "/token" && r.ParseForm() == nil:
		s.token(r.PostForm, write)
	case r.URL.Path == "/revoke" && r.ParseForm() == nil:
		s.revoked = append(s.revoked, r.PostForm.Get("token"))
		write(http.StatusOK, map[string]string{})
	case !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer access-canary-"):
		write(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	default:
		s.api(r, write)
	}
}

func (s *syntheticAccount) token(form url.Values, write func(int, any)) {
	secret := ""
	if s.provider == calendaraccounts.Google {
		secret = syntheticAccountSecret
	}
	if form.Get("client_id") != syntheticAccountClient || form.Get("client_secret") != secret {
		write(http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	s.issued++
	access := fmt.Sprintf("access-canary-%d", s.issued)
	switch form.Get("grant_type") {
	case "authorization_code":
		if form.Get("code") != "synthetic-code" || calendaraccounts.Challenge(form.Get("code_verifier")) != s.challenge {
			write(http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
		}
		s.ended, s.refresh = false, "refresh-canary-1"
		granted := s.requested
		if s.grant != "" {
			granted = s.grant
		}
		s.scopes[access] = granted
		// Short-lived, so the next request refreshes.
		write(http.StatusOK, map[string]any{
			"access_token": access, "refresh_token": s.refresh, "expires_in": 30, "scope": granted,
		})
	case "refresh_token":
		if s.ended || form.Get("refresh_token") != s.refresh {
			write(http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
		}
		s.refreshes++
		answer := map[string]any{"access_token": access, "expires_in": 3600}
		// Google's refresh carries everything granted; Microsoft's gives what
		// the refresh asks for.
		s.scopes[access] = s.requested
		if s.grant != "" {
			s.scopes[access] = s.grant
		}
		if s.provider == calendaraccounts.Microsoft {
			s.refresh = fmt.Sprintf("refresh-canary-%d", s.refreshes+1)
			answer["refresh_token"] = s.refresh
			s.scopes[access] = form.Get("scope")
		}
		write(http.StatusOK, answer)
	}
}

func (s *syntheticAccount) api(r *http.Request, write func(int, any)) {
	if s.gone != "" && strings.Contains(r.URL.Path, "/"+s.gone+"/") {
		write(http.StatusNotFound, map[string]string{"error": "notFound"})
		return
	}
	if s.serveWrites(r, write) {
		return
	}
	at := func(value string) map[string]string { return map[string]string{"dateTime": value} }
	switch {
	case r.URL.Path == "/api/users/me/calendarList":
		write(http.StatusOK, map[string]any{"items": []map[string]any{
			{"id": "team@group.example.test", "summary": "Team"},
			{"id": s.owner, "summary": "Owner", "primary": true},
		}})
	case strings.HasPrefix(r.URL.Path, "/api/calendars/"):
		items := []map[string]any{{"id": "g1", "summary": "Dentist",
			"start": at("2026-10-14T15:00:00-04:00"), "end": at("2026-10-14T16:00:00-04:00")}}
		if s.gained {
			items = append(items, map[string]any{"id": "g2", "summary": "Added later",
				"start": at("2026-10-15T12:00:00Z"), "end": at("2026-10-15T13:00:00Z")})
		}
		if s.empty {
			items = nil
		}
		write(http.StatusOK, map[string]any{"timeZone": "America/New_York", "items": append(items, s.storedEvents()...)})
	case r.URL.Path == "/api/me/calendars":
		write(http.StatusOK, map[string]any{"value": []map[string]any{
			{"id": "m-shared", "name": "Shared", "owner": map[string]string{"address": "colleague@example.test"}},
			{"id": "m-default", "name": "Calendar", "isDefaultCalendar": true, "owner": map[string]string{"address": s.owner}},
		}})
	case strings.HasSuffix(r.URL.Path, "/calendarView"):
		items := []map[string]any{{"id": "m1", "subject": "Standup", "showAs": "busy",
			"start": at("2026-10-14T13:00:00.0000000"), "end": at("2026-10-14T13:15:00.0000000")}}
		if s.empty {
			items = nil
		}
		write(http.StatusOK, map[string]any{"value": append(items, s.storedEvents()...)})
	default:
		write(http.StatusNotFound, map[string]string{"error": "not_found"})
	}
}

func connectAccount(t *testing.T, app *App, provider calendaraccounts.Provider) CalendarAccountCalendarsDTO {
	t.Helper()
	// Microsoft's stand-in refuses a secret, as Microsoft does from a public
	// client, so one given for it must be dropped.
	input := CalendarAccountInput{
		Provider: string(provider), ClientID: syntheticAccountClient, ClientSecret: syntheticAccountSecret,
	}
	connected, err := app.ConnectCalendarAccount(input)
	if err != nil {
		t.Fatal(err)
	}
	return connected
}

func assertNoAccountCredential(t *testing.T, values ...any) {
	t.Helper()
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "canary") {
			t.Fatalf("a credential reached the window: %s", encoded)
		}
	}
}

var accountWeek = CalendarQueryInput{StartCivilDate: "2026-10-13", Days: 5, ZoneID: "America/New_York"}

// A Google account signs in through the owner's registration; a calendar
// from it is previewed, brought in, shown with where it comes from, and read
// again every hour. No credential ever comes back to the window.
func TestAGoogleAccountBringsInACalendarAndKeepsItCurrent(t *testing.T) {
	app, fake := accountApp(t, calendaraccounts.Google)
	connected := connectAccount(t, app, calendaraccounts.Google)
	if connected.Account.Label != syntheticAccountOwner || connected.Account.Calendars != 0 || len(connected.Calendars) != 2 ||
		!connected.Calendars[0].Primary || connected.Calendars[1].Name != "Team" || connected.Calendars[0].SourceID != "" {
		t.Fatalf("connected %+v", connected)
	}
	owner := AccountCalendarInput{
		AccountID: connected.Account.AccountID, CalendarID: syntheticAccountOwner, Label: "Owner", ZoneID: "America/New_York",
	}
	preview, err := app.PreviewAccountCalendar(owner)
	if err != nil || preview.Imported || preview.EventCount != 1 {
		t.Fatalf("preview %+v, %v", preview, err)
	}
	if before, err := app.GetCalendar(accountWeek); err != nil || len(before.Sources) != 0 {
		t.Fatalf("a preview kept something: %+v, %v", before.Sources, err)
	}
	added, err := app.AddAccountCalendar(owner)
	if err != nil || !added.Imported || added.SourceID != preview.SourceID {
		t.Fatalf("added %+v, %v", added, err)
	}
	calendar, err := app.GetCalendar(accountWeek)
	if err != nil || len(calendar.Sources) != 1 {
		t.Fatalf("calendar %+v, %v", calendar.Sources, err)
	}
	source := calendar.Sources[0]
	if source.Label != "Owner" || source.Account == nil || source.Account.Provider != "google" ||
		source.Account.AccountLabel != syntheticAccountOwner || source.Account.RefreshedLabel == "" || source.Account.Failure != "" {
		t.Fatalf("source %+v, %+v", source, source.Account)
	}
	listed, err := app.ListAccountCalendars(CalendarAccountRef{AccountID: connected.Account.AccountID})
	if err != nil || listed.Account.Calendars != 1 || listed.Calendars[0].SourceID != source.SourceID || listed.Calendars[1].SourceID != "" {
		t.Fatalf("listed %+v, %v", listed, err)
	}

	// The account gains an event; the hourly pass reads it.
	fake.with(func(s *syntheticAccount) { s.gained = true })
	app.refreshFetchedCalendars(app.applicationContext())
	calendar, _ = app.GetCalendar(accountWeek)
	if events := countCalendarEvents(calendar); events != 2 {
		t.Fatalf("after the hourly pass %d events", events)
	}
	accounts, err := app.GetCalendarAccounts()
	if err != nil || len(accounts) != 1 || accounts[0].Calendars != 1 || accounts[0].Problem != "" ||
		accounts[0].ConnectedLabel != "Signed in Oct 12, 2026" {
		t.Fatalf("accounts %+v, %v", accounts, err)
	}
	assertNoAccountCredential(t, connected, preview, added, calendar, listed, accounts)
}

// Microsoft rotates refresh tokens, and the rotated one is kept. When the
// owner withdraws the grant, the copy stays, the account says its sign-in
// ended, and signing in again with the kept registration restores it.
func TestAnEndedSignInKeepsTheCopyAndSigningInAgainRestoresIt(t *testing.T) {
	app, fake := accountApp(t, calendaraccounts.Microsoft)
	ctx := app.applicationContext()
	connected := connectAccount(t, app, calendaraccounts.Microsoft)
	accountID := connected.Account.AccountID
	if connected.Account.Label != syntheticAccountOwner || connected.Calendars[0].CalendarID != "m-default" {
		t.Fatalf("connected %+v", connected)
	}
	added, err := app.AddAccountCalendar(AccountCalendarInput{AccountID: accountID, CalendarID: "m-default", Label: "Work"})
	if err != nil || added.EventCount != 1 {
		t.Fatalf("added %+v, %v", added, err)
	}
	kept, _ := app.store.CalendarAccounts(ctx)
	fake.with(func(s *syntheticAccount) {
		if s.refreshes != 1 || kept[0].RefreshToken != s.refresh {
			t.Errorf("after %d refreshes the kept token is not the rotated one", s.refreshes)
		}
		s.ended = true
	})

	app.forgetAccessToken(accountID) // the hour passes
	app.refreshFetchedCalendars(ctx)
	calendar, _ := app.GetCalendar(accountWeek)
	if events := countCalendarEvents(calendar); events != 1 || calendar.Sources[0].Account == nil ||
		!strings.Contains(calendar.Sources[0].Account.Failure, "has ended") {
		t.Fatalf("after the sign-in ended: %d events, %+v", events, calendar.Sources[0].Account)
	}
	accounts, _ := app.GetCalendarAccounts()
	if len(accounts) != 1 || !strings.Contains(accounts[0].Problem, "Sign in to it again") {
		t.Fatalf("accounts %+v", accounts)
	}

	again, err := app.ReconnectCalendarAccount(CalendarAccountRef{AccountID: accountID})
	if err != nil || again.Account.AccountID != accountID || again.Account.Problem != "" || again.Calendars[0].SourceID == "" {
		t.Fatalf("signed in again %+v, %v", again, err)
	}
	if _, err := app.RefreshCalendarSource(CalendarRefreshInput{SourceID: added.SourceID}); err != nil {
		t.Fatal(err)
	}
	calendar, _ = app.GetCalendar(accountWeek)
	if calendar.Sources[0].Account.Failure != "" {
		t.Fatalf("the failure outlived signing in again: %+v", calendar.Sources[0].Account)
	}
	assertNoAccountCredential(t, connected, added, calendar, accounts, again)
}

// Removing an account erases it with its calendars, and Google is asked to
// withdraw the grant; Microsoft's is withdrawn in the account's settings.
func TestRemovingAnAccountErasesItAndWithdrawsItsGrant(t *testing.T) {
	for _, provider := range []calendaraccounts.Provider{calendaraccounts.Google, calendaraccounts.Microsoft} {
		t.Run(string(provider), func(t *testing.T) {
			app, fake := accountApp(t, provider)
			connected := connectAccount(t, app, provider)
			accountID := connected.Account.AccountID
			if _, err := app.AddAccountCalendar(AccountCalendarInput{AccountID: accountID, CalendarID: connected.Calendars[0].CalendarID}); err != nil {
				t.Fatal(err)
			}
			if _, err := app.RemoveCalendarAccount(RemoveCalendarAccountInput{AccountID: accountID}); err == nil {
				t.Fatal("removed an account without REMOVE")
			}
			removed, err := app.RemoveCalendarAccount(RemoveCalendarAccountInput{AccountID: accountID, Confirmation: removeConfirm})
			if err != nil {
				t.Fatal(err)
			}
			calendar, _ := app.GetCalendar(accountWeek)
			accounts, _ := app.GetCalendarAccounts()
			if len(calendar.Sources) != 0 || len(accounts) != 0 {
				t.Fatalf("left %+v and %+v", calendar.Sources, accounts)
			}
			fake.with(func(s *syntheticAccount) {
				switch provider {
				case calendaraccounts.Google:
					if len(s.revoked) != 1 || s.revoked[0] != s.refresh || !strings.Contains(removed.Message, "Google has withdrawn") {
						t.Errorf("revoked %v; %s", s.revoked, removed.Message)
					}
				default:
					if len(s.revoked) != 0 || !strings.Contains(removed.Message, "account.live.com") {
						t.Errorf("revoked %v; %s", s.revoked, removed.Message)
					}
				}
			})
			if _, err := app.ListAccountCalendars(CalendarAccountRef{AccountID: accountID}); err == nil {
				t.Fatal("a removed account was read")
			}
		})
	}
}

// One sign-in waits for the browser at a time, and it can be cancelled; a
// refusal in the browser ends it with the provider's reason.
func TestOneSignInWaitsAtATimeAndCanBeCancelled(t *testing.T) {
	app, fake := accountApp(t, calendaraccounts.Google)
	opened := make(chan struct{})
	openSignInPage = func(context.Context, string) { close(opened) } // the browser never comes back
	input := CalendarAccountInput{Provider: "google", ClientID: syntheticAccountClient, ClientSecret: syntheticAccountSecret}
	waiting := make(chan error, 1)
	go func() {
		_, err := app.ConnectCalendarAccount(input)
		waiting <- err
	}()
	<-opened
	if _, err := app.ConnectCalendarAccount(input); err == nil || !strings.Contains(err.Error(), "already signing in") {
		t.Fatalf("a second sign-in read as %v", err)
	}
	app.CancelCalendarSignIn()
	if err := <-waiting; err == nil || err.Error() != "Signing in was cancelled." {
		t.Fatalf("a cancelled sign-in read as %v", err)
	}

	openSignInPage = fake.browser("access_denied")
	if _, err := app.ConnectCalendarAccount(input); err == nil || err.Error() != "Signing in did not finish: access_denied." {
		t.Fatalf("a refusal read as %v", err)
	}
	for name, bad := range map[string]CalendarAccountInput{
		"provider": {Provider: "fastmail", ClientID: syntheticAccountClient},
		"client":   {Provider: "microsoft"},
		"secret":   {Provider: "google", ClientID: syntheticAccountClient},
		"long secret": {
			Provider: "google", ClientID: syntheticAccountClient, ClientSecret: strings.Repeat("s", 513),
		},
	} {
		if _, err := app.ConnectCalendarAccount(bad); err == nil {
			t.Errorf("signed in with a bad %s", name)
		}
	}
	if accounts, err := app.GetCalendarAccounts(); err != nil || len(accounts) != 0 {
		t.Fatalf("accounts %+v, %v", accounts, err)
	}
}
