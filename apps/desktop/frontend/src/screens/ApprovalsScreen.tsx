import { useEffect, useState } from "react";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";
import { ProposalCard, ConfidenceDots } from "../components/ProposalCard";
import { useApprovals } from "../state/approvals";
import type { ProposalOrigin } from "../data/phaseTwo";
import {
  decideBackendProposal,
  loadBackendProposals,
  type BackendProposal,
  type BackendProposalsData,
} from "../data/backendProposals";

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
        <span className="proposal-kind">Synced</span>
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
          disabled={busy || !proposal.decisionToken}
          onClick={() => onDecide(proposal, "rejected")}
        >
          Reject proposal
        </button>
        <button
          className="button primary"
          type="button"
          disabled={busy || !proposal.decisionToken}
          onClick={() => onDecide(proposal, "approved")}
        >
          Accept proposal
        </button>
      </div>
    </article>
  );
}

function SyncedProposalsPanel() {
  const [data, setData] = useState<BackendProposalsData>({ status: "off", proposals: [] });
  const [busy, setBusy] = useState(false);
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    let current = true;
    void loadBackendProposals().then((result) => {
      if (current) setData(result);
    });
    return () => {
      current = false;
    };
  }, []);

  // Omit, don't disable: with sync off this surface simply is not there.
  if (data.status === "off") return null;

  const pending = data.proposals.filter((proposal) => proposal.status === "pending");
  const decided = data.proposals.length - pending.length;

  const onDecide = (proposal: BackendProposal, decision: "approved" | "rejected") => {
    if (!proposal.decisionToken) return;
    setBusy(true);
    void decideBackendProposal({
      proposalId: proposal.proposalId,
      decision,
      token: proposal.decisionToken,
    }).then((result) => {
      setBusy(false);
      setData(result);
      setAnnouncement(
        result.status === "ok"
          ? `${decision === "approved" ? "Approved" : "Rejected"} ${proposal.title}.`
          : (result.message ?? "The decision could not be recorded."),
      );
    });
  };

  return (
    <section className="panel synced-proposals-panel" aria-labelledby="synced-proposals-title">
      <div className="panel-heading">
        <div>
          <p className="section-kicker">Synced backend</p>
          <h2 id="synced-proposals-title">Assistant and agent proposals</h2>
        </div>
        <div className="status-cluster">
          <span className="sync-dot" data-mode="backend" aria-hidden="true" />
          <span>{pending.length} pending</span>
        </div>
      </div>
      {data.status === "error" && (
        <p className="diff-note">{data.message ?? "Could not reach the synced backend."}</p>
      )}
      {pending.length > 0 ? (
        <div className="proposal-stack">
          {pending.map((proposal) => (
            <SyncedProposalCard
              proposal={proposal}
              busy={busy}
              onDecide={onDecide}
              key={proposal.proposalId}
            />
          ))}
        </div>
      ) : (
        data.status === "ok" && (
          <p className="phase-two-copy">
            No synced proposals are waiting.{" "}
            {decided > 0 &&
              `${decided} earlier ${decided === 1 ? "decision is" : "decisions are"} recorded on the backend.`}
          </p>
        )
      )}
      <p role="status" aria-live="polite" className="sr-only">
        {announcement}
      </p>
    </section>
  );
}

export function ApprovalsScreen() {
  const {
    pending,
    decided,
    pendingCount,
    unplaced,
    source,
    undo,
    busyProposalId,
    error,
    ready,
    dismissError,
  } = useApprovals();
  const byOrigin = (origin: ProposalOrigin) =>
    pending.filter((proposal) => proposal.origin === origin).length;
  return (
    <>
      <PageHeader
        title="Approvals"
        description={
          !ready
            ? "Loading the local proposal queue."
            : pendingCount > 0
              ? `${pendingCount} pending ${pendingCount === 1 ? "proposal" : "proposals"}. Approve or reject each change before anything moves.`
              : "Nothing is waiting for your approval right now."
        }
        actions={
          <div className="status-cluster">
            <span className="sync-dot" data-mode={source} aria-hidden="true" />
            <span>
              {!ready ? "Loading local plan" : source === "local" ? "Local plan" : "Sample data"}
            </span>
          </div>
        }
      />
      <PlaceholderNotice>
        Approval writes a ZeitBoard-owned block to the local calendar. Imported events remain
        immutable, rejection writes nothing, and every local decision can be undone.
      </PlaceholderNotice>
      {error && (
        <div className="approval-error" role="alert">
          <span>{error}</span>
          <button className="text-button" type="button" onClick={dismissError}>
            Dismiss
          </button>
        </div>
      )}
      <section className="screen-grid approval-screen" aria-label="Pending approval queue">
        <div className="approval-filter" aria-label="Pending proposals by origin">
          <span>All {pendingCount}</span>
          <span>Scheduler {byOrigin("scheduler")}</span>
          <span>Assistant {byOrigin("assistant")}</span>
          <span>Sync {byOrigin("sync_conflict")}</span>
        </div>
        {!ready ? (
          <div className="panel empty-state" role="status">
            <p className="section-kicker">Local planner</p>
            <h2>Loading proposals</h2>
            <p>Reading the current sleep, task, and fixed-event snapshots.</p>
          </div>
        ) : pendingCount > 0 ? (
          <div className="proposal-stack">
            {pending.map((proposal) => (
              <ProposalCard proposal={proposal} key={proposal.id} />
            ))}
          </div>
        ) : (
          <div className="panel empty-state">
            <p className="section-kicker">All clear</p>
            <h2>Nothing waiting for approval</h2>
            <p>
              Proposals from the planner and assistant appear here. Nothing changes until you
              approve it.
            </p>
          </div>
        )}

        {decided.length > 0 && (
          <section className="panel approval-history-panel" aria-labelledby="local-decisions-title">
            <div className="panel-heading">
              <div>
                <p className="section-kicker">Local decision record</p>
                <h2 id="local-decisions-title">Approved and rejected proposals</h2>
              </div>
              <span>{decided.length} active</span>
            </div>
            <div className="approval-decision-list">
              {decided.map((proposal) => (
                <div key={proposal.id}>
                  <span className="decision-state" data-decision={proposal.status}>
                    {proposal.status}
                  </span>
                  <div>
                    <strong>{proposal.title}</strong>
                    <span>{proposal.to}</span>
                    <small>{proposal.createdLabel}</small>
                  </div>
                  <button
                    className="button ghost compact"
                    type="button"
                    disabled={!ready || busyProposalId !== null || !proposal.canUndo}
                    onClick={() => undo(proposal.id)}
                  >
                    {busyProposalId === proposal.id ? "Undoing..." : "Undo decision"}
                  </button>
                </div>
              ))}
            </div>
          </section>
        )}

        <SyncedProposalsPanel />

        {unplaced.length > 0 && (
          <section className="unplaced-panel" aria-labelledby="approvals-unplaced-title">
            <p className="section-kicker">Not proposed</p>
            <h2 id="approvals-unplaced-title">
              {unplaced.length} task{unplaced.length === 1 ? "" : "s"} without a safe window
            </h2>
            <ul className="unplaced-list">
              {unplaced.map((item) => (
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
    </>
  );
}
