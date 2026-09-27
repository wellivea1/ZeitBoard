import { useId, useState } from "react";
import {
  disableCalendarWriteBack,
  enableCalendarWriteBack,
  resolveCalendarWriteConflict,
  retryCalendarWrites,
  type CalendarWriteBack,
  type CalendarWriteDecision,
  type CalendarWriteProblem,
} from "../data/calendarWriteBack";
import { clockTime, dayInSentence, relativeDay } from "../utils/relativeTime";

// Writing accepted times to a CalDAV calendar (ADR-0053), set up on the
// calendar's own row in Data Sources › Calendars: first the offer with what it
// does and the sign-in it needs, then how it is going.

function failure(reason: unknown, fallback: string) {
  return reason instanceof Error && reason.message ? reason.message : fallback;
}

export function CalendarWriteBackForm({
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
            accepted time will no longer remove it. The sign-in is erased.
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
