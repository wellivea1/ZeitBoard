import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export type SleepClassification = "principal" | "nap" | "unknown";

export interface SleepEntryInput {
  startLocal: string;
  endLocal: string;
  zoneId: string;
  classification: SleepClassification;
}

export interface SleepCorrectionInput extends SleepEntryInput {
  observationId: string;
  reviewToken: string;
  excluded: boolean;
}

export interface SleepEntry {
  reviewToken: string;
  needsReview: boolean;
  sourceWindowLabel: string;
  activeEdits: SleepCorrection[];
  /** Whether the night has an earlier state for an undo to restore. */
  canUndo: boolean;
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
  supersedesCorrectionIds?: string[];
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

// What the log shows when it cannot be read: the reason, never "no entries".
export function sleepEntriesUnavailable(reason: unknown): SleepEntriesData {
  return {
    status: "unavailable",
    empty: true,
    message: reason instanceof Error ? reason.message : "The sleep log could not be read.",
    entries: [],
  };
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

/** A numbered page of the log, newest night first. */
export interface SleepLogPage extends SleepEntriesData {
  total: number;
  /** Zero-based; a page asked for past the end comes back as the last one. */
  page: number;
  pageSize: number;
}

/**
 * What the log is made of, per source, as the estimator sees it. The desktop
 * counts it; the window never receives the whole history to count itself.
 */
export interface SleepSources {
  status: "ready" | "empty" | "unavailable";
  message: string;
  total: number;
  correctedCount: number;
  suppressedCount: number;
  sources: SleepSourceSummary[];
  /** The newest night with an edit, for the correction inspector. */
  latestCorrected?: SleepEntry;
}

export function sleepLogUnavailable(reason: unknown): SleepLogPage {
  return { ...sleepEntriesUnavailable(reason), total: 0, page: 0, pageSize: 0 };
}

export function sleepSourcesUnavailable(reason: unknown): SleepSources {
  return {
    status: "unavailable",
    message: reason instanceof Error ? reason.message : "The sleep log could not be read.",
    total: 0,
    correctedCount: 0,
    suppressedCount: 0,
    sources: [],
  };
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
  return value === "principal" || value === "nap" || value === "unknown" ? value : undefined;
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
  const supersedesCorrectionIds = Array.isArray(value.supersedesCorrectionIds)
    ? value.supersedesCorrectionIds.filter((id): id is string => typeof id === "string")
    : [];
  return {
    correctionId,
    ...(supersedesCorrectionIds.length ? { supersedesCorrectionIds } : {}),
    createdLabel,
    reason,
    summary,
  };
}

function normalizeEntry(value: unknown): SleepEntry | undefined {
  if (!isRecord(value)) return undefined;
  const reviewToken = str(value.reviewToken);
  const sourceWindowLabel = str(value.sourceWindowLabel);
  if (
    !reviewToken ||
    !sourceWindowLabel ||
    typeof value.needsReview !== "boolean" ||
    !Array.isArray(value.activeEdits)
  )
    return undefined;
  const activeEdits = value.activeEdits.map(normalizeCorrection);
  if (activeEdits.some((edit) => !edit)) return undefined;
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
    reviewToken,
    sourceWindowLabel,
    needsReview: value.needsReview,
    activeEdits: activeEdits as SleepCorrection[],
    canUndo: value.canUndo === true,
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

export function normalizeSleepLogPage(value: unknown): SleepLogPage | undefined {
  const entries = normalizeSleepEntries(value);
  if (!entries || !isRecord(value)) return undefined;
  const total = nonNegativeInteger(value.total);
  const page = nonNegativeInteger(value.page);
  const pageSize = nonNegativeInteger(value.pageSize);
  if (total === undefined || page === undefined || pageSize === undefined) return undefined;
  return { ...entries, total, page, pageSize };
}

function normalizeSourceSummary(value: unknown): SleepSourceSummary | undefined {
  if (!isRecord(value)) return undefined;
  const source = str(value.source);
  const provenance = str(value.provenance);
  const total = nonNegativeInteger(value.total);
  const corrected = nonNegativeInteger(value.corrected);
  const suppressed = nonNegativeInteger(value.suppressed);
  if (
    !source ||
    !provenance ||
    total === undefined ||
    corrected === undefined ||
    suppressed === undefined
  ) {
    return undefined;
  }
  return { source, provenance, total, corrected, suppressed };
}

export function normalizeSleepSources(value: unknown): SleepSources | undefined {
  if (!isRecord(value) || !Array.isArray(value.sources)) return undefined;
  const status =
    value.status === "ready" || value.status === "empty" || value.status === "unavailable"
      ? value.status
      : undefined;
  const total = nonNegativeInteger(value.total);
  const correctedCount = nonNegativeInteger(value.correctedCount);
  const suppressedCount = nonNegativeInteger(value.suppressedCount);
  if (
    !status ||
    total === undefined ||
    correctedCount === undefined ||
    suppressedCount === undefined
  ) {
    return undefined;
  }
  const sources = value.sources.map(normalizeSourceSummary);
  if (sources.some((source) => !source)) return undefined;
  const latestCorrected =
    value.latestCorrected === undefined || value.latestCorrected === null
      ? undefined
      : normalizeEntry(value.latestCorrected);
  if (value.latestCorrected && !latestCorrected) return undefined;
  return {
    status,
    message: str(value.message) ?? "",
    total,
    correctedCount,
    suppressedCount,
    sources: sources as SleepSourceSummary[],
    ...(latestCorrected ? { latestCorrected } : {}),
  };
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

/** One page of the log, which shows fifty nights at a time. */
export async function loadSleepLogPage(
  page: number,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepLogPage> {
  const method = findWailsMethod(root, ["GetSleepLogPage"]);
  if (!method) return { ...emptySleepEntries, total: 0, page: 0, pageSize: 0 };
  const result = normalizeSleepLogPage(await method({ page }));
  if (!result) throw new Error("The sleep log returned an invalid page.");
  return result;
}

/** The nights that, as corrected, touch a range of at most 45 days. */
export async function loadSleepEntriesBetween(
  startAt: string,
  endAt: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntriesData> {
  const method = findWailsMethod(root, ["GetSleepEntriesBetween"]);
  if (!method) return emptySleepEntries;
  const result = normalizeSleepEntries(await method({ startAt, endAt }));
  if (!result) throw new Error("The sleep log returned an invalid range.");
  return result;
}

export async function loadSleepSources(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepSources> {
  const method = findWailsMethod(root, ["GetSleepSources"]);
  if (!method) return sleepSourcesUnavailable(new Error(emptySleepEntries.message));
  const result = normalizeSleepSources(await method());
  if (!result) throw new Error("The sleep log returned an invalid summary.");
  return result;
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

// Deleting returns nothing: each view re-reads what it shows.
export async function deleteSleepObservation(
  observationId: string,
  confirmation: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<void> {
  const method = findWailsMethod(root, ["DeleteSleepObservation"]);
  if (!method) throw new Error("Sleep data deletion service is unavailable.");
  await method({ observationId, confirmation });
}

export async function deleteAllSleepData(
  confirmation: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<void> {
  const method = findWailsMethod(root, ["DeleteAllSleepData"]);
  if (!method) throw new Error("Sleep data deletion service is unavailable.");
  await method({ confirmation });
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

/** Takes back the night's latest edit; the edit stays in its history. */
export async function undoSleepCorrection(
  observationId: string,
  reviewToken: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntry> {
  const method = findWailsMethod(root, ["UndoSleepCorrection"]);
  if (!method) throw new Error("Undoing a sleep edit is unavailable.");
  const entry = normalizeEntry(await method({ observationId, reviewToken }));
  if (!entry) throw new Error("Undoing a sleep edit returned an invalid entry.");
  return entry;
}

export async function suppressSleepEntry(
  observationId: string,
  reviewToken: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepEntry> {
  const method = findWailsMethod(root, ["SuppressSleepEntry"]);
  if (!method) throw new Error("Manual sleep suppression service is unavailable.");
  const result = await method({ observationId, reviewToken });
  const entry = normalizeEntry(result);
  if (!entry) throw new Error("Manual sleep suppression service returned an invalid entry.");
  return entry;
}
