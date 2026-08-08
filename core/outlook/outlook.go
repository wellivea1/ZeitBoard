// Package outlook answers the question the product exists for: over the next
// two or three days, when will this person be awake, and what can they actually
// do in that time.
//
// For someone whose day is 24 h 50 m long, that is not a question a calendar
// answers. The waking hours move about an hour every day, so "can I ring the
// pharmacy this week" has a real answer that changes daily and that nobody can
// work out in their head while tired.
//
// # Envelopes, not a timetable
//
// The estimator does not predict a sleep onset; it predicts a window that
// widens with forecast distance. Its sleep and waking windows deliberately
// overlap: the sleep envelope runs from the earliest plausible onset to the
// latest plausible wake, and the waking envelope from the earliest plausible
// wake to the latest plausible next onset. An instant inside both is one where
// the model genuinely does not know.
//
// Collapsing that into a two-state timetable would be the single most
// misleading thing this package could do — the measured P90 onset error is
// 5.41 h (ADR-0022), so a confident-looking boundary would be wrong by hours
// several times a month. The timeline therefore has three states, and the
// uncertain one is the honest answer rather than a rendering artefact.
//
// # Nothing here is medical
//
// Everything is derived from recorded sleep-wake times and civil clock
// arithmetic. There is no circadian phase claim, no DLMO, no fitness-to-drive
// or fitness-to-work judgement, and no advice about what to do with any of it.
package outlook

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/freshness"
	"non24.app/core/scheduling"
)

// Horizon bounds. The phase goal asks for 48–72 hours; below two days the view
// stops answering "which day should I do this on", and beyond three the
// forecast bounds are wide enough that the answer is mostly "it depends".
const (
	MinimumHorizon = 48 * time.Hour
	DefaultHorizon = 72 * time.Hour
	MaximumHorizon = 96 * time.Hour
)

// Status is the top-level outcome. A view that cannot be computed says which
// kind of nothing it has, because "no estimate yet" and "the records are too
// old to place you in your own cycle" need different responses from the reader.
type Status string

const (
	StatusAvailable Status = "available"

	// StatusRefused: the estimator declined, typically for want of history.
	StatusRefused Status = "refused"

	// StatusWithheld: an estimate exists but the shared freshness policy will
	// not support a current-state claim. A forecast is anchored to where the
	// person is *now* in their cycle; without that anchor the next 72 hours are
	// not a plan, they are a shape.
	StatusWithheld Status = "withheld"
)

// Presence is what can be said about one stretch of time.
type Presence string

const (
	// PresenceAwake: inside a predicted waking envelope and outside every
	// sleep envelope. The usable time.
	PresenceAwake Presence = "awake"

	// PresenceAsleep: inside a sleep envelope and outside every waking one.
	PresenceAsleep Presence = "asleep"

	// PresenceUncertain: inside both. The boundary sits somewhere in here and
	// the model will not say where.
	PresenceUncertain Presence = "uncertain"

	// PresenceUnknown: past the end of the forecast.
	PresenceUnknown Presence = "unknown"
)

// Segment is one stretch of the timeline. Segments are contiguous, ordered, and
// cover the whole horizon with no gaps.
type Segment struct {
	Presence Presence         `json:"presence"`
	Interval domain.TimeRange `json:"interval"`

	// Observed marks a segment that comes from a recorded sleep episode rather
	// than a forecast. A record beats a prediction, and the reader should be
	// able to tell which they are looking at.
	Observed bool `json:"observed"`
}

// Duration is how long the segment lasts.
func (s Segment) Duration() time.Duration {
	return s.Interval.End.UTC.Sub(s.Interval.Start.UTC)
}

// OfficeHours is when the rest of the world is reachable. It is civil-clock
// arithmetic, not a claim about the user.
type OfficeHours struct {
	// StartLocal and EndLocal are "HH:MM" in ZoneID.
	StartLocal string
	EndLocal   string

	// Days are the weekdays the hours apply to.
	Days []time.Weekday

	ZoneID string
}

// DefaultOfficeHours is the ordinary Monday-to-Friday working day, matching the
// scheduler's own business-hours default so the two cannot disagree about what
// "business hours" means.
func DefaultOfficeHours(zoneID string) OfficeHours {
	return OfficeHours{
		StartLocal: "09:00",
		EndLocal:   "17:00",
		Days: []time.Weekday{
			time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday,
		},
		ZoneID: zoneID,
	}
}

// OfficeWindow is one stretch of office-open time inside the horizon, with how
// much of it the person is predicted to be awake for.
//
// Reachable and Possible are reported separately and never added together. A
// window that is only "possible" depends on a boundary the model has not pinned
// down, and treating it as usable is how someone ends up asleep through the one
// call they needed to make this week.
type OfficeWindow struct {
	Interval domain.TimeRange `json:"interval"`

	Reachable []domain.TimeRange `json:"reachable"`
	Possible  []domain.TimeRange `json:"possible"`

	ReachableFor time.Duration `json:"reachableFor"`
	PossibleFor  time.Duration `json:"possibleFor"`
}

// Usable reports whether any of this window is confidently awake.
func (w OfficeWindow) Usable() bool { return w.ReachableFor > 0 }

// ConflictKind describes how a fixed commitment sits against the forecast.
type ConflictKind string

const (
	ConflictNone ConflictKind = "none"

	// ConflictInsidePredictedSleep: the whole commitment falls where sleep is
	// predicted. For a non-24 rhythm this is the ordinary case for anything
	// booked more than a few days out, and it is the single most useful thing
	// this view can point at.
	ConflictInsidePredictedSleep ConflictKind = "inside_predicted_sleep"

	// ConflictOverlapsPredictedSleep: part of it does.
	ConflictOverlapsPredictedSleep ConflictKind = "overlaps_predicted_sleep"

	// ConflictNearBoundary: it does not overlap predicted sleep, but it does
	// overlap a stretch where the boundary is uncertain.
	ConflictNearBoundary ConflictKind = "near_uncertain_boundary"
)

// Commitment is a fixed calendar event inside the horizon.
//
// It carries an id and no text. Imported event titles are private and stay in
// the local trust zone (ADR-0023); a caller that may legitimately render them
// joins them back by id, and one that may not cannot obtain them by accident.
type Commitment struct {
	EventID  domain.CalendarEventID `json:"eventId"`
	Interval domain.TimeRange       `json:"interval"`
	Conflict ConflictKind           `json:"conflict"`
}

// Opportunity is a task the scheduler can place inside the horizon.
type Opportunity struct {
	TaskID domain.FlexibleTaskID `json:"taskId"`

	// Window is set when the task could be placed; Unplaced says why not.
	Window   *domain.TimeRange         `json:"window,omitempty"`
	Unplaced scheduling.UnplacedReason `json:"unplaced,omitempty"`

	// NeedsApproval is always true in practice and is carried anyway, because
	// nothing in this package may be applied without a person deciding.
	NeedsApproval bool `json:"needsApproval"`
}

// Refusal mirrors the estimator's typed refusal.
type Refusal struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Outlook is the whole view.
type Outlook struct {
	Status  Status   `json:"status"`
	Refusal *Refusal `json:"refusal,omitempty"`

	// Freshness is always populated, including when the status is withheld —
	// it is the explanation for the withholding.
	Freshness freshness.Assessment `json:"freshness"`

	Horizon  domain.TimeRange `json:"horizon"`
	Segments []Segment        `json:"segments"`

	// NextSleep and NextWake are the next envelope of each kind that starts
	// after now, as ranges rather than instants. Nil when none falls inside the
	// horizon.
	NextSleep *domain.TimeRange `json:"nextSleep,omitempty"`
	NextWake  *domain.TimeRange `json:"nextWake,omitempty"`

	OfficeWindows []OfficeWindow `json:"officeWindows"`
	Commitments   []Commitment   `json:"commitments"`
	Opportunities []Opportunity  `json:"opportunities"`

	// AwakeFor and UncertainFor total the timeline. The pair is the honest
	// summary of a forecast: how much time is usable, and how much the model
	// declines to call.
	AwakeFor     time.Duration `json:"awakeFor"`
	UncertainFor time.Duration `json:"uncertainFor"`
}

// Input is everything the view is computed from. It is a value: this package
// reads no store, no clock, and no configuration.
type Input struct {
	Now     time.Time
	Horizon time.Duration

	// Estimate is the phase estimate whose predicted windows drive the
	// timeline. Its zero value means no estimate is available.
	Estimate domain.PhaseEstimate

	// EstimateError is the estimator's own refusal, if it produced one.
	EstimateError error

	// Freshness is the shared policy's verdict on the evidence behind the
	// estimate. A withheld verdict suppresses the whole view.
	Freshness freshness.Assessment

	// RecordedSleep are stored sleep intervals. Any part of one that reaches
	// into the horizon overrides the forecast: a record beats a prediction.
	RecordedSleep []domain.TimeRange

	Events []domain.CalendarEvent
	Tasks  []domain.FlexibleTask

	// WakeAnchor is the newest recorded wake, used for after-wake task
	// constraints. Optional.
	WakeAnchor *domain.WakeAnchor

	OfficeHours OfficeHours

	// MaximumOpportunities caps how many tasks are placed. Zero means the
	// default; a screen that lists everything is a screen nobody reads.
	MaximumOpportunities int
}

// DefaultMaximumOpportunities bounds the task list.
const DefaultMaximumOpportunities = 5

// Build computes the view. It is pure.
func Build(in Input) (Outlook, error) {
	now := in.Now.UTC()
	if now.IsZero() {
		return Outlook{}, errors.New("outlook requires the current time")
	}
	horizon := in.Horizon
	switch {
	case horizon <= 0:
		horizon = DefaultHorizon
	case horizon < MinimumHorizon:
		horizon = MinimumHorizon
	case horizon > MaximumHorizon:
		horizon = MaximumHorizon
	}

	zoneID := in.OfficeHours.ZoneID
	if zoneID == "" {
		zoneID = in.Estimate.AsOf.ZoneID
	}
	if zoneID == "" {
		zoneID = "UTC"
	}
	end := now.Add(horizon)
	span, err := newRange(now, end, zoneID)
	if err != nil {
		return Outlook{}, err
	}

	view := Outlook{
		Freshness:     in.Freshness,
		Horizon:       span,
		Segments:      []Segment{},
		OfficeWindows: []OfficeWindow{},
		Commitments:   []Commitment{},
		Opportunities: []Opportunity{},
	}

	if in.EstimateError != nil {
		view.Status = StatusRefused
		view.Refusal = refusalOf(in.EstimateError)
		return view, nil
	}
	if len(in.Estimate.PredictedSleepWindows) == 0 && len(in.Estimate.PredictedWakingWindows) == 0 {
		view.Status = StatusRefused
		view.Refusal = &Refusal{
			Code:    "insufficient_data",
			Message: "There is no rhythm estimate to project forward yet.",
		}
		return view, nil
	}

	// A forecast is anchored to where the person is now in their cycle. When
	// the freshness policy will not support a current-state claim, that anchor
	// is missing, and the next three days would be a shape rather than a plan.
	if !in.Freshness.MayClaimCurrentState() {
		view.Status = StatusWithheld
		return view, nil
	}

	view.Status = StatusAvailable
	view.Segments = buildSegments(in, now, end, zoneID)
	view.NextSleep = nextWindowAfter(in.Estimate.PredictedSleepWindows, now, end)
	view.NextWake = nextWindowAfter(in.Estimate.PredictedWakingWindows, now, end)
	for _, segment := range view.Segments {
		switch segment.Presence {
		case PresenceAwake:
			view.AwakeFor += segment.Duration()
		case PresenceUncertain:
			view.UncertainFor += segment.Duration()
		}
	}

	view.OfficeWindows, err = officeWindows(in.OfficeHours, view.Segments, now, end, zoneID)
	if err != nil {
		return Outlook{}, err
	}
	view.Commitments = commitments(in.Events, view.Segments, now, end)
	view.Opportunities = opportunities(in, view.Segments, now)
	return view, nil
}

// refusalOf preserves the estimator's own typed refusal rather than flattening
// every failure into one message. "Not enough history yet" and "these records
// contradict each other" call for different things from the reader.
func refusalOf(err error) *Refusal {
	var typed *estimation.EstimationRefusal
	if errors.As(err, &typed) {
		return &Refusal{Code: string(typed.Code), Message: typed.Message}
	}
	return &Refusal{Code: "estimate_unavailable", Message: err.Error()}
}

// buildSegments sweeps the envelope boundaries and classifies each elementary
// interval between them.
//
// The sweep is done on absolute instants. Doing it on civil times would put a
// daylight-saving change in the middle of a night and silently create or lose
// an hour of predicted sleep.
func buildSegments(in Input, now, end time.Time, zoneID string) []Segment {
	sleep := clipWindows(in.Estimate.PredictedSleepWindows, now, end)
	wake := clipWindows(in.Estimate.PredictedWakingWindows, now, end)
	recorded := clipRanges(in.RecordedSleep, now, end)

	// The estimator's first waking window is the one *after* the next sleep, so
	// the period the person is in right now has no window of its own. Without
	// this the next several hours — the most useful part of the whole view —
	// would read "unknown".
	//
	// Its end is the earliest plausible onset, which is where the next sleep
	// envelope opens. Beyond that the envelopes take over and say "uncertain",
	// which is correct: from then on the model will not say whether sleep has
	// started.
	if len(sleep) > 0 {
		earliestOnset := sleep[0].start
		for _, interval := range sleep {
			if interval.start.Before(earliestOnset) {
				earliestOnset = interval.start
			}
		}
		if earliestOnset.After(now) {
			wake = append(wake, span{start: now, stop: earliestOnset})
		}
	}

	cuts := []time.Time{now, end}
	for _, interval := range sleep {
		cuts = append(cuts, interval.start, interval.stop)
	}
	for _, interval := range wake {
		cuts = append(cuts, interval.start, interval.stop)
	}
	for _, interval := range recorded {
		cuts = append(cuts, interval.start, interval.stop)
	}
	cuts = sortedUnique(cuts, now, end)

	segments := make([]Segment, 0, len(cuts))
	for i := 0; i+1 < len(cuts); i++ {
		start, stop := cuts[i], cuts[i+1]
		if !stop.After(start) {
			continue
		}
		mid := start.Add(stop.Sub(start) / 2)

		presence := PresenceUnknown
		observed := false
		switch {
		case covers(recorded, mid):
			// A stored episode is not a forecast, and it wins.
			presence, observed = PresenceAsleep, true
		case covers(sleep, mid) && covers(wake, mid):
			presence = PresenceUncertain
		case covers(sleep, mid):
			presence = PresenceAsleep
		case covers(wake, mid):
			presence = PresenceAwake
		}

		interval, err := newRange(start, stop, zoneID)
		if err != nil {
			continue
		}
		segments = append(segments, Segment{Presence: presence, Interval: interval, Observed: observed})
	}
	return mergeAdjacent(segments)
}

// mergeAdjacent joins neighbouring segments that say the same thing, so the
// timeline has one entry per stretch rather than one per boundary crossing.
func mergeAdjacent(segments []Segment) []Segment {
	merged := make([]Segment, 0, len(segments))
	for _, segment := range segments {
		last := len(merged) - 1
		if last >= 0 &&
			merged[last].Presence == segment.Presence &&
			merged[last].Observed == segment.Observed &&
			merged[last].Interval.End.UTC.Equal(segment.Interval.Start.UTC) {
			merged[last].Interval.End = segment.Interval.End
			continue
		}
		merged = append(merged, segment)
	}
	return merged
}

type span struct {
	start time.Time
	stop  time.Time
}

func clipWindows(windows []domain.AvailabilityWindow, from, to time.Time) []span {
	ranges := make([]domain.TimeRange, 0, len(windows))
	for _, window := range windows {
		ranges = append(ranges, window.Interval)
	}
	return clipRanges(ranges, from, to)
}

func clipRanges(ranges []domain.TimeRange, from, to time.Time) []span {
	clipped := make([]span, 0, len(ranges))
	for _, interval := range ranges {
		start := interval.Start.UTC
		stop := interval.End.UTC
		if start.Before(from) {
			start = from
		}
		if stop.After(to) {
			stop = to
		}
		if !stop.After(start) {
			continue
		}
		clipped = append(clipped, span{start: start, stop: stop})
	}
	return clipped
}

func covers(spans []span, at time.Time) bool {
	for _, interval := range spans {
		if !at.Before(interval.start) && at.Before(interval.stop) {
			return true
		}
	}
	return false
}

func sortedUnique(values []time.Time, from, to time.Time) []time.Time {
	filtered := make([]time.Time, 0, len(values))
	for _, value := range values {
		if value.Before(from) || value.After(to) {
			continue
		}
		filtered = append(filtered, value.UTC())
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Before(filtered[j]) })
	unique := filtered[:0]
	for i, value := range filtered {
		if i > 0 && value.Equal(unique[len(unique)-1]) {
			continue
		}
		unique = append(unique, value)
	}
	return unique
}

func nextWindowAfter(windows []domain.AvailabilityWindow, now, end time.Time) *domain.TimeRange {
	var best *domain.TimeRange
	for i := range windows {
		interval := windows[i].Interval
		if !interval.End.UTC.After(now) || interval.Start.UTC.After(end) {
			continue
		}
		if best == nil || interval.Start.UTC.Before(best.Start.UTC) {
			candidate := interval
			best = &candidate
		}
	}
	return best
}

// officeWindows walks the civil calendar day by day and intersects each
// office-open stretch with the timeline.
func officeWindows(hours OfficeHours, segments []Segment, now, end time.Time, zoneID string) ([]OfficeWindow, error) {
	if hours.StartLocal == "" && hours.EndLocal == "" && len(hours.Days) == 0 {
		hours = DefaultOfficeHours(zoneID)
	}
	if hours.ZoneID == "" {
		hours.ZoneID = zoneID
	}
	location, err := time.LoadLocation(hours.ZoneID)
	if err != nil {
		return nil, fmt.Errorf("office hours zone %q: %w", hours.ZoneID, err)
	}
	startClock, err := parseClock(hours.StartLocal, "09:00")
	if err != nil {
		return nil, err
	}
	endClock, err := parseClock(hours.EndLocal, "17:00")
	if err != nil {
		return nil, err
	}
	days := hours.Days
	if len(days) == 0 {
		days = DefaultOfficeHours(hours.ZoneID).Days
	}
	open := make(map[time.Weekday]bool, len(days))
	for _, day := range days {
		open[day] = true
	}

	awake := spansWithPresence(segments, PresenceAwake)
	uncertain := spansWithPresence(segments, PresenceUncertain)

	windows := make([]OfficeWindow, 0, 4)
	localStart := now.In(location)
	day := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, location)
	lastLocal := end.In(location)
	last := time.Date(lastLocal.Year(), lastLocal.Month(), lastLocal.Day(), 0, 0, 0, 0, location)

	for !day.After(last) {
		if !open[day.Weekday()] {
			day = day.AddDate(0, 0, 1)
			continue
		}
		// Constructed in the location so a daylight-saving transition shifts
		// the office day with the civil clock, which is what an office does.
		openAt := time.Date(day.Year(), day.Month(), day.Day(), startClock.hour, startClock.minute, 0, 0, location).UTC()
		closeAt := time.Date(day.Year(), day.Month(), day.Day(), endClock.hour, endClock.minute, 0, 0, location).UTC()
		if openAt.Before(now) {
			openAt = now
		}
		if closeAt.After(end) {
			closeAt = end
		}
		if !closeAt.After(openAt) {
			day = day.AddDate(0, 0, 1)
			continue
		}
		interval, rangeErr := newRange(openAt, closeAt, hours.ZoneID)
		if rangeErr != nil {
			day = day.AddDate(0, 0, 1)
			continue
		}
		window := OfficeWindow{
			Interval:  interval,
			Reachable: intersect(awake, openAt, closeAt, hours.ZoneID),
			Possible:  intersect(uncertain, openAt, closeAt, hours.ZoneID),
		}
		window.ReachableFor = totalDuration(window.Reachable)
		window.PossibleFor = totalDuration(window.Possible)
		windows = append(windows, window)
		day = day.AddDate(0, 0, 1)
	}
	return windows, nil
}

type clock struct {
	hour   int
	minute int
}

func parseClock(value, fallback string) (clock, error) {
	if value == "" {
		value = fallback
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return clock{}, fmt.Errorf("office hours clock %q: %w", value, err)
	}
	return clock{hour: parsed.Hour(), minute: parsed.Minute()}, nil
}

func spansWithPresence(segments []Segment, presence Presence) []span {
	spans := make([]span, 0, len(segments))
	for _, segment := range segments {
		if segment.Presence != presence {
			continue
		}
		spans = append(spans, span{start: segment.Interval.Start.UTC, stop: segment.Interval.End.UTC})
	}
	return spans
}

func intersect(spans []span, from, to time.Time, zoneID string) []domain.TimeRange {
	result := make([]domain.TimeRange, 0, len(spans))
	for _, interval := range spans {
		start := interval.start
		stop := interval.stop
		if start.Before(from) {
			start = from
		}
		if stop.After(to) {
			stop = to
		}
		if !stop.After(start) {
			continue
		}
		clipped, err := newRange(start, stop, zoneID)
		if err != nil {
			continue
		}
		result = append(result, clipped)
	}
	return result
}

func totalDuration(ranges []domain.TimeRange) time.Duration {
	var total time.Duration
	for _, interval := range ranges {
		total += interval.End.UTC.Sub(interval.Start.UTC)
	}
	return total
}

// commitments classifies fixed events against the timeline.
func commitments(events []domain.CalendarEvent, segments []Segment, now, end time.Time) []Commitment {
	asleep := spansWithPresence(segments, PresenceAsleep)
	uncertain := spansWithPresence(segments, PresenceUncertain)

	result := make([]Commitment, 0, len(events))
	for _, event := range events {
		if !event.Interval.End.UTC.After(now) || !event.Interval.Start.UTC.Before(end) {
			continue
		}
		start := event.Interval.Start.UTC
		stop := event.Interval.End.UTC
		sleepOverlap := overlapDuration(asleep, start, stop)
		uncertainOverlap := overlapDuration(uncertain, start, stop)
		total := stop.Sub(start)

		conflict := ConflictNone
		switch {
		case sleepOverlap > 0 && sleepOverlap >= total:
			conflict = ConflictInsidePredictedSleep
		case sleepOverlap > 0:
			conflict = ConflictOverlapsPredictedSleep
		case uncertainOverlap > 0:
			conflict = ConflictNearBoundary
		}
		result = append(result, Commitment{
			EventID:  event.ID,
			Interval: event.Interval,
			Conflict: conflict,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Interval.Start.UTC.Before(result[j].Interval.Start.UTC)
	})
	return result
}

func overlapDuration(spans []span, from, to time.Time) time.Duration {
	var total time.Duration
	for _, interval := range spans {
		start := interval.start
		stop := interval.stop
		if start.Before(from) {
			start = from
		}
		if stop.After(to) {
			stop = to
		}
		if stop.After(start) {
			total += stop.Sub(start)
		}
	}
	return total
}

// opportunities places tasks using the shared scheduler.
//
// Availability is the *confident* awake time, not the full predicted waking
// envelope the estimator emits. Placing a task in the uncertain band would be
// scheduling something for a stretch where the model does not know whether the
// person is up, which is how a suggestion ends up landing mid-sleep.
func opportunities(in Input, segments []Segment, now time.Time) []Opportunity {
	limit := in.MaximumOpportunities
	if limit <= 0 {
		limit = DefaultMaximumOpportunities
	}
	if len(in.Tasks) == 0 {
		return []Opportunity{}
	}

	availability := make([]domain.AvailabilityWindow, 0, len(segments))
	for _, segment := range segments {
		if segment.Presence != PresenceAwake {
			continue
		}
		availability = append(availability, domain.AvailabilityWindow{
			ID:         domain.AvailabilityWindowID(fmt.Sprintf("outlook-awake-%d", len(availability)+1)),
			Kind:       domain.AvailabilityPredictedWake,
			Interval:   segment.Interval,
			Confidence: in.Estimate.Confidence,
			EstimateID: in.Estimate.ID,
		})
	}

	scheduler := scheduling.Scheduler{}
	result := make([]Opportunity, 0, limit)
	for _, task := range in.Tasks {
		if len(result) >= limit {
			break
		}
		proposal, err := scheduler.Propose(scheduling.Request{
			Task:         task,
			Availability: availability,
			Events:       in.Events,
			WakeAnchor:   in.WakeAnchor,
			Now:          now,
		})
		if err != nil {
			result = append(result, Opportunity{
				TaskID:        task.ID,
				Unplaced:      scheduling.ClassifyUnplaced(err),
				NeedsApproval: true,
			})
			continue
		}
		window := proposal.Window
		result = append(result, Opportunity{
			TaskID: task.ID,
			Window: &window,
			// Every placement here is a suggestion. Nothing in this package
			// applies anything; a person decides, always (ADR-0012).
			NeedsApproval: true,
		})
	}
	return result
}

func newRange(start, end time.Time, zoneID string) (domain.TimeRange, error) {
	startInstant, err := domain.NewZonedInstant(start.UTC(), zoneID)
	if err != nil {
		return domain.TimeRange{}, err
	}
	endInstant, err := domain.NewZonedInstant(end.UTC(), zoneID)
	if err != nil {
		return domain.TimeRange{}, err
	}
	return domain.TimeRange{Start: startInstant, End: endInstant}, nil
}
