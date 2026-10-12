package calendaraccounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	calendarcore "non24.app/core/calendar"
)

// fakeCalendars is a provider's calendar API holding events in one calendar
// the owner owns ("mine") and one shared with them ("theirs"). It versions
// every stored event and honours what each provider promises: Google's
// client-chosen IDs and If-Match, Microsoft's transaction IDs and change keys.
type fakeCalendars struct {
	t        *testing.T
	provider Provider
	server   *httptest.Server

	mu      sync.Mutex
	events  map[string]map[string]any // by ID
	version int
	posts   int
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
		f.stamp(event)
		f.events[id] = event
		write(http.StatusOK, event)
	case path == "/calendars/mine/events" && r.Method == http.MethodGet:
		items := []map[string]any{}
		for _, event := range f.events {
			items = append(items, event)
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
		case r.Header.Get("If-Match") != "" && r.Header.Get("If-Match") != event["etag"]:
			write(http.StatusPreconditionFailed, map[string]string{"error": "conditionNotMet"})
		default:
			delete(f.events, id)
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
		f.posts++
		id := fmt.Sprintf("m-%d", f.posts)
		event["id"] = id
		f.stamp(event)
		f.events[id] = event
		write(http.StatusCreated, event)
	case path == "/me/calendars/mine/calendarView":
		items := []map[string]any{}
		for _, event := range f.events {
			items = append(items, event)
		}
		write(http.StatusOK, map[string]any{"value": items})
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

// Google: an accepted time is written once under ZeitBoard's own ID, read
// back as ZeitBoard's, removed as it was left, and left alone once the
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
	written, err := client.CreateEvent(ctx, "access", "mine", first)
	if err != nil || written.ID != googleEventID(first.EventID) || written.Version == "" {
		t.Fatalf("written %+v, %v", written, err)
	}
	// The answer was lost, so it is written again: the first is found.
	again, err := client.CreateEvent(ctx, "access", "mine", first)
	if err != nil || again != written || fake.posts != 1 {
		t.Fatalf("a retried create gave %+v, %v after %d posts", again, err, fake.posts)
	}
	stored := fake.events[written.ID]
	if stored["transparency"] != "opaque" || stored["start"].(map[string]any)["timeZone"] != "America/New_York" {
		t.Fatalf("stored %+v", stored)
	}

	// Reading the calendar back names it as ZeitBoard's.
	events, err := client.Events(ctx, "access", "mine", first.StartAt.Add(-time.Hour), first.EndAt.Add(time.Hour))
	if err != nil || len(events) != 1 || events[0].ID != calendarcore.OwnUID(first.EventID) {
		t.Fatalf("read back %+v, %v", events, err)
	}

	// Someone else's event where ZeitBoard writes the second.
	second := acceptedTime("calendar_event_owned_02")
	fake.events[googleEventID(second.EventID)] = map[string]any{"id": googleEventID(second.EventID), "etag": `"0"`}
	if _, err := client.CreateEvent(ctx, "access", "mine", second); !errors.Is(err, ErrTaken) {
		t.Fatalf("an event in the way read as %v", err)
	}

	fake.change(written.ID)
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, written); !errors.Is(err, ErrChanged) {
		t.Fatalf("removing a changed event read as %v", err)
	}
	if err := client.DeleteEvent(ctx, "access", "mine", second.EventID, written); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("removing under another time's name read as %v", err)
	}
	current := Written{ID: written.ID, Version: fake.events[written.ID]["etag"].(string)}
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, current); err != nil {
		t.Fatal(err)
	}
	if _, kept := fake.events[written.ID]; kept {
		t.Fatal("the event was not removed")
	}
	if err := client.DeleteEvent(ctx, "access", "mine", first.EventID, current); err != nil {
		t.Fatalf("an event already gone read as %v", err)
	}
}

// Microsoft: a transaction ID makes a retried create find the first, a
// change key tells the owner's edit, and a removal touches only an event
// carrying ZeitBoard's UID.
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
	written, err := client.CreateEvent(ctx, "access", "mine", first)
	if err != nil || written.ID == "" || written.Version == "" {
		t.Fatalf("written %+v, %v", written, err)
	}
	stored := fake.events[written.ID]
	if stored["transactionId"] != calendarcore.OwnUID(first.EventID) || stored["showAs"] != "busy" ||
		stored["start"].(map[string]any)["dateTime"] != "2026-10-14T15:00:00" {
		t.Fatalf("stored %+v", stored)
	}
	if again, err := client.CreateEvent(ctx, "access", "mine", first); err != nil || again != written || fake.posts != 1 {
		t.Fatalf("a retried create gave %+v, %v after %d posts", again, err, fake.posts)
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
