import { describe, expect, it } from "vitest";
import { loadOutlook, normalizeOutlook, outlookUnavailable } from "./outlook";

function available(overrides: Record<string, unknown> = {}) {
  return {
    status: "available",
    freshness: { state: "current", explanation: "Based on recent records.", trusted: true },
    horizonLabel: "Next 72 hours",
    horizonHours: 72,
    days: [{ label: "Thu, Aug 6", offsetHours: 6 }],
    segments: [
      {
        presence: "awake",
        observed: false,
        rangeLabel: "6:00 PM to 11:20 PM",
        dayLabel: "Wed",
        durationLabel: "5 hours 20 minutes",
        offsetHours: 0,
        durationHours: 5.33,
      },
    ],
    officeHoursLabel: "Typical office hours, Monday to Friday 9:00 AM to 5:00 PM",
    officeWindows: [
      {
        dayLabel: "Thu, Aug 6",
        hoursLabel: "9:00 AM to 5:00 PM",
        status: "reachable",
        reachableLabel: "9:00 AM to 12:40 PM",
        detail: "Predicted awake for 3 hours 40 minutes of this window.",
        offsetHours: 15,
        durationHours: 8,
      },
    ],
    commitments: [],
    opportunities: [],
    awakeLabel: "30 hours 0 minutes",
    uncertainLabel: "9 hours 0 minutes",
    disclaimer: "This application does not provide medical advice.",
    ...overrides,
  };
}

describe("normalizeOutlook", () => {
  it("accepts a complete view", () => {
    const view = normalizeOutlook(available());
    expect(view?.status).toBe("available");
    expect(view?.segments).toHaveLength(1);
    expect(view?.officeWindows[0]?.status).toBe("reachable");
    expect(view?.days[0]?.offsetHours).toBe(6);
  });

  // A presence the UI has no colour for would render as a blank stretch of
  // timeline, which reads as "nothing is happening" rather than as an error.
  it("rejects a presence outside the closed set", () => {
    const view = normalizeOutlook(
      available({
        segments: [
          {
            presence: "probably-awake-ish",
            observed: false,
            rangeLabel: "x",
            dayLabel: "Wed",
            durationLabel: "y",
            offsetHours: 0,
            durationHours: 1,
          },
        ],
      }),
    );
    expect(view).toBeUndefined();
  });

  it("rejects an office status outside the closed set", () => {
    const view = normalizeOutlook(
      available({
        officeWindows: [
          {
            dayLabel: "Thu",
            hoursLabel: "9 to 5",
            status: "maybe",
            detail: "d",
            offsetHours: 1,
            durationHours: 8,
          },
        ],
      }),
    );
    expect(view).toBeUndefined();
  });

  it("keeps a withheld view's reason", () => {
    const view = normalizeOutlook(
      available({
        status: "withheld",
        segments: [],
        officeWindows: [],
        withheldMessage: "Sleep was expected by now and none has been recorded.",
        freshness: {
          state: "withheld",
          reason: "expected_sleep_unrecorded",
          explanation: "Sleep was expected by now and none has been recorded.",
          trusted: false,
        },
      }),
    );
    expect(view?.status).toBe("withheld");
    expect(view?.withheldMessage).toContain("expected");
    expect(view?.freshness.reason).toBe("expected_sleep_unrecorded");
  });

  it("keeps a refusal's own code", () => {
    const view = normalizeOutlook(
      available({
        status: "refused",
        segments: [],
        officeWindows: [],
        refusal: { code: "insufficient_data", message: "need at least 7 episodes" },
      }),
    );
    expect(view?.refusal?.code).toBe("insufficient_data");
  });

  it("rejects a view with no status", () => {
    expect(normalizeOutlook({ ...available(), status: undefined })).toBeUndefined();
  });
});

describe("loadOutlook", () => {
  it("returns the read-only preview when the desktop bridge is absent", async () => {
    await expect(loadOutlook({})).resolves.toEqual(outlookUnavailable);
  });

  it("returns the preview rather than a half-built view when the payload is wrong", async () => {
    const root = { go: { main: { App: { GetOutlook: async () => ({ status: "available" }) } } } };
    await expect(loadOutlook(root)).resolves.toEqual(outlookUnavailable);
  });

  it("reads a valid payload from the bridge", async () => {
    const root = { go: { main: { App: { GetOutlook: async () => available() } } } };
    const view = await loadOutlook(root);
    expect(view.status).toBe("available");
    expect(view.officeWindows[0]?.reachableLabel).toBe("9:00 AM to 12:40 PM");
  });
});
