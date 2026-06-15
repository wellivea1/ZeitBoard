import { describe, expect, it } from "vitest";

import { loadOverview } from "./backend";
import { overviewFixture } from "./fixture";

describe("loadOverview", () => {
  it("uses a valid Wails overview when available", async () => {
    const backendOverview = { ...overviewFixture, state: "Resting" };
    const result = await loadOverview({
      go: { main: { App: { GetOverview: async () => backendOverview } } },
    });

    expect(result).toEqual({ data: backendOverview, source: "backend" });
  });

  it("normalizes the desktop application's OverviewDTO", async () => {
    const result = await loadOverview({
      go: {
        main: {
          App: {
            GetOverview: async () => ({
              currentEstimatedState: "Likely awake",
              timeSinceWake: "6 hours 20 minutes",
              predictedNextSleepWindow: "Mon Jun 15, 3:10 PM to Mon Jun 15, 5:40 PM",
              driftEstimate: "+42 minutes per observed sleep cycle",
              confidence: "medium",
              confidenceReasons: ["Nine recent sleep episodes"],
              nextUsefulTaskWindow: "Mon Jun 15, 11:00 AM to Mon Jun 15, 1:45 PM",
              sharingStatus: "Static trusted-view prototype only; no public endpoint",
              fixtureMode: true,
            }),
          },
        },
      },
    });

    expect(result.source).toBe("backend");
    expect(result.data.state).toBe("Likely awake");
    expect(result.data.confidence).toEqual({
      level: "Medium",
      reason: "Nine recent sleep episodes",
    });
    expect(result.data.fixtureMode).toBe(true);
  });

  it("falls back when the Wails binding is unavailable", async () => {
    await expect(loadOverview({})).resolves.toEqual({
      data: overviewFixture,
      source: "fixture",
    });
  });

  it("falls back when the backend rejects", async () => {
    const result = await loadOverview({
      go: {
        service: {
          AppService: {
            GetOverview: async () => Promise.reject(new Error("unavailable")),
          },
        },
      },
    });

    expect(result.source).toBe("fixture");
  });
});
