import { clockRange } from "./relativeTime";

// How a night is said on a glanceable surface: its day and clock range in the
// night's own zone, and where it came from. The stored labels ("Wed Sep 23,
// 11:01 PM EDT to Thu Sep 24, 8:08 AM EDT") stay for hover and records.

/** The civil clock of a stored time, ignoring any UTC offset it carries. */
export function civilClock(value: string) {
  const date = new Date(value.slice(0, 16));
  return Number.isNaN(date.getTime()) ? undefined : date;
}

/** A night's times, as stored and as labelled. */
export interface NightTimes {
  startLocal: string;
  endLocal: string;
  startLabel: string;
  endLabel: string;
}

/** "Wed, Sep 23" and "11:01 PM – 8:08 AM", or the labels if a time is unreadable. */
export function nightWording(night: NightTimes) {
  const start = civilClock(night.startLocal);
  const end = civilClock(night.endLocal);
  if (!start || !end) return { day: night.startLabel, time: `to ${night.endLabel}` };
  return {
    day: start.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" }),
    time: clockRange(start, end),
  };
}

const sourceWords: readonly [prefix: string, words: string][] = [
  ["manual", "Logged by you"],
  ["file import", "Imported"],
  ["health connect", "Health Connect"],
  ["os activity", "Device activity"],
  ["synthetic", "Synthetic"],
];

/** Where a night came from, from the desktop's "method / evidence" label. */
export function sourceWording(provenanceLabel: string) {
  return sourceWords.find(([prefix]) => provenanceLabel.startsWith(prefix))?.[1] ?? provenanceLabel;
}

/** "25 minutes", "1 hour", "2 hours 10 minutes". */
export function minutesWording(minutes: number) {
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  const parts = [
    hours > 0 ? `${hours} ${hours === 1 ? "hour" : "hours"}` : "",
    rest > 0 || hours === 0 ? `${rest} ${rest === 1 ? "minute" : "minutes"}` : "",
  ];
  return parts.filter(Boolean).join(" ");
}
