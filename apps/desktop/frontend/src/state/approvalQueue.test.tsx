import { StrictMode, useState, type ReactNode } from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { proposalsFixture } from "../data/proposals";
import { ApprovalsProvider } from "./approvals";
import { BackendProposalsProvider } from "./backendProposals";
import { VisitorRequestsProvider, useVisitorRequests } from "./visitorRequests";
import { AgentProposalsProvider } from "./agentProposals";
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
  messages: [],
  canMessage: false,
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
          <AgentProposalsProvider>
            <ApprovalQueueProvider>{children}</ApprovalQueueProvider>
          </AgentProposalsProvider>
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
    GetAgentProposals: vi.fn(async () => ({ pending: [], history: [], nextExpiryAt: "" })),
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
  it("shows a request's thread and sends the owner's reply to it", async () => {
    const visitorLine = {
      author: "visitor",
      authorLabel: "They wrote",
      body: "Is Tuesday possible?",
      createdLabel: "Oct 31, 12:30 PM",
    };
    const withThread = { ...request, canMessage: true, messages: [visitorLine] };
    const reply = vi.fn(async (input: { proposalId: string; message: string }) => ({
      ...visitorPage,
      requests: [
        {
          ...withThread,
          messages: [
            visitorLine,
            {
              author: "owner",
              authorLabel: "You wrote",
              body: input.message,
              createdLabel: "Oct 31, 1:00 PM",
            },
          ],
        },
      ],
    }));
    const erase = vi.fn(async () => ({
      ...visitorPage,
      requests: [{ ...withThread, messages: [] }],
    }));
    install({
      GetBackendVisitorRequests: vi.fn(async () => ({ ...visitorPage, requests: [withThread] })),
      ReplyToBackendVisitorRequest: reply,
      EraseBackendVisitorThread: erase,
    });
    render(
      <Providers>
        <DecisionQueue />
      </Providers>,
    );

    expect(await screen.findByText("Is Tuesday possible?")).toBeVisible();
    const box = screen.getByLabelText("Reply to Sam");
    const send = screen.getByRole("button", { name: "Send" });
    expect(send).toBeDisabled();
    fireEvent.change(box, { target: { value: "Tuesday at 3 works." } });
    await act(async () => fireEvent.click(send));
    expect(reply).toHaveBeenCalledWith({ proposalId: "request-1", message: "Tuesday at 3 works." });
    expect(await screen.findByText("Tuesday at 3 works.")).toBeVisible();
    expect(screen.getByLabelText("Reply to Sam")).toHaveValue("");

    fireEvent.click(screen.getByRole("button", { name: "Delete the conversation with Sam" }));
    const confirm = screen.getByRole("button", { name: "Delete conversation" });
    expect(confirm).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Type DELETE to confirm"), {
      target: { value: "DELETE" },
    });
    await act(async () => fireEvent.click(confirm));
    expect(erase).toHaveBeenCalledWith({ proposalId: "request-1", message: "" });
    expect(screen.queryByText("Is Tuesday possible?")).toBeNull();
  });

  // One list, no filters: every origin is visible at once, and the count covers
  // pages that have not been loaded yet.
  // Suggested times are planned leaving room for each other, so several are
  // reviewed and decided in one step (ADR-0052).
  it("reviews suggested times together and decides the chosen ones in one step", async () => {
    let decided = new Set<string>();
    const decideTogether = vi.fn(
      async ({ proposalIds, decision }: { proposalIds: string[]; decision: string }) => {
        decided = new Set([...decided, ...proposalIds]);
        return {
          decisions: proposalIds.map((proposalId) => ({
            proposalId,
            decision,
            message: "Recorded.",
          })),
          message: "Recorded together.",
        };
      },
    );
    install({
      // The owner's own suggestions, not the sample: decisions reach the desktop.
      GetProposals: async () => ({
        ...proposalsFixture,
        fixtureMode: false,
        proposals: proposalsFixture.proposals.map((proposal) =>
          decided.has(proposal.id)
            ? { ...proposal, decision: "approved", canUndo: true }
            : proposal,
        ),
      }),
      DecideLocalProposals: decideTogether,
    });
    render(
      <Providers>
        <Probe />
        <DecisionQueue />
      </Providers>,
    );
    expect(await screen.findByText("13 ready")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Review 2 suggestions together" }));
    expect(screen.getByRole("heading", { name: "Review together" })).toBeVisible();
    // Leave one out to decide alone.
    fireEvent.click(screen.getByRole("checkbox", { name: /Taxes focus block/ }));
    fireEvent.click(screen.getByRole("button", { name: "Accept 1 suggested time" }));
    expect(await screen.findByText("12 ready")).toBeVisible();
    expect(decideTogether).toHaveBeenCalledWith({
      proposalIds: ["proposal-email-okafor"],
      decision: "approved",
    });
    // The left-out suggestion is back as a card of its own.
    expect(screen.queryByRole("heading", { name: "Review together" })).toBeNull();
    expect(screen.getByText("Taxes focus block")).toBeVisible();
  });

  it("decides nothing in a reviewed batch when the plan changed first", async () => {
    install({
      GetProposals: async () => ({ ...proposalsFixture, fixtureMode: false }),
      DecideLocalProposals: vi.fn(async () => {
        throw "proposal inputs changed; refresh proposals";
      }),
    });
    render(
      <Providers>
        <Probe />
        <DecisionQueue />
      </Providers>,
    );
    expect(await screen.findByText("13 ready")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Review 2 suggestions together" }));
    fireEvent.click(screen.getByRole("button", { name: "Decline 2 suggested times" }));
    expect(
      await screen.findByText(
        "Something changed since you reviewed these, so none was decided. Review them again.",
      ),
    ).toBeVisible();
    expect(screen.getByText("13 ready")).toBeVisible();
  });
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
  // What an agent asked for is one more decision: counted, shown with the
  // others, and made real only when the owner accepts it (ADR-0051).
  it("counts an assistant's dose and task and makes them only when accepted", async () => {
    const base = { state: "pending", createdAt: "2099-01-01T21:12:00Z", expiresAt: expiry };
    const dose = {
      ...base,
      proposalId: "agent_proposal_dose",
      title: "Record dose",
      dose: {
        medicationId: "med_synthetic",
        medicationLabel: "Synthetic evening tablet",
        status: "taken",
        doseAt: "2099-01-01T21:10:00Z",
        zoneId: "UTC",
      },
    };
    const task = {
      ...base,
      proposalId: "agent_proposal_task",
      title: "Add task",
      task: { title: "Call the pharmacy", durationMinutes: 15 },
    };
    let queue: Record<string, unknown> = {
      pending: [dose, task],
      history: [],
      nextExpiryAt: expiry,
    };
    const decide = vi.fn(
      async ({ proposalId, decision }: { proposalId: string; decision: string }) => {
        const decided = proposalId === dose.proposalId ? dose : task;
        queue = {
          pending: (queue.pending as (typeof dose | typeof task)[]).filter(
            (item) => item.proposalId !== proposalId,
          ),
          history: [
            { ...decided, state: decision, decidedAt: "2099-01-01T21:20:00Z" },
            ...(queue.history as unknown[]),
          ],
          nextExpiryAt: expiry,
        };
        return queue;
      },
    );
    const medicationsChanged = vi.fn();
    const tasksChanged = vi.fn();
    window.addEventListener("zeitboard:medication-data-changed", medicationsChanged);
    window.addEventListener("zeitboard:sleep-data-changed", tasksChanged);
    install({ GetAgentProposals: vi.fn(async () => queue), DecideAgentProposal: decide });
    render(
      <Providers>
        <Probe />
        <DecisionQueue />
        <DecisionHistory />
      </Providers>,
    );
    expect(await screen.findByText("15 ready")).toBeVisible();
    expect(screen.getByText("Synthetic evening tablet")).toBeVisible();
    expect(screen.getByText("Call the pharmacy")).toBeVisible();
    fireEvent.click(
      screen.getByRole("button", {
        name: "Accept and record the dose of Synthetic evening tablet",
      }),
    );
    expect(await screen.findByText("14 ready")).toBeVisible();
    expect(decide).toHaveBeenLastCalledWith({
      proposalId: "agent_proposal_dose",
      decision: "approved",
    });
    expect(medicationsChanged).toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Decline the task “Call the pharmacy”" }));
    expect(await screen.findByText("13 ready")).toBeVisible();
    expect(decide).toHaveBeenLastCalledWith({
      proposalId: "agent_proposal_task",
      decision: "rejected",
    });
    // Declining adds no task, so nothing is refreshed for it.
    expect(tasksChanged).not.toHaveBeenCalled();
    expect(screen.getByText("Record dose · Synthetic evening tablet")).toBeInTheDocument();
    expect(screen.getByText("Add task · Call the pharmacy")).toBeInTheDocument();
    window.removeEventListener("zeitboard:medication-data-changed", medicationsChanged);
    window.removeEventListener("zeitboard:sleep-data-changed", tasksChanged);
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
    expect(screen.getByRole("button", { name: /^Accept the chosen time for/ })).toBeDisabled();
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
    const accept = await screen.findByRole("button", { name: /^Accept the chosen time for/ });
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
    expect(screen.queryByRole("button", { name: /^Accept the chosen time for/ })).toBeNull();
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
    expect(screen.getByRole("button", { name: /^Accept the chosen time for/ })).toBeEnabled();
    await act(async () => {
      vi.advanceTimersByTime(1001);
    });
    expect(list).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole("button", { name: /^Accept the chosen time for/ })).toBeNull();
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
