import { describe, expect, it } from "vitest";

import { loadRhythm, normalizeRhythm, rhythmFixture } from "./rhythm";

// A backend-shaped projection (flat, as estimation.RhythmProjection serializes).
const backendProjection = {
  fixtureMode: false,
  status: "estimated",
  actogramSummary: "Double-plotted actogram derived from the local estimate.",
  observedRows: [
    {
      id: "sleep-1",
      day: "Jan 12",
      civilDate: "2026-01-12",
      zoneId: "America/New_York",
      startHour: 21.5,
      durationHours: 8,
      kind: "observed",
      startLabel: "Jan 12, 9:30 PM",
      wakeLabel: "Jan 13, 5:30 AM",
      durationLabel: "8 hr 0 min",
      source: "Manual sleep log",
      confidence: "High",
    },
  ],
  forecastRows: [
    {
      id: "forecast-1",
      day: "Jan 13",
      civilDate: "2026-01-13",
      zoneId: "America/New_York",
      startHour: 22.5,
      durationHours: 3.2,
      kind: "forecast",
      startLabel: "Jan 13, 10:30 PM earliest",
      wakeLabel: "Jan 14, 1:42 AM latest",
      durationLabel: "3 hr 12 min window",
      source: "Forecast cycle 1",
      confidence: "Medium",
    },
  ],
  now: {
    label: "now",
    day: "Jan 12",
    civilDate: "2026-01-12",
    zoneId: "America/New_York",
    hour: 13.5,
  },
  driftTitle: "Sleep-onset drift",
  slopeLabel: "+60 min per cycle",
  driftConfidence: "Medium",
  driftSummary: "Sleep onset drifts later by about 60 minutes per observed sleep cycle.",
  yMinHour: 20,
  yMaxHour: 26,
  driftPoints: [
    {
      id: "drift-1",
      day: "Jan 12",
      civilDate: "2026-01-12",
      zoneId: "America/New_York",
      onsetHour: 21.5,
      fitHour: 21.4,
      bandLowHour: 20.9,
      bandHighHour: 21.9,
      onsetLabel: "9:30 PM",
      source: "Manual sleep log",
      confidence: "High",
    },
  ],
};

describe("loadRhythm", () => {
  it("normalizes the backend projection into nested actogram/drift data", async () => {
    const result = await loadRhythm({
      go: { main: { App: { GetRhythm: async () => backendProjection } } },
    });

    expect(result.source).toBe("local");
    expect(result.data.actogram.observedRows).toHaveLength(1);
    expect(result.data.actogram.observedRows[0]?.kind).toBe("observed");
    expect(result.data.actogram.observedRows[0]?.civilDate).toBe("2026-01-12");
    expect(result.data.actogram.observedRows[0]?.zoneId).toBe("America/New_York");
    expect(result.data.actogram.forecastRows[0]?.kind).toBe("forecast");
    expect(result.data.actogram.now.hour).toBe(13.5);
    expect(result.data.drift.slopeLabel).toBe("+60 min per cycle");
    expect(result.data.drift.confidence).toBe("Medium");
    expect(result.data.drift.yMaxHour).toBeGreaterThan(result.data.drift.yMinHour);
  });

  it("marks synced server rhythm only when estimateSource is synced", async () => {
    const result = await loadRhythm({
      go: {
        main: {
          App: { GetRhythm: async () => ({ ...backendProjection, estimateSource: "synced" }) },
        },
      },
    });

    expect(result.source).toBe("synced");
    expect(result.data.actogram.summary).toBe(backendProjection.actogramSummary);
  });

  it("keeps a fixture projection labeled as sample data", async () => {
    const result = await loadRhythm({
      go: {
        main: {
          App: { GetRhythm: async () => ({ ...backendProjection, fixtureMode: true }) },
        },
      },
    });

    expect(result.source).toBe("fixture");
  });

  it("falls back when the Wails binding is unavailable", async () => {
    await expect(loadRhythm({})).resolves.toEqual({
      data: rhythmFixture,
      source: "fixture",
    });
  });

  it("falls back when the backend rejects", async () => {
    const result = await loadRhythm({
      go: { service: { AppService: { GetRhythm: async () => Promise.reject(new Error("nope")) } } },
    });
    expect(result.source).toBe("fixture");
  });

  it("accepts an empty local projection without chart rows", () => {
    expect(
      normalizeRhythm({
        fixtureMode: false,
        status: "empty",
        refusal: { code: "estimate_unavailable", message: "Add sleep entries." },
        actogramSummary: "Add sleep entries.",
        observedRows: [],
        forecastRows: [],
        now: {
          label: "now",
          day: "Jan 12",
          civilDate: "2026-01-12",
          zoneId: "America/New_York",
          hour: 13.5,
        },
        driftTitle: "Sleep-onset drift",
        slopeLabel: "Not enough data",
        driftConfidence: "Low",
        driftSummary: "Add sleep entries.",
        yMinHour: 0,
        yMaxHour: 24,
        driftPoints: [],
      }),
    ).toMatchObject({
      fixtureMode: false,
      status: "empty",
      refusal: { code: "estimate_unavailable" },
      actogram: { observedRows: [] },
      drift: { points: [] },
    });
  });

  it("rejects a malformed projection (missing drift points)", () => {
    const broken = { ...backendProjection, driftPoints: [] };
    expect(normalizeRhythm(broken)).toBeUndefined();
  });

  it("requires structured civil date and zone data for local estimates", () => {
    const rowsWithoutDate = backendProjection.observedRows.map((row) =>
      Object.fromEntries(Object.entries(row).filter(([key]) => key !== "civilDate")),
    );
    const rowsWithoutZone = backendProjection.observedRows.map((row) =>
      Object.fromEntries(Object.entries(row).filter(([key]) => key !== "zoneId")),
    );
    expect(
      normalizeRhythm({ ...backendProjection, observedRows: rowsWithoutDate }),
    ).toBeUndefined();
    expect(
      normalizeRhythm({ ...backendProjection, observedRows: rowsWithoutZone }),
    ).toBeUndefined();
  });

  it("accepts civil dates while preserving zone redaction for synced estimates", () => {
    const withoutZone = (row: Record<string, unknown>) =>
      Object.fromEntries(Object.entries(row).filter(([key]) => key !== "zoneId"));
    const synced = {
      ...backendProjection,
      estimateSource: "synced",
      observedRows: backendProjection.observedRows.map(withoutZone),
      forecastRows: backendProjection.forecastRows.map(withoutZone),
      driftPoints: backendProjection.driftPoints.map(withoutZone),
      now: withoutZone(backendProjection.now),
    };
    const normalized = normalizeRhythm(synced);
    expect(normalized?.actogram.observedRows[0]?.civilDate).toBe("2026-01-12");
    expect(normalized?.actogram.observedRows[0]?.zoneId).toBeUndefined();
  });

  it("rejects a collapsed y-range", () => {
    const broken = { ...backendProjection, yMinHour: 24, yMaxHour: 24 };
    expect(normalizeRhythm(broken)).toBeUndefined();
  });
});
