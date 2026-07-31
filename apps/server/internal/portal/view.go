package portal

import (
	"fmt"
	"time"
)

// Freshness thresholds from docs/portal-design.md section 1. Past StaleAfter
// the page says so; past UnavailableAfter it stops making a current-state
// claim entirely rather than presenting an old "awake now".
const (
	StaleAfter       = 6 * time.Hour
	UnavailableAfter = 24 * time.Hour
)

// Qualifier is required on every availability view. The numbers come from the
// measured real-history backtest in ADR-0022 (median 1.71 h, P90 5.41 h), not
// from a marketing estimate.
const Qualifier = "This is an estimate from recent patterns. It is often off by about 2 hours, and sometimes more."

// NoticeNotMedical keeps the public surface's purpose unambiguous.
const NoticeNotMedical = "This page shows scheduling availability only. It is not medical information and does not describe any health condition."

// AvailabilityView is the rendered form of a Snapshot. Every string here is
// derived from the allowlisted snapshot; none of it reaches back into private
// data.
type AvailabilityView struct {
	Headline    string
	Detail      string
	Freshness   string
	Qualifier   string
	Notice      string
	Windows     []WindowView
	LikelyAwake bool
	Stale       bool
	Unavailable bool
	ZoneLabel   string

	// ZoneIDInput is the raw IANA zone the request form submits, so a
	// visitor's chosen times are interpreted in the same zone the windows are
	// displayed in rather than silently in UTC.
	ZoneIDInput string
}

type WindowView struct {
	DayLabel   string
	RangeLabel string
	Current    bool
}

// BuildView classifies a snapshot for display. It is the single place the
// stale and unavailable rules are implemented, so the no-script page and any
// later live layer cannot drift apart.
func BuildView(snapshot Snapshot, now time.Time) AvailabilityView {
	view := AvailabilityView{
		Qualifier:   Qualifier,
		Notice:      NoticeNotMedical,
		ZoneIDInput: "UTC",
	}
	if len(snapshot.Windows) > 0 && snapshot.Windows[0].ZoneID != "" {
		view.ZoneIDInput = snapshot.Windows[0].ZoneID
	}
	now = now.UTC()

	if snapshot.GeneratedAt.IsZero() {
		view.Unavailable = true
		view.Headline = "Availability is not being shared right now"
		view.Detail = "This link is working, but there is no availability to show yet."
		view.Freshness = "No update has been published yet."
		return view
	}

	age := now.Sub(snapshot.GeneratedAt)
	view.Freshness = describeFreshness(age)

	switch {
	case snapshot.Status == StatusRefused:
		view.Unavailable = true
		view.Headline = "Availability cannot be estimated right now"
		view.Detail = "There is not enough recent, consistent history to estimate a waking pattern. You can still reach out and agree on a time directly."
		return view
	case snapshot.Status == StatusInsufficientData || len(snapshot.Windows) == 0:
		view.Unavailable = true
		view.Headline = "Availability is not being shared right now"
		view.Detail = "This link does not currently show waking windows."
		return view
	case age >= UnavailableAfter:
		view.Unavailable = true
		view.Headline = "This availability is out of date"
		view.Detail = "The last update is more than a day old, so it is not shown. An out-of-date estimate is worse than none."
		return view
	}

	view.Stale = age >= StaleAfter
	location := windowLocation(snapshot.Windows[0].ZoneID)
	view.ZoneLabel = zoneLabel(snapshot.Windows[0].ZoneID, now.In(location))

	upcoming := make([]WindowView, 0, len(snapshot.Windows))
	for _, window := range snapshot.Windows {
		if !window.EndAt.After(now) {
			continue
		}
		current := !window.StartAt.After(now) && window.EndAt.After(now)
		if current {
			view.LikelyAwake = true
		}
		upcoming = append(upcoming, WindowView{
			DayLabel:   describeDay(window.StartAt, now, location),
			RangeLabel: describeRange(window.StartAt, window.EndAt, location),
			Current:    current,
		})
	}
	view.Windows = upcoming

	switch {
	case view.LikelyAwake:
		view.Headline = "Likely awake right now"
		view.Detail = "Based on recent patterns, this is probably a reachable time."
	case len(upcoming) > 0:
		view.Headline = "Likely not awake right now"
		view.Detail = fmt.Sprintf("The next likely waking window starts %s.", lowerFirst(upcoming[0].DayLabel)+" at "+startOnly(snapshot.Windows, now, location))
	default:
		view.Unavailable = true
		view.Headline = "Availability is not being shared right now"
		view.Detail = "There are no upcoming waking windows to show."
	}

	if view.Stale {
		view.Detail += " This estimate has not refreshed recently, so treat it with extra caution."
	}
	return view
}

func windowLocation(zoneID string) *time.Location {
	if zoneID == "" {
		return time.UTC
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return time.UTC
	}
	return location
}

func zoneLabel(zoneID string, local time.Time) string {
	abbreviation := local.Format("MST")
	if zoneID == "" {
		return abbreviation
	}
	return fmt.Sprintf("%s (%s)", abbreviation, zoneID)
}

func startOnly(windows []Window, now time.Time, location *time.Location) string {
	for _, window := range windows {
		if window.EndAt.After(now) && window.StartAt.After(now) {
			return formatClock(window.StartAt.In(location))
		}
	}
	return "an unknown time"
}

func describeDay(value time.Time, now time.Time, location *time.Location) string {
	local := value.In(location)
	today := now.In(location)
	switch daysBetween(today, local) {
	case 0:
		return "Today"
	case 1:
		return "Tomorrow"
	default:
		return local.Format("Mon, Jan 2")
	}
}

func daysBetween(from, to time.Time) int {
	fromDay := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	toDay := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location())
	return int(toDay.Sub(fromDay).Hours() / 24)
}

func describeRange(start, end time.Time, location *time.Location) string {
	localStart := start.In(location)
	localEnd := end.In(location)
	if daysBetween(localStart, localEnd) == 0 {
		return fmt.Sprintf("%s to %s", formatClock(localStart), formatClock(localEnd))
	}
	return fmt.Sprintf("%s to %s on %s", formatClock(localStart), formatClock(localEnd), localEnd.Format("Mon, Jan 2"))
}

func formatClock(value time.Time) string {
	return value.Format("3:04 PM")
}

func describeFreshness(age time.Duration) string {
	switch {
	case age < 0:
		return "Updated just now."
	case age < 2*time.Minute:
		return "Updated just now."
	case age < time.Hour:
		return fmt.Sprintf("Updated %d minutes ago.", int(age.Minutes()))
	case age < 2*time.Hour:
		return "Updated about an hour ago."
	case age < UnavailableAfter:
		return fmt.Sprintf("Updated about %d hours ago.", int(age.Hours()))
	default:
		return "Last updated more than a day ago."
	}
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	if value[0] >= 'A' && value[0] <= 'Z' {
		return string(value[0]+('a'-'A')) + value[1:]
	}
	return value
}
