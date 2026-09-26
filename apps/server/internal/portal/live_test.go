package portal

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type streamEvent struct {
	name string
	data string
}

// openStream connects to a link's event stream over a real listener, since a
// response recorder cannot stream.
func openStream(t *testing.T, server *httptest.Server, token, cookie string) (*http.Response, *bufio.Reader) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/p/"+token+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response, bufio.NewReader(response.Body)
}

// nextEvent reads up to the next named event, skipping heartbeats and the
// retry hint.
func nextEvent(t *testing.T, reader *bufio.Reader) streamEvent {
	t.Helper()
	var event streamEvent
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("stream ended before an event: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			event.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			event.data = strings.TrimPrefix(line, "data: ")
		case line == "" && event.name != "":
			return event
		}
	}
}

func TestEventStreamSaysWhenTheProjectionChanges(t *testing.T) {
	h := newHarness(t)
	first := h.sampleSnapshot()
	h.publish(h.profile.ID, first)
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))
	// Registered first so it runs last, after each stream's own cleanup
	// has hung up; closing the server first would wait on open streams.
	server := httptest.NewServer(h.handler)
	t.Cleanup(server.Close)

	response, reader := openStream(t, server, h.token, cookie)
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream status = %d, type = %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	event := nextEvent(t, reader)
	var state streamState
	if err := json.Unmarshal([]byte(event.data), &state); err != nil || event.name != "state" {
		t.Fatalf("first event = %+v (%v)", event, err)
	}
	if state.Version != first.Version || state.Freshness != "current" {
		t.Errorf("first state = %+v", state)
	}

	second := first
	second.Version++
	h.publish(h.profile.ID, second)
	event = nextEvent(t, reader)
	if err := json.Unmarshal([]byte(event.data), &state); err != nil || state.Version != second.Version {
		t.Fatalf("after a publish the stream said %+v", event)
	}
	// An event says which version and how fresh, and nothing else: windows
	// travel only through the authenticated page.
	for _, leaked := range []string{"startAt", "windows", "zone", "2026-"} {
		if strings.Contains(event.data, leaked) {
			t.Errorf("event carries %q: %s", leaked, event.data)
		}
	}
}

func TestEventStreamsAreBoundedPerSession(t *testing.T) {
	h := newHarness(t)
	h.publish(h.profile.ID, h.sampleSnapshot())
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))
	// Registered first so it runs last, after each stream's own cleanup
	// has hung up; closing the server first would wait on open streams.
	server := httptest.NewServer(h.handler)
	t.Cleanup(server.Close)

	for i := 0; i < maxStreamsPerSession; i++ {
		response, reader := openStream(t, server, h.token, cookie)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("stream %d status = %d", i, response.StatusCode)
		}
		nextEvent(t, reader)
	}
	response, _ := openStream(t, server, h.token, cookie)
	if response.StatusCode != http.StatusTooManyRequests {
		t.Errorf("a stream past the session bound got %d, want 429 so the page polls", response.StatusCode)
	}
}

func TestRevocationEndsAnOpenStream(t *testing.T) {
	h := newHarness(t)
	h.publish(h.profile.ID, h.sampleSnapshot())
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))
	// Registered first so it runs last, after each stream's own cleanup
	// has hung up; closing the server first would wait on open streams.
	server := httptest.NewServer(h.handler)
	t.Cleanup(server.Close)

	_, reader := openStream(t, server, h.token, cookie)
	nextEvent(t, reader)
	if err := h.store.RevokeProfile(context.Background(), h.profile.ID, h.clock.Now()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if event := nextEvent(t, reader); event.name != "gone" {
		t.Errorf("after revocation the stream said %+v, want gone", event)
	}
}

func TestStreamNeedsASession(t *testing.T) {
	h := newHarness(t)
	// Registered first so it runs last, after each stream's own cleanup
	// has hung up; closing the server first would wait on open streams.
	server := httptest.NewServer(h.handler)
	t.Cleanup(server.Close)
	response, _ := openStream(t, server, h.token, "")
	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf("stream without a session = %d, want 401", response.StatusCode)
	}
}

// The page tells its script when its own claim next changes, so an open page
// changes its words as a window opens or closes rather than on a timer.
func TestPageSaysWhenItNextChanges(t *testing.T) {
	h := newHarness(t)
	snapshot := h.sampleSnapshot()
	h.publish(h.profile.ID, snapshot)
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))
	body := h.get("/p/"+h.token, cookie).Body.String()

	// The open window ends in six hours, when the estimate also turns stale.
	want := h.clock.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339)
	if !strings.Contains(body, `data-refresh-at="`+want+`"`) {
		t.Errorf("page does not say it next changes at %s", want)
	}
	if !strings.Contains(body, `<noscript><meta http-equiv="refresh" content="300"></noscript>`) {
		t.Error("a page without script no longer refreshes itself")
	}
}

// A refresh by the page's own script is a read of availability, not a visit,
// so the owner's "opened the page" count means what it says.
func TestInPlaceRefreshIsNotCountedAsAVisit(t *testing.T) {
	h := newHarness(t)
	h.publish(h.profile.ID, h.sampleSnapshot())
	cookie := h.sessionCookie(h.login(h.token, testPasscode, testOrigin))
	request := httptest.NewRequest(http.MethodGet, "/p/"+h.token, nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	request.Header.Set("X-Portal-Refresh", "1")
	request.RemoteAddr = "203.0.113.9:1234"
	h.handler.ServeHTTP(httptest.NewRecorder(), request)

	summaries, err := h.store.SummarizeAccess(context.Background(), h.profile.ID, h.clock.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range summaries {
		if summary.Event == EventPageView {
			t.Errorf("an in-place refresh was counted as a visit: %+v", summary)
		}
	}
}

func TestScriptIsServedFromThePortal(t *testing.T) {
	h := newHarness(t)
	recorder := h.get("/p/assets/portal.js", "")
	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("script status = %d type = %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	for _, reach := range []string{"http://", "https://", "eval(", "innerHTML"} {
		if strings.Contains(recorder.Body.String(), reach) {
			t.Errorf("script contains %q", reach)
		}
	}
}
