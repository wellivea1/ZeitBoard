import { describe, expect, it } from "vitest";

import { loadProposals, normalizeProposals, proposalsFixture } from "./proposals";

const backendProposals = {
  fixtureMode: true,
  status: "estimated",
  proposals: [
    {
      id: "proposal-task-email",
      origin: "scheduler",
      kind: "Place",
      title: "Email the clinic",
      to: "Mon Jun 15, 9:30 AM to 10:00 AM",
      rhythmContext: "at the start of a predicted waking window",
      confidence: "High",
      explanationCodes: ["within_predicted_waking_window", "avoids_fixed_event"],
      reasonLabels: ["In a predicted waking window", "Avoids a fixed event"],
      createdLabel: "Proposed by Scheduler",
      expiresLabel: "valid for the current estimate",
    },
  ],
  unplaced: [
    {
      title: "Call accountant before noon",
      reason: "No open window fits before its limits",
      reasonCode: "no_available_interval",
      nextAction: "Keep manual until the next estimate refresh.",
    },
  ],
};

describe("loadProposals", () => {
  it("normalizes the scheduler's proposals and unplaced list", async () => {
    const result = await loadProposals({
      go: { main: { App: { GetProposals: async () => backendProposals } } },
    });

    expect(result.source).toBe("backend");
    expect(result.data.proposals).toHaveLength(1);
    expect(result.data.proposals[0]?.kind).toBe("Place");
    expect(result.data.proposals[0]?.confidence).toBe("High");
    expect(result.data.proposals[0]?.explanationCodes).toContain("avoids_fixed_event");
    expect(result.data.unplaced[0]?.reasonCode).toBe("no_available_interval");
  });

  it("falls back when the Wails binding is unavailable", async () => {
    await expect(loadProposals({})).resolves.toEqual({
      data: proposalsFixture,
      source: "fixture",
    });
  });

  it("falls back when the backend rejects", async () => {
    const result = await loadProposals({
      go: { svc: { App: { GetProposals: async () => Promise.reject(new Error("nope")) } } },
    });
    expect(result.source).toBe("fixture");
  });

  it("rejects a proposal with no explanation codes (contract requires at least one)", () => {
    const broken = {
      ...backendProposals,
      proposals: [{ ...backendProposals.proposals[0], explanationCodes: [] }],
    };
    expect(normalizeProposals(broken)).toBeUndefined();
  });

  it("rejects an off-enum origin", () => {
    const broken = {
      ...backendProposals,
      proposals: [{ ...backendProposals.proposals[0], origin: "hacker" }],
    };
    expect(normalizeProposals(broken)).toBeUndefined();
  });

  it("accepts a valid empty plan", () => {
    expect(normalizeProposals({ fixtureMode: true, status: "estimated", proposals: [], unplaced: [] })).toEqual({
      fixtureMode: true,
      status: "estimated",
      proposals: [],
      unplaced: [],
    });
  });

  it("accepts estimate-unavailable unplaced tasks from the local app", () => {
    expect(
      normalizeProposals({
        fixtureMode: false,
        status: "empty",
        refusal: { code: "estimate_unavailable", message: "Add sleep entries." },
        proposals: [],
        unplaced: [
          {
            title: "Email the clinic",
            reason: "No current estimate to plan against",
            reasonCode: "estimate_unavailable",
            nextAction: "Add at least seven principal sleep entries before planning.",
          },
        ],
      }),
    ).toMatchObject({
      fixtureMode: false,
      status: "empty",
      refusal: { code: "estimate_unavailable" },
      proposals: [],
      unplaced: [{ reasonCode: "estimate_unavailable" }],
    });
  });
});
