package main

import (
	"errors"
	"path/filepath"
	"time"

	"non24.app/core/presence"
)

// A reported status (ADR-0070): what the owner says about the present.
// "I am awake", "I am trying to sleep" and "do not disturb" are the owner's
// word, with when it was said and when it stops standing. They sync to the
// owner's server and other devices, and a share link shows one only as far as
// the owner chose: for the link, and for each report.
//
// Nothing here moves the forecast. "Trying to sleep" is an intention and is
// never recorded as a sleep.

const (
	statusSettingsFileName = "status.json"
	statusSettingsSchema   = "status-settings-v1"
)

// StatusSettingsDTO is what the owner chose about reporting, on this computer.
// Both start off: a report is kept to the owner's own devices, and a tap on
// the sleep buttons records sleep and says nothing to anyone.
type StatusSettingsDTO struct {
	// ShareByDefault is whether a new report may be shown by the share links
	// the owner allowed. Each report can still be kept private or shared.
	ShareByDefault bool `json:"shareByDefault"`
	// FollowSleepTaps reports "trying to sleep" with "I am going to sleep"
	// and "awake" with "I woke up", so the owner does not say it twice.
	FollowSleepTaps bool `json:"followSleepTaps"`
}

func statusSettingsStore(path string) localSettingsStore[StatusSettingsDTO] {
	return localSettingsStore[StatusSettingsDTO]{Path: path, Schema: statusSettingsSchema, Name: "status"}
}

type ReportedStatusDTO struct {
	// Status is "ok", or "unavailable" when the desktop cannot keep a report.
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`

	// Reported is what stands now: "", "awake", "trying_to_sleep" or
	// "do_not_disturb".
	Reported      string `json:"reported"`
	ReportedLabel string `json:"reportedLabel,omitempty"`
	ReportedAt    string `json:"reportedAt,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
	// SaidLabel and UntilLabel are the two times in words.
	SaidLabel  string `json:"saidLabel,omitempty"`
	UntilLabel string `json:"untilLabel,omitempty"`
	// Shared is whether share links the owner allowed may show this report.
	Shared bool `json:"shared"`
	// Delivery says where the report has got to: "this_computer" while sync
	// is off, "waiting" until the owner's server has it, then "server".
	Delivery string `json:"delivery"`

	Settings         StatusSettingsDTO `json:"settings"`
	SettingsRevision uint64            `json:"settingsRevision"`
}

type ReportStatusInput struct {
	Status string `json:"status"`
	// Minutes is how long the report stands; zero takes the status's usual
	// length.
	Minutes int `json:"minutes"`
	// Shared, when given, overrides the default for this one report.
	Shared *bool `json:"shared,omitempty"`
}

type StatusSettingsInput struct {
	BaseRevision    uint64 `json:"baseRevision"`
	ShareByDefault  bool   `json:"shareByDefault"`
	FollowSleepTaps bool   `json:"followSleepTaps"`
}

func statusWords(status string) string {
	switch status {
	case presence.Awake:
		return "Awake"
	case presence.TryingToSleep:
		return "Trying to sleep"
	case presence.DoNotDisturb:
		return "Do not disturb"
	default:
		return ""
	}
}

// loadStatusSettings reads the owner's choices. With nothing stored, or
// nothing readable, both are off: the choice that shares least.
func (a *App) loadStatusSettings() (StatusSettingsDTO, uint64) {
	if a.configDir == "" {
		return a.statusSettings, a.statusRevision
	}
	stored, revision, found, _, err := statusSettingsStore(filepath.Join(a.configDir, statusSettingsFileName)).read()
	if err != nil || !found {
		return StatusSettingsDTO{}, 0
	}
	return stored, revision
}

func (a *App) GetReportedStatus() (ReportedStatusDTO, error) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	return a.reportedStatusLocked()
}

func (a *App) reportedStatusLocked() (ReportedStatusDTO, error) {
	settings, revision := a.loadStatusSettings()
	dto := ReportedStatusDTO{Status: "ok", Delivery: "this_computer", Settings: settings, SettingsRevision: revision}
	store, err := a.requireStore()
	if err != nil {
		dto.Status, dto.Message = "unavailable", "A status is reported from the ZeitBoard desktop app."
		return dto, nil
	}
	ctx := a.applicationContext()
	reports, err := store.StatusReports(ctx)
	if err != nil {
		return ReportedStatusDTO{}, err
	}
	now := a.currentTime().UTC()
	current, ok := presence.Current(reports, now)
	if !ok {
		return dto, nil
	}
	location := time.Local
	dto.Reported = current.Status
	dto.ReportedLabel = statusWords(current.Status)
	dto.ReportedAt = current.ReportedAt.UTC().Format(time.RFC3339)
	dto.ExpiresAt = current.ExpiresAt.UTC().Format(time.RFC3339)
	dto.SaidLabel = displayClock().Format(current.ReportedAt.In(location), "Mon 3:04 PM")
	dto.UntilLabel = displayClock().Format(current.ExpiresAt.In(location), "Mon 3:04 PM")
	dto.Shared = current.Shared
	if _, _, syncErr := a.requireBackendSync(); syncErr == nil {
		dto.Delivery = "waiting"
		if uploaded, err := store.StatusReportUploaded(ctx, current.StatusID); err == nil && uploaded {
			dto.Delivery = "server"
		}
	}
	return dto, nil
}

// ReportStatus keeps what the owner says about now, and sends it on.
func (a *App) ReportStatus(input ReportStatusInput) (ReportedStatusDTO, error) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	if input.Status == presence.Cleared || !presence.ValidStatus(input.Status) {
		return ReportedStatusDTO{}, errors.New("Choose awake, trying to sleep, or do not disturb.")
	}
	lifetime := presence.DefaultLifetime(input.Status)
	if input.Minutes != 0 {
		lifetime = time.Duration(input.Minutes) * time.Minute
	}
	if lifetime < 5*time.Minute || lifetime > presence.MaxLifetime {
		return ReportedStatusDTO{}, errors.New("A status can stand for between five minutes and a day.")
	}
	settings, _ := a.loadStatusSettings()
	shared := settings.ShareByDefault
	if input.Shared != nil {
		shared = *input.Shared
	}
	if err := a.keepStatus(input.Status, lifetime, shared); err != nil {
		return ReportedStatusDTO{}, err
	}
	return a.reportedStatusLocked()
}

// ClearReportedStatus takes back whatever was reported.
func (a *App) ClearReportedStatus() (ReportedStatusDTO, error) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	if err := a.keepStatus(presence.Cleared, 0, false); err != nil {
		return ReportedStatusDTO{}, err
	}
	return a.reportedStatusLocked()
}

func (a *App) keepStatus(status string, lifetime time.Duration, shared bool) error {
	store, err := a.requireStore()
	if err != nil {
		return err
	}
	ctx := a.applicationContext()
	now := a.currentTime().UTC().Truncate(time.Millisecond)
	// The newest report stands, so a report made here is always later than
	// the ones already kept: two in the same instant, as a report and its
	// taking back can be, would otherwise be ordered by their ids.
	kept, err := store.StatusReports(ctx)
	if err != nil {
		return err
	}
	for _, earlier := range kept {
		if !now.After(earlier.ReportedAt) {
			now = earlier.ReportedAt.UTC().Add(time.Millisecond)
		}
	}
	report := presence.Report{StatusID: newLocalID("status"), Status: status, ReportedAt: now, Shared: shared}
	if status != presence.Cleared {
		expires := now.Add(lifetime)
		report.ExpiresAt = &expires
	}
	if err := store.AddStatusReport(ctx, report, now); err != nil {
		return asSentence(err)
	}
	a.wakeDesktopBackground()
	return nil
}

// followSleepTap reports the status a tap on the sleep buttons implies, when
// the owner asked for that. A report that cannot be kept never fails the tap:
// the night is what the tap is for.
func (a *App) followSleepTap(status string) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	settings, _ := a.loadStatusSettings()
	if !settings.FollowSleepTaps {
		return
	}
	_ = a.keepStatus(status, presence.DefaultLifetime(status), settings.ShareByDefault)
}

// SaveStatusSettings stores the owner's choices about reporting. A stale base
// revision loses to what is stored, as the other settings files do.
func (a *App) SaveStatusSettings(input StatusSettingsInput) (ReportedStatusDTO, error) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	_, revision := a.loadStatusSettings()
	if input.BaseRevision != revision {
		return a.reportedStatusLocked()
	}
	state := StatusSettingsDTO{ShareByDefault: input.ShareByDefault, FollowSleepTaps: input.FollowSleepTaps}
	if a.configDir != "" {
		if err := statusSettingsStore(filepath.Join(a.configDir, statusSettingsFileName)).write(state, revision+1); err != nil {
			return ReportedStatusDTO{}, errors.New("Your choice could not be stored, so the previous one is still in use.")
		}
	} else {
		a.statusSettings, a.statusRevision = state, revision+1
	}
	return a.reportedStatusLocked()
}
