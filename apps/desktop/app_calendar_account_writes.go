package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

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
// is one ZeitBoard may add to. It is held as a sign-in is, so the window can
// cancel it at any point, whether or not the browser was asked.
func (a *App) EnableAccountWriteBack(input AccountWriteBackInput) (CalendarWriteBackDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	ctx, done, err := a.beginCalendarSignIn()
	if err != nil {
		return CalendarWriteBackDTO{}, err
	}
	defer done()
	if err := a.turnAccountWritingOn(ctx, store, strings.TrimSpace(input.SourceID)); err != nil {
		if ctx.Err() != nil {
			return CalendarWriteBackDTO{}, errors.New("Turning writing on was cancelled.")
		}
		return CalendarWriteBackDTO{}, err
	}
	// Writing is on now, whatever a cancel arriving after this would say.
	a.calendarWriter.nudge()
	return a.calendarWriteBack(a.applicationContext(), store)
}

func (a *App) turnAccountWritingOn(ctx context.Context, store *storage.Store, sourceID string) error {
	account, calendar, found, err := accountCalendar(ctx, store, sourceID)
	if err != nil {
		return err
	}
	if !found {
		return storage.ErrCalendarNotWritable
	}
	if account, err = a.allowWriting(ctx, account); err != nil {
		return err
	}
	client, access, err := a.accountAccess(ctx, store, account)
	if err != nil {
		return err
	}
	writable, err := client.CanWrite(ctx, access, calendar.CalendarID)
	if err != nil {
		a.forgetRefusedAccess(account.AccountID, err)
		return asSentence(err)
	}
	if !writable {
		return fmt.Errorf("ZeitBoard can add events only to a calendar of your own, and %s is not.", calendar.Label)
	}
	return store.SetCalendarWriteTarget(ctx, storage.CalendarWriteTarget{SourceID: calendar.SourceID}, a.currentTime())
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
	if _, err := a.signInCalendarAccount(ctx, client, &account); err != nil {
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
	create(ctx context.Context, accepted storage.AcceptedTime, stamp time.Time) calendarWriteResult
	remove(ctx context.Context, eventID, href, etag string) calendarWriteResult
	// removeUnconfirmed looks for what the create of a time undone before it
	// was confirmed may have written, and removes it if it is ZeitBoard's.
	removeUnconfirmed(ctx context.Context, created storage.UnconfirmedCreate) calendarWriteResult
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
		client: client, access: access, calendarID: target.CalendarID, writing: writingName(target.EnabledAt),
		refused: func(err error) { a.forgetRefusedAccess(account.AccountID, err) },
	}, nil
}

// writingName names one time of writing to a calendar, from turning it on to
// stopping, by the moment it was turned on. That moment is kept to the
// nanosecond, so no two times of writing share it; it is only ever compared
// for equality, never with a provider's clock.
func writingName(enabledAt time.Time) string {
	sum := sha256.Sum256([]byte(enabledAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(sum[:8])
}

// accountEventWriter writes to one calendar of an account. The provider
// package keeps ADR-0053's promises; this says how each write came out, and
// drops an access token the provider refused so the next pass refreshes it.
type accountEventWriter struct {
	client     calendaraccounts.Client
	access     string
	calendarID string
	writing    string // this time of writing to the calendar
	refused    func(error)
}

func (w accountEventWriter) create(ctx context.Context, accepted storage.AcceptedTime, _ time.Time) calendarWriteResult {
	if accepted.Approval == "" {
		// Its event is named by the approval, which no record holds.
		return calendarWriteResult{outcome: calendarWriteConflict, detail: "ZeitBoard has no record of accepting this time, so it was not written."}
	}
	written, err := w.client.CreateEvent(ctx, w.access, w.calendarID, accepted.Event, accepted.Approval, w.writing)
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

func (w accountEventWriter) removeUnconfirmed(ctx context.Context, created storage.UnconfirmedCreate) calendarWriteResult {
	err := w.client.RemoveUnconfirmed(ctx, w.access, w.calendarID, created.EventID, created.Approval, created.StartAt, created.EndAt)
	if err != nil {
		w.refused(err)
		return calendarWriteResult{outcome: calendarWriteFailed, detail: asSentence(err).Error()}
	}
	return calendarWriteResult{outcome: calendarWriteDone}
}

// deferCalendarWrites leaves writes waiting, saying why, when the account
// cannot be signed in to for any of them. It reports whether any was.
func (a *App) deferCalendarWrites(ctx context.Context, store *storage.Store, target storage.CalendarWriteTarget, due []storage.CalendarWrite, reason string) bool {
	now := a.currentTime().UTC()
	deferred := false
	for _, write := range due {
		retryAt := now.Add(calendarWriteRetryDelay(write.Attempts + 1))
		deferred = store.DeferCalendarWrite(ctx, target, write, reason, retryAt, now) == nil || deferred
	}
	return deferred
}

func providerName(provider calendaraccounts.Provider) string {
	if provider == calendaraccounts.Microsoft {
		return "Microsoft"
	}
	return "Google"
}
