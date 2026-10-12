package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"non24.app/core/inference"
	storage "non24.app/core/storage/sqlite"
	"non24.app/core/takeout"
)

// Activity from a Google export (ADR-0059). The person chooses a Takeout .zip
// or a My Activity file; ZeitBoard reads the minutes out of it, says what it
// read, and keeps them only when asked. Gaps in that activity become suggested
// nights, compared here with recorded ones. They reach nothing else: no
// estimate, plan, snapshot, link or projection reads them.

var openActivityExportDialog = runtime.OpenFileDialog

type AccountActivityPreviewDTO struct {
	Canceled  bool   `json:"canceled"`
	Token     string `json:"token"`
	FileName  string `json:"fileName"`
	Entries   int    `json:"entries"`
	Unusable  int    `json:"unusable"`
	Minutes   int    `json:"minutes"`
	Nights    int    `json:"nights"`
	SpanLabel string `json:"spanLabel"`
}

type AccountActivityImportInput struct {
	Token string `json:"token"`
}

type AccountActivityRemoveInput struct {
	Confirmation string `json:"confirmation"`
}

// AccountActivityDTO is the kept activity, and once there is some, the nights
// it suggests beside the recorded ones.
type AccountActivityDTO struct {
	Imported      bool             `json:"imported"`
	Minutes       int              `json:"minutes"`
	ImportedLabel string           `json:"importedLabel"`
	Figure        *NightsFigureDTO `json:"figure,omitempty"`
}

type pendingAccountActivity struct {
	token    string
	fileName string
	summary  takeout.Summary
	minutes  []int64
}

// cachedNights are the nights a source suggests, worked out once for each
// state of its evidence, which key names.
type cachedNights struct {
	key         string
	count       int
	first, last time.Time
	nights      []inference.Candidate
}

// PreviewActivityExport reads an export the person chooses and holds its
// minutes, and nothing else from it, until they are kept or another is chosen.
func (a *App) PreviewActivityExport() (AccountActivityPreviewDTO, error) {
	if _, err := a.requireStore(); err != nil {
		return AccountActivityPreviewDTO{}, err
	}
	a.holdActivityExport(nil)
	path, err := openActivityExportDialog(a.applicationContext(), runtime.OpenDialogOptions{
		Title: "Import activity from a Google export",
		Filters: []runtime.FileFilter{
			{DisplayName: "Google Takeout or My Activity (*.zip;*.json)", Pattern: "*.zip;*.json"},
		},
	})
	if err != nil {
		return AccountActivityPreviewDTO{}, errors.New("The export could not be chosen.")
	}
	if path == "" {
		return AccountActivityPreviewDTO{Canceled: true}, nil
	}
	minutes, summary, err := takeout.Read(path, a.currentTime())
	if err != nil {
		return AccountActivityPreviewDTO{}, activityExportError(err)
	}
	if len(minutes) == 0 {
		return AccountActivityPreviewDTO{}, errors.New("No activity in this export has a time ZeitBoard can use.")
	}
	pending := &pendingAccountActivity{token: newLocalID("activity_export"), fileName: filepath.Base(path), summary: summary, minutes: minutes}
	a.holdActivityExport(pending)
	location := displayLocation()
	return AccountActivityPreviewDTO{
		Token: pending.token, FileName: pending.fileName,
		Entries: summary.Entries, Unusable: summary.Unusable, Minutes: len(minutes),
		Nights:    len(suggestNights(inference.FromMoments(inference.SourceAccountActivity, minuteTimes(minutes)))),
		SpanLabel: dateSpanLabel(summary.First, summary.Last, location),
	}, nil
}

func activityExportError(err error) error {
	switch {
	case errors.Is(err, takeout.ErrHTMLExport):
		return errors.New("This export holds My Activity as web pages. In Google Takeout, set My Activity's format to JSON and export it again.")
	case errors.Is(err, takeout.ErrNoActivity):
		return errors.New("No Google My Activity was found in this file. Choose the Takeout .zip, or a My Activity .json file from it.")
	case errors.Is(err, takeout.ErrTooLarge):
		return fmt.Errorf("This export has more than %d activity entries, more than one person's My Activity holds.", takeout.MaxEntries)
	default:
		return fmt.Errorf("The export could not be read: %w.", err)
	}
}

// ImportActivityExport keeps the minutes of the export last previewed.
func (a *App) ImportActivityExport(input AccountActivityImportInput) (AccountActivityDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return AccountActivityDTO{}, err
	}
	a.accountActivityMu.Lock()
	defer a.accountActivityMu.Unlock()
	pending := a.accountActivityPending
	if pending == nil || input.Token == "" || pending.token != input.Token {
		return AccountActivityDTO{}, errors.New("This preview has expired. Choose the export again.")
	}
	a.accountActivityPending = nil
	if _, err := store.ImportAccountActivity(a.applicationContext(), storage.AccountActivityInput{
		Source: storage.AccountActivityGoogleTakeout, Files: pending.summary.Files,
		Entries: pending.summary.Entries, Unusable: pending.summary.Unusable, Minutes: pending.minutes,
	}, a.currentTime()); err != nil {
		return AccountActivityDTO{}, errors.New("The activity could not be kept. Nothing from this export was saved.")
	}
	return a.accountActivityLocked(store)
}

// GetAccountActivity reports the kept activity beside the recorded nights.
func (a *App) GetAccountActivity() (AccountActivityDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return AccountActivityDTO{}, err
	}
	a.accountActivityMu.Lock()
	defer a.accountActivityMu.Unlock()
	return a.accountActivityLocked(store)
}

// RemoveAccountActivity erases every kept minute. The export itself, and the
// account it came from, are not touched.
func (a *App) RemoveAccountActivity(input AccountActivityRemoveInput) (AccountActivityDTO, error) {
	if input.Confirmation != removeConfirm {
		return AccountActivityDTO{}, errors.New("Type REMOVE to remove the imported activity.")
	}
	store, err := a.requireStore()
	if err != nil {
		return AccountActivityDTO{}, err
	}
	a.accountActivityMu.Lock()
	defer a.accountActivityMu.Unlock()
	a.accountActivityPending = nil
	a.accountActivityNights = cachedNights{}
	if err := store.RemoveAccountActivity(a.applicationContext()); err != nil {
		return AccountActivityDTO{}, errors.New("The imported activity could not be removed. Try again.")
	}
	return a.accountActivityLocked(store)
}

func (a *App) holdActivityExport(pending *pendingAccountActivity) {
	a.accountActivityMu.Lock()
	a.accountActivityPending = pending
	a.accountActivityMu.Unlock()
}

func (a *App) accountActivityLocked(store *storage.Store) (AccountActivityDTO, error) {
	ctx := a.applicationContext()
	imports, err := store.AccountActivityImports(ctx)
	if err != nil {
		return AccountActivityDTO{}, errors.New("The imported activity could not be read.")
	}
	if len(imports) == 0 {
		return AccountActivityDTO{}, nil
	}
	nights, err := a.suggestedNightsLocked(ctx, store, imports)
	if err != nil {
		return AccountActivityDTO{}, err
	}
	sessions, err := store.EffectiveSleepSessions(ctx)
	if err != nil {
		return AccountActivityDTO{}, errors.New("Recorded nights could not be read.")
	}
	location := displayLocation()
	return AccountActivityDTO{
		Imported:      true,
		Minutes:       nights.count,
		ImportedLabel: imports[0].ImportedAt.In(location).Format("Jan 2, 2006"),
		Figure:        nightsFigure(nights.nights, sessions, nights.first, nights.last, location),
	}, nil
}

func (a *App) suggestedNightsLocked(ctx context.Context, store *storage.Store, imports []storage.AccountActivityImport) (cachedNights, error) {
	key := fmt.Sprintf("%d %s", len(imports), imports[0].ImportedAt.Format(time.RFC3339Nano))
	if a.accountActivityNights.key == key {
		return a.accountActivityNights, nil
	}
	minutes, err := store.AccountActivityMinutes(ctx)
	if err != nil || len(minutes) == 0 {
		return cachedNights{}, errors.New("The imported activity could not be read.")
	}
	a.accountActivityNights = cachedNights{
		key: key, count: len(minutes),
		first: minutes[0], last: minutes[len(minutes)-1],
		nights: suggestNights(inference.FromMoments(inference.SourceAccountActivity, minutes)),
	}
	return a.accountActivityNights, nil
}

func minuteTimes(minutes []int64) []time.Time {
	times := make([]time.Time, len(minutes))
	for i, minute := range minutes {
		times[i] = time.Unix(minute*60, 0).UTC()
	}
	return times
}
