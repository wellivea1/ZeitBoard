import { describe, expect, it } from "vitest";

import {
  decideBackendProposal,
  loadBackendProposalPage,
  loadBackendProposals,
  normalizeBackendProposalPage,
  normalizeBackendProposals,
} from "./backendProposals";

const backendList = {
  status: "ok",
  proposals: [
    {
      proposalId: "proposal_abc123def456",
      action: "propose_place_task",
      status: "pending",
      title: 'Place task "taxes"',
      window: "Thu Mar 12, 10:00 AM to 11:30 AM EDT",
      confidence: "medium",
      reasonLabels: ["In a predicted waking window"],
      answer: "Queued for approval.",
      createdLabel: "Proposed Mar 12, 6:00 AM",
      expiresLabel: "expires Mar 12, 6:15 AM",
      decisionToken: "one-use-token",
    },
  ],
  pagination: { nextCursor: "cursor-older-01", hasMore: true },
};

describe("backend proposals", () => {
  it("normalizes the synced proposal list", () => {
    const data = normalizeBackendProposals(backendList);
    expect(data?.status).toBe("ok");
    expect(data?.proposals[0]?.confidence).toBe("Medium");
    expect(data?.proposals[0]?.decisionToken).toBe("one-use-token");
    expect(data?.pagination).toEqual({ nextCursor: "cursor-older-01", hasMore: true });
  });

  it("defaults legacy lists to a terminal page and validates cursor invariants", () => {
    expect(
      normalizeBackendProposals({ status: "ok", proposals: backendList.proposals })?.pagination,
    ).toEqual({ nextCursor: "", hasMore: false });
    expect(
      normalizeBackendProposalPage({
        ...backendList,
        pagination: { nextCursor: "", hasMore: true },
      }),
    ).toBeUndefined();
    expect(
      normalizeBackendProposalPage({
        ...backendList,
        pagination: { nextCursor: "cursor-without-more", hasMore: false },
      }),
    ).toBeUndefined();
    expect(normalizeBackendProposalPage({ status: "ok", proposals: [] })).toBeUndefined();
  });

  it("rejects an off-enum status or malformed proposal", () => {
    expect(normalizeBackendProposals({ status: "weird", proposals: [] })).toBeUndefined();
    expect(
      normalizeBackendProposals({
        status: "ok",
        proposals: [{ proposalId: "p", status: "applied" }],
      }),
    ).toBeUndefined();
  });

  it("is absent (off) when the Wails bridge is missing", async () => {
    await expect(loadBackendProposals({})).resolves.toEqual({
      status: "off",
      proposals: [],
      pagination: { nextCursor: "", hasMore: false },
    });
  });

  it("loads both page methods and decides with the one-use token", async () => {
    const decisions: unknown[] = [];
    const pageRequests: unknown[] = [];
    const root = {
      go: {
        main: {
          App: {
            GetBackendProposals: async () => backendList,
            GetBackendProposalPage: async (input: unknown) => {
              pageRequests.push(input);
              return {
                status: "ok",
                proposals: [],
                pagination: { nextCursor: "", hasMore: false },
              };
            },
            DecideBackendProposal: async (input: unknown) => {
              decisions.push(input);
              return {
                status: "ok",
                proposals: [{ ...backendList.proposals[0], status: "approved", decisionToken: "" }],
              };
            },
          },
        },
      },
    };
    const loaded = await loadBackendProposals(root);
    expect(loaded.status).toBe("ok");
    expect(loaded.proposals).toHaveLength(1);

    const older = await loadBackendProposalPage("cursor-older-01", root);
    expect(pageRequests).toEqual([{ cursor: "cursor-older-01" }]);
    expect(older.status).toBe("ok");
    expect(older.pagination).toEqual({ nextCursor: "", hasMore: false });

    const refreshed = await decideBackendProposal(
      { proposalId: "proposal_abc123def456", decision: "approved", token: "one-use-token" },
      root,
    );
    expect(decisions).toEqual([
      { proposalId: "proposal_abc123def456", decision: "approved", token: "one-use-token" },
    ]);
    expect(refreshed.proposals[0]?.status).toBe("approved");
    expect(refreshed.proposals[0]?.decisionToken).toBeUndefined();
  });

  it("reports an error state when the bridge rejects", async () => {
    const root = {
      go: { main: { App: { GetBackendProposals: async () => Promise.reject(new Error("boom")) } } },
    };
    const result = await loadBackendProposals(root);
    expect(result.status).toBe("error");
    expect(result.proposals).toEqual([]);
    expect(result.pagination).toEqual({ nextCursor: "", hasMore: false });
  });
});
