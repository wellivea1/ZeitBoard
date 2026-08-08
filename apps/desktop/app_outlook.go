package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"non24.app/core/domain"
	"non24.app/core/freshness"
	"non24.app/core/outlook"
)

// The operational view (ADR-0034, P7 slice 8). It answers the question the
// product exists for — over the next two or three days, when is this person
// awake and what can they actually do in that time — for someone whose waking
// hours move about an hour every day.
//
// It is computed from local records even when backend sync is on. The synced
// projection is a v1 contract carrying rendered strings, not the estimate's
// predicted windows, so the server cannot supply the envelopes this view is
// built from. Sync pushes and pulls the underlying records, so the local store
// holds the same evidence; this is a question of which contract to read, not of
// which device knows more.

type OutlookSegmentDTO struct {
	Presence      string  `json:"presence"`
	Observed      bool    `json:"observed"`
	RangeLabel    string  `json:"rangeLabel"`
	DayLabel      string  `json:"dayLabel"`
	DurationLabel string  `json:"durationLabel"`
	OffsetHours   float64 `json:"offsetHours"`
	DurationHours float64 `json:"durationHours"`
}

type OutlookDayMarkDTO struct {
	Label       string  `json:"label"`
	OffsetHours float64 `json:"offsetHours"`
}

type OutlookOfficeDTO struct {
	DayLabel   string `json:"dayLabel"`
	HoursLabel string `json:"hoursLabel"`

	// Status is reachable, partial, or unreachable. Partial means the only
	// overlap sits on a boundary the model has not pinned down.
	Status         string  `json:"status"`
	ReachableLabel string  `json:"reachableLabel,omitempty"`
	Detail         string  `json:"detail"`
	OffsetHours    float64 `json:"offsetHours"`
	DurationHours  float64 `json:"durationHours"`
}

type OutlookCommitmentDTO struct {
	Title         string `json:"title"`
	WhenLabel     string `json:"whenLabel"`
	Conflict      string `json:"conflict"`
	ConflictLabel string `json:"conflictLabel,omitempty"`
}

type OutlookOpportunityDTO struct {
	TaskID        string `json:"taskId"`
	Title         string `json:"title"`
	WhenLabel     string `json:"whenLabel,omitempty"`
	UnplacedLabel string `json:"unplacedLabel,omitempty"`
	NeedsApproval bool   `json:"needsApproval"`
}

type OutlookDTO struct {
	Status  string      `json:"status"`
	Refusal *RefusalDTO `json:"refusal,omitempty"`

	// Freshness is present in every status, including withheld — there it is
	// the reason.
	Freshness FreshnessDTO `json:"freshness"`

	HorizonLabel   string              `json:"horizonLabel"`
	HorizonHours   float64             `json:"horizonHours"`
	Days           []OutlookDayMarkDTO `json:"days"`
	Segments       []OutlookSegmentDTO `json:"segments"`
	NextSleepLabel string              `json:"nextSleepLabel,omitempty"`
	NextWakeLabel  string              `json:"nextWakeLabel,omitempty"`

	OfficeHoursLabel string                  `json:"officeHoursLabel"`
	OfficeWindows    []OutlookOfficeDTO      `json:"officeWindows"`
	Commitments      []OutlookCommitmentDTO  `json:"commitments"`
	Opportunities    []OutlookOpportunityDTO `json:"opportunities"`

	AwakeLabel     string `json:"awakeLabel"`
	UncertainLabel string `json:"uncertainLabel"`

	// WithheldMessage is what to show instead of a timeline.
	WithheldMessage string `json:"withheldMessage,omitempty"`

	Disclaimer string `json:"disclaimer"`
}

// GetOutlook builds the 48–72 hour operational view.
func (a *App) GetOutlook() (OutlookDTO, error) {
	ctx := a.applicationContext()
	now := a.currentTime().UTC().Truncate(time.Minute)

	state, err := a.localEstimate(ctx, now)
	if err != nil {
		return OutlookDTO{}, err
	}

	zoneID := state.Estimate.AsOf.ZoneID
	if zoneID == "" {
		zoneID = time.Now().Location().String()
	}

	in := outlook.Input{
		Now:         now,
		Estimate:    state.Estimate,
		OfficeHours: outlook.DefaultOfficeHours(zoneID),
	}
	if state.Status != "estimated" {
		in.EstimateError = outlookRefusalError(state)
	}

	var latest domain.SleepSession
	if session, ok := latestPrincipalSession(state.Sessions); ok {
		latest = session
	}
	var nextSleep domain.TimeRange
	if len(state.Estimate.PredictedSleepWindows) > 0 {
		nextSleep = state.Estimate.PredictedSleepWindows[0].Interval
	}
	assessment := freshness.Default().Assess(desktopFreshnessInputs(state, latest, nextSleep, now))
	in.Freshness = assessment

	// Recorded sleep reaching into the horizon overrides the forecast, and the
	// newest wake anchors after-wake task constraints.
	horizonEnd := now.Add(outlook.DefaultHorizon)
	for _, session := range state.Sessions {
		for _, interval := range session.Intervals {
			if interval.Interval.End.UTC.After(now) && interval.Interval.Start.UTC.Before(horizonEnd) {
				in.RecordedSleep = append(in.RecordedSleep, interval.Interval)
			}
		}
	}
	if len(latest.Intervals) > 0 {
		in.WakeAnchor = &domain.WakeAnchor{
			ID:         "latest-wake",
			At:         latest.Intervals[0].Interval.End,
			Confidence: state.Estimate.Confidence,
		}
	}

	titles := map[domain.CalendarEventID]string{}
	taskTitles := map[domain.FlexibleTaskID]string{}
	if store, storeErr := a.requireStore(); storeErr == nil {
		events, _, eventsErr := store.BusyDomainEvents(ctx, now, horizonEnd, zoneID)
		if eventsErr != nil {
			return OutlookDTO{}, eventsErr
		}
		in.Events = events
		for _, event := range events {
			titles[event.ID] = event.Title
		}
		tasks, _, tasksErr := store.OpenDomainTasks(ctx, zoneID)
		if tasksErr != nil {
			return OutlookDTO{}, tasksErr
		}
		in.Tasks = tasks
		for _, task := range tasks {
			taskTitles[task.ID] = task.Title
		}
	}

	view, err := outlook.Build(in)
	if err != nil {
		return OutlookDTO{}, err
	}
	return outlookDTO(view, titles, taskTitles, zoneID), nil
}

// outlookRefusalError turns the local estimate's state into the typed refusal
// the view reports. An empty history and a rejected one are different messages.
func outlookRefusalError(state localEstimateState) error {
	if state.Refusal != nil {
		return state.Refusal
	}
	message := state.Message
	if message == "" {
		message = "There is no rhythm estimate to project forward yet."
	}
	return errors.New(message)
}

func outlookDTO(
	view outlook.Outlook,
	eventTitles map[domain.CalendarEventID]string,
	taskTitles map[domain.FlexibleTaskID]string,
	zoneID string,
) OutlookDTO {
	location := loadLocationOrUTC(zoneID)
	start := view.Horizon.Start.UTC
	horizonHours := view.Horizon.End.UTC.Sub(start).Hours()

	dto := OutlookDTO{
		Status:           string(view.Status),
		Freshness:        freshnessDTO(view.Freshness),
		HorizonLabel:     fmt.Sprintf("Next %d hours", int(horizonHours+0.5)),
		HorizonHours:     horizonHours,
		Days:             []OutlookDayMarkDTO{},
		Segments:         []OutlookSegmentDTO{},
		OfficeWindows:    []OutlookOfficeDTO{},
		Commitments:      []OutlookCommitmentDTO{},
		Opportunities:    []OutlookOpportunityDTO{},
		OfficeHoursLabel: "Typical office hours, Monday to Friday 9:00 AM to 5:00 PM",
		AwakeLabel:       formatDuration(view.AwakeFor),
		UncertainLabel:   formatDuration(view.UncertainFor),
		Disclaimer:       disclaimer,
	}
	if view.Refusal != nil {
		dto.Refusal = &RefusalDTO{Code: view.Refusal.Code, Message: view.Refusal.Message}
	}
	if view.Status == outlook.StatusWithheld {
		dto.WithheldMessage = view.Freshness.Explanation
		return dto
	}
	if view.Status != outlook.StatusAvailable {
		return dto
	}

	for _, segment := range view.Segments {
		dto.Segments = append(dto.Segments, OutlookSegmentDTO{
			Presence:      string(segment.Presence),
			Observed:      segment.Observed,
			RangeLabel:    clockRange(segment.Interval, location),
			DayLabel:      segment.Interval.Start.UTC.In(location).Format("Mon"),
			DurationLabel: formatDuration(segment.Duration()),
			OffsetHours:   segment.Interval.Start.UTC.Sub(start).Hours(),
			DurationHours: segment.Duration().Hours(),
		})
	}
	dto.Days = dayMarks(start, view.Horizon.End.UTC, location)

	if view.NextSleep != nil {
		dto.NextSleepLabel = civilRange(*view.NextSleep, location)
	}
	if view.NextWake != nil {
		dto.NextWakeLabel = civilRange(*view.NextWake, location)
	}

	for _, window := range view.OfficeWindows {
		dto.OfficeWindows = append(dto.OfficeWindows, officeDTO(window, location, start))
	}
	for _, commitment := range view.Commitments {
		title := eventTitles[commitment.EventID]
		if strings.TrimSpace(title) == "" {
			title = "Untitled event"
		}
		dto.Commitments = append(dto.Commitments, OutlookCommitmentDTO{
			Title:         title,
			WhenLabel:     civilRange(commitment.Interval, location),
			Conflict:      string(commitment.Conflict),
			ConflictLabel: conflictLabel(commitment.Conflict),
		})
	}
	for _, opportunity := range view.Opportunities {
		entry := OutlookOpportunityDTO{
			TaskID:        string(opportunity.TaskID),
			Title:         taskTitles[opportunity.TaskID],
			NeedsApproval: opportunity.NeedsApproval,
		}
		if opportunity.Window != nil {
			entry.WhenLabel = civilRange(*opportunity.Window, location)
		} else {
			entry.UnplacedLabel = unplacedLabel(string(opportunity.Unplaced))
		}
		dto.Opportunities = append(dto.Opportunities, entry)
	}
	return dto
}

func officeDTO(window outlook.OfficeWindow, location *time.Location, start time.Time) OutlookOfficeDTO {
	dto := OutlookOfficeDTO{
		DayLabel:      window.Interval.Start.UTC.In(location).Format("Mon, Jan 2"),
		HoursLabel:    clockRange(window.Interval, location),
		OffsetHours:   window.Interval.Start.UTC.Sub(start).Hours(),
		DurationHours: window.Interval.End.UTC.Sub(window.Interval.Start.UTC).Hours(),
	}
	switch {
	case window.ReachableFor > 0:
		dto.Status = "reachable"
		dto.ReachableLabel = clockRange(window.Reachable[0], location)
		if len(window.Reachable) > 1 {
			dto.ReachableLabel += fmt.Sprintf(" and %d more", len(window.Reachable)-1)
		}
		dto.Detail = fmt.Sprintf("Predicted awake for %s of this window.", formatDuration(window.ReachableFor))
	case window.PossibleFor > 0:
		// Deliberately not counted as reachable. The overlap sits on a boundary
		// the model has not pinned down, and a plan made on it is a plan made on
		// arithmetic.
		dto.Status = "partial"
		dto.Detail = fmt.Sprintf(
			"Possibly awake for up to %s, but this falls where the sleep boundary is uncertain.",
			formatDuration(window.PossibleFor))
	default:
		dto.Status = "unreachable"
		dto.Detail = "Predicted asleep for all of this window."
	}
	return dto
}

func conflictLabel(kind outlook.ConflictKind) string {
	switch kind {
	case outlook.ConflictInsidePredictedSleep:
		return "Falls entirely inside predicted sleep"
	case outlook.ConflictOverlapsPredictedSleep:
		return "Overlaps predicted sleep"
	case outlook.ConflictNearBoundary:
		return "Sits where the sleep boundary is uncertain"
	default:
		return ""
	}
}

func unplacedLabel(reason string) string {
	switch reason {
	case "no_available_interval":
		return "No window long enough in the next three days"
	case "outside_forecast_horizon":
		return "Its deadline is past the forecast"
	case "estimate_unavailable":
		return "No usable estimate"
	case "invalid_constraints":
		return "Its constraints cannot be satisfied"
	default:
		return "Not placed"
	}
}

// dayMarks are civil midnights inside the horizon, for the timeline's day
// separators. Computed in the location so a daylight-saving change moves the
// mark with the civil day rather than sliding it an hour.
func dayMarks(start, end time.Time, location *time.Location) []OutlookDayMarkDTO {
	marks := make([]OutlookDayMarkDTO, 0, 4)
	local := start.In(location)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	for {
		day = day.AddDate(0, 0, 1)
		at := day.UTC()
		if !at.Before(end) {
			break
		}
		marks = append(marks, OutlookDayMarkDTO{
			Label:       day.Format("Mon, Jan 2"),
			OffsetHours: at.Sub(start).Hours(),
		})
	}
	return marks
}

func loadLocationOrUTC(zoneID string) *time.Location {
	if zoneID == "" {
		return time.UTC
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return time.UTC
	}
	return location
}

// clockRange renders times of day only, for a range the reader already knows
// the day of.
func clockRange(interval domain.TimeRange, location *time.Location) string {
	start := interval.Start.UTC.In(location)
	end := interval.End.UTC.In(location)
	if sameCivilDay(start, end) {
		return start.Format("3:04 PM") + " to " + end.Format("3:04 PM")
	}
	return start.Format("3:04 PM") + " to " + end.Format("3:04 PM on Mon, Jan 2")
}

// civilRange includes the day, for ranges that stand alone.
func civilRange(interval domain.TimeRange, location *time.Location) string {
	start := interval.Start.UTC.In(location)
	end := interval.End.UTC.In(location)
	if sameCivilDay(start, end) {
		return start.Format("Mon, Jan 2, 3:04 PM") + " to " + end.Format("3:04 PM")
	}
	return start.Format("Mon, Jan 2, 3:04 PM") + " to " + end.Format("Mon, Jan 2, 3:04 PM")
}

func sameCivilDay(left, right time.Time) bool {
	leftYear, leftMonth, leftDay := left.Date()
	rightYear, rightMonth, rightDay := right.Date()
	return leftYear == rightYear && leftMonth == rightMonth && leftDay == rightDay
}
