import { describe, expect, it } from "vitest";

import { loadOverview } from "./backend";
import { overviewFixture } from "./fixture";

describe("loadOverview", () => {
  it("uses a valid Wails overview when available", async () => {
    const backendOverview = { ...overviewFixture, state: "Resting" };
    const result = await loadOverview({
      go: { main: { App: { GetOverview: async () => backendOverview } } },
    });

    expect(result).toEqual({ data: backendOverview, source: "local" });
  });

  it("normalizes the desktop application's OverviewDTO", async () => {
    const result = await loadOverview({
      go: {
        main: {
          App: {
            GetOverview: async () => ({
              status: "estimated",
              empty: false,
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

    expect(result.source).toBe("local");
    expect(result.data.state).toBe("Likely awake");
    expect(result.data.confidence).toEqual({
      level: "Medium",
      reason: "Nine recent sleep episodes",
    });
    expect(result.data.fixtureMode).toBe(true);
  });

  it("marks synced server estimates only when the desktop DTO says so", async () => {
    const result = await loadOverview({
      go: {
        main: {
          App: {
            GetOverview: async () => ({
              estimateSource: "synced",
              status: "estimated",
              empty: false,
              currentEstimatedState: "Likely awake from server",
              timeSinceWake: "5 hours",
              predictedNextSleepWindow: "Tonight",
              driftEstimate: "+45 minutes per observed sleep cycle",
              confidence: "medium",
              confidenceReasons: ["Server estimate"],
              nextUsefulTaskWindow: "Later today",
              sharingStatus: "Server projection only",
              fixtureMode: false,
              updatedLabel: "Synced - server estimate just now",
            }),
          },
        },
      },
    });

    expect(result.source).toBe("synced");
    expect(result.data.stateDetail).toBe("Synced server estimate from the enrolled backend");
  });

  it("normalizes an empty local overview without falling back to fixture data", async () => {
    const result = await loadOverview({
      go: {
        main: {
          App: {
            GetOverview: async () => ({
              status: "empty",
              empty: true,
              refusal: {
                code: "estimate_unavailable",
                message: "Add your first sleep entry.",
              },
              currentEstimatedState: "No sleep entries yet",
              timeSinceWake: "Not available",
              predictedNextSleepWindow: "Not enough local data",
              driftEstimate: "Not enough local data",
              confidence: "low",
              confidenceReasons: ["Add your first sleep entry."],
              nextUsefulTaskWindow: "No reliable proposal",
              sharingStatus: "No active trusted view; local data only",
              fixtureMode: false,
              updatedLabel: "Waiting for local sleep entries",
            }),
          },
        },
      },
    });

    expect(result.source).toBe("local");
    expect(result.data.status).toBe("empty");
    expect(result.data.empty).toBe(true);
    expect(result.data.fixtureMode).toBe(false);
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
