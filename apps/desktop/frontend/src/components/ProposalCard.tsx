import { decisionButton, decisionLabel } from "../data/decisionWords";
import { useApprovals } from "../state/approvals";
import type { ChangeProposal, ProposalOrigin } from "../data/proposals";
import { blockWording } from "../utils/relativeTime";

// One suggested change, read in the order a person decides it: what, when,
// why, then accept or reject.
//
// The card used to lead with a High/Medium/Low meter. ADR-0022 measured those
// buckets inverted on real history, and Home already keeps the label behind a
// disclosure for that reason; a proposal is no place to put it back on the
// surface. The reasons say what the planner actually guaranteed.

const originLabels: Record<ProposalOrigin, string> = {
  scheduler: "Planner suggestion",
  assistant: "Assistant proposal",
  sync_conflict: "Sync conflict",
};

const kindLabels: Record<ChangeProposal["kind"], string> = {
  Place: "new time",
  Move: "moves an existing block",
  Reminder: "reminder",
};

export function ProposalCard({ proposal }: { proposal: ChangeProposal }) {
  const { decide, busyProposalId, ready } = useApprovals();
  const busy = !ready || busyProposalId !== null;
  return (
    <article className="proposal-card" data-origin={proposal.origin}>
      <div className="proposal-heading">
        <p>
          {originLabels[proposal.origin]} · {kindLabels[proposal.kind]}
        </p>
        <h3>{proposal.title}</h3>
      </div>
      <p className="proposal-change">
        <strong>{blockWording(proposal.startAt, proposal.endAt, proposal.to)}</strong>
        <small>{proposal.rhythmContext}</small>
        {proposal.from && <small>Currently {proposal.from}</small>}
      </p>
      {proposal.reasonLabels.length > 0 && (
        <p className="proposal-reasons">{proposal.reasonLabels.join(" · ")}</p>
      )}
      <p className="proposal-meta">{proposal.expiresLabel}</p>
      <div className="approval-actions">
        <button
          className="button primary"
          type="button"
          disabled={busy}
          aria-label={decisionLabel("approved", proposal.title)}
          onClick={() => decide(proposal.id, "approved")}
        >
          {busyProposalId === proposal.id ? "Recording…" : decisionButton("approved")}
        </button>
        <button
          className="button secondary"
          type="button"
          disabled={busy}
          aria-label={decisionLabel("rejected", proposal.title)}
          onClick={() => decide(proposal.id, "rejected")}
        >
          {decisionButton("rejected")}
        </button>
      </div>
    </article>
  );
}
