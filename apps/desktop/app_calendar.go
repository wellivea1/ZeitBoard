package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
	"non24.app/core/domain"
	"non24.app/core/outlook"
	storage "non24.app/core/storage/sqlite"
)

const (
	maxCalDAVResponseBytes   = 16 << 20
	maxCalendarPreviewEvents = 50
)

type calendarHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type CalendarFileInput struct {
	FileName string `json:"fileName"`
	Contents string `json:"contents"`
	ZoneID   string `json:"zoneId"`
}

type CalDAVInput struct {
	Endpoint string `json:"endpoint"`
	Label    string `json:"label"`
	Username string `json:"username"`
	Password string `json:"password"`
	ZoneID   string `json:"zoneId"`
}

type CalendarImportDTO struct {
	SourceID         string                   `json:"sourceId"`
	Label            string                   `json:"label"`
	Kind             string                   `json:"kind"`
	ReadOnly         bool                     `json:"readOnly"`
	Imported         bool                     `json:"imported"`
	EventCount       int                      `json:"eventCount"`
	BusyCount        int                      `json:"busyCount"`
	AllDayCount      int                      `json:"allDayCount"`
	CoverageStartAt  string                   `json:"coverageStartAt"`
	CoverageEndAt    string                   `json:"coverageEndAt"`
	CoverageLabel    string                   `json:"coverageLabel"`
	PreviewTruncated bool                     `json:"previewTruncated"`
	Events           []CalendarImportEventDTO `json:"events"`
	Message          string                   `json:"message"`
}

type CalendarImportEventDTO struct {
	EventID    string `json:"eventId"`
	Title      string `json:"title"`
	StartLabel string `json:"startLabel"`
	EndLabel   string `json:"endLabel"`
	AllDay     bool   `json:"allDay"`
	Busy       bool   `json:"busy"`
}

type RemoveCalendarSourceInput struct {
	SourceID     string `json:"sourceId"`
	Confirmation string `json:"confirmation"`
}

type CalendarExportDTO struct {
	FileName       string `json:"fileName"`
	ICS            string `json:"ics"`
	GeneratedAt    string `json:"generatedAt"`
	GeneratedLabel string `json:"generatedLabel"`
	EventCount     int    `json:"eventCount"`
}

type CalendarQueryInput struct {
	StartCivilDate string `json:"startCivilDate"`
	Days           int    `json:"days"`
	ZoneID         string `json:"zoneId"`
}

type CalendarDTO struct {
	Status         string              `json:"status"`
	Message        string              `json:"message"`
	FixtureMode    bool                `json:"fixtureMode"`
	ZoneID         string              `json:"zoneId"`
	StartCivilDate string              `json:"startCivilDate"`
	EndCivilDate   string              `json:"endCivilDate"`
	UpdatedLabel   string              `json:"updatedLabel"`
	Sources        []CalendarSourceDTO `json:"sources"`
	Days           []CalendarDayDTO    `json:"days"`
	Warnings       []string            `json:"warnings"`
}

type CalendarSourceDTO struct {
	SourceID      string `json:"sourceId"`
	Label         string `json:"label"`
	Kind          string `json:"kind"`
	ReadOnly      bool   `json:"readOnly"`
	Endpoint      string `json:"endpoint,omitempty"`
	VisibleEvents int    `json:"visibleEvents"`
	CoverageLabel string `json:"coverageLabel"`
	CoverageStart string `json:"coverageStart"`
	CoverageEnd   string `json:"coverageEnd"`

	// Subscription is set for a calendar kept by its address (ADR-0062), and
	// Account for one brought in from a Google or Microsoft account
	// (ADR-0064).
	Subscription *CalendarSubscriptionDTO  `json:"subscription,omitempty"`
	Account      *CalendarAccountSourceDTO `json:"account,omitempty"`
}

// CalendarSubscriptionDTO says where a subscribed calendar comes from, by
// host alone, and how its last refresh went.
type CalendarSubscriptionDTO struct {
	Host           string `json:"host"`
	RefreshedLabel string `json:"refreshedLabel"`
	Failure        string `json:"failure,omitempty"`
}

type CalendarDayDTO struct {
	CivilDate   string                    `json:"civilDate"`
	Label       string                    `json:"label"`
	IsToday     bool                      `json:"isToday"`
	Events      []CalendarEventSegmentDTO `json:"events"`
	Predictions []CalendarBandSegmentDTO  `json:"predictions"`
}

type CalendarEventSegmentDTO struct {
	SegmentID       string  `json:"segmentId"`
	EventID         string  `json:"eventId"`
	SourceID        string  `json:"sourceId"`
	SourceLabel     string  `json:"sourceLabel"`
	SourceKind      string  `json:"sourceKind"`
	Title           string  `json:"title"`
	StartAt         string  `json:"startAt"`
	EndAt           string  `json:"endAt"`
	StartLabel      string  `json:"startLabel"`
	EndLabel        string  `json:"endLabel"`
	StartMinute     float64 `json:"startMinute"`
	EndMinute       float64 `json:"endMinute"`
	AllDay          bool    `json:"allDay"`
	PointInTime     bool    `json:"pointInTime"`
	Busy            bool    `json:"busy"`
	Ownership       string  `json:"ownership"`
	ReadOnly        bool    `json:"readOnly"`
	ContinuesBefore bool    `json:"continuesBefore"`
	ContinuesAfter  bool    `json:"continuesAfter"`
	Location        string  `json:"location,omitempty"`
	Notes           string  `json:"notes,omitempty"`
}

type CalendarBandSegmentDTO struct {
	SegmentID       string  `json:"segmentId"`
	Kind            string  `json:"kind"`
	Title           string  `json:"title"`
	StartAt         string  `json:"startAt"`
	EndAt           string  `json:"endAt"`
	StartLabel      string  `json:"startLabel"`
	EndLabel        string  `json:"endLabel"`
	StartMinute     float64 `json:"startMinute"`
	EndMinute       float64 `json:"endMinute"`
	Confidence      string  `json:"confidence"`
	ContinuesBefore bool    `json:"continuesBefore"`
	ContinuesAfter  bool    `json:"continuesAfter"`
}

func newCalendarHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many CalDAV redirects")
			}
			if len(via) > 0 && !sameURLOrigin(via[0].URL, request.URL) {
				return errors.New("CalDAV redirect changed origin")
			}
			return nil
		},
	}
}

func (a *App) PreviewCalendarFile(input CalendarFileInput) (CalendarImportDTO, error) {
	set, err := parseCalendarFile(input, a.currentTime().UTC())
	if err != nil {
		return CalendarImportDTO{}, err
	}
	return a.calendarImportResult(a.applicationContext(), set, nil)
}

func (a *App) ImportCalendarFile(input CalendarFileInput) (CalendarImportDTO, error) {
	set, err := parseCalendarFile(input, a.currentTime().UTC())
	if err != nil {
		return CalendarImportDTO{}, err
	}
	return a.calendarImportResult(a.applicationContext(), set, keepImported(""))
}

func (a *App) PreviewCalDAVCalendar(input CalDAVInput) (CalendarImportDTO, error) {
	ctx := a.applicationContext()
	set, _, err := a.fetchCalDAVCalendar(ctx, input, a.currentTime().UTC())
	if err != nil {
		return CalendarImportDTO{}, err
	}
	return a.calendarImportResult(ctx, set, nil)
}

func (a *App) ImportCalDAVCalendar(input CalDAVInput) (CalendarImportDTO, error) {
	ctx := a.applicationContext()
	set, endpoint, err := a.fetchCalDAVCalendar(ctx, input, a.currentTime().UTC())
	if err != nil {
		return CalendarImportDTO{}, err
	}
	return a.calendarImportResult(ctx, set, keepImported(endpoint))
}

// calendarKeep keeps a fetched calendar in the store.
type calendarKeep func(ctx context.Context, store *storage.Store, set calendarcore.EventSet) error

// keepImported keeps an imported file or CalDAV calendar.
func keepImported(endpoint string) calendarKeep {
	return func(ctx context.Context, store *storage.Store, set calendarcore.EventSet) error {
		return store.ReplaceImportedCalendar(ctx, set.Sources[0], set.Events, endpoint)
	}
}

// keepSubscribed keeps a subscribed calendar with its address (ADR-0062).
func keepSubscribed(address string) calendarKeep {
	return func(ctx context.Context, store *storage.Store, set calendarcore.EventSet) error {
		return store.ReplaceSubscribedCalendar(ctx, set.Sources[0], set.Events, address)
	}
}

// calendarImportResult previews a calendar, or keeps it when keep is given,
// without ZeitBoard's own accepted times: a calendar ZeitBoard writes to
// returns them, and each is already on the board. Kept, the copy would be a
// busy commitment clashing with the very placement it copies.
func (a *App) calendarImportResult(ctx context.Context, set calendarcore.EventSet, keep calendarKeep) (CalendarImportDTO, error) {
	commit := keep != nil
	store, err := a.requireStore()
	if err != nil && commit {
		return CalendarImportDTO{}, err
	}
	recognised := 0
	if store != nil {
		own, err := store.OwnCalendarEventIDs(ctx)
		if err != nil {
			return CalendarImportDTO{}, err
		}
		kept := make([]calendarcore.Event, 0, len(set.Events))
		for _, event := range set.Events {
			if eventID, ownUID := calendarcore.OwnEventID(event.SourceRecordID); ownUID && own[eventID] {
				recognised++
				continue
			}
			kept = append(kept, event)
		}
		set.Events = kept
	}
	if commit {
		if err := keep(ctx, store, set); err != nil {
			return CalendarImportDTO{}, err
		}
	}
	dto := calendarImportDTO(set, commit)
	if recognised > 0 {
		dto.Message += fmt.Sprintf(" %d %s ZeitBoard's own accepted %s, already on the board.",
			recognised, plural(recognised, "was", "were"), plural(recognised, "time", "times"))
	}
	return dto, nil
}

func (a *App) RemoveCalendarSource(input RemoveCalendarSourceInput) error {
	if strings.TrimSpace(input.Confirmation) != removeConfirm {
		return errors.New("type REMOVE to confirm calendar source erasure")
	}
	store, err := a.requireStore()
	if err != nil {
		return err
	}
	return store.RemoveImportedCalendar(a.applicationContext(), strings.TrimSpace(input.SourceID))
}

func (a *App) ExportOwnedCalendar() (CalendarExportDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return CalendarExportDTO{}, err
	}
	events, err := store.OwnedCalendarEvents(a.applicationContext())
	if err != nil {
		return CalendarExportDTO{}, err
	}
	now := a.currentTime().UTC().Truncate(time.Second)
	data, err := calendarcore.ExportOwnedICS(events, now)
	if err != nil {
		return CalendarExportDTO{}, err
	}
	return CalendarExportDTO{
		FileName:       "zeitboard-placements-" + now.Format("20060102") + ".ics",
		ICS:            string(data),
		GeneratedAt:    now.Format(time.RFC3339),
		GeneratedLabel: displayClock().Format(now.Local(), "Jan 2, 2006, 3:04 PM"),
		EventCount:     len(events),
	}, nil
}

func (a *App) GetCalendar(input CalendarQueryInput) (CalendarDTO, error) {
	return a.getCalendar(a.applicationContext(), input)
}

func (a *App) getCalendar(ctx context.Context, input CalendarQueryInput) (CalendarDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return CalendarDTO{}, err
	}
	now := a.currentTime().UTC().Truncate(time.Minute)
	span, err := calendarSpanOf(input, now)
	if err != nil {
		return CalendarDTO{}, err
	}
	events, err := store.CalendarEvents(ctx, span.start.UTC(), span.end.UTC())
	if err != nil {
		return CalendarDTO{}, err
	}
	sources, err := store.ListCalendarSources(ctx)
	if err != nil {
		return CalendarDTO{}, err
	}
	sourceByID, warnings, err := calendarSourceDTOs(ctx, store, sources, events, span, now)
	if err != nil {
		return CalendarDTO{}, err
	}
	estimate, err := a.localEstimate(ctx, now)
	if err != nil {
		return CalendarDTO{}, err
	}

	dayDTOs, dayStarts := calendarDays(span, now)
	for _, event := range events {
		first, last := calendarOverlappingDayIndexes(dayStarts, event.StartAt, event.EndAt, event.StartAt.Equal(event.EndAt))
		for index := first; index < last; index++ {
			if segment, include := calendarEventSegment(event, sourceByID[event.SourceID], dayStarts[index], dayStarts[index+1], span.location); include {
				dayDTOs[index].Events = append(dayDTOs[index].Events, segment)
			}
		}
	}
	if estimate.Status == "estimated" {
		addCalendarPredictions(dayDTOs, dayStarts, estimate, now, span)
	}
	for index := range dayDTOs {
		sortCalendarDayEvents(dayDTOs[index].Events)
	}

	sourceDTOs := make([]CalendarSourceDTO, 0, len(sources))
	for _, source := range sources {
		sourceDTOs = append(sourceDTOs, sourceByID[source.SourceID])
	}
	message := fmt.Sprintf("%d real calendar event", len(events))
	if len(events) != 1 {
		message += "s"
	}
	message += " in this range. Imported text remains on this device."
	return CalendarDTO{
		Status:         estimate.Status,
		Message:        message,
		FixtureMode:    false,
		ZoneID:         span.zoneID,
		StartCivilDate: span.start.Format("2006-01-02"),
		EndCivilDate:   span.end.AddDate(0, 0, -1).Format("2006-01-02"),
		UpdatedLabel:   displayClock().Format(now.In(span.location), "Updated Jan 2, 3:04 PM MST"),
		Sources:        sourceDTOs,
		Days:           dayDTOs,
		Warnings:       warnings,
	}, nil
}

// calendarSpan is the days a calendar view shows, in its zone.
type calendarSpan struct {
	zoneID     string
	location   *time.Location
	days       int
	start, end time.Time
}

// calendarSpanOf reads the days asked for: from a civil date, in a zone, for
// one to fourteen days, five unless said.
func calendarSpanOf(input CalendarQueryInput, now time.Time) (calendarSpan, error) {
	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		zoneID = localZoneID()
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return calendarSpan{}, fmt.Errorf("load calendar zone %q: %w", zoneID, err)
	}
	days := input.Days
	if days == 0 {
		days = 5
	}
	if days < 1 || days > 14 {
		return calendarSpan{}, errors.New("calendar days must be between 1 and 14")
	}
	start, err := calendarCivilDate(input.StartCivilDate, now, location)
	if err != nil {
		return calendarSpan{}, err
	}
	return calendarSpan{zoneID: zoneID, location: location, days: days, start: start, end: start.AddDate(0, 0, days)}, nil
}

// calendarSourceDTOs describes each calendar: what of the range it holds, how
// it is refreshed, and the account it comes from. Each read-only one that
// does not cover the range gives a warning.
func calendarSourceDTOs(ctx context.Context, store *storage.Store, sources []storage.CalendarSourceRecord, events []calendarcore.Event, span calendarSpan, now time.Time) (map[string]CalendarSourceDTO, []string, error) {
	subscriptions, err := store.CalendarSubscriptions(ctx)
	if err != nil {
		return nil, nil, err
	}
	subscribed := make(map[string]storage.CalendarSubscription, len(subscriptions))
	for _, subscription := range subscriptions {
		subscribed[subscription.SourceID] = subscription
	}
	fromAccounts, err := calendarAccountSources(ctx, store, now, span.location)
	if err != nil {
		return nil, nil, err
	}

	visibleBySource := make(map[string]int, len(sources))
	for _, event := range events {
		visibleBySource[event.SourceID]++
	}
	sourceByID := make(map[string]CalendarSourceDTO, len(sources))
	warnings := make([]string, 0)
	for _, source := range sources {
		coverageStart := source.CoverageStartAt.In(span.location)
		coverageEnd := source.CoverageEndAt.In(span.location)
		coverageLabel := coverageStart.Format("Jan 2, 2006") + " to " + coverageEnd.Format("Jan 2, 2006")
		switch {
		case source.SourceID == storage.OwnerEventsCalendarSourceID:
			coverageLabel = "Events you add in Plan"
		case !source.ReadOnly:
			// The placements calendar has no range; it holds whatever is accepted.
			coverageLabel = "Times you accept in Plan"
		}
		dto := CalendarSourceDTO{
			SourceID:      source.SourceID,
			Label:         source.Label,
			Kind:          string(source.Kind),
			ReadOnly:      source.ReadOnly,
			Endpoint:      source.Endpoint,
			VisibleEvents: visibleBySource[source.SourceID],
			CoverageLabel: coverageLabel,
			CoverageStart: source.CoverageStartAt.UTC().Format(time.RFC3339),
			CoverageEnd:   source.CoverageEndAt.UTC().Format(time.RFC3339),
		}
		if subscription, found := subscribed[source.SourceID]; found {
			dto.Subscription = calendarSubscriptionDTO(subscription, now, span.location)
		}
		dto.Account = fromAccounts[source.SourceID]
		sourceByID[source.SourceID] = dto
		if source.ReadOnly && (span.start.UTC().Before(source.CoverageStartAt) || span.end.UTC().After(source.CoverageEndAt)) {
			warnings = append(warnings, source.Label+" does not cover this entire date range; refresh the source to extend it.")
		}
	}
	return sourceByID, warnings, nil
}

// calendarDays are the span's days, empty, with where each starts and where
// the last ends.
func calendarDays(span calendarSpan, now time.Time) ([]CalendarDayDTO, []time.Time) {
	today := now.In(span.location).Format("2006-01-02")
	dayDTOs := make([]CalendarDayDTO, span.days)
	dayStarts := make([]time.Time, span.days+1)
	for index := 0; index < span.days; index++ {
		dayStart := span.start.AddDate(0, 0, index)
		dayStarts[index] = dayStart
		dayDTOs[index] = CalendarDayDTO{
			CivilDate:   dayStart.Format("2006-01-02"),
			Label:       dayStart.Format("Mon, Jan 2"),
			IsToday:     dayStart.Format("2006-01-02") == today,
			Events:      []CalendarEventSegmentDTO{},
			Predictions: []CalendarBandSegmentDTO{},
		}
	}
	dayStarts[span.days] = span.end
	return dayDTOs, dayStarts
}

// addCalendarPredictions draws the forecast's sleep and waking windows on
// the days they cross.
func addCalendarPredictions(dayDTOs []CalendarDayDTO, dayStarts []time.Time, estimate localEstimateState, now time.Time, span calendarSpan) {
	waking := estimate.Estimate.PredictedWakingWindows
	// The stretch under way has no estimator window; draw it as the Home
	// outlook does, or today reads blank until the night's onset band.
	if stop, ok := outlook.CurrentWakingEnd(estimate.Estimate, now); ok {
		from, fromErr := domain.NewZonedInstant(now, span.zoneID)
		to, toErr := domain.NewZonedInstant(stop, span.zoneID)
		if fromErr == nil && toErr == nil {
			current := domain.AvailabilityWindow{
				ID:         domain.AvailabilityWindowID(string(estimate.Estimate.ID) + "_current_waking"),
				Kind:       domain.AvailabilityPredictedWake,
				Interval:   domain.TimeRange{Start: from, End: to},
				Confidence: estimate.Estimate.Confidence,
				EstimateID: estimate.Estimate.ID,
			}
			waking = append([]domain.AvailabilityWindow{current}, waking...)
		}
	}
	addCalendarBands(dayDTOs, dayStarts, estimate.Estimate.PredictedSleepWindows, "predicted_sleep", "Predicted sleep window", span.location)
	addCalendarBands(dayDTOs, dayStarts, waking, "predicted_wake", "Predicted waking window", span.location)
}

// addCalendarBands draws windows of one kind on the days they cross.
func addCalendarBands(dayDTOs []CalendarDayDTO, dayStarts []time.Time, windows []domain.AvailabilityWindow, kind, title string, location *time.Location) {
	for _, window := range windows {
		first, last := calendarOverlappingDayIndexes(dayStarts, window.Interval.Start.UTC, window.Interval.End.UTC, false)
		for index := first; index < last; index++ {
			if segment, include := calendarBandSegment(window, kind, title, dayStarts[index], dayStarts[index+1], location); include {
				dayDTOs[index].Predictions = append(dayDTOs[index].Predictions, segment)
			}
		}
	}
}

// sortCalendarDayEvents puts a day's all-day events first, then the rest by
// when they start.
func sortCalendarDayEvents(events []CalendarEventSegmentDTO) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].AllDay != events[j].AllDay {
			return events[i].AllDay
		}
		if events[i].StartMinute == events[j].StartMinute {
			return events[i].EventID < events[j].EventID
		}
		return events[i].StartMinute < events[j].StartMinute
	})
}

func parseCalendarFile(input CalendarFileInput, importedAt time.Time) (calendarcore.EventSet, error) {
	fileName := strings.TrimSpace(filepath.Base(input.FileName))
	if fileName == "" || fileName == "." || !strings.EqualFold(filepath.Ext(fileName), ".ics") {
		return calendarcore.EventSet{}, errors.New("select an .ics calendar file")
	}
	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		zoneID = localZoneID()
	}
	coverageStart, coverageEnd := calendarcore.CoverageAround(importedAt)
	return calendarcore.ParseICS([]byte(input.Contents), calendarcore.ParseOptions{
		SourceID:      stableCalendarID("calendar_source_ics", strings.ToLower(fileName)),
		SourceLabel:   fileName,
		Kind:          calendarcore.SourceICS,
		ImportedAt:    importedAt,
		CoverageStart: coverageStart,
		CoverageEnd:   coverageEnd,
		DefaultZoneID: zoneID,
	})
}

// calendarOverlappingDayIndexes returns the smallest output range that can
// overlap an interval. Work then scales with emitted day segments rather than
// every stored event multiplied by every visible day.
func calendarOverlappingDayIndexes(dayStarts []time.Time, start, end time.Time, point bool) (int, int) {
	dayCount := len(dayStarts) - 1
	if dayCount <= 0 {
		return 0, 0
	}
	start = start.UTC()
	end = end.UTC()
	first := sort.Search(dayCount, func(index int) bool {
		return dayStarts[index+1].UTC().After(start)
	})
	if first == dayCount {
		return 0, 0
	}
	if point {
		if start.Before(dayStarts[first].UTC()) {
			return 0, 0
		}
		return first, first + 1
	}
	last := sort.Search(dayCount, func(index int) bool {
		return !dayStarts[index].UTC().Before(end)
	})
	if first >= last {
		return 0, 0
	}
	return first, last
}

func (a *App) fetchCalDAVCalendar(ctx context.Context, input CalDAVInput, importedAt time.Time) (calendarcore.EventSet, string, error) {
	endpoint, parsedURL, err := sanitizeCalDAVEndpoint(input.Endpoint)
	if err != nil {
		return calendarcore.EventSet{}, "", err
	}
	if input.Password != "" && strings.TrimSpace(input.Username) == "" {
		return calendarcore.EventSet{}, "", errors.New("CalDAV password requires a username")
	}
	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		zoneID = localZoneID()
	}
	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = "CalDAV " + parsedURL.Hostname()
	}
	coverageStart, coverageEnd := calendarcore.CoverageAround(importedAt)
	body := calDAVReportBody(coverageStart, coverageEnd)
	request, err := http.NewRequestWithContext(ctx, "REPORT", endpoint, strings.NewReader(body))
	if err != nil {
		return calendarcore.EventSet{}, "", errors.New("build CalDAV request")
	}
	request.Header.Set("Depth", "1")
	request.Header.Set("Content-Type", "application/xml; charset=utf-8")
	request.Header.Set("Accept", "application/xml")
	if input.Username != "" {
		request.SetBasicAuth(input.Username, input.Password)
	}
	client := a.calendarHTTPClient
	if client == nil {
		client = newCalendarHTTPClient()
	}
	response, err := client.Do(request)
	if err != nil {
		return calendarcore.EventSet{}, "", errors.New("CalDAV request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMultiStatus && response.StatusCode != http.StatusOK {
		return calendarcore.EventSet{}, "", fmt.Errorf("CalDAV server returned HTTP %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, maxCalDAVResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return calendarcore.EventSet{}, "", errors.New("read CalDAV response")
	}
	if len(data) > maxCalDAVResponseBytes {
		return calendarcore.EventSet{}, "", fmt.Errorf("CalDAV response exceeds %d bytes", maxCalDAVResponseBytes)
	}
	documents, err := extractCalendarData(data)
	if err != nil {
		return calendarcore.EventSet{}, "", err
	}
	options := calendarcore.ParseOptions{
		SourceID:      stableCalendarID("calendar_source_caldav", endpoint),
		SourceLabel:   label,
		Kind:          calendarcore.SourceCalDAV,
		ImportedAt:    importedAt,
		CoverageStart: coverageStart,
		CoverageEnd:   coverageEnd,
		DefaultZoneID: zoneID,
	}
	set := calendarcore.EventSet{
		SchemaVersion: "v1",
		GeneratedAt:   importedAt.UTC(),
	}
	seen := make(map[string]calendarcore.Event)
	for _, document := range documents {
		parsed, err := calendarcore.ParseICS([]byte(document), options)
		if err != nil {
			return calendarcore.EventSet{}, "", fmt.Errorf("CalDAV calendar-data: %w", err)
		}
		if len(set.Sources) == 0 {
			set.Sources = parsed.Sources
		}
		for _, event := range parsed.Events {
			if existing, found := seen[event.EventID]; found {
				if !sameCalendarEvent(existing, event) {
					return calendarcore.EventSet{}, "", fmt.Errorf("CalDAV returned conflicting event %s", event.EventID)
				}
				continue
			}
			seen[event.EventID] = event
			set.Events = append(set.Events, event)
			if len(set.Events) > calendarcore.MaxMaterializedEvents {
				return calendarcore.EventSet{}, "", fmt.Errorf("CalDAV materializes more than %d events", calendarcore.MaxMaterializedEvents)
			}
		}
	}
	if len(set.Sources) == 0 {
		set.Sources = []calendarcore.Source{{
			SourceID:        options.SourceID,
			Label:           options.SourceLabel,
			Kind:            options.Kind,
			ReadOnly:        true,
			CoverageStartAt: options.CoverageStart.UTC(),
			CoverageEndAt:   options.CoverageEnd.UTC(),
			LastImportedAt:  options.ImportedAt.UTC(),
		}}
	}
	sort.Slice(set.Events, func(i, j int) bool {
		if set.Events[i].StartAt.Equal(set.Events[j].StartAt) {
			return set.Events[i].EventID < set.Events[j].EventID
		}
		return set.Events[i].StartAt.Before(set.Events[j].StartAt)
	})
	return set, endpoint, nil
}

func sanitizeCalDAVEndpoint(raw string) (string, *url.URL, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", nil, errors.New("CalDAV endpoint must be an absolute URL without credentials, query, or fragment")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && calendarLoopbackHost(parsed.Hostname())) {
		return "", nil, errors.New("CalDAV endpoint must use HTTPS except on loopback")
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), parsed, nil
}

func calendarLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sameURLOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

func calDAVReportBody(start, end time.Time) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">` +
		`<D:prop><D:getetag/><C:calendar-data/></D:prop>` +
		`<C:filter><C:comp-filter name="VCALENDAR"><C:comp-filter name="VEVENT">` +
		`<C:time-range start="` + start.UTC().Format("20060102T150405Z") + `" end="` + end.UTC().Format("20060102T150405Z") + `"/>` +
		`</C:comp-filter></C:comp-filter></C:filter></C:calendar-query>`
}

// extractCalendarData returns the calendar objects a calendar-query answered
// with. A multistatus without responses is an empty calendar, such as one made
// for ZeitBoard to write accepted times to.
func extractCalendarData(data []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var documents []string
	total, responses, multistatus := 0, 0, false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("CalDAV response is not valid XML")
		}
		start, ok := token.(xml.StartElement)
		switch {
		case !ok:
			continue
		case start.Name.Local == "multistatus":
			multistatus = true
			continue
		case start.Name.Local == "response":
			responses++
			continue
		case start.Name.Local != "calendar-data":
			continue
		}
		var contents string
		if err := decoder.DecodeElement(&contents, &start); err != nil {
			return nil, errors.New("CalDAV calendar-data is invalid")
		}
		total += len(contents)
		if total > calendarcore.MaxDocumentBytes {
			return nil, fmt.Errorf("CalDAV calendar-data exceeds %d bytes", calendarcore.MaxDocumentBytes)
		}
		if strings.TrimSpace(contents) != "" {
			documents = append(documents, contents)
		}
	}
	if !multistatus || (len(documents) == 0 && responses > 0) {
		return nil, errors.New("CalDAV response did not contain calendar-data")
	}
	return documents, nil
}

func sameCalendarEvent(left, right calendarcore.Event) bool {
	return left.EventID == right.EventID && left.SourceID == right.SourceID &&
		left.SourceRecordID == right.SourceRecordID && left.Title == right.Title &&
		left.StartAt.Equal(right.StartAt) && left.EndAt.Equal(right.EndAt) &&
		left.ZoneID == right.ZoneID && left.AllDay == right.AllDay && left.Busy == right.Busy &&
		left.Location == right.Location && left.Notes == right.Notes
}

func calendarImportDTO(set calendarcore.EventSet, imported bool) CalendarImportDTO {
	source := set.Sources[0]
	busyCount := 0
	allDayCount := 0
	previewCount := min(len(set.Events), maxCalendarPreviewEvents)
	preview := make([]CalendarImportEventDTO, 0, previewCount)
	for index, event := range set.Events {
		if event.Busy {
			busyCount++
		}
		if event.AllDay {
			allDayCount++
		}
		if index < previewCount {
			preview = append(preview, CalendarImportEventDTO{
				EventID:    event.EventID,
				Title:      event.Title,
				StartLabel: calendarTimeLabel(event.StartAt, event.ZoneID, event.AllDay),
				EndLabel:   calendarTimeLabel(event.EndAt, event.ZoneID, event.AllDay),
				AllDay:     event.AllDay,
				Busy:       event.Busy,
			})
		}
	}
	verb := "Previewed"
	if imported {
		verb = "Imported"
	}
	return CalendarImportDTO{
		SourceID:         source.SourceID,
		Label:            source.Label,
		Kind:             string(source.Kind),
		ReadOnly:         source.ReadOnly,
		Imported:         imported,
		EventCount:       len(set.Events),
		BusyCount:        busyCount,
		AllDayCount:      allDayCount,
		CoverageStartAt:  source.CoverageStartAt.UTC().Format(time.RFC3339),
		CoverageEndAt:    source.CoverageEndAt.UTC().Format(time.RFC3339),
		CoverageLabel:    source.CoverageStartAt.Local().Format("Jan 2, 2006") + " to " + source.CoverageEndAt.Local().Format("Jan 2, 2006"),
		PreviewTruncated: len(set.Events) > previewCount,
		Events:           preview,
		Message:          fmt.Sprintf("%s %d %s, %d busy.", verb, len(set.Events), plural(len(set.Events), "event", "events"), busyCount),
	}
}

func stableCalendarID(prefix, value string) string {
	digest := sha256.Sum256([]byte(value))
	return prefix + "_" + hex.EncodeToString(digest[:12])
}

func calendarTimeLabel(value time.Time, zoneID string, allDay bool) string {
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		location = time.UTC
	}
	if allDay {
		return value.In(location).Format("Jan 2, 2006")
	}
	return displayClock().Format(value.In(location), "Jan 2, 2006, 3:04 PM MST")
}

func calendarCivilDate(raw string, now time.Time, location *time.Location) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = now.In(location).Format("2006-01-02")
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		return time.Time{}, errors.New("calendar start date must use YYYY-MM-DD")
	}
	if parsed.Format("2006-01-02") != value {
		return time.Time{}, errors.New("calendar start date is not a valid civil date")
	}
	return parsed, nil
}

// segmentOwnership is "owner" for an event the owner added (ADR-0068), which
// the window offers to change and delete; it is kept as a fixed commitment.
func segmentOwnership(event calendarcore.Event) string {
	if event.SourceID == storage.OwnerEventsCalendarSourceID {
		return "owner"
	}
	return string(event.Ownership)
}

func calendarEventSegment(event calendarcore.Event, source CalendarSourceDTO, dayStart, dayEnd time.Time, location *time.Location) (CalendarEventSegmentDTO, bool) {
	point := event.StartAt.Equal(event.EndAt)
	if (!point && (!event.EndAt.After(dayStart.UTC()) || !event.StartAt.Before(dayEnd.UTC()))) ||
		(point && (event.StartAt.Before(dayStart.UTC()) || !event.StartAt.Before(dayEnd.UTC()))) {
		return CalendarEventSegmentDTO{}, false
	}
	startMinute, endMinute, before, after := calendarSegmentMinutes(event.StartAt, event.EndAt, dayStart, dayEnd, location, point)
	return CalendarEventSegmentDTO{
		SegmentID:       event.EventID + "_" + dayStart.Format("20060102"),
		EventID:         event.EventID,
		SourceID:        event.SourceID,
		SourceLabel:     source.Label,
		SourceKind:      source.Kind,
		Title:           event.Title,
		StartAt:         event.StartAt.UTC().Format(time.RFC3339),
		EndAt:           event.EndAt.UTC().Format(time.RFC3339),
		StartLabel:      calendarTimeLabel(event.StartAt, location.String(), event.AllDay),
		EndLabel:        calendarTimeLabel(event.EndAt, location.String(), event.AllDay),
		StartMinute:     startMinute,
		EndMinute:       endMinute,
		AllDay:          event.AllDay,
		PointInTime:     point,
		Busy:            event.Busy,
		Ownership:       segmentOwnership(event),
		ReadOnly:        source.ReadOnly,
		ContinuesBefore: before,
		ContinuesAfter:  after,
		Location:        event.Location,
		Notes:           event.Notes,
	}, true
}

func calendarBandSegment(window domain.AvailabilityWindow, kind, title string, dayStart, dayEnd time.Time, location *time.Location) (CalendarBandSegmentDTO, bool) {
	start := window.Interval.Start.UTC
	end := window.Interval.End.UTC
	if !end.After(dayStart.UTC()) || !start.Before(dayEnd.UTC()) {
		return CalendarBandSegmentDTO{}, false
	}
	startMinute, endMinute, before, after := calendarSegmentMinutes(start, end, dayStart, dayEnd, location, false)
	return CalendarBandSegmentDTO{
		SegmentID:       string(window.ID) + "_" + dayStart.Format("20060102"),
		Kind:            kind,
		Title:           title,
		StartAt:         start.UTC().Format(time.RFC3339),
		EndAt:           end.UTC().Format(time.RFC3339),
		StartLabel:      calendarTimeLabel(start, location.String(), false),
		EndLabel:        calendarTimeLabel(end, location.String(), false),
		StartMinute:     startMinute,
		EndMinute:       endMinute,
		Confidence:      string(window.Confidence.Level),
		ContinuesBefore: before,
		ContinuesAfter:  after,
	}, true
}

func calendarSegmentMinutes(start, end, dayStart, dayEnd time.Time, location *time.Location, point bool) (float64, float64, bool, bool) {
	continuesBefore := start.Before(dayStart.UTC())
	continuesAfter := end.After(dayEnd.UTC())
	startMinute := 0.0
	if !continuesBefore {
		startMinute = civilMinute(start.In(location))
	}
	endMinute := 1440.0
	if !continuesAfter {
		endMinute = civilMinute(end.In(location))
		if end.Equal(dayEnd.UTC()) {
			endMinute = 1440
		}
	}
	if point {
		endMinute = min(startMinute+15, 1440)
	}
	return startMinute, endMinute, continuesBefore, continuesAfter
}

func civilMinute(value time.Time) float64 {
	hour, minute, second := value.Clock()
	return float64(hour*60+minute) + float64(second)/60
}
