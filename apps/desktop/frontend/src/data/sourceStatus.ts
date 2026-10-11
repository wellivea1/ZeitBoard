import { findWailsMethod, type WailsRoot } from "./wailsBridge";
import { momentInSentence } from "../utils/relativeTime";

// What a phone found when it last checked a source of sleep records
// (ADR-0054), and how the window says it. The phone reports after each
// Health Connect check when anything changed, and every six hours otherwise,
// so a report older than twelve hours means it has not been reading.

export type SourceOutcome = "read" | "permission_missing" | "unavailable" | "read_failed";

export interface SourceStatus {
  source: "health_connect";
  outcome: SourceOutcome;
  reportedAt: string;
  since: string;
  readThrough?: string;
  backgroundReads: boolean;
  held: number;
  /** When the newest sleep record from the source ends. */
  newestRecordEnd?: string;
}

const quietAfterMs = 12 * 3_600_000;

const outcomes: readonly SourceOutcome[] = [
  "read",
  "permission_missing",
  "unavailable",
  "read_failed",
];

function isInstant(value: unknown): value is string {
  return typeof value === "string" && !Number.isNaN(Date.parse(value));
}

export function normalizeSourceStatuses(value: unknown): SourceStatus[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const statuses: SourceStatus[] = [];
  for (const item of value) {
    if (typeof item !== "object" || item === null) return undefined;
    const record = item as Record<string, unknown>;
    if (
      record.source !== "health_connect" ||
      !outcomes.includes(record.outcome as SourceOutcome) ||
      !isInstant(record.reportedAt) ||
      !isInstant(record.since) ||
      (record.readThrough !== undefined && !isInstant(record.readThrough)) ||
      (record.newestRecordEnd !== undefined && !isInstant(record.newestRecordEnd)) ||
      typeof record.backgroundReads !== "boolean" ||
      typeof record.held !== "number" ||
      !Number.isInteger(record.held) ||
      record.held < 0
    ) {
      return undefined;
    }
    statuses.push({
      source: "health_connect",
      outcome: record.outcome as SourceOutcome,
      reportedAt: record.reportedAt,
      since: record.since,
      ...(record.readThrough ? { readThrough: record.readThrough } : {}),
      backgroundReads: record.backgroundReads,
      held: record.held,
      ...(record.newestRecordEnd ? { newestRecordEnd: record.newestRecordEnd } : {}),
    });
  }
  return statuses;
}

export async function loadSourceStatuses(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SourceStatus[]> {
  const method = findWailsMethod(root, ["GetSourceStatuses"]);
  // Outside the desktop app no phone has reported anything.
  if (!method) return [];
  const statuses = normalizeSourceStatuses(await method());
  if (!statuses) throw new Error("The phone's source reports could not be read.");
  return statuses;
}

/** "yesterday at 9:05 AM", "Tue, Sep 22 at 7:40 AM": a moment after "since" or "after". */
function sinceMoment(value: string, now: Date) {
  return momentInSentence(new Date(value), now).replace(/^on /, "");
}

function quiet(status: SourceStatus, now: Date) {
  return now.getTime() - Date.parse(status.reportedAt) >= quietAfterMs;
}

function heldWords(held: number) {
  return held === 1
    ? "1 night is held on the phone: its time zone differs from home."
    : `${held} nights are held on the phone: their time zone differs from home.`;
}

export interface SourceStatusWording {
  name: string;
  label: string;
  state: "available" | "error" | "off";
  detail: string;
}

/** Data Sources' row for a reporting source. */
export function sourceStatusWording(status: SourceStatus, now = new Date()): SourceStatusWording {
  const at = (value: string) => momentInSentence(new Date(value), now);
  const name = "Health Connect on your phone";
  const opened = "It reads only while ZeitBoard is open on the phone.";
  switch (status.outcome) {
    case "permission_missing":
      return {
        name,
        label: "Permission off",
        state: "error",
        detail: `The phone has not been allowed to read sleep since ${sinceMoment(status.since, now)}. Allow it in ZeitBoard on the phone.`,
      };
    case "unavailable":
      return {
        name,
        label: "Unavailable",
        state: "error",
        detail: `Health Connect has not been available on the phone since ${sinceMoment(status.since, now)}. It may need installing or updating.`,
      };
    case "read_failed":
      return {
        name,
        label: "Read failed",
        state: "error",
        detail: `The last read, ${at(status.reportedAt)}, failed. ${
          status.backgroundReads ? "The phone tries again within the hour." : opened
        }`,
      };
    case "read": {
      if (quiet(status, now)) {
        return {
          name,
          label: "Not reading",
          state: "off",
          detail: `Last read ${at(status.reportedAt)}. ${
            status.backgroundReads ? "The phone may be off, or ZeitBoard closed on it." : opened
          }`,
        };
      }
      const parts = [
        status.newestRecordEnd
          ? `Read ${at(status.reportedAt)}. Its newest night ends ${at(status.newestRecordEnd)}.`
          : `Read ${at(status.reportedAt)}. It holds no sleep from the last 30 days.`,
      ];
      if (status.held > 0) parts.push(heldWords(status.held));
      if (!status.backgroundReads) parts.push(opened);
      return { name, label: "Reading", state: "available", detail: parts.join(" ") };
    }
  }
}

/**
 * The likely cause of an estimate that is not current, in one or two
 * sentences, from the source that reported. Undefined when no source reported.
 */
export function sourceNote(statuses: SourceStatus[], now = new Date()): string | undefined {
  const status = statuses.find((candidate) => candidate.source === "health_connect");
  if (!status) return undefined;
  const at = (value: string) => momentInSentence(new Date(value), now);
  switch (status.outcome) {
    case "permission_missing":
      return `The phone has not been allowed to read Health Connect since ${sinceMoment(status.since, now)}.`;
    case "unavailable":
      return `Health Connect has not been available on the phone since ${sinceMoment(status.since, now)}.`;
    case "read_failed":
      return `The phone's last Health Connect read, ${at(status.reportedAt)}, failed.`;
    case "read": {
      if (quiet(status, now)) {
        const note = `The phone last read Health Connect ${at(status.reportedAt)}.`;
        return status.backgroundReads
          ? note
          : `${note} It reads only while ZeitBoard is open on the phone.`;
      }
      const found = status.newestRecordEnd
        ? `found no sleep after ${sinceMoment(status.newestRecordEnd, now)}`
        : "found no sleep in the last 30 days";
      const note = `The phone read Health Connect ${at(status.reportedAt)} and ${found}.`;
      return status.held > 0 ? `${note} ${heldWords(status.held)}` : note;
    }
  }
}
