package calendaraccounts

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
)

// Writing accepted times to a calendar the owner owns (ADR-0065), with the
// guarantees CalDAV write-back gives (ADR-0053). ZeitBoard writes and removes
// only events it made: in Google under an ID derived from its own and with a
// private mark, in Microsoft with its UID as the transaction ID. A create
// cannot overwrite another event, and a removal leaves an event the owner has
// changed since.

var (
	// ErrTaken is a create that found a different event where ZeitBoard
	// writes this one.
	ErrTaken = errors.New("Your calendar already has a different event where ZeitBoard writes this time.")

	// ErrChanged is a removal that found the event changed since ZeitBoard
	// wrote it.
	ErrChanged = errors.New("It was changed in your calendar after ZeitBoard wrote it.")

	// ErrNotOurs is a removal of an event ZeitBoard did not write.
	ErrNotOurs = errors.New("This event is not one ZeitBoard wrote, so it was left alone.")
)

// Written is an event ZeitBoard wrote: where it is, and the version of it a
// removal must still find.
type Written struct {
	ID      string
	Version string
}

// ownMark is the private property that marks a Google event as ZeitBoard's.
const ownMark = "zeitboardEvent"

// immutableIDs keeps a Microsoft event's ID when the owner moves it between
// calendars, so a kept ID still finds it.
const immutableIDs = `IdType="ImmutableId"`

// googleEventID is the ID an accepted time is written under in Google
// Calendar: base32hex, as Google requires of an ID a client chooses, derived
// from ZeitBoard's own so that a retried create finds the first.
func googleEventID(eventID string) string {
	sum := sha256.Sum256([]byte(eventID))
	return strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:20]))
}

// CreateEvent writes an accepted time into a calendar. An event an earlier
// attempt wrote, whose answer was lost, is found and kept rather than written
// twice.
func (c Client) CreateEvent(ctx context.Context, access, calendarID string, event calendarcore.Event) (Written, error) {
	if event.Ownership != calendarcore.OwnershipAppOwned || strings.TrimSpace(calendarID) == "" {
		return Written{}, errors.New("only an accepted time is written, to a named calendar")
	}
	if c.Provider == Microsoft {
		return c.microsoftCreate(ctx, access, calendarID, event)
	}
	return c.googleCreate(ctx, access, calendarID, event)
}

// DeleteEvent removes an undone accepted time, unless the owner has changed
// it since ZeitBoard wrote it. An event already gone counts as removed.
func (c Client) DeleteEvent(ctx context.Context, access, calendarID, eventID string, written Written) error {
	if c.Provider == Microsoft {
		return c.microsoftDelete(ctx, access, eventID, written)
	}
	return c.googleDelete(ctx, access, calendarID, eventID, written)
}

// CanWrite reports whether ZeitBoard may add events to a calendar: in Google
// one the owner owns, which is all its write scope reaches; in Microsoft one
// the owner can edit.
func (c Client) CanWrite(ctx context.Context, access, calendarID string) (bool, error) {
	if c.Provider == Microsoft {
		var calendar struct {
			CanEdit bool `json:"canEdit"`
		}
		address := c.Endpoints.API + "/me/calendars/" + url.PathEscape(calendarID) + "?" + url.Values{"$select": {"canEdit"}}.Encode()
		err := c.get(ctx, address, access, nil, &calendar)
		return calendar.CanEdit, err
	}
	var entry struct {
		AccessRole string `json:"accessRole"`
	}
	err := c.get(ctx, c.Endpoints.API+"/users/me/calendarList/"+url.PathEscape(calendarID), access, nil, &entry)
	return entry.AccessRole == "owner", err
}

func (c Client) googleEventsAddress(calendarID string) string {
	return c.Endpoints.API + "/calendars/" + url.PathEscape(calendarID) + "/events"
}

// googleBoundary is an event's start or end as Google takes it: a date for an
// all-day event, an instant otherwise, with the event's zone when it names one.
func googleBoundary(event calendarcore.Event, at time.Time) map[string]string {
	location, err := time.LoadLocation(event.ZoneID)
	if event.AllDay {
		if err != nil {
			location = time.UTC
		}
		return map[string]string{"date": at.In(location).Format("2006-01-02")}
	}
	boundary := map[string]string{"dateTime": at.UTC().Format(time.RFC3339)}
	if err == nil && event.ZoneID != "" {
		boundary["timeZone"] = event.ZoneID
	}
	return boundary
}

func (c Client) googleCreate(ctx context.Context, access, calendarID string, event calendarcore.Event) (Written, error) {
	id := googleEventID(event.EventID)
	transparency := "opaque"
	if !event.Busy {
		transparency = "transparent"
	}
	body := map[string]any{
		"id":                 id,
		"summary":            event.Title,
		"start":              googleBoundary(event, event.StartAt),
		"end":                googleBoundary(event, event.EndAt),
		"transparency":       transparency,
		"extendedProperties": map[string]any{"private": map[string]string{ownMark: event.EventID}},
	}
	if event.Location != "" {
		body["location"] = event.Location
	}
	if event.Notes != "" {
		body["description"] = event.Notes
	}
	var created struct {
		ID   string `json:"id"`
		ETag string `json:"etag"`
	}
	status, err := c.call(ctx, http.MethodPost, c.googleEventsAddress(calendarID), access, nil, body, &created)
	switch {
	case err != nil:
		return Written{}, err
	case status == http.StatusOK || status == http.StatusCreated:
		return Written{ID: id, Version: created.ETag}, nil
	case status != http.StatusConflict:
		return Written{}, answered(c.Endpoints.API, status)
	}
	// An event already has this ID: an earlier attempt whose answer was lost,
	// if it carries ZeitBoard's mark for this time.
	var existing struct {
		ETag               string `json:"etag"`
		ExtendedProperties struct {
			Private map[string]string `json:"private"`
		} `json:"extendedProperties"`
	}
	if err := c.get(ctx, c.googleEventsAddress(calendarID)+"/"+id, access, nil, &existing); err != nil {
		return Written{}, err
	}
	if existing.ExtendedProperties.Private[ownMark] != event.EventID {
		return Written{}, ErrTaken
	}
	return Written{ID: id, Version: existing.ETag}, nil
}

func (c Client) googleDelete(ctx context.Context, access, calendarID, eventID string, written Written) error {
	if written.ID != googleEventID(eventID) {
		return ErrNotOurs
	}
	headers := map[string]string{}
	if written.Version != "" {
		headers["If-Match"] = written.Version
	}
	address := c.googleEventsAddress(calendarID) + "/" + written.ID
	status, err := c.call(ctx, http.MethodDelete, address, access, headers, nil, nil)
	switch {
	case err != nil:
		return err
	case status == http.StatusOK || status == http.StatusNoContent || status == http.StatusNotFound || status == http.StatusGone:
		return nil
	case status == http.StatusPreconditionFailed:
		return ErrChanged
	default:
		return answered(address, status)
	}
}

// graphBoundary is an event's start or end as Graph takes it, in UTC; an
// all-day event's is its date at midnight.
func graphBoundary(event calendarcore.Event, at time.Time) map[string]string {
	if event.AllDay {
		location, err := time.LoadLocation(event.ZoneID)
		if err != nil {
			location = time.UTC
		}
		at = time.Date(at.In(location).Year(), at.In(location).Month(), at.In(location).Day(), 0, 0, 0, 0, time.UTC)
	}
	return map[string]string{"dateTime": at.UTC().Format("2006-01-02T15:04:05"), "timeZone": "UTC"}
}

func (c Client) microsoftCreate(ctx context.Context, access, calendarID string, event calendarcore.Event) (Written, error) {
	uid := calendarcore.OwnUID(event.EventID)
	// An earlier attempt may have landed with its answer lost; look first.
	// Graph also drops a second create with the same transaction ID.
	if found, ok, err := c.microsoftFind(ctx, access, calendarID, event, uid); err != nil || ok {
		return found, err
	}
	showAs := "busy"
	if !event.Busy {
		showAs = "free"
	}
	body := map[string]any{
		"subject":       event.Title,
		"start":         graphBoundary(event, event.StartAt),
		"end":           graphBoundary(event, event.EndAt),
		"isAllDay":      event.AllDay,
		"showAs":        showAs,
		"transactionId": uid,
	}
	if event.Location != "" {
		body["location"] = map[string]string{"displayName": event.Location}
	}
	if event.Notes != "" {
		body["body"] = map[string]string{"contentType": "text", "content": event.Notes}
	}
	var created struct {
		ID        string `json:"id"`
		ChangeKey string `json:"changeKey"`
	}
	address := c.Endpoints.API + "/me/calendars/" + url.PathEscape(calendarID) + "/events"
	status, err := c.call(ctx, http.MethodPost, address, access, map[string]string{"Prefer": immutableIDs}, body, &created)
	switch {
	case err != nil:
		return Written{}, err
	case (status == http.StatusCreated || status == http.StatusOK) && created.ID != "":
		return Written{ID: created.ID, Version: created.ChangeKey}, nil
	default:
		return Written{}, answered(address, status)
	}
}

// microsoftFind looks for an accepted time already written, by its
// transaction ID, over the time it takes.
func (c Client) microsoftFind(ctx context.Context, access, calendarID string, event calendarcore.Event, uid string) (Written, bool, error) {
	query := url.Values{
		"startDateTime": {event.StartAt.UTC().Format(time.RFC3339)},
		"endDateTime":   {event.EndAt.UTC().Format(time.RFC3339)},
		"$select":       {"id,changeKey,transactionId"},
		"$top":          {"100"},
	}
	var answer struct {
		Value []struct {
			ID            string `json:"id"`
			ChangeKey     string `json:"changeKey"`
			TransactionID string `json:"transactionId"`
		} `json:"value"`
	}
	address := c.Endpoints.API + "/me/calendars/" + url.PathEscape(calendarID) + "/calendarView?" + query.Encode()
	if err := c.get(ctx, address, access, map[string]string{"Prefer": immutableIDs}, &answer); err != nil {
		return Written{}, false, err
	}
	for _, item := range answer.Value {
		if item.TransactionID == uid {
			return Written{ID: item.ID, Version: item.ChangeKey}, true, nil
		}
	}
	return Written{}, false, nil
}

func (c Client) microsoftDelete(ctx context.Context, access, eventID string, written Written) error {
	if strings.TrimSpace(written.ID) == "" {
		return ErrNotOurs
	}
	address := c.Endpoints.API + "/me/events/" + url.PathEscape(written.ID)
	headers := map[string]string{"Prefer": immutableIDs}
	var current struct {
		ChangeKey     string `json:"changeKey"`
		TransactionID string `json:"transactionId"`
	}
	status, err := c.call(ctx, http.MethodGet, address+"?"+url.Values{"$select": {"changeKey,transactionId"}}.Encode(), access, headers, nil, &current)
	switch {
	case err != nil:
		return err
	case status == http.StatusNotFound:
		return nil
	case status != http.StatusOK:
		return answered(address, status)
	case current.TransactionID != calendarcore.OwnUID(eventID):
		return ErrNotOurs
	case written.Version != "" && current.ChangeKey != written.Version:
		return ErrChanged
	}
	status, err = c.call(ctx, http.MethodDelete, address, access, headers, nil, nil)
	switch {
	case err != nil:
		return err
	case status == http.StatusNoContent || status == http.StatusOK || status == http.StatusNotFound:
		return nil
	default:
		return answered(address, status)
	}
}
