package calendaraccounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
)

// Calendar is one calendar an account holds.
type Calendar struct {
	ID      string
	Name    string
	Primary bool
	// Owner is the account the calendar belongs to, as the provider names it.
	Owner string
}

// maxPages bounds how many pages one listing may take.
const maxPages = 50

// Calendars lists the account's calendars.
func (c Client) Calendars(ctx context.Context, access string) ([]Calendar, error) {
	if c.Provider == Microsoft {
		return c.microsoftCalendars(ctx, access)
	}
	return c.googleCalendars(ctx, access)
}

// Events reads one calendar's events between from and to, each occurrence
// of a repeating event on its own. An accepted time ZeitBoard wrote there
// (ADR-0065) comes back under ZeitBoard's own UID, so that reading it is
// not mistaken for the owner's commitment.
func (c Client) Events(ctx context.Context, access, calendarID string, from, to time.Time) ([]calendarcore.AccountEvent, error) {
	if strings.TrimSpace(calendarID) == "" {
		return nil, errors.New("no calendar was named")
	}
	if c.Provider == Microsoft {
		return c.microsoftEvents(ctx, access, calendarID, from, to)
	}
	return c.googleEvents(ctx, access, calendarID, from, to)
}

// call sends one request to the provider's API, with a JSON body when one is
// given, and gives back the status for the caller to judge.
func (c Client) call(ctx context.Context, method, address, access string, headers map[string]string, body, value any) (int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, address, reader)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+access)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, header := range headers {
		request.Header.Set(name, header)
	}
	if value == nil {
		value = &struct{}{}
	}
	return c.send(request, value)
}

func (c Client) get(ctx context.Context, address, access string, headers map[string]string, value any) error {
	status, err := c.call(ctx, http.MethodGet, address, access, headers, nil, value)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return answered(address, status)
	}
	return nil
}

// answered is an answer a request did not expect, naming the host alone.
func answered(address string, status int) error {
	host := address
	if parsed, err := url.Parse(address); err == nil {
		host = parsed.Hostname()
	}
	return fmt.Errorf("%s answered HTTP %d", host, status)
}

func (c Client) googleCalendars(ctx context.Context, access string) ([]Calendar, error) {
	var calendars []Calendar
	page := ""
	for range maxPages {
		query := url.Values{"maxResults": {"250"}}
		if page != "" {
			query.Set("pageToken", page)
		}
		var answer struct {
			Items []struct {
				ID      string `json:"id"`
				Summary string `json:"summary"`
				Primary bool   `json:"primary"`
			} `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := c.get(ctx, c.Endpoints.API+"/users/me/calendarList?"+query.Encode(), access, nil, &answer); err != nil {
			return nil, err
		}
		for _, item := range answer.Items {
			calendar := Calendar{ID: item.ID, Name: item.Summary, Primary: item.Primary}
			if item.Primary {
				calendar.Owner = item.ID
			}
			calendars = append(calendars, calendar)
		}
		if page = answer.NextPageToken; page == "" {
			return calendars, nil
		}
	}
	return nil, errors.New("the account lists more calendars than ZeitBoard reads")
}

// googleTime is an event boundary: a date for an all-day event, a time
// otherwise.
type googleTime struct {
	Date     string `json:"date"`
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

func (t googleTime) instant(fallback *time.Location) (time.Time, bool, error) {
	if t.DateTime != "" {
		at, err := time.Parse(time.RFC3339, t.DateTime)
		return at, false, err
	}
	location := fallback
	if t.TimeZone != "" {
		if zone, err := time.LoadLocation(t.TimeZone); err == nil {
			location = zone
		}
	}
	day, err := time.ParseInLocation("2006-01-02", t.Date, location)
	return day, true, err
}

func (c Client) googleEvents(ctx context.Context, access, calendarID string, from, to time.Time) ([]calendarcore.AccountEvent, error) {
	var events []calendarcore.AccountEvent
	page := ""
	// The calendar's own zone dates its all-day events; a later page may
	// leave it out.
	calendarZone, fallback := "", time.UTC
	for range maxPages {
		query := url.Values{
			"timeMin":      {from.UTC().Format(time.RFC3339)},
			"timeMax":      {to.UTC().Format(time.RFC3339)},
			"singleEvents": {"true"},
			"maxResults":   {"2500"},
		}
		if page != "" {
			query.Set("pageToken", page)
		}
		var answer struct {
			TimeZone string `json:"timeZone"`
			Items    []struct {
				ID                 string     `json:"id"`
				Status             string     `json:"status"`
				Summary            string     `json:"summary"`
				Location           string     `json:"location"`
				Transparency       string     `json:"transparency"`
				Start              googleTime `json:"start"`
				End                googleTime `json:"end"`
				ExtendedProperties struct {
					Private map[string]string `json:"private"`
				} `json:"extendedProperties"`
			} `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		address := c.Endpoints.API + "/calendars/" + url.PathEscape(calendarID) + "/events?" + query.Encode()
		if err := c.get(ctx, address, access, nil, &answer); err != nil {
			return nil, err
		}
		if answer.TimeZone != "" {
			if zone, err := time.LoadLocation(answer.TimeZone); err == nil {
				calendarZone, fallback = answer.TimeZone, zone
			}
		}
		for _, item := range answer.Items {
			if item.Status == "cancelled" {
				continue
			}
			start, allDay, err := item.Start.instant(fallback)
			if err != nil {
				return nil, fmt.Errorf("an event's start could not be read")
			}
			end, _, err := item.End.instant(fallback)
			if err != nil || end.Before(start) {
				return nil, fmt.Errorf("an event's end could not be read")
			}
			zone := item.Start.TimeZone
			if zone == "" {
				zone = calendarZone
			}
			id := item.ID
			if own := item.ExtendedProperties.Private[ownMark]; own != "" {
				id = calendarcore.OwnUID(own)
			}
			events = append(events, calendarcore.AccountEvent{
				ID: id, Title: item.Summary, Location: item.Location,
				StartAt: start, EndAt: end, ZoneID: validZone(zone), AllDay: allDay,
				Busy: item.Transparency != "transparent",
			})
		}
		if page = answer.NextPageToken; page == "" {
			return events, nil
		}
	}
	return nil, errors.New("the calendar holds more events than ZeitBoard reads at once")
}

func (c Client) microsoftCalendars(ctx context.Context, access string) ([]Calendar, error) {
	var calendars []Calendar
	address := c.Endpoints.API + "/me/calendars?$top=100"
	for range maxPages {
		var answer struct {
			Value []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				IsDefault bool   `json:"isDefaultCalendar"`
				Owner     struct {
					Address string `json:"address"`
				} `json:"owner"`
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}
		if err := c.get(ctx, address, access, nil, &answer); err != nil {
			return nil, err
		}
		for _, item := range answer.Value {
			calendars = append(calendars, Calendar{ID: item.ID, Name: item.Name, Primary: item.IsDefault, Owner: item.Owner.Address})
		}
		if answer.NextLink == "" {
			return calendars, nil
		}
		if !sameOrigin(answer.NextLink, c.Endpoints.API) {
			return nil, errors.New("the provider pointed the next page elsewhere")
		}
		address = answer.NextLink
	}
	return nil, errors.New("the account lists more calendars than ZeitBoard reads")
}

func (c Client) microsoftEvents(ctx context.Context, access, calendarID string, from, to time.Time) ([]calendarcore.AccountEvent, error) {
	query := url.Values{
		"startDateTime": {from.UTC().Format(time.RFC3339)},
		"endDateTime":   {to.UTC().Format(time.RFC3339)},
		"$top":          {"500"},
		"$select":       {"id,subject,location,start,end,isAllDay,showAs,isCancelled,transactionId"},
	}
	address := c.Endpoints.API + "/me/calendars/" + url.PathEscape(calendarID) + "/calendarView?" + query.Encode()
	// Times come back in UTC, so nothing depends on Windows zone names.
	headers := map[string]string{"Prefer": `outlook.timezone="UTC"`}
	var events []calendarcore.AccountEvent
	for range maxPages {
		var answer struct {
			Value []struct {
				ID       string `json:"id"`
				Subject  string `json:"subject"`
				Location struct {
					DisplayName string `json:"displayName"`
				} `json:"location"`
				Start struct {
					DateTime string `json:"dateTime"`
				} `json:"start"`
				End struct {
					DateTime string `json:"dateTime"`
				} `json:"end"`
				IsAllDay      bool   `json:"isAllDay"`
				ShowAs        string `json:"showAs"`
				IsCancelled   bool   `json:"isCancelled"`
				TransactionID string `json:"transactionId"`
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}
		if err := c.get(ctx, address, access, headers, &answer); err != nil {
			return nil, err
		}
		for _, item := range answer.Value {
			if item.IsCancelled {
				continue
			}
			start, err := graphTime(item.Start.DateTime)
			if err != nil {
				return nil, errors.New("an event's start could not be read")
			}
			end, err := graphTime(item.End.DateTime)
			if err != nil || end.Before(start) {
				return nil, errors.New("an event's end could not be read")
			}
			id := item.ID
			if _, own := calendarcore.OwnEventID(item.TransactionID); own {
				id = item.TransactionID
			}
			events = append(events, calendarcore.AccountEvent{
				ID: id, Title: item.Subject, Location: item.Location.DisplayName,
				StartAt: start, EndAt: end, ZoneID: "UTC", AllDay: item.IsAllDay,
				Busy: item.ShowAs != "free" && item.ShowAs != "workingElsewhere",
			})
		}
		if answer.NextLink == "" {
			return events, nil
		}
		if !sameOrigin(answer.NextLink, c.Endpoints.API) {
			return nil, errors.New("the provider pointed the next page elsewhere")
		}
		address = answer.NextLink
	}
	return nil, errors.New("the calendar holds more events than ZeitBoard reads at once")
}

// graphTime reads Graph's zone-less date-time, which with the UTC preference
// is UTC, written to seven decimal places.
func graphTime(value string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02T15:04:05.9999999", value, time.UTC)
}

func validZone(zone string) string {
	if zone == "" {
		return ""
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return ""
	}
	return zone
}

// sameOrigin keeps paging on the provider the listing began with, so a token
// is never sent to an address an answer chose.
func sameOrigin(address, base string) bool {
	next, err := url.Parse(address)
	if err != nil {
		return false
	}
	origin, err := url.Parse(base)
	if err != nil {
		return false
	}
	return strings.EqualFold(next.Scheme, origin.Scheme) && strings.EqualFold(next.Host, origin.Host)
}
