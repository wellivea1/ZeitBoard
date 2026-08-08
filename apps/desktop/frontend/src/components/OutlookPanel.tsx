import { Icon } from "./Icon";
import type { OutlookData, OutlookOfficeWindow, OutlookSegment, Presence } from "../data/outlook";

// The 48-72 hour operational view (ADR-0034).
//
// The strip has three states, not two. The estimator's sleep and waking
// envelopes overlap on purpose, and an instant inside both is one where the
// model does not know which side of the boundary it is on. Painting a sharp
// line there would be a confident claim against a measured P90 onset error of
// over five hours, so the uncertain band is drawn as a band.

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

function OutlookTimeline({ data }: { data: OutlookData }) {
  return (
    <figure className="outlook-timeline">
      <div className="outlook-track" aria-hidden="true">
        {data.segments.map((segment) => (
          <span
            key={`${segment.presence}-${segment.offsetHours}`}
            className="outlook-band"
            data-presence={segment.presence}
            data-observed={segment.observed || undefined}
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
          >
            <small>{day.label}</small>
          </span>
        ))}
      </div>

      {/* The strip carries the shape; this carries the same facts in words, so
          a screen reader loses nothing and the drawing gives up nothing. */}
      <ol className="sr-only">
        {data.segments.map((segment) => (
          <li key={`text-${segment.presence}-${segment.offsetHours}`}>
            {PRESENCE_LABELS[segment.presence]}
            {segment.observed ? " (recorded)" : ""}, {segment.dayLabel} {segment.rangeLabel},
            lasting {segment.durationLabel}.
          </li>
        ))}
      </ol>

      <figcaption className="outlook-legend">
        <span data-presence="awake">Awake</span>
        <span data-presence="uncertain">Uncertain</span>
        <span data-presence="asleep">Asleep</span>
        <small>
          {data.awakeLabel} awake, {data.uncertainLabel} the model will not call.
        </small>
      </figcaption>
    </figure>
  );
}

function officeIcon(status: OutlookOfficeWindow["status"]) {
  if (status === "reachable") return "focus" as const;
  if (status === "partial") return "clock" as const;
  return "moon" as const;
}

function OfficeList({ data }: { data: OutlookData }) {
  if (data.officeWindows.length === 0) return null;
  return (
    <section className="outlook-office" aria-labelledby="outlook-office-title">
      <div className="outlook-section-head">
        <span className="overview-row-label">Reaching people</span>
        <h4 id="outlook-office-title">Office hours</h4>
        <small>{data.officeHoursLabel}</small>
      </div>
      <ul>
        {data.officeWindows.map((window) => (
          <li key={`${window.dayLabel}-${window.offsetHours}`} data-status={window.status}>
            <Icon name={officeIcon(window.status)} />
            <span>
              <strong>{window.dayLabel}</strong>
              <em>{window.hoursLabel}</em>
              {window.reachableLabel && <b>Awake {window.reachableLabel}</b>}
              <small>{window.detail}</small>
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function CommitmentList({ data }: { data: OutlookData }) {
  if (data.commitments.length === 0) return null;
  return (
    <section className="outlook-commitments" aria-labelledby="outlook-commitments-title">
      <div className="outlook-section-head">
        <span className="overview-row-label">Already booked</span>
        <h4 id="outlook-commitments-title">Fixed events</h4>
      </div>
      <ul>
        {data.commitments.map((commitment) => (
          <li
            key={`${commitment.title}-${commitment.whenLabel}`}
            data-conflict={commitment.conflict}
          >
            <span>
              <strong>{commitment.title}</strong>
              <em>{commitment.whenLabel}</em>
              {commitment.conflictLabel && <small>{commitment.conflictLabel}</small>}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function OpportunityList({ data }: { data: OutlookData }) {
  if (data.opportunities.length === 0) return null;
  return (
    <section className="outlook-opportunities" aria-labelledby="outlook-opportunities-title">
      <div className="outlook-section-head">
        <span className="overview-row-label">Suggestions only</span>
        <h4 id="outlook-opportunities-title">Where your tasks could go</h4>
        <small>Nothing here is scheduled; every placement needs your approval.</small>
      </div>
      <ul>
        {data.opportunities.map((opportunity) => (
          <li key={opportunity.taskId} data-placed={opportunity.whenLabel ? "yes" : "no"}>
            <span>
              <strong>{opportunity.title}</strong>
              {opportunity.whenLabel ? (
                <em>{opportunity.whenLabel}</em>
              ) : (
                <small>{opportunity.unplacedLabel}</small>
              )}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function OutlookNotice({ data }: { data: OutlookData }) {
  const withheld = data.status === "withheld" || data.status === "unavailable";
  return (
    <div className="outlook-notice" data-status={data.status}>
      <Icon name={withheld ? "clock" : "focus"} />
      <span>
        <strong>
          {withheld
            ? "The next three days are not being shown"
            : "Not enough history to look ahead yet"}
        </strong>
        <small>{data.withheldMessage ?? data.refusal?.message ?? data.freshness.explanation}</small>
        {withheld && (
          <small>
            A forecast is anchored to where you are in your cycle right now. Without a recent record
            there is nothing to anchor it to, so office windows drawn over it would be arithmetic
            rather than a plan.
          </small>
        )}
      </span>
      <a className="button secondary" href="#/log/sleep">
        Add sleep entry
      </a>
    </div>
  );
}

export function OutlookPanel({ data }: { data: OutlookData }) {
  return (
    <section className="outlook" aria-labelledby="outlook-title">
      <header className="outlook-head">
        <div>
          <span className="overview-row-label">{data.horizonLabel}</span>
          <h3 id="outlook-title">What the next three days look like</h3>
        </div>
        {data.status === "available" && data.nextSleepLabel && (
          <p className="outlook-next">
            <Icon name="moon" />
            <span>
              <strong>Next sleep</strong>
              <small>{data.nextSleepLabel}</small>
            </span>
          </p>
        )}
      </header>

      {data.status === "available" ? (
        <>
          <OutlookTimeline data={data} />
          <OfficeList data={data} />
          <CommitmentList data={data} />
          <OpportunityList data={data} />
        </>
      ) : (
        <OutlookNotice data={data} />
      )}
    </section>
  );
}

export type { OutlookSegment };
