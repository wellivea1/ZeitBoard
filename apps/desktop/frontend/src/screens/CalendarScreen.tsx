import { useCallback } from "react";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";
import { TimeProbe } from "../components/TimeProbe";
import { civilProbeLabel, useTimeProbe } from "../components/timeProbeLogic";

interface CalendarDayPreview {
  civilDate: string;
  dayLabel: string;
  zoneId: string;
  sleepStartHour: number;
  usefulStartHour: number;
  fixedEventHour?: number;
}

const calendarDays: CalendarDayPreview[] = [
  {
    civilDate: "2026-06-15",
    dayLabel: "Mon 15",
    zoneId: "America/New_York",
    sleepStartHour: 0.96,
    usefulStartHour: 8.16,
    fixedEventHour: 18,
  },
  {
    civilDate: "2026-06-16",
    dayLabel: "Tue 16",
    zoneId: "America/New_York",
    sleepStartHour: 2.4,
    usefulStartHour: 9.6,
  },
  {
    civilDate: "2026-06-17",
    dayLabel: "Wed 17",
    zoneId: "America/New_York",
    sleepStartHour: 3.84,
    usefulStartHour: 11.04,
  },
  {
    civilDate: "2026-06-18",
    dayLabel: "Thu 18",
    zoneId: "America/New_York",
    sleepStartHour: 5.28,
    usefulStartHour: 12.48,
  },
  {
    civilDate: "2026-06-19",
    dayLabel: "Fri 19",
    zoneId: "America/New_York",
    sleepStartHour: 6.72,
    usefulStartHour: 13.92,
  },
];

const SLEEP_DURATION_HOURS = 6;
const USEFUL_DURATION_HOURS = 9.36;

function hourPercent(hour: number) {
  return `${(hour / 24) * 100}%`;
}

function withinSpan(hour: number, start: number, duration: number) {
  return hour >= start && hour <= start + duration;
}

function CalendarDayTrack({ day }: { day: CalendarDayPreview }) {
  const resolveProbe = useCallback(
    (fraction: number) => {
      const hour = fraction * 24;
      const predicted =
        withinSpan(hour, day.sleepStartHour, SLEEP_DURATION_HOURS) ||
        withinSpan(hour, day.usefulStartHour, USEFUL_DURATION_HOURS);
      return {
        position: fraction,
        label: civilProbeLabel(day.civilDate, fraction * 24 * 60, {
          predicted,
          approximate: predicted,
        }),
        zoneId: day.zoneId,
      };
    },
    [day],
  );
  const probe = useTimeProbe(resolveProbe);

  return (
    <article>
      <h2>
        <time dateTime={day.civilDate}>{day.dayLabel}</time>
      </h2>
      <div
        className="day-track has-time-probe"
        onPointerMove={probe.onPointerMove}
        onPointerLeave={probe.onPointerLeave}
      >
        <span
          className="sleep-band"
          style={{
            left: hourPercent(day.sleepStartHour),
            width: hourPercent(SLEEP_DURATION_HOURS),
          }}
        >
          Predicted sleep
        </span>
        <span
          className="wake-band"
          style={{
            left: hourPercent(day.usefulStartHour),
            width: hourPercent(USEFUL_DURATION_HOURS),
          }}
        >
          Useful window
        </span>
        {day.fixedEventHour !== undefined && (
          <span className="fixed-event" style={{ left: hourPercent(day.fixedEventHour) }}>
            Check-in
          </span>
        )}
        <TimeProbe probeRef={probe.probeRef} labelRef={probe.labelRef} />
      </div>
    </article>
  );
}

export function CalendarScreen() {
  return (
    <>
      <PageHeader
        title="Calendar"
        description="Compare fixed events with uncertain predicted sleep and waking windows."
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode="fixture" aria-hidden="true" />
            <span>Sample preview</span>
          </div>
        }
      />
      <PlaceholderNotice>
        This five-day board is a synthetic design preview. Calendar import arrives with the
        interoperability phase; fixed events will remain immutable inputs.
      </PlaceholderNotice>
      <section className="panel calendar-board" aria-label="Five day planning preview">
        <div className="calendar-hours" aria-hidden="true">
          <span>12 AM</span>
          <span>6 AM</span>
          <span>12 PM</span>
          <span>6 PM</span>
          <span>12 AM</span>
        </div>
        <div className="calendar-days">
          {calendarDays.map((day) => (
            <CalendarDayTrack day={day} key={day.civilDate} />
          ))}
        </div>
      </section>
    </>
  );
}
