import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export interface ActivityCollection {
  enabled: boolean;
  running: boolean;
  supported: boolean;
  zoneId: string;
  recordCount: number;
  lastError: string;
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
  return v as unknown as ActivityCollection;
}

export async function loadActivityCollection() {
  return collection(await invoke("GetActivityCollection"));
}

export async function setActivityCollection(enabled: boolean, zoneId: string) {
  return collection(await invoke("SetActivityCollection", { enabled, zoneId }));
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
