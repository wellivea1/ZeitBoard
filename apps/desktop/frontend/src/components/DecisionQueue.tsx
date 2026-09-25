import { useState } from "react";
import { ProposalCard } from "./ProposalCard";
import { TaskConflictCard, TaskConflictHistoryCard } from "./TaskConflictCard";
import { VisitorRequestCard } from "./VisitorRequestCard";
import { useApprovals } from "../state/approvals";
import { useBackendProposals } from "../state/backendProposals";
import { useVisitorRequests } from "../state/visitorRequests";
import { useApprovalQueue } from "../state/approvalQueue";
import { reviewIsPending, reviewStatus } from "../data/reviewQueue";
import type { BackendProposal } from "../data/backendProposals";
import { blockWording } from "../utils/relativeTime";

// Every change waiting for the owner's decision, in one list beside the tasks
// it is about.
//
// This used to be its own Approvals tab, with five filter chips and a
// paragraph on how each origin was stored, while Tasks showed the first
// suggestion again under "Proposed times". Adding a task and accepting its
// time were two screens apart. The decision now sits directly under the task
// list that produced it; each card still says where it came from, and each
// origin keeps its own authority and decision path.

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
      <div className="proposal-heading">
        <p>Assistant proposal</p>
        <h3>{proposal.title}</h3>
      </div>
      <p className="proposal-change">
        <strong>{proposal.window}</strong>
        {proposal.answer && <small>{proposal.answer}</small>}
      </p>
      {proposal.reasonLabels.length > 0 && (
        <p className="proposal-reasons">{proposal.reasonLabels.join(" · ")}</p>
      )}
      <p className="proposal-meta">
        {proposal.createdLabel} · {proposal.expiresLabel}
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

function QueueErrors() {
  const local = useApprovals();
  const backend = useBackendProposals();
  const visitor = useVisitorRequests();
  const messages = [
    local.error,
    backend.decisionError,
    backend.data.status === "error" ? backend.data.message : "",
    visitor.decisionError,
    visitor.data.status === "error" ? visitor.data.message : "",
  ].filter((message, index, all) => message && all.indexOf(message) === index);
  return (
    <>
      {messages.map((message) => (
        <p className="approval-error" role="alert" key={message}>
          {message}
        </p>
      ))}
    </>
  );
}

function QueuePagination() {
  const backend = useBackendProposals();
  const visitor = useVisitorRequests();
  const moreProposals = backend.data.status !== "off" && backend.data.pagination.hasMore;
  const moreRequests = visitor.data.status !== "off" && visitor.data.pagination.hasMore;
  if (!moreProposals && !moreRequests && !backend.loadOlderError && !visitor.loadOlderError) {
    return null;
  }
  return (
    <div className="synced-proposal-pagination">
      {backend.loadOlderError && <p role="alert">{backend.loadOlderError}</p>}
      {moreProposals && (
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
            ? "Loading older proposals…"
            : `Load older assistant proposals (${backend.data.pendingCount} pending in total)`}
        </button>
      )}
      {visitor.loadOlderError && <p role="alert">{visitor.loadOlderError}</p>}
      {moreRequests && (
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
          {`Load more time requests (${visitor.data.pendingCount} pending in total)`}
        </button>
      )}
    </div>
  );
}

export function DecisionQueue() {
  const local = useApprovals();
  const backend = useBackendProposals();
  const visitor = useVisitorRequests();
  const summary = useApprovalQueue();
  const [announcement, setAnnouncement] = useState("");
  const remotePending = backend.data.proposals.filter((item) => reviewIsPending(item, summary.now));
  const requests = visitor.data.requests.filter((item) => reviewIsPending(item, summary.now));
  const loaded =
    local.taskConflicts.length + local.pending.length + remotePending.length + requests.length;

  return (
    <section className="decision-queue" aria-labelledby="decision-title">
      <div className="plan-section-head">
        <h2 id="decision-title">
          Needs your decision
          {summary.pendingCount > 0 && <span className="count">{summary.pendingCount}</span>}
        </h2>
        {local.source === "fixture" && <span className="task-chip">Sample data</span>}
      </div>
      <QueueErrors />
      {!summary.ready && <p role="status">Loading…</p>}
      {summary.ready && loaded === 0 && (
        <p className="plan-empty">
          {summary.incomplete
            ? "Some sources could not be checked. They will be retried shortly."
            : "Nothing is waiting for you. New tasks get a suggested time here."}
        </p>
      )}
      {loaded > 0 && (
        <div className="proposal-stack">
          {local.taskConflicts.map((conflict) => (
            <TaskConflictCard conflict={conflict} key={conflict.reviewToken} />
          ))}
          {local.pending.map((proposal) => (
            <ProposalCard proposal={proposal} key={`local-${proposal.id}`} />
          ))}
          {remotePending.map((proposal) => (
            <SyncedProposalCard
              proposal={proposal}
              busy={backend.busyProposalId !== null || backend.data.status !== "ok"}
              key={`backend-${proposal.proposalId}`}
              onDecide={(item, decision) => {
                void backend
                  .decide(item, decision)
                  .then((result) =>
                    setAnnouncement(
                      result.status === "ok"
                        ? `${decision === "approved" ? "Approved" : "Rejected"} ${item.title}.`
                        : (result.message ??
                            "The decision could not be confirmed yet; it is checked again shortly."),
                    ),
                  );
              }}
            />
          ))}
          {requests.map((request) => (
            <VisitorRequestCard request={request} key={`visitor-${request.proposalId}`} />
          ))}
        </div>
      )}
      <QueuePagination />
      {local.unplaced.length > 0 && (
        <ul className="unplaced-list" aria-label="Tasks without a suggested time">
          {local.unplaced.map((item) => (
            <li key={item.title}>
              <strong>{item.title}</strong>
              <span>{item.reason}</span>
              <small>{item.nextAction}</small>
            </li>
          ))}
        </ul>
      )}
      <p role="status" className="sr-only">
        {announcement || visitor.announcement}
      </p>
    </section>
  );
}

export function DecisionHistory() {
  const local = useApprovals();
  const backend = useBackendProposals();
  const visitor = useVisitorRequests();
  const summary = useApprovalQueue();
  const backendHistory = backend.data.proposals.filter(
    (item) => !reviewIsPending(item, summary.now),
  );
  const visitorHistory = visitor.data.requests.filter(
    (item) => !reviewIsPending(item, summary.now),
  );
  const total =
    local.decided.length +
    local.taskConflictHistory.length +
    backendHistory.length +
    visitorHistory.length;

  return (
    <details className="decision-history">
      <summary>
        Decision history <span className="count">{total}</span>
      </summary>
      <p className="plan-empty">Only your own planner decisions can be undone.</p>
      <div className="approval-decision-list">
        {local.decided.map((proposal) => (
          <div key={`local-${proposal.id}`}>
            <span className="decision-state" data-decision={proposal.status}>
              {proposal.status}
            </span>
            <div>
              <strong>{proposal.title}</strong>
              <span title={proposal.to}>
                {blockWording(proposal.startAt, proposal.endAt, proposal.to, new Date(summary.now))}
              </span>
              <small>Planner · {proposal.createdLabel}</small>
            </div>
            <button
              className="button secondary compact"
              type="button"
              disabled={!local.ready || local.busyProposalId !== null || !proposal.canUndo}
              aria-label={`Undo decision on ${proposal.title}`}
              onClick={() => void local.undo(proposal.id)}
            >
              {local.busyProposalId === proposal.id ? "Undoing…" : "Undo"}
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
                Assistant · {proposal.createdLabel} · {proposal.expiresLabel}
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
      {local.taskConflictHistory.map((item) => (
        <TaskConflictHistoryCard item={item} key={item.reviewToken} />
      ))}
      {total === 0 && <p className="plan-empty">No decisions yet.</p>}
    </details>
  );
}
