import { useId, useState } from "react";
import type { CalendarAccountProvider } from "../data/calendar";
import { cancelCalendarSignIn, providerNames } from "../data/calendarAccounts";
import {
  disableCalendarWriteBack,
  enableAccountWriteBack,
  enableCalendarWriteBack,
  resolveCalendarWriteConflict,
  retryCalendarWrites,
  type CalendarWriteBack,
  type CalendarWriteDecision,
  type CalendarWriteProblem,
} from "../data/calendarWriteBack";
import { clockTime, dayInSentence, relativeDay } from "../utils/relativeTime";

// Writing accepted times to a calendar (ADR-0053), set up on the calendar's
// own row in Data Sources › Calendars: first the offer with what it does and
// the sign-in it needs, then how it is going. A calendar from a Google or
// Microsoft account needs no sign-in typed here: the account is asked, in the
// browser, to allow adding events (ADR-0065).

function failure(reason: unknown, fallback: string) {
  return reason instanceof Error && reason.message ? reason.message : fallback;
}

// The offer for a calendar from an account: what writing does, then the
// browser asks the owner, once, to allow it.
function AccountWriteBackForm({
  sourceId,
  label,
  provider,
  onStarted,
  onCancel,
}: {
  sourceId: string;
  label: string;
  provider: CalendarAccountProvider;
  onStarted: (status: CalendarWriteBack) => void;
  onCancel: () => void;
}) {
  const id = useId();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const name = providerNames[provider];

  const start = () => {
    if (busy) return;
    setBusy(true);
    setError("");
    void enableAccountWriteBack(sourceId).then(
      (status) => {
        setBusy(false);
        onStarted(status);
      },
      (reason: unknown) => {
        setBusy(false);
        setError(failure(reason, "Writing could not be turned on."));
      },
    );
  };

  return (
    <section className="calendar-write" aria-labelledby={`${id}-title`}>
      <strong id={`${id}-title`}>Write accepted times to {label}</strong>
      <p>
        Each time you accept is added to {label} as ZeitBoard’s own event, named after its task, and
        removed if you undo it. Your own events are never changed. If you change one of ZeitBoard’s
        events there, it asks you before removing it.
      </p>
      <small>
        The first time, {name} asks in your browser whether ZeitBoard may add events to{" "}
        {provider === "google" ? "calendars you own" : "your calendars"}. Nothing else is asked for,
        and the account’s sign-in stays on this computer.
      </small>
      {busy ? (
        <div className="calendar-account-waiting" role="status">
          <span>If your browser asks, allow ZeitBoard to add events.</span>
          <button
            className="button ghost compact"
            type="button"
            onClick={() => void cancelCalendarSignIn()}
          >
            Cancel
          </button>
        </div>
      ) : (
        <div className="calendar-import-actions">
          <button className="button primary compact" type="button" onClick={start}>
            Allow writing
          </button>
          <button className="button ghost compact" type="button" onClick={onCancel}>
            Cancel
          </button>
        </div>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}

export function CalendarWriteBackForm({
  sourceId,
  label,
  provider,
  onStarted,
  onCancel,
}: {
  sourceId: string;
  label: string;
  /** Set for a calendar brought in from an account. */
  provider?: CalendarAccountProvider;
  onStarted: (status: CalendarWriteBack) => void;
  onCancel: () => void;
}) {
  if (provider) {
    return (
      <AccountWriteBackForm
        sourceId={sourceId}
        label={label}
        provider={provider}
        onStarted={onStarted}
        onCancel={onCancel}
      />
    );
  }
  return (
    <CalDAVWriteBackForm
      sourceId={sourceId}
      label={label}
      onStarted={onStarted}
      onCancel={onCancel}
    />
  );
}

function CalDAVWriteBackForm({
  sourceId,
  label,
  onStarted,
  onCancel,
}: {
  sourceId: string;
  label: string;
  onStarted: (status: CalendarWriteBack) => void;
  onCancel: () => void;
}) {
  const id = useId();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const start = () => {
    if (busy) return;
    setBusy(true);
    setError("");
    void enableCalendarWriteBack({ sourceId, username: username.trim(), password }).then(
      (status) => {
        setBusy(false);
        setPassword("");
        onStarted(status);
      },
      (reason: unknown) => {
        setBusy(false);
        setPassword("");
        setError(failure(reason, "Writing could not be turned on."));
      },
    );
  };

  return (
    <form
      className="calendar-write"
      aria-labelledby={`${id}-title`}
      onSubmit={(event) => {
        event.preventDefault();
        start();
      }}
    >
      <strong id={`${id}-title`}>Write accepted times to {label}</strong>
      <p>
        Each time you accept is added to {label} as ZeitBoard’s own event, named after its task, and
        removed if you undo it. Whoever hosts the calendar can see those names. Your own events are
        never changed. If you change one of ZeitBoard’s events there, it asks you before removing
        it.
      </p>
      <div className="calendar-credential-row">
        <label>
          <span>Username</span>
          <input
            autoComplete="username"
            value={username}
            disabled={busy}
            onChange={(event) => setUsername(event.currentTarget.value)}
          />
        </label>
        <label>
          <span>App password</span>
          <input
            type="password"
            autoComplete="current-password"
            value={password}
            disabled={busy}
            onChange={(event) => setPassword(event.currentTarget.value)}
          />
        </label>
      </div>
      <small>
        ZeitBoard keeps this sign-in on this computer, in its private files, until you stop writing.
      </small>
      <div className="calendar-import-actions">
        <button className="button primary compact" type="submit" disabled={busy}>
          {busy ? "Checking the sign-in…" : "Start writing"}
        </button>
        <button className="button ghost compact" type="button" disabled={busy} onClick={onCancel}>
          Cancel
        </button>
      </div>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}

// "Today 3:30 PM" and "Trying again tonight at 6:05 PM", as Plan words times.
function startWording(startAt: string, now: Date) {
  const start = new Date(startAt);
  return `${relativeDay(start, now)} ${clockTime(start)}`;
}

function retryWording(retryAt: string, now: Date) {
  const retry = new Date(retryAt);
  return `Trying again ${dayInSentence(retry, now)} at ${clockTime(retry)}.`;
}

function ProblemRow({
  problem,
  now,
  busy,
  onDecide,
}: {
  problem: CalendarWriteProblem;
  now: Date;
  busy: boolean;
  onDecide: (decision: CalendarWriteDecision) => void;
}) {
  return (
    <li className="calendar-write-problem" data-conflict={problem.conflict}>
      <strong>{problem.title}</strong>
      {problem.startAt && <span>{startWording(problem.startAt, now)}</span>}
      <p>
        {problem.detail}
        {problem.retryAt && ` ${retryWording(problem.retryAt, now)}`}
      </p>
      {problem.conflict && (
        <div className="calendar-import-actions">
          {problem.removing ? (
            <>
              <button
                className="button secondary compact"
                type="button"
                disabled={busy}
                onClick={() => onDecide("remove")}
              >
                Remove it anyway
              </button>
              <button
                className="button ghost compact"
                type="button"
                disabled={busy}
                onClick={() => onDecide("keep")}
              >
                Keep it in the calendar
              </button>
            </>
          ) : (
            <button
              className="button secondary compact"
              type="button"
              disabled={busy}
              onClick={() => onDecide("keep")}
            >
              Leave it out of the calendar
            </button>
          )}
        </div>
      )}
    </li>
  );
}

export function CalendarWriteBackStatus({
  status,
  onChanged,
}: {
  status: CalendarWriteBack;
  onChanged: (status: CalendarWriteBack) => void;
}) {
  const id = useId();
  const [busy, setBusy] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [error, setError] = useState("");
  const retrying = status.problems.some((problem) => !problem.conflict);
  const now = new Date();

  const run = (action: () => Promise<CalendarWriteBack>, fallback: string) => {
    if (busy) return;
    setBusy(true);
    setError("");
    void action().then(
      (next) => {
        setBusy(false);
        setStopping(false);
        onChanged(next);
      },
      (reason: unknown) => {
        setBusy(false);
        setError(failure(reason, fallback));
      },
    );
  };

  return (
    <section className="calendar-write" aria-labelledby={`${id}-title`}>
      <strong id={`${id}-title`}>Accepted times are written here</strong>
      <p aria-live="polite">{status.summary}</p>
      {status.problems.length > 0 && (
        <ul className="calendar-write-problems" aria-label="Not written yet">
          {status.problems.map((problem) => (
            <ProblemRow
              key={problem.eventId}
              problem={problem}
              now={now}
              busy={busy}
              onDecide={(decision) =>
                run(
                  () => resolveCalendarWriteConflict(problem.eventId, decision),
                  "That could not be settled.",
                )
              }
            />
          ))}
        </ul>
      )}
      {stopping ? (
        <div className="calendar-write-stop" role="group" aria-labelledby={`${id}-stop`}>
          <p id={`${id}-stop`}>
            Stop writing to {status.label}? What ZeitBoard wrote stays there, and undoing an
            accepted time will no longer remove it.{" "}
            {status.provider
              ? `The ${providerNames[status.provider]} account stays signed in for reading; removing it withdraws ZeitBoard’s access.`
              : "The sign-in is erased."}
          </p>
          <div className="calendar-import-actions">
            <button
              className="button secondary compact"
              type="button"
              disabled={busy}
              onClick={() => run(disableCalendarWriteBack, "Writing could not be stopped.")}
            >
              {busy ? "Stopping…" : "Stop writing"}
            </button>
            <button
              className="button ghost compact"
              type="button"
              disabled={busy}
              onClick={() => setStopping(false)}
            >
              Keep writing
            </button>
          </div>
        </div>
      ) : (
        <div className="calendar-import-actions">
          {retrying && (
            <button
              className="button secondary compact"
              type="button"
              disabled={busy}
              onClick={() => run(retryCalendarWrites, "Trying again did not start.")}
            >
              Try again now
            </button>
          )}
          <button
            className="button ghost compact"
            type="button"
            disabled={busy}
            onClick={() => setStopping(true)}
          >
            Stop writing
          </button>
        </div>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
