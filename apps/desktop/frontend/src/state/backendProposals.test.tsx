import { useEffect, useState } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BackendProposal } from "../data/backendProposals";
import { BackendProposalsProvider, useBackendProposals } from "./backendProposals";

const proposal: BackendProposal = {
  proposalId: "proposal_shared_01",
  action: "propose_place_task",
  status: "pending",
  title: "Place shared task",
  window: "Tomorrow, 10:00 AM to 10:30 AM",
  confidence: "Medium",
  reasonLabels: ["Fits predicted waking time"],
  createdLabel: "Created now",
  expiresLabel: "expires soon",
  decisionToken: "one-use-token",
};

const overlapProposal: BackendProposal = {
  ...proposal,
  proposalId: "proposal_overlap_01",
  title: "Current overlap title",
  decisionToken: "overlap-token",
};

const newerProposal: BackendProposal = {
  ...proposal,
  proposalId: "proposal_newer_01",
  title: "Newly ingested proposal",
  decisionToken: "newer-token",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}
function QueueView({ label }: { label: string }) {
  const { data, refresh, decide } = useBackendProposals();
  useEffect(refresh, [refresh]);
  const current = data.proposals[0];
  return (
    <section aria-label={label}>
      <span>{current?.status ?? data.status}</span>
      {current?.decisionToken && (
        <button type="button" onClick={() => void decide(current, "approved")}>
          Approve from {label}
        </button>
      )}
    </section>
  );
}

function DecisionRaceView() {
  const { data, refresh, decide } = useBackendProposals();
  const [message, setMessage] = useState("");
  useEffect(refresh, [refresh]);
  const current = data.proposals[0];
  return (
    <>
      <span>{current?.status ?? data.status}</span>
      {current?.decisionToken && (
        <button
          type="button"
          onClick={() =>
            void decide(current, "approved").then((result) => {
              if (result.status === "error") setMessage(result.message ?? "Decision blocked.");
            })
          }
        >
          Approve rapidly
        </button>
      )}
      <output>{message}</output>
    </>
  );
}

function PageQueueView({ requestTwice = false }: { requestTwice?: boolean }) {
  const { data, loadingOlder, loadOlderError, refresh, loadOlder, ingest } = useBackendProposals();
  useEffect(refresh, [refresh]);

  const requestOlder = () => {
    void loadOlder();
    if (requestTwice) void loadOlder();
  };

  return (
    <>
      {data.proposals.map((item) => (
        <span key={item.proposalId} data-testid="backend-proposal">
          {item.title}
        </span>
      ))}
      <span>
        {data.pagination.hasMore
          ? `Next cursor: ${data.pagination.nextCursor}`
          : "No older proposals"}
      </span>
      {data.pagination.hasMore && (
        <button type="button" disabled={loadingOlder} onClick={requestOlder}>
          {loadingOlder ? "Loading older proposals..." : "Load older proposals"}
        </button>
      )}
      <button type="button" onClick={() => ingest([newerProposal])}>
        Ingest newer proposal
      </button>
      {loadOlderError && <span role="alert">{loadOlderError}</span>}
    </>
  );
}

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("BackendProposalsProvider", () => {
  it("coalesces simultaneous consumers and publishes decisions to both", async () => {
    const list = vi.fn(async () => ({ status: "ok", proposals: [proposal] }));
    const decide = vi.fn(async () => ({ status: "ok", proposals: [] }));
    (globalThis as { go?: unknown }).go = {
      main: { App: { GetBackendProposals: list, DecideBackendProposal: decide } },
    };

    render(
      <BackendProposalsProvider>
        <QueueView label="approvals" />
        <QueueView label="assistant" />
      </BackendProposalsProvider>,
    );

    expect(await screen.findAllByText("pending")).toHaveLength(2);
    expect(list).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Approve from approvals" }));

    await waitFor(() => expect(screen.getAllByText("approved")).toHaveLength(2));
    expect(decide).toHaveBeenCalledTimes(1);
    expect(decide).toHaveBeenCalledWith({
      proposalId: proposal.proposalId,
      decision: "approved",
      token: proposal.decisionToken,
    });
    expect(screen.queryByRole("button", { name: /Approve from/ })).not.toBeInTheDocument();
  });

  it("reports a concurrent decision as blocked instead of false success", async () => {
    let resolveDecision!: (value: unknown) => void;
    const pendingDecision = new Promise((resolve) => {
      resolveDecision = resolve;
    });
    const decide = vi.fn(() => pendingDecision);
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetBackendProposals: vi.fn(async () => ({ status: "ok", proposals: [proposal] })),
          DecideBackendProposal: decide,
        },
      },
    };

    render(
      <BackendProposalsProvider>
        <DecisionRaceView />
      </BackendProposalsProvider>,
    );

    const button = await screen.findByRole("button", { name: "Approve rapidly" });
    fireEvent.click(button);
    fireEvent.click(button);

    expect(decide).toHaveBeenCalledTimes(1);
    expect(
      await screen.findByText("Another proposal decision is already in progress."),
    ).toBeVisible();

    resolveDecision({ status: "ok", proposals: [] });
    await waitFor(() => expect(screen.getByText("approved")).toBeVisible());
  });

  it("coalesces older-page loads and merges unique proposals without replacing newer records", async () => {
    const olderPage = deferred<unknown>();
    const loadPage = vi.fn(() => olderPage.promise);
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetBackendProposals: vi.fn(async () => ({
            status: "ok",
            proposals: [proposal, overlapProposal],
            pagination: { nextCursor: "cursor-older-01", hasMore: true },
          })),
          GetBackendProposalPage: loadPage,
        },
      },
    };

    render(
      <BackendProposalsProvider>
        <PageQueueView requestTwice />
      </BackendProposalsProvider>,
    );

    expect(await screen.findByText("Next cursor: cursor-older-01")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Load older proposals" }));

    expect(loadPage).toHaveBeenCalledTimes(1);
    expect(loadPage).toHaveBeenCalledWith({ cursor: "cursor-older-01" });
    expect(screen.getByRole("button", { name: "Loading older proposals..." })).toBeDisabled();

    olderPage.resolve({
      status: "ok",
      proposals: [
        { ...overlapProposal, title: "Stale overlap title" },
        {
          ...proposal,
          proposalId: "proposal_older_01",
          title: "Older proposal",
          decisionToken: undefined,
        },
      ],
      pagination: { nextCursor: "", hasMore: false },
    });

    expect(await screen.findByText("Older proposal")).toBeVisible();
    expect(screen.getAllByTestId("backend-proposal")).toHaveLength(3);
    expect(screen.getByText("Current overlap title")).toBeVisible();
    expect(screen.queryByText("Stale overlap title")).not.toBeInTheDocument();
    expect(screen.getByText("No older proposals")).toBeVisible();
    expect(screen.queryByRole("button", { name: /older proposals/i })).not.toBeInTheDocument();
  });

  it("ignores an older-page completion after newer proposals are ingested", async () => {
    const olderPage = deferred<unknown>();
    const loadPage = vi.fn(() => olderPage.promise);
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetBackendProposals: vi.fn(async () => ({
            status: "ok",
            proposals: [proposal],
            pagination: { nextCursor: "cursor-older-01", hasMore: true },
          })),
          GetBackendProposalPage: loadPage,
        },
      },
    };

    render(
      <BackendProposalsProvider>
        <PageQueueView />
      </BackendProposalsProvider>,
    );

    expect(await screen.findByText("Next cursor: cursor-older-01")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Load older proposals" }));
    expect(screen.getByRole("button", { name: "Loading older proposals..." })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Ingest newer proposal" }));
    expect(await screen.findByText("Newly ingested proposal")).toBeVisible();

    olderPage.resolve({
      status: "ok",
      proposals: [
        { ...proposal, title: "Stale current title" },
        {
          ...proposal,
          proposalId: "proposal_stale_older_01",
          title: "Stale older proposal",
        },
      ],
      pagination: { nextCursor: "", hasMore: false },
    });

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Load older proposals" })).toBeEnabled(),
    );
    expect(loadPage).toHaveBeenCalledTimes(1);
    expect(screen.getByText("Newly ingested proposal")).toBeVisible();
    expect(screen.getByText("Place shared task")).toBeVisible();
    expect(screen.queryByText("Stale current title")).not.toBeInTheDocument();
    expect(screen.queryByText("Stale older proposal")).not.toBeInTheDocument();
    expect(screen.getByText("Next cursor: cursor-older-01")).toBeVisible();
  });
});
