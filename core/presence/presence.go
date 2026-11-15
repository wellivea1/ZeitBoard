// Package presence defines a reported status (ADR-0070): what the owner says
// about the present, as against what the sleep records predict. "I am awake",
// "I am trying to sleep" and "do not disturb" are reports, with the time they
// were made and the time they stop standing.
//
// A report is the owner's word and nothing more. It is never an estimator
// input: "trying to sleep" is an intention, not a sleep onset, and "awake" says
// nothing about being free to answer. It is defined and validated once, here,
// for the server's sync validation, the desktop's store and the shared page.
package presence

import (
	"errors"
	"regexp"
	"sort"
	"time"
)

// What the owner may report.
const (
	Awake         = "awake"
	TryingToSleep = "trying_to_sleep"
	DoNotDisturb  = "do_not_disturb"
	// Cleared takes back whatever was reported before it. It has no expiry
	// and nothing is shown for it.
	Cleared = "cleared"
)

// MaxLifetime is the longest a report may stand. A status nobody renewed for a
// day is not a report about now.
const MaxLifetime = 24 * time.Hour

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)

// Report is one status the owner reported. Reports are immutable: a new one
// replaces the last, and Cleared takes it back.
type Report struct {
	StatusID   string    `json:"status_id"`
	Status     string    `json:"status"`
	ReportedAt time.Time `json:"reported_at"`
	// ExpiresAt is when the report stops standing; absent for Cleared.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Shared says whether a share link whose owner allowed it may show this
	// report. A report kept private still syncs to the owner's own devices.
	Shared bool `json:"shared"`
}

// ValidStatus reports whether value is one the owner may report.
func ValidStatus(value string) bool {
	switch value {
	case Awake, TryingToSleep, DoNotDisturb, Cleared:
		return true
	default:
		return false
	}
}

// DefaultLifetime is how long a report stands when the owner does not say:
// long enough to cover what it describes, short enough that a forgotten one
// lapses before it misleads.
func DefaultLifetime(status string) time.Duration {
	switch status {
	case TryingToSleep:
		return 10 * time.Hour
	case DoNotDisturb:
		return 2 * time.Hour
	default:
		return 4 * time.Hour
	}
}

func (report Report) Validate() error {
	if !identifier.MatchString(report.StatusID) {
		return errors.New("status_id must match the v1 identifier format")
	}
	if !ValidStatus(report.Status) {
		return errors.New("status must be awake, trying_to_sleep, do_not_disturb, or cleared")
	}
	if report.ReportedAt.IsZero() {
		return errors.New("reported_at is required")
	}
	if report.Status == Cleared {
		if report.ExpiresAt != nil {
			return errors.New("a cleared status has no expiry")
		}
		if report.Shared {
			return errors.New("a cleared status shares nothing")
		}
		return nil
	}
	if report.ExpiresAt == nil {
		return errors.New("expires_at is required")
	}
	if !report.ExpiresAt.After(report.ReportedAt) {
		return errors.New("expires_at must be after reported_at")
	}
	if report.ExpiresAt.Sub(report.ReportedAt) > MaxLifetime {
		return errors.New("a status can stand for at most 24 hours")
	}
	return nil
}

// Current is the report that stands at now: the newest one, unless it was
// cleared, has expired, or claims to have been made in the future. A report
// from a clock that runs ahead is not believed early.
func Current(reports []Report, now time.Time) (Report, bool) {
	if len(reports) == 0 {
		return Report{}, false
	}
	ordered := append([]Report(nil), reports...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].ReportedAt.Equal(ordered[j].ReportedAt) {
			return ordered[i].StatusID < ordered[j].StatusID
		}
		return ordered[i].ReportedAt.Before(ordered[j].ReportedAt)
	})
	newest := ordered[len(ordered)-1]
	if newest.Status == Cleared || newest.ExpiresAt == nil {
		return Report{}, false
	}
	if newest.ReportedAt.After(now) || !newest.ExpiresAt.After(now) {
		return Report{}, false
	}
	return newest, true
}
