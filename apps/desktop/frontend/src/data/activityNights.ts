import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// Suggested nights beside recorded ones, from a Google export (ADR-0059) or
// this computer (ADR-0060). They are shadow evidence: nothing that plans or
// estimates reads them.

/** Part of a night cut at midnight: [day, start minute, end minute], day 0
 * being the first date drawn. */
export type NightSegment = readonly [day: number, start: number, end: number];

export interface NightsAgreement {
  /** Nights recorded in the time the source covers. */
  recorded: number;
  /** Of those, the nights a suggestion overlapped. */
  matched: number;
  /** How far apart their edges typically were, or empty when none matched. */
  summary: string;
}

export interface NightsFigure {
  nights: number;
  spanLabel: string;
  zoneId: string;
  firstDate: string;
  days: number;
  suggested: NightSegment[];
  recorded: NightSegment[];
  agreement?: NightsAgreement;
}

export interface ComputerNights {
  collected: boolean;
  figure?: NightsFigure;
}

export const noComputerNights: ComputerNights = { collected: false };

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

export function count(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : undefined;
}

function segments(value: unknown, days: number): NightSegment[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const result: NightSegment[] = [];
  for (const item of value) {
    if (!Array.isArray(item) || item.length !== 3) return undefined;
    const [day, start, end] = item.map(count);
    if (day === undefined || start === undefined || end === undefined) return undefined;
    if (day >= days || end <= start || end > 24 * 60) return undefined;
    result.push([day, start, end]);
  }
  return result;
}

function agreement(value: unknown): NightsAgreement | undefined {
  if (!isRecord(value)) return undefined;
  const recorded = count(value.recorded);
  const matched = count(value.matched);
  if (recorded === undefined || matched === undefined || matched > recorded) return undefined;
  return {
    recorded,
    matched,
    summary: typeof value.summary === "string" ? value.summary : "",
  };
}

/** A figure the window can draw truthfully, or none. */
export function normalizeNightsFigure(value: unknown): NightsFigure | undefined {
  if (!isRecord(value)) return undefined;
  const nights = count(value.nights);
  const days = count(value.days);
  if (nights === undefined || days === undefined || days === 0) return undefined;
  const suggested = segments(value.suggested, days);
  const recorded = segments(value.recorded, days);
  if (!suggested || !recorded || typeof value.firstDate !== "string") return undefined;
  const compared = agreement(value.agreement);
  return {
    nights,
    spanLabel: typeof value.spanLabel === "string" ? value.spanLabel : "",
    zoneId: typeof value.zoneId === "string" ? value.zoneId : "",
    firstDate: value.firstDate,
    days,
    suggested,
    recorded,
    ...(compared ? { agreement: compared } : {}),
  };
}

export function normalizeComputerNights(value: unknown): ComputerNights | undefined {
  if (!isRecord(value) || typeof value.collected !== "boolean") return undefined;
  if (!value.collected) return noComputerNights;
  const figure = normalizeNightsFigure(value.figure);
  return figure ? { collected: true, figure } : undefined;
}

/** What this computer suggests; nothing outside the desktop app. */
export async function loadComputerNights(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ComputerNights> {
  const method = findWailsMethod(root, ["GetComputerNights"]);
  if (!method) return noComputerNights;
  const nights = normalizeComputerNights(await method());
  if (!nights) throw new Error("This computer's activity could not be read.");
  return nights;
}
