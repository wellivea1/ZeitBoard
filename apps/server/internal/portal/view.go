package portal

import (
	"fmt"
	"math"
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

	// Figure draws the same windows as a picture: a row per day from local
	// midnight. It adds nothing the window list does not already say.
	Figure []FigureDay
}

// FigureWidth is the figure's own coordinate width. Positions are computed
// here on that scale, so the template writes numbers into SVG attributes and
// never a style, which the portal's content-security policy forbids.
const FigureWidth = 240.0

const figureDays = 3

// FigureBeyond reports whether the figure shades any time past the estimate,
// so the legend explains the shading only when there is some.
func (v AvailabilityView) FigureBeyond() bool {
	for _, day := range v.Figure {
		if day.HasBeyond {
			return true
		}
	}
	return false
}

// FigureDay is one row of the figure: a civil day in the display zone.
type FigureDay struct {
	Label string
	Bands []FigureBand
	// Now is where the present falls on today's row.
	HasNow bool
	Now    float64
	// Beyond is where the estimate stops on this row, if it stops here or
	// earlier; the rest of the row is shaded rather than read as empty.
	HasBeyond   bool
	Beyond      float64
	BeyondWidth float64
}

// FigureBand is one likely waking window, or the part of it on this day.
type FigureBand struct {
	X     float64
	Width float64
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
		// A window already in progress is shown from now, so the page never
		// implies knowledge about a past the visitor cannot use. This runs at
		// render rather than at materialization, where it was only correct for
		// the instant the snapshot happened to be written.
		displayStart := window.StartAt
		if displayStart.Before(now) {
			displayStart = now
		}
		upcoming = append(upcoming, WindowView{
			DayLabel:   describeDay(displayStart, now, location),
			RangeLabel: describeRange(displayStart, window.EndAt, location),
			Current:    current,
		})
	}
	view.Windows = upcoming
	view.Figure = buildFigure(snapshot.Windows, snapshot.HorizonEnd, now, location)

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

// nextChange is the next instant the page's claim changes with no new
// estimate: a window opening or closing, or the estimate turning stale or
// unavailable. An open page refreshes itself then.
func nextChange(snapshot Snapshot, now time.Time) time.Time {
	var next time.Time
	consider := func(at time.Time) {
		if at.After(now) && (next.IsZero() || at.Before(next)) {
			next = at
		}
	}
	for _, window := range snapshot.Windows {
		consider(window.StartAt)
		consider(window.EndAt)
	}
	if !snapshot.GeneratedAt.IsZero() {
		consider(snapshot.GeneratedAt.Add(StaleAfter))
		consider(snapshot.GeneratedAt.Add(UnavailableAfter))
	}
	return next
}

// buildFigure lays the windows on three civil days from local midnight. A
// window is drawn from now at the earliest, like the list: the page never
// implies knowledge about a past the visitor cannot use. Days that are 23 or
// 25 hours long across a clock change keep their true proportions.
func buildFigure(windows []Window, horizon, now time.Time, location *time.Location) []FigureDay {
	local := now.In(location)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	position := func(at, dayStart time.Time, length time.Duration) float64 {
		return math.Round(float64(at.Sub(dayStart))/float64(length)*FigureWidth*100) / 100
	}
	days := make([]FigureDay, 0, figureDays)
	for index := 0; index < figureDays; index++ {
		start := midnight.AddDate(0, 0, index)
		end := midnight.AddDate(0, 0, index+1)
		length := end.Sub(start)
		day := FigureDay{Label: describeDay(start, now, location)}
		for _, window := range windows {
			from := latest(window.StartAt, now, start)
			to := window.EndAt
			if to.After(end) {
				to = end
			}
			if !to.After(from) {
				continue
			}
			x := position(from, start, length)
			day.Bands = append(day.Bands, FigureBand{X: x, Width: math.Max(0.5, position(to, start, length)-x)})
		}
		if index == 0 {
			day.HasNow, day.Now = true, position(now, start, length)
		}
		if !horizon.IsZero() && horizon.Before(end) {
			day.HasBeyond = true
			day.Beyond = math.Max(0, position(horizon, start, length))
			day.BeyondWidth = FigureWidth - day.Beyond
		}
		days = append(days, day)
	}
	return days
}

func latest(values ...time.Time) time.Time {
	result := values[0]
	for _, value := range values[1:] {
		if value.After(result) {
			result = value
		}
	}
	return result
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
