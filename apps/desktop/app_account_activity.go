package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"non24.app/core/domain"
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

// AccountActivityDTO is the kept activity, the nights it suggests and how they
// compare with recorded ones. Suggested and Recorded are segments of nights cut
// at midnight in ZoneID: [day, start minute, end minute], day 0 being
// FirstDate.
type AccountActivityDTO struct {
	Imported      bool                         `json:"imported"`
	Minutes       int                          `json:"minutes"`
	Nights        int                          `json:"nights"`
	SpanLabel     string                       `json:"spanLabel"`
	ImportedLabel string                       `json:"importedLabel"`
	ZoneID        string                       `json:"zoneId"`
	FirstDate     string                       `json:"firstDate"`
	Days          int                          `json:"days"`
	Suggested     [][3]int                     `json:"suggested"`
	Recorded      [][3]int                     `json:"recorded"`
	Agreement     *AccountActivityAgreementDTO `json:"agreement,omitempty"`
}

// AccountActivityAgreementDTO measures suggestions on recorded nights only: a
// night nobody recorded is not evidence either way.
type AccountActivityAgreementDTO struct {
	Recorded int    `json:"recorded"`
	Matched  int    `json:"matched"`
	Summary  string `json:"summary"`
}

type pendingAccountActivity struct {
	token    string
	fileName string
	summary  takeout.Summary
	minutes  []int64
}

// accountActivityNights are the nights the kept activity suggests, worked out
// once per set of imports.
type accountActivityNights struct {
	key         string
	minutes     int
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
		Nights:    len(suggestNights(minuteTimes(minutes))),
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
	a.accountActivityNights = accountActivityNights{}
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
	location := displayLocation()
	if len(imports) == 0 {
		return AccountActivityDTO{ZoneID: location.String(), Suggested: [][3]int{}, Recorded: [][3]int{}}, nil
	}
	nights, err := a.suggestedNightsLocked(ctx, store, imports)
	if err != nil {
		return AccountActivityDTO{}, err
	}
	sessions, err := store.EffectiveSleepSessions(ctx)
	if err != nil {
		return AccountActivityDTO{}, errors.New("Recorded nights could not be read.")
	}
	recorded := recordedNightsWithin(sessions, nights.first, nights.last)
	firstDay := civilDay(nights.first, location)
	suggested := make([]inference.TruthInterval, len(nights.nights))
	for i, night := range nights.nights {
		suggested[i] = inference.TruthInterval{StartAt: night.StartAt, EndAt: night.EndAt}
	}
	return AccountActivityDTO{
		Imported: true, Minutes: nights.minutes, Nights: len(nights.nights),
		SpanLabel:     dateSpanLabel(nights.first, nights.last, location),
		ImportedLabel: imports[0].ImportedAt.In(location).Format("Jan 2, 2006"),
		ZoneID:        location.String(),
		FirstDate:     firstDay.Format("2006-01-02"),
		Days:          civilDaysBetween(firstDay, civilDay(nights.last, location)) + 1,
		Suggested:     nightSegments(suggested, firstDay, location),
		Recorded:      nightSegments(recorded, firstDay, location),
		Agreement:     nightAgreement(nights.nights, recorded),
	}, nil
}

func (a *App) suggestedNightsLocked(ctx context.Context, store *storage.Store, imports []storage.AccountActivityImport) (accountActivityNights, error) {
	key := fmt.Sprintf("%d %s", len(imports), imports[0].ImportedAt.Format(time.RFC3339Nano))
	if a.accountActivityNights.key == key {
		return a.accountActivityNights, nil
	}
	minutes, err := store.AccountActivityMinutes(ctx)
	if err != nil || len(minutes) == 0 {
		return accountActivityNights{}, errors.New("The imported activity could not be read.")
	}
	a.accountActivityNights = accountActivityNights{
		key: key, minutes: len(minutes),
		first: minutes[0], last: minutes[len(minutes)-1],
		nights: suggestNights(minutes),
	}
	return a.accountActivityNights, nil
}

// suggestNights applies ADR-0031's rules to gaps in account activity alone, so
// that what it suggests can be measured against what was recorded.
func suggestNights(minutes []time.Time) []inference.Candidate {
	nights, refusal := inference.Build(inference.FromMoments(inference.SourceAccountActivity, minutes), time.Time{})
	if refusal != nil {
		return nil
	}
	return nights
}

func minuteTimes(minutes []int64) []time.Time {
	times := make([]time.Time, len(minutes))
	for i, minute := range minutes {
		times[i] = time.Unix(minute*60, 0).UTC()
	}
	return times
}

// recordedNightsWithin are the principal sleeps recorded wholly inside the
// span the activity covers: the only nights it could have suggested.
func recordedNightsWithin(sessions []domain.SleepSession, first, last time.Time) []inference.TruthInterval {
	var nights []inference.TruthInterval
	for _, session := range sessions {
		if !session.IsPrincipalSleep() || len(session.Intervals) == 0 {
			continue
		}
		start := session.Intervals[0].Interval.Start.UTC
		end := session.Intervals[len(session.Intervals)-1].Interval.End.UTC
		if start.Before(first) || end.After(last) {
			continue
		}
		nights = append(nights, inference.TruthInterval{StartAt: start, EndAt: end})
	}
	return nights
}

// nightAgreement says how many recorded nights had a suggestion beside them
// and, where one did, how far apart their edges typically were.
func nightAgreement(suggested []inference.Candidate, recorded []inference.TruthInterval) *AccountActivityAgreementDTO {
	if len(recorded) == 0 {
		return nil
	}
	score := inference.ScoreCandidates(suggested, recorded)
	agreement := &AccountActivityAgreementDTO{Recorded: len(recorded), Matched: score.Matched}
	if score.Matched > 0 {
		agreement.Summary = fmt.Sprintf("On those nights, suggestions typically began %s and ended %s.",
			edgeWords(score.OnsetBias, "your recorded sleep"), edgeWords(score.WakeBias, "your recorded wake"))
	}
	return agreement
}

// edgeWords puts a signed median, a suggestion's edge minus the recorded one,
// into words. Within five minutes is the same time: the export knows minutes,
// and people put the phone down a little before sleeping.
func edgeWords(bias time.Duration, edge string) string {
	bias = bias.Round(time.Minute)
	switch {
	case bias > -5*time.Minute && bias < 5*time.Minute:
		return "at about " + edge
	case bias < 0:
		return formatDuration(-bias) + " before " + edge
	default:
		return formatDuration(bias) + " after " + edge
	}
}

// nightSegments cuts each night at midnight in location, giving each part as
// [day, start minute, end minute] with day 0 being firstDay.
func nightSegments(nights []inference.TruthInterval, firstDay time.Time, location *time.Location) [][3]int {
	segments := [][3]int{}
	for _, night := range nights {
		start := night.StartAt.In(location)
		end := night.EndAt.In(location)
		for day := civilDay(start, location); day.Before(end); day = day.AddDate(0, 0, 1) {
			next := day.AddDate(0, 0, 1)
			from, to := laterOf(start, day), earlierOf(end, next)
			if !to.After(from) {
				continue
			}
			index := civilDaysBetween(firstDay, day)
			if index < 0 {
				continue
			}
			endMinute := 24 * 60
			if to.Before(next) {
				endMinute = minuteOfDay(to)
			}
			segments = append(segments, [3]int{index, minuteOfDay(from), endMinute})
		}
	}
	return segments
}

func civilDay(at time.Time, location *time.Location) time.Time {
	local := at.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

// civilDaysBetween counts calendar days, whatever a daylight-saving change
// does to the hours between them.
func civilDaysBetween(from, to time.Time) int {
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}

func minuteOfDay(at time.Time) int { return at.Hour()*60 + at.Minute() }

func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlierOf(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func dateSpanLabel(first, last time.Time, location *time.Location) string {
	from, to := first.In(location).Format("Jan 2, 2006"), last.In(location).Format("Jan 2, 2006")
	if from == to {
		return from
	}
	return from + " to " + to
}

// displayLocation is this computer's zone, in which the desktop states civil
// times.
func displayLocation() *time.Location {
	location, err := time.LoadLocation(localZoneID())
	if err != nil {
		return time.UTC
	}
	return location
}
