import { useCallback, useMemo, useState, type CSSProperties } from "react";
import type { CalendarData, CalendarDay, CalendarEventSegment } from "../data/calendar";
import { TimeProbe } from "./TimeProbe";
import { assignEventLanes } from "./calendarLayout";
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

  return (
    <section className="calendar-day-row" data-today={day.isToday || undefined}>
      <header className="calendar-day-label">
        <time dateTime={day.civilDate}>{day.label}</time>
        <small>{day.events.length} fixed</small>
      </header>
      <div
        className="calendar-day-track has-time-probe"
        style={trackStyle}
        onPointerMove={probe.onPointerMove}
        onPointerLeave={probe.onPointerLeave}
      >
        {day.predictions.map((band) => (
          <span
            className="calendar-prediction-band"
            data-kind={band.kind}
            data-confidence={band.confidence}
            style={positionStyle(band.startMinute, band.endMinute)}
            title={`${band.title}: ${band.startLabel} to ${band.endLabel}`}
            key={band.segmentId}
          >
            <span className="sr-only">
              {band.title}, {band.startLabel} to {band.endLabel}
            </span>
          </span>
        ))}
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
        {day.events.length === 0 && <span className="calendar-empty-track">No fixed events</span>}
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
          {event.ownership === "app_owned" ? "ZeitBoard placement" : "Read-only import"}
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
      <summary>Event list ({rows.length} visible segments)</summary>
      {rows.length === 0 ? (
        <p>No fixed events occur in this range.</p>
      ) : (
        <div className="calendar-table-scroll">
          <table>
            <thead>
              <tr>
                <th scope="col">Date</th>
                <th scope="col">Event</th>
                <th scope="col">Time</th>
                <th scope="col">Source</th>
                <th scope="col">Effect</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(({ day, event }) => (
                <tr key={`${day.civilDate}-${event.segmentId}`}>
                  <td>{day.label}</td>
                  <td>{event.title}</td>
                  <td>{event.allDay ? "All day" : `${event.startLabel} to ${event.endLabel}`}</td>
                  <td>{event.sourceLabel}</td>
                  <td>{event.busy ? "Blocks placement" : "Available"}</td>
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
      <header className="calendar-board-header">
        <div>
          <p className="section-kicker">Civil-time board</p>
          <h2>
            {data.startCivilDate} to {data.endCivilDate}
          </h2>
        </div>
        <div className="calendar-legend" aria-label="Calendar legend">
          <span data-kind="predicted_sleep">Predicted sleep</span>
          <span data-kind="predicted_wake">Predicted waking</span>
          <span data-kind="imported">Imported fixed</span>
          <span data-kind="app_owned">ZeitBoard placement</span>
        </div>
      </header>
      <div className="calendar-hour-axis" aria-hidden="true">
        <span>12 AM</span>
        <span>6 AM</span>
        <span>12 PM</span>
        <span>6 PM</span>
        <span>12 AM</span>
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
      <CalendarEventTable data={data} />
    </section>
  );
}
