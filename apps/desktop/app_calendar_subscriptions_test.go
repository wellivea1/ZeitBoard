package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// syntheticSubscription serves a calendar at an address holding a canary
// secret, and can be told to fail or to add an event.
type syntheticSubscription struct {
	server  *httptest.Server
	failing atomic.Bool
	moved   atomic.Bool
	fetches atomic.Int32
}

const subscriptionSecret = "private-canary-secret-path"

func serveSubscription(t *testing.T) *syntheticSubscription {
	t.Helper()
	served := &syntheticSubscription{}
	served.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		served.fetches.Add(1)
		if request.Method != http.MethodGet || !strings.Contains(request.URL.Path, subscriptionSecret) {
			http.NotFound(response, request)
			return
		}
		if served.failing.Load() {
			http.Error(response, "unavailable", http.StatusServiceUnavailable)
			return
		}
		calendar := desktopCalendarFixture
		if served.moved.Load() {
			calendar = strings.Replace(calendar, "END:VCALENDAR\r\n", "BEGIN:VEVENT\r\n"+
				"UID:desktop-calendar-3@example.test\r\n"+
				"DTSTART;TZID=America/New_York:20260722T150000\r\n"+
				"DTEND;TZID=America/New_York:20260722T160000\r\n"+
				"SUMMARY:Added later\r\n"+
				"END:VEVENT\r\nEND:VCALENDAR\r\n", 1)
		}
		response.Header().Set("Content-Type", "text/calendar")
		_, _ = response.Write([]byte(calendar))
	}))
	t.Cleanup(served.server.Close)
	return served
}

func (s *syntheticSubscription) address() string {
	return s.server.URL + "/ical/" + subscriptionSecret + "/basic.ics"
}

func subscriptionApp(t *testing.T) *App {
	t.Helper()
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }
	return app
}

// A calendar is previewed from its address, kept on subscribing, refreshed
// whole, and keeps its copy when a refresh fails. Its address never comes
// back to the window.
func TestASubscribedCalendarRefreshesItself(t *testing.T) {
	app := subscriptionApp(t)
	served := serveSubscription(t)
	input := CalendarSubscriptionInput{Address: served.address(), Label: "Work", ZoneID: "America/New_York"}

	preview, err := app.PreviewCalendarSubscription(input)
	if err != nil || preview.Imported || preview.EventCount != 2 {
		t.Fatalf("preview %+v, %v", preview, err)
	}
	query := CalendarQueryInput{StartCivilDate: "2026-07-22", Days: 2, ZoneID: "America/New_York"}
	if before, err := app.GetCalendar(query); err != nil || len(before.Sources) != 0 {
		t.Fatalf("a preview kept something: %+v, %v", before.Sources, err)
	}

	kept, err := app.SubscribeCalendar(input)
	if err != nil || !kept.Imported || kept.SourceID != preview.SourceID {
		t.Fatalf("subscribe %+v, %v", kept, err)
	}
	calendar, err := app.GetCalendar(query)
	if err != nil || len(calendar.Sources) != 1 {
		t.Fatalf("calendar %+v, %v", calendar.Sources, err)
	}
	source := calendar.Sources[0]
	host := served.server.Listener.Addr().String()
	if source.Label != "Work" || source.Subscription == nil || !strings.HasPrefix(host, source.Subscription.Host) ||
		source.Subscription.RefreshedLabel == "" || source.Subscription.Failure != "" || source.Endpoint != "" {
		t.Fatalf("source %+v", source)
	}
	assertNoSecret(t, calendar)

	// The calendar gains an event; the hourly pass replaces the copy.
	served.moved.Store(true)
	app.refreshFetchedCalendars(app.applicationContext())
	calendar, _ = app.GetCalendar(query)
	if events := countCalendarEvents(calendar); events != 3 {
		t.Fatalf("after a refresh %d events", events)
	}

	// The server fails: the copy stays, and the row says why.
	served.failing.Store(true)
	if _, err := app.RefreshCalendarSource(CalendarRefreshInput{SourceID: source.SourceID}); err == nil ||
		!strings.Contains(err.Error(), "answered HTTP 503") || strings.Contains(err.Error(), subscriptionSecret) {
		t.Fatalf("a failed refresh read as %v", err)
	}
	calendar, _ = app.GetCalendar(query)
	if events := countCalendarEvents(calendar); events != 3 || calendar.Sources[0].Subscription.Failure == "" {
		t.Fatalf("after a failure %d events, %+v", events, calendar.Sources[0].Subscription)
	}
	assertNoSecret(t, calendar)

	// Removing the calendar erases its address.
	if err := app.RemoveCalendarSource(RemoveCalendarSourceInput{SourceID: source.SourceID, Confirmation: removeConfirm}); err != nil {
		t.Fatal(err)
	}
	if subscriptions, err := app.store.CalendarSubscriptions(app.applicationContext()); err != nil || len(subscriptions) != 0 {
		t.Fatalf("the address outlived its calendar: %+v, %v", subscriptions, err)
	}
	fetches := served.fetches.Load()
	app.refreshFetchedCalendars(app.applicationContext())
	if served.fetches.Load() != fetches {
		t.Fatal("a removed calendar was fetched again")
	}
}

func countCalendarEvents(calendar CalendarDTO) int {
	seen := map[string]bool{}
	for _, day := range calendar.Days {
		for _, event := range day.Events {
			seen[event.EventID] = true
		}
	}
	return len(seen)
}

func assertNoSecret(t *testing.T, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), subscriptionSecret) {
		t.Fatal("the calendar's address reached the window")
	}
}

func TestAnAddressIsReadAsPasted(t *testing.T) {
	address, _, err := calendarSubscriptionAddress("  webcal://calendar.example.test/ical/basic.ics ")
	if err != nil || address != "https://calendar.example.test/ical/basic.ics" {
		t.Fatalf("webcal read as %q, %v", address, err)
	}
	for _, refused := range []string{
		"http://calendar.example.test/basic.ics",
		"https://owner:secret@calendar.example.test/basic.ics",
		"calendar.example.test/basic.ics",
		"",
	} {
		if _, _, err := calendarSubscriptionAddress(refused); err == nil {
			t.Errorf("accepted %q", refused)
		}
	}
}

// A provider may redirect, but only to another HTTPS address.
func TestRedirectsStayOnHTTPS(t *testing.T) {
	client := newCalendarSubscriptionClient()
	to := func(raw string) *http.Request {
		parsed, _ := url.Parse(raw)
		return &http.Request{URL: parsed}
	}
	via := []*http.Request{to("https://calendar.example.test/a.ics")}
	if err := client.CheckRedirect(to("https://p45-calendars.example.test/a.ics"), via); err != nil {
		t.Errorf("an HTTPS redirect was refused: %v", err)
	}
	if err := client.CheckRedirect(to("http://calendar.example.test/a.ics"), via); err == nil {
		t.Error("a redirect to plain HTTP was followed")
	}
}
