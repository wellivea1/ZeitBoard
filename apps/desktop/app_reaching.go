package main

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"non24.app/core/outlook"
)

// Reaching hours: when the people this person needs to reach are available.
//
// The outlook has always intersected predicted waking time with a set of office
// hours, and until now those hours were Monday to Friday, 09:00 to 17:00, in the
// user's own zone, with no way to say otherwise. For a great many people with a
// drifting rhythm that is simply false — the pharmacy is open until eight, the
// clinic runs Tuesday and Thursday, the family who answer the phone are six
// time zones away — and the "reachable for three hours" figure derived from it
// was false with them.
//
// Two properties matter more than the flexibility. The hours belong to the
// other party, not the user, so the zone is theirs. And the setting is a
// statement the user made rather than something the app inferred, so the
// surface says so instead of presenting it as a fact about the world.

const (
	reachingFileName = "reaching-hours.json"
	reachingSchema   = "v1"
)

// ReachingHoursDTO is the stored schedule.
type ReachingHoursDTO struct {
	// Enabled is false when the person has no schedule worth showing. The
	// outlook then reports no reaching windows at all, which is more honest
	// than a default nobody chose.
	Enabled bool `json:"enabled"`

	// Label names whose hours these are, for the person's own benefit.
	Label string `json:"label"`

	// StartLocal and EndLocal are "HH:MM" in ZoneID. An end at or before the
	// start crosses midnight; equal times mean the whole day.
	StartLocal string `json:"startLocal"`
	EndLocal   string `json:"endLocal"`

	// Days are weekday numbers, Sunday 0 through Saturday 6.
	Days []int `json:"days"`

	// ZoneID is the other party's zone, which need not be the user's.
	ZoneID string `json:"zoneId"`
}

type ReachingHoursEnvelopeDTO struct {
	State    ReachingHoursDTO `json:"state"`
	Revision uint64           `json:"revision"`
	Conflict bool             `json:"conflict"`

	// Summary is the sentence the outlook shows.
	Summary string `json:"summary"`

	// Message reports a storage problem without pretending the setting saved.
	Message string `json:"message,omitempty"`
}

type ReachingHoursSaveInput struct {
	State        ReachingHoursDTO `json:"state"`
	BaseRevision uint64           `json:"baseRevision"`
}

func defaultReachingHours(zoneID string) ReachingHoursDTO {
	defaults := outlook.DefaultOfficeHours(zoneID)
	days := make([]int, 0, len(defaults.Days))
	for _, day := range defaults.Days {
		days = append(days, int(day))
	}
	return ReachingHoursDTO{
		Enabled:    true,
		Label:      "Typical office hours",
		StartLocal: defaults.StartLocal,
		EndLocal:   defaults.EndLocal,
		Days:       days,
		ZoneID:     defaults.ZoneID,
	}
}

func validateReachingHours(state ReachingHoursDTO) error {
	if len(strings.TrimSpace(state.Label)) > 60 {
		return errors.New("the label must be 60 characters or fewer")
	}
	if !civilClockPattern.MatchString(state.StartLocal) || state.StartLocal == "" {
		return errors.New("the opening time must be a civil time like 09:00")
	}
	if !civilClockPattern.MatchString(state.EndLocal) || state.EndLocal == "" {
		return errors.New("the closing time must be a civil time like 17:00")
	}
	if state.ZoneID == "" {
		return errors.New("a time zone is required")
	}
	if _, err := time.LoadLocation(state.ZoneID); err != nil {
		return errors.New("that time zone is not one this computer knows")
	}
	seen := make(map[int]bool, len(state.Days))
	for _, day := range state.Days {
		if day < int(time.Sunday) || day > int(time.Saturday) {
			return errors.New("days must be Sunday through Saturday")
		}
		if seen[day] {
			return errors.New("a day was listed twice")
		}
		seen[day] = true
	}
	// An enabled schedule with no open day would report nothing while claiming
	// to be on. Turning it off is the way to say "nobody to reach".
	if state.Enabled && len(state.Days) == 0 {
		return errors.New("choose at least one day, or turn reaching hours off")
	}
	return nil
}

func reachingStore(path string) localSettingsStore[ReachingHoursDTO] {
	return localSettingsStore[ReachingHoursDTO]{
		Path:     path,
		Schema:   reachingSchema,
		Name:     "reaching-hours",
		Validate: validateReachingHours,
	}
}

// GetReachingHours returns the stored schedule, or the default when none has
// been saved.
func (a *App) GetReachingHours() (ReachingHoursEnvelopeDTO, error) {
	a.reachingMu.RLock()
	defer a.reachingMu.RUnlock()
	return a.reachingEnvelopeLocked(false), nil
}

// SaveReachingHours stores a schedule the person chose. A stale base revision
// loses to whatever is stored rather than overwriting it.
func (a *App) SaveReachingHours(input ReachingHoursSaveInput) (ReachingHoursEnvelopeDTO, error) {
	state := input.State
	state.Label = strings.TrimSpace(state.Label)
	if state.ZoneID == "" {
		state.ZoneID = localZoneID()
	}
	sort.Ints(state.Days)
	if err := validateReachingHours(state); err != nil {
		return ReachingHoursEnvelopeDTO{}, err
	}

	a.reachingMu.Lock()
	defer a.reachingMu.Unlock()
	if input.BaseRevision != a.reachingRevision {
		return a.reachingEnvelopeLocked(true), nil
	}
	next := a.reachingRevision + 1
	if err := a.persistReachingLocked(state, next); err != nil {
		a.reachingErr = "Reaching hours could not be stored, so the previous schedule is still in use."
		return a.reachingEnvelopeLocked(false), errors.New("reaching hours could not be stored")
	}
	a.reaching = state
	a.reachingRevision = next
	a.reachingErr = ""
	return a.reachingEnvelopeLocked(false), nil
}

func (a *App) reachingEnvelopeLocked(conflict bool) ReachingHoursEnvelopeDTO {
	return ReachingHoursEnvelopeDTO{
		State:    a.reaching,
		Revision: a.reachingRevision,
		Conflict: conflict,
		Summary:  reachingSummary(a.reaching),
		Message:  a.reachingErr,
	}
}

func (a *App) currentReachingHours() ReachingHoursDTO {
	a.reachingMu.RLock()
	defer a.reachingMu.RUnlock()
	return a.reaching
}

// officeHours converts the stored schedule into what core/outlook consumes. A
// disabled schedule yields no open days, so no window is produced.
func (state ReachingHoursDTO) officeHours(fallbackZoneID string) outlook.OfficeHours {
	zoneID := state.ZoneID
	if zoneID == "" {
		zoneID = fallbackZoneID
	}
	hours := outlook.OfficeHours{
		StartLocal: state.StartLocal,
		EndLocal:   state.EndLocal,
		ZoneID:     zoneID,
		Days:       []time.Weekday{},
	}
	if !state.Enabled {
		return hours
	}
	for _, day := range state.Days {
		hours.Days = append(hours.Days, time.Weekday(day))
	}
	return hours
}

func (a *App) loadReachingFromDisk() error {
	if a.configDir == "" {
		return nil
	}
	path := filepath.Join(a.configDir, reachingFileName)
	store := reachingStore(path)
	stored, revision, found, recovered, err := store.read()
	if err != nil || !found {
		return err
	}
	if recovered {
		if err := store.restorePrimary(stored, revision); err != nil {
			return err
		}
	}
	a.reachingMu.Lock()
	a.reaching = stored
	a.reachingRevision = revision
	a.reachingErr = ""
	a.reachingMu.Unlock()
	return nil
}

func (a *App) persistReachingLocked(state ReachingHoursDTO, revision uint64) error {
	if a.configDir == "" {
		return nil
	}
	return reachingStore(filepath.Join(a.configDir, reachingFileName)).write(state, revision)
}

var weekdayNames = [...]string{
	"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
}

// reachingSummary describes the schedule in the reader's words. It never states
// the hours as a fact about the world: they are what this person recorded, and
// saying so is the difference between a setting and a claim.
func reachingSummary(state ReachingHoursDTO) string {
	if !state.Enabled || len(state.Days) == 0 {
		return "No reaching hours are set, so the outlook does not say when anyone is available."
	}
	label := state.Label
	if label == "" {
		label = "Reaching hours"
	}
	parts := []string{label, "you set:", reachingDayPhrase(state.Days)}
	parts = append(parts, reachingClockPhrase(state.StartLocal, state.EndLocal))
	summary := strings.Join(parts, " ")
	if zone := reachingZonePhrase(state.ZoneID); zone != "" {
		summary += " " + zone
	}
	return summary + "."
}

func reachingDayPhrase(days []int) string {
	sorted := append([]int(nil), days...)
	sort.Ints(sorted)
	if len(sorted) == 7 {
		return "every day,"
	}
	names := make([]string, 0, len(sorted))
	for _, day := range sorted {
		if day >= 0 && day < len(weekdayNames) {
			names = append(names, weekdayNames[day])
		}
	}
	// A contiguous run reads as a range; anything else is listed, because
	// "Monday, Thursday" is a real pattern and compressing it would lie.
	if len(sorted) > 2 && sorted[len(sorted)-1]-sorted[0] == len(sorted)-1 {
		return names[0] + " to " + names[len(names)-1] + ","
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0] + ","
	case 2:
		return names[0] + " and " + names[1] + ","
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1] + ","
	}
}

func reachingClockPhrase(startLocal, endLocal string) string {
	start := formatCivilClock(startLocal)
	end := formatCivilClock(endLocal)
	if startLocal == endLocal {
		return "all day"
	}
	phrase := start + " to " + end
	if !civilClockIsAfter(endLocal, startLocal) {
		// Otherwise "10:00 PM to 6:00 AM" reads as a sixteen-hour mistake.
		phrase += " the next day"
	}
	return phrase
}

func reachingZonePhrase(zoneID string) string {
	if zoneID == "" || zoneID == localZoneID() {
		return ""
	}
	return "(" + zoneID + " time)"
}

func formatCivilClock(value string) string {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return value
	}
	return parsed.Format("3:04 PM")
}

func civilClockIsAfter(a, b string) bool {
	first, errA := time.Parse("15:04", a)
	second, errB := time.Parse("15:04", b)
	if errA != nil || errB != nil {
		return true
	}
	return first.After(second)
}
