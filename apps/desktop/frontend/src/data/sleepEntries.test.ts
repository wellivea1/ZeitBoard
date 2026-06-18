import { describe, expect, it } from "vitest";

import { addSleepEntry, loadSleepEntries, normalizeSleepEntries } from "./sleepEntries";

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
      addSleepEntry({
        startLocal: "2026-03-01T22:00",
        endLocal: "2026-03-02T06:00",
        zoneId: "America/New_York",
        classification: "principal",
      }, root),
    ).resolves.toMatchObject({ observationId: "obs_sleep_01" });
  });
});
