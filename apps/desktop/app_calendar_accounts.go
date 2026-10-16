package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	calendarcore "non24.app/core/calendar"
	storage "non24.app/core/storage/sqlite"
	"non24.app/desktop/internal/calendaraccounts"
)

// Calendars from a Google or Microsoft account (ADR-0064), read through the
// owner's own app registration: the owner registers ZeitBoard with the
// provider, pastes its client ID here, and signs in in the browser. ZeitBoard
// ships no credentials of its own. The refresh token stays on this computer
// like the CalDAV write-back sign-in, never shown, synced or exported, and
// every calendar brought in from an account is read again each hour with the
// subscribed ones.

const (
	// calendarSignInTimeout is how long a sign-in waits for the browser.
	calendarSignInTimeout = 5 * time.Minute

	// calendarRevokeTimeout bounds asking a provider to withdraw a grant.
	calendarRevokeTimeout = 10 * time.Second
)

// openSignInPage opens a provider's sign-in page in the owner's browser;
// tests answer it themselves.
var openSignInPage = runtime.BrowserOpenURL

type CalendarAccountInput struct {
	Provider     string `json:"provider"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

type CalendarAccountRef struct {
	AccountID string `json:"accountId"`
}

type AccountCalendarInput struct {
	AccountID  string `json:"accountId"`
	CalendarID string `json:"calendarId"`
	Label      string `json:"label"`
	ZoneID     string `json:"zoneId"`
}

type RemoveCalendarAccountInput struct {
	AccountID    string `json:"accountId"`
	Confirmation string `json:"confirmation"`
}

// CalendarAccountDTO is a signed-in account, without its credentials.
type CalendarAccountDTO struct {
	AccountID      string `json:"accountId"`
	Provider       string `json:"provider"`
	Label          string `json:"label"`
	ConnectedLabel string `json:"connectedLabel"`
	// Calendars counts the calendars brought in from the account, and
	// Writing is set when accepted times are written to one of them.
	Calendars int  `json:"calendars"`
	Writing   bool `json:"writing,omitempty"`
	// Problem says the provider ended the sign-in; signing in again with
	// the kept registration restores it.
	Problem string `json:"problem,omitempty"`
}

// AccountCalendarChoiceDTO is one calendar an account holds. SourceID is set
// once it has been brought in.
type AccountCalendarChoiceDTO struct {
	CalendarID string `json:"calendarId"`
	Name       string `json:"name"`
	Primary    bool   `json:"primary"`
	SourceID   string `json:"sourceId,omitempty"`
}

type CalendarAccountCalendarsDTO struct {
	Account   CalendarAccountDTO         `json:"account"`
	Calendars []AccountCalendarChoiceDTO `json:"calendars"`
}

type RemovedCalendarAccountDTO struct {
	Message string `json:"message"`
}

// CalendarAccountSourceDTO says which account a calendar comes from and how
// its last refresh went.
type CalendarAccountSourceDTO struct {
	Provider       string `json:"provider"`
	AccountLabel   string `json:"accountLabel"`
	RefreshedLabel string `json:"refreshedLabel"`
	Failure        string `json:"failure,omitempty"`
}

// ConnectCalendarAccount signs in to an account through the owner's app
// registration and keeps it, giving back the calendars it holds to choose
// from.
func (a *App) ConnectCalendarAccount(input CalendarAccountInput) (CalendarAccountCalendarsDTO, error) {
	provider := calendaraccounts.Provider(strings.TrimSpace(input.Provider))
	if !provider.Valid() {
		return CalendarAccountCalendarsDTO{}, errors.New("Choose Google or Microsoft.")
	}
	clientID := strings.TrimSpace(input.ClientID)
	if clientID == "" || len(clientID) > 512 {
		return CalendarAccountCalendarsDTO{}, errors.New("Paste the client ID of your app registration.")
	}
	// Microsoft refuses a secret from a public client, which a desktop app
	// registered for http://localhost is.
	secret := ""
	if provider == calendaraccounts.Google {
		secret = strings.TrimSpace(input.ClientSecret)
		if secret == "" || len(secret) > 512 {
			return CalendarAccountCalendarsDTO{}, errors.New("Paste the client secret Google gave with the client ID.")
		}
	}
	ctx, done, err := a.beginCalendarSignIn()
	if err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	defer done()
	return a.signInCalendarAccount(ctx, a.calendarAccountClient(provider, clientID, secret), nil)
}

// ReconnectCalendarAccount signs in again with the registration an account
// was kept with, as after the provider ended its sign-in. It asks for what is
// in use: reading, and adding events while accepted times are written to the
// account.
func (a *App) ReconnectCalendarAccount(input CalendarAccountRef) (CalendarAccountCalendarsDTO, error) {
	ctx := a.applicationContext()
	store, account, err := a.calendarAccount(ctx, input.AccountID)
	if err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	writing, err := accountWrittenTo(ctx, store)
	if err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	client := a.accountClient(account)
	client.Write = writing == account.AccountID
	held, done, err := a.beginCalendarSignIn()
	if err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	defer done()
	return a.signInCalendarAccount(held, client, &account)
}

// CancelCalendarSignIn stops the sign-in under way, or turning writing on,
// whether or not it is waiting for the browser.
func (a *App) CancelCalendarSignIn() {
	a.calendarAccountsMu.Lock()
	defer a.calendarAccountsMu.Unlock()
	if a.calendarSignInCancel != nil {
		a.calendarSignInCancel()
	}
}

// GetCalendarAccounts lists the signed-in accounts.
func (a *App) GetCalendarAccounts() ([]CalendarAccountDTO, error) {
	ctx := a.applicationContext()
	store, err := a.requireStore()
	if err != nil {
		return nil, err
	}
	accounts, err := store.CalendarAccounts(ctx)
	if err != nil {
		return nil, err
	}
	calendars, err := store.AccountCalendars(ctx)
	if err != nil {
		return nil, err
	}
	kept := make(map[string]int, len(accounts))
	for _, calendar := range calendars {
		kept[calendar.AccountID]++
	}
	writing, err := accountWrittenTo(ctx, store)
	if err != nil {
		return nil, err
	}
	result := make([]CalendarAccountDTO, 0, len(accounts))
	for _, account := range accounts {
		result = append(result, calendarAccountDTO(account, kept[account.AccountID], writing == account.AccountID))
	}
	return result, nil
}

// ListAccountCalendars reads the calendars an account holds, marking those
// already brought in.
func (a *App) ListAccountCalendars(input CalendarAccountRef) (CalendarAccountCalendarsDTO, error) {
	ctx := a.applicationContext()
	store, account, err := a.calendarAccount(ctx, input.AccountID)
	if err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	client, access, err := a.accountAccess(ctx, store, account)
	if err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	calendars, err := client.Calendars(ctx, access)
	if err != nil {
		a.forgetRefusedAccess(account.AccountID, err)
		return CalendarAccountCalendarsDTO{}, asSentence(err)
	}
	return accountCalendarsDTO(ctx, store, account, calendars)
}

// PreviewAccountCalendar reads one of an account's calendars without keeping
// it.
func (a *App) PreviewAccountCalendar(input AccountCalendarInput) (CalendarImportDTO, error) {
	ctx := a.applicationContext()
	store, account, err := a.calendarAccount(ctx, input.AccountID)
	if err != nil {
		return CalendarImportDTO{}, err
	}
	set, err := a.fetchAccountCalendar(ctx, store, account, input, a.currentTime().UTC())
	if err != nil {
		return CalendarImportDTO{}, err
	}
	return a.calendarImportResult(ctx, set, nil)
}

// AddAccountCalendar brings one of an account's calendars in, to be read
// again every hour from then on.
func (a *App) AddAccountCalendar(input AccountCalendarInput) (CalendarImportDTO, error) {
	ctx := a.applicationContext()
	store, account, err := a.calendarAccount(ctx, input.AccountID)
	if err != nil {
		return CalendarImportDTO{}, err
	}
	set, err := a.fetchAccountCalendar(ctx, store, account, input, a.currentTime().UTC())
	if err != nil {
		return CalendarImportDTO{}, err
	}
	return a.calendarImportResult(ctx, set, keepAccountCalendar(account.AccountID, strings.TrimSpace(input.CalendarID)))
}

// RemoveCalendarAccount erases an account's sign-in and every calendar
// brought in from it, then asks the provider to withdraw the grant where it
// can.
func (a *App) RemoveCalendarAccount(input RemoveCalendarAccountInput) (RemovedCalendarAccountDTO, error) {
	if strings.TrimSpace(input.Confirmation) != removeConfirm {
		return RemovedCalendarAccountDTO{}, errors.New("type REMOVE to confirm removing the account")
	}
	ctx := a.applicationContext()
	store, account, err := a.calendarAccount(ctx, input.AccountID)
	if err != nil {
		return RemovedCalendarAccountDTO{}, err
	}
	if err := store.RemoveCalendarAccount(ctx, account.AccountID); err != nil {
		return RemovedCalendarAccountDTO{}, err
	}
	a.forgetAccessToken(account.AccountID)
	provider := calendaraccounts.Provider(account.Provider)
	revokeCtx, cancel := context.WithTimeout(ctx, calendarRevokeTimeout)
	defer cancel()
	removed := account.Label + " and its calendars are removed from ZeitBoard."
	switch {
	case a.accountClient(account).Revoke(revokeCtx, account.RefreshToken):
		return RemovedCalendarAccountDTO{Message: removed + " Google has withdrawn ZeitBoard's access too."}, nil
	case provider == calendaraccounts.Google:
		return RemovedCalendarAccountDTO{Message: removed + " Google did not confirm withdrawing ZeitBoard's access; " +
			"you can withdraw it at myaccount.google.com/connections."}, nil
	default:
		return RemovedCalendarAccountDTO{Message: removed + " To withdraw its access at Microsoft too, remove it at " +
			"account.live.com/consent/Manage, or at myapps.microsoft.com for a work account."}, nil
	}
}

// calendarAccountClient talks to a provider for one app registration.
func (a *App) calendarAccountClient(provider calendaraccounts.Provider, clientID, clientSecret string) calendaraccounts.Client {
	endpoints := calendaraccounts.DefaultEndpoints(provider)
	if a.calendarAccountEndpoints != nil {
		endpoints = a.calendarAccountEndpoints(provider)
	}
	return calendaraccounts.Client{
		Provider: provider, ClientID: clientID, ClientSecret: clientSecret,
		Endpoints: endpoints, HTTP: a.calendarAccountHTTP,
	}
}

// accountClient talks to an account's provider with the registration it was
// kept with, asking for writing where the account allowed it.
func (a *App) accountClient(account storage.CalendarAccount) calendaraccounts.Client {
	provider := calendaraccounts.Provider(account.Provider)
	client := a.calendarAccountClient(provider, account.ClientID, account.ClientSecret)
	client.Write = calendaraccounts.Writes(provider, account.Scopes)
	return client
}

// signInCalendarAccount opens the provider's sign-in in the browser, waits
// for it to come back to a one-time loopback address, and keeps the account
// it signed in to: any account, or only the one wanted. ctx is the one
// beginCalendarSignIn holds.
func (a *App) signInCalendarAccount(ctx context.Context, client calendaraccounts.Client, want *storage.CalendarAccount) (CalendarAccountCalendarsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	receiver, err := calendaraccounts.Listen()
	if err != nil {
		return CalendarAccountCalendarsDTO{}, asSentence(err)
	}
	defer receiver.Close()
	auth, err := client.Begin(receiver.Redirect(client.Provider))
	if err != nil {
		return CalendarAccountCalendarsDTO{}, asSentence(err)
	}
	openSignInPage(a.applicationContext(), auth.URL)
	code, err := receiver.Wait(ctx, auth.State)
	if err != nil {
		return CalendarAccountCalendarsDTO{}, signInStopped(err)
	}
	a.showDesktopWindow()
	token, err := client.Exchange(ctx, auth, code)
	if err != nil {
		return CalendarAccountCalendarsDTO{}, signInStopped(err)
	}
	// Google lets the owner untick what is asked; without reading there is
	// nothing to keep.
	if !calendaraccounts.Reads(client.Provider, token.Scope) {
		return CalendarAccountCalendarsDTO{}, fmt.Errorf("%s did not allow ZeitBoard to read your calendars, so the account was not added.", providerName(client.Provider))
	}
	calendars, err := client.Calendars(ctx, token.Access)
	if err != nil {
		return CalendarAccountCalendarsDTO{}, signInStopped(err)
	}
	label := accountLabel(client.Provider, calendars)
	account := storage.CalendarAccount{
		AccountID:    stableCalendarID("calendar_account", string(client.Provider)+"\n"+strings.ToLower(label)),
		Provider:     string(client.Provider),
		Label:        label,
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		RefreshToken: token.Refresh,
		ConnectedAt:  a.currentTime().UTC(),
		Scopes:       token.Scope,
	}
	if want != nil && account.AccountID != want.AccountID {
		return CalendarAccountCalendarsDTO{}, fmt.Errorf("The browser signed in to %s. Sign in to %s instead.", label, want.Label)
	}
	// What a sign-in states is what is kept. One that does not allow adding
	// events must not replace the sign-in of an account written to.
	writing, err := accountWrittenTo(ctx, store)
	if err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	if writing == account.AccountID && !calendaraccounts.Writes(client.Provider, account.Scopes) {
		return CalendarAccountCalendarsDTO{}, writingKept(client, label)
	}
	if err := store.SaveCalendarAccount(ctx, account); err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	a.holdAccessToken(account.AccountID, token)
	return accountCalendarsDTO(ctx, store, account, calendars)
}

// writingKept refuses a sign-in that would take adding events from the
// account written to, saying what to do instead: a sign-in that asked for it
// was refused it in the browser, while the form asks only to read and
// “Sign in again” asks for both.
func writingKept(client calendaraccounts.Client, label string) error {
	if client.Write {
		return fmt.Errorf("Accepted times are written to a calendar of %s, so signing in to it again must allow adding events. Allow that when %s asks, or stop writing first.", label, providerName(client.Provider))
	}
	return fmt.Errorf("Accepted times are written to a calendar of %s, so signing in to it must allow adding events. Use “Sign in again” on the account, which asks for that, or stop writing first.", label)
}

// signInStopped words why a sign-in stopped: cancelled, out of time, or the
// provider's own reason.
func signInStopped(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("Signing in waited five minutes for the browser and stopped. Try again when you are ready.")
	case errors.Is(err, context.Canceled):
		return errors.New("Signing in was cancelled.")
	default:
		return asSentence(err)
	}
}

// beginCalendarSignIn holds the one sign-in, or turning writing on, that may
// run at a time, until it is done, five minutes pass, or it is cancelled.
func (a *App) beginCalendarSignIn() (context.Context, func(), error) {
	a.calendarAccountsMu.Lock()
	defer a.calendarAccountsMu.Unlock()
	if a.calendarSignInCancel != nil {
		return nil, nil, errors.New("ZeitBoard is already signing in to a calendar account. Finish or cancel that first.")
	}
	ctx, cancel := context.WithTimeout(a.applicationContext(), calendarSignInTimeout)
	a.calendarSignInCancel = cancel
	return ctx, func() {
		a.calendarAccountsMu.Lock()
		a.calendarSignInCancel = nil
		a.calendarAccountsMu.Unlock()
		cancel()
	}, nil
}

// accountAccess gives a client for an account and an access token: the one
// held from within its hour, or a new one, keeping a refresh token the
// provider rotated. A sign-in the provider has ended is recorded on the
// account, for the window to offer signing in again.
func (a *App) accountAccess(ctx context.Context, store *storage.Store, account storage.CalendarAccount) (calendaraccounts.Client, string, error) {
	// One refresh at a time, so a provider that rotates refresh tokens is
	// never sent one it has already replaced.
	a.calendarRefreshMu.Lock()
	defer a.calendarRefreshMu.Unlock()
	a.calendarAccountsMu.Lock()
	held, found := a.calendarAccessTokens[account.AccountID]
	a.calendarAccountsMu.Unlock()
	if found && time.Until(held.ExpiresAt) > time.Minute {
		return a.accountClient(account), held.Access, nil
	}
	// The sign-in as it is kept now: a refresh since the caller read the
	// account may have rotated its token.
	_, account, err := a.calendarAccount(ctx, account.AccountID)
	if err != nil {
		return calendaraccounts.Client{}, "", err
	}
	client := a.accountClient(account)
	token, err := client.Refresh(ctx, account.RefreshToken)
	if errors.Is(err, calendaraccounts.ErrSignInEnded) {
		ended := fmt.Sprintf("The sign-in to %s has ended. Sign in to it again to keep reading its calendars.", account.Label)
		_ = store.RecordCalendarAccountProblem(ctx, account.AccountID, ended)
		return client, "", errors.New(ended)
	}
	if err != nil {
		return client, "", asSentence(err)
	}
	if token.Refresh != account.RefreshToken {
		if err := store.RotateCalendarAccountToken(ctx, account.AccountID, token.Refresh); err != nil {
			return client, "", err
		}
	}
	if account.Problem != "" {
		_ = store.RecordCalendarAccountProblem(ctx, account.AccountID, "")
	}
	a.holdAccessToken(account.AccountID, token)
	return client, token.Access, nil
}

// holdAccessToken keeps an access token in memory for its hour. It is never
// written anywhere.
func (a *App) holdAccessToken(accountID string, token calendaraccounts.Token) {
	a.calendarAccountsMu.Lock()
	defer a.calendarAccountsMu.Unlock()
	if a.calendarAccessTokens == nil {
		a.calendarAccessTokens = make(map[string]calendaraccounts.Token)
	}
	a.calendarAccessTokens[accountID] = token
}

// forgetAccessToken drops a held access token, so the next request
// refreshes.
func (a *App) forgetAccessToken(accountID string) {
	a.calendarAccountsMu.Lock()
	defer a.calendarAccountsMu.Unlock()
	delete(a.calendarAccessTokens, accountID)
}

// forgetRefusedAccess drops an access token the provider refused. Any other
// failure, such as a calendar deleted at the provider, leaves it good for
// the account's other calendars.
func (a *App) forgetRefusedAccess(accountID string, err error) {
	if calendaraccounts.Unauthorized(err) {
		a.forgetAccessToken(accountID)
	}
}

// calendarAccount finds a signed-in account.
func (a *App) calendarAccount(ctx context.Context, accountID string) (*storage.Store, storage.CalendarAccount, error) {
	store, err := a.requireStore()
	if err != nil {
		return nil, storage.CalendarAccount{}, err
	}
	accounts, err := store.CalendarAccounts(ctx)
	if err != nil {
		return nil, storage.CalendarAccount{}, err
	}
	for _, account := range accounts {
		if account.AccountID == strings.TrimSpace(accountID) {
			return store, account, nil
		}
	}
	return nil, storage.CalendarAccount{}, errors.New("That account is no longer signed in.")
}

// fetchAccountCalendar reads one of an account's calendars over the span
// every calendar is kept for.
func (a *App) fetchAccountCalendar(ctx context.Context, store *storage.Store, account storage.CalendarAccount, input AccountCalendarInput, importedAt time.Time) (calendarcore.EventSet, error) {
	calendarID := strings.TrimSpace(input.CalendarID)
	if calendarID == "" || len(calendarID) > 1024 {
		return calendarcore.EventSet{}, errors.New("Choose one of the account's calendars.")
	}
	client, access, err := a.accountAccess(ctx, store, account)
	if err != nil {
		return calendarcore.EventSet{}, err
	}
	coverageStart, coverageEnd := calendarcore.CoverageAround(importedAt)
	events, err := client.Events(ctx, access, calendarID, coverageStart, coverageEnd)
	if err != nil {
		a.forgetRefusedAccess(account.AccountID, err)
		return calendarcore.EventSet{}, asSentence(err)
	}
	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = "Calendar from " + account.Label
	}
	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		zoneID = localZoneID()
	}
	set, err := calendarcore.AccountEventSet(calendarcore.ParseOptions{
		SourceID:      stableCalendarID("calendar_source_account", account.AccountID+"\n"+calendarID),
		SourceLabel:   label,
		Kind:          calendarcore.SourceICS,
		ImportedAt:    importedAt,
		CoverageStart: coverageStart,
		CoverageEnd:   coverageEnd,
		DefaultZoneID: zoneID,
	}, events)
	if err != nil {
		return calendarcore.EventSet{}, fmt.Errorf("The calendar from %s could not be kept: %w", account.Label, err)
	}
	return set, nil
}

// keepAccountCalendar keeps a calendar from an account with the link that
// says where it comes from.
func keepAccountCalendar(accountID, calendarID string) calendarKeep {
	return func(ctx context.Context, store *storage.Store, set calendarcore.EventSet) error {
		return store.ReplaceAccountCalendar(ctx, accountID, calendarID, set.Sources[0], set.Events)
	}
}

// refreshAccountCalendars reads every calendar brought in from an account
// once, refreshing each account's access once, and reports whether there
// was any to read.
func (a *App) refreshAccountCalendars(ctx context.Context, store *storage.Store) bool {
	calendars, err := store.AccountCalendars(ctx)
	if err != nil || len(calendars) == 0 {
		return false
	}
	accounts, err := store.CalendarAccounts(ctx)
	if err != nil {
		return false
	}
	for _, account := range accounts {
		var kept []storage.AccountCalendar
		for _, calendar := range calendars {
			if calendar.AccountID == account.AccountID {
				kept = append(kept, calendar)
			}
		}
		if len(kept) == 0 || ctx.Err() != nil {
			continue
		}
		if _, _, err := a.accountAccess(ctx, store, account); err != nil {
			now := a.currentTime().UTC()
			for _, calendar := range kept {
				_ = store.RecordAccountCalendarFailure(ctx, calendar.SourceID, now, err.Error())
			}
			continue
		}
		for _, calendar := range kept {
			_, _ = a.refreshAccountCalendar(ctx, store, account, calendar)
		}
	}
	return true
}

// refreshAccountCalendar replaces a calendar's copy from its account, or
// records why it could not, keeping the copy it has.
func (a *App) refreshAccountCalendar(ctx context.Context, store *storage.Store, account storage.CalendarAccount, calendar storage.AccountCalendar) (CalendarImportDTO, error) {
	now := a.currentTime().UTC()
	set, err := a.fetchAccountCalendar(ctx, store, account,
		AccountCalendarInput{CalendarID: calendar.CalendarID, Label: calendar.Label}, now)
	var result CalendarImportDTO
	if err == nil {
		result, err = a.calendarImportResult(ctx, set, keepAccountCalendar(account.AccountID, calendar.CalendarID))
	}
	if err != nil {
		_ = store.RecordAccountCalendarFailure(ctx, calendar.SourceID, now, err.Error())
		return CalendarImportDTO{}, err
	}
	return result, nil
}

// accountCalendar finds the account a source was brought in from.
func accountCalendar(ctx context.Context, store *storage.Store, sourceID string) (storage.CalendarAccount, storage.AccountCalendar, bool, error) {
	calendars, err := store.AccountCalendars(ctx)
	if err != nil {
		return storage.CalendarAccount{}, storage.AccountCalendar{}, false, err
	}
	for _, calendar := range calendars {
		if calendar.SourceID != sourceID {
			continue
		}
		accounts, err := store.CalendarAccounts(ctx)
		if err != nil {
			return storage.CalendarAccount{}, storage.AccountCalendar{}, false, err
		}
		for _, account := range accounts {
			if account.AccountID == calendar.AccountID {
				return account, calendar, true, nil
			}
		}
	}
	return storage.CalendarAccount{}, storage.AccountCalendar{}, false, nil
}

// calendarAccountSources says, for each calendar brought in from an account,
// which account and how its last refresh went.
func calendarAccountSources(ctx context.Context, store *storage.Store, now time.Time, location *time.Location) (map[string]*CalendarAccountSourceDTO, error) {
	calendars, err := store.AccountCalendars(ctx)
	if err != nil || len(calendars) == 0 {
		return nil, err
	}
	accounts, err := store.CalendarAccounts(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]storage.CalendarAccount, len(accounts))
	for _, account := range accounts {
		byID[account.AccountID] = account
	}
	sources := make(map[string]*CalendarAccountSourceDTO, len(calendars))
	for _, calendar := range calendars {
		account := byID[calendar.AccountID]
		sources[calendar.SourceID] = &CalendarAccountSourceDTO{
			Provider:       account.Provider,
			AccountLabel:   account.Label,
			RefreshedLabel: calendarRefreshedLabel(calendar.RefreshedAt, now, location),
			Failure:        calendar.Failure,
		}
	}
	return sources, nil
}

func calendarAccountDTO(account storage.CalendarAccount, calendars int, writing bool) CalendarAccountDTO {
	return CalendarAccountDTO{
		AccountID:      account.AccountID,
		Provider:       account.Provider,
		Label:          account.Label,
		ConnectedLabel: "Signed in " + account.ConnectedAt.In(displayLocation()).Format("Jan 2, 2006"),
		Calendars:      calendars,
		Writing:        writing,
		Problem:        account.Problem,
	}
}

// accountWrittenTo names the account whose calendar accepted times are
// written to, if any is.
func accountWrittenTo(ctx context.Context, store *storage.Store) (string, error) {
	target, on, err := store.CalendarWriteTarget(ctx)
	if err != nil || !on {
		return "", err
	}
	return target.AccountID, nil
}

// accountCalendarsDTO lists an account's calendars, its main one first.
func accountCalendarsDTO(ctx context.Context, store *storage.Store, account storage.CalendarAccount, calendars []calendaraccounts.Calendar) (CalendarAccountCalendarsDTO, error) {
	kept, err := store.AccountCalendars(ctx)
	if err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	sources := make(map[string]string)
	for _, calendar := range kept {
		if calendar.AccountID == account.AccountID {
			sources[calendar.CalendarID] = calendar.SourceID
		}
	}
	choices := make([]AccountCalendarChoiceDTO, 0, len(calendars))
	for _, calendar := range calendars {
		name := strings.TrimSpace(calendar.Name)
		if name == "" {
			name = calendar.ID
		}
		choices = append(choices, AccountCalendarChoiceDTO{
			CalendarID: calendar.ID, Name: name, Primary: calendar.Primary, SourceID: sources[calendar.ID],
		})
	}
	sort.SliceStable(choices, func(i, j int) bool {
		if choices[i].Primary != choices[j].Primary {
			return choices[i].Primary
		}
		return strings.ToLower(choices[i].Name) < strings.ToLower(choices[j].Name)
	})
	writing, err := accountWrittenTo(ctx, store)
	if err != nil {
		return CalendarAccountCalendarsDTO{}, err
	}
	return CalendarAccountCalendarsDTO{
		Account:   calendarAccountDTO(account, len(sources), writing == account.AccountID),
		Calendars: choices,
	}, nil
}

// accountLabel names an account by its owner's address, which its main
// calendar carries.
func accountLabel(provider calendaraccounts.Provider, calendars []calendaraccounts.Calendar) string {
	owner := ""
	for _, calendar := range calendars {
		if address := strings.TrimSpace(calendar.Owner); address != "" && (owner == "" || calendar.Primary) {
			owner = address
		}
	}
	if owner == "" || len(owner) > 320 {
		if provider == calendaraccounts.Microsoft {
			return "Microsoft account"
		}
		return "Google account"
	}
	return owner
}

// asSentence reads an error from the provider package, written in Go's lower
// case, as a sentence for the window. One that begins with a host name, as
// "oauth2.googleapis.com could not be reached", keeps it as it is spelled.
func asSentence(err error) error {
	message := strings.TrimSpace(err.Error())
	first, size := utf8.DecodeRuneInString(message)
	if size == 0 {
		return err
	}
	if word, _, _ := strings.Cut(message, " "); !hostName.MatchString(word) {
		message = string(unicode.ToUpper(first)) + message[size:]
	}
	if !strings.HasSuffix(message, ".") {
		message += "."
	}
	return errors.New(message)
}

// hostName is a host as an error names it: lowercase labels joined by dots.
var hostName = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)+$`)
