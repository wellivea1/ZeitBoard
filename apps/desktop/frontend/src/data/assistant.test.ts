import { describe, expect, it } from "vitest";

import {
  loadAssistantStatus,
  normalizeAssistantReply,
  normalizeAssistantStatus,
  sendAssistantMessage,
} from "./assistant";

const proposal = {
  proposalId: "proposal_srv_01",
  action: "propose_place_task",
  status: "pending",
  title: "Place task “Call clinic”",
  window: "Thu Jul 10, 11:00 AM to 11:45 AM EDT",
  confidence: "Medium",
  reasonLabels: ["Fits the predicted waking window"],
  createdLabel: "Proposed Jul 10, 8:00 AM",
  expiresLabel: "expires Jul 10, 8:15 AM",
  decisionToken: "one-use-token",
};

describe("assistant adapter", () => {
  it("normalizes status and reply shapes", () => {
    expect(
      normalizeAssistantStatus({ enabled: true, configured: true, provider: "anthropic" }),
    ).toMatchObject({ enabled: true, configured: true, provider: "anthropic" });
    const reply = normalizeAssistantReply({
      available: true,
      result: "proposal_pending",
      answer: "Found a window.",
      configured: true,
      provider: "anthropic",
      proposals: [proposal],
    });
    expect(reply?.result).toBe("proposal_pending");
    expect(reply?.proposals[0]?.decisionToken).toBe("one-use-token");
    expect(normalizeAssistantReply({ available: true, result: "??", proposals: [] })?.result).toBe(
      "unknown",
    );
    expect(normalizeAssistantReply({ notAReply: true })).toBeUndefined();
  });

  it("is honestly unavailable without the Wails bridge", async () => {
    const status = await loadAssistantStatus({});
    expect(status.enabled).toBe(false);
    expect(status.message).toContain("desktop app");
    const reply = await sendAssistantMessage("hello", {});
    expect(reply.available).toBe(false);
    expect(reply.result).toBe("unavailable");
  });

  it("passes messages through the Wails method and reads the reply", async () => {
    let sent: unknown;
    const root = {
      go: {
        main: {
          App: {
            SendAssistantMessage: async (input?: unknown) => {
              sent = input;
              return {
                available: true,
                result: "refused_medical",
                answer: "I can't help with medical decisions like medication or dosing.",
                configured: false,
                proposals: [],
              };
            },
          },
        },
      },
    };
    const reply = await sendAssistantMessage("when should I take melatonin?", root);
    expect(sent).toEqual({ message: "when should I take melatonin?" });
    expect(reply.result).toBe("refused_medical");
    expect(reply.proposals).toEqual([]);
  });
});
