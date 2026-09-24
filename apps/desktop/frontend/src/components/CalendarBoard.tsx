import { useCallback, useMemo, useState, type CSSProperties } from "react";
import type { CalendarData, CalendarDay, CalendarEventSegment } from "../data/calendar";
import { TimeProbe } from "./TimeProbe";
import { assignEventLanes, rhythmSegments } from "./calendarLayout";
import { civilProbeLabel, useTimeProbe } from "./timeProbeLogic";

type PositionedStyle = CSSProperties & {
  "--calendar-left"?: string;
  "--calendar-width"?: string;
  "--calendar-lane"?: number;
  "--calendar-lanes"?: number;
};

function percent(minutes: number) {
  return `${(minutes / 1440) * 100}%`;
}

function positionStyle(startMinute: number, endMinute: number): PositionedStyle {
  return {
    "--calendar-left": percent(startMinute),
    "--calendar-width": percent(Math.max(endMinute - startMinute, 15)),
  };
}

/** Minutes since local midnight in `zoneId`, or undefined for an unknown zone. */
function minuteOfDay(zoneId: string, now = new Date()) {
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: zoneId,
      hour: "numeric",
      minute: "numeric",
      hourCycle: "h23",
    }).formatToParts(now);
    const value = (type: string) => Number(parts.find((part) => part.type === type)?.value);
    const minutes = value("hour") * 60 + value("minute");
    return Number.isFinite(minutes) ? minutes : undefined;
  } catch {
    return undefined;
  }
}

const hourMarks = ["12 AM", "6 AM", "12 PM", "6 PM", "12 AM"];

function CalendarDayTrack({
  day,
  zoneId,
  onSelect,
}: {
  day: CalendarDay;
  zoneId: string;
  onSelect: (event: CalendarEventSegment) => void;
}) {
  const layout = useMemo(() => assignEventLanes(day.events), [day.events]);
  const resolveProbe = useCallback(
    (fraction: number) => {
      const minutes = fraction * 1440;
      const predicted = day.predictions.some(
        (band) => minutes >= band.startMinute && minutes <= band.endMinute,
      );
      return {
        position: fraction,
        label: civilProbeLabel(day.civilDate, minutes, {
          predicted,
          approximate: predicted,
        }),
        zoneId,
      };
    },
    [day.civilDate, day.predictions, zoneId],
  );
  const probe = useTimeProbe(resolveProbe);
  const trackStyle = { "--calendar-lanes": layout.count } as PositionedStyle;
  const segments = useMemo(() => rhythmSegments(day.predictions), [day.predictions]);
  const now = day.isToday ? minuteOfDay(zoneId) : undefined;

  return (
    <section className="calendar-day-row" data-today={day.isToday || undefined}>
      <header className="calendar-day-label">
        <time dateTime={day.civilDate}>{day.label}</time>
        <small>
          {day.events.length === 0
            ? "No events"
            : `${day.events.length} ${day.events.length === 1 ? "event" : "events"}`}
        </small>
      </header>
      <div
        className="calendar-day-track has-time-probe"
        style={trackStyle}
        onPointerMove={probe.onPointerMove}
        onPointerLeave={probe.onPointerLeave}
      >
        {segments.map((segment) => (
          <span
            className="calendar-prediction-band"
            data-state={segment.state}
            aria-hidden="true"
            style={
              {
                "--calendar-left": percent(segment.startMinute),
                "--calendar-width": percent(segment.endMinute - segment.startMinute),
              } as PositionedStyle
            }
            key={`${segment.state}-${segment.startMinute}`}
          />
        ))}
        {day.predictions.length > 0 && (
          <ul className="sr-only">
            {day.predictions.map((band) => (
              <li key={band.segmentId}>
                {band.title}, {band.startLabel} to {band.endLabel}
              </li>
            ))}
          </ul>
        )}
        {now !== undefined && (
          <span className="calendar-now" aria-hidden="true" style={{ left: percent(now) }} />
        )}
        {day.events.map((event) => {
          const style = {
            ...positionStyle(event.startMinute, event.endMinute),
            "--calendar-lane": layout.lanes.get(event.segmentId) ?? 0,
          } as PositionedStyle;
          return (
            <button
              className="calendar-event-block"
              data-ownership={event.ownership}
              data-busy={event.busy || undefined}
              data-all-day={event.allDay || undefined}
              type="button"
              style={style}
              title={`${event.title}: ${event.startLabel} to ${event.endLabel}`}
              aria-label={`${event.title}, ${event.startLabel} to ${event.endLabel}, ${event.sourceLabel}`}
              onClick={() => onSelect(event)}
              key={event.segmentId}
            >
              <strong>{event.title}</strong>
            </button>
          );
        })}
        <TimeProbe probeRef={probe.probeRef} labelRef={probe.labelRef} />
      </div>
    </section>
  );
}

function EventInspector({ event, onClose }: { event: CalendarEventSegment; onClose: () => void }) {
  return (
    <section className="calendar-event-inspector" aria-live="polite" aria-label="Selected event">
      <div>
        <p className="section-kicker">
          {event.ownership === "app_owned" ? "Accepted time" : "From your calendar"}
        </p>
        <h2>{event.title}</h2>
      </div>
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
      <button className="button ghost compact" type="button" onClick={onClose}>
        Close details
      </button>
    </section>
  );
}

function CalendarEventTable({ data }: { data: CalendarData }) {
  const rows = data.days.flatMap((day) => day.events.map((event) => ({ day, event })));
  return (
    <details className="calendar-list-disclosure">
      <summary>List these events ({rows.length})</summary>
      {rows.length === 0 ? (
        <p>No events in these days.</p>
      ) : (
        <div className="calendar-table-scroll">
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

export function CalendarBoard({ data }: { data: CalendarData }) {
  const [selected, setSelected] = useState<CalendarEventSegment | null>(null);
  const visibleSelection = selected
    ? (data.days
        .flatMap((day) => day.events)
        .find((event) => event.segmentId === selected.segmentId) ?? null)
    : null;
  return (
    <section className="calendar-board" aria-label="Calendar events and rhythm forecast">
      <div className="calendar-hour-axis" aria-hidden="true">
        <span />
        <div>
          {hourMarks.map((label, index) => (
            <span style={{ left: `${index * 25}%` }} key={`${label}-${index}`}>
              {label}
            </span>
          ))}
        </div>
      </div>
      <div className="calendar-day-list">
        {data.days.map((day) => (
          <CalendarDayTrack
            day={day}
            zoneId={data.zoneId}
            onSelect={setSelected}
            key={day.civilDate}
          />
        ))}
      </div>
      {visibleSelection && (
        <EventInspector event={visibleSelection} onClose={() => setSelected(null)} />
      )}
      <footer className="calendar-board-foot">
        <div className="calendar-legend" aria-label="Calendar legend">
          <span data-state="awake">Likely awake</span>
          <span data-state="uncertain">Uncertain</span>
          <span data-state="asleep">Likely asleep</span>
          <span data-kind="imported">Calendar event</span>
          <span data-kind="app_owned">Accepted time</span>
        </div>
        {data.status !== "estimated" && (
          <p>
            No sleep prediction to draw yet. <a href="#/log/sleep">Log sleep</a> to see it here.
          </p>
        )}
      </footer>
      <CalendarEventTable data={data} />
    </section>
  );
}
