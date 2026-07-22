import { describe, expect, it, vi } from "vitest";

import {
  addRhythmMarker,
  deleteRhythmMarker,
  exportRhythmMarkers,
  loadRhythmMarkers,
  normalizeRhythmMarkers,
  rhythmMarkerDeleteConfirmation,
  type RhythmMarkerInput,
} from "./rhythmMarkers";

const marker = {
  markerId: "marker_test_01",
  kind: "travel",
  kindLabel: "Travel / time-zone context",
  startAt: "2026-07-22T13:00:00Z",
  endAt: "2026-07-22T15:00:00Z",
  zoneId: "America/New_York",
  civilDate: "2026-07-22",
  hour: 9,
  startLabel: "Jul 22, 2026, 9:00 AM",
  endLabel: "Jul 22, 2026, 11:00 AM",
  rangeLabel: "Jul 22, 2026, 9:00 AM to Jul 22, 2026, 11:00 AM",
  note: "Private travel context",
  recordedLabel: "Jul 22, 2026, 12:00 PM",
};

const response = {
  status: "ready",
  empty: false,
  message: "1 self-reported context marker. It does not establish cause.",
  markers: [marker],
  fixtureMode: false,
  updatedLabel: "Updated Jul 22, 12:00 PM",
};

function rootWith(methods: Record<string, (input?: unknown) => Promise<unknown>>) {
  return { go: { main: { App: methods } } };
}

describe("rhythm marker data boundary", () => {
  it("normalizes only coherent exact responses", () => {
    expect(normalizeRhythmMarkers(response)?.markers[0]).toMatchObject({
      markerId: "marker_test_01",
      kind: "travel",
      note: "Private travel context",
    });
    expect(normalizeRhythmMarkers({ ...response, unexpected: true })).toBeUndefined();
    expect(
      normalizeRhythmMarkers({
        ...response,
        markers: [{ ...marker, kindLabel: "Travel" }],
      }),
    ).toBeUndefined();
    expect(
      normalizeRhythmMarkers({
        ...response,
        markers: [{ ...marker, endAt: marker.startAt }],
      }),
    ).toBeUndefined();
    expect(normalizeRhythmMarkers({ ...response, markers: [marker, marker] })).toBeUndefined();
  });

  it("uses an honest unavailable state when Wails is absent", async () => {
    await expect(loadRhythmMarkers({})).resolves.toMatchObject({
      status: "unavailable",
      empty: true,
      markers: [],
    });
  });

  it("passes append and typed erase inputs through the desktop bridge", async () => {
    const add = vi.fn(async () => response);
    const erase = vi.fn(async () => ({
      ...response,
      status: "empty",
      empty: true,
      markers: [],
    }));
    const input: RhythmMarkerInput = {
      kind: "travel",
      startLocal: "2026-07-22T09:00",
      endLocal: "2026-07-22T11:00",
      zoneId: "America/New_York",
      note: "Private travel context",
    };
    const root = rootWith({ AddRhythmMarker: add, DeleteRhythmMarker: erase });

    await expect(addRhythmMarker(input, root)).resolves.toMatchObject({ status: "ready" });
    await expect(
      deleteRhythmMarker(marker.markerId, rhythmMarkerDeleteConfirmation, root),
    ).resolves.toMatchObject({ status: "empty" });
    expect(add).toHaveBeenCalledWith(input);
    expect(erase).toHaveBeenCalledWith({
      markerId: marker.markerId,
      confirmation: "DELETE",
    });
  });

  it("validates the embedded v1 export and declared count", async () => {
    const payload = {
      schema_version: "v1",
      generated_at: "2026-07-22T16:00:00Z",
      markers: [
        {
          marker_id: "marker_test_01",
          kind: "travel",
          start_at: "2026-07-22T13:00:00Z",
          end_at: "2026-07-22T15:00:00Z",
          zone_id: "America/New_York",
          note: "Private travel context",
          provenance: {
            acquisition_method: "manual",
            evidence_status: "user_reported",
            recorded_at: "2026-07-22T16:00:00Z",
          },
        },
      ],
    };
    const value = {
      fileName: "zeitboard-rhythm-context-v1-20260722.json",
      json: `${JSON.stringify(payload)}\n`,
      generatedAt: payload.generated_at,
      generatedLabel: "Jul 22, 2026, 12:00 PM",
      markerCount: 1,
    };
    await expect(
      exportRhythmMarkers(rootWith({ ExportRhythmMarkers: async () => value })),
    ).resolves.toEqual(value);
    await expect(
      exportRhythmMarkers(
        rootWith({ ExportRhythmMarkers: async () => ({ ...value, markerCount: 2 }) }),
      ),
    ).rejects.toThrow("declared count");
    const overShared = structuredClone(payload);
    Object.assign(overShared.markers[0]?.provenance ?? {}, { source_record_id: "remote" });
    await expect(
      exportRhythmMarkers(
        rootWith({
          ExportRhythmMarkers: async () => ({ ...value, json: JSON.stringify(overShared) }),
        }),
      ),
    ).rejects.toThrow("v1 contract");
  });
});
