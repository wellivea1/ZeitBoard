import type { CalendarData, CalendarEventSegment } from "../data/calendar";
import type { MedicationsData } from "../data/medications";
import type { ProposalRecord } from "../data/proposals";
import type { SleepEntry } from "../data/sleepEntries";
import { clockRange, clockTime } from "../utils/relativeTime";
import { rhythmSegments, type RhythmState } from "./calendarLayout";

// Plan › Week lays days out as columns on one vertical clock, the way a
// calendar app does, and paints the rhythm into them: what was recorded
// behind the past, what is likely ahead of now. Plans sit on top. This module
// turns the calendar, the pending suggestions, recorded sleep and scheduled
// doses into positioned pieces; WeekBoard only draws them.

const DAY_MINUTES = 1440;

/** Blocks shorter than this are drawn this tall, so a title and a decision fit. */
export const MIN_BLOCK_MINUTES = 40;

export interface CivilPoint {
  date: string;
  minute: number;
}

/** The civil date and minute of an instant in a zone. */
export function civilPoint(instant: string | Date, zoneId: string): CivilPoint | undefined {
  const date = typeof instant === "string" ? new Date(instant) : instant;
  if (Number.isNaN(date.getTime())) return undefined;
  try {
    const parts = new Intl.DateTimeFormat("en-CA", {
      timeZone: zoneId,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }).formatToParts(date);
    const part = (type: string) => parts.find((candidate) => candidate.type === type)?.value ?? "";
    const minute = Number(part("hour")) * 60 + Number(part("minute"));
    if (!Number.isFinite(minute)) return undefined;
    return { date: `${part("year")}-${part("month")}-${part("day")}`, minute };
  } catch {
    return undefined;
  }
}

/** "2026-09-24T22:30" as a civil point, for records kept in local time. */
export function localPoint(value: string): CivilPoint | undefined {
  const match = /^(\d{4}-\d{2}-\d{2})T(\d{2}):(\d{2})/.exec(value);
  if (!match) return undefined;
  return { date: match[1] as string, minute: Number(match[2]) * 60 + Number(match[3]) };
}

interface DaySpan {
  startMinute: number;
  endMinute: number;
  continuesBefore: boolean;
  continuesAfter: boolean;
}

/** The part of a span that falls on `date`, if any. */
function spanOn(date: string, start: CivilPoint, end: CivilPoint): DaySpan | undefined {
  if (date < start.date || date > end.date) return undefined;
  const startMinute = date === start.date ? start.minute : 0;
  const endMinute = date === end.date ? end.minute : DAY_MINUTES;
  if (endMinute <= startMinute) return undefined;
  return {
    startMinute,
    endMinute,
    continuesBefore: date !== start.date,
    continuesAfter: date !== end.date,
  };
}

export type WeekBlockKind = "event" | "accepted" | "suggested";

export interface WeekBlock {
  key: string;
  kind: WeekBlockKind;
  title: string;
  timeLabel: string;
  startMinute: number;
  endMinute: number;
  continuesBefore: boolean;
  continuesAfter: boolean;
  /**
   * The first part of the block on the board. A block across midnight is
   * drawn in each day it touches, but only this part carries its controls,
   * so it is decided, and read aloud, once.
   */
  primary: boolean;
  /** Position among the blocks it overlaps, and how many share the width. */
  lane: number;
  lanes: number;
  event?: CalendarEventSegment;
  proposalId?: string;
}

export type WeekBandKind = "recorded" | Exclude<RhythmState, "awake">;

export interface WeekBand {
  key: string;
  kind: WeekBandKind;
  startMinute: number;
  endMinute: number;
  label: string;
}

export interface WeekDose {
  key: string;
  minute: number;
  label: string;
}

export interface WeekColumn {
  civilDate: string;
  weekday: string;
  dayNumber: string;
  /** The day in words for assistive technology: "Friday 25 September, today". */
  spokenDate: string;
  isToday: boolean;
  isPast: boolean;
  bands: WeekBand[];
  allDay: CalendarEventSegment[];
  blocks: WeekBlock[];
  doses: WeekDose[];
  /** Minute from which nothing is forecast, when the forecast stops in this day. */
  forecastEndsAt?: number;
  nowMinute?: number;
}

/**
 * Overlapping blocks share the column's width, each cluster on its own: an
 * evening with two clashing events should not squeeze the lone morning one.
 */
export function assignLanes<T extends { startMinute: number; endMinute: number }>(
  items: T[],
): (T & { lane: number; lanes: number })[] {
  const ordered = [...items].sort(
    (a, b) => a.startMinute - b.startMinute || b.endMinute - a.endMinute,
  );
  const placed: (T & { lane: number; lanes: number })[] = [];
  let cluster: (T & { lane: number; lanes: number })[] = [];
  let clusterEnd = -1;
  let laneEnds: number[] = [];
  const closeCluster = () => {
    for (const item of cluster) item.lanes = laneEnds.length;
    cluster = [];
    laneEnds = [];
  };
  for (const item of ordered) {
    const end = Math.max(item.endMinute, item.startMinute + MIN_BLOCK_MINUTES);
    if (item.startMinute >= clusterEnd) closeCluster();
    let lane = laneEnds.findIndex((laneEnd) => laneEnd <= item.startMinute);
    if (lane < 0) {
      lane = laneEnds.length;
      laneEnds.push(end);
    } else laneEnds[lane] = end;
    const positioned = { ...item, lane, lanes: 1 };
    cluster.push(positioned);
    placed.push(positioned);
    clusterEnd = Math.max(clusterEnd, end);
  }
  closeCluster();
  return placed;
}

type Segments = ReturnType<typeof rhythmSegments>;

/**
 * Labels the forecast bands by what they are next to, looking across
 * midnight: an uncertain evening before a night asleep is where sleep may
 * begin, even though the night is drawn in the next column.
 */
function forecastBands(
  date: string,
  segments: Segments,
  from: number,
  before?: Segments,
  after?: Segments,
) {
  const bands: WeekBand[] = [];
  const carriedIn = before?.at(-1)?.endMinute === DAY_MINUTES ? before.at(-1) : undefined;
  const carriedOut = after?.[0]?.startMinute === 0 ? after[0] : undefined;
  segments.forEach((segment, index) => {
    if (segment.state === "awake") return;
    const startMinute = Math.max(segment.startMinute, from);
    if (segment.endMinute <= startMinute) return;
    let label = "Likely asleep";
    if (segment.state === "uncertain") {
      const next =
        segments[index + 1] ?? (segment.endMinute === DAY_MINUTES ? carriedOut : undefined);
      const previous = segments[index - 1] ?? (segment.startMinute === 0 ? carriedIn : undefined);
      const touches = (other: Segments[number] | undefined, edge: "start" | "end") =>
        other?.state === "asleep" &&
        (edge === "end"
          ? other.startMinute === segment.endMinute % DAY_MINUTES
          : other.endMinute % DAY_MINUTES === segment.startMinute);
      label = touches(next, "end")
        ? "Sleep may begin"
        : touches(previous, "start")
          ? "Waking likely"
          : "Uncertain";
    }
    bands.push({
      key: `${date}-${segment.state}-${segment.startMinute}`,
      kind: segment.state,
      startMinute,
      endMinute: segment.endMinute,
      label,
    });
  });
  return bands;
}

export interface WeekInput {
  calendar: CalendarData;
  suggestions: ProposalRecord[];
  sleep: SleepEntry[];
  medications: MedicationsData | null;
  now: Date;
}

export function weekColumns({ calendar, suggestions, sleep, medications, now }: WeekInput) {
  const zoneId = calendar.zoneId;
  const today = civilPoint(now, zoneId);
  const predictionEnds = calendar.days
    .flatMap((day) => day.predictions.map((band) => Date.parse(band.endAt)))
    .filter(Number.isFinite);
  const forecastEnd =
    calendar.status === "estimated" && predictionEnds.length > 0
      ? civilPoint(new Date(Math.max(...predictionEnds)), zoneId)
      : undefined;

  const segments = calendar.days.map((day) => rhythmSegments(day.predictions));

  return calendar.days.map((day, index): WeekColumn => {
    const date = day.civilDate;
    const noon = new Date(`${date}T12:00:00`);
    const isToday = today?.date === date;
    const isPast = today !== undefined && date < today.date;
    const nowMinute = isToday ? today.minute : undefined;

    // Forecast from now on; what happened before now is the record's to say.
    const from = isPast ? DAY_MINUTES : (nowMinute ?? 0);
    const bands = forecastBands(
      date,
      segments[index] ?? [],
      from,
      segments[index - 1],
      segments[index + 1],
    );
    for (const entry of sleep) {
      if (entry.suppressed) continue;
      const start = localPoint(entry.effectiveStartLocal || entry.startLocal);
      const end = localPoint(entry.effectiveEndLocal || entry.endLocal);
      const span = start && end ? spanOn(date, start, end) : undefined;
      if (!span) continue;
      bands.push({
        key: `recorded-${entry.observationId}-${date}`,
        kind: "recorded",
        startMinute: span.startMinute,
        endMinute: span.endMinute,
        label: span.continuesBefore ? "Recorded" : `Recorded · ${entry.durationLabel}`,
      });
    }

    const pieces: Omit<WeekBlock, "lane" | "lanes">[] = [];
    // The first column may open partway through a block that began earlier.
    const primary = (continuesBefore: boolean) => !continuesBefore || index === 0;
    for (const event of day.events) {
      if (event.allDay) continue;
      pieces.push({
        key: event.segmentId,
        kind: event.ownership === "app_owned" ? "accepted" : "event",
        title: event.title,
        timeLabel: event.pointInTime
          ? clockTime(new Date(event.startAt))
          : clockRange(new Date(event.startAt), new Date(event.endAt)),
        startMinute: event.startMinute,
        endMinute: event.pointInTime ? event.startMinute + 15 : event.endMinute,
        continuesBefore: event.continuesBefore,
        continuesAfter: event.continuesAfter,
        primary: primary(event.continuesBefore),
        event,
      });
    }
    for (const proposal of suggestions) {
      if (!proposal.startAt || !proposal.endAt) continue;
      const start = civilPoint(proposal.startAt, zoneId);
      const end = civilPoint(proposal.endAt, zoneId);
      const span = start && end ? spanOn(date, start, end) : undefined;
      if (!span) continue;
      pieces.push({
        key: `suggested-${proposal.id}-${date}`,
        kind: "suggested",
        title: proposal.title,
        timeLabel: clockRange(new Date(proposal.startAt), new Date(proposal.endAt)),
        ...span,
        primary: primary(span.continuesBefore),
        proposalId: proposal.id,
      });
    }

    const doses: WeekDose[] = [];
    for (const medication of medications?.medications ?? []) {
      if (!medication.active || !medication.schedule) continue;
      for (const occurrence of medication.schedule.forecast.occurrences) {
        const point = civilPoint(occurrence.at, zoneId);
        if (point?.date !== date) continue;
        doses.push({
          key: `${medication.medicationId}-${occurrence.at}`,
          minute: point.minute,
          label: medication.label,
        });
      }
    }

    // Past the last forecast band the page is shaded and says so, rather than
    // reading as a stretch of certain waking.
    let forecastEndsAt: number | undefined;
    if (!isPast && calendar.status === "estimated") {
      const floor = nowMinute ?? 0;
      if (!forecastEnd || date > forecastEnd.date) forecastEndsAt = floor;
      else if (date === forecastEnd.date && forecastEnd.minute < DAY_MINUTES)
        forecastEndsAt = Math.max(forecastEnd.minute, floor);
    }

    return {
      civilDate: date,
      weekday: noon.toLocaleDateString(undefined, { weekday: "short" }),
      dayNumber: String(noon.getDate()),
      spokenDate: `${noon.toLocaleDateString(undefined, { weekday: "long", day: "numeric", month: "long" })}${isToday ? ", today" : ""}`,
      isToday,
      isPast,
      bands,
      allDay: day.events.filter((event) => event.allDay),
      blocks: assignLanes(pieces),
      doses: doses.sort((a, b) => a.minute - b.minute),
      ...(forecastEndsAt !== undefined ? { forecastEndsAt } : {}),
      ...(nowMinute !== undefined ? { nowMinute } : {}),
    };
  });
}

/** "24 – 27 September", "30 September – 3 October", "Thursday 24 September". */
export function weekTitle(first: string, last: string) {
  const a = new Date(`${first}T12:00:00`);
  const b = new Date(`${last}T12:00:00`);
  const month = (date: Date) => date.toLocaleDateString(undefined, { month: "long" });
  if (first === last)
    return `${a.toLocaleDateString(undefined, { weekday: "long" })} ${a.getDate()} ${month(a)}`;
  if (a.getMonth() === b.getMonth()) return `${a.getDate()} – ${b.getDate()} ${month(b)}`;
  return `${a.getDate()} ${month(a)} – ${b.getDate()} ${month(b)}`;
}

/** "3 AM", "Noon", "9 PM": the gutter's labels. */
/** A minute of the day as a clock time, with the day's edges named. */
export function minuteClock(minute: number) {
  if (minute <= 0 || minute >= DAY_MINUTES) return "midnight";
  if (minute === 12 * 60) return "noon";
  const hour = Math.floor(minute / 60);
  return `${((hour + 11) % 12) + 1}:${String(minute % 60).padStart(2, "0")} ${hour < 12 ? "AM" : "PM"}`;
}

export function hourLabel(hour: number) {
  if (hour === 12) return "Noon";
  if (hour === 0 || hour === 24) return "Midnight";
  return `${((hour + 11) % 12) + 1} ${hour < 12 ? "AM" : "PM"}`;
}
