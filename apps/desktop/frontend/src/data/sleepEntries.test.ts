import { describe, expect, it } from "vitest";

import {
  addSleepEntry,
  deleteAllSleepData,
  deleteSleepObservation,
  exportSleepData,
  loadSleepEntriesBetween,
  loadSleepLogPage,
  loadSleepSources,
  normalizeSleepDataExport,
  normalizeSleepEntries,
  normalizeSleepSources,
  undoSleepCorrection,
} from "./sleepEntries";

const entry = {
  observationId: "obs_sleep_01",
  startLocal: "2026-03-01T22:00",
  endLocal: "2026-03-02T06:00",
  startLabel: "Sun Mar 1, 10:00 PM EST",
  endLabel: "Mon Mar 2, 6:00 AM EST",
  zoneId: "America/New_York",
  classification: "principal",
  effectiveStartLocal: "2026-03-01T22:30",
  effectiveEndLocal: "2026-03-02T06:00",
  effectiveStartLabel: "Sun Mar 1, 10:30 PM EST",
  effectiveEndLabel: "Mon Mar 2, 6:00 AM EST",
  effectiveClassification: "principal",
  reviewToken: "synthetic-review-token",
  needsReview: false,
  sourceWindowLabel: "Synthetic source window",
  activeEdits: [],
  durationLabel: "7 hours 30 minutes",
  suppressed: false,
  sourceLabel: "Manual sleep log",
  provenanceLabel: "manual / user reported",
  history: [
    {
      correctionId: "corr_sleep_01",
      createdLabel: "Mar 2, 6:15 AM",
      reason: "user edit",
      summary: "start Mar 1, 10:30 PM",
    },
  ],
};

describe("sleep entry adapter", () => {
  it("normalizes sleep entries from the desktop service", () => {
    expect(
      normalizeSleepEntries({
        status: "ready",
        empty: false,
        message: "1 local sleep entry stored on this device.",
        entries: [entry],
      }),
    ).toMatchObject({
      status: "ready",
      empty: false,
      entries: [{ observationId: "obs_sleep_01", history: [{ correctionId: "corr_sleep_01" }] }],
    });
  });

  it("rejects invalid classifications", () => {
    expect(
      normalizeSleepEntries({
        status: "ready",
        empty: false,
        message: "bad",
        entries: [{ ...entry, classification: "invalid" }],
      }),
    ).toBeUndefined();
  });

  it("normalizes contract-shaped sleep exports", () => {
    expect(
      normalizeSleepDataExport({
        fileName: "zeitboard-sleep-export-20260302-060000.json",
        json: '{"schema_version":"v1","observation_set":{"observations":[]}}',
        generatedLabel: "Mar 2, 2026, 6:00 AM",
        observationCount: 1,
        correctionCount: 2,
      }),
    ).toMatchObject({
      fileName: "zeitboard-sleep-export-20260302-060000.json",
      observationCount: 1,
      correctionCount: 2,
    });
    expect(
      normalizeSleepDataExport({
        fileName: "bad.json",
        json: "{}",
        generatedLabel: "now",
        observationCount: -1,
        correctionCount: 0,
      }),
    ).toBeUndefined();
  });

  it("reads a page, a range, and adds through Wails methods", async () => {
    let pageInput: unknown;
    let rangeInput: unknown;
    const root = {
      go: {
        main: {
          App: {
            GetSleepLogPage: async (input: unknown) => {
              pageInput = input;
              return {
                status: "ready",
                empty: false,
                message: "51 local sleep entries stored on this device.",
                entries: [entry],
                total: 51,
                page: 1,
                pageSize: 50,
              };
            },
            GetSleepEntriesBetween: async (input: unknown) => {
              rangeInput = input;
              return {
                status: "ready",
                empty: false,
                message: "1 night in these days.",
                entries: [entry],
              };
            },
            AddSleepEntry: async () => entry,
          },
        },
      },
    };

    await expect(loadSleepLogPage(1, root)).resolves.toMatchObject({
      entries: [entry],
      total: 51,
      page: 1,
      pageSize: 50,
    });
    expect(pageInput).toEqual({ page: 1 });
    await expect(
      loadSleepEntriesBetween("2026-03-01T00:00:00.000Z", "2026-03-09T00:00:00.000Z", root),
    ).resolves.toMatchObject({ entries: [entry] });
    expect(rangeInput).toEqual({
      startAt: "2026-03-01T00:00:00.000Z",
      endAt: "2026-03-09T00:00:00.000Z",
    });
    await expect(
      addSleepEntry(
        {
          startLocal: "2026-03-01T22:00",
          endLocal: "2026-03-02T06:00",
          zoneId: "America/New_York",
          classification: "principal",
        },
        root,
      ),
    ).resolves.toMatchObject({ observationId: "obs_sleep_01" });
  });

  it("undoes a night's last edit through the desktop", async () => {
    let undoInput: unknown;
    const root = {
      go: {
        main: {
          App: {
            UndoSleepCorrection: async (input: unknown) => {
              undoInput = input;
              return { ...entry, canUndo: true };
            },
          },
        },
      },
    };

    await expect(
      undoSleepCorrection("obs_sleep_01", "synthetic-review-token", root),
    ).resolves.toMatchObject({ observationId: "obs_sleep_01", canUndo: true });
    expect(undoInput).toEqual({
      observationId: "obs_sleep_01",
      reviewToken: "synthetic-review-token",
    });
    // A night the desktop says nothing about has nothing to undo.
    const unsaid = normalizeSleepEntries({
      status: "ready",
      empty: false,
      message: "1 local sleep entry stored on this device.",
      entries: [entry],
    });
    expect(unsaid?.entries[0]?.canUndo).toBe(false);
    await expect(undoSleepCorrection("obs_sleep_01", "token", {})).rejects.toThrow(
      "Undoing a sleep edit is unavailable.",
    );
  });

  it("exports and deletes through Wails methods", async () => {
    let singleDeleteInput: unknown;
    let deleteAllInput: unknown;
    const root = {
      go: {
        main: {
          App: {
            ExportSleepData: async () => ({
              fileName: "zeitboard-sleep-export-20260302-060000.json",
              json: '{"schema_version":"v1","observation_set":{"observations":[]}}',
              generatedLabel: "Mar 2, 2026, 6:00 AM",
              observationCount: 1,
              correctionCount: 1,
            }),
            DeleteSleepObservation: async (input: unknown) => {
              singleDeleteInput = input;
            },
            DeleteAllSleepData: async (input: unknown) => {
              deleteAllInput = input;
            },
          },
        },
      },
    };

    await expect(exportSleepData(root)).resolves.toMatchObject({
      observationCount: 1,
      correctionCount: 1,
    });
    // Deleting returns nothing: each view re-reads what it shows.
    await expect(deleteSleepObservation("obs_sleep_01", "DELETE", root)).resolves.toBeUndefined();
    await expect(deleteAllSleepData("DELETE", root)).resolves.toBeUndefined();
    expect(singleDeleteInput).toEqual({ observationId: "obs_sleep_01", confirmation: "DELETE" });
    expect(deleteAllInput).toEqual({ confirmation: "DELETE" });
  });
});

describe("source summaries", () => {
  const summary = {
    status: "ready",
    message: "2 local sleep entries stored on this device.",
    total: 2,
    correctedCount: 1,
    suppressedCount: 0,
    sources: [
      {
        source: "Imported sleep",
        provenance: "file import / directly observed",
        total: 1,
        corrected: 0,
        suppressed: 0,
      },
      {
        source: "Manual sleep log",
        provenance: "manual / user reported",
        total: 1,
        corrected: 1,
        suppressed: 0,
      },
    ],
    latestCorrected: entry,
  };

  it("reads the counts the desktop made, with the newest corrected night", () => {
    expect(normalizeSleepSources(summary)).toMatchObject({
      total: 2,
      correctedCount: 1,
      sources: summary.sources,
      latestCorrected: { observationId: "obs_sleep_01" },
    });
    const uncorrected = { ...summary, correctedCount: 0, latestCorrected: undefined };
    expect(normalizeSleepSources(uncorrected)?.latestCorrected).toBeUndefined();
  });

  it("refuses counts it cannot show truthfully", () => {
    expect(normalizeSleepSources({ ...summary, total: -1 })).toBeUndefined();
    expect(
      normalizeSleepSources({ ...summary, sources: [{ source: "Imported sleep" }] }),
    ).toBeUndefined();
    expect(
      normalizeSleepSources({ ...summary, latestCorrected: { observationId: "x" } }),
    ).toBeUndefined();
  });

  it("is unavailable, not empty, outside the desktop app", async () => {
    await expect(loadSleepSources({})).resolves.toMatchObject({
      status: "unavailable",
      sources: [],
    });
  });
});
