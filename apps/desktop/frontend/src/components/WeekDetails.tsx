import type { CalendarData, CalendarEventSegment } from "../data/calendar";
import type { ProposalRecord } from "../data/proposals";
import type { ProposalDecision } from "../state/approvals";
import { blockWording } from "../utils/relativeTime";

// What sits under the Week board: the event you clicked, and the same events
// as a table for anyone who would rather read than look.

export function EventInspector({
  event,
  onClose,
}: {
  event: CalendarEventSegment;
  onClose: () => void;
}) {
  return (
    <section className="week-inspector" aria-live="polite" aria-label="Selected event">
      <p className="section-kicker">
        {event.ownership === "app_owned" ? "A time you accepted" : "From your calendar"}
      </p>
      <h2>{event.title}</h2>
      <dl>
        <div>
          <dt>Time</dt>
          <dd>
            {event.startLabel} to {event.endLabel}
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
      <button className="button ghost" type="button" onClick={onClose}>
        Close details
      </button>
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
  return (
    <section className="week-inspector" aria-live="polite" aria-label="Selected suggestion">
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
