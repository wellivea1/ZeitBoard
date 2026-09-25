import { notifyReviewQueueChanged } from "./reviewQueue";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export interface BackendSyncInput {
  enabled: boolean;
  backendUrl: string;
  enrollmentSecret: string;
  deviceLabel: string;
  insecureSkipVerify: boolean;
}

export interface BackendSyncStatus {
  enabled: boolean;
  status: "off" | "connected" | "error";
  backendUrl: string;
  deviceId: string;
  insecureSkipVerify: boolean;
  lastSyncLabel: string;
  lastError: string;
  pendingPushCount: number;
  pendingErasureCount: number;
  waitingCorrectionCount: number;
  taskConflictCount: number;
  pushedCount: number;
  pulledCount: number;
  cursor: number;
}

type UnknownRecord = Record<string, unknown>;
const unavailableStatus: BackendSyncStatus = {
  enabled: false,
  status: "off",
  backendUrl: "",
  deviceId: "",
  insecureSkipVerify: false,
  lastSyncLabel: "Not synced yet",
  lastError: "",
  pendingPushCount: 0,
  pendingErasureCount: 0,
  waitingCorrectionCount: 0,
  taskConflictCount: 0,
  pushedCount: 0,
  pulledCount: 0,
  cursor: 0,
};

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function count(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

export function normalizeBackendSyncStatus(value: unknown): BackendSyncStatus | undefined {
  if (!isRecord(value)) return undefined;
  const rawStatus = value.status;
  const status =
    rawStatus === "connected" || rawStatus === "error" || rawStatus === "off"
      ? rawStatus
      : value.enabled === true
        ? "error"
        : "off";
  return {
    enabled: value.enabled === true,
    status,
    backendUrl: str(value.backendUrl),
    deviceId: str(value.deviceId),
    insecureSkipVerify: value.insecureSkipVerify === true,
    lastSyncLabel: str(value.lastSyncLabel) || "Not synced yet",
    lastError: str(value.lastError),
    pendingPushCount: count(value.pendingPushCount),
    pendingErasureCount: count(value.pendingErasureCount),
    waitingCorrectionCount: count(value.waitingCorrectionCount),
    taskConflictCount: count(value.taskConflictCount),
    pushedCount: count(value.pushedCount),
    pulledCount: count(value.pulledCount),
    cursor: count(value.cursor),
  };
}

export async function loadBackendSyncStatus(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendSyncStatus> {
  const method = findWailsMethod(root, ["GetBackendSyncStatus"]);
  if (!method) return unavailableStatus;
  const result = await method();
  return normalizeBackendSyncStatus(result) ?? unavailableStatus;
}

export async function configureBackendSync(
  input: BackendSyncInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendSyncStatus> {
  const method = findWailsMethod(root, ["ConfigureBackendSync"]);
  if (!method) throw new Error("Backend sync service is unavailable.");
  const result = await method(input);
  const status = normalizeBackendSyncStatus(result);
  if (!status) throw new Error("Backend sync service returned an invalid status.");
  notifyReviewQueueChanged();
  return status;
}

export async function disableBackendSync(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendSyncStatus> {
  const method = findWailsMethod(root, ["DisableBackendSync"]);
  if (!method) throw new Error("Backend sync service is unavailable.");
  const result = await method();
  const status = normalizeBackendSyncStatus(result);
  if (!status) throw new Error("Backend sync service returned an invalid status.");
  notifyReviewQueueChanged();
  return status;
}

export async function syncNow(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendSyncStatus> {
  const method = findWailsMethod(root, ["SyncNow"]);
  if (!method) throw new Error("Backend sync service is unavailable.");
  const result = await method();
  const status = normalizeBackendSyncStatus(result);
  if (!status) throw new Error("Backend sync service returned an invalid status.");
  notifyReviewQueueChanged();
  return status;
}
