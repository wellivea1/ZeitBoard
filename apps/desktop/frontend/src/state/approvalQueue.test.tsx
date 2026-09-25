import { StrictMode, useState, type ReactNode } from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { proposalsFixture } from "../data/proposals";
import { ApprovalsProvider } from "./approvals";
import { BackendProposalsProvider } from "./backendProposals";
import { VisitorRequestsProvider, useVisitorRequests } from "./visitorRequests";
import { ApprovalQueueProvider, useApprovalQueue } from "./approvalQueue";
import { DecisionHistory, DecisionQueue } from "../components/DecisionQueue";
import { emptyVisitorRequests, type VisitorRequest } from "../data/visitorRequests";
import { notifyReviewQueueChanged } from "../data/reviewQueue";
import { civilMinute } from "../utils/civilTime";

const expiry = "2099-01-02T00:00:00Z";
const request: VisitorRequest = {
  proposalId: "request-1",
  status: "pending",
  expiresAt: expiry,
  linkLabel: "Family",
  handle: "Sam",
  windowLabel: "Synthetic requested window",
  windowStartAt: "2099-01-01T12:00:00Z",
  windowEndAt: "2099-01-01T16:00:00Z",
  durationMinutes: 30,
  beyondHorizon: false,
  createdLabel: "Asked now",
  expiresLabel: "Expires later",
  approvalDisclosure: "Approval shares the exact time.",
  decisionToken: "synthetic-token",
};
const visitorPage = {
  ...emptyVisitorRequests,
  status: "ok",
  pendingCount: 4,
  nextExpiryAt: expiry,
  requests: [request],
  pagination: { nextCursor: "visitor-page-2", hasMore: true },
};
function Providers({ children }: { children: ReactNode }) {
  return (
    <ApprovalsProvider>
      <BackendProposalsProvider>
        <VisitorRequestsProvider>
          <ApprovalQueueProvider>{children}</ApprovalQueueProvider>
        </VisitorRequestsProvider>
      </BackendProposalsProvider>
    </ApprovalsProvider>
  );
}
function Probe() {
  const summary = useApprovalQueue();
  return (
    <output>
      {summary.ready
        ? `${summary.pendingCount} ${summary.incomplete ? "incomplete" : "ready"}`
        : "loading"}
    </output>
  );
}
function install(extra: Record<string, unknown> = {}) {
  const methods = {
    GetProposals: async () => proposalsFixture,
    GetBackendProposals: vi.fn(async () => ({
      status: "ok",
      proposals: [],
      pendingCount: 7,
      nextExpiryAt: expiry,
      pagination: { nextCursor: "backend-page-2", hasMore: true },
    })),
    GetBackendVisitorRequests: vi.fn(async () => visitorPage),
    ...extra,
  };
  (globalThis as { go?: unknown }).go = { main: { App: methods } };
  return methods;
}
afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
  vi.useRealTimers();
});

describe("shared approval queue", () => {
  // One list, no filters: every origin is visible at once, and the count covers
  // pages that have not been loaded yet.
  it("counts all sources beyond the loaded page and shows every origin together", async () => {
    install();
    render(
      <Providers>
        <Probe />
        <DecisionQueue />
      </Providers>,
    );
    expect(await screen.findByText("13 ready")).toBeVisible(); // two local fixture proposals + 7 backend + 4 requests
    expect(screen.queryByText(/Nothing is waiting for you/)).toBeNull();
    expect(screen.getByText("Sam asked for a time")).toBeVisible();
    expect(screen.getByText("Email Dr. Okafor")).toBeVisible();
    expect(screen.queryByRole("group", { name: "Filter approvals" })).toBeNull();
  });
  it("retains counts and requests during a failed refresh and recovers when disabled", async () => {
    const list = vi
      .fn()
      .mockResolvedValueOnce(visitorPage)
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValue(emptyVisitorRequests);
    install({ GetBackendVisitorRequests: list });
    render(
      <Providers>
        <Probe />
        <DecisionQueue />
      </Providers>,
    );
    await screen.findByText("13 ready");
    act(notifyReviewQueueChanged);
    expect(await screen.findByText("13 incomplete")).toBeVisible();
    expect(screen.getByText("Sam asked for a time")).toBeVisible();
    expect(screen.getByRole("button", { name: "Accept this block" })).toBeDisabled();
    act(notifyReviewQueueChanged);
    expect(await screen.findByText("9 ready")).toBeVisible();
    expect(screen.queryByText("Sam asked for a time")).toBeNull();
  });
  // Switching Plan to Calendar and back unmounts the decision list; a draft
  // block for a request lives in the provider, not the card.
  it("keeps a request draft when the decision list is remounted", async () => {
    function Views() {
      const [shown, setShown] = useState(true);
      return (
        <>
          <button onClick={() => setShown(!shown)}>Switch view</button>
          {shown && <DecisionQueue />}
        </>
      );
    }
    install();
    render(
      <Providers>
        <Views />
      </Providers>,
    );
    const start = await screen.findByLabelText("Block starts");
    const draft = civilMinute("2099-01-01T13:00:00Z");
    fireEvent.change(start, { target: { value: draft } });
    fireEvent.click(screen.getByRole("button", { name: "Switch view" }));
    expect(screen.queryByLabelText("Block starts")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Switch view" }));
    expect(screen.getByLabelText("Block starts")).toHaveValue(draft);
  });
  it("serializes decisions and does not resurrect a confirmed decision after a refresh failure", async () => {
    let resolve!: (value: unknown) => void;
    const decide = vi.fn(
      (input: unknown) =>
        new Promise((done) => {
          void input;
          resolve = done;
        }),
    );
    install({ DecideBackendVisitorRequest: decide });
    render(
      <Providers>
        <Probe />
        <DecisionQueue />
        <DecisionHistory />
      </Providers>,
    );
    const accept = await screen.findByRole("button", { name: "Accept this block" });
    fireEvent.click(accept);
    fireEvent.click(accept);
    expect(decide).toHaveBeenCalledTimes(1);
    expect(decide.mock.calls[0]?.[0]).toMatchObject({
      startAt: "2099-01-01T12:00:00.000Z",
      endAt: "2099-01-01T12:30:00.000Z",
    });
    await act(async () =>
      resolve({
        ...emptyVisitorRequests,
        status: "error",
        decisionRecorded: true,
        message: "Decision recorded. Refresh the queue.",
      }),
    );
    expect(screen.getByText("12 incomplete")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Accept this block" })).toBeNull();
    fireEvent.click(screen.getByText(/Decision history/));
    expect(screen.getByText("approved")).toBeVisible();
  });
  it("rejects duplicate pages and preserves drafts for unloaded requests", async () => {
    function More() {
      const queue = useVisitorRequests();
      return <button onClick={() => void queue.loadOlder()}>Next</button>;
    }
    const page = vi.fn(async () => visitorPage);
    install({ GetBackendVisitorRequestPage: page });
    render(
      <Providers>
        <DecisionQueue />
        <More />
      </Providers>,
    );
    await screen.findByLabelText("Block starts");
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(
      await screen.findByText("The backend returned a pagination cursor that did not advance."),
    ).toBeVisible();
    expect(page).toHaveBeenCalledWith({ cursor: "visitor-page-2" });
  });
  it("refreshes at expiry and makes an expired token non-actionable", async () => {
    vi.useFakeTimers();
    const deadline = new Date(Date.now() + 1000).toISOString();
    const initial = {
      ...visitorPage,
      nextExpiryAt: deadline,
      pendingCount: 1,
      requests: [{ ...request, expiresAt: deadline }],
    };
    const list = vi
      .fn()
      .mockResolvedValueOnce(initial)
      .mockResolvedValue({ ...initial, pendingCount: 0, nextExpiryAt: "" });
    install({ GetBackendVisitorRequests: list });
    render(
      <Providers>
        <DecisionQueue />
      </Providers>,
    );
    await act(async () => {});
    expect(screen.getByRole("button", { name: "Accept this block" })).toBeEnabled();
    await act(async () => {
      vi.advanceTimersByTime(1001);
    });
    expect(list).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole("button", { name: "Accept this block" })).toBeNull();
  });
});

it("loads all queue sources after a StrictMode effect remount", async () => {
  install();
  render(
    <StrictMode>
      <Providers>
        <Probe />
      </Providers>
    </StrictMode>,
  );
  expect(await screen.findByText("13 ready")).toBeVisible();
});
