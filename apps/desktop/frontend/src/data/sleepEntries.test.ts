import { describe, expect, it } from "vitest";

import {
  addSleepEntry,
  deleteAllSleepData,
  deleteSleepObservation,
  exportSleepData,
  latestCorrectedEntry,
  loadSleepEntries,
  normalizeSleepDataExport,
  normalizeSleepEntries,
  summarizeSleepSources,
  type SleepEntry,
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
        entries: [{ ...entry, classification: "unknown" }],
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

  it("loads and adds through Wails methods", async () => {
    const root = {
      go: {
        main: {
          App: {
            ListSleepEntries: async () => ({
              status: "ready",
              empty: false,
              message: "1 local sleep entry stored on this device.",
              entries: [entry],
            }),
            AddSleepEntry: async () => entry,
          },
        },
      },
    };

    await expect(loadSleepEntries(root)).resolves.toMatchObject({ entries: [entry] });
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

  it("exports and deletes through Wails methods", async () => {
    const deleted = {
      status: "empty",
      empty: true,
      message: "No sleep entries yet.",
      entries: [],
    };
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
              return deleted;
            },
            DeleteAllSleepData: async (input: unknown) => {
              deleteAllInput = input;
              return deleted;
            },
          },
        },
      },
    };

    await expect(exportSleepData(root)).resolves.toMatchObject({
      observationCount: 1,
      correctionCount: 1,
    });
    await expect(deleteSleepObservation("obs_sleep_01", "DELETE", root)).resolves.toMatchObject({
      empty: true,
      entries: [],
    });
    await expect(deleteAllSleepData("DELETE", root)).resolves.toMatchObject({
      empty: true,
      entries: [],
    });
    expect(singleDeleteInput).toEqual({ observationId: "obs_sleep_01", confirmation: "DELETE" });
    expect(deleteAllInput).toEqual({ confirmation: "DELETE" });
  });
});

describe("source summaries", () => {
  const typed = entry as SleepEntry;
  const uncorrected: SleepEntry = {
    ...typed,
    observationId: "obs_sleep_02",
    sourceLabel: "Manual sleep log",
    history: [],
  };
  const suppressedWearable: SleepEntry = {
    ...typed,
    observationId: "obs_sleep_03",
    sourceLabel: "Wearable import",
    suppressed: true,
    history: [],
  };

  it("summarizes real per-source composition with corrected and suppressed counts", () => {
    const summary = summarizeSleepSources([typed, uncorrected, suppressedWearable]);
    expect(summary).toEqual([
      {
        source: "Manual sleep log",
        provenance: "manual / user reported",
        total: 2,
        corrected: 1,
        suppressed: 0,
      },
      {
        source: "Wearable import",
        provenance: "manual / user reported",
        total: 1,
        corrected: 0,
        suppressed: 1,
      },
    ]);
    expect(summarizeSleepSources([])).toEqual([]);
  });

  it("finds the newest corrected entry (log is newest-first)", () => {
    expect(latestCorrectedEntry([uncorrected, typed])?.observationId).toBe("obs_sleep_01");
    expect(latestCorrectedEntry([uncorrected, suppressedWearable])).toBeUndefined();
  });
});
