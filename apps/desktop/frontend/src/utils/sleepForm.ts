import type { SleepClassification, SleepEntry, SleepEntryInput } from "../data/sleepEntries";
import {
  civilCandidates,
  offsetTimestamp,
  selectedCivilInstant,
  zoneWords,
  zonedWall,
} from "./civilTime";

// The sleep form, for a night added and a night corrected alike: date-and-time
// pickers in the night's own zone. A clock time that happens twice, when the
// clocks go back, asks which is meant; one that never happens says so. The
// times sent are exact, with the offset of the occurrence chosen.

/**
 * One time on the form: the wall clock shown, the instant chosen when that
 * clock time repeats, and the stored value itself while it is untouched, so a
 * correction keeps what the owner did not change to the second.
 */
export interface SleepTimeField {
  wall: string;
  at?: string;
  exact?: string;
}

export interface SleepForm {
  start: SleepTimeField;
  end: SleepTimeField;
  zoneId: string;
  classification: SleepClassification;
}

/** Whether the zone is one the browser can write times in. */
export function knownZone(zone: string): boolean {
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: zone });
    return zone.trim() !== "";
  } catch {
    return false;
  }
}

/** A night as it stands, ready to correct. */
export function correctionForm(entry: SleepEntry): SleepForm {
  const field = (value: string): SleepTimeField => {
    const { wall, at } = zonedWall(value, entry.zoneId);
    return at ? { wall, at, exact: value } : { wall };
  };
  return {
    start: field(entry.effectiveStartLocal),
    end: field(entry.effectiveEndLocal),
    zoneId: entry.zoneId,
    classification: entry.effectiveClassification,
  };
}

/** "1:30 AM" for 2026-11-01T01:30. */
export function wallClock(wall: string): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
  }).format(Date.parse(`${wall}:00Z`));
}

/** Why a clock time cannot be used: the clocks went forward past it. */
export function skippedTime(wall: string, zone: string): string {
  return `${wallClock(wall)} did not happen that day in ${zoneWords(zone)}: the clocks went forward.`;
}

/** The occurrences of a changed clock time, or none to choose between. */
export function occurrences(field: SleepTimeField, zone: string) {
  if (!field.wall || field.exact || !knownZone(zone)) return undefined;
  return civilCandidates(field.wall, zone);
}

type FieldValue = { value: string; instant: string } | { problem: string };

function fieldValue(field: SleepTimeField, zone: string, what: string): FieldValue {
  if (!field.wall) return { problem: `Enter ${what}.` };
  if (field.exact && field.at) return { value: field.exact, instant: field.at };
  if (civilCandidates(field.wall, zone).length === 0) {
    return { problem: skippedTime(field.wall, zone) };
  }
  const instant = selectedCivilInstant(field.wall, field.at, zone);
  if (!instant) return { problem: `Choose which ${wallClock(field.wall)} you mean.` };
  return { value: offsetTimestamp(field.wall, instant), instant };
}

/** The desktop's input, each time exact, or what is still missing. */
export function sleepFormInput(form: SleepForm): { input: SleepEntryInput } | { problem: string } {
  if (!knownZone(form.zoneId)) return { problem: "ZeitBoard does not know that time zone." };
  const start = fieldValue(form.start, form.zoneId, "when you fell asleep");
  if ("problem" in start) return start;
  const end = fieldValue(form.end, form.zoneId, "when you woke");
  if ("problem" in end) return end;
  if (Date.parse(end.instant) <= Date.parse(start.instant)) {
    return { problem: "Wake time must be after sleep start." };
  }
  return {
    input: {
      startLocal: start.value,
      endLocal: end.value,
      zoneId: form.zoneId,
      classification: form.classification,
    },
  };
}
