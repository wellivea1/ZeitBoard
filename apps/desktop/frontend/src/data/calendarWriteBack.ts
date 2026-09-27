// Writing accepted times to the owner's CalDAV calendar (ADR-0053). Off until
// the owner turns it on for one imported CalDAV calendar with a sign-in that
// may add events. Then each accepted time is added there as ZeitBoard's own
// event and removed again on undo; what the owner changed there waits for them.
// Without the desktop service there is nothing to turn on.

import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export const calendarWriteBackChangedEvent = "zeitboard:calendar-write-back-changed";

export interface CalendarWriteProblem {
  eventId: string;
  title: string;
  /** When the accepted time starts, ISO 8601. */
  startAt?: string;
  detail: string;
  /** The owner decides; otherwise ZeitBoard tries again on its own. */
  conflict: boolean;
  /** The write was a removal: ZeitBoard's event is still in the calendar. */
  removing: boolean;
  /** When ZeitBoard tries again, ISO 8601. */
  retryAt?: string;
}

export interface CalendarWriteBack {
  on: boolean;
  sourceId?: string;
  label?: string;
  username?: string;
  summary: string;
  problems: CalendarWriteProblem[];
}

export type CalendarWriteDecision = "remove" | "keep";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function text(value: unknown) {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function instant(value: unknown) {
  const parsed = text(value);
  return parsed && Number.isFinite(Date.parse(parsed)) ? parsed : undefined;
}

function normalizeProblem(value: unknown): CalendarWriteProblem | undefined {
  if (!isRecord(value)) return undefined;
  const eventId = text(value.eventId);
  const detail = text(value.detail);
  if (
    !eventId ||
    !detail ||
    typeof value.conflict !== "boolean" ||
    typeof value.removing !== "boolean"
  ) {
    return undefined;
  }
  const startAt = instant(value.startAt);
  const retryAt = instant(value.retryAt);
  return {
    eventId,
    title: text(value.title) ?? "An accepted time",
    ...(startAt ? { startAt } : {}),
    detail,
    conflict: value.conflict,
    removing: value.removing,
    ...(retryAt ? { retryAt } : {}),
  };
}

export function normalizeCalendarWriteBack(value: unknown): CalendarWriteBack | undefined {
  if (!isRecord(value) || typeof value.on !== "boolean" || !text(value.summary)) return undefined;
  const problems = Array.isArray(value.problems) ? value.problems.map(normalizeProblem) : [];
  if (problems.some((problem) => problem === undefined)) return undefined;
  const on = value.on;
  const sourceId = text(value.sourceId);
  const label = text(value.label);
  if (on && (!sourceId || !label)) return undefined;
  return {
    on,
    ...(sourceId ? { sourceId } : {}),
    ...(label ? { label } : {}),
    ...(text(value.username) ? { username: text(value.username) } : {}),
    summary: text(value.summary) as string,
    problems: problems as CalendarWriteProblem[],
  };
}

async function writeBackCall(
  methodName: string,
  input: unknown,
  root: WailsRoot,
): Promise<CalendarWriteBack> {
  const method = findWailsMethod(root, [methodName]);
  if (!method) throw new Error("Writing to a calendar is available in the ZeitBoard desktop app.");
  const result = normalizeCalendarWriteBack(await method(input));
  if (!result) throw new Error("The calendar writer returned an invalid status.");
  return result;
}

/** The status, or undefined outside the desktop app. */
export async function loadCalendarWriteBack(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<CalendarWriteBack | undefined> {
  if (!findWailsMethod(root, ["GetCalendarWriteBack"])) return undefined;
  return writeBackCall("GetCalendarWriteBack", undefined, root);
}

export function enableCalendarWriteBack(
  input: { sourceId: string; username: string; password: string },
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return writeBackCall("EnableCalendarWriteBack", input, root);
}

export function disableCalendarWriteBack(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return writeBackCall("DisableCalendarWriteBack", undefined, root);
}

export function retryCalendarWrites(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return writeBackCall("RetryCalendarWrites", undefined, root);
}

export function resolveCalendarWriteConflict(
  eventId: string,
  decision: CalendarWriteDecision,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return writeBackCall("ResolveCalendarWriteConflict", { eventId, decision }, root);
}
