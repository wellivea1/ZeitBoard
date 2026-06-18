export type SleepClassification = "principal" | "nap";

export interface SleepEntryInput {
  startLocal: string;
  endLocal: string;
  zoneId: string;
  classification: SleepClassification;
}

export interface SleepCorrectionInput extends SleepEntryInput {
  observationId: string;
}

export interface SleepEntry {
  observationId: string;
  startLocal: string;
  endLocal: string;
  startLabel: string;
  endLabel: string;
  zoneId: string;
  classification: SleepClassification;
  effectiveStartLocal: string;
  effectiveEndLocal: string;
  effectiveStartLabel: string;
  effectiveEndLabel: string;
  effectiveClassification: SleepClassification;
  durationLabel: string;
  suppressed: boolean;
  sourceLabel: string;
  provenanceLabel: string;
  history: SleepCorrection[];
}

export interface SleepCorrection {
  correctionId: string;
  supersedesCorrectionId?: string;
  createdLabel: string;
  reason: string;
  summary: string;
}

export interface SleepEntriesData {
  status: "ready" | "empty" | "unavailable";
  empty: boolean;
  message: string;
  entries: SleepEntry[];
}

type UnknownRecord = Record<string, unknown>;
type WailsMethod = (input?: unknown) => Promise<unknown>;

interface WailsRoot {
  go?: Record<string, Record<string, Record<string, unknown>>>;
}

const emptySleepEntries: SleepEntriesData = {
  status: "unavailable",
  empty: true,
  message: "Manual sleep entry is available in the desktop app service.",
  entries: [],
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

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function classification(value: unknown): SleepClassification | undefined {
  return value === "principal" || value === "nap" ? value : undefined;
}

function normalizeCorrection(value: unknown): SleepCorrection | undefined {
  if (!isRecord(value)) return undefined;
  const correctionId = str(value.correctionId);
  const createdLabel = str(value.createdLabel);
  const reason = str(value.reason);
  const summary = str(value.summary);
  if (!correctionId || !createdLabel || !reason || !summary) return undefined;
  const supersedesCorrectionId = str(value.supersedesCorrectionId);
  return {
    correctionId,
    ...(supersedesCorrectionId ? { supersedesCorrectionId } : {}),
    createdLabel,
    reason,
    summary,
  };
}

function normalizeEntry(value: unknown): SleepEntry | undefined {
  if (!isRecord(value)) return undefined;
  const observationId = str(value.observationId);
  const startLocal = str(value.startLocal);
  const endLocal = str(value.endLocal);
  const startLabel = str(value.startLabel);
  const endLabel = str(value.endLabel);
  const zoneId = str(value.zoneId);
  const rawClassification = classification(value.classification);
  const effectiveStartLocal = str(value.effectiveStartLocal);
  const effectiveEndLocal = str(value.effectiveEndLocal);
  const effectiveStartLabel = str(value.effectiveStartLabel);
  const effectiveEndLabel = str(value.effectiveEndLabel);
  const effectiveClassification = classification(value.effectiveClassification);
  const durationLabel = str(value.durationLabel);
  const sourceLabel = str(value.sourceLabel);
  const provenanceLabel = str(value.provenanceLabel);
  if (
    !observationId ||
    !startLocal ||
    !endLocal ||
    !startLabel ||
    !endLabel ||
    !zoneId ||
    !rawClassification ||
    !effectiveStartLocal ||
    !effectiveEndLocal ||
    !effectiveStartLabel ||
    !effectiveEndLabel ||
    !effectiveClassification ||
    !durationLabel ||
    typeof value.suppressed !== "boolean" ||
    !sourceLabel ||
    !provenanceLabel ||
    !Array.isArray(value.history)
  ) {
    return undefined;
  }
  const history: SleepCorrection[] = [];
  for (const item of value.history) {
    const normalized = normalizeCorrection(item);
    if (!normalized) return undefined;
    history.push(normalized);
  }
  return {
    observationId,
    startLocal,
    endLocal,
    startLabel,
    endLabel,
    zoneId,
    classification: rawClassification,
    effectiveStartLocal,
    effectiveEndLocal,
    effectiveStartLabel,
    effectiveEndLabel,
    effectiveClassification,
    durationLabel,
    suppressed: value.suppressed,
    sourceLabel,
    provenanceLabel,
    history,
  };
}

export function normalizeSleepEntries(value: unknown): SleepEntriesData | undefined {
  if (!isRecord(value) || !Array.isArray(value.entries)) return undefined;
  const status =
    value.status === "ready" || value.status === "empty" || value.status === "unavailable"
      ? value.status
      : undefined;
  const message = str(value.message);
  if (!status || typeof value.empty !== "boolean" || !message) return undefined;
  const entries: SleepEntry[] = [];
  for (const item of value.entries) {
    const entry = normalizeEntry(item);
    if (!entry) return undefined;
    entries.push(entry);
  }
  return { status, empty: value.empty, message, entries };
}

export async function loadSleepEntries(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntriesData> {
  const method = findMethod(root, ["ListSleepEntries"]);
  if (!method) return emptySleepEntries;

  const result = await method();
  return normalizeSleepEntries(result) ?? emptySleepEntries;
}

export async function addSleepEntry(
  input: SleepEntryInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntry> {
  const method = findMethod(root, ["AddSleepEntry"]);
  if (!method) throw new Error("Manual sleep entry service is unavailable.");
  const result = await method(input);
  const entry = normalizeEntry(result);
  if (!entry) throw new Error("Manual sleep entry service returned an invalid entry.");
  return entry;
}

export async function correctSleepEntry(
  input: SleepCorrectionInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntry> {
  const method = findMethod(root, ["CorrectSleepEntry"]);
  if (!method) throw new Error("Manual sleep correction service is unavailable.");
  const result = await method(input);
  const entry = normalizeEntry(result);
  if (!entry) throw new Error("Manual sleep correction service returned an invalid entry.");
  return entry;
}

export async function suppressSleepEntry(
  observationId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntry> {
  const method = findMethod(root, ["SuppressSleepEntry"]);
  if (!method) throw new Error("Manual sleep suppression service is unavailable.");
  const result = await method({ observationId });
  const entry = normalizeEntry(result);
  if (!entry) throw new Error("Manual sleep suppression service returned an invalid entry.");
  return entry;
}
