package main

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"non24.app/core/presence"
	storage "non24.app/core/storage/sqlite"
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
	at, err := a.nextStatusTime(ctx, store)
	if err != nil {
		return err
	}
	if err := store.AddStatusReport(ctx, statusReportAt(at, status, lifetime, shared), at); err != nil {
		return asSentence(err)
	}
	a.wakeDesktopBackground()
	return nil
}

// nextStatusTime is when a report made on this computer now is dated: later
// than every report already kept. The newest report stands, and two in the
// same instant, as a report and its taking back can be, would otherwise be
// ordered by their ids.
func (a *App) nextStatusTime(ctx context.Context, store *storage.Store) (time.Time, error) {
	now := a.currentTime().UTC().Truncate(time.Millisecond)
	kept, err := store.StatusReports(ctx)
	if err != nil {
		return time.Time{}, err
	}
	for _, earlier := range kept {
		if !now.After(earlier.ReportedAt) {
			now = earlier.ReportedAt.UTC().Add(time.Millisecond)
		}
	}
	return now, nil
}

// statusReportAt is a report made on this computer at a time.
func statusReportAt(at time.Time, status string, lifetime time.Duration, shared bool) presence.Report {
	report := presence.Report{StatusID: newLocalID("status"), Status: status, ReportedAt: at, Shared: shared}
	if status != presence.Cleared {
		expires := at.Add(lifetime)
		report.ExpiresAt = &expires
	}
	return report
}

// agentReportedStatusDTO is what an agent may read about the status the owner
// reports: the one that stands, and the owner's two choices about reporting.
// It is read on this computer's own endpoint and is not part of what the chat
// assistant's provider is sent.
type agentReportedStatusDTO struct {
	SchemaVersion string `json:"schema_version"`
	// Status is "none" when nothing stands.
	Status     string `json:"status"`
	ReportedAt string `json:"reported_at,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	// MayShowOnShareLinks is whether the standing report may be shown by the
	// links the owner allowed.
	MayShowOnShareLinks bool `json:"may_show_on_share_links"`
	// ShareByDefault and FollowSleepTaps are the owner's settings, read-only here.
	ShareByDefault  bool   `json:"share_by_default"`
	FollowSleepTaps bool   `json:"follow_sleep_taps"`
	Note            string `json:"note"`
}

func (a *App) agentReportedStatusProjection(ctx context.Context) (agentReportedStatusDTO, error) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	settings, _ := a.loadStatusSettings()
	dto := agentReportedStatusDTO{
		SchemaVersion: "v1", Status: "none",
		ShareByDefault: settings.ShareByDefault, FollowSleepTaps: settings.FollowSleepTaps,
		Note: "A status is the owner's own word about now, not a forecast and not a sleep record. " +
			"Only the owner reports or clears one; propose_report_status asks them to.",
	}
	store, err := a.requireStore()
	if err != nil {
		return dto, nil
	}
	reports, err := store.StatusReports(ctx)
	if err != nil {
		return agentReportedStatusDTO{}, err
	}
	if current, ok := presence.Current(reports, a.currentTime().UTC()); ok {
		dto.Status = current.Status
		dto.ReportedAt = current.ReportedAt.UTC().Format(time.RFC3339)
		dto.ExpiresAt = current.ExpiresAt.UTC().Format(time.RFC3339)
		dto.MayShowOnShareLinks = current.Shared
	}
	return dto, nil
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
	// Kept for the owner's other devices first (ADR-0074): a choice stored
	// here and not there would be undone by the next one to arrive.
	current, _ := a.loadStatusSettings()
	if err := a.sharedStatusChanges(a.applicationContext(), current, state); err != nil {
		return ReportedStatusDTO{}, errors.New("Your choice could not be stored, so the previous one is still in use.")
	}
	if a.configDir != "" {
		if err := statusSettingsStore(filepath.Join(a.configDir, statusSettingsFileName)).write(state, revision+1); err != nil {
			return ReportedStatusDTO{}, errors.New("Your choice could not be stored, so the previous one is still in use.")
		}
	} else {
		a.statusSettings, a.statusRevision = state, revision+1
	}
	return a.reportedStatusLocked()
}
