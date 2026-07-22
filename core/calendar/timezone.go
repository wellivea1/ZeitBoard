package calendar

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
)

var windowsZones = map[string]string{
	"dateline standard time":       "Etc/GMT+12",
	"utc-11":                       "Etc/GMT+11",
	"aleutian standard time":       "America/Adak",
	"hawaiian standard time":       "Pacific/Honolulu",
	"alaskan standard time":        "America/Anchorage",
	"pacific standard time":        "America/Los_Angeles",
	"us mountain standard time":    "America/Phoenix",
	"mountain standard time":       "America/Denver",
	"central standard time":        "America/Chicago",
	"eastern standard time":        "America/New_York",
	"atlantic standard time":       "America/Halifax",
	"newfoundland standard time":   "America/St_Johns",
	"utc":                          "UTC",
	"gmt standard time":            "Europe/London",
	"greenwich standard time":      "Atlantic/Reykjavik",
	"w. europe standard time":      "Europe/Berlin",
	"central europe standard time": "Europe/Budapest",
	"romance standard time":        "Europe/Paris",
	"russian standard time":        "Europe/Moscow",
	"turkey standard time":         "Europe/Istanbul",
	"israel standard time":         "Asia/Jerusalem",
	"arabian standard time":        "Asia/Dubai",
	"india standard time":          "Asia/Kolkata",
	"se asia standard time":        "Asia/Bangkok",
	"china standard time":          "Asia/Shanghai",
	"tokyo standard time":          "Asia/Tokyo",
	"korea standard time":          "Asia/Seoul",
	"aus eastern standard time":    "Australia/Sydney",
	"e. australia standard time":   "Australia/Brisbane",
	"new zealand standard time":    "Pacific/Auckland",
}

func loadLocation(rawID string) (*time.Location, string, error) {
	id := strings.Trim(strings.TrimSpace(rawID), `"`)
	if id == "" {
		return nil, "", fmt.Errorf("time zone id is empty")
	}
	if loc, err := time.LoadLocation(id); err == nil {
		return loc, id, nil
	}
	for slash := strings.IndexByte(id, '/'); slash >= 0; {
		candidate := strings.TrimPrefix(id[slash:], "/")
		if strings.Contains(candidate, "/") {
			if loc, err := time.LoadLocation(candidate); err == nil {
				return loc, candidate, nil
			}
		}
		next := strings.IndexByte(id[slash+1:], '/')
		if next < 0 {
			break
		}
		slash += next + 1
	}

	lowerID := strings.ToLower(id)
	for windowsID, ianaID := range windowsZones {
		if lowerID == windowsID || strings.HasSuffix(lowerID, "/"+windowsID) {
			loc, err := time.LoadLocation(ianaID)
			if err != nil {
				return nil, "", fmt.Errorf("load mapped time zone %q: %w", ianaID, err)
			}
			return loc, ianaID, nil
		}
	}
	return nil, "", fmt.Errorf("unsupported time zone %q", rawID)
}

// resolveCivil rejects nonexistent wall times and chooses the earlier instant
// when a fall-back transition makes the same wall time occur twice, matching
// RFC 5545's first-occurrence rule.
func resolveCivil(loc *time.Location, year int, month time.Month, day, hour, minute, second int) (time.Time, error) {
	candidate := time.Date(year, month, day, hour, minute, second, 0, loc)
	if !sameCivil(candidate, year, month, day, hour, minute, second) {
		return time.Time{}, fmt.Errorf("nonexistent civil time %04d-%02d-%02d %02d:%02d:%02d in %s", year, month, day, hour, minute, second, loc)
	}

	first := candidate
	for _, delta := range []time.Duration{-3 * time.Hour, -2 * time.Hour, -time.Hour, -30 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 3 * time.Hour} {
		other := candidate.Add(delta)
		if sameCivil(other.In(loc), year, month, day, hour, minute, second) && other.Before(first) {
			first = other
		}
	}
	return first, nil
}

func sameCivil(value time.Time, year int, month time.Month, day, hour, minute, second int) bool {
	y, m, d := value.Date()
	h, min, sec := value.Clock()
	return y == year && m == month && d == day && h == hour && min == minute && sec == second
}
