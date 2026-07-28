import { exportSleepData, type SleepDataExport } from "./sleepEntries";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export const deleteConfirmationToken = "DELETE";

export interface SleepDataExportSummary {
  fileName: string;
  generatedLabel: string;
  observationCount: number;
  correctionCount: number;
  preview: string;
  previewTruncated: boolean;
  saved: boolean;
  canceled: boolean;
}

function nonNegativeInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : undefined;
}

export function downloadSleepDataExport(exported: SleepDataExport): boolean {
  if (
    (typeof navigator !== "undefined" && navigator.userAgent.toLowerCase().includes("jsdom")) ||
    typeof Blob === "undefined" ||
    typeof URL === "undefined" ||
    typeof URL.createObjectURL !== "function" ||
    typeof document === "undefined"
  ) {
    return false;
  }

  const blob = new Blob([exported.json], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = exported.fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
  return true;
}

function normalizeSleepDataExportSummary(value: unknown): SleepDataExportSummary | undefined {
  if (typeof value !== "object" || value === null) return undefined;
  const record = value as Record<string, unknown>;
  const observationCount = nonNegativeInteger(record.observationCount);
  const correctionCount = nonNegativeInteger(record.correctionCount);
  if (
    typeof record.fileName !== "string" ||
    record.fileName.length === 0 ||
    typeof record.generatedLabel !== "string" ||
    record.generatedLabel.length === 0 ||
    observationCount === undefined ||
    correctionCount === undefined ||
    typeof record.preview !== "string" ||
    typeof record.previewTruncated !== "boolean" ||
    typeof record.saved !== "boolean" ||
    typeof record.canceled !== "boolean" ||
    (record.saved && record.canceled)
  ) {
    return undefined;
  }
  return {
    fileName: record.fileName,
    generatedLabel: record.generatedLabel,
    observationCount,
    correctionCount,
    preview: record.preview,
    previewTruncated: record.previewTruncated,
    saved: record.saved,
    canceled: record.canceled,
  };
}

export async function saveSleepDataExport(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<SleepDataExportSummary> {
  const nativeSave = findWailsMethod(root, ["SaveSleepDataExport"]);
  if (nativeSave) {
    const summary = normalizeSleepDataExportSummary(await nativeSave());
    if (!summary) throw new Error("Sleep data export service returned an invalid summary.");
    return summary;
  }

  const exported = await exportSleepData(root);
  const saved = downloadSleepDataExport(exported);
  const previewLength = 512;
  return {
    fileName: exported.fileName,
    generatedLabel: exported.generatedLabel,
    observationCount: exported.observationCount,
    correctionCount: exported.correctionCount,
    preview: exported.json.slice(0, previewLength),
    previewTruncated: exported.json.length > previewLength,
    saved,
    canceled: false,
  };
}
