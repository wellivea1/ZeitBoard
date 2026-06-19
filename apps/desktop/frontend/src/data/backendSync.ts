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
  pushedCount: number;
  pulledCount: number;
  cursor: number;
}

type UnknownRecord = Record<string, unknown>;
type WailsMethod = (input?: unknown) => Promise<unknown>;

interface WailsRoot {
  go?: Record<string, Record<string, Record<string, unknown>>>;
}

const unavailableStatus: BackendSyncStatus = {
  enabled: false,
  status: "off",
  backendUrl: "",
  deviceId: "",
  insecureSkipVerify: false,
  lastSyncLabel: "Not synced yet",
  lastError: "",
  pendingPushCount: 0,
  pushedCount: 0,
  pulledCount: 0,
  cursor: 0,
};

function findMethod(root: WailsRoot, names: readonly string[]): WailsMethod | undefined {
  const packages = root.go;
  if (!packages) return undefined;

  for (const packageValue of Object.values(packages)) {
    for (const serviceValue of Object.values(packageValue)) {
      for (const methodName of names) {
        const candidate = serviceValue[methodName];
        if (typeof candidate === "function") {
          return (candidate as WailsMethod).bind(serviceValue);
        }
      }
    }
  }

  return undefined;
}

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
    pushedCount: count(value.pushedCount),
    pulledCount: count(value.pulledCount),
    cursor: count(value.cursor),
  };
}

export async function loadBackendSyncStatus(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendSyncStatus> {
  const method = findMethod(root, ["GetBackendSyncStatus"]);
  if (!method) return unavailableStatus;
  const result = await method();
  return normalizeBackendSyncStatus(result) ?? unavailableStatus;
}

export async function configureBackendSync(
  input: BackendSyncInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendSyncStatus> {
  const method = findMethod(root, ["ConfigureBackendSync"]);
  if (!method) throw new Error("Backend sync service is unavailable.");
  const result = await method(input);
  const status = normalizeBackendSyncStatus(result);
  if (!status) throw new Error("Backend sync service returned an invalid status.");
  return status;
}

export async function disableBackendSync(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendSyncStatus> {
  const method = findMethod(root, ["DisableBackendSync"]);
  if (!method) throw new Error("Backend sync service is unavailable.");
  const result = await method();
  const status = normalizeBackendSyncStatus(result);
  if (!status) throw new Error("Backend sync service returned an invalid status.");
  return status;
}

export async function syncNow(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendSyncStatus> {
  const method = findMethod(root, ["SyncNow"]);
  if (!method) throw new Error("Backend sync service is unavailable.");
  const result = await method();
  const status = normalizeBackendSyncStatus(result);
  if (!status) throw new Error("Backend sync service returned an invalid status.");
  return status;
}
