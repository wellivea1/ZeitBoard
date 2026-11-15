import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// A reported status (ADR-0070): what the owner says about the present, as
// against what the sleep records predict. It is the owner's word, with when it
// was said and when it stops standing, and never moves the forecast.

export const statusChangedEvent = "zeitboard:status-changed";

export type ReportableStatus = "awake" | "trying_to_sleep" | "do_not_disturb";

/** The three things the owner can report, in the order they are offered. */
export const reportableStatuses: { status: ReportableStatus; label: string }[] = [
  { status: "awake", label: "Awake" },
  { status: "trying_to_sleep", label: "Trying to sleep" },
  { status: "do_not_disturb", label: "Do not disturb" },
];

export interface StatusSettings {
  /** Whether a new report may be shown by the share links that allow it. */
  shareByDefault: boolean;
  /** Whether the sleep buttons also report "trying to sleep" and "awake". */
  followSleepTaps: boolean;
}

export interface ReportedStatus {
  status: "ok" | "unavailable";
  message?: string;
  /** What stands now, or "" when nothing does. */
  reported: ReportableStatus | "";
  reportedLabel: string;
  saidLabel: string;
  untilLabel: string;
  shared: boolean;
  /** Where the report has got to. */
  delivery: "this_computer" | "waiting" | "server";
  settings: StatusSettings;
  settingsRevision: number;
}

export const statusUnavailable: ReportedStatus = {
  status: "unavailable",
  message: "A status is reported from the ZeitBoard desktop app.",
  reported: "",
  reportedLabel: "",
  saidLabel: "",
  untilLabel: "",
  shared: false,
  delivery: "this_computer",
  settings: { shareByDefault: false, followSleepTaps: false },
  settingsRevision: 0,
};

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

const text = (value: unknown) => (typeof value === "string" ? value : "");

export function normalizeReportedStatus(value: unknown): ReportedStatus | undefined {
  if (!isRecord(value) || !isRecord(value.settings)) return undefined;
  const status = value.status === "ok" || value.status === "unavailable" ? value.status : undefined;
  const reported = text(value.reported);
  const known = reported === "" || reportableStatuses.some((item) => item.status === reported);
  const delivery =
    value.delivery === "this_computer" ||
    value.delivery === "waiting" ||
    value.delivery === "server"
      ? value.delivery
      : undefined;
  const revision = value.settingsRevision;
  if (
    !status ||
    !known ||
    !delivery ||
    typeof value.shared !== "boolean" ||
    typeof value.settings.shareByDefault !== "boolean" ||
    typeof value.settings.followSleepTaps !== "boolean" ||
    typeof revision !== "number" ||
    !Number.isSafeInteger(revision) ||
    revision < 0
  ) {
    return undefined;
  }
  // A status that stands says what it is, when it was said and until when.
  if (
    reported &&
    (!text(value.reportedLabel) || !text(value.saidLabel) || !text(value.untilLabel))
  ) {
    return undefined;
  }
  const message = text(value.message);
  return {
    status,
    ...(message ? { message } : {}),
    reported: reported as ReportedStatus["reported"],
    reportedLabel: text(value.reportedLabel),
    saidLabel: text(value.saidLabel),
    untilLabel: text(value.untilLabel),
    shared: value.shared,
    delivery,
    settings: {
      shareByDefault: value.settings.shareByDefault,
      followSleepTaps: value.settings.followSleepTaps,
    },
    settingsRevision: revision,
  };
}

function notify() {
  if (typeof window !== "undefined") window.dispatchEvent(new Event(statusChangedEvent));
}

async function call(name: string, input: unknown, root: WailsRoot): Promise<ReportedStatus> {
  const method = findWailsMethod(root, [name]);
  if (!method) throw new Error("A status is reported from the ZeitBoard desktop app.");
  const result = normalizeReportedStatus(await method(input));
  if (!result) throw new Error("The status could not be read back.");
  return result;
}

export async function loadReportedStatus(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ReportedStatus> {
  if (!findWailsMethod(root, ["GetReportedStatus"])) return statusUnavailable;
  return call("GetReportedStatus", undefined, root);
}

export async function reportStatus(
  input: { status: ReportableStatus; shared: boolean; minutes?: number },
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ReportedStatus> {
  const result = await call(
    "ReportStatus",
    { status: input.status, minutes: input.minutes ?? 0, shared: input.shared },
    root,
  );
  notify();
  return result;
}

export async function clearReportedStatus(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ReportedStatus> {
  const result = await call("ClearReportedStatus", undefined, root);
  notify();
  return result;
}

export async function saveStatusSettings(
  settings: StatusSettings,
  baseRevision: number,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ReportedStatus> {
  const result = await call("SaveStatusSettings", { baseRevision, ...settings }, root);
  notify();
  return result;
}

/** Where a report has got to, in the owner's words. */
export function deliveryWords(status: ReportedStatus): string {
  switch (status.delivery) {
    case "server":
      return "It has reached your server.";
    case "waiting":
      return "Not yet on your server; it is sent at the next sync.";
    default:
      return "Sync is off, so it stays on this computer.";
  }
}

/**
 * How long a report may be made to last. Zero is the status's own usual
 * length: 4 hours awake, 10 trying to sleep, 2 for do not disturb.
 */
export const statusLifetimes: { minutes: number; label: string }[] = [
  { minutes: 0, label: "the usual time" },
  { minutes: 60, label: "1 hour" },
  { minutes: 120, label: "2 hours" },
  { minutes: 240, label: "4 hours" },
  { minutes: 480, label: "8 hours" },
  { minutes: 720, label: "12 hours" },
];
