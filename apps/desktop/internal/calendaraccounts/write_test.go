package calendaraccounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	calendarcore "non24.app/core/calendar"
)

// fakeCalendars is a provider's calendar API holding events in one calendar
// the owner owns ("mine") and one shared with them ("theirs"). It versions
// every stored event and behaves as each provider does: Google keeps the ID
// of an event it removed, marked cancelled, and honours If-Match; Microsoft
// drops a create that repeats a transaction ID, versions events by change
// key, and pages its calendar view.
type fakeCalendars struct {
	t        *testing.T
	provider Provider
	server   *httptest.Server

	mu      sync.Mutex
	events  map[string]map[string]any // by ID
	version int
	posts   int
	views   int  // calendar views read
	paged   bool // the calendar view comes a page per event
	lapsed  bool // Graph no longer drops a create that repeats a transaction ID
}

func newFakeCalendars(t *testing.T, provider Provider) *fakeCalendars {
	t.Helper()
	fake := &fakeCalendars{t: t, provider: provider, events: map[string]map[string]any{}}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeCalendars) client() Client {
	return Client{
		Provider: f.provider, ClientID: "synthetic-client", Write: true,
		Endpoints: Endpoints{API: f.server.URL + "/api"}, HTTP: f.server.Client(),
	}
}

func (f *fakeCalendars) stamp(event map[string]any) {
	f.version++
	event["etag"] = fmt.Sprintf(`"%d"`, f.version)
	event["changeKey"] = fmt.Sprintf("ck-%d", f.version)
}

// change edits an event as the owner's own calendar app would.
func (f *fakeCalendars) change(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events[id]["summary"] = "Moved by the owner"
	f.stamp(f.events[id])
}

// removeByOwner removes a Google event as the owner's own calendar app would:
// Google keeps it, and its ID, as cancelled.
func (f *fakeCalendars) removeByOwner(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events[id]["status"] = "cancelled"
	f.stamp(f.events[id])
}

// shown counts the events the owner sees: Google's cancelled ones are not.
func (f *fakeCalendars) shown() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, event := range f.events {
		if event["status"] != "cancelled" {
			count++
		}
	}
	return count
}

func (f *fakeCalendars) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	write := func(status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Header.Get("Authorization") != "Bearer access" {
		write(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if f.provider == Microsoft {
		f.serveGraph(w, r, write)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api")
	switch {
	case path == "/users/me/calendarList/mine":
		write(http.StatusOK, map[string]string{"accessRole": "owner"})
	case path == "/users/me/calendarList/theirs":
		write(http.StatusOK, map[string]string{"accessRole": "writer"})
	case path == "/calendars/mine/events" && r.Method == http.MethodPost:
		var event map[string]any
		_ = json.NewDecoder(r.Body).Decode(&event)
		id, _ := event["id"].(string)
		if _, taken := f.events[id]; taken {
			write(http.StatusConflict, map[string]string{"error": "duplicate"})
			return
		}
		f.posts++
		event["status"] = "confirmed"
		f.stamp(event)
		f.events[id] = event
		write(http.StatusOK, event)
	case path == "/calendars/mine/events" && r.Method == http.MethodGet:
		items := []map[string]any{}
		for _, event := range f.events {
			if event["status"] != "cancelled" {
				items = append(items, event)
			}
		}
		write(http.StatusOK, map[string]any{"timeZone": "UTC", "items": items})
	case strings.HasPrefix(path, "/calendars/mine/events/"):
		f.serveGoogleEvent(w, r, strings.TrimPrefix(path, "/calendars/mine/events/"), write)
	default:
		write(http.StatusNotFound, map[string]string{"error": "notFound"})
	}
}

func (f *fakeCalendars) serveGoogleEvent(w http.ResponseWriter, r *http.Request, id string, write func(int, any)) {
	event, found := f.events[id]
	switch {
	case !found:
		write(http.StatusNotFound, map[string]string{"error": "notFound"})
	case r.Method == http.MethodGet:
		write(http.StatusOK, event)
	case r.Method == http.MethodPut && r.Header.Get("If-Match") != event["etag"]:
		write(http.StatusPreconditionFailed, map[string]string{"error": "conditionNotMet"})
	case r.Method == http.MethodPut:
		var update map[string]any
		_ = json.NewDecoder(r.Body).Decode(&update)
		f.stamp(update)
		f.events[id] = update
		write(http.StatusOK, update)
	case event["status"] == "cancelled":
		write(http.StatusGone, map[string]string{"error": "deleted"})
	case r.Header.Get("If-Match") != "" && r.Header.Get("If-Match") != event["etag"]:
		write(http.StatusPreconditionFailed, map[string]string{"error": "conditionNotMet"})
	default:
		// Google keeps a removed event, and its ID, as cancelled.
		event["status"] = "cancelled"
		f.stamp(event)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (f *fakeCalendars) serveGraph(w http.ResponseWriter, r *http.Request, write func(int, any)) {
	path := strings.TrimPrefix(r.URL.Path, "/api")
	// An event's ID must survive the owner moving it.
	if (strings.Contains(path, "/events") || strings.HasSuffix(path, "/calendarView")) &&
		r.Header.Get("Prefer") != `IdType="ImmutableId"` {
		write(http.StatusBadRequest, map[string]string{"error": "ids must be immutable"})
		return
	}
	switch {
	case path == "/me/calendars/mine" || path == "/me/calendars/theirs":
		write(http.StatusOK, map[string]bool{"canEdit": path == "/me/calendars/mine"})
	case path == "/me/calendars/mine/events" && r.Method == http.MethodPost:
		var event map[string]any
		_ = json.NewDecoder(r.Body).Decode(&event)
		for _, existing := range f.events {
			if existing["transactionId"] == event["transactionId"] && !f.lapsed {
				// Graph drops a create that repeats a transaction ID.
				write(http.StatusCreated, existing)
				return
			}
		}
		f.posts++
		id := fmt.Sprintf("m-%d", f.posts)
		event["id"] = id
		f.stamp(event)
		f.events[id] = event
		write(http.StatusCreated, event)
	case path == "/me/calendars/mine/calendarView":
		f.views++
		write(http.StatusOK, f.calendarView(r))
	case strings.HasPrefix(path, "/me/events/"):
		id := strings.TrimPrefix(path, "/me/events/")
		event, found := f.events[id]
		switch {
		case !found:
			write(http.StatusNotFound, map[string]string{"error": "ErrorItemNotFound"})
		case r.Method == http.MethodGet:
			write(http.StatusOK, event)
		default:
			delete(f.events, id)
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		write(http.StatusNotFound, map[string]string{"error": "notFound"})
	}
}

// calendarView is one page of the calendar's events: all of them, or one a
// page, the first page empty, when the fake is paged.
func (f *fakeCalendars) calendarView(r *http.Request) map[string]any {
	ids := make([]string, 0, len(f.events))
	for id := range f.events {
		ids = append(ids, id)
	}
	sort.Strings(ids) // a stable order, so pages neither repeat nor skip
	items := []map[string]any{}
	for _, id := range ids {
		items = append(items, f.events[id])
	}
	if !f.paged {
		return map[string]any{"value": items}
	}
	skip := 0
	fmt.Sscan(r.URL.Query().Get("page"), &skip)
	page := map[string]any{"value": []map[string]any{}}
	if skip > 0 && skip <= len(items) {
		page["value"] = items[skip-1 : skip]
	}
	if skip < len(items) {
		page["@odata.nextLink"] = fmt.Sprintf("%s%s?page=%d", f.server.URL, r.URL.Path, skip+1)
	}
	return page
}

func acceptedTime(id string) calendarcore.Event {
	start := time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC)
	return calendarcore.Event{
		EventID: id, SourceID: "calendar_source_zeitboard", Title: "Write the report",
		StartAt: start, EndAt: start.Add(90 * time.Minute), ZoneID: "America/New_York", Busy: true,
		Ownership: calendarcore.OwnershipAppOwned, CreatedAt: start.Add(-time.Hour),
	}
}

// Google: an accepted time is written once under the ID of its acceptance,
// read back as ZeitBoard's, removed as it was left, and left alone once the
// owner changes it. An event ZeitBoard did not make is never touched.
func TestGoogleWritesOnlyItsOwnEventsAndLeavesChangedOnes(t *testing.T) {
	fake := newFakeCalendars(t, Google)
	client, ctx := fake.client(), context.Background()
	if mine, err := client.CanWrite(ctx, "access", "mine"); err != nil || !mine {
		t.Fatalf("an owned calendar: %v, %v", mine, err)
	}
	if theirs, err := client.CanWrite(ctx, "access", "theirs"); err != nil || theirs {
		t.Fatalf("a shared calendar: %v, %v", theirs, err)
	}
	if _, err := client.CreateEvent(ctx, "access", "mine", acceptedTime("calendar_event_owned_01"), "", "writing_now"); err == nil {
		t.Fatal("a time with no approval was written")
	}

	first := acceptedTime("calendar_event_owned_01")
	written, err := client.CreateEvent(ctx, "access", "mine", first, "decision_first", "writing_now")
	if err != nil || written.ID != googleEventID(first.EventID, "decision_first") || written.Version == "" {
		t.Fatalf("written %+v, %v", written, err)
	}
	// The answer was lost, so it is written again: the first is found.
	again, err := client.CreateEvent(ctx, "access", "mine", first, "decision_first", "writing_now")
	if err != nil || again != written || fake.posts != 1 {
		t.Fatalf("a retried create gave %+v, %v after %d posts", again, err, fake.posts)
	}
	stored := fake.events[written.ID]
	private := stored["extendedProperties"].(map[string]any)["private"].(map[string]any)
	if stored["transparency"] != "opaque" || stored["start"].(map[string]any)["timeZone"] != "America/New_York" ||
		stored["reminders"].(map[string]any)["useDefault"] != false || private["zeitboardWriting"] != "writing_now" {
		t.Fatalf("stored %+v", stored)
	}

	// Reading the calendar back names it as ZeitBoard's.
	events, err := client.Events(ctx, "access", "mine", first.StartAt.Add(-time.Hour), first.EndAt.Add(time.Hour))
	if err != nil || len(events) != 1 || events[0].ID != calendarcore.OwnUID(first.EventID) {
		t.Fatalf("read back %+v, %v", events, err)
	}

	// Someone else's event where ZeitBoard writes the second.
	second := acceptedTime("calendar_event_owned_02")
	taken := googleEventID(second.EventID, "decision_second")
	fake.events[taken] = map[string]any{"id": taken, "etag": `"0"`}
	if _, err := client.CreateEvent(ctx, "access", "mine", second, "decision_second", "writing_now"); !errors.Is(err, ErrTaken) {
		t.Fatalf("an event in the way read as %v", err)
	}

	// A removal finds this time's own event, at the version ZeitBoard wrote.
	third, err := client.CreateEvent(ctx, "access", "mine", acceptedTime("calendar_event_owned_03"), "decision_third", "writing_now")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, third); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("removing another time's event at its own version read as %v", err)
	}
	fake.change(written.ID)
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, written); !errors.Is(err, ErrChanged) {
		t.Fatalf("removing a changed event read as %v", err)
	}
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, Written{ID: "not-an-id-zeitboard-makes"}); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("removing an event under another name read as %v", err)
	}
	current := Written{ID: written.ID, Version: fake.events[written.ID]["etag"].(string)}
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, current); err != nil {
		t.Fatal(err)
	}
	if fake.events[written.ID]["status"] != "cancelled" || fake.shown() != 2 {
		t.Fatalf("the event was not removed alone: %d shown", fake.shown())
	}
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, current); err != nil {
		t.Fatalf("an event already gone read as %v", err)
	}
}

// Google keeps the ID of an event it removed. A time accepted again after an
// undo is written under its new approval, as a new event the owner sees.
func TestGoogleWritesATimeAcceptedAgainAfresh(t *testing.T) {
	fake := newFakeCalendars(t, Google)
	client, ctx := fake.client(), context.Background()
	accepted := acceptedTime("calendar_event_owned_01")
	written, err := client.CreateEvent(ctx, "access", "mine", accepted, "decision_first", "writing_now")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteEvent(ctx, "access", "mine", accepted.EventID, written); err != nil {
		t.Fatal(err)
	}
	rewritten, err := client.CreateEvent(ctx, "access", "mine", accepted, "decision_again", "writing_now")
	if err != nil || rewritten.ID == written.ID || fake.posts != 2 || fake.shown() != 1 {
		t.Fatalf("accepted again: %+v, %v; %d posts, %d shown", rewritten, err, fake.posts, fake.shown())
	}

	// Removed anyway, whatever its version, it must still carry this time's mark.
	if err := client.DeleteEvent(ctx, "access", "mine", "calendar_event_other", Written{ID: rewritten.ID}); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("removing another time's event anyway read as %v", err)
	}
	if err := client.DeleteEvent(ctx, "access", "mine", accepted.EventID, Written{ID: rewritten.ID}); err != nil || fake.shown() != 0 {
		t.Fatalf("removing it anyway: %v; %d shown", err, fake.shown())
	}
}

// ZeitBoard's own event removed while writing was off is put back when it is
// turned on again, as turning writing on writes every accepted time. One the
// owner removed after it was written in this time of writing stays removed,
// even when the create that wrote it lost its answer.
func TestGoogleRemovedEventsComeBackOnlyFromAnEarlierWriting(t *testing.T) {
	fake := newFakeCalendars(t, Google)
	client, ctx := fake.client(), context.Background()
	accepted := acceptedTime("calendar_event_owned_01")

	// Written while writing was on before, then removed while it was off.
	before, err := client.CreateEvent(ctx, "access", "mine", accepted, "decision_first", "writing_before")
	if err != nil {
		t.Fatal(err)
	}
	fake.removeByOwner(before.ID)
	back, err := client.CreateEvent(ctx, "access", "mine", accepted, "decision_first", "writing_now")
	if err != nil || back.ID != before.ID || back.Version == before.Version || fake.shown() != 1 {
		t.Fatalf("writing turned on again: %+v, %v; %d shown", back, err, fake.shown())
	}

	// The owner removes it after a create in this time of writing lost its
	// answer: the retry keeps it removed and takes it as written.
	fake.removeByOwner(back.ID)
	retried, err := client.CreateEvent(ctx, "access", "mine", accepted, "decision_first", "writing_now")
	if err != nil || retried.ID != back.ID || fake.shown() != 0 || fake.posts != 1 {
		t.Fatalf("a retry after the owner's removal: %+v, %v; %d shown, %d posts", retried, err, fake.shown(), fake.posts)
	}
	if err := client.DeleteEvent(ctx, "access", "mine", accepted.EventID, retried); err != nil {
		t.Fatalf("undoing a time the owner removed read as %v", err)
	}
}

// Microsoft: a transaction ID makes a retried create the same transaction,
// a change key tells the owner's edit, and a removal touches only an event
// carrying ZeitBoard's UID. A time accepted again is a new transaction.
func TestMicrosoftWritesOnceAndLeavesChangedOnes(t *testing.T) {
	fake := newFakeCalendars(t, Microsoft)
	client, ctx := fake.client(), context.Background()
	if mine, err := client.CanWrite(ctx, "access", "mine"); err != nil || !mine {
		t.Fatalf("an editable calendar: %v, %v", mine, err)
	}
	if theirs, err := client.CanWrite(ctx, "access", "theirs"); err != nil || theirs {
		t.Fatalf("a read-only calendar: %v, %v", theirs, err)
	}

	first := acceptedTime("calendar_event_owned_01")
	written, err := client.CreateEvent(ctx, "access", "mine", first, "decision_first", "writing_now")
	if err != nil || written.ID == "" || written.Version == "" || fake.views != 1 {
		t.Fatalf("written %+v, %v after %d views", written, err, fake.views)
	}
	stored := fake.events[written.ID]
	if stored["transactionId"] != TransactionID(first.EventID, "decision_first") || stored["showAs"] != "busy" ||
		stored["isReminderOn"] != false || stored["start"].(map[string]any)["dateTime"] != "2026-10-14T15:00:00" {
		t.Fatalf("stored %+v", stored)
	}
	// Written again, its answer lost or the app quitting first, where Graph
	// no longer drops the repeat: the first is found before posting.
	fake.lapsed = true
	if again, err := client.CreateEvent(ctx, "access", "mine", first, "decision_first", "writing_now"); err != nil ||
		again != written || fake.posts != 1 || fake.views != 2 {
		t.Fatalf("a retried create gave %+v, %v after %d posts, %d views", again, err, fake.posts, fake.views)
	}

	fake.change(written.ID)
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, written); !errors.Is(err, ErrChanged) {
		t.Fatalf("removing a changed event read as %v", err)
	}
	if err := client.DeleteEvent(ctx, "access", "mine", "calendar_event_other", written); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("removing another time's event read as %v", err)
	}
	current := Written{ID: written.ID, Version: fake.events[written.ID]["changeKey"].(string)}
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, current); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, current); err != nil || len(fake.events) != 0 {
		t.Fatalf("an event already gone read as %v; %d left", err, len(fake.events))
	}

	if _, err := client.CreateEvent(ctx, "access", "mine", first, "decision_again", "writing_now"); err != nil || fake.posts != 2 || len(fake.events) != 1 {
		t.Fatalf("accepted again: %v; %d posts, %d held", err, fake.posts, len(fake.events))
	}
}

// A create finds an earlier attempt on any page of the time's events. Past
// the pages it reads, it goes ahead, and Graph keeps it from writing twice.
func TestAMicrosoftCreateLooksOnEveryPage(t *testing.T) {
	fake := newFakeCalendars(t, Microsoft)
	client, ctx := fake.client(), context.Background()
	accepted := acceptedTime("calendar_event_owned_01")
	written, err := client.CreateEvent(ctx, "access", "mine", accepted, "decision_first", "writing_now")
	if err != nil {
		t.Fatal(err)
	}
	fake.events["other"] = map[string]any{"id": "other", "transactionId": "someone-elses"}
	fake.paged, fake.lapsed, fake.views = true, true, 0
	again, err := client.CreateEvent(ctx, "access", "mine", accepted, "decision_first", "writing_now")
	if err != nil || again != written || fake.posts != 1 || fake.views < 2 {
		t.Fatalf("found %+v, %v after %d posts, %d pages", again, err, fake.posts, fake.views)
	}

	// More events in the time than ZeitBoard reads, all before its own.
	for n := range maxPages {
		fake.events[fmt.Sprintf("a-%03d", n)] = map[string]any{"id": "filler", "transactionId": "someone-elses"}
	}
	fake.lapsed, fake.views = false, 0
	past, err := client.CreateEvent(ctx, "access", "mine", accepted, "decision_first", "writing_now")
	if err != nil || past != written || fake.posts != 1 || fake.views != maxPages {
		t.Fatalf("past the pages read: %+v, %v after %d posts, %d pages", past, err, fake.posts, fake.views)
	}
}

// The create of a time undone before it was confirmed is looked for by the
// name it was written under and removed if it is ZeitBoard's. Nothing there,
// or another's event under that name, leaves the calendar as it is.
func TestAnUnconfirmedCreateIsLookedForAndRemoved(t *testing.T) {
	for _, provider := range []Provider{Google, Microsoft} {
		t.Run(string(provider), func(t *testing.T) {
			fake := newFakeCalendars(t, provider)
			client, ctx := fake.client(), context.Background()
			accepted := acceptedTime("calendar_event_owned_01")
			removeUnconfirmed := func(approval string) error {
				return client.RemoveUnconfirmed(ctx, "access", "mine", accepted.EventID, approval, accepted.StartAt, accepted.EndAt)
			}
			if err := removeUnconfirmed("decision_first"); err != nil || len(fake.events) != 0 {
				t.Fatalf("with nothing written: %v; %d held", err, len(fake.events))
			}
			if _, err := client.CreateEvent(ctx, "access", "mine", accepted, "decision_first", "writing_now"); err != nil {
				t.Fatal(err)
			}
			if err := removeUnconfirmed("decision_first"); err != nil || fake.shown() != 0 {
				t.Fatalf("with its create landed: %v; %d shown", err, fake.shown())
			}
			if provider == Microsoft {
				return
			}
			taken := googleEventID(accepted.EventID, "decision_other")
			fake.events[taken] = map[string]any{"id": taken, "etag": `"0"`}
			if err := removeUnconfirmed("decision_other"); err != nil || fake.shown() != 1 {
				t.Fatalf("with another's event under the name: %v; %d shown", err, fake.shown())
			}
		})
	}
}

func TestWritingIsAskedForAndReadFromWhatWasGranted(t *testing.T) {
	auth, err := Client{Provider: Google, ClientID: "x", Write: true}.Begin("http://127.0.0.1:1")
	if err != nil || !strings.Contains(mustQuery(t, auth.URL).Get("scope"), "calendar.events.owned") {
		t.Fatalf("Google was not asked to let ZeitBoard add events: %v", err)
	}
	for granted, writes := range map[string]bool{
		"https://www.googleapis.com/auth/calendar.readonly https://www.googleapis.com/auth/calendar.events.owned": true,
		"https://www.googleapis.com/auth/calendar.readonly":                                                       false,
	} {
		if Writes(Google, granted) != writes {
			t.Errorf("Google %q writes = %v", granted, !writes)
		}
	}
	for granted, writes := range map[string]bool{
		"https://graph.microsoft.com/Calendars.ReadWrite": true,
		"Calendars.ReadWrite offline_access":              true,
		"https://graph.microsoft.com/Calendars.Read":      false,
	} {
		if Writes(Microsoft, granted) != writes {
			t.Errorf("Microsoft %q writes = %v", granted, !writes)
		}
	}
}
