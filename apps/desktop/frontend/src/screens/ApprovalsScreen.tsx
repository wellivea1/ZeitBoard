import { useState } from "react";

import { PageHeader, PlaceholderNotice } from "../components/AppShell";

import { ProposalCard, ConfidenceDots } from "../components/ProposalCard";

import { VisitorRequestCard } from "../components/VisitorRequestsPanel";

import { useApprovals } from "../state/approvals";

import { useBackendProposals } from "../state/backendProposals";

import { useVisitorRequests } from "../state/visitorRequests";

import { useApprovalQueue } from "../state/approvalQueue";

import { reviewIsPending, reviewStatus } from "../data/reviewQueue";

import type { BackendProposal } from "../data/backendProposals";

function SyncedProposalCard({
  proposal,

  busy,

  onDecide,
}: {
  proposal: BackendProposal;

  busy: boolean;

  onDecide: (proposal: BackendProposal, decision: "approved" | "rejected") => void;
}) {
  return (
    <article className="proposal-card" data-origin="assistant">
      <div className="proposal-header">
        <span className="proposal-kind">Assistant / agent</span>

        <div>
          <p className="section-kicker">Backend proposal</p>

          <h2>{proposal.title}</h2>
        </div>

        <ConfidenceDots value={proposal.confidence} />
      </div>

      <p className="proposal-change">
        <strong>{proposal.window}</strong>

        {proposal.answer && <small>{proposal.answer}</small>}
      </p>

      {proposal.reasonLabels.length > 0 && (
        <div className="proposal-reasons" aria-label="Proposal reasons">
          {proposal.reasonLabels.map((reason) => (
            <span className="task-chip" key={reason}>
              {reason}
            </span>
          ))}
        </div>
      )}

      <p className="proposal-meta">
        {proposal.createdLabel} - {proposal.expiresLabel}
      </p>

      <div className="approval-actions">
        <button
          className="button secondary"
          type="button"
          disabled={busy || !reviewIsPending(proposal)}
          onClick={() => onDecide(proposal, "rejected")}
        >
          Reject proposal
        </button>

        <button
          className="button primary"
          type="button"
          disabled={busy || !reviewIsPending(proposal)}
          onClick={() => onDecide(proposal, "approved")}
        >
          Accept proposal
        </button>
      </div>
    </article>
  );
}

export function ApprovalsScreen({ embedded }: { embedded?: boolean } = {}) {
  const local = useApprovals();

  const backend = useBackendProposals();

  const visitor = useVisitorRequests();

  const summary = useApprovalQueue();

  const [filter, setFilter] = useState("all");

  const [announcement, setAnnouncement] = useState("");

  const retry = () => {
    void local.refresh();
    backend.refresh(true);
    visitor.refresh();
  };

  return (
    <>
      <PageHeader
        title="Approvals"
        level={embedded ? "panel" : "page"}
        description={
          !summary.ready
            ? "Loading your review queue."
            : summary.incomplete
              ? `${summary.pendingCount} pending at last check. Part of the queue could not be refreshed.`
              : summary.pendingCount > 0
                ? `${summary.pendingCount} pending changes and time requests. Review each before approving.`
                : "Nothing is waiting for your approval right now."
        }
        actions={
          <>
            {local.source === "fixture" && <span className="task-chip">Sample data</span>}
            <button className="button secondary" type="button" onClick={retry}>
              Refresh approvals
            </button>
          </>
        }
      />

      <PlaceholderNotice>
        Planner approvals write ZeitBoard-owned blocks to the local calendar and can be undone.
        Assistant and agent proposals use the connected backend. Time requests share only the exact
        block you accept. Imported events stay unchanged.
      </PlaceholderNotice>

      {[
        local.error,
        backend.decisionError,
        backend.data.status === "error" ? backend.data.message : "",
        visitor.decisionError,
        visitor.data.status === "error" ? visitor.data.message : "",
      ]
        .filter((message, index, all) => message && all.indexOf(message) === index)
        .map((message) => (
          <p className="approval-error" role="alert" key={message}>
            {message}
          </p>
        ))}

      <section className="screen-grid approval-screen" aria-label="Approval queue">
        <div className="approval-filter" role="group" aria-label="Filter approvals">
          {[
            ["all", `All ${summary.pendingCount}`],
            ["planner", `Local plan ${local.pendingCount}`],
            ["backend", `Assistant / agent ${backend.data.pendingCount}`],
            ["requests", `Time requests ${visitor.data.pendingCount}`],
            ["history", "History"],
          ].map(([value = "all", label]) => (
            <button
              className="button secondary compact"
              type="button"
              aria-pressed={filter === value}
              key={value}
              onClick={() => setFilter(value)}
            >
              {label}
            </button>
          ))}
        </div>

        {filter !== "history" && <PendingReviews filter={filter} onAnnounce={setAnnouncement} />}

        {filter === "history" && <ReviewHistory />}

        <QueuePagination filter={filter} />

        {local.unplaced.length > 0 && ["all", "planner"].includes(filter) && (
          <section className="unplaced-panel" aria-labelledby="approvals-unplaced-title">
            <p className="section-kicker">Not proposed</p>
            <h2 id="approvals-unplaced-title">
              {local.unplaced.length} tasks without a safe window
            </h2>
            <ul className="unplaced-list">
              {local.unplaced.map((item) => (
                <li key={item.title}>
                  <strong>{item.title}</strong>
                  <span>{item.reason}</span>
                  <small>{item.nextAction}</small>
                </li>
              ))}
            </ul>
          </section>
        )}
      </section>
      <p role="status" className="sr-only">
        {announcement || visitor.announcement}
      </p>
    </>
  );
}

function ReviewHistory() {
  const local = useApprovals(),
    backend = useBackendProposals(),
    visitor = useVisitorRequests(),
    summary = useApprovalQueue();
  const backendHistory = backend.data.proposals.filter(
    (item) => !reviewIsPending(item, summary.now),
  );

  const visitorHistory = visitor.data.requests.filter(
    (item) => !reviewIsPending(item, summary.now),
  );

  return (
    <section className="panel approval-history-panel" aria-labelledby="approval-history-title">
      <h2 id="approval-history-title">Review history</h2>
      <p>
        Recent decisions and requests that can no longer be decided. Only local planner decisions
        support undo.
      </p>

      <div className="approval-decision-list">
        {local.decided.map((proposal) => (
          <div key={`local-${proposal.id}`}>
            <span className="decision-state" data-decision={proposal.status}>
              {proposal.status}
            </span>
            <div>
              <strong>{proposal.title}</strong>
              <span>{proposal.to}</span>
              <small>Local planner · {proposal.createdLabel}</small>
            </div>
            <button
              className="button secondary compact"
              type="button"
              disabled={!local.ready || local.busyProposalId !== null || !proposal.canUndo}
              onClick={() => void local.undo(proposal.id)}
            >
              {local.busyProposalId === proposal.id ? "Undoing..." : "Undo decision"}
            </button>
          </div>
        ))}

        {backendHistory.map((proposal) => (
          <div key={`backend-${proposal.proposalId}`}>
            <span className="decision-state" data-decision={proposal.status}>
              {reviewStatus(proposal, summary.now)}
            </span>
            <div>
              <strong>{proposal.title}</strong>
              <span>{proposal.window}</span>
              <small>
                Assistant / agent · {proposal.createdLabel} · {proposal.expiresLabel}
              </small>
            </div>
          </div>
        ))}

        {visitorHistory.map((request) => (
          <div key={`visitor-${request.proposalId}`}>
            <span className="decision-state" data-decision={request.status}>
              {reviewStatus(request, summary.now)}
            </span>
            <div>
              <strong>
                {request.handle || "Visitor"} · {request.linkLabel}
              </strong>
              <span>Requested: {request.windowLabel}</span>
              <small>
                {request.createdLabel} · {request.expiresLabel}
              </small>
            </div>
          </div>
        ))}
      </div>

      {local.decided.length + backendHistory.length + visitorHistory.length === 0 && (
        <p>No history loaded yet.</p>
      )}
    </section>
  );
}

function QueuePagination({ filter }: { filter: string }) {
  const backend = useBackendProposals(),
    visitor = useVisitorRequests();
  return (
    <>
      {" "}
      {(filter === "all" || filter === "backend" || filter === "history") &&
        backend.data.status !== "off" && (
          <div className="synced-proposal-pagination">
            <span>
              {backend.data.proposals.length} assistant / agent items loaded ·{" "}
              {backend.data.pendingCount} pending in total
            </span>
            {backend.loadOlderError && <p role="alert">{backend.loadOlderError}</p>}
            {backend.data.pagination.hasMore && (
              <button
                className="button secondary compact"
                type="button"
                disabled={
                  backend.loading ||
                  backend.loadingOlder ||
                  backend.busyProposalId !== null ||
                  backend.data.status !== "ok"
                }
                onClick={() => void backend.loadOlder()}
              >
                {backend.loadingOlder
                  ? "Loading older proposals..."
                  : "Load older synced proposals"}
              </button>
            )}
          </div>
        )}
      {(filter === "all" || filter === "requests" || filter === "history") &&
        visitor.data.status !== "off" && (
          <div className="synced-proposal-pagination">
            <span>
              {visitor.data.requests.length} time requests loaded · {visitor.data.pendingCount}{" "}
              pending in total
            </span>
            {visitor.loadOlderError && <p role="alert">{visitor.loadOlderError}</p>}
            {visitor.data.pagination.hasMore && (
              <button
                className="button secondary compact"
                type="button"
                disabled={
                  visitor.loading ||
                  visitor.loadingOlder ||
                  visitor.busyId !== null ||
                  visitor.data.status !== "ok"
                }
                onClick={() => void visitor.loadOlder()}
              >
                Load more time requests
              </button>
            )}
          </div>
        )}
    </>
  );
}

function PendingReviews({
  filter,
  onAnnounce,
}: {
  filter: string;
  onAnnounce: (message: string) => void;
}) {
  const local = useApprovals(),
    backend = useBackendProposals(),
    visitor = useVisitorRequests(),
    summary = useApprovalQueue();
  const remotePending = backend.data.proposals.filter((item) => reviewIsPending(item, summary.now));

  const requests = visitor.data.requests.filter((item) => reviewIsPending(item, summary.now));

  const loaded =
    (filter === "all" || filter === "planner" ? local.pending.length : 0) +
    (filter === "all" || filter === "backend" ? remotePending.length : 0) +
    (filter === "all" || filter === "requests" ? requests.length : 0);

  return (
    <>
      {!summary.ready && <p role="status">Loading approvals...</p>}

      {loaded > 0 && (
        <div className="proposal-stack">
          {(filter === "all" || filter === "planner") &&
            local.pending.map((proposal) => (
              <ProposalCard proposal={proposal} key={`local-${proposal.id}`} />
            ))}

          {(filter === "all" || filter === "backend") &&
            remotePending.map((proposal) => (
              <SyncedProposalCard
                proposal={proposal}
                busy={backend.busyProposalId !== null || backend.data.status !== "ok"}
                key={`backend-${proposal.proposalId}`}
                onDecide={(item, decision) => {
                  void backend
                    .decide(item, decision)
                    .then((result) =>
                      onAnnounce(
                        result.status === "ok"
                          ? `${decision === "approved" ? "Approved" : "Rejected"} ${item.title}.`
                          : (result.message ?? "Refresh to confirm the decision."),
                      ),
                    );
                }}
              />
            ))}

          {(filter === "all" || filter === "requests") &&
            requests.map((request) => (
              <VisitorRequestCard request={request} key={`visitor-${request.proposalId}`} />
            ))}
        </div>
      )}

      {summary.ready && loaded === 0 && (
        <div className="panel empty-state">
          <h2>
            {summary.incomplete
              ? "Queue needs a refresh"
              : summary.pendingCount === 0
                ? "Nothing waiting for approval"
                : "No loaded items in this filter"}
          </h2>
          <p>
            {summary.incomplete
              ? "Some sources are unavailable. Refresh to check the full queue."
              : "Use the filters or load more items below to review the queue."}
          </p>
        </div>
      )}
    </>
  );
}
