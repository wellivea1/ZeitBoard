package main

import (
	"errors"
	"path/filepath"
	"sync/atomic"

	"non24.app/core/civilfmt"
	"non24.app/desktop/platform/regional"
)

// The clock the desktop writes times on (ADR-0057): the one in Windows'
// regional format, unless the owner chose one in Settings › Display. Every
// label the desktop writes goes through it, and the window uses the same one.

const (
	regionalFileName = "regional.json"
	regionalSchema   = "v1"

	clockSystem = "system"
	clock12     = "12"
	clock24     = "24"
)

// RegionalSettingsDTO is what the owner chose.
type RegionalSettingsDTO struct {
	// Clock is "system", "12" or "24".
	Clock string `json:"clock"`
}

// ClockSettingDTO is the clock in use and why.
type ClockSettingDTO struct {
	// Preference is what the owner chose: "system", "12" or "24".
	Preference string `json:"preference"`

	// Clock is the clock in use, and System the computer's own: "12" or "24".
	Clock  string `json:"clock"`
	System string `json:"system"`

	Revision uint64 `json:"revision"`
	Conflict bool   `json:"conflict"`

	// Message reports a storage problem without pretending the setting saved.
	Message string `json:"message,omitempty"`
}

type ClockSettingInput struct {
	Preference   string `json:"preference"`
	BaseRevision uint64 `json:"baseRevision"`
}

// windowClock is the clock in use. It belongs to the process because the
// window does: there is one reader, and their labels are written in many
// places that have no App to ask.
var windowClock atomic.Uint32

// displayClock is the clock labels are written on.
func displayClock() civilfmt.Clock {
	return civilfmt.Clock(windowClock.Load())
}

func defaultRegionalSettings() RegionalSettingsDTO {
	return RegionalSettingsDTO{Clock: clockSystem}
}

// clock is the clock the settings choose, given the computer's own.
func (state RegionalSettingsDTO) clock(system civilfmt.Clock) civilfmt.Clock {
	switch state.Clock {
	case clock12:
		return civilfmt.TwelveHour
	case clock24:
		return civilfmt.TwentyFourHour
	default:
		return system
	}
}

func clockName(clock civilfmt.Clock) string {
	if clock == civilfmt.TwentyFourHour {
		return clock24
	}
	return clock12
}

func validateRegionalSettings(state RegionalSettingsDTO) error {
	switch state.Clock {
	case clockSystem, clock12, clock24:
		return nil
	}
	return errors.New("the clock must follow Windows, or be 12-hour or 24-hour")
}

func regionalStore(path string) localSettingsStore[RegionalSettingsDTO] {
	return localSettingsStore[RegionalSettingsDTO]{
		Path:     path,
		Schema:   regionalSchema,
		Name:     "regional",
		Validate: validateRegionalSettings,
	}
}

// systemClock is the clock in Windows' regional format.
func systemClock() civilfmt.Clock {
	if regional.TwentyFourHour() {
		return civilfmt.TwentyFourHour
	}
	return civilfmt.TwelveHour
}

// GetClockSetting says which clock times are written on, and why.
func (a *App) GetClockSetting() (ClockSettingDTO, error) {
	a.regionalMu.RLock()
	defer a.regionalMu.RUnlock()
	return a.clockSettingLocked(false), nil
}

// SaveClockSetting stores the clock the owner chose, and every label is written
// on it from then on. A stale base revision loses to what is stored.
func (a *App) SaveClockSetting(input ClockSettingInput) (ClockSettingDTO, error) {
	state := RegionalSettingsDTO{Clock: input.Preference}
	if err := validateRegionalSettings(state); err != nil {
		return ClockSettingDTO{}, err
	}
	a.regionalMu.Lock()
	defer a.regionalMu.Unlock()
	if input.BaseRevision != a.regionalRevision {
		return a.clockSettingLocked(true), nil
	}
	next := a.regionalRevision + 1
	if a.configDir != "" {
		if err := regionalStore(filepath.Join(a.configDir, regionalFileName)).write(state, next); err != nil {
			a.regionalErr = "The clock could not be stored, so the previous one is still in use."
			return a.clockSettingLocked(false), errors.New("the clock could not be stored")
		}
	}
	a.regional = state
	a.regionalRevision = next
	a.regionalErr = ""
	a.applyClockLocked()
	return a.clockSettingLocked(false), nil
}

func (a *App) clockSettingLocked(conflict bool) ClockSettingDTO {
	return ClockSettingDTO{
		Preference: a.regional.Clock,
		Clock:      clockName(a.regional.clock(a.systemClock)),
		System:     clockName(a.systemClock),
		Revision:   a.regionalRevision,
		Conflict:   conflict,
		Message:    a.regionalErr,
	}
}

func (a *App) applyClockLocked() {
	windowClock.Store(uint32(a.regional.clock(a.systemClock)))
}

// loadRegionalFromDisk reads what the owner chose, and puts the clock in use
// either way: with nothing stored, it is the computer's own.
func (a *App) loadRegionalFromDisk() error {
	a.regionalMu.Lock()
	defer a.regionalMu.Unlock()
	defer a.applyClockLocked()
	if a.configDir == "" {
		return nil
	}
	store := regionalStore(filepath.Join(a.configDir, regionalFileName))
	stored, revision, found, recovered, err := store.read()
	if err != nil || !found {
		return err
	}
	if recovered {
		if err := store.restorePrimary(stored, revision); err != nil {
			return err
		}
	}
	a.regional = stored
	a.regionalRevision = revision
	a.regionalErr = ""
	return nil
}
