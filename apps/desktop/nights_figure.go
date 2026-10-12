package main

import (
	"fmt"
	"time"

	"non24.app/core/domain"
	"non24.app/core/inference"
)

// Suggested nights beside recorded ones, for any source that suggests them:
// a Google export (ADR-0059) or this computer (ADR-0060). They are shadow
// evidence; nothing that plans or estimates reads them.

// NightsFigureDTO is a source's suggested nights and the recorded nights over
// the span it covers. Suggested and Recorded are nights cut at midnight in
// ZoneID: [day, start minute, end minute], day 0 being FirstDate.
type NightsFigureDTO struct {
	Nights    int                 `json:"nights"`
	SpanLabel string              `json:"spanLabel"`
	ZoneID    string              `json:"zoneId"`
	FirstDate string              `json:"firstDate"`
	Days      int                 `json:"days"`
	Suggested [][3]int            `json:"suggested"`
	Recorded  [][3]int            `json:"recorded"`
	Agreement *NightsAgreementDTO `json:"agreement,omitempty"`
}

// NightsAgreementDTO measures suggestions on recorded nights only: a night
// nobody recorded is not evidence either way.
type NightsAgreementDTO struct {
	Recorded int    `json:"recorded"`
	Matched  int    `json:"matched"`
	Summary  string `json:"summary"`
}

// suggestNights applies ADR-0031's rules to one source's quiet intervals
// alone, so that what it suggests can be measured against what was recorded.
func suggestNights(quiet []inference.Interval) []inference.Candidate {
	nights, refusal := inference.Build(quiet, time.Time{})
	if refusal != nil {
		return nil
	}
	return nights
}

// nightsFigure lays a source's suggested nights out beside the principal
// sleeps recorded wholly inside the span it covers, first to last.
func nightsFigure(nights []inference.Candidate, sessions []domain.SleepSession, first, last time.Time, location *time.Location) *NightsFigureDTO {
	recorded := recordedNightsWithin(sessions, first, last)
	suggested := make([]inference.TruthInterval, len(nights))
	for i, night := range nights {
		suggested[i] = inference.TruthInterval{StartAt: night.StartAt, EndAt: night.EndAt}
	}
	firstDay := civilDay(first, location)
	return &NightsFigureDTO{
		Nights:    len(nights),
		SpanLabel: dateSpanLabel(first, last, location),
		ZoneID:    location.String(),
		FirstDate: firstDay.Format("2006-01-02"),
		Days:      civilDaysBetween(firstDay, civilDay(last, location)) + 1,
		Suggested: nightSegments(suggested, firstDay, location),
		Recorded:  nightSegments(recorded, firstDay, location),
		Agreement: nightAgreement(nights, recorded),
	}
}

// recordedNightsWithin are the principal sleeps recorded wholly inside the
// span a source covers: the only nights it could have suggested.
func recordedNightsWithin(sessions []domain.SleepSession, first, last time.Time) []inference.TruthInterval {
	var nights []inference.TruthInterval
	for _, session := range sessions {
		if !session.IsPrincipalSleep() || len(session.Intervals) == 0 {
			continue
		}
		start := session.Intervals[0].Interval.Start.UTC
		end := session.Intervals[len(session.Intervals)-1].Interval.End.UTC
		if start.Before(first) || end.After(last) {
			continue
		}
		nights = append(nights, inference.TruthInterval{StartAt: start, EndAt: end})
	}
	return nights
}

// nightAgreement says how many recorded nights had a suggestion beside them
// and, where one did, how far apart their edges typically were.
func nightAgreement(suggested []inference.Candidate, recorded []inference.TruthInterval) *NightsAgreementDTO {
	if len(recorded) == 0 {
		return nil
	}
	score := inference.ScoreCandidates(suggested, recorded)
	agreement := &NightsAgreementDTO{Recorded: len(recorded), Matched: score.Matched}
	if score.Matched > 0 {
		agreement.Summary = fmt.Sprintf("On those nights, suggestions typically began %s and ended %s.",
			edgeWords(score.OnsetBias, "your recorded sleep"), edgeWords(score.WakeBias, "your recorded wake"))
	}
	return agreement
}

// edgeWords puts a signed median, a suggestion's edge minus the recorded one,
// into words. Within five minutes is the same time: the evidence knows
// minutes, and people stop using things a little before they sleep.
func edgeWords(bias time.Duration, edge string) string {
	bias = bias.Round(time.Minute)
	switch {
	case bias > -5*time.Minute && bias < 5*time.Minute:
		return "at about " + edge
	case bias < 0:
		return formatDuration(-bias) + " before " + edge
	default:
		return formatDuration(bias) + " after " + edge
	}
}

// nightSegments cuts each night at midnight in location, giving each part as
// [day, start minute, end minute] with day 0 being firstDay.
func nightSegments(nights []inference.TruthInterval, firstDay time.Time, location *time.Location) [][3]int {
	segments := [][3]int{}
	for _, night := range nights {
		start := night.StartAt.In(location)
		end := night.EndAt.In(location)
		for day := civilDay(start, location); day.Before(end); day = day.AddDate(0, 0, 1) {
			next := day.AddDate(0, 0, 1)
			from, to := laterOf(start, day), earlierOf(end, next)
			if !to.After(from) {
				continue
			}
			index := civilDaysBetween(firstDay, day)
			if index < 0 {
				continue
			}
			endMinute := 24 * 60
			if to.Before(next) {
				endMinute = minuteOfDay(to)
			}
			segments = append(segments, [3]int{index, minuteOfDay(from), endMinute})
		}
	}
	return segments
}

func civilDay(at time.Time, location *time.Location) time.Time {
	local := at.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

// civilDaysBetween counts calendar days, whatever a daylight-saving change
// does to the hours between them.
func civilDaysBetween(from, to time.Time) int {
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}

func minuteOfDay(at time.Time) int { return at.Hour()*60 + at.Minute() }

func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlierOf(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func dateSpanLabel(first, last time.Time, location *time.Location) string {
	from, to := first.In(location).Format("Jan 2, 2006"), last.In(location).Format("Jan 2, 2006")
	if from == to {
		return from
	}
	return from + " to " + to
}

// displayLocation is this computer's zone, in which the desktop states civil
// times.
func displayLocation() *time.Location {
	location, err := time.LoadLocation(localZoneID())
	if err != nil {
		return time.UTC
	}
	return location
}
