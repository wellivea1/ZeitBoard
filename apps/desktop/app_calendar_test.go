package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const desktopCalendarFixture = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//ZeitBoard Test//EN\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:desktop-calendar-1@example.test\r\n" +
	"DTSTART;TZID=America/New_York:20260722T233000\r\n" +
	"DTEND;TZID=America/New_York:20260723T010000\r\n" +
	"SUMMARY:Private appointment\r\n" +
	"LOCATION:Private office\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:desktop-calendar-2@example.test\r\n" +
	"DTSTART;VALUE=DATE:20260723\r\n" +
	"DTEND;VALUE=DATE:20260724\r\n" +
	"SUMMARY:Reserved day\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestCalendarFilePreviewCommitDisplayAndRemoval(t *testing.T) {
	app := newTestApp(t)
	input := CalendarFileInput{
		FileName: "commitments.ics",
		Contents: desktopCalendarFixture,
		ZoneID:   "America/New_York",
	}
	preview, err := app.PreviewCalendarFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Imported || preview.EventCount != 2 || preview.BusyCount != 2 || preview.AllDayCount != 1 || len(preview.Events) != 2 {
		t.Fatalf("preview = %#v", preview)
	}
	before, err := app.GetCalendar(CalendarQueryInput{StartCivilDate: "2026-07-22", Days: 2, ZoneID: input.ZoneID})
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Sources) != 0 || len(before.Days[0].Events) != 0 {
		t.Fatalf("preview wrote calendar data: %#v", before)
	}

	committed, err := app.ImportCalendarFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if !committed.Imported || committed.SourceID != preview.SourceID {
		t.Fatalf("commit = %#v", committed)
	}
	calendar, err := app.GetCalendar(CalendarQueryInput{StartCivilDate: "2026-07-22", Days: 2, ZoneID: input.ZoneID})
	if err != nil {
		t.Fatal(err)
	}
	if calendar.FixtureMode || len(calendar.Sources) != 1 || !calendar.Sources[0].ReadOnly || len(calendar.Days) != 2 {
		t.Fatalf("calendar = %#v", calendar)
	}
	if len(calendar.Days[0].Events) != 1 || !calendar.Days[0].Events[0].ContinuesAfter || calendar.Days[0].Events[0].Title != "Private appointment" {
		t.Fatalf("first civil day = %#v", calendar.Days[0])
	}
	if len(calendar.Days[1].Events) != 2 || !calendar.Days[1].Events[0].AllDay {
		t.Fatalf("second civil day = %#v", calendar.Days[1])
	}
	if err := app.RemoveCalendarSource(RemoveCalendarSourceInput{SourceID: preview.SourceID, Confirmation: "remove"}); err == nil {
		t.Fatal("source removal accepted the wrong confirmation")
	}
	if err := app.RemoveCalendarSource(RemoveCalendarSourceInput{SourceID: preview.SourceID, Confirmation: "REMOVE"}); err != nil {
		t.Fatal(err)
	}
	after, err := app.GetCalendar(CalendarQueryInput{StartCivilDate: "2026-07-22", Days: 2, ZoneID: input.ZoneID})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Sources) != 0 || len(after.Days[0].Events) != 0 || len(after.Days[1].Events) != 0 {
		t.Fatalf("removed source remained visible: %#v", after)
	}
}

func TestCalDAVPreviewAndImportUseBoundedReadOnlyReportWithoutPersistingCredentials(t *testing.T) {
	app := newTestApp(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != "REPORT" || request.Header.Get("Depth") != "1" {
			t.Errorf("request = %s, Depth %q", request.Method, request.Header.Get("Depth"))
		}
		username, password, ok := request.BasicAuth()
		if !ok || username != "owner" || password != "one-shot-secret" {
			t.Errorf("basic auth = %q, %q, %t", username, password, ok)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
		}
		if !strings.Contains(string(body), "calendar-query") || !strings.Contains(string(body), "time-range") {
			t.Errorf("REPORT body = %s", body)
		}
		response.Header().Set("Content-Type", "application/xml")
		response.WriteHeader(http.StatusMultiStatus)
		_, _ = response.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:response><D:propstat><D:prop><C:calendar-data><![CDATA[` + desktopCalendarFixture + `]]></C:calendar-data></D:prop></D:propstat></D:response>
</D:multistatus>`))
	}))
	defer server.Close()
	app.calendarHTTPClient = server.Client()
	input := CalDAVInput{
		Endpoint: server.URL + "/owner/calendar/",
		Label:    "Owner CalDAV",
		Username: "owner",
		Password: "one-shot-secret",
		ZoneID:   "America/New_York",
	}

	preview, err := app.PreviewCalDAVCalendar(input)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Imported || preview.Kind != "caldav" || preview.EventCount != 2 {
		t.Fatalf("preview = %#v", preview)
	}
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	sources, err := store.ListCalendarSources(requestContext(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("CalDAV preview wrote a source: %#v", sources)
	}

	committed, err := app.ImportCalDAVCalendar(input)
	if err != nil {
		t.Fatal(err)
	}
	if !committed.Imported || requests.Load() != 2 {
		t.Fatalf("commit = %#v, requests = %d", committed, requests.Load())
	}
	sources, err = store.ListCalendarSources(requestContext(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Endpoint != input.Endpoint {
		t.Fatalf("stored source = %#v", sources)
	}
	if strings.Contains(sources[0].Endpoint, input.Password) || strings.Contains(sources[0].Endpoint, "@") {
		t.Fatalf("stored endpoint contains credentials: %q", sources[0].Endpoint)
	}
}

func TestCalDAVRejectsCredentialURLsAndUnboundedResponses(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.PreviewCalDAVCalendar(CalDAVInput{
		Endpoint: "https://owner:secret@example.test/calendar/",
		ZoneID:   defaultZoneID,
	}); err == nil {
		t.Fatal("credential-bearing CalDAV URL was accepted")
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusMultiStatus)
		_, _ = response.Write([]byte(strings.Repeat("x", maxCalDAVResponseBytes+1)))
	}))
	defer server.Close()
	app.calendarHTTPClient = server.Client()
	if _, err := app.PreviewCalDAVCalendar(CalDAVInput{
		Endpoint: server.URL + "/calendar/",
		ZoneID:   defaultZoneID,
	}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func requestContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
