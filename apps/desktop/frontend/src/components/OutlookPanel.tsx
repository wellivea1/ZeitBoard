import { isCommitmentConflict } from "../data/outlook";
import type { OutlookData, OutlookSegment, Presence } from "../data/outlook";

// The 48-72 hour operational view (ADR-0034), drawn as one timeline.
//
// The rhythm row has three states, not two. The estimator's sleep and waking
// envelopes overlap on purpose, and an instant inside both is one where the
// model does not know which side of the boundary it is on. Painting a sharp
// line there would be a confident claim against a measured P90 onset error of
// over five hours, so the uncertain band is drawn as a band.
//
// Reachable hours and fixed events used to be separate lists under the strip,
// each restating days and times the strip already showed. They are rows on the
// same axis now, so "can I ring the clinic while I'm awake" and "does Monday's
// appointment land in sleep" are read off the picture rather than cross-checked
// between three lists.

const PRESENCE_LABELS: Record<Presence, string> = {
  awake: "Likely awake",
  asleep: "Likely asleep",
  uncertain: "Boundary uncertain",
  unknown: "Beyond the forecast",
};

function percent(hours: number, horizonHours: number) {
  if (horizonHours <= 0) return "0%";
  return `${Math.max(0, Math.min(100, (hours / horizonHours) * 100))}%`;
}

const DAY_WORDS = ["", "one", "two", "three", "four", "five", "six", "seven"];

function horizonTitle(hours: number) {
  const days = Math.round(hours / 24);
  return days >= 2
    ? `The next ${DAY_WORDS[days] ?? days} days`
    : `The next ${Math.round(hours)} hours`;
}

function segmentTitle(segment: OutlookSegment) {
  return `${PRESENCE_LABELS[segment.presence]}${segment.observed ? " (recorded)" : ""}: ${
    segment.dayLabel
  } ${segment.rangeLabel}`;
}

function OutlookTimeline({ data }: { data: OutlookData }) {
  const placedEvents = data.commitments.filter(
    (commitment) => commitment.offsetHours !== undefined && commitment.durationHours !== undefined,
  );
  return (
    <figure className="outlook-timeline">
      <div className="outlook-grid">
        <span className="outlook-row-label">Rhythm</span>
        <div className="outlook-track" aria-hidden="true">
          {data.segments.map((segment) => (
            <span
              key={`${segment.presence}-${segment.offsetHours}`}
              className="outlook-band"
              data-presence={segment.presence}
              data-observed={segment.observed || undefined}
              title={segmentTitle(segment)}
              style={{
                left: percent(segment.offsetHours, data.horizonHours),
                width: percent(segment.durationHours, data.horizonHours),
              }}
            />
          ))}
          {data.days.map((day) => (
            <span
              key={day.label}
              className="outlook-day-mark"
              style={{ left: percent(day.offsetHours, data.horizonHours) }}
            />
          ))}
          <span className="outlook-now" title="Now" />
        </div>

        {data.officeWindows.length > 0 && (
          <>
            <span className="outlook-row-label" title={data.officeHoursLabel}>
              Reachable
            </span>
            <div className="outlook-track outlook-track-thin" aria-hidden="true">
              {data.officeWindows.map((window) => (
                <span
                  key={`${window.dayLabel}-${window.offsetHours}`}
                  className="outlook-reach"
                  data-status={window.status}
                  title={`${window.dayLabel}, ${window.hoursLabel}. ${window.detail}`}
                  style={{
                    left: percent(window.offsetHours, data.horizonHours),
                    width: percent(window.durationHours, data.horizonHours),
                  }}
                />
              ))}
            </div>
          </>
        )}

        {placedEvents.length > 0 && (
          <>
            <span className="outlook-row-label">Events</span>
            <div className="outlook-track outlook-track-events" aria-hidden="true">
              {placedEvents.map((commitment) => (
                <span
                  key={`${commitment.title}-${commitment.offsetHours}`}
                  className="outlook-event"
                  data-conflict={isCommitmentConflict(commitment) || undefined}
                  title={`${commitment.title}, ${commitment.whenLabel}${
                    commitment.conflictLabel ? `. ${commitment.conflictLabel}` : ""
                  }`}
                  style={{
                    left: percent(commitment.offsetHours ?? 0, data.horizonHours),
                    width: percent(commitment.durationHours ?? 0, data.horizonHours),
                  }}
                />
              ))}
            </div>
          </>
        )}

        {/* The day labels live outside the tracks. Inside they were clipped by
            the overflow that keeps the bands' rounded corners. */}
        <span />
        <div className="outlook-axis" aria-hidden="true">
          <small className="outlook-axis-now">Now</small>
          {data.days.map((day) => (
            <small key={day.label} style={{ left: percent(day.offsetHours, data.horizonHours) }}>
              {day.label}
            </small>
          ))}
        </div>
      </div>

      {/* The picture carries the shape; this carries the same facts in words,
          so a screen reader loses nothing and the drawing gives up nothing. */}
      <div className="sr-only">
        <ol aria-label="Predicted sleep and waking">
          {data.segments.map((segment) => (
            <li key={`text-${segment.presence}-${segment.offsetHours}`}>
              {PRESENCE_LABELS[segment.presence]}
              {segment.observed ? " (recorded)" : ""}, {segment.dayLabel} {segment.rangeLabel},
              lasting {segment.durationLabel}.
            </li>
          ))}
        </ol>
        {data.officeWindows.length > 0 && (
          <ol aria-label={data.officeHoursLabel}>
            {data.officeWindows.map((window) => (
              <li key={`text-${window.dayLabel}-${window.offsetHours}`}>
                {window.dayLabel}, {window.hoursLabel}: {window.detail}
              </li>
            ))}
          </ol>
        )}
      </div>

      <figcaption className="outlook-legend">
        <span data-presence="awake">Awake</span>
        <span data-presence="uncertain">Uncertain</span>
        <span data-presence="asleep">Asleep</span>
        {data.officeWindows.length > 0 && <span data-legend="reach">Reachable</span>}
        {placedEvents.length > 0 && <span data-legend="event">Event</span>}
        <small>
          {data.awakeLabel} likely awake · {data.uncertainLabel} uncertain
        </small>
      </figcaption>
    </figure>
  );
}

function OutlookNotice({ data }: { data: OutlookData }) {
  // Home's lead sentence already says why the state is unknown, so a withheld
  // figure says what brings it back rather than repeating the reason.
  const withheld = data.status === "withheld";
  const unavailable = data.status === "unavailable";
  const title = withheld
    ? "The next three days are not drawn"
    : unavailable
      ? "The next three days could not be read"
      : "Not enough history to look ahead yet";
  const detail = withheld
    ? "A forecast starts from where you are in the cycle. Record your last sleep or waking and it returns."
    : unavailable
      ? (data.withheldMessage ?? "Nothing saved has changed.")
      : (data.refusal?.message ?? data.freshness.explanation);
  return (
    <div className="outlook-notice" data-status={data.status}>
      <span>
        <strong>{title}</strong>
        <small>{detail}</small>
      </span>
      <a className="button secondary" href={unavailable ? "#/data-sources" : "#/log/sleep"}>
        {unavailable ? "Check Data Sources" : "Log sleep"}
      </a>
    </div>
  );
}

export function OutlookPanel({ data }: { data: OutlookData }) {
  return (
    <section className="outlook" aria-labelledby="outlook-title">
      <header className="outlook-head">
        <h2 id="outlook-title">
          <span className="figure-number">Fig. 1</span> {horizonTitle(data.horizonHours)}
        </h2>
        <a href="#/rhythm">Full rhythm</a>
      </header>
      {data.status === "available" ? (
        <OutlookTimeline data={data} />
      ) : (
        <OutlookNotice data={data} />
      )}
    </section>
  );
}

export type { OutlookSegment };
