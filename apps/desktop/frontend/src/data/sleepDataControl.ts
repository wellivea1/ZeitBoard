import type { SleepDataExport } from "./sleepEntries";

export const deleteConfirmationToken = "DELETE";

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
