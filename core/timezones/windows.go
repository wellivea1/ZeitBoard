// Package timezones maps zone names other systems use to IANA zone ids.
package timezones

import "strings"

// windowsZones maps Windows zone names, lower-cased, to the IANA zone CLDR
// names for each one's main territory. It covers the zones calendar exports
// commonly carry; Windows' own ICU names the rest (core/platform/localzone).
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
	"aus eastern standard time":    "Australia/Sydney",
	"e. australia standard time":   "Australia/Brisbane",
	"new zealand standard time":    "Pacific/Auckland",
}

// FromWindows returns the IANA zone for a Windows zone name, in any case.
func FromWindows(name string) (string, bool) {
	id, ok := windowsZones[strings.ToLower(strings.TrimSpace(name))]
	return id, ok
}
