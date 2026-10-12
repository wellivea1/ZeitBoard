package sqlite

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	calendarcore "non24.app/core/calendar"
)

const syntheticRefreshToken = "synthetic-refresh-canary-4c1e9a"

func testCalendarAccount() CalendarAccount {
	return CalendarAccount{
		AccountID: "calendar_account_01", Provider: "google", Label: "owner@example.test",
		ClientID: "synthetic-client.apps.example.test", ClientSecret: "synthetic-desktop-secret",
		RefreshToken: syntheticRefreshToken, ConnectedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Scopes: "https://www.googleapis.com/auth/calendar.readonly",
	}
}

func accountSource(id, label string) calendarcore.Source {
	source := testCalendarSource(calendarcore.SourceICS)
	source.SourceID, source.Label = id, label
	return source
}

func accountEvent(sourceID, id string, start time.Time) calendarcore.Event {
	event := testImportedEvent(id, id+"/provider", "Standup", start, true)
	event.SourceID = sourceID
	return event
}

// An account keeps the calendars brought in from it; a failed refresh keeps
// the copy, a rotated token replaces the old, and removing the account takes
// its calendars and its token with it, out of the file as well.
func TestACalendarAccountKeepsItsCalendarsUntilItIsRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	account := testCalendarAccount()
	if err := store.SaveCalendarAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 1, 5, 14, 0, 0, 0, time.UTC)
	work, family := accountSource("calendar_source_account_work", "Work"), accountSource("calendar_source_account_family", "Family")
	if err := store.ReplaceAccountCalendar(ctx, account.AccountID, "work@example.test", work,
		[]calendarcore.Event{accountEvent(work.SourceID, "calendar_event_work_01", start)}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceAccountCalendar(ctx, account.AccountID, "family@example.test", family,
		[]calendarcore.Event{accountEvent(family.SourceID, "calendar_event_family_01", start.Add(3*time.Hour))}); err != nil {
		t.Fatal(err)
	}
	calendars, err := store.AccountCalendars(ctx)
	if err != nil || len(calendars) != 2 || calendars[0].Label != "Family" || calendars[1].CalendarID != "work@example.test" {
		t.Fatalf("calendars %+v, %v", calendars, err)
	}

	failedAt := work.LastImportedAt.Add(time.Hour)
	if err := store.RecordAccountCalendarFailure(ctx, work.SourceID, failedAt, "the provider answered HTTP 503"); err != nil {
		t.Fatal(err)
	}
	calendars, _ = store.AccountCalendars(ctx)
	if got := calendars[1]; got.Failure == "" || !got.AttemptedAt.Equal(failedAt) || !got.RefreshedAt.Equal(work.LastImportedAt) {
		t.Fatalf("after a failure %+v", got)
	}
	if events, err := store.CalendarEvents(ctx, start.Add(-time.Hour), start.Add(time.Hour)); err != nil || len(events) != 1 {
		t.Fatalf("a failure lost the copy: %d events, %v", len(events), err)
	}

	ended := "The sign-in to owner@example.test has ended."
	if err := store.RecordCalendarAccountProblem(ctx, account.AccountID, ended); err != nil {
		t.Fatal(err)
	}
	if accounts, _ := store.CalendarAccounts(ctx); accounts[0].Problem != ended {
		t.Fatalf("the problem was not kept: %+v", accounts[0])
	}
	// Signing in again replaces the sign-in and clears its problem.
	if err := store.SaveCalendarAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	if accounts, _ := store.CalendarAccounts(ctx); accounts[0].Problem != "" {
		t.Fatalf("signing in again kept the problem: %+v", accounts[0])
	}

	rotated := syntheticRefreshToken + "-rotated"
	if err := store.RotateCalendarAccountToken(ctx, account.AccountID, rotated); err != nil {
		t.Fatal(err)
	}
	if accounts, err := store.CalendarAccounts(ctx); err != nil || len(accounts) != 1 || accounts[0].RefreshToken != rotated ||
		accounts[0].ClientSecret != account.ClientSecret || !accounts[0].ConnectedAt.Equal(account.ConnectedAt) ||
		accounts[0].Scopes != account.Scopes {
		t.Fatalf("accounts %+v, %v", accounts, err)
	}

	// Removing one calendar leaves the account and its other calendar.
	if err := store.RemoveImportedCalendar(ctx, family.SourceID); err != nil {
		t.Fatal(err)
	}
	if calendars, _ := store.AccountCalendars(ctx); len(calendars) != 1 || calendars[0].SourceID != work.SourceID {
		t.Fatalf("after removing a calendar %+v", calendars)
	}

	if err := store.RemoveCalendarAccount(ctx, account.AccountID); err != nil {
		t.Fatal(err)
	}
	if accounts, _ := store.CalendarAccounts(ctx); len(accounts) != 0 {
		t.Fatalf("the account outlived its removal: %+v", accounts)
	}
	if sources, _ := store.ListCalendarSources(ctx); len(sources) != 0 {
		t.Fatalf("the account's calendars outlived it: %+v", sources)
	}
	for _, file := range []string{path, path + "-wal"} {
		data, err := os.ReadFile(file)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(syntheticRefreshToken)) {
			t.Fatalf("the token remains in %s", filepath.Base(file))
		}
	}
}

func TestAnAccountCalendarNeedsASignedInAccount(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	work := accountSource("calendar_source_account_work", "Work")
	if err := store.ReplaceAccountCalendar(ctx, "calendar_account_gone", "work", work, nil); !errors.Is(err, ErrCalendarAccountNotFound) {
		t.Fatalf("kept a calendar for no account: %v", err)
	}
	if err := store.RotateCalendarAccountToken(ctx, "calendar_account_gone", "token"); !errors.Is(err, ErrCalendarAccountNotFound) {
		t.Fatalf("rotated a token for no account: %v", err)
	}
	if err := store.RecordCalendarAccountProblem(ctx, "calendar_account_gone", "ended"); !errors.Is(err, ErrCalendarAccountNotFound) {
		t.Fatalf("recorded a problem for no account: %v", err)
	}
	if err := store.RemoveCalendarAccount(ctx, "calendar_account_gone"); !errors.Is(err, ErrCalendarAccountNotFound) {
		t.Fatalf("removed no account: %v", err)
	}
	account := testCalendarAccount()
	if err := store.SaveCalendarAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceAccountCalendar(ctx, account.AccountID, "", work, nil); err == nil {
		t.Error("kept a calendar without the provider's id")
	}
	if err := store.ReplaceAccountCalendar(ctx, account.AccountID, "work", testCalendarSource(calendarcore.SourceCalDAV), nil); err == nil {
		t.Error("kept a CalDAV source as an account calendar")
	}
	if err := store.RecordAccountCalendarFailure(ctx, work.SourceID, time.Now(), "failed"); !errors.Is(err, ErrCalendarSourceNotFound) {
		t.Errorf("recorded a failure for a calendar never brought in: %v", err)
	}
	for name, change := range map[string]func(*CalendarAccount){
		"provider": func(a *CalendarAccount) { a.Provider = "fastmail" },
		"id":       func(a *CalendarAccount) { a.AccountID = "Account 1" },
		"label":    func(a *CalendarAccount) { a.Label = " " },
		"client":   func(a *CalendarAccount) { a.ClientID = "" },
		"token":    func(a *CalendarAccount) { a.RefreshToken = "" },
		"time":     func(a *CalendarAccount) { a.ConnectedAt = time.Time{} },
	} {
		bad := testCalendarAccount()
		change(&bad)
		if err := store.SaveCalendarAccount(ctx, bad); err == nil {
			t.Errorf("kept an account with a bad %s", name)
		}
	}
}

// A calendar brought in from an account can take accepted times, written with
// the account's own sign-in: the target keeps no sign-in of its own, names
// the account and calendar, and goes with the account.
func TestAnAccountCalendarTakesAcceptedTimesUntilTheAccountGoes(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 9, 27, 18, 0, 0, 0, time.UTC)
	account := testCalendarAccount()
	if err := store.SaveCalendarAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	work := accountSource("calendar_source_account_work", "Work")
	if err := store.ReplaceAccountCalendar(ctx, account.AccountID, "work@example.test", work, nil); err != nil {
		t.Fatal(err)
	}
	acceptTime(t, store, "calendar_event_ahead", now.Add(24*time.Hour))
	if err := store.SetCalendarWriteTarget(ctx, CalendarWriteTarget{
		SourceID: work.SourceID, Username: "ignored", Password: "ignored-canary",
	}, now); err != nil {
		t.Fatal(err)
	}
	target, on, err := store.CalendarWriteTarget(ctx)
	if err != nil || !on || target.AccountID != account.AccountID || target.CalendarID != "work@example.test" ||
		target.Label != "Work" || target.CollectionURL != "" || target.Username != "" || target.Password != "" {
		t.Fatalf("target %+v, %v, %v", target, on, err)
	}
	if states := writeStates(t, store); states["calendar_event_ahead"] != CalendarWriteCreate {
		t.Fatalf("queued %v", states)
	}

	if err := store.RemoveCalendarAccount(ctx, account.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, on, err := store.CalendarWriteTarget(ctx); err != nil || on {
		t.Fatalf("writing outlived the account: %v, %v", on, err)
	}
	if states := writeStates(t, store); len(states) != 0 {
		t.Fatalf("writes outlived the account: %v", states)
	}
}
