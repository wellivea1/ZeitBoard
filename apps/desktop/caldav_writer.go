package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
)

// calDAVWriter writes ZeitBoard's own events into one calendar collection
// (RFC 4791). It creates an event only where nothing is yet (If-None-Match),
// and removes one only as ZeitBoard left it (If-Match), so it cannot overwrite
// or delete anything the owner made or changed.
type calDAVWriter struct {
	client     calendarHTTPDoer
	collection *url.URL
	username   string
	password   string
}

// What one write came to. A conflict waits for the owner; a failure is retried.
const (
	calendarWriteDone     = "done"
	calendarWriteConflict = "conflict"
	calendarWriteFailed   = "failed"
)

type calendarWriteResult struct {
	outcome string
	href    string
	etag    string
	detail  string
}

const (
	maxCalendarObjectBytes = 1 << 20
	calDAVProbeBody        = `<?xml version="1.0" encoding="utf-8"?>` +
		`<D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop>` +
		`<D:resourcetype/><D:current-user-privilege-set/><C:supported-calendar-component-set/>` +
		`</D:prop></D:propfind>`
)

func newCalDAVWriter(client calendarHTTPDoer, collection, username, password string) (calDAVWriter, error) {
	_, parsed, err := sanitizeCalDAVEndpoint(collection)
	if err != nil {
		return calDAVWriter{}, err
	}
	if password != "" && username == "" {
		return calDAVWriter{}, errors.New("CalDAV password requires a username")
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	if client == nil {
		client = newCalendarHTTPClient()
	}
	return calDAVWriter{client: client, collection: parsed, username: username, password: password}, nil
}

// objectURL is where an accepted time is written: named by the UID it carries,
// as calendar clients name their own events.
func (w calDAVWriter) objectURL(eventID string) string {
	return w.collection.JoinPath(calendarcore.OwnUID(eventID) + ".ics").String()
}

// create writes an accepted time as a new event.
func (w calDAVWriter) create(ctx context.Context, event calendarcore.Event, stamp time.Time) calendarWriteResult {
	body, err := calendarcore.CalendarObject(event, stamp)
	if err != nil {
		return calendarWriteResult{outcome: calendarWriteConflict, detail: "ZeitBoard could not turn this time into a calendar event."}
	}
	href := w.objectURL(event.EventID)
	response, reply, err := w.do(ctx, http.MethodPut, href, body, map[string]string{
		"Content-Type":  "text/calendar; charset=utf-8",
		"If-None-Match": "*",
	})
	if err != nil {
		return calendarWriteResult{outcome: calendarWriteFailed, detail: calendarErrorDetail(err)}
	}
	switch status := response.StatusCode; {
	case status >= 200 && status < 300:
		etag := strongETag(response.Header.Get("ETag"))
		if etag == "" {
			// The server changed what it stored, or does not say: ask what it
			// holds now, so a later removal can still tell an owner's change.
			if uid, current, status, err := w.fetch(ctx, href); err == nil && status == http.StatusOK && uid == calendarcore.OwnUID(event.EventID) {
				etag = current
			}
		}
		return calendarWriteResult{outcome: calendarWriteDone, href: href, etag: etag}
	case status == http.StatusPreconditionFailed:
		// Something is already there. Usually this very event, written by an
		// attempt whose answer was lost; then it is ZeitBoard's to keep.
		uid, etag, status, err := w.fetch(ctx, href)
		switch {
		case err != nil:
			return calendarWriteResult{outcome: calendarWriteFailed, detail: calendarErrorDetail(err)}
		case status == http.StatusOK && uid == calendarcore.OwnUID(event.EventID):
			return calendarWriteResult{outcome: calendarWriteDone, href: href, etag: etag}
		case status == http.StatusOK:
			return calendarWriteResult{outcome: calendarWriteConflict, detail: "Your calendar already has a different event where ZeitBoard writes this time."}
		default:
			return calendarWriteResult{outcome: calendarWriteFailed, detail: calendarStatusDetail(status)}
		}
	case (status == http.StatusForbidden || status == http.StatusConflict) && bytes.Contains(reply, []byte("no-uid-conflict")):
		return calendarWriteResult{outcome: calendarWriteConflict, detail: "Your calendar already has this time as another event, perhaps from a file ZeitBoard exported."}
	default:
		return calendarWriteResult{outcome: calendarWriteFailed, detail: calendarStatusDetail(status)}
	}
}

// remove takes an undone time out of the calendar, if the owner has not
// changed it there since ZeitBoard wrote it.
func (w calDAVWriter) remove(ctx context.Context, href, etag string) calendarWriteResult {
	if !w.owns(href) {
		return calendarWriteResult{outcome: calendarWriteConflict, detail: "This event is not where ZeitBoard writes, so it was left alone."}
	}
	headers := map[string]string{}
	if etag != "" {
		headers["If-Match"] = etag
	}
	response, _, err := w.do(ctx, http.MethodDelete, href, nil, headers)
	if err != nil {
		return calendarWriteResult{outcome: calendarWriteFailed, detail: calendarErrorDetail(err)}
	}
	switch status := response.StatusCode; {
	case status >= 200 && status < 300, status == http.StatusNotFound, status == http.StatusGone:
		return calendarWriteResult{outcome: calendarWriteDone}
	case status == http.StatusPreconditionFailed:
		return calendarWriteResult{outcome: calendarWriteConflict, detail: "It was changed in your calendar after ZeitBoard wrote it."}
	default:
		return calendarWriteResult{outcome: calendarWriteFailed, detail: calendarStatusDetail(status)}
	}
}

// probe checks that the sign-in opens the calendar and may add events to it,
// before ZeitBoard keeps it.
func (w calDAVWriter) probe(ctx context.Context) error {
	response, reply, err := w.do(ctx, "PROPFIND", w.collection.String(), []byte(calDAVProbeBody), map[string]string{
		"Content-Type": "application/xml; charset=utf-8",
		"Depth":        "0",
	})
	if err != nil {
		return errors.New(calendarErrorDetail(err))
	}
	switch response.StatusCode {
	case http.StatusMultiStatus:
	case http.StatusUnauthorized:
		return errors.New("Your calendar did not accept this sign-in.")
	case http.StatusForbidden:
		return errors.New("This sign-in cannot open the calendar.")
	default:
		return errors.New(calendarStatusDetail(response.StatusCode))
	}
	var status davMultistatus
	if err := xml.Unmarshal(reply, &status); err != nil {
		return errors.New("Your calendar's answer could not be read.")
	}
	return status.writableCalendar()
}

// owns reports whether href is an event ZeitBoard writes: one resource directly
// in its collection.
func (w calDAVWriter) owns(href string) bool {
	parsed, err := url.Parse(href)
	if err != nil || !sameURLOrigin(parsed, w.collection) || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	name, found := strings.CutPrefix(parsed.EscapedPath(), w.collection.EscapedPath())
	return found && name != "" && !strings.Contains(name, "/")
}

func (w calDAVWriter) fetch(ctx context.Context, href string) (uid, etag string, status int, err error) {
	response, reply, err := w.do(ctx, http.MethodGet, href, nil, map[string]string{"Accept": "text/calendar"})
	if err != nil {
		return "", "", 0, err
	}
	if response.StatusCode != http.StatusOK {
		return "", "", response.StatusCode, nil
	}
	uid, _ = calendarcore.CalendarObjectUID(reply)
	return uid, strongETag(response.Header.Get("ETag")), http.StatusOK, nil
}

// do sends one request and reads a bounded reply. The sign-in goes only to the
// collection's origin: the client refuses redirects that leave it.
func (w calDAVWriter) do(ctx context.Context, method, target string, body []byte, headers map[string]string) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, nil, err
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	if w.username != "" {
		request.SetBasicAuth(w.username, w.password)
	}
	response, err := w.client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	// A client following a 301 or 302 turns a write into a GET, whose success
	// would read as the write's.
	if response.Request != nil && response.Request.Method != method {
		return nil, nil, errCalendarMoved
	}
	reply, err := io.ReadAll(io.LimitReader(response.Body, maxCalendarObjectBytes))
	if err != nil {
		return nil, nil, err
	}
	return response, reply, nil
}

// strongETag keeps an ETag If-Match can compare. A weak one never matches, so
// keeping it would make every removal look like the owner's change.
func strongETag(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "W/") {
		return ""
	}
	return value
}

const calendarUnreachable = "ZeitBoard could not reach your calendar."

var errCalendarMoved = errors.New("Your calendar has moved. Add it again from its new address.")

func calendarErrorDetail(err error) string {
	if errors.Is(err, errCalendarMoved) {
		return errCalendarMoved.Error()
	}
	return calendarUnreachable
}

func calendarStatusDetail(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return "Your calendar did not accept ZeitBoard's sign-in."
	case status == http.StatusForbidden:
		return "Your calendar refused the change."
	case status == http.StatusNotFound:
		return "Your calendar was not found at its address."
	case status >= 500:
		return fmt.Sprintf("Your calendar's server had a problem (HTTP %d).", status)
	default:
		return fmt.Sprintf("Your calendar answered HTTP %d.", status)
	}
}

// What a PROPFIND on the collection says about it (RFC 4918, RFC 3744 and
// RFC 4791). A property a server does not report is not held against it.
type davMultistatus struct {
	Responses []struct {
		Propstats []struct {
			Prop   davCollectionProps `xml:"DAV: prop"`
			Status string             `xml:"DAV: status"`
		} `xml:"DAV: propstat"`
	} `xml:"DAV: response"`
}

type davCollectionProps struct {
	ResourceType *struct {
		Kinds []struct{ XMLName xml.Name } `xml:",any"`
	} `xml:"DAV: resourcetype"`
	Privileges *struct {
		Privilege []struct {
			Names []struct{ XMLName xml.Name } `xml:",any"`
		} `xml:"DAV: privilege"`
	} `xml:"DAV: current-user-privilege-set"`
	Components *struct {
		Comp []struct {
			Name string `xml:"name,attr"`
		} `xml:"urn:ietf:params:xml:ns:caldav comp"`
	} `xml:"urn:ietf:params:xml:ns:caldav supported-calendar-component-set"`
}

const calDAVNamespace = "urn:ietf:params:xml:ns:caldav"

func (m davMultistatus) writableCalendar() error {
	if len(m.Responses) == 0 {
		return errors.New("Your calendar's answer could not be read.")
	}
	for _, propstat := range m.Responses[0].Propstats {
		if !strings.Contains(propstat.Status, " 200") {
			continue
		}
		props := propstat.Prop
		if props.ResourceType != nil && !containsName(props.ResourceType.Kinds, calDAVNamespace, "calendar") {
			return errors.New("This address is not a calendar.")
		}
		if props.Privileges != nil {
			writable := false
			for _, privilege := range props.Privileges.Privilege {
				for _, name := range []string{"all", "write", "write-content", "bind"} {
					writable = writable || containsName(privilege.Names, "DAV:", name)
				}
			}
			if !writable {
				return errors.New("This sign-in can read the calendar but cannot add events to it.")
			}
		}
		if props.Components != nil {
			holdsEvents := false
			for _, component := range props.Components.Comp {
				holdsEvents = holdsEvents || strings.EqualFold(component.Name, "VEVENT")
			}
			if !holdsEvents {
				return errors.New("This calendar does not hold events.")
			}
		}
	}
	return nil
}

func containsName(names []struct{ XMLName xml.Name }, space, local string) bool {
	for _, name := range names {
		if name.XMLName.Space == space && name.XMLName.Local == local {
			return true
		}
	}
	return false
}
