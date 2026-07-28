import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export type SleepImportStatus = "ready" | "duplicate" | "invalid" | "imported";

export interface SleepImportInput {
  fileName: string;
  contents: string;
}

export interface SleepImportRow {
  rowNumber: number;
  observationId?: string;
  sourceRecordId?: string;
  startLabel?: string;
  endLabel?: string;
  zoneId?: string;
  classification?: string;
  status: SleepImportStatus;
  errors: string[];
  statusDetail?: string;
}

export interface SleepImportReport {
  fileName: string;
  format: "json" | "csv" | "";
  dryRun: boolean;
  totalRows: number;
  readyRows: number;
  duplicateRows: number;
  invalidRows: number;
  importedRows: number;
  canImport: boolean;
  errors: string[];
  rows: SleepImportRow[];
  message: string;
  importToken?: string;
  canceled: boolean;
}

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function stringValue(value: unknown, allowEmpty = false): string | undefined {
  if (typeof value !== "string") return undefined;
  if (!allowEmpty && value.length === 0) return undefined;
  return value;
}

function optionalString(value: unknown): string | undefined {
  return value === undefined || value === "" ? undefined : stringValue(value);
}

function nonNegativeInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isInteger(value) && value >= 0 ? value : undefined;
}

function stringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const result: string[] = [];
  for (const item of value) {
    const text = stringValue(item);
    if (!text) return undefined;
    result.push(text);
  }
  return result;
}

function importStatus(value: unknown): SleepImportStatus | undefined {
  return value === "ready" || value === "duplicate" || value === "invalid" || value === "imported"
    ? value
    : undefined;
}

function normalizeSleepImportRow(value: unknown): SleepImportRow | undefined {
  if (!isRecord(value)) return undefined;
  const rowNumber = nonNegativeInteger(value.rowNumber);
  const status = importStatus(value.status);
  const errors = stringArray(value.errors);
  if (rowNumber === undefined || rowNumber < 1 || !status || !errors) return undefined;
  return {
    rowNumber,
    status,
    errors,
    ...(optionalString(value.observationId)
      ? { observationId: optionalString(value.observationId) }
      : {}),
    ...(optionalString(value.sourceRecordId)
      ? { sourceRecordId: optionalString(value.sourceRecordId) }
      : {}),
    ...(optionalString(value.startLabel) ? { startLabel: optionalString(value.startLabel) } : {}),
    ...(optionalString(value.endLabel) ? { endLabel: optionalString(value.endLabel) } : {}),
    ...(optionalString(value.zoneId) ? { zoneId: optionalString(value.zoneId) } : {}),
    ...(optionalString(value.classification)
      ? { classification: optionalString(value.classification) }
      : {}),
    ...(optionalString(value.statusDetail)
      ? { statusDetail: optionalString(value.statusDetail) }
      : {}),
  };
}

export function normalizeSleepImportReport(value: unknown): SleepImportReport | undefined {
  if (!isRecord(value) || !Array.isArray(value.rows)) return undefined;
  const fileName = stringValue(value.fileName);
  const format =
    value.format === "json" || value.format === "csv" || value.format === ""
      ? value.format
      : undefined;
  const totalRows = nonNegativeInteger(value.totalRows);
  const readyRows = nonNegativeInteger(value.readyRows);
  const duplicateRows = nonNegativeInteger(value.duplicateRows);
  const invalidRows = nonNegativeInteger(value.invalidRows);
  const importedRows = nonNegativeInteger(value.importedRows);
  const errors = stringArray(value.errors);
  const message = stringValue(value.message);
  const importToken = optionalString(value.importToken);
  const canceled = value.canceled === undefined ? false : value.canceled;
  if (
    !fileName ||
    format === undefined ||
    typeof value.dryRun !== "boolean" ||
    totalRows === undefined ||
    readyRows === undefined ||
    duplicateRows === undefined ||
    invalidRows === undefined ||
    importedRows === undefined ||
    typeof value.canImport !== "boolean" ||
    !errors ||
    !message ||
    typeof canceled !== "boolean"
  ) {
    return undefined;
  }
  const rows: SleepImportRow[] = [];
  for (const item of value.rows) {
    const row = normalizeSleepImportRow(item);
    if (!row) return undefined;
    rows.push(row);
  }
  const statusCounts = { ready: 0, duplicate: 0, invalid: 0, imported: 0 };
  for (const row of rows) statusCounts[row.status] += 1;
  if (
    rows.length !== totalRows ||
    statusCounts.ready !== readyRows ||
    statusCounts.duplicate !== duplicateRows ||
    statusCounts.invalid !== invalidRows ||
    statusCounts.imported !== importedRows ||
    (value.canImport && (!value.dryRun || readyRows === 0 || invalidRows > 0)) ||
    (canceled && (value.canImport || totalRows !== 0 || Boolean(importToken)))
  ) {
    return undefined;
  }
  return {
    fileName,
    format,
    dryRun: value.dryRun,
    totalRows,
    readyRows,
    duplicateRows,
    invalidRows,
    importedRows,
    canImport: value.canImport,
    errors,
    rows,
    message,
    ...(importToken ? { importToken } : {}),
    canceled,
  };
}

export function hasNativeSleepImport(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): boolean {
  return Boolean(
    findWailsMethod(root, ["PreviewSleepImportFile"]) &&
    findWailsMethod(root, ["ImportSleepDataFile"]),
  );
}

export async function previewNativeSleepImport(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepImportReport> {
  const method = findWailsMethod(root, ["PreviewSleepImportFile"]);
  if (!method) throw new Error("Native sleep import is unavailable.");
  const report = normalizeSleepImportReport(await method());
  if (!report || (!report.canceled && !report.importToken)) {
    throw new Error("Sleep import preview returned an invalid native selection.");
  }
  return report;
}

export async function importNativeSleepData(
  importToken: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepImportReport> {
  const method = findWailsMethod(root, ["ImportSleepDataFile"]);
  if (!method) throw new Error("Native sleep import is unavailable.");
  const report = normalizeSleepImportReport(await method({ importToken }));
  if (!report || report.canceled) throw new Error("Sleep import returned an invalid report.");
  return report;
}

export async function previewSleepImport(
  input: SleepImportInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepImportReport> {
  const method = findWailsMethod(root, ["PreviewSleepImport"]);
  if (!method) throw new Error("Sleep import preview is available in the ZeitBoard desktop app.");
  const report = normalizeSleepImportReport(await method(input));
  if (!report) throw new Error("Sleep import preview returned an invalid report.");
  return report;
}

export async function importSleepData(
  input: SleepImportInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepImportReport> {
  const method = findWailsMethod(root, ["ImportSleepData"]);
  if (!method) throw new Error("Sleep import is available in the ZeitBoard desktop app.");
  const report = normalizeSleepImportReport(await method(input));
  if (!report) throw new Error("Sleep import returned an invalid report.");
  return report;
}

export const transcriptionTemplateCSV =
  "source_record_id,start_local,end_local,zone_id,classification,review_status\r\n";

export function downloadTranscriptionTemplate(): boolean {
  if (
    (typeof navigator !== "undefined" && navigator.userAgent.toLowerCase().includes("jsdom")) ||
    typeof Blob === "undefined" ||
    typeof URL === "undefined" ||
    typeof URL.createObjectURL !== "function" ||
    typeof document === "undefined"
  ) {
    return false;
  }
  const blob = new Blob([transcriptionTemplateCSV], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = "zeitboard-sleep-transcription-template.csv";
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
  return true;
}
