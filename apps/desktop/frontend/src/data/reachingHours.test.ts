import { describe, expect, it } from "vitest";
import {
  loadReachingHours,
  normalizeReachingHours,
  reachingHoursUnavailable,
  saveReachingHours,
} from "./reachingHours";

function payload(overrides: Record<string, unknown> = {}) {
  return {
    state: {
      enabled: true,
      label: "The clinic",
      startLocal: "09:00",
      endLocal: "13:00",
      days: [2, 4],
      zoneId: "America/New_York",
    },
    revision: 3,
    conflict: false,
    summary: "The clinic you set: Tuesday and Thursday, 9:00 AM to 1:00 PM.",
    ...overrides,
  };
}

function rootWith(name: string, method: (input?: unknown) => Promise<unknown>) {
  return { go: { main: { App: { [name]: method } } } };
}

describe("normalizeReachingHours", () => {
  it("accepts a stored schedule", () => {
    const parsed = normalizeReachingHours(payload());
    expect(parsed?.state.label).toBe("The clinic");
    expect(parsed?.state.days).toEqual([2, 4]);
    expect(parsed?.revision).toBe(3);
  });

  // Showing reaching windows nobody asked for is the failure this replaced, so
  // an absent flag has to read as off rather than on.
  it("treats a missing enabled flag as off", () => {
    const parsed = normalizeReachingHours(
      payload({
        state: { label: "x", startLocal: "09:00", endLocal: "17:00", days: [1], zoneId: "UTC" },
      }),
    );
    expect(parsed?.state.enabled).toBe(false);
  });

  it("drops weekday numbers that are not weekdays", () => {
    const parsed = normalizeReachingHours(
      payload({
        state: {
          enabled: true,
          startLocal: "09:00",
          endLocal: "17:00",
          days: [1, 9, -2, 6, "tuesday", 1],
          zoneId: "UTC",
        },
      }),
    );
    expect(parsed?.state.days).toEqual([1, 6]);
  });

  it("refuses a payload with no clock times", () => {
    expect(
      normalizeReachingHours(payload({ state: { enabled: true, days: [1], zoneId: "UTC" } })),
    ).toBeUndefined();
  });

  it("refuses a payload with no summary to show", () => {
    expect(normalizeReachingHours(payload({ summary: undefined }))).toBeUndefined();
  });

  it("carries a conflict through so a lost edit is not reported as saved", () => {
    const parsed = normalizeReachingHours(payload({ conflict: true }));
    expect(parsed?.conflict).toBe(true);
  });
});

describe("loadReachingHours", () => {
  it("reads the stored schedule", async () => {
    const envelope = await loadReachingHours(rootWith("GetReachingHours", async () => payload()));
    expect(envelope.state.label).toBe("The clinic");
  });

  it("falls back outside the desktop app", async () => {
    expect(await loadReachingHours({})).toEqual(reachingHoursUnavailable);
  });

  // The fallback must not read as a schedule the person chose.
  it("says the fallback is not a real schedule", () => {
    expect(reachingHoursUnavailable.summary).toMatch(/desktop app/i);
  });
});

describe("saveReachingHours", () => {
  it("sends the schedule and the base revision", async () => {
    const sent: unknown[] = [];
    const root = rootWith("SaveReachingHours", async (input) => {
      sent.push(input);
      return payload({ revision: 4 });
    });
    const saved = await saveReachingHours(
      {
        enabled: true,
        label: "The clinic",
        startLocal: "09:00",
        endLocal: "13:00",
        days: [2, 4],
        zoneId: "America/New_York",
      },
      3,
      root,
    );
    expect(saved.revision).toBe(4);
    expect(sent[0]).toMatchObject({ baseRevision: 3, state: { label: "The clinic" } });
  });

  it("fails rather than pretending a save landed", async () => {
    await expect(saveReachingHours(reachingHoursUnavailable.state, 0, {})).rejects.toThrow(
      /desktop app/,
    );
  });

  it("fails when the answer cannot be trusted", async () => {
    const root = rootWith("SaveReachingHours", async () => ({ revision: 4 }));
    await expect(saveReachingHours(reachingHoursUnavailable.state, 0, root)).rejects.toThrow();
  });
});
