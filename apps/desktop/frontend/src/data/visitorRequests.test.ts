import { describe, expect, it } from "vitest";
import {
  decideVisitorRequest,
  defaultSlot,
  loadVisitorRequests,
  normalizeVisitorRequest,
  normalizeVisitorRequests,
  type VisitorRequest,
} from "./visitorRequests";

const raw = {
  proposalId: "visitor-1",
  linkLabel: "Mum",
  handle: "Sam",
  message: "coffee?",
  windowLabel: "Tue, Aug 4, 10:00 AM to 2:00 PM",
  durationLabel: "45 minutes",
  windowStartLocal: "2026-08-04T10:00",
  windowEndLocal: "2026-08-04T14:00",
  durationMinutes: 45,
  beyondHorizon: false,
  createdLabel: "Asked Aug 3, 9:00 AM",
  expiresLabel: "expires Aug 10, 9:00 AM",
  approvalDisclosure: "Approving tells them the exact time you pick.",
  decisionToken: "token",
};

describe("visitor request normalization", () => {
  it("keeps the owner-facing fields", () => {
    const request = normalizeVisitorRequest(raw);
    expect(request?.handle).toBe("Sam");
    expect(request?.message).toBe("coffee?");
    expect(request?.approvalDisclosure).toContain("exact time");
    expect(request?.decisionToken).toBe("token");
  });

  it("rejects a record missing the approval disclosure", () => {
    // The disclosure states what approving reveals, so a card without it must
    // not render at all rather than render silently incomplete.
    const withoutDisclosure: Record<string, unknown> = { ...raw };
    delete withoutDisclosure.approvalDisclosure;
    expect(normalizeVisitorRequest(withoutDisclosure)).toBeUndefined();
  });

  it("rejects records missing the picker bounds", () => {
    const withoutStart: Record<string, unknown> = { ...raw };
    delete withoutStart.windowStartLocal;
    expect(normalizeVisitorRequest(withoutStart)).toBeUndefined();
  });

  it("rejects the whole payload when one request is malformed", () => {
    expect(
      normalizeVisitorRequests({ status: "ok", requests: [raw, { proposalId: "broken" }] }),
    ).toBeUndefined();
  });

  it("accepts an empty list and unknown statuses are refused", () => {
    expect(normalizeVisitorRequests({ status: "ok", requests: [] })).toEqual({
      status: "ok",
      requests: [],
    });
    expect(normalizeVisitorRequests({ status: "weird", requests: [] })).toBeUndefined();
  });
});

describe("defaultSlot", () => {
  it("offers the requested length from the window start", () => {
    const request = normalizeVisitorRequest(raw) as VisitorRequest;
    expect(defaultSlot(request)).toEqual({
      start: "2026-08-04T10:00",
      end: "2026-08-04T10:45",
    });
  });

  it("offers the whole window when no length was requested", () => {
    const request = normalizeVisitorRequest({
      ...raw,
      durationMinutes: 0,
      durationLabel: undefined,
    }) as VisitorRequest;
    expect(defaultSlot(request)).toEqual({
      start: "2026-08-04T10:00",
      end: "2026-08-04T14:00",
    });
  });
});

describe("bridge behaviour", () => {
  it("is absent rather than broken when the binding does not exist", async () => {
    await expect(loadVisitorRequests({})).resolves.toEqual({ status: "off", requests: [] });
  });

  it("reports an error without inventing an empty request list state", async () => {
    const root = {
      go: {
        main: {
          App: {
            GetBackendVisitorRequests: async () => {
              throw new Error("unreachable");
            },
          },
        },
      },
    };
    await expect(loadVisitorRequests(root)).resolves.toMatchObject({
      status: "error",
      requests: [],
    });
  });

  it("sends the chosen block only when approving", async () => {
    const sent: unknown[] = [];
    const root = {
      go: {
        main: {
          App: {
            DecideBackendVisitorRequest: async (input: unknown) => {
              sent.push(input);
              return { status: "ok", requests: [] };
            },
          },
        },
      },
    };

    await decideVisitorRequest(
      {
        proposalId: "visitor-1",
        decision: "approved",
        token: "token",
        startLocal: "2026-08-04T11:00",
        endLocal: "2026-08-04T11:45",
      },
      root,
    );
    await decideVisitorRequest(
      { proposalId: "visitor-1", decision: "rejected", token: "token" },
      root,
    );

    expect(sent[0]).toMatchObject({
      decision: "approved",
      startLocal: "2026-08-04T11:00",
      endLocal: "2026-08-04T11:45",
    });
    // Declining carries no block: there is nothing to reveal.
    expect(sent[1]).toMatchObject({ decision: "rejected", startLocal: "", endLocal: "" });
  });
});
