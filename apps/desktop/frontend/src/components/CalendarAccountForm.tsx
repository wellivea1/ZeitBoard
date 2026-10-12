import { useCallback, useEffect, useId, useState, type ReactNode } from "react";
import { ConfirmDelete } from "./ConfirmDelete";
import {
  calendarDataChangedEvent,
  type CalendarAccountProvider,
  type CalendarImportReport,
} from "../data/calendar";
import {
  addAccountCalendar,
  cancelCalendarSignIn,
  connectCalendarAccount,
  listAccountCalendars,
  loadCalendarAccounts,
  previewAccountCalendar,
  providerNames,
  reconnectCalendarAccount,
  removeCalendarAccount,
  type AccountCalendarChoice,
  type CalendarAccount,
  type CalendarAccountCalendars,
  type CalendarAccountInput,
} from "../data/calendarAccounts";

// A Google or Microsoft account, signed in through the owner's own app
// registration (ADR-0064): the accounts signed in, each opening onto the
// calendars it holds, then signing in to another. ZeitBoard ships no client
// of its own, so the steps to register one sit beside the form.

const registrationSteps: Record<CalendarAccountProvider, string[]> = {
  google: [
    "In the Google Cloud console (console.cloud.google.com), create a project and enable the Google Calendar API.",
    "Under Google Auth Platform, make the audience External and add your own address as a test user.",
    "Create a client of the type Desktop app, then paste its client ID and client secret here.",
  ],
  microsoft: [
    "In Microsoft Entra (entra.microsoft.com), add a new app under App registrations.",
    "For who may sign in, choose accounts in any organizational directory and personal Microsoft accounts.",
    "For its redirect URI, choose Public client/native (mobile & desktop) and enter http://localhost.",
    "Paste its Application (client) ID here. Microsoft needs no secret for it.",
  ],
};

const registrationNotes: Record<CalendarAccountProvider, string> = {
  google:
    "While the app is in testing, Google ends its sign-ins after seven days. Publishing it keeps you signed in; Google will call it unverified, since only you use it.",
  microsoft: "Microsoft keeps the sign-in going for as long as you use the account.",
};

function calendarsBroughtIn(count: number) {
  if (count === 0) return "no calendars brought in";
  return count === 1 ? "1 calendar brought in" : `${count} calendars brought in`;
}

function AccountRow({
  account,
  open,
  disabled,
  children,
  onChoose,
  onReconnect,
  onRemove,
}: {
  account: CalendarAccount;
  open: boolean;
  disabled: boolean;
  children?: ReactNode;
  onChoose: () => void;
  onReconnect: () => void;
  onRemove: () => void;
}) {
  return (
    <li className="calendar-account-row" data-provider={account.provider}>
      <div className="calendar-source-name">
        <strong>{account.label}</strong>
        <span>
          {providerNames[account.provider]} account · {account.connectedLabel} ·{" "}
          {calendarsBroughtIn(account.calendars)}
        </span>
        {account.problem && <small className="calendar-source-failure">{account.problem}</small>}
      </div>
      <div className="calendar-source-actions">
        {account.problem ? (
          <button
            className="button secondary compact"
            type="button"
            disabled={disabled}
            onClick={onReconnect}
          >
            Sign in again
          </button>
        ) : (
          <button
            className="button ghost compact"
            type="button"
            disabled={disabled}
            aria-expanded={open}
            aria-label={`Calendars in ${account.label}`}
            onClick={onChoose}
          >
            {open ? "Hide calendars" : "Calendars"}
          </button>
        )}
        <button
          className="button ghost compact"
          type="button"
          disabled={disabled}
          aria-label={`Remove ${account.label}`}
          onClick={onRemove}
        >
          Remove
        </button>
      </div>
      {children}
    </li>
  );
}

// The calendars an account holds: previewed first, then brought in.
function AccountCalendarList({
  calendars,
  previewed,
  disabled,
  onPreview,
  onBringIn,
}: {
  calendars: AccountCalendarChoice[];
  previewed?: string;
  disabled: boolean;
  onPreview: (calendar: AccountCalendarChoice) => void;
  onBringIn: (calendar: AccountCalendarChoice) => void;
}) {
  if (calendars.length === 0) {
    return <p className="calendar-account-calendars-empty">This account holds no calendars.</p>;
  }
  return (
    <ul className="calendar-account-calendars">
      {calendars.map((calendar) => (
        <li key={calendar.calendarId}>
          <div className="calendar-source-name">
            <strong>{calendar.name}</strong>
            {calendar.primary && <span>Main calendar</span>}
          </div>
          <div className="calendar-source-actions">
            {calendar.sourceId ? (
              <span className="calendar-account-kept">Brought in · read every hour</span>
            ) : previewed === calendar.calendarId ? (
              <button
                className="button primary compact"
                type="button"
                disabled={disabled}
                onClick={() => onBringIn(calendar)}
              >
                Bring in {calendar.name}
              </button>
            ) : (
              <button
                className="button secondary compact"
                type="button"
                disabled={disabled}
                aria-label={`Preview ${calendar.name}`}
                onClick={() => onPreview(calendar)}
              >
                Preview
              </button>
            )}
          </div>
        </li>
      ))}
    </ul>
  );
}

// Signing in: the whole form for a first account, and behind a button once
// there is one, so the calendars being chosen stay beside their preview.
function SignInForm({
  available,
  busy,
  waiting,
  first,
  onSignIn,
  onCancel,
}: {
  available: boolean;
  busy: boolean;
  waiting: boolean;
  first: boolean;
  /** Resolves to whether the account was signed in. */
  onSignIn: (input: CalendarAccountInput) => Promise<boolean>;
  onCancel: () => void;
}) {
  const id = useId();
  const [opened, setOpened] = useState(false);
  const [provider, setProvider] = useState<CalendarAccountProvider>("google");
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const google = provider === "google";
  const locked = !available || busy || waiting;
  const ready = !locked && Boolean(clientId.trim()) && (!google || Boolean(clientSecret.trim()));

  const signIn = () => {
    if (!ready) return;
    void onSignIn({
      provider,
      clientId: clientId.trim(),
      clientSecret: google ? clientSecret.trim() : "",
    }).then((signedIn) => {
      if (!signedIn) return;
      setOpened(false);
      setClientId("");
      setClientSecret("");
    });
  };

  if (!first && !opened && !waiting) {
    return (
      <div className="calendar-account-signin">
        <div className="calendar-import-actions">
          <button
            className="button secondary compact"
            type="button"
            disabled={locked}
            onClick={() => setOpened(true)}
          >
            Sign in to another account
          </button>
        </div>
      </div>
    );
  }

  return (
    <section className="calendar-account-signin" aria-labelledby={`${id}-title`}>
      <strong id={`${id}-title`}>
        {first ? "Sign in to an account" : "Sign in to another account"}
      </strong>
      <div className="calendar-account-providers" role="radiogroup" aria-label="Provider">
        {(["google", "microsoft"] as const).map((each) => (
          <button
            key={each}
            type="button"
            role="radio"
            aria-checked={provider === each}
            disabled={waiting}
            onClick={() => setProvider(each)}
          >
            {providerNames[each]}
          </button>
        ))}
      </div>
      <label>
        <span>Client ID</span>
        <input
          value={clientId}
          autoComplete="off"
          spellCheck={false}
          disabled={locked}
          onChange={(event) => setClientId(event.currentTarget.value)}
        />
      </label>
      {google && (
        <label>
          <span>Client secret</span>
          <input
            type="password"
            value={clientSecret}
            autoComplete="off"
            disabled={locked}
            onChange={(event) => setClientSecret(event.currentTarget.value)}
          />
        </label>
      )}
      <details className="calendar-account-steps">
        <summary>Registering ZeitBoard with {providerNames[provider]}</summary>
        <ol>
          {registrationSteps[provider].map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
        <p>{registrationNotes[provider]}</p>
      </details>
      <small>
        ZeitBoard signs in through the app you register, asks only to read your calendars, and keeps
        the sign-in on this computer. It is never shown, synced or exported.
      </small>
      {waiting ? (
        <div className="calendar-account-waiting" role="status">
          <span>Finish signing in in your browser. ZeitBoard waits five minutes.</span>
          <button className="button ghost compact" type="button" onClick={onCancel}>
            Cancel
          </button>
        </div>
      ) : (
        <div className="calendar-import-actions">
          <button
            className="button primary compact"
            type="button"
            disabled={!ready}
            onClick={signIn}
          >
            Sign in with {providerNames[provider]}
          </button>
          {!first && (
            <button className="button ghost compact" type="button" onClick={() => setOpened(false)}>
              Not now
            </button>
          )}
        </div>
      )}
    </section>
  );
}

function useCalendarAccounts() {
  const [accounts, setAccounts] = useState<CalendarAccount[]>([]);
  const [version, setVersion] = useState(0);
  const reload = useCallback(() => setVersion((current) => current + 1), []);
  useEffect(() => {
    let current = true;
    void loadCalendarAccounts().then(
      (loaded) => {
        if (current) setAccounts(loaded);
      },
      () => {
        if (current) setAccounts([]);
      },
    );
    return () => {
      current = false;
    };
  }, [version]);
  useEffect(() => {
    window.addEventListener(calendarDataChangedEvent, reload);
    return () => window.removeEventListener(calendarDataChangedEvent, reload);
  }, [reload]);
  return [accounts, reload] as const;
}

function failure(reason: unknown, fallback: string) {
  return reason instanceof Error ? reason.message : fallback;
}

export function CalendarAccountForm({
  available,
  busy,
  zoneId,
  onBusy,
  onReport,
  onError,
  onChanged,
}: {
  available: boolean;
  busy: boolean;
  zoneId: string;
  onBusy: (busy: boolean) => void;
  onReport: (report: CalendarImportReport | null) => void;
  onError: (message: string) => void;
  onChanged: () => void;
}) {
  const [accounts, reload] = useCalendarAccounts();
  const [choosing, setChoosing] = useState<CalendarAccountCalendars | undefined>();
  const [previewed, setPreviewed] = useState<string | undefined>();
  const [waiting, setWaiting] = useState(false);
  const [removing, setRemoving] = useState<CalendarAccount | undefined>();
  const [status, setStatus] = useState("");
  const locked = !available || busy || waiting;

  const begin = () => {
    onError("");
    setStatus("");
  };

  const signedIn = (result: CalendarAccountCalendars) => {
    setChoosing(result);
    setPreviewed(undefined);
    onReport(null);
    reload();
  };

  const signIn = (operation: Promise<CalendarAccountCalendars>) => {
    begin();
    onReport(null);
    setWaiting(true);
    return operation.then(
      (result) => {
        setWaiting(false);
        signedIn(result);
        return true;
      },
      (reason: unknown) => {
        setWaiting(false);
        onError(failure(reason, "Signing in did not finish."));
        return false;
      },
    );
  };

  const run = <T,>(operation: () => Promise<T>, done: (result: T) => void, fallback: string) => {
    if (locked) return;
    begin();
    onBusy(true);
    void operation().then(
      (result) => {
        onBusy(false);
        done(result);
      },
      (reason: unknown) => {
        onBusy(false);
        onError(failure(reason, fallback));
      },
    );
  };

  const choose = (account: CalendarAccount) => {
    if (choosing?.account.accountId === account.accountId) {
      setChoosing(undefined);
      return;
    }
    run(
      () => listAccountCalendars(account.accountId),
      signedIn,
      "The calendars could not be read.",
    );
  };

  const input = (calendar: AccountCalendarChoice) => ({
    accountId: choosing?.account.accountId ?? "",
    calendarId: calendar.calendarId,
    label: calendar.name,
    zoneId,
  });

  const preview = (calendar: AccountCalendarChoice) =>
    run(
      () => previewAccountCalendar(input(calendar)),
      (report) => {
        onReport(report);
        setPreviewed(calendar.calendarId);
      },
      "The calendar could not be read.",
    );

  const bringIn = (calendar: AccountCalendarChoice) =>
    run(
      () => addAccountCalendar(input(calendar)),
      (report) => {
        onReport(report);
        setPreviewed(undefined);
        setChoosing(
          (current) =>
            current && {
              ...current,
              calendars: current.calendars.map((each) =>
                each.calendarId === calendar.calendarId
                  ? { ...each, sourceId: report.sourceId }
                  : each,
              ),
            },
        );
        reload();
        onChanged();
      },
      "The calendar could not be brought in.",
    );

  const remove = (account: CalendarAccount) =>
    run(
      () => removeCalendarAccount(account.accountId),
      (message) => {
        setRemoving(undefined);
        setStatus(message);
        if (choosing?.account.accountId === account.accountId) setChoosing(undefined);
        onReport(null);
        reload();
        onChanged();
      },
      "The account could not be removed.",
    );

  return (
    <div className="calendar-import-form calendar-accounts" role="tabpanel">
      {accounts.length > 0 && (
        <ul className="calendar-account-list" aria-label="Signed-in accounts">
          {accounts.map((account) => {
            const open = choosing?.account.accountId === account.accountId;
            return (
              <AccountRow
                key={account.accountId}
                account={account}
                open={open}
                disabled={locked}
                onChoose={() => choose(account)}
                onReconnect={() => void signIn(reconnectCalendarAccount(account.accountId))}
                onRemove={() => {
                  begin();
                  setRemoving(account);
                }}
              >
                {open && choosing && (
                  <AccountCalendarList
                    calendars={choosing.calendars}
                    previewed={previewed}
                    disabled={locked}
                    onPreview={preview}
                    onBringIn={bringIn}
                  />
                )}
              </AccountRow>
            );
          })}
        </ul>
      )}
      {removing && (
        <ConfirmDelete
          question={`Remove ${removing.label} from ZeitBoard?`}
          action="Remove account"
          busyAction="Removing…"
          word="REMOVE"
          busy={busy}
          onConfirm={() => remove(removing)}
          onCancel={() => setRemoving(undefined)}
        >
          <p>
            Its sign-in is erased, and the calendars brought in from it are deleted from ZeitBoard.
            The calendars themselves are not changed.
          </p>
          {removing.writing && (
            <p>
              ZeitBoard also stops writing accepted times to its calendar. What it wrote stays
              there.
            </p>
          )}
        </ConfirmDelete>
      )}
      {status && (
        <p className="form-status" role="status">
          {status}
        </p>
      )}
      <SignInForm
        available={available}
        busy={busy}
        waiting={waiting}
        first={accounts.length === 0}
        onSignIn={(details) => signIn(connectCalendarAccount(details))}
        onCancel={() => void cancelCalendarSignIn()}
      />
    </div>
  );
}
