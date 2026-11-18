package main

import (
	calendarcore "non24.app/core/calendar"
	storage "non24.app/core/storage/sqlite"
)

// What else there is to set up once Home draws a forecast. The first-run setup
// ends at the first forecast (C7). After it, nothing pointed at the phone
// companion, the people to reach, a calendar or starting with Windows, and
// each changes what Home can say. The window words each step and links to
// where it is done; this says only which steps are done.

const (
	setupStepPhone    = "phone"
	setupStepReaching = "reaching"
	setupStepCalendar = "calendar"
	setupStepStartup  = "startup"
)

type SetupGuideDTO struct {
	Steps []SetupStepDTO `json:"steps"`
}

// SetupStepDTO is one step and whether it is done.
type SetupStepDTO struct {
	ID   string `json:"id"`
	Done bool   `json:"done"`
}

// GetSetupGuide says which of the optional setup steps are done. A step this
// computer cannot take, login startup away from Windows, is left out.
func (a *App) GetSetupGuide() (SetupGuideDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SetupGuideDTO{}, err
	}
	ctx := a.applicationContext()
	// A phone counts once it has sent a night or said what it found; an older
	// companion sends nights without reports.
	reports, err := store.SourceReports(ctx)
	if err != nil {
		return SetupGuideDTO{}, err
	}
	phoneNight, err := store.NewestSleepEndFrom(ctx, storage.ProvenanceAcquisitionHealthConnect)
	if err != nil {
		return SetupGuideDTO{}, err
	}
	calendars, err := store.ListCalendarSources(ctx)
	if err != nil {
		return SetupGuideDTO{}, err
	}
	// The list counts once it has been saved: the working week a new install
	// starts with is a placeholder, not a choice.
	a.reachingMu.RLock()
	reachingSaved := a.reachingRevision > 0
	a.reachingMu.RUnlock()

	steps := []SetupStepDTO{
		{ID: setupStepPhone, Done: len(reports) > 0 || !phoneNight.IsZero()},
		{ID: setupStepReaching, Done: reachingSaved},
		{ID: setupStepCalendar, Done: ownCalendarCount(calendars) > 0},
	}
	if startup, err := a.GetStartupSettings(); err == nil && startup.Available {
		steps = append(steps, SetupStepDTO{
			ID: setupStepStartup, Done: startup.Registered && startup.MatchesCurrent,
		})
	}
	return SetupGuideDTO{Steps: steps}, nil
}

// ownCalendarCount counts the calendars the owner connected. ZeitBoard's own,
// the times accepted in Plan and the events added there, are not one: the
// review of 2026-10-28 found "Your calendar" ticked by accepting a time.
func ownCalendarCount(sources []storage.CalendarSourceRecord) int {
	count := 0
	for _, source := range sources {
		if source.Kind != calendarcore.SourceZeitBoard {
			count++
		}
	}
	return count
}
