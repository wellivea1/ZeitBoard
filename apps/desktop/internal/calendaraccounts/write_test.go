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
		id := strings.TrimPrefix(path, "/calendars/mine/events/")
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
	default:
		write(http.StatusNotFound, map[string]string{"error": "notFound"})
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
			if existing["transactionId"] == event["transactionId"] {
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
		ids := make([]string, 0, len(f.events))
		for id := range f.events {
			ids = append(ids, id)
		}
		sort.Strings(ids) // a stable order, so pages neither repeat nor skip
		items := []map[string]any{}
		for _, id := range ids {
			items = append(items, f.events[id])
		}
		page := map[string]any{"value": items}
		if f.paged {
			// One event a page, the first page empty.
			skip := 0
			fmt.Sscan(r.URL.Query().Get("page"), &skip)
			page["value"] = []map[string]any{}
			if skip > 0 && skip <= len(items) {
				page["value"] = items[skip-1 : skip]
			}
			if skip < len(items) {
				page["@odata.nextLink"] = fmt.Sprintf("%s%s?page=%d", f.server.URL, r.URL.Path, skip+1)
			}
		}
		write(http.StatusOK, page)
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

	first := acceptedTime("calendar_event_owned_01")
	written, err := client.CreateEvent(ctx, "access", "mine", first, false)
	if err != nil || written.ID != googleEventID(first) || written.Version == "" {
		t.Fatalf("written %+v, %v", written, err)
	}
	// The answer was lost, so it is written again: the first is found.
	again, err := client.CreateEvent(ctx, "access", "mine", first, true)
	if err != nil || again != written || fake.posts != 1 {
		t.Fatalf("a retried create gave %+v, %v after %d posts", again, err, fake.posts)
	}
	stored := fake.events[written.ID]
	if stored["transparency"] != "opaque" || stored["start"].(map[string]any)["timeZone"] != "America/New_York" ||
		stored["reminders"].(map[string]any)["useDefault"] != false {
		t.Fatalf("stored %+v", stored)
	}

	// Reading the calendar back names it as ZeitBoard's.
	events, err := client.Events(ctx, "access", "mine", first.StartAt.Add(-time.Hour), first.EndAt.Add(time.Hour))
	if err != nil || len(events) != 1 || events[0].ID != calendarcore.OwnUID(first.EventID) {
		t.Fatalf("read back %+v, %v", events, err)
	}

	// Someone else's event where ZeitBoard writes the second.
	second := acceptedTime("calendar_event_owned_02")
	fake.events[googleEventID(second)] = map[string]any{"id": googleEventID(second), "etag": `"0"`}
	if _, err := client.CreateEvent(ctx, "access", "mine", second, false); !errors.Is(err, ErrTaken) {
		t.Fatalf("an event in the way read as %v", err)
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
	if fake.shown() != 1 {
		t.Fatalf("the event was not removed: %d shown", fake.shown())
	}
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, current); err != nil {
		t.Fatalf("an event already gone read as %v", err)
	}
}

// Google keeps the ID of an event it removed. Accepting the same time again
// after undoing it writes a new event the owner sees, not the removed one.
func TestGoogleWritesATimeAcceptedAgainAfresh(t *testing.T) {
	fake := newFakeCalendars(t, Google)
	client, ctx := fake.client(), context.Background()
	accepted := acceptedTime("calendar_event_owned_01")
	written, err := client.CreateEvent(ctx, "access", "mine", accepted, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteEvent(ctx, "access", "mine", accepted.EventID, written); err != nil {
		t.Fatal(err)
	}

	again := accepted
	again.CreatedAt = accepted.CreatedAt.Add(time.Minute) // accepted again
	rewritten, err := client.CreateEvent(ctx, "access", "mine", again, false)
	if err != nil || rewritten.ID == written.ID || fake.posts != 2 || fake.shown() != 1 {
		t.Fatalf("accepted again: %+v, %v; %d posts, %d shown", rewritten, err, fake.posts, fake.shown())
	}

	// Accepted again within the same second, under the removed event's own
	// ID: the removed event is put back, not taken for written.
	if err := client.DeleteEvent(ctx, "access", "mine", again.EventID, rewritten); err != nil {
		t.Fatal(err)
	}
	restored, err := client.CreateEvent(ctx, "access", "mine", again, false)
	if err != nil || restored.ID != rewritten.ID || restored.Version == rewritten.Version || fake.shown() != 1 {
		t.Fatalf("accepted again at once: %+v, %v; %d shown", restored, err, fake.shown())
	}
	rewritten = restored

	// Removed anyway, whatever its version, it must still carry this time's mark.
	if err := client.DeleteEvent(ctx, "access", "mine", "calendar_event_other", Written{ID: rewritten.ID}); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("removing another time's event anyway read as %v", err)
	}
	if err := client.DeleteEvent(ctx, "access", "mine", again.EventID, Written{ID: rewritten.ID}); err != nil || fake.shown() != 0 {
		t.Fatalf("removing it anyway: %v; %d shown", err, fake.shown())
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
	written, err := client.CreateEvent(ctx, "access", "mine", first, false)
	if err != nil || written.ID == "" || written.Version == "" || fake.views != 0 {
		t.Fatalf("written %+v, %v after %d views", written, err, fake.views)
	}
	stored := fake.events[written.ID]
	if stored["transactionId"] != TransactionID(first) || stored["showAs"] != "busy" || stored["isReminderOn"] != false ||
		stored["start"].(map[string]any)["dateTime"] != "2026-10-14T15:00:00" {
		t.Fatalf("stored %+v", stored)
	}
	if again, err := client.CreateEvent(ctx, "access", "mine", first, true); err != nil || again != written || fake.posts != 1 || fake.views != 1 {
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

	again := first
	again.CreatedAt = first.CreatedAt.Add(time.Minute) // accepted again after the undo
	if _, err := client.CreateEvent(ctx, "access", "mine", again, false); err != nil || fake.posts != 2 || len(fake.events) != 1 {
		t.Fatalf("accepted again: %v; %d posts, %d held", err, fake.posts, len(fake.events))
	}
}

// A retry finds an earlier attempt on any page of the time's events.
func TestAMicrosoftRetryLooksOnEveryPage(t *testing.T) {
	fake := newFakeCalendars(t, Microsoft)
	client, ctx := fake.client(), context.Background()
	accepted := acceptedTime("calendar_event_owned_01")
	written, err := client.CreateEvent(ctx, "access", "mine", accepted, false)
	if err != nil {
		t.Fatal(err)
	}
	fake.events["other"] = map[string]any{"id": "other", "transactionId": "someone-elses"}
	fake.paged = true
	found, ok, err := client.microsoftFind(ctx, "access", "mine", accepted, TransactionID(accepted))
	if err != nil || !ok || found.ID != written.ID || fake.views < 2 {
		t.Fatalf("found %+v, %v, %v after %d pages", found, ok, err, fake.views)
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
