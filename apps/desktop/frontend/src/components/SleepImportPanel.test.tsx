import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SleepImportPanel } from "./SleepImportPanel";

const readyReport = {
  fileName: "owner-history.json",
  format: "json",
  dryRun: true,
  totalRows: 1,
  readyRows: 1,
  duplicateRows: 0,
  invalidRows: 0,
  importedRows: 0,
  canImport: true,
  errors: [],
  message: "1 row is ready to append; 0 exact duplicates are already present.",
  rows: [
    {
      rowNumber: 1,
      observationId: "obs_owner_001",
      sourceRecordId: "fitbit-owner-001",
      startLabel: "Jan 1, 2023, 12:00 AM",
      endLabel: "Jan 1, 2023, 8:00 AM",
      zoneId: "America/New_York",
      classification: "principal",
      status: "ready",
      errors: [],
      statusDetail: "Ready to append",
    },
  ],
};

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("SleepImportPanel", () => {
  it("previews before enabling the separate append action", async () => {
    const preview = vi.fn(async () => readyReport);
    const commit = vi.fn(async () => ({
      ...readyReport,
      dryRun: false,
      readyRows: 0,
      importedRows: 1,
      canImport: false,
      message: "Imported 1 sleep observation; 0 exact duplicates were already present.",
      rows: [{ ...readyReport.rows[0], status: "imported", statusDetail: "Imported" }],
    }));
    (globalThis as { go?: unknown }).go = {
      main: { App: { PreviewSleepImport: preview, ImportSleepData: commit } },
    };
    const onImported = vi.fn(async () => undefined);
    render(<SleepImportPanel onImported={onImported} />);

    const importButton = screen.getByRole("button", { name: "Import 0 ready rows" });
    expect(importButton).toBeDisabled();
    const file = new File(["{}"], "owner-history.json", { type: "application/json" });
    Object.defineProperty(file, "text", { value: async () => "{}" });
    fireEvent.change(screen.getByLabelText("Observation file"), { target: { files: [file] } });

    expect(await screen.findByText(readyReport.message)).toBeVisible();
    const enabledButton = screen.getByRole("button", { name: "Import 1 ready rows" });
    expect(enabledButton).toBeEnabled();
    expect(preview).toHaveBeenCalledWith({ fileName: "owner-history.json", contents: "{}" });

    fireEvent.click(enabledButton);
    await waitFor(() => expect(commit).toHaveBeenCalledTimes(1));
    expect(await screen.findByText(/Imported 1 sleep observation/)).toBeVisible();
    expect(onImported).toHaveBeenCalledTimes(1);
  });

  it("paginates large reports instead of rendering every row at once", async () => {
    const rows = Array.from({ length: 101 }, (_, index) => ({
      ...readyReport.rows[0],
      rowNumber: index + 1,
      observationId: `obs_owner_${String(index + 1).padStart(3, "0")}`,
      sourceRecordId: `fitbit-owner-${String(index + 1).padStart(3, "0")}`,
    }));
    const preview = vi.fn(async () => ({
      ...readyReport,
      totalRows: rows.length,
      readyRows: rows.length,
      rows,
    }));
    (globalThis as { go?: unknown }).go = {
      main: { App: { PreviewSleepImport: preview, ImportSleepData: vi.fn() } },
    };
    render(<SleepImportPanel onImported={vi.fn(async () => undefined)} />);

    const file = new File(["{}"], "owner-history.json", { type: "application/json" });
    Object.defineProperty(file, "text", { value: async () => "{}" });
    fireEvent.change(screen.getByLabelText("Observation file"), { target: { files: [file] } });

    fireEvent.click(await screen.findByText("Review all 101 row results"));
    expect(screen.getByText("Rows 1-100 of 101")).toBeVisible();
    expect(screen.queryByText("fitbit-owner-101")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next rows" }));
    expect(await screen.findByText("Rows 101-101 of 101")).toBeVisible();
    expect(screen.getByText("fitbit-owner-101")).toBeVisible();
  });
});
