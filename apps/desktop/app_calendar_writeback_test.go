package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	calendarcore "non24.app/core/calendar"
	storage "non24.app/core/storage/sqlite"
)

// fakeCalDAV is a calendar collection as a CalDAV server keeps one: it honours
// If-None-Match and If-Match, gives each stored revision its own ETag, and
// answers the PROPFIND and calendar-query ZeitBoard sends.
type fakeCalDAV struct {
	mu         sync.Mutex
	objects    map[string]fakeCalendarObject // by escaped path
	revision   int
	privileges []string
	failOnce   map[string]bool // paths whose next PUT fails
	redirect   bool
}

type fakeCalendarObject struct{ body, etag string }

const (
	fakeCollectionPath = "/dav/owner/zeitboard/"
	fakeUsername       = "owner"
	fakePassword       = "synthetic-app-password"
)

func newFakeCalDAV(t *testing.T) (*fakeCalDAV, *httptest.Server) {
	t.Helper()
	fake := &fakeCalDAV{objects: map[string]fakeCalendarObject{}, failOnce: map[string]bool{}, privileges: []string{"read", "write"}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return fake, server
}

func (f *fakeCalDAV) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if username, password, ok := request.BasicAuth(); !ok || username != fakeUsername || password != fakePassword {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}
	path := request.URL.EscapedPath()
	switch request.Method {
	case "PROPFIND":
		privileges := ""
		for _, privilege := range f.privileges {
			privileges += "<D:privilege><D:" + privilege + "/></D:privilege>"
		}
		response.WriteHeader(http.StatusMultiStatus)
		_, _ = fmt.Fprintf(response, `<?xml version="1.0" encoding="utf-8"?>
<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:response><D:href>%s</D:href>
<D:propstat><D:prop><D:resourcetype><D:collection/><C:calendar/></D:resourcetype>
<D:current-user-privilege-set>%s</D:current-user-privilege-set>
<C:supported-calendar-component-set><C:comp name="VEVENT"/></C:supported-calendar-component-set></D:prop>
<D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response></D:multistatus>`, fakeCollectionPath, privileges)
	case "REPORT":
		response.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(response, `<?xml version="1.0" encoding="utf-8"?><D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">`)
		for objectPath, object := range f.objects {
			_, _ = fmt.Fprintf(response, `<D:response><D:href>%s</D:href><D:propstat><D:prop><D:getetag>%s</D:getetag><C:calendar-data><![CDATA[%s]]></C:calendar-data></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`,
				objectPath, object.etag, object.body)
		}
		_, _ = io.WriteString(response, `</D:multistatus>`)
	case http.MethodPut:
		switch {
		case f.redirect:
			http.Redirect(response, request, fakeCollectionPath, http.StatusMovedPermanently)
		case f.failOnce[path]:
			delete(f.failOnce, path)
			response.WriteHeader(http.StatusServiceUnavailable)
		case !strings.HasPrefix(request.Header.Get("Content-Type"), "text/calendar"):
			response.WriteHeader(http.StatusUnsupportedMediaType)
		case f.objects[path].etag != "" && request.Header.Get("If-None-Match") == "*":
			response.WriteHeader(http.StatusPreconditionFailed)
		default:
			body, _ := io.ReadAll(request.Body)
			response.Header().Set("ETag", f.store(path, string(body)))
			response.WriteHeader(http.StatusCreated)
		}
	case http.MethodGet:
		object, exists := f.objects[path]
		if !exists {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		response.Header().Set("ETag", object.etag)
		_, _ = io.WriteString(response, object.body)
	case http.MethodDelete:
		object, exists := f.objects[path]
		switch {
		case !exists:
			response.WriteHeader(http.StatusNotFound)
		case request.Header.Get("If-Match") != "" && request.Header.Get("If-Match") != object.etag:
			response.WriteHeader(http.StatusPreconditionFailed)
		default:
			delete(f.objects, path)
			response.WriteHeader(http.StatusNoContent)
		}
	default:
		response.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// store keeps a new revision of an object, as the server does on a write.
func (f *fakeCalDAV) store(path, body string) string {
	f.revision++
	etag := fmt.Sprintf(`"rev-%d"`, f.revision)
	f.objects[path] = fakeCalendarObject{body: body, etag: etag}
	return etag
}

// edit changes an event the way the owner's own calendar app would.
func (f *fakeCalDAV) edit(t *testing.T, uid string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for path, object := range f.objects {
		if found, _ := calendarcore.CalendarObjectUID([]byte(object.body)); found == uid {
			f.store(path, strings.Replace(object.body, "SUMMARY:", "SUMMARY:Moved by the owner: ", 1))
			return
		}
	}
	t.Fatalf("no event %s to edit", uid)
}

func (f *fakeCalDAV) uids(t *testing.T) []string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	uids := make([]string, 0, len(f.objects))
	for _, object := range f.objects {
		uid, err := calendarcore.CalendarObjectUID([]byte(object.body))
		if err != nil {
			t.Fatal(err)
		}
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	return uids
}

// writeBackApp is an app with three pending suggestions and the fake calendar
// imported, ready for writing to be turned on.
func writeBackApp(t *testing.T) (*App, *fakeCalDAV, CalDAVInput, string, []string) {
	t.Helper()
	app, _, ids := threeSuggestions(t)
	fake, server := newFakeCalDAV(t)
	app.calendarHTTPClient = server.Client()
	calendar := CalDAVInput{
		Endpoint: server.URL + fakeCollectionPath, Label: "Owner calendar",
		Username: fakeUsername, Password: fakePassword, ZoneID: defaultZoneID,
	}
	imported, err := app.ImportCalDAVCalendar(calendar)
	if err != nil {
		t.Fatal(err)
	}
	if imported.EventCount != 0 {
		t.Fatalf("an empty calendar imported as %+v", imported)
	}
	return app, fake, calendar, imported.SourceID, ids
}

func turnOnWriting(t *testing.T, app *App, sourceID string) {
	t.Helper()
	if _, err := app.EnableCalendarWriteBack(CalendarWriteBackInput{SourceID: sourceID, Username: fakeUsername, Password: fakePassword}); err != nil {
		t.Fatal(err)
	}
}

// ownEvents are the accepted times, by suggestion.
func ownEvents(t *testing.T, app *App) map[string]calendarcore.Event {
	t.Helper()
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.OwnedCalendarEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byProposal := map[string]calendarcore.Event{}
	for _, event := range events {
		byProposal[event.ProposalID] = event
	}
	return byProposal
}

// ownUIDs are the UIDs the accepted times are written under, by suggestion.
func ownUIDs(t *testing.T, app *App) map[string]string {
	t.Helper()
	uids := map[string]string{}
	for proposalID, event := range ownEvents(t, app) {
		uids[proposalID] = calendarcore.OwnUID(event.EventID)
	}
	return uids
}

func writeStatus(t *testing.T, app *App) CalendarWriteBackDTO {
	t.Helper()
	status, err := app.GetCalendarWriteBack()
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func TestAcceptedTimesAreWrittenToTheCalendarAndUndoingRemovesThem(t *testing.T) {
	app, fake, calendar, sourceID, ids := writeBackApp(t)
	ctx := context.Background()
	if status := writeStatus(t, app); status.On || status.Summary != "Accepted times stay in ZeitBoard." {
		t.Fatalf("before turning on: %+v", status)
	}

	if _, err := app.EnableCalendarWriteBack(CalendarWriteBackInput{SourceID: sourceID, Username: fakeUsername, Password: "not-the-password"}); err == nil ||
		!strings.Contains(err.Error(), "did not accept this sign-in") {
		t.Fatalf("a wrong sign-in: %v", err)
	}
	status, err := app.EnableCalendarWriteBack(CalendarWriteBackInput{SourceID: sourceID, Username: fakeUsername, Password: fakePassword})
	if err != nil {
		t.Fatal(err)
	}
	if !status.On || status.Label != "Owner calendar" || status.Username != fakeUsername ||
		status.Summary != "The next time you accept is added to Owner calendar." {
		t.Fatalf("turned on: %+v", status)
	}
	if encoded, _ := json.Marshal(status); strings.Contains(string(encoded), fakePassword) {
		t.Fatalf("the status carries the password: %s", encoded)
	}

	if _, err := app.DecideLocalProposals(LocalProposalsDecisionInput{ProposalIDs: ids, Decision: storage.ProposalApproved}); err != nil {
		t.Fatal(err)
	}
	if status := writeStatus(t, app); status.Summary != "3 accepted times are being written to Owner calendar." {
		t.Fatalf("before the writer ran: %q", status.Summary)
	}
	app.writeCalendarPass(ctx)
	uids := ownUIDs(t, app)
	want := []string{uids[ids[0]], uids[ids[1]], uids[ids[2]]}
	sort.Strings(want)
	if got := fake.uids(t); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("the calendar holds %v, want %v", got, want)
	}
	if status := writeStatus(t, app); status.Summary != "3 accepted times are in Owner calendar." || len(status.Problems) != 0 {
		t.Fatalf("after writing: %+v", status)
	}

	// Refreshing the calendar brings ZeitBoard's own events back. They are
	// recognised, not imported as busy time clashing with themselves.
	refreshed, err := app.ImportCalDAVCalendar(calendar)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.EventCount != 0 || !strings.HasSuffix(refreshed.Message, " 3 were ZeitBoard's own accepted times, already on the board.") {
		t.Fatalf("refreshed = %+v", refreshed)
	}

	// Undone: removed from the calendar.
	if _, err := app.UndoLocalProposalDecision(LocalProposalUndoInput{ProposalID: ids[0]}); err != nil {
		t.Fatal(err)
	}
	app.writeCalendarPass(ctx)
	if got := fake.uids(t); len(got) != 2 || strings.Contains(strings.Join(got, " "), uids[ids[0]]) {
		t.Fatalf("after undoing one, the calendar holds %v", got)
	}

	// Changed by the owner, then undone: left for the owner.
	fake.edit(t, uids[ids[1]])
	if _, err := app.UndoLocalProposalDecision(LocalProposalUndoInput{ProposalID: ids[1]}); err != nil {
		t.Fatal(err)
	}
	app.writeCalendarPass(ctx)
	status = writeStatus(t, app)
	if len(fake.uids(t)) != 2 || len(status.Problems) != 1 || !status.Problems[0].Conflict || !status.Problems[0].Removing ||
		status.Problems[0].Detail != "It was changed in your calendar after ZeitBoard wrote it." || status.Problems[0].Title == "" {
		t.Fatalf("a changed event was not left for the owner: %+v, %v", status, fake.uids(t))
	}
	app.writeCalendarPass(ctx)
	if len(fake.uids(t)) != 2 {
		t.Fatal("a conflict was retried on its own")
	}
	if _, err := app.ResolveCalendarWriteConflict(CalendarWriteConflictInput{EventID: status.Problems[0].EventID, Decision: "remove"}); err != nil {
		t.Fatal(err)
	}
	app.writeCalendarPass(ctx)
	if got := fake.uids(t); len(got) != 1 || got[0] != uids[ids[2]] {
		t.Fatalf("removing it anyway left %v", got)
	}

	// Stopping forgets the sign-in; what was written stays in the calendar.
	if status, err := app.DisableCalendarWriteBack(); err != nil || status.On {
		t.Fatalf("stopped: %+v, %v", status, err)
	}
	store, _ := app.requireStore()
	if _, on, _ := store.CalendarWriteTarget(ctx); on {
		t.Fatal("the sign-in outlived writing")
	}
	if got := fake.uids(t); len(got) != 1 {
		t.Fatalf("stopping changed the calendar: %v", got)
	}
}

func TestFailedWritesAreRetriedAndAnEarlierWriteIsKept(t *testing.T) {
	app, fake, _, sourceID, ids := writeBackApp(t)
	ctx := context.Background()
	turnOnWriting(t, app, sourceID)
	if _, err := app.DecideLocalProposals(LocalProposalsDecisionInput{ProposalIDs: ids, Decision: storage.ProposalApproved}); err != nil {
		t.Fatal(err)
	}
	events := ownEvents(t, app)
	placeOf := func(id string) string { return fakeCollectionPath + calendarcore.OwnUID(events[id].EventID) + ".ics" }

	// The first is in the calendar already, written by an attempt whose answer
	// was lost. Someone else's event sits where the second goes. The third's
	// first attempt fails.
	earlier, err := calendarcore.CalendarObject(events[ids[0]], time.Now())
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	earlierETag := fake.store(placeOf(ids[0]), string(earlier))
	fake.store(placeOf(ids[1]), "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:someone-else@example.test\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	fake.failOnce[placeOf(ids[2])] = true
	fake.mu.Unlock()

	app.writeCalendarPass(ctx)
	status := writeStatus(t, app)
	problems := map[string]CalendarWriteProblemDTO{}
	for _, problem := range status.Problems {
		problems[problem.EventID] = problem
	}
	taken, failed := problems[events[ids[1]].EventID], problems[events[ids[2]].EventID]
	if status.Summary != "1 accepted time is in Owner calendar." || len(status.Problems) != 2 ||
		!taken.Conflict || taken.Removing || taken.Detail != "Your calendar already has a different event where ZeitBoard writes this time." ||
		failed.Conflict || failed.Detail != "Your calendar's server had a problem (HTTP 503)." || failed.StartAt != events[ids[2]].StartAt.UTC().Format(time.RFC3339) ||
		failed.RetryAt != app.currentTime().Add(time.Minute).UTC().Format(time.RFC3339) {
		t.Fatalf("after the first pass: %+v", status)
	}

	// Not before its time, unless the owner asks.
	app.writeCalendarPass(ctx)
	if len(fake.uids(t)) != 2 {
		t.Fatal("a failed write was retried early")
	}
	if _, err := app.RetryCalendarWrites(); err != nil {
		t.Fatal(err)
	}
	app.writeCalendarPass(ctx)
	if status := writeStatus(t, app); status.Summary != "2 accepted times are in Owner calendar." || len(status.Problems) != 1 {
		t.Fatalf("after trying again: %+v", status)
	}

	// Someone else's event cannot be removed, only left alone.
	if _, err := app.ResolveCalendarWriteConflict(CalendarWriteConflictInput{EventID: taken.EventID, Decision: "remove"}); err == nil {
		t.Fatal("ZeitBoard offered to remove an event it did not write")
	}
	if status, err := app.ResolveCalendarWriteConflict(CalendarWriteConflictInput{EventID: taken.EventID, Decision: "keep"}); err != nil || len(status.Problems) != 0 {
		t.Fatalf("leaving it alone: %+v, %v", status, err)
	}

	// The earlier write was kept as it was, so undoing removes that revision.
	fake.mu.Lock()
	kept := fake.objects[placeOf(ids[0])].etag
	fake.mu.Unlock()
	if kept != earlierETag {
		t.Fatal("the earlier write was overwritten")
	}
	if _, err := app.UndoLocalProposalDecision(LocalProposalUndoInput{ProposalID: ids[0]}); err != nil {
		t.Fatal(err)
	}
	app.writeCalendarPass(ctx)
	if got := fake.uids(t); len(got) != 2 || strings.Contains(strings.Join(got, " "), calendarcore.OwnUID(events[ids[0]].EventID)) {
		t.Fatalf("after undoing the adopted write: %v", got)
	}
}

func TestTurningOnNeedsASignInThatMayAddEvents(t *testing.T) {
	app, fake, _, sourceID, _ := writeBackApp(t)
	fake.mu.Lock()
	fake.privileges = []string{"read"}
	fake.mu.Unlock()
	if _, err := app.EnableCalendarWriteBack(CalendarWriteBackInput{SourceID: sourceID, Username: fakeUsername, Password: fakePassword}); err == nil ||
		!strings.Contains(err.Error(), "cannot add events") {
		t.Fatalf("a read-only sign-in: %v", err)
	}
	if _, err := app.EnableCalendarWriteBack(CalendarWriteBackInput{SourceID: "calendar_source_missing", Username: fakeUsername, Password: fakePassword}); err == nil {
		t.Fatal("writing was turned on for a calendar that is not there")
	}
	if status := writeStatus(t, app); status.On {
		t.Fatalf("writing is on: %+v", status)
	}
}

func TestAWriteTurnedIntoAReadIsNotTakenAsDone(t *testing.T) {
	app, fake, _, sourceID, ids := writeBackApp(t)
	turnOnWriting(t, app, sourceID)
	if _, err := app.DecideLocalProposals(LocalProposalsDecisionInput{ProposalIDs: ids[:1], Decision: storage.ProposalApproved}); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.redirect = true
	fake.mu.Unlock()
	app.writeCalendarPass(context.Background())
	status := writeStatus(t, app)
	if len(fake.uids(t)) != 0 || len(status.Problems) != 1 || status.Problems[0].Conflict ||
		status.Problems[0].Detail != errCalendarMoved.Error() || status.Problems[0].RetryAt == "" {
		t.Fatalf("a redirected write: %+v", status)
	}
}

func TestOnlyZeitBoardsOwnEventsAreRemoved(t *testing.T) {
	writer, err := newCalDAVWriter(nil, "https://calendar.example/dav/owner/zeitboard", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := writer.objectURL("calendar_event_1"); got != "https://calendar.example/dav/owner/zeitboard/calendar_event_1@zeitboard.local.ics" {
		t.Fatalf("object URL = %q", got)
	}
	for href, want := range map[string]bool{
		"https://calendar.example/dav/owner/zeitboard/calendar_event_1@zeitboard.local.ics": true,
		"https://calendar.example/dav/owner/zeitboard/":                                     false,
		"https://calendar.example/dav/owner/other/event.ics":                                false,
		"https://calendar.example/dav/owner/zeitboard/nested/event.ics":                     false,
		"https://elsewhere.example/dav/owner/zeitboard/event.ics":                           false,
		"https://calendar.example/dav/owner/zeitboard/event.ics?x=1":                        false,
	} {
		if writer.owns(href) != want {
			t.Errorf("owns(%q) = %v", href, !want)
		}
	}
	if strongETag(`W/"weak"`) != "" || strongETag(` "strong" `) != `"strong"` {
		t.Fatal("weak ETags must not be kept")
	}
	if _, err := newCalDAVWriter(nil, "http://calendar.example/dav/", "", ""); err == nil {
		t.Fatal("a plain-HTTP calendar away from this computer was accepted")
	}
}

func TestTheSummarySaysWhereAcceptedTimesAre(t *testing.T) {
	for _, test := range []struct {
		written, waiting int
		want             string
	}{
		{0, 0, "The next time you accept is added to Work."},
		{0, 1, "1 accepted time is being written to Work."},
		{1, 0, "1 accepted time is in Work."},
		{3, 2, "3 accepted times are in Work. 2 more are being written."},
		{2, 1, "2 accepted times are in Work. 1 more is being written."},
	} {
		if got := calendarWriteSummary("Work", test.written, test.waiting); got != test.want {
			t.Errorf("%d written, %d waiting: %q, want %q", test.written, test.waiting, got, test.want)
		}
	}
}
