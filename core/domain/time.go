package domain

import (
	"errors"
	"fmt"
	"time"
	_ "time/tzdata"
)

type CivilTimeResolution struct {
	Time      time.Time
	Ambiguous bool
}

// ResolveCivilTime rejects wall times skipped by a time-zone transition and
// chooses the earlier instant when a fall-back transition repeats a wall time.
func ResolveCivilTime(location *time.Location, year int, month time.Month, day, hour, minute, second int) (CivilTimeResolution, error) {
	if location == nil {
		return CivilTimeResolution{}, errors.New("time zone is required")
	}
	candidate := time.Date(year, month, day, hour, minute, second, 0, location)
	if !sameCivilTime(candidate, year, month, day, hour, minute, second) {
		return CivilTimeResolution{}, fmt.Errorf("nonexistent civil time %04d-%02d-%02d %02d:%02d:%02d in %s", year, month, day, hour, minute, second, location)
	}

	first := candidate
	ambiguous := false
	for _, delta := range []time.Duration{-3 * time.Hour, -2 * time.Hour, -time.Hour, -30 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 3 * time.Hour} {
		other := candidate.Add(delta)
		if sameCivilTime(other.In(location), year, month, day, hour, minute, second) {
			ambiguous = ambiguous || !other.Equal(candidate)
			if other.Before(first) {
				first = other
			}
		}
	}
	return CivilTimeResolution{Time: first, Ambiguous: ambiguous}, nil
}

func sameCivilTime(value time.Time, year int, month time.Month, day, hour, minute, second int) bool {
	y, m, d := value.Date()
	h, min, sec := value.Clock()
	return y == year && m == month && d == day && h == hour && min == minute && sec == second
}

func NewZonedInstant(value time.Time, zoneID string) (ZonedInstant, error) {
	if value.IsZero() {
		return ZonedInstant{}, errors.New("instant must not be zero")
	}
	if zoneID == "" {
		return ZonedInstant{}, errors.New("IANA time-zone identifier is required")
	}
	if _, err := time.LoadLocation(zoneID); err != nil {
		return ZonedInstant{}, fmt.Errorf("load time zone %q: %w", zoneID, err)
	}
	return ZonedInstant{UTC: value.UTC(), ZoneID: zoneID}, nil
}

func MustZonedInstant(value time.Time, zoneID string) ZonedInstant {
	instant, err := NewZonedInstant(value, zoneID)
	if err != nil {
		panic(err)
	}
	return instant
}

func (z ZonedInstant) Validate() error {
	_, err := NewZonedInstant(z.UTC, z.ZoneID)
	return err
}

func (z ZonedInstant) InLocation() (time.Time, error) {
	if err := z.Validate(); err != nil {
		return time.Time{}, err
	}
	location, _ := time.LoadLocation(z.ZoneID)
	return z.UTC.In(location), nil
}

func (r TimeRange) Validate() error {
	if err := r.Start.Validate(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if err := r.End.Validate(); err != nil {
		return fmt.Errorf("end: %w", err)
	}
	if !r.End.UTC.After(r.Start.UTC) {
		return errors.New("interval end must be after start")
	}
	return nil
}

func (r TimeRange) Duration() time.Duration {
	return r.End.UTC.Sub(r.Start.UTC)
}

func (r TimeRange) Overlaps(other TimeRange) bool {
	return r.Start.UTC.Before(other.End.UTC) && other.Start.UTC.Before(r.End.UTC)
}

func (r TimeRange) Contains(value time.Time) bool {
	value = value.UTC()
	return !value.Before(r.Start.UTC) && value.Before(r.End.UTC)
}

func ConfidenceRank(level ConfidenceLevel) int {
	switch level {
	case ConfidenceHigh:
		return 3
	case ConfidenceMedium:
		return 2
	case ConfidenceLow:
		return 1
	default:
		return 0
	}
}
