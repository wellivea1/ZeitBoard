package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	calendarcore "non24.app/core/calendar"
	storage "non24.app/core/storage/sqlite"
	"non24.app/desktop/internal/calendaraccounts"
)

// serveWrites answers what writing asks of the synthetic provider, and
// reports whether the request was one. Adding or removing an event needs an
// access token granted for writing, as the providers' own do.
func (s *syntheticAccount) serveWrites(r *http.Request, write func(int, any)) bool {
	access := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	writable := calendaraccounts.Writes(s.provider, s.scopes[access])
	path := strings.TrimPrefix(r.URL.Path, "/api")
	if r.Method != http.MethodGet && !writable {
		write(http.StatusForbidden, map[string]string{"error": "insufficient_scope"})
		return true
	}
	if s.provider == calendaraccounts.Google {
		return s.serveGoogleWrites(r, path, write)
	}
	return s.serveGraphWrites(r, path, write)
}

func (s *syntheticAccount) serveGoogleWrites(r *http.Request, path string, write func(int, any)) bool {
	events := "/calendars/" + s.owner + "/events"
	switch {
	case strings.HasPrefix(path, "/users/me/calendarList/"):
		role := "reader"
		if strings.TrimPrefix(path, "/users/me/calendarList/") == s.owner {
			role = "owner"
		}
		write(http.StatusOK, map[string]string{"accessRole": role})
	case path == events && r.Method == http.MethodPost:
		var event map[string]any
		_ = json.NewDecoder(r.Body).Decode(&event)
		id, _ := event["id"].(string)
		if _, taken := s.stored[id]; taken {
			write(http.StatusConflict, map[string]string{"error": "duplicate"})
			return true
		}
		s.posts++
		s.stamp(event)
		s.stored[id] = event
		write(http.StatusOK, event)
	case strings.HasPrefix(path, events+"/"):
		id := strings.TrimPrefix(path, events+"/")
		event, found := s.stored[id]
		switch {
		case !found:
			write(http.StatusGone, map[string]string{"error": "deleted"})
		case r.Method == http.MethodGet:
			write(http.StatusOK, event)
		case r.Header.Get("If-Match") != "" && r.Header.Get("If-Match") != event["etag"]:
			write(http.StatusPreconditionFailed, map[string]string{"error": "conditionNotMet"})
		default:
			delete(s.stored, id)
			write(http.StatusNoContent, nil)
		}
	default:
		return false
	}
	return true
}

func (s *syntheticAccount) serveGraphWrites(r *http.Request, path string, write func(int, any)) bool {
	switch {
	case path == "/me/calendars/m-default" || path == "/me/calendars/m-shared":
		write(http.StatusOK, map[string]bool{"canEdit": path == "/me/calendars/m-default"})
	case path == "/me/calendars/m-default/events" && r.Method == http.MethodPost:
		var event map[string]any
		_ = json.NewDecoder(r.Body).Decode(&event)
		s.posts++
		id := fmt.Sprintf("graph-%d", s.posts)
		event["id"] = id
		s.stamp(event)
		s.stored[id] = event
		write(http.StatusCreated, event)
	case strings.HasPrefix(path, "/me/events/"):
		id := strings.TrimPrefix(path, "/me/events/")
		event, found := s.stored[id]
		switch {
		case !found:
			write(http.StatusNotFound, map[string]string{"error": "ErrorItemNotFound"})
		case r.Method == http.MethodGet:
			write(http.StatusOK, event)
		default:
			delete(s.stored, id)
			write(http.StatusNoContent, nil)
		}
	default:
		return false
	}
	return true
}

// stamp gives a stored event a new version, as the provider does on a change.
func (s *syntheticAccount) stamp(event map[string]any) {
	s.version++
	event["etag"] = fmt.Sprintf(`"%d"`, s.version)
	event["changeKey"] = fmt.Sprintf("ck-%d", s.version)
}

func (s *syntheticAccount) storedEvents() []map[string]any {
	events := make([]map[string]any, 0, len(s.stored))
	for _, event := range s.stored {
		events = append(events, event)
	}
	return events
}

// edit changes a written event the way the owner's own calendar app would.
func (s *syntheticAccount) edit(t *testing.T, eventID string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range s.stored {
		if ownedBy(event) == eventID {
			event["summary"], event["subject"] = "Moved by the owner", "Moved by the owner"
			s.stamp(event)
			return
		}
	}
	t.Fatalf("no written event for %s", eventID)
}

// written names the accepted times the calendar holds.
func (s *syntheticAccount) written() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := map[string]bool{}
	for _, event := range s.stored {
		held[ownedBy(event)] = true
	}
	return held
}

// ownedBy is the accepted time an event was written for: Google's private
// mark, or Microsoft's transaction ID.
func ownedBy(event map[string]any) string {
	if properties, ok := event["extendedProperties"].(map[string]any); ok {
		if private, ok := properties["private"].(map[string]any); ok {
			id, _ := private["zeitboardEvent"].(string)
			return id
		}
	}
	transaction, _ := event["transactionId"].(string)
	id, _ := calendarcore.OwnEventID(transaction)
	return id
}

// writeApp is three suggestions waiting, with an account's main calendar
// brought in and writing not yet on. The calendar holds nothing of the
// owner's, as one kept for ZeitBoard would, so the suggestions still stand.
func writeApp(t *testing.T, provider calendaraccounts.Provider) (*App, *syntheticAccount, CalendarAccountDTO, string, []string) {
	t.Helper()
	app, _, ids := threeSuggestions(t)
	fake := serveAccount(t, app, provider)
	fake.empty = true
	connected := connectAccount(t, app, provider)
	added, err := app.AddAccountCalendar(AccountCalendarInput{
		AccountID: connected.Account.AccountID, CalendarID: connected.Calendars[0].CalendarID, Label: "Owner",
	})
	if err != nil {
		t.Fatal(err)
	}
	return app, fake, connected.Account, added.SourceID, ids
}

// Turning writing on asks Google to let ZeitBoard add events, once. Accepted
// times are then written to the owner's calendar, read back as ZeitBoard's
// own, removed on undo, and left for the owner once changed there.
func TestAcceptedTimesAreWrittenToAGoogleCalendarOfTheOwners(t *testing.T) {
	app, fake, account, sourceID, ids := writeApp(t, calendaraccounts.Google)
	ctx := context.Background()
	status, err := app.EnableAccountWriteBack(AccountWriteBackInput{SourceID: sourceID})
	if err != nil {
		t.Fatal(err)
	}
	if !status.On || status.Provider != "google" || status.Label != "Owner" || status.Username != "" ||
		status.Summary != "The next time you accept is added to Owner." {
		t.Fatalf("turned on: %+v", status)
	}
	fake.with(func(s *syntheticAccount) {
		if !strings.Contains(s.requested, "calendar.events.owned") || strings.Contains(s.requested, "auth/calendar.events ") {
			t.Errorf("writing asked Google for %q", s.requested)
		}
	})
	if accounts, _ := app.GetCalendarAccounts(); len(accounts) != 1 || !accounts[0].Writing {
		t.Fatalf("the account does not say it is written to: %+v", accounts)
	}

	if _, err := app.DecideLocalProposals(LocalProposalsDecisionInput{ProposalIDs: ids, Decision: storage.ProposalApproved}); err != nil {
		t.Fatal(err)
	}
	app.writeCalendarPass(ctx)
	events := ownEvents(t, app)
	if held := fake.written(); len(held) != 3 || !held[events[ids[0]].EventID] {
		t.Fatalf("the calendar holds %v", held)
	}
	if status := writeStatus(t, app); status.Summary != "3 accepted times are in Owner." || len(status.Problems) != 0 {
		t.Fatalf("after writing: %+v", status)
	}

	// Reading the calendar again recognises ZeitBoard's own events.
	refreshed, err := app.RefreshCalendarSource(CalendarRefreshInput{SourceID: sourceID})
	if err != nil || !strings.HasSuffix(refreshed.Message, " 3 were ZeitBoard's own accepted times, already on the board.") {
		t.Fatalf("refreshed %+v, %v", refreshed, err)
	}

	if _, err := app.UndoLocalProposalDecision(LocalProposalUndoInput{ProposalID: ids[0]}); err != nil {
		t.Fatal(err)
	}
	app.writeCalendarPass(ctx)
	if held := fake.written(); len(held) != 2 || held[events[ids[0]].EventID] {
		t.Fatalf("after undoing one, the calendar holds %v", held)
	}

	fake.edit(t, events[ids[1]].EventID)
	if _, err := app.UndoLocalProposalDecision(LocalProposalUndoInput{ProposalID: ids[1]}); err != nil {
		t.Fatal(err)
	}
	app.writeCalendarPass(ctx)
	status = writeStatus(t, app)
	if len(fake.written()) != 2 || len(status.Problems) != 1 || !status.Problems[0].Conflict ||
		status.Problems[0].Detail != "It was changed in your calendar after ZeitBoard wrote it." {
		t.Fatalf("a changed event was not left for the owner: %+v", status)
	}
	if _, err := app.ResolveCalendarWriteConflict(CalendarWriteConflictInput{EventID: status.Problems[0].EventID, Decision: "remove"}); err != nil {
		t.Fatal(err)
	}
	app.writeCalendarPass(ctx)
	if held := fake.written(); len(held) != 1 || !held[events[ids[2]].EventID] {
		t.Fatalf("removing it anyway left %v", held)
	}

	// Removing the account stops writing; what was written stays.
	if _, err := app.RemoveCalendarAccount(RemoveCalendarAccountInput{AccountID: account.AccountID, Confirmation: removeConfirm}); err != nil {
		t.Fatal(err)
	}
	if status := writeStatus(t, app); status.On || len(fake.written()) != 1 {
		t.Fatalf("after removing the account: %+v, %v", status, fake.written())
	}
	assertNoAccountCredential(t, writeStatus(t, app))
}

// Microsoft: a refresh asks for writing again, an accepted time that landed
// with its answer lost is found rather than written twice, and an ended
// sign-in leaves the writes waiting with the reason.
func TestAMicrosoftCalendarIsWrittenOnceAndWaitsOnAnEndedSignIn(t *testing.T) {
	app, fake, account, sourceID, ids := writeApp(t, calendaraccounts.Microsoft)
	ctx := context.Background()
	if _, err := app.EnableAccountWriteBack(AccountWriteBackInput{SourceID: sourceID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DecideLocalProposals(LocalProposalsDecisionInput{ProposalIDs: ids, Decision: storage.ProposalApproved}); err != nil {
		t.Fatal(err)
	}
	events := ownEvents(t, app)
	first := events[ids[0]]
	fake.with(func(s *syntheticAccount) {
		landed := map[string]any{
			"id": "landed", "subject": first.Title, "transactionId": calendarcore.OwnUID(first.EventID),
			"start": map[string]string{"dateTime": first.StartAt.UTC().Format("2006-01-02T15:04:05"), "timeZone": "UTC"},
			"end":   map[string]string{"dateTime": first.EndAt.UTC().Format("2006-01-02T15:04:05"), "timeZone": "UTC"},
		}
		s.stamp(landed)
		s.stored["landed"] = landed
	})

	app.forgetAccessToken(account.AccountID) // the hour passes: writing must be asked for again
	app.writeCalendarPass(ctx)
	fake.with(func(s *syntheticAccount) {
		if s.posts != 2 || len(s.stored) != 3 {
			t.Errorf("%d created, %d held: the landed one was written again or a write failed", s.posts, len(s.stored))
		}
	})
	if status := writeStatus(t, app); status.Summary != "3 accepted times are in Owner." || status.Provider != "microsoft" {
		t.Fatalf("after writing: %+v", status)
	}

	if _, err := app.UndoLocalProposalDecision(LocalProposalUndoInput{ProposalID: ids[0]}); err != nil {
		t.Fatal(err)
	}
	fake.with(func(s *syntheticAccount) { s.ended = true })
	app.forgetAccessToken(account.AccountID)
	app.writeCalendarPass(ctx)
	status := writeStatus(t, app)
	if len(status.Problems) != 1 || status.Problems[0].Conflict || status.Problems[0].RetryAt == "" ||
		!strings.Contains(status.Problems[0].Detail, "has ended") || !fake.written()[first.EventID] {
		t.Fatalf("an ended sign-in did not leave the removal waiting: %+v", status)
	}
}

// Writing is turned on only with the owner's permission, for the account
// that holds the calendar, and for a calendar of the owner's own.
func TestWritingNeedsPermissionTheSameAccountAndACalendarOfTheOwners(t *testing.T) {
	app, fake, account, sourceID, _ := writeApp(t, calendaraccounts.Google)
	enable := func(sourceID string) error {
		_, err := app.EnableAccountWriteBack(AccountWriteBackInput{SourceID: sourceID})
		return err
	}

	// The owner leaves adding events unticked on Google's consent page.
	fake.with(func(s *syntheticAccount) { s.grant = "https://www.googleapis.com/auth/calendar.readonly" })
	if err := enable(sourceID); err == nil || err.Error() != "Google did not allow ZeitBoard to add events, so writing was not turned on." {
		t.Fatalf("a refused permission read as %v", err)
	}
	// The browser signs in to someone else.
	fake.with(func(s *syntheticAccount) { s.grant, s.owner = "", "someone-else@example.test" })
	if err := enable(sourceID); err == nil ||
		err.Error() != "The browser signed in to someone-else@example.test. Sign in to owner@example.test instead." {
		t.Fatalf("another account read as %v", err)
	}
	if accounts, _ := app.GetCalendarAccounts(); len(accounts) != 1 || accounts[0].AccountID != account.AccountID {
		t.Fatalf("the other account was kept: %+v", accounts)
	}
	// A calendar shared with the owner takes no events from ZeitBoard.
	fake.with(func(s *syntheticAccount) { s.owner = syntheticAccountOwner })
	team, err := app.AddAccountCalendar(AccountCalendarInput{AccountID: account.AccountID, CalendarID: "team@group.example.test", Label: "Team"})
	if err != nil {
		t.Fatal(err)
	}
	if err := enable(team.SourceID); err == nil || err.Error() != "ZeitBoard can add events only to a calendar of your own, and Team is not." {
		t.Fatalf("a shared calendar read as %v", err)
	}
	if status := writeStatus(t, app); status.On {
		t.Fatalf("writing was turned on: %+v", status)
	}
	if err := enable("calendar_source_missing"); err == nil {
		t.Fatal("writing was turned on for no calendar")
	}
}
