import { describe, expect, it } from "vitest";

import {
  loadSourceStatuses,
  normalizeSourceStatuses,
  sourceNote,
  sourceStatusWording,
  type SourceStatus,
} from "./sourceStatus";

// Thursday 4:10 PM local.
const now = new Date(2026, 8, 24, 16, 10);
const hoursAgo = (hours: number) => new Date(now.getTime() - hours * 3_600_000).toISOString();

const reading: SourceStatus = {
  source: "health_connect",
  outcome: "read",
  reportedAt: hoursAgo(2),
  since: hoursAgo(50),
  readThrough: hoursAgo(2),
  backgroundReads: true,
  held: 0,
  // Tuesday 7:40 AM.
  newestRecordEnd: new Date(2026, 8, 22, 7, 40).toISOString(),
};

describe("what the phone found", () => {
  it("reads the desktop's statuses and refuses anything else", () => {
    expect(normalizeSourceStatuses([reading])).toEqual([reading]);
    expect(normalizeSourceStatuses([])).toEqual([]);
    for (const bad of [
      [{ ...reading, outcome: "maybe" }],
      [{ ...reading, source: "fitbit_cloud" }],
      [{ ...reading, reportedAt: "yesterday" }],
      [{ ...reading, held: -1 }],
      [{ ...reading, backgroundReads: "yes" }],
      { statuses: [] },
    ]) {
      expect(normalizeSourceStatuses(bad)).toBeUndefined();
    }
  });

  it("finds no reports outside the desktop app", async () => {
    await expect(loadSourceStatuses({})).resolves.toEqual([]);
    const root = { go: { main: { App: { GetSourceStatuses: async () => [reading] } } } };
    await expect(loadSourceStatuses(root)).resolves.toEqual([reading]);
  });

  it("words a reading phone, what it holds back, and when it reads", () => {
    expect(sourceStatusWording(reading, now)).toEqual({
      name: "Health Connect on your phone",
      label: "Reading",
      state: "available",
      detail: "Read today at 2:10 PM. Its newest night ends on Tue, Sep 22 at 7:40 AM.",
    });
    const held = sourceStatusWording({ ...reading, held: 2, backgroundReads: false }, now);
    expect(held.detail).toBe(
      "Read today at 2:10 PM. Its newest night ends on Tue, Sep 22 at 7:40 AM. 2 nights are held on the phone: their time zone differs from home. It reads only while ZeitBoard is open on the phone.",
    );
    const empty = sourceStatusWording({ ...reading, newestRecordEnd: undefined }, now);
    expect(empty.detail).toBe("Read today at 2:10 PM. It holds no sleep from the last 30 days.");
  });

  it("says when the phone has stopped reading, and why it may have", () => {
    const quiet = { ...reading, reportedAt: hoursAgo(30) };
    expect(sourceStatusWording(quiet, now)).toMatchObject({
      label: "Not reading",
      state: "off",
      detail: "Last read yesterday at 10:10 AM. The phone may be off, or ZeitBoard closed on it.",
    });
    expect(sourceStatusWording({ ...quiet, backgroundReads: false }, now).detail).toBe(
      "Last read yesterday at 10:10 AM. It reads only while ZeitBoard is open on the phone.",
    );
  });

  it("names each outage from when it began", () => {
    const since = hoursAgo(30);
    expect(
      sourceStatusWording({ ...reading, outcome: "permission_missing", since }, now),
    ).toMatchObject({
      label: "Permission off",
      state: "error",
      detail:
        "The phone has not been allowed to read sleep since yesterday at 10:10 AM. Allow it in ZeitBoard on the phone.",
    });
    expect(sourceStatusWording({ ...reading, outcome: "unavailable", since }, now)).toMatchObject({
      label: "Unavailable",
      detail:
        "Health Connect has not been available on the phone since yesterday at 10:10 AM. It may need installing or updating.",
    });
    expect(sourceStatusWording({ ...reading, outcome: "read_failed" }, now)).toMatchObject({
      label: "Read failed",
      detail: "The last read, today at 2:10 PM, failed. The phone tries again within the hour.",
    });
  });

  it("gives the likely cause of a stale estimate", () => {
    expect(sourceNote([], now)).toBeUndefined();
    expect(sourceNote([reading], now)).toBe(
      "The phone read Health Connect today at 2:10 PM and found no sleep after Tue, Sep 22 at 7:40 AM.",
    );
    expect(sourceNote([{ ...reading, held: 1 }], now)).toContain(
      "1 night is held on the phone: its time zone differs from home.",
    );
    expect(
      sourceNote([{ ...reading, reportedAt: hoursAgo(30), backgroundReads: false }], now),
    ).toBe(
      "The phone last read Health Connect yesterday at 10:10 AM. It reads only while ZeitBoard is open on the phone.",
    );
    expect(
      sourceNote([{ ...reading, outcome: "permission_missing", since: hoursAgo(30) }], now),
    ).toBe("The phone has not been allowed to read Health Connect since yesterday at 10:10 AM.");
  });
});
