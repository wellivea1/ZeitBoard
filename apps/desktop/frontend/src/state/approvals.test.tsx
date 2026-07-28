import { Profiler } from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { proposalsFixture } from "../data/proposals";
import { ApprovalsProvider, useApprovals, usePendingApprovalsCount } from "./approvals";

function PendingCountProbe() {
  const pendingCount = usePendingApprovalsCount();
  return <span>{pendingCount} pending</span>;
}

function DecisionControl() {
  const { pending, decide } = useApprovals();
  return (
    <button
      type="button"
      disabled={pending.length === 0}
      onClick={() => decide(pending[0]!.id, "approved")}
    >
      Decide first proposal
    </button>
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

afterEach(() => {
  vi.useRealTimers();
  delete (globalThis as { go?: unknown }).go;
});

describe("ApprovalsProvider contexts", () => {
  it("does not rerender count-only consumers when unrelated toast state expires", () => {
    vi.useFakeTimers();
    const onRender = vi.fn();

    render(
      <ApprovalsProvider>
        <Profiler id="pending-count" onRender={onRender}>
          <PendingCountProbe />
        </Profiler>
        <DecisionControl />
      </ApprovalsProvider>,
    );

    expect(screen.getByText("2 pending")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Decide first proposal" }));
    expect(screen.getByText("1 pending")).toBeVisible();
    const rendersAfterDecision = onRender.mock.calls.length;

    act(() => vi.advanceTimersByTime(6_000));

    expect(onRender).toHaveBeenCalledTimes(rendersAfterDecision);
  });

  it("guards the local decision service synchronously against rapid clicks", async () => {
    const decision = deferred<unknown>();
    const pending = {
      ...proposalsFixture,
      fixtureMode: false,
      proposals: [{ ...proposalsFixture.proposals[0]!, decision: "pending", canUndo: false }],
      unplaced: [],
    };
    const decided = {
      ...pending,
      proposals: [{ ...pending.proposals[0]!, decision: "approved", canUndo: true }],
    };
    const getProposals = vi.fn().mockResolvedValueOnce(pending).mockResolvedValue(decided);
    const decideProposal = vi.fn(() => decision.promise);
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetProposals: getProposals,
          DecideLocalProposal: decideProposal,
        },
      },
    };

    render(
      <ApprovalsProvider>
        <PendingCountProbe />
        <DecisionControl />
      </ApprovalsProvider>,
    );

    const button = await screen.findByRole("button", { name: "Decide first proposal" });
    fireEvent.click(button);
    fireEvent.click(button);
    expect(decideProposal).toHaveBeenCalledTimes(1);

    decision.resolve({
      proposalId: pending.proposals[0]!.id,
      decision: "approved",
      message: "approved",
    });
    await waitFor(() => expect(screen.getByText("0 pending")).toBeVisible());
    expect(getProposals).toHaveBeenCalledTimes(2);
  });
});
