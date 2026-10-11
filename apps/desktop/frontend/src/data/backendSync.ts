import { notifyReviewQueueChanged } from "./reviewQueue";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export interface BackendSyncInput {
  enabled: boolean;
  backendUrl: string;
  enrollmentSecret: string;
  deviceLabel: string;
  insecureSkipVerify: boolean;
  /** PEM of the certificate trusted for the server; empty for none (ADR-0055). */
  serverCertificate: string;
}

/** A certificate the owner chose for their server, and what a person checks it by. */
export interface ServerCertificate {
  pem: string;
  fingerprint: string;
  subject: string;
  expiresAt: string;
}

export interface BackendSyncStatus {
  enabled: boolean;
  status: "off" | "connected" | "error";
  backendUrl: string;
  deviceId: string;
  insecureSkipVerify: boolean;
  serverCertificate?: ServerCertificate;
  /** When sync last completed, ISO 8601; absent before the first. */
  lastSyncAt?: string;
  lastError: string;
  pendingPushCount: number;
  pendingErasureCount: number;
  waitingRecordCount: number;
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
  lastError: "",
  pendingPushCount: 0,
  pendingErasureCount: 0,
  waitingRecordCount: 0,
  taskConflictCount: 0,
  pushedCount: 0,
  pulledCount: 0,
  cursor: 0,
};

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function instant(value: unknown): string | undefined {
  return typeof value === "string" && Number.isFinite(Date.parse(value)) ? value : undefined;
}

function str(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function normalizeServerCertificate(value: unknown): ServerCertificate | undefined {
  if (!isRecord(value)) return undefined;
  const pem = str(value.pem);
  const fingerprint = str(value.fingerprint);
  const expiresAt = instant(value.expiresAt);
  if (
    !pem.includes("CERTIFICATE") ||
    !/^[0-9A-F]{2}(:[0-9A-F]{2}){31}$/.test(fingerprint) ||
    !expiresAt
  ) {
    return undefined;
  }
  return { pem, fingerprint, subject: str(value.subject), expiresAt };
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
    ...(normalizeServerCertificate(value.serverCertificate)
      ? { serverCertificate: normalizeServerCertificate(value.serverCertificate) }
      : {}),
    ...(instant(value.lastSyncAt) ? { lastSyncAt: instant(value.lastSyncAt) } : {}),
    lastError: str(value.lastError),
    pendingPushCount: count(value.pendingPushCount),
    pendingErasureCount: count(value.pendingErasureCount),
    waitingRecordCount: count(value.waitingRecordCount),
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

/** Opens a certificate file; undefined when the owner cancels. */
export async function chooseServerCertificate(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ServerCertificate | undefined> {
  const method = findWailsMethod(root, ["ChooseServerCertificate"]);
  if (!method) throw new Error("Choosing a certificate needs the desktop app.");
  const result = await method();
  if (isRecord(result) && result.canceled === true) return undefined;
  const certificate = normalizeServerCertificate(result);
  if (!certificate) throw new Error("The desktop returned a certificate it could not describe.");
  return certificate;
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
