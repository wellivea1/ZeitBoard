package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
	storage "non24.app/core/storage/sqlite"
)

// Calendars by the private address their provider publishes (ADR-0062).
// Google Calendar, Outlook and iCloud each give a calendar an iCal address
// anyone holding it can read. ZeitBoard keeps the address on this computer,
// never shows it whole, and fetches it every hour to replace its copy.

const (
	calendarSubscriptionInterval = time.Hour
	maxCalendarSubscriptionBytes = 16 << 20

	// calendarRefreshedAnnouncement tells the window that a subscribed
	// calendar changed, or failed to, without it being asked.
	calendarRefreshedAnnouncement = "zeitboard:calendar-refreshed"
)

type CalendarSubscriptionInput struct {
	Address string `json:"address"`
	Label   string `json:"label"`
	ZoneID  string `json:"zoneId"`
}

// newCalendarSubscriptionClient follows a provider's redirects only while
// they stay on HTTPS. The address is the secret, and a redirect is chosen by
// the provider, so following one sends nothing it did not already have.
func newCalendarSubscriptionClient() *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many calendar redirects")
			}
			if !secureCalendarURL(request.URL) {
				return errors.New("a calendar redirect left HTTPS")
			}
			return nil
		},
	}
}

func secureCalendarURL(address *url.URL) bool {
	return strings.EqualFold(address.Scheme, "https") ||
		(strings.EqualFold(address.Scheme, "http") && calendarLoopbackHost(address.Hostname()))
}

// calendarSubscriptionAddress reads an address as pasted: webcal is HTTPS by
// another name.
func calendarSubscriptionAddress(raw string) (string, *url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if scheme, rest, found := strings.Cut(trimmed, "://"); found && strings.EqualFold(scheme, "webcal") {
		trimmed = "https://" + rest
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", nil, errors.New("Paste the calendar's whole address, starting with https:// or webcal://.")
	}
	if !secureCalendarURL(parsed) {
		return "", nil, errors.New("The calendar's address must use HTTPS.")
	}
	return parsed.String(), parsed, nil
}

func (a *App) PreviewCalendarSubscription(input CalendarSubscriptionInput) (CalendarImportDTO, error) {
	ctx := a.applicationContext()
	set, _, err := a.fetchCalendarSubscription(ctx, input, a.currentTime().UTC())
	if err != nil {
		return CalendarImportDTO{}, err
	}
	return a.calendarImportResult(ctx, set, nil)
}

// SubscribeCalendar keeps a calendar and its address, to be refreshed every
// hour from then on.
func (a *App) SubscribeCalendar(input CalendarSubscriptionInput) (CalendarImportDTO, error) {
	ctx := a.applicationContext()
	set, address, err := a.fetchCalendarSubscription(ctx, input, a.currentTime().UTC())
	if err != nil {
		return CalendarImportDTO{}, err
	}
	return a.calendarImportResult(ctx, set, keepSubscribed(address))
}

func (a *App) startCalendarSubscriptions(parent context.Context) {
	a.calendarSubscriber.start(parent, calendarSubscriptionInterval, a.refreshFetchedCalendars)
}

// refreshFetchedCalendars reads every subscribed calendar and every calendar
// brought in from an account once, and tells the window when there were any.
func (a *App) refreshFetchedCalendars(ctx context.Context) {
	store, err := a.requireStore()
	if err != nil {
		return
	}
	subscribed := a.refreshCalendarSubscriptions(ctx, store)
	fromAccounts := a.refreshAccountCalendars(ctx, store)
	if subscribed || fromAccounts {
		a.announce(calendarRefreshedAnnouncement)
	}
}

// refreshCalendarSubscriptions fetches every subscribed calendar once and
// reports whether there was any to fetch.
func (a *App) refreshCalendarSubscriptions(ctx context.Context, store *storage.Store) bool {
	subscriptions, err := store.CalendarSubscriptions(ctx)
	if err != nil || len(subscriptions) == 0 {
		return false
	}
	for _, subscription := range subscriptions {
		if ctx.Err() != nil {
			break
		}
		_, _ = a.refreshCalendarSubscription(ctx, store, subscription)
	}
	return true
}

// refreshCalendarSubscription replaces a subscribed calendar's copy, or
// records why it could not, keeping the copy it has.
func (a *App) refreshCalendarSubscription(ctx context.Context, store *storage.Store, subscription storage.CalendarSubscription) (CalendarImportDTO, error) {
	now := a.currentTime().UTC()
	set, address, err := a.fetchCalendarSubscription(ctx, CalendarSubscriptionInput{
		Address: subscription.Address, Label: subscription.Label,
	}, now)
	var result CalendarImportDTO
	if err == nil {
		result, err = a.calendarImportResult(ctx, set, keepSubscribed(address))
	}
	if err != nil {
		_ = store.RecordCalendarSubscriptionFailure(ctx, subscription.SourceID, now, err.Error())
		return CalendarImportDTO{}, err
	}
	return result, nil
}

// subscribedCalendar finds the subscription a source was kept under.
func subscribedCalendar(ctx context.Context, store *storage.Store, sourceID string) (storage.CalendarSubscription, bool, error) {
	subscriptions, err := store.CalendarSubscriptions(ctx)
	if err != nil {
		return storage.CalendarSubscription{}, false, err
	}
	for _, subscription := range subscriptions {
		if subscription.SourceID == sourceID {
			return subscription, true, nil
		}
	}
	return storage.CalendarSubscription{}, false, nil
}

// fetchCalendarSubscription reads a calendar from its address. No message it
// returns holds the address, which is a secret.
func (a *App) fetchCalendarSubscription(ctx context.Context, input CalendarSubscriptionInput, importedAt time.Time) (calendarcore.EventSet, string, error) {
	address, parsed, err := calendarSubscriptionAddress(input.Address)
	if err != nil {
		return calendarcore.EventSet{}, "", err
	}
	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = "Calendar from " + parsed.Hostname()
	}
	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		zoneID = localZoneID()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return calendarcore.EventSet{}, "", errors.New("The calendar's address could not be read.")
	}
	request.Header.Set("Accept", "text/calendar")
	client := a.calendarSubscriptionClient
	if client == nil {
		client = newCalendarSubscriptionClient()
	}
	response, err := client.Do(request)
	if err != nil {
		return calendarcore.EventSet{}, "", fmt.Errorf("%s could not be reached.", parsed.Hostname())
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return calendarcore.EventSet{}, "", fmt.Errorf("%s answered HTTP %d.", parsed.Hostname(), response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCalendarSubscriptionBytes+1))
	if err != nil {
		return calendarcore.EventSet{}, "", fmt.Errorf("The calendar from %s could not be read.", parsed.Hostname())
	}
	if len(data) > maxCalendarSubscriptionBytes {
		return calendarcore.EventSet{}, "", fmt.Errorf("The calendar from %s is larger than 16 MiB.", parsed.Hostname())
	}
	coverageStart, coverageEnd := calendarcore.CoverageAround(importedAt)
	set, err := calendarcore.ParseICS(data, calendarcore.ParseOptions{
		SourceID:      stableCalendarID("calendar_source_subscription", address),
		SourceLabel:   label,
		Kind:          calendarcore.SourceICS,
		ImportedAt:    importedAt,
		CoverageStart: coverageStart,
		CoverageEnd:   coverageEnd,
		DefaultZoneID: zoneID,
	})
	if err != nil {
		return calendarcore.EventSet{}, "", fmt.Errorf("The calendar from %s could not be read: %w", parsed.Hostname(), err)
	}
	return set, address, nil
}

// subscriptionHost names where a subscribed calendar comes from without
// saying its address.
func subscriptionHost(address string) string {
	parsed, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func calendarSubscriptionDTO(subscription storage.CalendarSubscription, now time.Time, location *time.Location) *CalendarSubscriptionDTO {
	return &CalendarSubscriptionDTO{
		Host:           subscriptionHost(subscription.Address),
		RefreshedLabel: calendarRefreshedLabel(subscription.RefreshedAt, now, location),
		Failure:        subscription.Failure,
	}
}

// calendarRefreshedLabel says when a fetched calendar was read: the time
// alone today, with its day before.
func calendarRefreshedLabel(at, now time.Time, location *time.Location) string {
	refreshed := at.In(location)
	label := displayClock().Time(refreshed)
	if today := now.In(location); refreshed.YearDay() != today.YearDay() || refreshed.Year() != today.Year() {
		label = refreshed.Format("Jan 2") + ", " + label
	}
	return label
}
