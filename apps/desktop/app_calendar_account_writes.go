package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
	storage "non24.app/core/storage/sqlite"
	"non24.app/desktop/internal/calendaraccounts"
)

// Writing accepted times to a calendar brought in from a Google or Microsoft
// account (ADR-0065), with what writing to a CalDAV calendar promises
// (ADR-0053): off until turned on for one calendar, ZeitBoard's own events
// only, a create that cannot overwrite, and a removal that leaves an event
// the owner changed. Reading never asked to add events, so turning writing on
// signs the account in again to allow it, in the browser.

type AccountWriteBackInput struct {
	SourceID string `json:"sourceId"`
}

// EnableAccountWriteBack starts writing accepted times to a calendar brought
// in from an account, once the account allows adding events and the calendar
// is one ZeitBoard may add to.
func (a *App) EnableAccountWriteBack(input AccountWriteBackInput) (CalendarWriteBackDTO, error) {
	ctx := a.applicationContext()
	store, err := a.requireStore()
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	account, calendar, found, err := accountCalendar(ctx, store, strings.TrimSpace(input.SourceID))
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	if !found {
		return CalendarWriteBackDTO{}, storage.ErrCalendarNotWritable
	}
	if account, err = a.allowWriting(ctx, account); err != nil {
		return CalendarWriteBackDTO{}, err
	}
	client, access, err := a.accountAccess(ctx, store, account)
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	writable, err := client.CanWrite(ctx, access, calendar.CalendarID)
	if err != nil {
		a.forgetRefusedAccess(account.AccountID, err)
		return CalendarWriteBackDTO{}, asSentence(err)
	}
	if !writable {
		return CalendarWriteBackDTO{}, fmt.Errorf("ZeitBoard can add events only to a calendar of your own, and %s is not.", calendar.Label)
	}
	if err := store.SetCalendarWriteTarget(ctx, storage.CalendarWriteTarget{SourceID: calendar.SourceID}, a.currentTime()); err != nil {
		return CalendarWriteBackDTO{}, err
	}
	a.calendarWriter.nudge()
	return a.calendarWriteBack(ctx, store)
}

// allowWriting gives back the account as it stands once it allows adding
// events: as it is, or signed in again in the browser to ask.
func (a *App) allowWriting(ctx context.Context, account storage.CalendarAccount) (storage.CalendarAccount, error) {
	provider := calendaraccounts.Provider(account.Provider)
	if calendaraccounts.Writes(provider, account.Scopes) {
		return account, nil
	}
	client := a.accountClient(account)
	client.Write = true
	if _, err := a.signInCalendarAccount(client, &account); err != nil {
		return storage.CalendarAccount{}, err
	}
	_, signedIn, err := a.calendarAccount(ctx, account.AccountID)
	if err != nil {
		return storage.CalendarAccount{}, err
	}
	if !calendaraccounts.Writes(provider, signedIn.Scopes) {
		return storage.CalendarAccount{}, fmt.Errorf("%s did not allow ZeitBoard to add events, so writing was not turned on.", providerName(provider))
	}
	return signedIn, nil
}

// eventWriter carries writes to the calendar written to: a CalDAV collection
// (ADR-0053) or an account's calendar (ADR-0065).
type eventWriter interface {
	// create writes an accepted time; retry says an earlier attempt may have
	// written it with its answer lost.
	create(ctx context.Context, event calendarcore.Event, stamp time.Time, retry bool) calendarWriteResult
	remove(ctx context.Context, eventID, href, etag string) calendarWriteResult
}

// eventWriterFor is the writer for the calendar written to, with an account's
// access to hand.
func (a *App) eventWriterFor(ctx context.Context, store *storage.Store, target storage.CalendarWriteTarget) (eventWriter, error) {
	if target.AccountID == "" {
		return newCalDAVWriter(a.calendarHTTPClient, target.CollectionURL, target.Username, target.Password)
	}
	_, account, err := a.calendarAccount(ctx, target.AccountID)
	if err != nil {
		return nil, err
	}
	client, access, err := a.accountAccess(ctx, store, account)
	if err != nil {
		return nil, err
	}
	return accountEventWriter{
		client: client, access: access, calendarID: target.CalendarID,
		refused: func(err error) { a.forgetRefusedAccess(account.AccountID, err) },
	}, nil
}

// accountEventWriter writes to one calendar of an account. The provider
// package keeps ADR-0053's promises; this says how each write came out, and
// drops an access token the provider refused so the next pass refreshes it.
type accountEventWriter struct {
	client     calendaraccounts.Client
	access     string
	calendarID string
	refused    func(error)
}

func (w accountEventWriter) create(ctx context.Context, event calendarcore.Event, _ time.Time, retry bool) calendarWriteResult {
	written, err := w.client.CreateEvent(ctx, w.access, w.calendarID, event, retry)
	switch {
	case errors.Is(err, calendaraccounts.ErrTaken):
		return calendarWriteResult{outcome: calendarWriteConflict, detail: err.Error()}
	case err != nil:
		w.refused(err)
		return calendarWriteResult{outcome: calendarWriteFailed, detail: asSentence(err).Error()}
	}
	return calendarWriteResult{outcome: calendarWriteDone, href: written.ID, etag: written.Version}
}

func (w accountEventWriter) remove(ctx context.Context, eventID, href, etag string) calendarWriteResult {
	err := w.client.DeleteEvent(ctx, w.access, w.calendarID, eventID, calendaraccounts.Written{ID: href, Version: etag})
	switch {
	case errors.Is(err, calendaraccounts.ErrChanged), errors.Is(err, calendaraccounts.ErrNotOurs):
		return calendarWriteResult{outcome: calendarWriteConflict, detail: err.Error()}
	case err != nil:
		w.refused(err)
		return calendarWriteResult{outcome: calendarWriteFailed, detail: asSentence(err).Error()}
	}
	return calendarWriteResult{outcome: calendarWriteDone}
}

// deferCalendarWrites leaves writes waiting, saying why, when the account
// cannot be signed in to for any of them. It reports whether any was.
func (a *App) deferCalendarWrites(ctx context.Context, store *storage.Store, due []storage.CalendarWrite, reason string) bool {
	now := a.currentTime().UTC()
	deferred := false
	for _, write := range due {
		retryAt := now.Add(calendarWriteRetryDelay(write.Attempts + 1))
		deferred = store.DeferCalendarWrite(ctx, write.EventID, reason, retryAt, now) == nil || deferred
	}
	return deferred
}

func providerName(provider calendaraccounts.Provider) string {
	if provider == calendaraccounts.Microsoft {
		return "Microsoft"
	}
	return "Google"
}
