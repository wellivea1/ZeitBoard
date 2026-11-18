import { useEffect, useRef, useState } from "react";
import type { CalendarData, CalendarEventSegment } from "../data/calendar";
import { deleteOwnerEvent, loadOwnerEvent, type OwnerEvent } from "../data/ownerEvents";
import type { ProposalRecord } from "../data/proposals";
import type { ProposalDecision } from "../state/approvals";
import { sleepWindowOver, type SleepWindow } from "../utils/ownerEventForm";
import { blockWording, relativeRange } from "../utils/relativeTime";

// What sits under the Week board: the event you clicked, and the same events
// as a table for anyone who would rather read than look.

/**
 * The details open under the board, which can be below the fold: a click on a
 * block that seemed to do nothing. They come into view as they open.
 */
function useShownOnOpen(key: string) {
  const ref = useRef<HTMLElement>(null);
  useEffect(() => {
    ref.current?.scrollIntoView?.({ block: "nearest" });
  }, [key]);
  return ref;
}

const kickers: Record<CalendarEventSegment["ownership"], string> = {
  app_owned: "A time you accepted",
  imported: "From your calendar",
  owner: "An event you added",
};

/**
 * Change and delete, for an event the owner added (ADR-0068). The event is
 * read again first, so the change or deletion names it as it now stands.
 */
function OwnerEventActions({
  event,
  onChange,
  onDeleted,
  onClose,
}: {
  event: CalendarEventSegment;
  onChange: (event: OwnerEvent) => void;
  onDeleted: (message: string) => void;
  onClose: () => void;
}) {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const run = (action: (current: OwnerEvent) => Promise<void> | void) => {
    setBusy(true);
    setError("");
    void loadOwnerEvent(event.eventId)
      .then(action)
      .then(
        () => setBusy(false),
        (reason: unknown) => {
          setBusy(false);
          setError(reason instanceof Error ? reason.message : "That did not work.");
        },
      );
  };
  if (confirming) {
    return (
      <div className="week-inspector-confirm" role="group" aria-label={`Delete ${event.title}`}>
        <strong>Delete “{event.title}”?</strong>
        <p>
          It goes from ZeitBoard here and, once they sync, from your server and your other devices.
          The planner may then suggest that time.
        </p>
        {error && <p role="alert">{error}</p>}
        <div className="week-inspector-actions">
          <button
            className="button danger"
            type="button"
            disabled={busy}
            onClick={() =>
              run(async (current) => {
                await deleteOwnerEvent(current);
                onDeleted(`Deleted ${event.title}.`);
              })
            }
          >
            {busy ? "Deleting…" : "Delete event"}
          </button>
          <button
            className="button ghost"
            type="button"
            disabled={busy}
            onClick={() => {
              setConfirming(false);
              setError("");
            }}
          >
            Keep it
          </button>
        </div>
      </div>
    );
  }
  return (
    <>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <div className="week-inspector-actions">
        <button
          className="button compact"
          type="button"
          disabled={busy}
          onClick={() => run(onChange)}
        >
          Change
        </button>
        <button
          className="button compact"
          type="button"
          disabled={busy}
          onClick={() => setConfirming(true)}
        >
          Delete
        </button>
        <button className="button ghost" data-quiet type="button" onClick={onClose}>
          Close details
        </button>
      </div>
    </>
  );
}

export function EventInspector({
  event,
  sleepWindows = [],
  onClose,
  onChange,
  onDeleted,
}: {
  event: CalendarEventSegment;
  /** The predicted sleep windows in view, to say when the event falls in one. */
  sleepWindows?: SleepWindow[];
  onClose: () => void;
  /** Given for events the owner can change here. */
  onChange?: (event: OwnerEvent) => void;
  onDeleted?: (message: string) => void;
}) {
  const inSleep =
    event.busy && !event.allDay
      ? sleepWindowOver(sleepWindows, event.startAt, event.endAt)
      : undefined;
  const editable = event.ownership === "owner" && onChange && onDeleted;
  const shown = useShownOnOpen(event.segmentId);
  return (
    <section className="week-inspector" aria-live="polite" aria-label="Selected event" ref={shown}>
      <p className="section-kicker">{kickers[event.ownership]}</p>
      <h2>{event.title}</h2>
      <dl>
        <div>
          <dt>Time</dt>
          <dd>
            {event.allDay
              ? "All day"
              : blockWording(
                  event.startAt,
                  event.endAt,
                  `${event.startLabel} to ${event.endLabel}`,
                )}
          </dd>
        </div>
        <div>
          <dt>Source</dt>
          <dd>{event.sourceLabel}</dd>
        </div>
        {event.location && (
          <div>
            <dt>Location</dt>
            <dd>{event.location}</dd>
          </div>
        )}
        {event.notes && (
          <div>
            <dt>Notes</dt>
            <dd>{event.notes}</dd>
          </div>
        )}
      </dl>
      {inSleep && (
        <p className="week-inspector-why">
          This falls in a predicted sleep window,{" "}
          {relativeRange(new Date(inSleep.startAt), new Date(inSleep.endAt), new Date())}.
        </p>
      )}
      {editable ? (
        <OwnerEventActions
          event={event}
          onChange={onChange}
          onDeleted={onDeleted}
          onClose={onClose}
        />
      ) : (
        <button className="button ghost" type="button" onClick={onClose}>
          Close details
        </button>
      )}
    </section>
  );
}

/** A suggestion opened from the board: when, why, and the two answers. */
export function SuggestionInspector({
  proposal,
  deciding,
  onDecide,
  onClose,
}: {
  proposal: ProposalRecord;
  deciding: boolean;
  onDecide: (decision: ProposalDecision) => void;
  onClose: () => void;
}) {
  const shown = useShownOnOpen(proposal.id);
  return (
    <section
      className="week-inspector"
      aria-live="polite"
      aria-label="Selected suggestion"
      ref={shown}
    >
      <p className="section-kicker">{proposal.createdLabel || "Suggested"}</p>
      <h2>{proposal.title}</h2>
      <p className="week-inspector-when">
        {blockWording(proposal.startAt, proposal.endAt, proposal.to)}
        {proposal.rhythmContext && <span>, {proposal.rhythmContext}</span>}.
      </p>
      {proposal.reasonLabels.length > 0 && (
        <p className="week-inspector-why">{proposal.reasonLabels.join("; ")}.</p>
      )}
      <div className="week-inspector-actions">
        <button
          className="button primary compact"
          type="button"
          disabled={deciding}
          onClick={() => onDecide("approved")}
        >
          Accept
        </button>
        <button
          className="button compact"
          type="button"
          disabled={deciding}
          onClick={() => onDecide("rejected")}
        >
          Decline
        </button>
        <button className="button ghost" data-quiet type="button" onClick={onClose}>
          Close
        </button>
      </div>
    </section>
  );
}

export function EventTable({ data }: { data: CalendarData }) {
  const seen = new Set<string>();
  const rows = data.days.flatMap((day) =>
    day.events
      .filter((event) => {
        if (seen.has(event.eventId)) return false;
        seen.add(event.eventId);
        return true;
      })
      .map((event) => ({ day, event })),
  );
  return (
    <details className="week-list fold">
      <summary>List these events ({rows.length})</summary>
      {rows.length === 0 ? (
        <p>No events in these days.</p>
      ) : (
        <div className="week-table-scroll">
          <table>
            <thead>
              <tr>
                <th scope="col">Date</th>
                <th scope="col">Event</th>
                <th scope="col">Time</th>
                <th scope="col">Source</th>
                <th scope="col">Shows as</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(({ day, event }) => (
                <tr key={`${day.civilDate}-${event.segmentId}`}>
                  <td>{day.label}</td>
                  <td>{event.title}</td>
                  <td>{event.allDay ? "All day" : `${event.startLabel} to ${event.endLabel}`}</td>
                  <td>{event.sourceLabel}</td>
                  <td>{event.busy ? "Busy" : "Free"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </details>
  );
}
