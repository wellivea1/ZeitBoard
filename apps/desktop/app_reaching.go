package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"non24.app/core/outlook"
)

// Reaching hours: when the people this person needs to reach are available.
//
// The outlook has always intersected predicted waking time with a set of office
// hours, and at first those were Monday to Friday, 09:00 to 17:00, in the
// user's own zone, with no way to say otherwise. For a great many people with a
// drifting rhythm that is simply false — the pharmacy is open until eight, the
// clinic runs Tuesday mornings and Thursday afternoons, the family who answer
// the phone are six time zones away — and the "reachable for three hours"
// figure derived from it was false with them. So the setting is a short list of
// people, each with their own zone and their own hours on each weekday
// (ADR-0056).
//
// Two properties matter more than the flexibility. The hours belong to the
// other party, not the user, so the zone is theirs. And the setting is a
// statement the user made rather than something the app inferred, so the
// surface says so instead of presenting it as a fact about the world.

const (
	reachingFileName = "reaching.json"
	reachingSchema   = "v2"

	// legacyReachingFileName held the single schedule kept before ADR-0056. It
	// is read once, to carry that schedule over, and removed once the list has
	// been written in its place.
	legacyReachingFileName = "reaching-hours.json"
	legacyReachingSchema   = "v1"

	// maxReachingContacts is a display limit, not a data one: Home draws a row
	// for each person shown.
	maxReachingContacts = 5
	maxReachingLabel    = 60
)

var reachingContactIDPattern = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// ReachingHoursDTO is the list of people to reach, in the order the owner
// arranged them. An empty list means nobody to reach, and the outlook then
// says nothing about reaching anyone.
type ReachingHoursDTO struct {
	Contacts []ReachingContactDTO `json:"contacts"`
}

// ReachingContactDTO is one person, or place, the owner needs to reach.
type ReachingContactDTO struct {
	// ID is stable across edits and never shown. The assistant snapshot does
	// not use it either: it names people by their place in the list.
	ID string `json:"id"`

	// Label names whose hours these are, for the owner's own benefit. It stays
	// on this computer.
	Label string `json:"label"`

	// ZoneID is their zone, which need not be the owner's.
	ZoneID string `json:"zoneId"`

	// Off keeps the hours without showing them, for someone the owner does not
	// need to reach for now.
	Off bool `json:"off"`

	// Hours are the weekdays they are available, each at most once.
	Hours []ReachingDayHoursDTO `json:"hours"`
}

// ReachingDayHoursDTO is when someone is available on one weekday.
type ReachingDayHoursDTO struct {
	// Day is the weekday number, Sunday 0 through Saturday 6. Hours that run
	// past midnight belong to the day they open.
	Day int `json:"day"`

	// StartLocal and EndLocal are "HH:MM" in the person's zone. An end at or
	// before the start crosses midnight; equal times mean the whole day.
	StartLocal string `json:"startLocal"`
	EndLocal   string `json:"endLocal"`
}

type ReachingHoursEnvelopeDTO struct {
	State    ReachingHoursDTO `json:"state"`
	Revision uint64           `json:"revision"`
	Conflict bool             `json:"conflict"`

	// Message reports a storage problem without pretending the setting saved.
	Message string `json:"message,omitempty"`
}

type ReachingHoursSaveInput struct {
	State        ReachingHoursDTO `json:"state"`
	BaseRevision uint64           `json:"baseRevision"`
}

// defaultReachingHours is what a new install shows: the working week the
// outlook has always assumed, in the owner's own zone, as one person they can
// rename, change, switch off or remove.
func defaultReachingHours(zoneID string) ReachingHoursDTO {
	defaults := outlook.DefaultOfficeHours(zoneID)
	contact := ReachingContactDTO{
		ID:     newReachingContactID(),
		Label:  "Typical office hours",
		ZoneID: defaults.ZoneID,
		Hours:  make([]ReachingDayHoursDTO, 0, len(defaults.Open)),
	}
	for _, day := range defaults.Open {
		contact.Hours = append(contact.Hours, ReachingDayHoursDTO{
			Day: int(day.Day), StartLocal: day.StartLocal, EndLocal: day.EndLocal,
		})
	}
	return ReachingHoursDTO{Contacts: []ReachingContactDTO{contact}}
}

// newReachingContactID names a new person for the app's own bookkeeping.
func newReachingContactID() string {
	var raw [5]byte
	_, _ = rand.Read(raw[:]) // crypto/rand.Read does not fail
	return "contact-" + hex.EncodeToString(raw[:])
}

// tidyReachingHours puts a submitted list into its stored form: names trimmed,
// a missing zone taken as the owner's, a new person given an id, and each
// person's days in order.
func tidyReachingHours(state ReachingHoursDTO) ReachingHoursDTO {
	tidy := ReachingHoursDTO{Contacts: make([]ReachingContactDTO, 0, len(state.Contacts))}
	for _, contact := range state.Contacts {
		contact.Label = strings.TrimSpace(contact.Label)
		if contact.ZoneID == "" {
			contact.ZoneID = localZoneID()
		}
		if contact.ID == "" {
			contact.ID = newReachingContactID()
		}
		hours := append([]ReachingDayHoursDTO{}, contact.Hours...)
		sort.SliceStable(hours, func(i, j int) bool { return hours[i].Day < hours[j].Day })
		contact.Hours = hours
		tidy.Contacts = append(tidy.Contacts, contact)
	}
	return tidy
}

func validateReachingHours(state ReachingHoursDTO) error {
	if len(state.Contacts) > maxReachingContacts {
		return fmt.Errorf("home has room for %d people; remove someone first", maxReachingContacts)
	}
	ids := make(map[string]bool, len(state.Contacts))
	names := make(map[string]bool, len(state.Contacts))
	for _, contact := range state.Contacts {
		if !reachingContactIDPattern.MatchString(contact.ID) || ids[contact.ID] {
			return errors.New("each person needs an id of their own")
		}
		ids[contact.ID] = true
		if err := validateReachingContact(contact); err != nil {
			return err
		}
		// Two rows with one name would leave Home unable to say which is which.
		name := strings.ToLower(contact.Label)
		if names[name] {
			return fmt.Errorf("two people are called %s; give them different names so Home can tell them apart", contact.Label)
		}
		names[name] = true
	}
	return nil
}

func validateReachingContact(contact ReachingContactDTO) error {
	label := contact.Label
	if strings.TrimSpace(label) == "" {
		return errors.New("give each person a name, so Home can tell them apart")
	}
	if utf8.RuneCountInString(label) > maxReachingLabel {
		return fmt.Errorf("keep each name to %d characters or fewer", maxReachingLabel)
	}
	if contact.ZoneID == "" {
		return fmt.Errorf("choose a time zone for %s", label)
	}
	if _, err := time.LoadLocation(contact.ZoneID); err != nil {
		return fmt.Errorf("the time zone for %s is not one this computer knows", label)
	}
	seen := make(map[int]bool, len(contact.Hours))
	for _, hours := range contact.Hours {
		if hours.Day < int(time.Sunday) || hours.Day > int(time.Saturday) {
			return errors.New("days must be Sunday through Saturday")
		}
		day := weekdayNames[hours.Day]
		if seen[hours.Day] {
			return fmt.Errorf("%s is listed twice for %s", day, label)
		}
		seen[hours.Day] = true
		if hours.StartLocal == "" || !civilClockPattern.MatchString(hours.StartLocal) {
			return fmt.Errorf("the opening time for %s on %s must be a civil time like 09:00", label, day)
		}
		if hours.EndLocal == "" || !civilClockPattern.MatchString(hours.EndLocal) {
			return fmt.Errorf("the closing time for %s on %s must be a civil time like 17:00", label, day)
		}
	}
	// Someone shown with no open day would leave a row that never fills.
	// Switching them off keeps the hours without showing them.
	if !contact.Off && len(contact.Hours) == 0 {
		return fmt.Errorf("choose at least one day for %s, or switch them off", label)
	}
	return nil
}

func reachingStore(path string) localSettingsStore[ReachingHoursDTO] {
	return localSettingsStore[ReachingHoursDTO]{
		Path:     path,
		Schema:   reachingSchema,
		Name:     "reaching",
		Validate: validateReachingHours,
	}
}

// GetReachingHours returns the stored list, or the default when none has been
// saved.
func (a *App) GetReachingHours() (ReachingHoursEnvelopeDTO, error) {
	a.reachingMu.RLock()
	defer a.reachingMu.RUnlock()
	return a.reachingEnvelopeLocked(false), nil
}

// SaveReachingHours stores the list the person arranged. A stale base revision
// loses to whatever is stored rather than overwriting it.
func (a *App) SaveReachingHours(input ReachingHoursSaveInput) (ReachingHoursEnvelopeDTO, error) {
	state := tidyReachingHours(input.State)
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
		a.reachingErr = "Reaching hours could not be stored, so the previous list is still in use."
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
		Message:  a.reachingErr,
	}
}

func (a *App) currentReachingHours() ReachingHoursDTO {
	a.reachingMu.RLock()
	defer a.reachingMu.RUnlock()
	return a.reaching
}

// shown is everyone Home draws, in list order.
func (state ReachingHoursDTO) shown() []ReachingContactDTO {
	shown := make([]ReachingContactDTO, 0, len(state.Contacts))
	for _, contact := range state.Contacts {
		if !contact.Off {
			shown = append(shown, contact)
		}
	}
	return shown
}

// officeHours is everyone shown as core/outlook reads them, each schedule
// carrying the person's id. Someone switched off is left out, so produces no
// window, and an empty list produces none at all.
func (state ReachingHoursDTO) officeHours(fallbackZoneID string) []outlook.OfficeHours {
	shown := state.shown()
	schedules := make([]outlook.OfficeHours, 0, len(shown))
	for _, contact := range shown {
		zoneID := contact.ZoneID
		if zoneID == "" {
			zoneID = fallbackZoneID
		}
		hours := outlook.OfficeHours{
			ID: contact.ID, ZoneID: zoneID, Open: make([]outlook.DayHours, 0, len(contact.Hours)),
		}
		for _, day := range contact.Hours {
			hours.Open = append(hours.Open, outlook.DayHours{
				Day: time.Weekday(day.Day), StartLocal: day.StartLocal, EndLocal: day.EndLocal,
			})
		}
		schedules = append(schedules, hours)
	}
	return schedules
}

// contactRefs names each person as the assistant snapshot does: by place in
// the list, never by label (ADR-0056).
func (state ReachingHoursDTO) contactRefs() map[string]string {
	refs := make(map[string]string, len(state.Contacts))
	for index, contact := range state.Contacts {
		refs[contact.ID] = fmt.Sprintf("contact_%02d", index+1)
	}
	return refs
}

func (a *App) loadReachingFromDisk() error {
	if a.configDir == "" {
		return nil
	}
	store := reachingStore(filepath.Join(a.configDir, reachingFileName))
	stored, revision, found, recovered, err := store.read()
	if err != nil {
		return err
	}
	if !found {
		stored, revision, found, err = a.migrateLegacyReaching()
		if err != nil || !found {
			return err
		}
	} else if recovered {
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

// migrateLegacyReaching carries the single schedule kept before ADR-0056 over
// as one person. The list is written before the old file is removed, so an
// interruption leaves one or the other and never neither. If the list cannot
// be written the hours are used all the same, and the move is tried again at
// the next start.
func (a *App) migrateLegacyReaching() (ReachingHoursDTO, uint64, bool, error) {
	legacyPath := filepath.Join(a.configDir, legacyReachingFileName)
	legacy, revision, found, _, err := legacyReachingStore(legacyPath).read()
	if err != nil || !found {
		return ReachingHoursDTO{}, 0, false, err
	}
	state := ReachingHoursDTO{Contacts: []ReachingContactDTO{legacy.contact()}}
	if err := reachingStore(filepath.Join(a.configDir, reachingFileName)).write(state, revision); err == nil {
		for _, path := range []string{legacyPath, legacyPath + ".bak"} {
			_ = os.Remove(path)
		}
	}
	return state, revision, true, nil
}

func (a *App) persistReachingLocked(state ReachingHoursDTO, revision uint64) error {
	if a.configDir == "" {
		return nil
	}
	return reachingStore(filepath.Join(a.configDir, reachingFileName)).write(state, revision)
}

// legacyReachingHours is the single schedule kept before ADR-0056: the same
// hours on each chosen day, with one switch for the whole of it.
type legacyReachingHours struct {
	Enabled    bool   `json:"enabled"`
	Label      string `json:"label"`
	StartLocal string `json:"startLocal"`
	EndLocal   string `json:"endLocal"`
	Days       []int  `json:"days"`
	ZoneID     string `json:"zoneId"`
}

// legacyReachingStore reads the old file. Its rules were a subset of a
// person's, so a schedule that becomes a valid person was a valid schedule.
func legacyReachingStore(path string) localSettingsStore[legacyReachingHours] {
	return localSettingsStore[legacyReachingHours]{
		Path:   path,
		Schema: legacyReachingSchema,
		Name:   "reaching-hours",
		Validate: func(legacy legacyReachingHours) error {
			return validateReachingContact(legacy.contact())
		},
	}
}

// contact is the old schedule as one person, with the same hours on each of
// its days. A schedule that was off becomes a person who is off, so no hours
// are lost.
func (legacy legacyReachingHours) contact() ReachingContactDTO {
	label := strings.TrimSpace(legacy.Label)
	if label == "" {
		label = "Reaching hours"
	}
	days := append([]int(nil), legacy.Days...)
	sort.Ints(days)
	contact := ReachingContactDTO{
		ID:     newReachingContactID(),
		Label:  label,
		ZoneID: legacy.ZoneID,
		Off:    !legacy.Enabled,
		Hours:  make([]ReachingDayHoursDTO, 0, len(days)),
	}
	for _, day := range days {
		contact.Hours = append(contact.Hours, ReachingDayHoursDTO{
			Day: day, StartLocal: legacy.StartLocal, EndLocal: legacy.EndLocal,
		})
	}
	return contact
}

var weekdayNames = [...]string{
	"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
}

// mondayFirst is the week as the summary reads it, so the working week and the
// weekend each read as one run.
var mondayFirst = [...]int{1, 2, 3, 4, 5, 6, 0}

// reachingSummary is what Home says about one person's hours. It never states
// them as a fact about the world: they are what the owner recorded, and saying
// so is the difference between a setting and a claim.
func reachingSummary(contact ReachingContactDTO) string {
	if len(contact.Hours) == 0 {
		return contact.Label + ": no hours are set."
	}
	summary := contact.Label + ": you set " + reachingHoursPhrase(contact.Hours)
	if zone := reachingZonePhrase(contact.ZoneID); zone != "" {
		summary += " " + zone
	}
	return summary + "."
}

// reachingHoursPhrase reads a week of hours in week order. Days with the same
// hours share a phrase: "Monday to Friday, 9:00 AM to 5:00 PM; Saturday,
// 10:00 AM to 1:00 PM".
func reachingHoursPhrase(hours []ReachingDayHoursDTO) string {
	type opening struct{ start, end string }
	byDay := make(map[int]opening, len(hours))
	for _, day := range hours {
		byDay[day.Day] = opening{day.StartLocal, day.EndLocal}
	}
	var order []opening
	days := make(map[opening][]int, len(hours))
	for _, day := range mondayFirst {
		open, isOpen := byDay[day]
		if !isOpen {
			continue
		}
		if _, seen := days[open]; !seen {
			order = append(order, open)
		}
		days[open] = append(days[open], day)
	}
	phrases := make([]string, 0, len(order))
	for _, open := range order {
		phrases = append(phrases, reachingDayPhrase(days[open])+", "+reachingClockPhrase(open.start, open.end))
	}
	return strings.Join(phrases, "; ")
}

// reachingDayPhrase names days given in week order.
func reachingDayPhrase(days []int) string {
	if len(days) == len(mondayFirst) {
		return "every day"
	}
	names := make([]string, 0, len(days))
	for _, day := range days {
		names = append(names, weekdayNames[day])
	}
	// A run of three or more reads as a range; anything else is listed, because
	// "Monday and Thursday" is a real pattern and compressing it would lie.
	if len(days) > 2 && weekPlace(days[len(days)-1])-weekPlace(days[0]) == len(days)-1 {
		return names[0] + " to " + names[len(names)-1]
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// weekPlace is a weekday's place in mondayFirst.
func weekPlace(day int) int {
	return (day + 6) % 7
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
