package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"non24.app/desktop/internal/calendaraccounts"
)

// A calendar deleted at the provider fails on its own row. It leaves the
// access token good for the account's other calendars, so the pass refreshes
// once and the account's sign-in carries on.
func TestACalendarDeletedAtTheProviderLeavesTheAccountSignedIn(t *testing.T) {
	app, fake := accountApp(t, calendaraccounts.Microsoft)
	ctx := app.applicationContext()
	connected := connectAccount(t, app, calendaraccounts.Microsoft)
	accountID := connected.Account.AccountID
	for _, calendar := range connected.Calendars {
		if _, err := app.AddAccountCalendar(AccountCalendarInput{AccountID: accountID, CalendarID: calendar.CalendarID, Label: calendar.Name}); err != nil {
			t.Fatal(err)
		}
	}
	refreshes := 0
	fake.with(func(s *syntheticAccount) { s.gone, refreshes = "m-shared", s.refreshes })
	app.forgetAccessToken(accountID) // the hour passes

	app.refreshFetchedCalendars(ctx)
	fake.with(func(s *syntheticAccount) {
		if s.refreshes != refreshes+1 {
			t.Errorf("one pass refreshed %d times", s.refreshes-refreshes)
		}
	})
	accounts, _ := app.GetCalendarAccounts()
	calendars, _ := app.store.AccountCalendars(ctx)
	failures := map[string]string{}
	for _, calendar := range calendars {
		failures[calendar.CalendarID] = calendar.Failure
	}
	if len(accounts) != 1 || accounts[0].Problem != "" || failures["m-default"] != "" ||
		!strings.Contains(failures["m-shared"], "answered HTTP 404") {
		t.Fatalf("account %+v, failures %v", accounts, failures)
	}
}

// A copy of the account read before its token was rotated never sends the
// replaced token: access is refreshed from the sign-in as it is kept now.
func TestAccessIsRefreshedFromTheSignInAsKeptNow(t *testing.T) {
	app, _ := accountApp(t, calendaraccounts.Microsoft)
	ctx := app.applicationContext()
	connectAccount(t, app, calendaraccounts.Microsoft)
	stale, _ := app.store.CalendarAccounts(ctx)
	app.forgetAccessToken(stale[0].AccountID)
	if _, _, err := app.accountAccess(ctx, app.store, stale[0]); err != nil { // rotates the token
		t.Fatal(err)
	}
	app.forgetAccessToken(stale[0].AccountID)
	if _, _, err := app.accountAccess(ctx, app.store, stale[0]); err != nil {
		t.Fatalf("the stale copy's replaced token was sent: %v", err)
	}
	if accounts, _ := app.GetCalendarAccounts(); accounts[0].Problem != "" {
		t.Fatalf("the sign-in was marked ended: %+v", accounts[0])
	}
}

// Google lets the owner untick reading. A sign-in that cannot read keeps
// nothing and says why.
func TestASignInThatCannotReadIsNotKept(t *testing.T) {
	app, fake := accountApp(t, calendaraccounts.Google)
	fake.with(func(s *syntheticAccount) { s.grant = "https://www.googleapis.com/auth/calendar.events.owned" })
	_, err := app.ConnectCalendarAccount(CalendarAccountInput{
		Provider: "google", ClientID: syntheticAccountClient, ClientSecret: syntheticAccountSecret,
	})
	if err == nil || err.Error() != "Google did not allow ZeitBoard to read your calendars, so the account was not added." {
		t.Fatalf("a sign-in without reading read as %v", err)
	}
	if accounts, _ := app.GetCalendarAccounts(); len(accounts) != 0 {
		t.Fatalf("an account that cannot read was kept: %+v", accounts)
	}
}

func TestAnAccountIsNamedWithinWhatItCanKeep(t *testing.T) {
	long := strings.Repeat("é", 200) + "@example.test" // 413 bytes, 213 characters
	calendars := []calendaraccounts.Calendar{{ID: long, Primary: true, Owner: long}}
	if got := accountLabel(calendaraccounts.Google, calendars); got != "Google account" {
		t.Fatalf("a name longer than an account keeps read as %q", got)
	}
	short := []calendaraccounts.Calendar{{ID: "x", Owner: "colleague@example.test"}, {ID: "y", Primary: true, Owner: "owner@example.test"}}
	if got := accountLabel(calendaraccounts.Microsoft, short); got != "owner@example.test" {
		t.Fatalf("the main calendar's owner did not name the account: %q", got)
	}
}

func TestAStoppedSignInSaysSo(t *testing.T) {
	for err, want := range map[error]string{
		context.Canceled:         "Signing in was cancelled.",
		context.DeadlineExceeded: "Signing in waited five minutes for the browser and stopped. Try again when you are ready.",
		errors.New("oauth2.googleapis.com could not be reached"): "oauth2.googleapis.com could not be reached.",
		errors.New("signing in did not finish: access_denied"):   "Signing in did not finish: access_denied.",
	} {
		if got := signInStopped(err).Error(); got != want {
			t.Errorf("%v read as %q", err, got)
		}
	}
}

// A host name keeps its spelling at the start of a sentence; anything else
// starts with a capital, even a first word with a dot in it.
func TestAProviderErrorReadsAsASentence(t *testing.T) {
	for raw, want := range map[string]string{
		"oauth2.googleapis.com could not be reached": "oauth2.googleapis.com could not be reached.",
		"127.0.0.1 answered HTTP 503":                "127.0.0.1 answered HTTP 503.",
		"the provider refused: invalid_client":       "The provider refused: invalid_client.",
		"e.g. this, said first":                      "E.g. this, said first.",
	} {
		if got := asSentence(errors.New(raw)).Error(); got != want {
			t.Errorf("%q read as %q", raw, got)
		}
	}
}
