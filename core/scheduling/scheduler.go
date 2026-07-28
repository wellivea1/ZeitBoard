package scheduling

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"non24.app/core/domain"
)

var ErrNoReliableWindow = errors.New("no reliable scheduling window")

// Explanation codes are the v1 schedule-proposals vocabulary. The scheduler
// emits only codes it actually enforced, so a proposal never claims a guarantee
// the engine did not apply.
const (
	CodeWithinPredictedWakingWindow = "within_predicted_waking_window"
	CodeAvoidsFixedEvent            = "avoids_fixed_event"
	CodeWithinTaskBounds            = "within_task_bounds"
	CodeUncertaintyBufferApplied    = "uncertainty_buffer_applied"
)

// UnplacedReason is the v1 vocabulary for why a task produced no proposal.
type UnplacedReason string

const (
	ReasonNoAvailableInterval    UnplacedReason = "no_available_interval"
	ReasonOutsideForecastHorizon UnplacedReason = "outside_forecast_horizon"
	ReasonEstimateUnavailable    UnplacedReason = "estimate_unavailable"
	ReasonInvalidConstraints     UnplacedReason = "invalid_constraints"
)

// ClassifyUnplaced maps a Propose error to its contract reason code.
func ClassifyUnplaced(err error) UnplacedReason {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNoReliableWindow):
		return ReasonNoAvailableInterval
	default:
		return ReasonInvalidConstraints
	}
}

type Proposal struct {
	TaskID           domain.FlexibleTaskID      `json:"taskId"`
	Window           domain.TimeRange           `json:"window"`
	EstimateID       domain.PhaseEstimateID     `json:"estimateId"`
	Confidence       domain.InferenceConfidence `json:"confidence"`
	Explanation      string                     `json:"explanation"`
	ExplanationCodes []string                   `json:"explanationCodes"`
	NeedsApproval    bool                       `json:"needsApproval"`
	CreatedAt        time.Time                  `json:"createdAt"`
}

type Request struct {
	Task         domain.FlexibleTask
	Availability []domain.AvailabilityWindow
	Events       []domain.CalendarEvent
	WakeAnchor   *domain.WakeAnchor
	Now          time.Time
}

type Scheduler struct{}

func (Scheduler) Propose(request Request) (Proposal, error) {
	if request.Task.EstimatedDuration <= 0 {
		return Proposal{}, errors.New("task duration must be positive")
	}
	constraint := request.Task.Constraint
	busyIntervals := mergeEventIntervals(request.Events)
	var candidates []candidate
	for _, availability := range request.Availability {
		if availability.Kind != domain.AvailabilityPredictedWake && availability.Kind != domain.AvailabilityFunctional && availability.Kind != domain.AvailabilityFree {
			continue
		}
		if domain.ConfidenceRank(availability.Confidence.Level) < domain.ConfidenceRank(constraint.MinimumConfidence) {
			continue
		}
		ranges := []domain.TimeRange{availability.Interval}
		ranges = applyHardBounds(ranges, request)
		if constraint.BusinessHours {
			ranges = intersectBusinessHours(ranges, constraint)
		}
		ranges = subtractBusyIntervals(ranges, busyIntervals)
		for _, interval := range ranges {
			if interval.Duration() >= request.Task.EstimatedDuration {
				candidates = append(candidates, candidate{interval: interval, availability: availability})
			}
		}
	}
	if len(candidates) == 0 {
		return Proposal{}, ErrNoReliableWindow
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].interval.Start.UTC.Before(candidates[j].interval.Start.UTC)
	})
	selected := candidates[0]
	end, _ := domain.NewZonedInstant(selected.interval.Start.UTC.Add(request.Task.EstimatedDuration), selected.interval.Start.ZoneID)
	return Proposal{
		TaskID:           request.Task.ID,
		Window:           domain.TimeRange{Start: selected.interval.Start, End: end},
		EstimateID:       selected.availability.EstimateID,
		Confidence:       selected.availability.Confidence,
		Explanation:      fmt.Sprintf("Earliest deterministic fit in a %s window after applying fixed events and task constraints.", selected.availability.Kind),
		ExplanationCodes: explanationCodes(selected, request),
		NeedsApproval:    constraint.RequiresApproval || !constraint.AllowAutomaticMovement,
		CreatedAt:        time.Now().UTC(),
	}, nil
}

// explanationCodes reports only the guarantees the engine actually applied to
// this placement. uncertainty_buffer_applied is intentionally never emitted
// yet: the scheduler takes the earliest deterministic fit and does not pad the
// window edges, so claiming a buffer would be false.
func explanationCodes(selected candidate, request Request) []string {
	var codes []string
	if selected.availability.Kind == domain.AvailabilityPredictedWake {
		codes = append(codes, CodeWithinPredictedWakingWindow)
	}
	if windowOverlapsEvent(selected.availability.Interval, request.Events) {
		codes = append(codes, CodeAvoidsFixedEvent)
	}
	if hasExplicitBounds(request.Task.Constraint, request.WakeAnchor) {
		codes = append(codes, CodeWithinTaskBounds)
	}
	if len(codes) == 0 {
		// The placement always sits inside the engine's feasible region.
		codes = append(codes, CodeWithinTaskBounds)
	}
	return codes
}

func windowOverlapsEvent(window domain.TimeRange, events []domain.CalendarEvent) bool {
	for _, event := range events {
		if window.Overlaps(event.Interval) {
			return true
		}
	}
	return false
}

func hasExplicitBounds(constraint domain.TaskConstraint, wake *domain.WakeAnchor) bool {
	if constraint.BusinessHours || constraint.Deadline != nil || constraint.EarliestStart != nil || constraint.LatestFinish != nil {
		return true
	}
	return constraint.PreferredAfterWake != nil && wake != nil
}

type candidate struct {
	interval     domain.TimeRange
	availability domain.AvailabilityWindow
}

func applyHardBounds(ranges []domain.TimeRange, request Request) []domain.TimeRange {
	constraint := request.Task.Constraint
	earliest := request.Now.UTC()
	if constraint.EarliestStart != nil && constraint.EarliestStart.UTC.After(earliest) {
		earliest = constraint.EarliestStart.UTC
	}
	if constraint.PreferredAfterWake != nil && request.WakeAnchor != nil {
		preferred := request.WakeAnchor.At.UTC.Add(*constraint.PreferredAfterWake)
		if preferred.After(earliest) {
			earliest = preferred
		}
	}
	var latest time.Time
	if constraint.Deadline != nil {
		latest = constraint.Deadline.UTC
	}
	if constraint.LatestFinish != nil && (latest.IsZero() || constraint.LatestFinish.UTC.Before(latest)) {
		latest = constraint.LatestFinish.UTC
	}
	var result []domain.TimeRange
	for _, interval := range ranges {
		start := interval.Start.UTC
		end := interval.End.UTC
		if earliest.After(start) {
			start = earliest
		}
		if !latest.IsZero() && latest.Before(end) {
			end = latest
		}
		if !end.After(start) {
			continue
		}
		startInstant, _ := domain.NewZonedInstant(start, interval.Start.ZoneID)
		endInstant, _ := domain.NewZonedInstant(end, interval.End.ZoneID)
		result = append(result, domain.TimeRange{Start: startInstant, End: endInstant})
	}
	return result
}

func intersectBusinessHours(ranges []domain.TimeRange, constraint domain.TaskConstraint) []domain.TimeRange {
	startClock := constraint.BusinessStartLocal
	endClock := constraint.BusinessEndLocal
	if startClock == "" {
		startClock = "09:00"
	}
	if endClock == "" {
		endClock = "17:00"
	}
	startParsed, startErr := time.Parse("15:04", startClock)
	endParsed, endErr := time.Parse("15:04", endClock)
	if startErr != nil || endErr != nil {
		return nil
	}
	var result []domain.TimeRange
	for _, interval := range ranges {
		location, err := time.LoadLocation(interval.Start.ZoneID)
		if err != nil {
			continue
		}
		localStart := interval.Start.UTC.In(location)
		localEnd := interval.End.UTC.In(location)
		day := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, location)
		lastDay := time.Date(localEnd.Year(), localEnd.Month(), localEnd.Day(), 0, 0, 0, 0, location)
		for !day.After(lastDay) {
			businessStart := time.Date(day.Year(), day.Month(), day.Day(), startParsed.Hour(), startParsed.Minute(), 0, 0, location)
			businessEnd := time.Date(day.Year(), day.Month(), day.Day(), endParsed.Hour(), endParsed.Minute(), 0, 0, location)
			start := maxInstant(interval.Start.UTC, businessStart.UTC())
			end := minInstant(interval.End.UTC, businessEnd.UTC())
			if end.After(start) {
				startInstant, _ := domain.NewZonedInstant(start, interval.Start.ZoneID)
				endInstant, _ := domain.NewZonedInstant(end, interval.Start.ZoneID)
				result = append(result, domain.TimeRange{Start: startInstant, End: endInstant})
			}
			day = day.AddDate(0, 0, 1)
		}
	}
	return result
}

func subtractEvents(ranges []domain.TimeRange, events []domain.CalendarEvent) []domain.TimeRange {
	return subtractBusyIntervals(ranges, mergeEventIntervals(events))
}

type busyInterval struct {
	start time.Time
	end   time.Time
}

func mergeEventIntervals(events []domain.CalendarEvent) []busyInterval {
	intervals := make([]busyInterval, 0, len(events))
	for _, event := range events {
		start := event.Interval.Start.UTC
		end := event.Interval.End.UTC
		if !end.After(start) {
			continue
		}
		intervals = append(intervals, busyInterval{start: start, end: end})
	}
	sort.Slice(intervals, func(i, j int) bool {
		if intervals[i].start.Equal(intervals[j].start) {
			return intervals[i].end.Before(intervals[j].end)
		}
		return intervals[i].start.Before(intervals[j].start)
	})

	merged := intervals[:0]
	for _, interval := range intervals {
		if len(merged) == 0 || interval.start.After(merged[len(merged)-1].end) {
			merged = append(merged, interval)
			continue
		}
		if interval.end.After(merged[len(merged)-1].end) {
			merged[len(merged)-1].end = interval.end
		}
	}
	return merged
}

func subtractBusyIntervals(ranges []domain.TimeRange, busy []busyInterval) []domain.TimeRange {
	if len(ranges) == 0 {
		return nil
	}
	if len(busy) == 0 {
		return append([]domain.TimeRange(nil), ranges...)
	}

	result := make([]domain.TimeRange, 0, len(ranges)+len(busy))
	for _, interval := range ranges {
		rangeStart := interval.Start.UTC
		rangeEnd := interval.End.UTC
		cursor := rangeStart
		segmentStart := interval.Start

		first := sort.Search(len(busy), func(i int) bool {
			return busy[i].end.After(rangeStart)
		})
		for i := first; i < len(busy) && busy[i].start.Before(rangeEnd); i++ {
			if !busy[i].end.After(cursor) {
				continue
			}
			if busy[i].start.After(cursor) {
				segmentEnd, _ := domain.NewZonedInstant(busy[i].start, interval.Start.ZoneID)
				result = append(result, domain.TimeRange{Start: segmentStart, End: segmentEnd})
			}
			cursor = busy[i].end
			if !cursor.Before(rangeEnd) {
				break
			}
			segmentStart, _ = domain.NewZonedInstant(cursor, interval.Start.ZoneID)
		}
		if cursor.Before(rangeEnd) {
			result = append(result, domain.TimeRange{Start: segmentStart, End: interval.End})
		}
	}
	return result
}

func maxInstant(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}

func minInstant(left, right time.Time) time.Time {
	if right.Before(left) {
		return right
	}
	return left
}
