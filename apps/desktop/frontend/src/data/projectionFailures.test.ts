import { describe, expect, it } from "vitest";
import { loadOverview } from "./backend";
import { loadRhythm } from "./rhythm";
import { loadOutlook } from "./outlook";
import { loadProposals } from "./proposals";
import { outlookFixture, overviewFixture } from "./fixture";

describe("desktop preview isolation", () => {
  it.each([undefined, {}, { status: "estimated" }])(
    "withholds malformed desktop projections: %j",
    async (payload) => {
      const root = {
        go: {
          main: {
            App: {
              GetOverview: async () => payload,
              GetRhythm: async () => payload,
              GetOutlook: async () => payload,
            },
          },
        },
      };
      expect((await loadOverview(root)).data).toMatchObject({
        status: "unavailable",
        fixtureMode: false,
      });
      expect((await loadRhythm(root)).data).toMatchObject({
        status: "unavailable",
        fixtureMode: false,
      });
      expect(await loadOutlook(root, outlookFixture)).toMatchObject({
        status: "unavailable",
        segments: [],
        opportunities: [],
      });
    },
  );

  it("does not mistake missing methods in a desktop bridge for browser preview", async () => {
    const root = { go: {} };
    expect((await loadOverview(root)).source).toBe("local");
    expect((await loadRhythm(root)).source).toBe("local");
    expect((await loadOutlook(root, outlookFixture)).status).toBe("unavailable");
    await expect(loadProposals(root)).rejects.toThrow("unavailable");
  });

  it("normalizes absent freshness in a nested DTO instead of crashing Home", async () => {
    const root = {
      go: {
        main: {
          App: {
            GetOverview: async () => ({
              ...overviewFixture,
              fixtureMode: false,
              freshness: undefined,
            }),
          },
        },
      },
    };
    expect((await loadOverview(root)).data.freshness).toMatchObject({
      state: "withheld",
      trusted: false,
    });
  });

  it("cannot trust a withheld freshness verdict", async () => {
    const root = {
      go: {
        main: {
          App: {
            GetOverview: async () => ({
              ...overviewFixture,
              fixtureMode: false,
              freshness: { ...overviewFixture.freshness, state: "withheld", trusted: true },
            }),
          },
        },
      },
    };
    expect((await loadOverview(root)).data.freshness.trusted).toBe(false);
  });
});
