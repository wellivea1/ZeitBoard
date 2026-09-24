// Short, relative time wording for glanceable surfaces.
//
// The backend's labels are complete and unambiguous — "Thu Sep 24, 11:30 PM EDT
// to Fri Sep 25, 10:08 AM EDT" — which is right for a record and too long to
// read at a glance. Where a screen knows the instant, it says "Tonight 11:30 PM
// – 1:35 AM" instead: the day relative to today, the clock, and nothing
// repeated.

const HOUR = 3_600_000;

function startOfDay(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}

/** Whole civil days from `now` to `date`, in local time. */
export function civilDayOffset(date: Date, now: Date) {
  return Math.round((startOfDay(date) - startOfDay(now)) / (24 * HOUR));
}

/**
 * "Tonight" for this evening, "Today" for earlier today, "Tomorrow", then a
 * weekday for the rest of the week and a date beyond it.
 */
export function relativeDay(date: Date, now: Date) {
  const offset = civilDayOffset(date, now);
  if (offset === 0) return date.getHours() >= 17 ? "Tonight" : "Today";
  if (offset === 1) return "Tomorrow";
  if (offset === -1) return "Yesterday";
  if (offset > 1 && offset < 7) return date.toLocaleDateString(undefined, { weekday: "long" });
  return date.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" });
}

function clockParts(date: Date) {
  const parts = new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
  }).formatToParts(date);
  const part = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((candidate) => candidate.type === type)?.value ?? "";
  // A 24-hour locale has no day period, and then ranges repeat nothing anyway.
  return { clock: `${part("hour")}:${part("minute")}`, dayPeriod: part("dayPeriod") };
}

export function clockTime(date: Date) {
  return date.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
}

/** "7:15 – 10:10 AM", or "11:30 PM – 1:35 AM" when the halves of the day differ. */
export function clockRange(start: Date, end: Date) {
  const a = clockParts(start);
  const b = clockParts(end);
  if (a.dayPeriod && a.dayPeriod === b.dayPeriod) return `${a.clock} – ${b.clock} ${b.dayPeriod}`;
  return `${clockTime(start)} – ${clockTime(end)}`;
}

/** "Tonight 11:30 PM – 1:35 AM", named by the day the range begins. */
export function relativeRange(start: Date, end: Date, now: Date) {
  return `${relativeDay(start, now)} ${clockRange(start, end)}`;
}

/** Rounds to the nearest `minutes`, for ranges whose uncertainty is in hours. */
export function roundToMinutes(date: Date, minutes: number) {
  const step = minutes * 60_000;
  return new Date(Math.round(date.getTime() / step) * step);
}

/** The instant `offsetHours` after `start`, or undefined without a start. */
export function atOffset(start: string | undefined, offsetHours: number) {
  if (!start) return undefined;
  const base = Date.parse(start);
  if (!Number.isFinite(base)) return undefined;
  return new Date(base + offsetHours * HOUR);
}
