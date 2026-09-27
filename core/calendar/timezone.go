package calendar

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"non24.app/core/domain"
	"non24.app/core/timezones"
)

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

	// A Windows name, alone or after a prefix such as "Microsoft/".
	for _, name := range []string{id, id[strings.LastIndexByte(id, '/')+1:]} {
		if ianaID, ok := timezones.FromWindows(name); ok {
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
	resolution, err := domain.ResolveCivilTime(loc, year, month, day, hour, minute, second)
	if err != nil {
		return time.Time{}, err
	}
	return resolution.Time, nil
}
