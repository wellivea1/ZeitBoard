import type { OwnerEventInput } from "../data/ownerEvents";
import { fieldValue, knownZone, type SleepTimeField } from "./sleepForm";

// What the event form sends: a title and two exact times, or the one thing
// still wrong with it, said as the owner would need to hear it.

const DAY = 24 * 60 * 60_000;

/** A predicted sleep window, by its two instants. */
export interface SleepWindow {
  startAt: string;
  endAt: string;
}

export type ReadyOwnerEvent =
  | { input: OwnerEventInput; startInstant: string; endInstant: string }
  | { problem: string };

type EventTimes =
  | { startLocal: string; endLocal: string; startInstant: string; endInstant: string }
  | { problem: string };

/** The two times alone, exact, whatever the title says yet. */
export function ownerEventTimes(
  form: { start: SleepTimeField; end: SleepTimeField },
  zone: string,
): EventTimes {
  if (!knownZone(zone)) return { problem: "ZeitBoard does not know that time zone." };
  const start = fieldValue(form.start, zone, "when it starts");
  if ("problem" in start) return start;
  const end = fieldValue(form.end, zone, "when it ends");
  if ("problem" in end) return end;
  const length = Date.parse(end.instant) - Date.parse(start.instant);
  if (length <= 0) return { problem: "An event must end after it starts." };
  if (length > DAY) return { problem: "An event can last at most a day." };
  return {
    startLocal: start.value,
    endLocal: end.value,
    startInstant: start.instant,
    endInstant: end.instant,
  };
}

export function ownerEventInput(
  form: { title: string; start: SleepTimeField; end: SleepTimeField },
  zone: string,
): ReadyOwnerEvent {
  const title = form.title.trim();
  if (!title) return { problem: "Give the event a title." };
  if (/[\r\n]/.test(title)) return { problem: "Keep the title on one line." };
  const times = ownerEventTimes(form, zone);
  if ("problem" in times) return times;
  return {
    input: { title, startLocal: times.startLocal, endLocal: times.endLocal, zoneId: zone },
    startInstant: times.startInstant,
    endInstant: times.endInstant,
  };
}

/**
 * Windows that touch are one window: the calendar hands a night that crosses
 * midnight over as a piece for each day.
 */
export function joinedWindows(windows: SleepWindow[]): SleepWindow[] {
  const sorted = windows
    .filter((window) => Date.parse(window.endAt) > Date.parse(window.startAt))
    .sort((a, b) => Date.parse(a.startAt) - Date.parse(b.startAt));
  const joined: SleepWindow[] = [];
  for (const window of sorted) {
    const last = joined[joined.length - 1];
    if (last && Date.parse(window.startAt) <= Date.parse(last.endAt)) {
      if (Date.parse(window.endAt) > Date.parse(last.endAt)) last.endAt = window.endAt;
    } else {
      joined.push({ ...window });
    }
  }
  return joined;
}

/** The first predicted sleep window the stretch from start to end falls in. */
export function sleepWindowOver(
  windows: SleepWindow[],
  startInstant: string,
  endInstant: string,
): SleepWindow | undefined {
  const start = Date.parse(startInstant);
  const end = Date.parse(endInstant);
  return joinedWindows(windows).find(
    (window) => Date.parse(window.startAt) < end && Date.parse(window.endAt) > start,
  );
}
