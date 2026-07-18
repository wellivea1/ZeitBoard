import { findWailsMethod, type WailsRoot } from "./wailsBridge";

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

export interface SleepDataExport {
  fileName: string;
  json: string;
  generatedLabel: string;
  observationCount: number;
  correctionCount: number;
}

export interface SleepSourceSummary {
  source: string;
  provenance: string;
  total: number;
  corrected: number;
  suppressed: number;
}

// Real evidence composition for the Rhythm Sources tab: counts per source of
// what the estimator actually sees. No synthetic conflicts — overlap
// resolution happens inside the estimation engine.
export function summarizeSleepSources(entries: SleepEntry[]): SleepSourceSummary[] {
  const bySource = new Map<string, SleepSourceSummary>();
  for (const entry of entries) {
    const summary = bySource.get(entry.sourceLabel) ?? {
      source: entry.sourceLabel,
      provenance: entry.provenanceLabel,
      total: 0,
      corrected: 0,
      suppressed: 0,
    };
    summary.total += 1;
    if (entry.history.length > 0) summary.corrected += 1;
    if (entry.suppressed) summary.suppressed += 1;
    bySource.set(entry.sourceLabel, summary);
  }
  return [...bySource.values()].sort(
    (a, b) => b.total - a.total || a.source.localeCompare(b.source),
  );
}

// The most recently corrected entry drives the real correction inspector;
// the desktop log is ordered newest episode first. Undefined when no
// corrections exist yet.
export function latestCorrectedEntry(entries: SleepEntry[]): SleepEntry | undefined {
  return entries.find((entry) => entry.history.length > 0);
}

type UnknownRecord = Record<string, unknown>;

const emptySleepEntries: SleepEntriesData = {
  status: "unavailable",
  empty: true,
  message:
    "This browser preview is read-only. Open the ZeitBoard desktop app to add sleep entries.",
  entries: [],
};

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function classification(value: unknown): SleepClassification | undefined {
  return value === "principal" || value === "nap" ? value : undefined;
}

function nonNegativeInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isInteger(value) && value >= 0 ? value : undefined;
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

export function normalizeSleepDataExport(value: unknown): SleepDataExport | undefined {
  if (!isRecord(value)) return undefined;
  const fileName = str(value.fileName);
  const json = str(value.json);
  const generatedLabel = str(value.generatedLabel);
  const observationCount = nonNegativeInteger(value.observationCount);
  const correctionCount = nonNegativeInteger(value.correctionCount);
  if (
    !fileName ||
    !json ||
    !generatedLabel ||
    observationCount === undefined ||
    correctionCount === undefined
  ) {
    return undefined;
  }
  return { fileName, json, generatedLabel, observationCount, correctionCount };
}

export async function loadSleepEntries(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntriesData> {
  const method = findWailsMethod(root, ["ListSleepEntries"]);
  if (!method) return emptySleepEntries;

  const result = await method();
  return normalizeSleepEntries(result) ?? emptySleepEntries;
}

export async function addSleepEntry(
  input: SleepEntryInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntry> {
  const method = findWailsMethod(root, ["AddSleepEntry"]);
  if (!method) throw new Error("Manual sleep entry service is unavailable.");
  const result = await method(input);
  const entry = normalizeEntry(result);
  if (!entry) throw new Error("Manual sleep entry service returned an invalid entry.");
  return entry;
}

export async function exportSleepData(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepDataExport> {
  const method = findWailsMethod(root, ["ExportSleepData"]);
  if (!method) throw new Error("Sleep data export service is unavailable.");
  const result = await method();
  const exported = normalizeSleepDataExport(result);
  if (!exported) throw new Error("Sleep data export service returned an invalid export.");
  return exported;
}

export async function deleteSleepObservation(
  observationId: string,
  confirmation: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntriesData> {
  const method = findWailsMethod(root, ["DeleteSleepObservation"]);
  if (!method) throw new Error("Sleep data deletion service is unavailable.");
  const result = await method({ observationId, confirmation });
  const entries = normalizeSleepEntries(result);
  if (!entries) throw new Error("Sleep data deletion service returned an invalid entry list.");
  return entries;
}

export async function deleteAllSleepData(
  confirmation: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntriesData> {
  const method = findWailsMethod(root, ["DeleteAllSleepData"]);
  if (!method) throw new Error("Sleep data deletion service is unavailable.");
  const result = await method({ confirmation });
  const entries = normalizeSleepEntries(result);
  if (!entries) throw new Error("Sleep data deletion service returned an invalid entry list.");
  return entries;
}

export async function correctSleepEntry(
  input: SleepCorrectionInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntry> {
  const method = findWailsMethod(root, ["CorrectSleepEntry"]);
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
  const method = findWailsMethod(root, ["SuppressSleepEntry"]);
  if (!method) throw new Error("Manual sleep suppression service is unavailable.");
  const result = await method({ observationId });
  const entry = normalizeEntry(result);
  if (!entry) throw new Error("Manual sleep suppression service returned an invalid entry.");
  return entry;
}
