import { describe, expect, it, vi } from "vitest";

import {
  hasNativeSleepImport,
  importNativeSleepData,
  importSleepData,
  normalizeSleepImportReport,
  previewNativeSleepImport,
  previewSleepImport,
  transcriptionTemplateCSV,
} from "./sleepImport";

const preview = {
  fileName: "owner-history.json",
  format: "json",
  dryRun: true,
  totalRows: 2,
  readyRows: 1,
  duplicateRows: 0,
  invalidRows: 1,
  importedRows: 0,
  canImport: false,
  errors: [],
  message: "Fix 1 invalid row; no data will be written.",
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
    {
      rowNumber: 2,
      status: "invalid",
      errors: ["source_record_id is required"],
      statusDetail: "Conflicting identifier",
    },
  ],
};

describe("sleep import adapter", () => {
  it("normalizes a complete row-accounting report", () => {
    expect(normalizeSleepImportReport(preview)).toMatchObject({
      totalRows: 2,
      invalidRows: 1,
      rows: [{ sourceRecordId: "fitbit-owner-001" }, { status: "invalid" }],
    });
  });

  it("rejects reports whose counts do not account for every row", () => {
    expect(normalizeSleepImportReport({ ...preview, totalRows: 3 })).toBeUndefined();
    expect(
      normalizeSleepImportReport({ ...preview, readyRows: 0, duplicateRows: 1 }),
    ).toBeUndefined();
    expect(
      normalizeSleepImportReport({ ...preview, canImport: true, invalidRows: 1 }),
    ).toBeUndefined();
  });

  it("calls separate preview and commit bindings with the same file", async () => {
    const input = { fileName: "owner-history.json", contents: "{}" };
    const committed = {
      ...preview,
      dryRun: false,
      readyRows: 0,
      invalidRows: 0,
      importedRows: 1,
      totalRows: 2,
      duplicateRows: 1,
      canImport: false,
      message: "Imported 1 observation; 1 duplicate was already present.",
      rows: [
        { ...preview.rows[0], status: "imported", statusDetail: "Imported" },
        {
          ...preview.rows[1],
          status: "duplicate",
          errors: [],
          statusDetail: "Already imported",
        },
      ],
    };
    const previewMethod = vi.fn(async () => preview);
    const importMethod = vi.fn(async () => committed);
    const root = {
      go: { main: { App: { PreviewSleepImport: previewMethod, ImportSleepData: importMethod } } },
    };

    await expect(previewSleepImport(input, root)).resolves.toMatchObject({ dryRun: true });
    await expect(importSleepData(input, root)).resolves.toMatchObject({ importedRows: 1 });
    expect(previewMethod).toHaveBeenCalledWith(input);
    expect(importMethod).toHaveBeenCalledWith(input);
  });

  it("normalizes and commits a native tokenized selection", async () => {
    const nativePreview = vi.fn(async () => ({
      ...preview,
      importToken: "sleep_import_token",
      canceled: false,
    }));
    const nativeCommit = vi.fn(async () => ({
      ...preview,
      dryRun: false,
      readyRows: 0,
      invalidRows: 0,
      importedRows: 1,
      totalRows: 2,
      duplicateRows: 1,
      canImport: false,
      message: "Imported 1 observation; 1 duplicate was already present.",
      rows: [
        { ...preview.rows[0], status: "imported", statusDetail: "Imported" },
        { ...preview.rows[1], status: "duplicate", errors: [], statusDetail: "Already imported" },
      ],
    }));
    const root = {
      go: {
        main: {
          App: { PreviewSleepImportFile: nativePreview, ImportSleepDataFile: nativeCommit },
        },
      },
    };

    expect(hasNativeSleepImport(root)).toBe(true);
    await expect(previewNativeSleepImport(root)).resolves.toMatchObject({
      importToken: "sleep_import_token",
      canceled: false,
    });
    await expect(importNativeSleepData("sleep_import_token", root)).resolves.toMatchObject({
      importedRows: 1,
    });
    expect(nativeCommit).toHaveBeenCalledWith({ importToken: "sleep_import_token" });
  });

  it("ships a header-only owner transcription template", () => {
    expect(transcriptionTemplateCSV).toBe(
      "source_record_id,start_local,end_local,zone_id,classification,review_status\r\n",
    );
  });
});
