package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	calendarcore "non24.app/core/calendar"
)

// Calendar accounts (ADR-0064): a Google or Microsoft account signed in
// through the owner's own app registration, and the calendars brought in from
// it. The refresh token is a credential, kept here like the CalDAV write-back
// sign-in and never synced, exported or shown; removing the account removes
// it with every calendar brought in from it.

// ErrCalendarAccountNotFound reports an account that is not, or no longer,
// signed in.
var ErrCalendarAccountNotFound = errors.New("calendar account is not signed in")

// CalendarAccount is a signed-in account.
type CalendarAccount struct {
	AccountID    string
	Provider     string
	Label        string
	ClientID     string
	ClientSecret string
	RefreshToken string
	ConnectedAt  time.Time

	// Scopes is what the owner consented to at sign-in, as the provider
	// stated it.
	Scopes string

	// Problem says the provider ended the sign-in, so that only signing in
	// again restores it; it is empty otherwise.
	Problem string
}

// AccountCalendar is a calendar brought in from an account, and how its last
// refresh went.
type AccountCalendar struct {
	SourceID    string
	AccountID   string
	CalendarID  string
	Label       string
	RefreshedAt time.Time
	AttemptedAt time.Time
	Failure     string
}

func (s *Store) initializeCalendarAccounts(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS local_calendar_accounts (
			account_id TEXT PRIMARY KEY,
			provider TEXT NOT NULL CHECK(provider IN ('google', 'microsoft')),
			label TEXT NOT NULL,
			client_id TEXT NOT NULL,
			client_secret TEXT NOT NULL DEFAULT '',
			refresh_token TEXT NOT NULL,
			connected_at TEXT NOT NULL,
			scopes TEXT NOT NULL DEFAULT '',
			problem TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS local_calendar_account_calendars (
			source_id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL,
			calendar_id TEXT NOT NULL,
			refreshed_at TEXT NOT NULL,
			attempted_at TEXT NOT NULL,
			failure TEXT NOT NULL DEFAULT '',
			UNIQUE(account_id, calendar_id),
			FOREIGN KEY(source_id) REFERENCES local_calendar_sources(source_id) ON DELETE CASCADE,
			FOREIGN KEY(account_id) REFERENCES local_calendar_accounts(account_id) ON DELETE CASCADE
		)`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate calendar accounts: %w", err)
		}
	}
	return nil
}

func validateCalendarAccount(account CalendarAccount) error {
	switch {
	case !contractIdentifier.MatchString(account.AccountID):
		return errors.New("a calendar account needs an identifier")
	case account.Provider != "google" && account.Provider != "microsoft":
		return fmt.Errorf("calendar accounts from %q are not read", account.Provider)
	case strings.TrimSpace(account.Label) == "" || len(account.Label) > 320:
		return errors.New("a calendar account needs a name")
	case strings.TrimSpace(account.ClientID) == "" || len(account.ClientID) > 512 || len(account.ClientSecret) > 512:
		return errors.New("a calendar account needs the client id it signed in with")
	case strings.TrimSpace(account.RefreshToken) == "" || len(account.RefreshToken) > 8192:
		return errors.New("a calendar account needs lasting access")
	case len(account.Scopes) > 2048:
		return errors.New("a calendar account's scopes are too long")
	case account.ConnectedAt.IsZero():
		return errors.New("a calendar account needs when it was connected")
	}
	return nil
}

// SaveCalendarAccount keeps an account, replacing a sign-in kept before and
// the problem that sign-in had.
func (s *Store) SaveCalendarAccount(ctx context.Context, account CalendarAccount) error {
	if err := validateCalendarAccount(account); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO local_calendar_accounts(
			account_id, provider, label, client_id, client_secret, refresh_token, connected_at, scopes)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET label = excluded.label, client_id = excluded.client_id,
			client_secret = excluded.client_secret, refresh_token = excluded.refresh_token,
			connected_at = excluded.connected_at, scopes = excluded.scopes, problem = ''`,
		account.AccountID, account.Provider, account.Label, account.ClientID, account.ClientSecret,
		account.RefreshToken, formatSQLiteTime(account.ConnectedAt), strings.TrimSpace(account.Scopes))
	return err
}

// RotateCalendarAccountToken keeps the refresh token a provider replaced.
func (s *Store) RotateCalendarAccountToken(ctx context.Context, accountID, refresh string) error {
	if strings.TrimSpace(refresh) == "" || len(refresh) > 8192 {
		return errors.New("a calendar account needs lasting access")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE local_calendar_accounts SET refresh_token = ? WHERE account_id = ?`, refresh, accountID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed == 0 {
		return errors.Join(err, ErrCalendarAccountNotFound)
	}
	return nil
}

// RecordCalendarAccountProblem notes that the provider ended an account's
// sign-in, or clears that with an empty problem.
func (s *Store) RecordCalendarAccountProblem(ctx context.Context, accountID, problem string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE local_calendar_accounts SET problem = ? WHERE account_id = ?`, strings.TrimSpace(problem), accountID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed == 0 {
		return errors.Join(err, ErrCalendarAccountNotFound)
	}
	return nil
}

// CalendarAccounts lists the signed-in accounts, credentials included, for
// the desktop to read calendars with. Nothing that reaches the window may
// carry them.
func (s *Store) CalendarAccounts(ctx context.Context) ([]CalendarAccount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT account_id, provider, label, client_id, client_secret, refresh_token,
			connected_at, scopes, problem
		FROM local_calendar_accounts ORDER BY provider, label, account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []CalendarAccount
	for rows.Next() {
		var account CalendarAccount
		var connected string
		if err := rows.Scan(&account.AccountID, &account.Provider, &account.Label, &account.ClientID,
			&account.ClientSecret, &account.RefreshToken, &connected, &account.Scopes, &account.Problem); err != nil {
			return nil, err
		}
		if account.ConnectedAt, err = parseCalendarTime(connected); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

// ReplaceAccountCalendar keeps a fetched copy of a calendar from an account,
// with the link that says where it comes from.
func (s *Store) ReplaceAccountCalendar(ctx context.Context, accountID, calendarID string, source calendarcore.Source, events []calendarcore.Event) error {
	if strings.TrimSpace(calendarID) == "" || len(calendarID) > 1024 {
		return errors.New("an account calendar needs the provider's id for it")
	}
	if source.Kind != calendarcore.SourceICS {
		return errors.New("an account calendar is kept as a read-only imported source")
	}
	if err := validateImportedCalendar(source, events, ""); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var account string
	if err := tx.QueryRowContext(ctx, `SELECT account_id FROM local_calendar_accounts WHERE account_id = ?`, accountID).Scan(&account); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCalendarAccountNotFound
		}
		return err
	}
	if err := replaceImportedCalendarTx(ctx, tx, source, events, ""); err != nil {
		return err
	}
	fetched := formatSQLiteTime(source.LastImportedAt)
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_calendar_account_calendars(
			source_id, account_id, calendar_id, refreshed_at, attempted_at, failure)
		VALUES(?, ?, ?, ?, ?, '')
		ON CONFLICT(source_id) DO UPDATE SET refreshed_at = excluded.refreshed_at,
			attempted_at = excluded.attempted_at, failure = ''`,
		source.SourceID, accountID, calendarID, fetched, fetched); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordAccountCalendarFailure notes a refresh that failed; the copy kept
// from the last good one stays.
func (s *Store) RecordAccountCalendarFailure(ctx context.Context, sourceID string, at time.Time, failure string) error {
	if strings.TrimSpace(failure) == "" {
		return errors.New("a failed refresh needs its reason")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE local_calendar_account_calendars SET attempted_at = ?, failure = ? WHERE source_id = ?`,
		formatSQLiteTime(at), failure, sourceID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed == 0 {
		return errors.Join(err, ErrCalendarSourceNotFound)
	}
	return nil
}

// AccountCalendars lists the calendars brought in from accounts.
func (s *Store) AccountCalendars(ctx context.Context) ([]AccountCalendar, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT link.source_id, link.account_id, link.calendar_id, source.label,
			link.refreshed_at, link.attempted_at, link.failure
		FROM local_calendar_account_calendars AS link
		JOIN local_calendar_sources AS source ON source.source_id = link.source_id
		ORDER BY source.label, link.source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var calendars []AccountCalendar
	for rows.Next() {
		var item AccountCalendar
		var refreshed, attempted string
		if err := rows.Scan(&item.SourceID, &item.AccountID, &item.CalendarID, &item.Label, &refreshed, &attempted, &item.Failure); err != nil {
			return nil, err
		}
		if item.RefreshedAt, err = parseCalendarTime(refreshed); err != nil {
			return nil, err
		}
		if item.AttemptedAt, err = parseCalendarTime(attempted); err != nil {
			return nil, err
		}
		calendars = append(calendars, item)
	}
	return calendars, rows.Err()
}

// RemoveCalendarAccount erases an account's sign-in and every calendar
// brought in from it, and compacts the database so the token is gone too.
func (s *Store) RemoveCalendarAccount(ctx context.Context, accountID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_calendar_sources WHERE source_id IN (
		SELECT source_id FROM local_calendar_account_calendars WHERE account_id = ?)`, accountID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM local_calendar_accounts WHERE account_id = ?`, accountID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed == 0 {
		return errors.Join(err, ErrCalendarAccountNotFound)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.compactDeletedData(ctx)
}
