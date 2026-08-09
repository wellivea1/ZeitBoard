import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// Reaching hours are when the people this person needs to reach are available —
// the clinic, the pharmacy, the family abroad — not the person's own schedule.
// Until this existed the outlook assumed Monday to Friday, nine to five, which
// is wrong for a great many people with a drifting rhythm, and every "reachable
// for three hours" figure drawn from it was wrong with it.

export interface ReachingHours {
  enabled: boolean;
  label: string;
  startLocal: string;
  endLocal: string;

  /** Weekday numbers, Sunday 0 through Saturday 6. */
  days: number[];
  zoneId: string;
}

export interface ReachingHoursEnvelope {
  state: ReachingHours;
  revision: number;

  /** True when a stale edit lost to what was already stored. */
  conflict: boolean;
  summary: string;
  message?: string;
}

export const WEEKDAY_LABELS = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
] as const;

export const reachingHoursUnavailable: ReachingHoursEnvelope = {
  state: {
    enabled: true,
    label: "Typical office hours",
    startLocal: "09:00",
    endLocal: "17:00",
    days: [1, 2, 3, 4, 5],
    zoneId: "",
  },
  revision: 0,
  conflict: false,
  summary: "Open the ZeitBoard desktop app to set when people are reachable.",
};

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function weekdays(value: unknown): number[] {
  if (!Array.isArray(value)) return [];
  const days = value.filter(
    (day): day is number =>
      typeof day === "number" && Number.isInteger(day) && day >= 0 && day <= 6,
  );
  return [...new Set(days)].sort((a, b) => a - b);
}

export function normalizeReachingHours(value: unknown): ReachingHoursEnvelope | undefined {
  if (!isRecord(value)) return undefined;
  const state = value.state;
  const summary = str(value.summary);
  if (!isRecord(state) || !summary) return undefined;
  const startLocal = str(state.startLocal);
  const endLocal = str(state.endLocal);
  if (!startLocal || !endLocal) return undefined;
  const message = str(value.message);
  return {
    state: {
      // A missing flag reads as off. Showing reaching windows nobody asked for
      // is the failure this replaced.
      enabled: state.enabled === true,
      label: str(state.label) ?? "",
      startLocal,
      endLocal,
      days: weekdays(state.days),
      zoneId: str(state.zoneId) ?? "",
    },
    revision: typeof value.revision === "number" ? value.revision : 0,
    conflict: value.conflict === true,
    summary,
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
  return reachingHoursUnavailable;
}

export async function saveReachingHours(
  state: ReachingHours,
  baseRevision: number,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ReachingHoursEnvelope> {
  const method = findWailsMethod(root, ["SaveReachingHours"]);
  if (!method) throw new Error("Reaching hours need the ZeitBoard desktop app.");
  const normalized = normalizeReachingHours(await method({ state, baseRevision }));
  if (!normalized) throw new Error("The schedule could not be saved.");
  return normalized;
}

/** timeZones lists what this machine offers, with the current zone first. */
export function timeZones(): string[] {
  const current = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  const supported =
    typeof Intl.supportedValuesOf === "function" ? Intl.supportedValuesOf("timeZone") : [];
  const rest = supported.filter((zone) => zone !== current);
  return [current, ...rest];
}
