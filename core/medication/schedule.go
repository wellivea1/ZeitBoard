package medication

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"non24.app/core/domain"
)

const (
	ScheduleAsNeeded   = "as_needed"
	ScheduleFixedClock = "fixed_clock"
	ScheduleCycling    = "cycling"

	ForecastNotApplicable = "not_applicable"
	ForecastUnavailable   = "unavailable"
	ForecastNoOverlap     = "no_overlap"
	ForecastCollision     = "collision"

	OccurrenceInsidePredictedSleep  = "inside_predicted_sleep"
	OccurrenceOutsidePredictedSleep = "outside_predicted_sleep"
	OccurrenceOutsideForecast       = "outside_forecast"
)

var civilClockPattern = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)

type Schedule struct {
	Kind            string   `json:"kind"`
	ZoneID          string   `json:"zone_id,omitempty"`
	CivilTimes      []string `json:"civil_times,omitempty"`
	DaysOn          int      `json:"days_on,omitempty"`
	DaysOff         int      `json:"days_off,omitempty"`
	CycleStartedOn  string   `json:"cycle_started_on,omitempty"`
	ReminderEnabled bool     `json:"reminder_enabled"`
}

type ScheduleOccurrence struct {
	At        domain.ZonedInstant
	CivilDate string
	CivilTime string
	Ambiguous bool
}

type ScheduleGap struct {
	CivilDate string
	CivilTime string
	Reason    string
}

type ScheduleExpansion struct {
	ScheduleKind string
	Occurrences  []ScheduleOccurrence
	Gaps         []ScheduleGap
}

type OccurrenceAssessment struct {
	Occurrence ScheduleOccurrence
	Status     string
	Confidence domain.InferenceConfidence
	WindowID   domain.AvailabilityWindowID
}

type CollisionForecast struct {
	Status              string
	Assessments         []OccurrenceAssessment
	Gaps                []ScheduleGap
	CoveredCount        int
	CollisionCount      int
	OutsideHorizonCount int
	CoverageEndsAt      *time.Time
	EstimateID          domain.PhaseEstimateID
}

func (schedule Schedule) Validate() error {
	seen := make(map[string]struct{}, len(schedule.CivilTimes))
	for _, civilTime := range schedule.CivilTimes {
		if !civilClockPattern.MatchString(civilTime) {
			return errors.New("schedule civil times must use HH:MM")
		}
		if _, duplicate := seen[civilTime]; duplicate {
			return errors.New("schedule civil times must be unique")
		}
		seen[civilTime] = struct{}{}
	}
	switch schedule.Kind {
	case ScheduleAsNeeded:
		if schedule.ZoneID != "" || len(schedule.CivilTimes) != 0 || schedule.DaysOn != 0 || schedule.DaysOff != 0 || schedule.CycleStartedOn != "" || schedule.ReminderEnabled {
			return errors.New("as-needed schedules cannot include a zone, clock, cycle, or reminder")
		}
	case ScheduleFixedClock:
		if err := validateClockSchedule(schedule); err != nil {
			return err
		}
		if schedule.DaysOn != 0 || schedule.DaysOff != 0 || schedule.CycleStartedOn != "" {
			return errors.New("fixed-clock schedules cannot include cycle fields")
		}
	case ScheduleCycling:
		if err := validateClockSchedule(schedule); err != nil {
			return err
		}
		if schedule.DaysOn < 1 || schedule.DaysOn > 365 || schedule.DaysOff < 1 || schedule.DaysOff > 365 {
			return errors.New("cycling schedules require 1 to 365 on/off days")
		}
		parsed, err := time.Parse(time.DateOnly, schedule.CycleStartedOn)
		if err != nil || parsed.Format(time.DateOnly) != schedule.CycleStartedOn {
			return errors.New("cycling schedule start must be a real YYYY-MM-DD date")
		}
	default:
		return errors.New("schedule kind must be as_needed, fixed_clock, or cycling")
	}
	return nil
}

func validateClockSchedule(schedule Schedule) error {
	if len(schedule.CivilTimes) == 0 || len(schedule.CivilTimes) > 8 {
		return errors.New("clock schedules require 1 to 8 civil times")
	}
	if schedule.ZoneID == "" || schedule.ZoneID == "Local" {
		return errors.New("clock schedules require an IANA time zone")
	}
	if _, err := time.LoadLocation(schedule.ZoneID); err != nil {
		return fmt.Errorf("load schedule time zone %q: %w", schedule.ZoneID, err)
	}
	return nil
}

func ExpandSchedule(schedule Schedule, from, to time.Time) (ScheduleExpansion, error) {
	result := ScheduleExpansion{Occurrences: []ScheduleOccurrence{}, Gaps: []ScheduleGap{}}
	if err := schedule.Validate(); err != nil {
		return result, err
	}
	result.ScheduleKind = schedule.Kind
	if from.IsZero() || to.IsZero() || !to.After(from) {
		return result, errors.New("schedule expansion requires a non-empty forward interval")
	}
	if schedule.Kind == ScheduleAsNeeded {
		return result, nil
	}
	location, _ := time.LoadLocation(schedule.ZoneID)
	startLocal := from.In(location)
	endLocal := to.Add(-time.Nanosecond).In(location)
	day := civilDateAtNoon(startLocal, location)
	lastDay := civilDateAtNoon(endLocal, location)
	times := append([]string(nil), schedule.CivilTimes...)
	sort.Strings(times)

	for !day.After(lastDay) {
		if schedule.Kind != ScheduleCycling || cyclingDayIsOn(schedule, day) {
			for _, civilTime := range times {
				hour, minute := parseCivilClock(civilTime)
				resolution, err := domain.ResolveCivilTime(location, day.Year(), day.Month(), day.Day(), hour, minute, 0)
				if err != nil {
					if !civilTimeWithinInterval(day, hour, minute, from, to, location) {
						continue
					}
					result.Gaps = append(result.Gaps, ScheduleGap{
						CivilDate: day.Format(time.DateOnly),
						CivilTime: civilTime,
						Reason:    "nonexistent_civil_time",
					})
					continue
				}
				if resolution.Time.Before(from) || !resolution.Time.Before(to) {
					continue
				}
				instant, err := domain.NewZonedInstant(resolution.Time, schedule.ZoneID)
				if err != nil {
					return result, err
				}
				result.Occurrences = append(result.Occurrences, ScheduleOccurrence{
					At:        instant,
					CivilDate: day.Format(time.DateOnly),
					CivilTime: civilTime,
					Ambiguous: resolution.Ambiguous,
				})
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	sort.Slice(result.Occurrences, func(i, j int) bool {
		return result.Occurrences[i].At.UTC.Before(result.Occurrences[j].At.UTC)
	})
	return result, nil
}

func AnalyzeCollisions(expansion ScheduleExpansion, estimate *domain.PhaseEstimate, coverageStartsAt time.Time) (CollisionForecast, error) {
	result := CollisionForecast{
		Status:      ForecastUnavailable,
		Assessments: make([]OccurrenceAssessment, 0, len(expansion.Occurrences)),
		Gaps:        append([]ScheduleGap(nil), expansion.Gaps...),
	}
	if expansion.ScheduleKind == ScheduleAsNeeded {
		result.Status = ForecastNotApplicable
		return result, nil
	}
	if len(expansion.Occurrences) == 0 && len(expansion.Gaps) == 0 {
		return result, nil
	}
	if estimate == nil || len(estimate.PredictedSleepWindows) == 0 {
		for _, occurrence := range expansion.Occurrences {
			result.Assessments = append(result.Assessments, unavailableAssessment(occurrence))
		}
		result.OutsideHorizonCount = len(expansion.Occurrences)
		return result, nil
	}
	result.EstimateID = estimate.ID
	var coverageEnd time.Time
	for _, window := range estimate.PredictedSleepWindows {
		if err := window.Interval.Validate(); err != nil {
			return CollisionForecast{}, fmt.Errorf("predicted sleep window %s: %w", window.ID, err)
		}
		if window.Interval.End.UTC.After(coverageEnd) {
			coverageEnd = window.Interval.End.UTC
		}
	}
	result.CoverageEndsAt = &coverageEnd
	for _, occurrence := range expansion.Occurrences {
		assessment := OccurrenceAssessment{
			Occurrence: occurrence,
			Status:     OccurrenceOutsidePredictedSleep,
			Confidence: estimate.Confidence,
		}
		if occurrence.At.UTC.Before(coverageStartsAt.UTC()) || !occurrence.At.UTC.Before(coverageEnd) {
			assessment = unavailableAssessment(occurrence)
			result.OutsideHorizonCount++
			result.Assessments = append(result.Assessments, assessment)
			continue
		}
		result.CoveredCount++
		for _, window := range estimate.PredictedSleepWindows {
			if window.Interval.Contains(occurrence.At.UTC) {
				assessment.Status = OccurrenceInsidePredictedSleep
				assessment.Confidence = window.Confidence
				assessment.WindowID = window.ID
				result.CollisionCount++
				break
			}
		}
		result.Assessments = append(result.Assessments, assessment)
	}
	if result.CoveredCount == 0 {
		result.Status = ForecastUnavailable
	} else if result.CollisionCount > 0 {
		result.Status = ForecastCollision
	} else {
		result.Status = ForecastNoOverlap
	}
	return result, nil
}

func unavailableAssessment(occurrence ScheduleOccurrence) OccurrenceAssessment {
	return OccurrenceAssessment{
		Occurrence: occurrence,
		Status:     OccurrenceOutsideForecast,
		Confidence: domain.InferenceConfidence{
			Level:   domain.ConfidenceUnknown,
			Reasons: []string{"outside the current estimate horizon"},
		},
	}
}

func cyclingDayIsOn(schedule Schedule, day time.Time) bool {
	started, _ := time.Parse(time.DateOnly, schedule.CycleStartedOn)
	currentOrdinal := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	startOrdinal := time.Date(started.Year(), started.Month(), started.Day(), 0, 0, 0, 0, time.UTC)
	days := int(currentOrdinal.Sub(startOrdinal) / (24 * time.Hour))
	if days < 0 {
		return false
	}
	return days%(schedule.DaysOn+schedule.DaysOff) < schedule.DaysOn
}

func civilDateAtNoon(value time.Time, location *time.Location) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 12, 0, 0, 0, location)
}

func parseCivilClock(value string) (int, int) {
	parsed, _ := time.Parse("15:04", value)
	return parsed.Hour(), parsed.Minute()
}

func civilTimeWithinInterval(day time.Time, hour, minute int, from, to time.Time, location *time.Location) bool {
	candidate := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.UTC)
	fromLocal := from.In(location)
	fromCivil := time.Date(fromLocal.Year(), fromLocal.Month(), fromLocal.Day(), fromLocal.Hour(), fromLocal.Minute(), fromLocal.Second(), fromLocal.Nanosecond(), time.UTC)
	toLocal := to.In(location)
	toCivil := time.Date(toLocal.Year(), toLocal.Month(), toLocal.Day(), toLocal.Hour(), toLocal.Minute(), toLocal.Second(), toLocal.Nanosecond(), time.UTC)
	return !candidate.Before(fromCivil) && candidate.Before(toCivil)
}
