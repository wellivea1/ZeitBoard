import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// Reaching hours are when the people this person needs to reach are available —
// the clinic, the pharmacy, the family abroad — not the person's own schedule.
// Until they existed the outlook assumed Monday to Friday, nine to five, which
// is wrong for a great many people with a drifting rhythm. The setting is a
// short list of people, each with their own zone and their own hours on each
// weekday (ADR-0056), and Home draws a row for each one shown.

export interface ReachingDayHours {
  /** Weekday number, Sunday 0 through Saturday 6. Hours past midnight belong to the day they open. */
  day: number;
  startLocal: string;
  endLocal: string;
}

export interface ReachingContact {
  id: string;
  label: string;
  zoneId: string;
  /** Kept on the list, with their hours, without a row on Home. */
  off: boolean;
  hours: ReachingDayHours[];
}

export interface ReachingHours {
  contacts: ReachingContact[];
}

export interface ReachingHoursEnvelope {
  state: ReachingHours;
  revision: number;

  /** True when a stale edit lost to what was already stored. */
  conflict: boolean;
  message?: string;
}

/** Home draws a row for each person shown, and has room for five. */
export const MAX_REACHING_CONTACTS = 5;

export const WEEKDAY_LABELS = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
] as const;

/** The week as the form lays it out and Home's summary reads it: Monday first. */
export const WEEK_ORDER: readonly number[] = [1, 2, 3, 4, 5, 6, 0];

const CLOCK = /^(?:[01]\d|2[0-3]):[0-5]\d$/;
const DAY_MINUTES = 24 * 60;

// Where a new person, or a newly opened day with no neighbour, starts. It is
// a starting point to change, not a description of anyone.
const STARTING_HOURS = { startLocal: "09:00", endLocal: "17:00" };

export const reachingHoursUnavailable: ReachingHoursEnvelope = {
  state: { contacts: [] },
  revision: 0,
  conflict: false,
  message: "Open the ZeitBoard desktop app to set when people are reachable.",
};

const reachingHoursUnreadable: ReachingHoursEnvelope = {
  ...reachingHoursUnavailable,
  message:
    "Reaching hours could not be read, so none are shown here. What is saved has not changed.",
};

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function normalizeDay(value: unknown): ReachingDayHours | undefined {
  if (!isRecord(value)) return undefined;
  const { day, startLocal, endLocal } = value;
  if (typeof day !== "number" || !Number.isInteger(day) || day < 0 || day > 6) return undefined;
  if (typeof startLocal !== "string" || !CLOCK.test(startLocal)) return undefined;
  if (typeof endLocal !== "string" || !CLOCK.test(endLocal)) return undefined;
  return { day, startLocal, endLocal };
}

function normalizeContact(value: unknown): ReachingContact | undefined {
  if (!isRecord(value) || !Array.isArray(value.hours)) return undefined;
  const id = str(value.id);
  if (!id || typeof value.label !== "string") return undefined;
  const hours: ReachingDayHours[] = [];
  for (const item of value.hours) {
    const day = normalizeDay(item);
    if (!day || hours.some((known) => known.day === day.day)) return undefined;
    hours.push(day);
  }
  return {
    id,
    label: value.label,
    zoneId: str(value.zoneId) ?? "",
    off: value.off === true,
    hours: hours.sort((a, b) => a.day - b.day),
  };
}

/** A list in which anything fails to parse is no list: half of it would be saved back as the whole. */
export function normalizeReachingHours(value: unknown): ReachingHoursEnvelope | undefined {
  if (!isRecord(value) || !isRecord(value.state) || !Array.isArray(value.state.contacts)) {
    return undefined;
  }
  const contacts: ReachingContact[] = [];
  for (const item of value.state.contacts) {
    const contact = normalizeContact(item);
    if (!contact) return undefined;
    contacts.push(contact);
  }
  const message = str(value.message);
  return {
    state: { contacts },
    revision: typeof value.revision === "number" ? value.revision : 0,
    conflict: value.conflict === true,
    ...(message ? { message } : {}),
  };
}

export async function loadReachingHours(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ReachingHoursEnvelope> {
  const method = findWailsMethod(root, ["GetReachingHours"]);
  if (!method) return reachingHoursUnavailable;
  try {
    const normalized = normalizeReachingHours(await method());
    if (normalized) return normalized;
  } catch {
    // fall through
  }
  // Its revision is 0, so saving from here loses to what is stored rather
  // than replacing it.
  return reachingHoursUnreadable;
}

export async function saveReachingHours(
  state: ReachingHours,
  baseRevision: number,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ReachingHoursEnvelope> {
  const method = findWailsMethod(root, ["SaveReachingHours"]);
  if (!method) throw new Error("Reaching hours need the ZeitBoard desktop app.");
  const normalized = normalizeReachingHours(await method({ state, baseRevision }));
  if (!normalized) throw new Error("The list could not be saved.");
  return normalized;
}

/** A new person on the list, open on the working week in the given zone until changed. */
export function newReachingContact(zoneId: string): ReachingContact {
  return {
    id: newContactId(),
    label: "",
    zoneId,
    off: false,
    hours: [1, 2, 3, 4, 5].map((day) => ({ day, ...STARTING_HOURS })),
  };
}

function newContactId(): string {
  const bytes = new Uint8Array(5);
  globalThis.crypto.getRandomValues(bytes);
  return `contact-${Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("")}`;
}

/**
 * Opens or closes one day. A day that opens takes the hours of the nearest
 * open day before it in the week, else after it, so a run of days with the
 * same hours is one click each.
 */
export function withDayOpen(contact: ReachingContact, day: number, open: boolean): ReachingContact {
  const others = contact.hours.filter((hours) => hours.day !== day);
  if (!open) return { ...contact, hours: others };
  const place = WEEK_ORDER.indexOf(day);
  const before = WEEK_ORDER.slice(0, place).reverse();
  const after = WEEK_ORDER.slice(place + 1);
  const neighbour = [...before, ...after]
    .map((candidate) => others.find((hours) => hours.day === candidate))
    .find((hours) => hours !== undefined);
  const opening = neighbour
    ? { startLocal: neighbour.startLocal, endLocal: neighbour.endLocal }
    : STARTING_HOURS;
  return {
    ...contact,
    hours: [...others, { day, ...opening }].sort((a, b) => a.day - b.day),
  };
}

export function withDayHours(
  contact: ReachingContact,
  day: number,
  changes: Partial<Pick<ReachingDayHours, "startLocal" | "endLocal">>,
): ReachingContact {
  return {
    ...contact,
    hours: contact.hours.map((hours) => (hours.day === day ? { ...hours, ...changes } : hours)),
  };
}

/** How a day's hours read beside the clock fields: closing the next day, or open all day. */
export function dayHoursNote(hours: ReachingDayHours): string | undefined {
  if (!CLOCK.test(hours.startLocal) || !CLOCK.test(hours.endLocal)) return undefined;
  if (hours.startLocal === hours.endLocal) return "all day";
  return hours.endLocal < hours.startLocal ? "next day" : undefined;
}

/** A stretch of one day's 24 hours, as fractions of the day. */
export interface DaySpan {
  start: number;
  end: number;

  /** True for the part of the previous day's hours that runs past midnight. */
  carried: boolean;
}

function minutes(clock: string): number | undefined {
  if (!CLOCK.test(clock)) return undefined;
  return Number(clock.slice(0, 2)) * 60 + Number(clock.slice(3, 5));
}

/** Where someone is available on one day, including what the day before carries past midnight. */
export function daySpans(hours: readonly ReachingDayHours[], day: number): DaySpan[] {
  const spans: DaySpan[] = [];
  const previous = hours.find((entry) => entry.day === (day + 6) % 7);
  const previousStart = previous ? minutes(previous.startLocal) : undefined;
  const previousEnd = previous ? minutes(previous.endLocal) : undefined;
  if (
    previousStart !== undefined &&
    previousEnd !== undefined &&
    previousEnd < previousStart &&
    previousEnd > 0
  ) {
    spans.push({ start: 0, end: previousEnd / DAY_MINUTES, carried: true });
  }
  const today = hours.find((entry) => entry.day === day);
  const start = today ? minutes(today.startLocal) : undefined;
  const end = today ? minutes(today.endLocal) : undefined;
  if (start !== undefined && end !== undefined) {
    spans.push({
      start: start === end ? 0 : start / DAY_MINUTES,
      end: end > start ? end / DAY_MINUTES : 1,
      carried: false,
    });
  }
  return spans;
}

/** timeZones lists what this machine offers, with the current zone first. */
export function timeZones(): string[] {
  const current = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  const supported =
    typeof Intl.supportedValuesOf === "function" ? Intl.supportedValuesOf("timeZone") : [];
  const rest = supported.filter((zone) => zone !== current);
  return [current, ...rest];
}
