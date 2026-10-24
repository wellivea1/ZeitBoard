import { describe, expect, it } from "vitest";

import {
  listAssistantModels,
  loadAssistantConnection,
  loadAssistantStatus,
  normalizeAssistantConnection,
  normalizeAssistantReply,
  normalizeAssistantStatus,
  sendAssistantMessage,
  usesChatGPTPlan,
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
  expiresAt: "2099-01-01T00:00:00Z",
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

  // ADR-0066: what pays for the assistant, and why it did not answer.
  it("reads the plan and the reasons the server gives, and nothing else", () => {
    const status = normalizeAssistantStatus({
      enabled: true,
      configured: false,
      provider: "chatgpt",
      auth: "chatgpt_plan",
      problem: "sign_in_ended",
    });
    expect(status).toMatchObject({ auth: "chatgpt_plan", problem: "sign_in_ended" });
    expect(usesChatGPTPlan(status)).toBe(true);
    expect(usesChatGPTPlan(normalizeAssistantStatus({ enabled: true, configured: true }))).toBe(
      false,
    );
    expect(
      normalizeAssistantStatus({ enabled: true, problem: "anything" })?.problem,
    ).toBeUndefined();

    const reply = normalizeAssistantReply({
      available: true,
      result: "answer_only",
      answer: "The plan's limit is reached.",
      configured: true,
      auth: "chatgpt_plan",
      reason: "usage_limit",
      proposals: [],
    });
    expect(reply).toMatchObject({ auth: "chatgpt_plan", reason: "usage_limit" });
    expect(
      normalizeAssistantReply({ available: true, reason: "made_up", proposals: [] })?.reason,
    ).toBeUndefined();
  });

  it("reads where the ChatGPT sign-in stands", async () => {
    expect(
      normalizeAssistantConnection({
        reachable: true,
        offered: true,
        state: "connected",
        email: "owner@example.com",
        model: "synthetic-fast",
        revoked: false,
      }),
    ).toEqual({
      reachable: true,
      offered: true,
      state: "connected",
      email: "owner@example.com",
      model: "synthetic-fast",
      revoked: false,
    });
    expect(normalizeAssistantConnection({ offered: true })).toBeUndefined();
    expect(normalizeAssistantConnection({ reachable: true, state: "odd" })?.state).toBeUndefined();
    // A window without the desktop app has no sign-in to read.
    expect(await loadAssistantConnection({})).toBeNull();
  });

  it("keeps only models that have a slug and a name", async () => {
    const root = {
      go: {
        main: {
          App: {
            ListAssistantModels: async () => [
              { slug: "synthetic-fast", displayName: "Synthetic Fast" },
              { slug: "", displayName: "No slug" },
              { slug: "synthetic-nameless" },
              "not a model",
            ],
          },
        },
      },
    };
    expect(await listAssistantModels(root)).toEqual([
      { slug: "synthetic-fast", displayName: "Synthetic Fast" },
    ]);
  });
});
