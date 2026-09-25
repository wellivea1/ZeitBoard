import { isCommitmentConflict, sleepAhead, type OutlookData, type OutlookSegment } from "./outlook";
import type { CalendarDay } from "./calendar";
import type { MedicationsData } from "./medications";
import type { OverviewData } from "./overview";
import {
  atOffset,
  betweenClocks,
  civilDayOffset,
  clockTime,
  dayInSentence,
  roundToMinutes,
} from "../utils/relativeTime";

// The words at the top of Home. The Almanac design leads with a sentence
// rather than a status tile: "You have been awake for about 8 hours. Sleep is
// likely to begin between 11:30 PM and 1:35 AM tonight, and you will probably
// wake between 7:15 and 10:10 AM tomorrow." Each time is a range because the
// estimate is one; nothing here claims a moment.

export type StateTone = "awake" | "asleep" | "uncertain";

export function stateTone(state: string): StateTone {
  const normalized = state.toLowerCase();
  if (
    normalized.includes("uncertain") ||
    normalized.includes("transition") ||
    normalized.includes("no sleep") ||
    normalized.includes("need more") ||
    normalized.includes("unavailable")
  )
    return "uncertain";
  if (normalized.includes("asleep") || normalized.includes("sleep")) return "asleep";
  return "awake";
}

export interface LeadPart {
  text: string;
  strong?: boolean;
}

/** "about 8 hours" once it is long enough that minutes are noise. */
export function awakeFor(label: string) {
  const hours = Number(/(\d+)\s*hours?/.exec(label)?.[1] ?? NaN);
  const minutes = Number(/(\d+)\s*minutes?/.exec(label)?.[1] ?? 0);
  if (!Number.isFinite(hours) || hours < 2) return label;
  const rounded = Math.round(hours + minutes / 60);
  return `about ${rounded} hours`;
}

function sentence(text: string) {
  const trimmed = text.trim();
  return /[.!?]$/.test(trimmed) ? trimmed : `${trimmed}.`;
}

function edges(outlook: OutlookData, segment: OutlookSegment) {
  const start = atOffset(outlook.horizonStart, segment.offsetHours);
  const end = atOffset(outlook.horizonStart, segment.offsetHours + segment.durationHours);
  if (!start || !end) return undefined;
  return { start: roundToMinutes(start, 5), end: roundToMinutes(end, 5) };
}

export function leadParts(overview: OverviewData, outlook: OutlookData, now = new Date()) {
  const parts: LeadPart[] = [];
  const say = (text: string) => parts.push({ text });
  const mark = (text: string) => parts.push({ text, strong: true });

  if (overview.status === "unavailable") {
    say("Your records could not be read just now. Nothing saved has changed.");
    return parts;
  }
  if (overview.status !== "estimated") {
    say("There is not enough recorded sleep to look ahead yet.");
    return parts;
  }

  const tone = stateTone(overview.state);
  if (!overview.freshness.trusted) say(`${sentence(overview.freshness.explanation)} `);
  else if (tone === "awake" && overview.timeSinceWake)
    say(`You have been awake for ${awakeFor(overview.timeSinceWake)}. `);
  else if (tone === "asleep") say("The forecast places you in a sleep now. ");
  else say(`${sentence(overview.state)} `);

  const ahead = outlook.status === "available" ? sleepAhead(outlook.segments) : {};
  const onset = ahead.onset ? edges(outlook, ahead.onset) : undefined;
  const wake = ahead.wake ? edges(outlook, ahead.wake) : undefined;

  if (onset) {
    const [a, b] = betweenClocks(onset.start, onset.end);
    if (onset.start.getTime() <= now.getTime() + 5 * 60_000) {
      say("Sleep is likely any time before ");
      mark(b);
    } else {
      say("Sleep is likely to begin between ");
      mark(a);
      say(" and ");
      mark(b);
    }
    say(` ${dayInSentence(onset.start, now)}`);
    if (wake) {
      const [c, d] = betweenClocks(wake.start, wake.end);
      say(", and you will probably wake between ");
      mark(c);
      say(" and ");
      mark(d);
      say(` ${dayInSentence(wake.start, now)}.`);
    } else say(".");
  } else if (wake) {
    const [c, d] = betweenClocks(wake.start, wake.end);
    say("This sleep is likely to end between ");
    mark(c);
    say(" and ");
    mark(d);
    say(` ${dayInSentence(wake.start, now)}.`);
  } else if (
    overview.freshness.trusted &&
    outlook.status === "available" &&
    overview.nextSleepWindow.label
  ) {
    // No timeline to measure from: the estimate's own label, once. A stale or
    // withheld outlook gets nothing here; its figure says why.
    say("Sleep is likely ");
    mark(overview.nextSleepWindow.label);
    say(".");
  }
  return parts;
}

/** An imported event, a time accepted in Plan, or a scheduled dose. */
export type DiaryKind = "event" | "task" | "dose";

export interface DiaryEntry {
  key: string;
  at: Date;
  time: string;
  title: string;
  kind: DiaryKind;
  note?: string;
}

export interface DiaryDay {
  key: string;
  label: string;
  entries: DiaryEntry[];
}

function dayLabel(date: Date, now: Date) {
  const offset = civilDayOffset(date, now);
  if (offset === 0) return "Today";
  if (offset === 1) return "Tomorrow";
  // "Sunday 27": the locale's own order put the number first.
  return `${date.toLocaleDateString(undefined, { weekday: "long" })} ${date.getDate()}`;
}

/**
 * What is fixed in the days ahead, by day: calendar events, the times you
 * accepted in Plan (the calendar holds those too) and scheduled doses. Events
 * come from the calendar rather than the forecast so they stay listed while
 * the forecast is withheld; the forecast only adds a note where an event sits
 * on a boundary it cannot place. Plan › Week draws the same things on a grid.
 */
export function diaryDays(
  calendar: CalendarDay[],
  outlook: OutlookData,
  medications: MedicationsData | null,
  now = new Date(),
  horizonHours = 72,
): DiaryDay[] {
  const until = now.getTime() + horizonHours * 3_600_000;
  const entries: DiaryEntry[] = [];

  const minuteKey = (title: string, at: Date) => `${title}|${Math.round(at.getTime() / 60_000)}`;
  const notes = new Map<string, string>();
  if (outlook.status === "available") {
    for (const commitment of outlook.commitments) {
      if (commitment.offsetHours === undefined || !isCommitmentConflict(commitment)) continue;
      const at = atOffset(outlook.horizonStart, commitment.offsetHours);
      if (at && commitment.conflictLabel)
        notes.set(minuteKey(commitment.title, at), commitment.conflictLabel);
    }
  }

  const seen = new Set<string>();
  for (const day of calendar) {
    for (const event of day.events) {
      if (seen.has(event.eventId)) continue;
      const at = new Date(event.startAt);
      const end = new Date(event.endAt);
      // Still under way counts; finished does not.
      const endsAt = event.pointInTime ? at.getTime() : end.getTime();
      if (Number.isNaN(at.getTime()) || endsAt < now.getTime() || at.getTime() > until) continue;
      seen.add(event.eventId);
      const note = notes.get(minuteKey(event.title, at));
      entries.push({
        key: `event-${event.eventId}`,
        at,
        time: event.allDay ? "All day" : clockTime(at),
        title: event.title,
        kind: event.ownership === "app_owned" ? "task" : "event",
        ...(note ? { note } : {}),
      });
    }
  }
  for (const medication of medications?.medications ?? []) {
    if (!medication.active || !medication.schedule) continue;
    for (const occurrence of medication.schedule.forecast.occurrences) {
      const at = new Date(occurrence.at);
      if (at.getTime() < now.getTime() || at.getTime() > until) continue;
      entries.push({
        key: `dose-${medication.medicationId}-${occurrence.at}`,
        at,
        time: clockTime(at),
        title: medication.label,
        kind: "dose",
      });
    }
  }

  entries.sort((a, b) => a.at.getTime() - b.at.getTime());
  const days: DiaryDay[] = [];
  for (const entry of entries) {
    // An event under way sits under today, whenever it began.
    const day = entry.at.getTime() < now.getTime() ? now : entry.at;
    const key = day.toDateString();
    let group = days.find((candidate) => candidate.key === key);
    if (!group) {
      group = { key, label: dayLabel(day, now), entries: [] };
      days.push(group);
    }
    group.entries.push(entry);
  }
  return days;
}
