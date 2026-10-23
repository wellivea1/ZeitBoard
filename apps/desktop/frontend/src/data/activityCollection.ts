import { findWailsMethod, type WailsRoot } from "./wailsBridge";

/** Reading Windows' own log of sleep, wake, shutdown and start (ADR-0060). */
export interface PowerLogReading {
  supported: boolean;
  enabled: boolean;
  /** Entries kept from the log. */
  entries: number;
  /** The date of the oldest, once there is one. */
  sinceLabel: string;
  lastError: string;
}

export interface ActivityCollection {
  enabled: boolean;
  running: boolean;
  supported: boolean;
  zoneId: string;
  recordCount: number;
  lastError: string;
  powerLog: PowerLogReading;
}

export type ActivityState = "unavailable" | "off" | "stopped" | "running";

/**
 * What desktop activity is doing, in one word for every screen that says it:
 * unavailable where its status cannot be read or the system cannot record it,
 * even if it was left on there, as in a data folder brought from Windows;
 * otherwise off, stopped or running.
 */
export function activityState(status: ActivityCollection | null | undefined): ActivityState {
  if (!status || !status.supported) return "unavailable";
  if (status.running) return "running";
  return status.enabled ? "stopped" : "off";
}

export const activityStateLabels: Record<ActivityState, string> = {
  unavailable: "Unavailable",
  off: "Off",
  stopped: "Stopped",
  running: "Running",
};

const noPowerLog: PowerLogReading = {
  supported: false,
  enabled: false,
  entries: 0,
  sinceLabel: "",
  lastError: "",
};

function powerLog(value: unknown): PowerLogReading {
  if (typeof value !== "object" || value === null) return noPowerLog;
  const v = value as Record<string, unknown>;
  return {
    supported: v.supported === true,
    enabled: v.enabled === true,
    entries:
      typeof v.entries === "number" && Number.isSafeInteger(v.entries) && v.entries >= 0
        ? v.entries
        : 0,
    sinceLabel: typeof v.sinceLabel === "string" ? v.sinceLabel : "",
    lastError: typeof v.lastError === "string" ? v.lastError : "",
  };
}

async function invoke(name: string, input?: unknown, root: WailsRoot = globalThis as WailsRoot) {
  const method = findWailsMethod(root, [name]);
  if (!method) throw new Error("Open the ZeitBoard desktop app to manage activity collection.");
  return method(input);
}

function collection(value: unknown): ActivityCollection {
  if (typeof value !== "object" || value === null)
    throw new Error("Activity status is unavailable.");
  const v = value as Record<string, unknown>;
  if (
    typeof v.enabled !== "boolean" ||
    typeof v.running !== "boolean" ||
    typeof v.supported !== "boolean" ||
    typeof v.zoneId !== "string" ||
    typeof v.recordCount !== "number" ||
    !Number.isSafeInteger(v.recordCount) ||
    v.recordCount < 0 ||
    typeof v.lastError !== "string"
  )
    throw new Error("Activity status is invalid. Refresh to try again.");
  return { ...(v as unknown as ActivityCollection), powerLog: powerLog(v.powerLog) };
}

export async function loadActivityCollection() {
  return collection(await invoke("GetActivityCollection"));
}

export async function setActivityCollection(enabled: boolean, zoneId: string) {
  return collection(await invoke("SetActivityCollection", { enabled, zoneId }));
}

export async function setPowerLogReading(enabled: boolean) {
  return collection(await invoke("SetPowerLogReading", { enabled }));
}

export async function deleteActivityData(confirmation: string) {
  return collection(await invoke("DeleteActivityData", confirmation));
}

export async function saveActivityDataExport(): Promise<string> {
  const value = await invoke("SaveActivityDataExport");
  if (typeof value !== "object" || value === null)
    throw new Error("Activity export status is unavailable.");
  const v = value as Record<string, unknown>;
  if (v.saved === false) return "Export canceled.";
  if (v.saved !== true || typeof v.fileName !== "string" || typeof v.recordCount !== "number") {
    throw new Error("Activity export status is invalid.");
  }
  return `Exported ${v.recordCount} activity records to ${v.fileName}.`;
}
