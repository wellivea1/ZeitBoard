import { useApprovals } from "../state/approvals";
import type { ChangeProposalFixture, ProposalOrigin } from "../data/phaseTwo";
import type { ConfidenceLevel } from "../data/overview";

const confidenceSegments: Record<ConfidenceLevel, number> = { Low: 1, Medium: 2, High: 3 };
const originLabels: Record<ProposalOrigin, string> = {
  scheduler: "Scheduler",
  assistant: "Assistant",
  sync_conflict: "Sync conflict",
};

export function ConfidenceDots({ value }: { value: ConfidenceLevel }) {
  const filled = confidenceSegments[value];
  return (
    <span className="proposal-confidence" aria-label={`${value} confidence`}>
      <span className="proposal-confidence-label" aria-hidden="true">
        {value}
      </span>
      <span className="proposal-confidence-meter" aria-hidden="true">
        {[0, 1, 2].map((index) => (
          <span key={index} data-muted={index >= filled || undefined} />
        ))}
      </span>
    </span>
  );
}

export function ProposalCard({ proposal }: { proposal: ChangeProposalFixture }) {
  const { decide, busyProposalId, ready } = useApprovals();
  const busy = !ready || busyProposalId !== null;
  return (
    <article className="proposal-card" data-origin={proposal.origin}>
      <div className="proposal-header">
        <span className="proposal-kind">{proposal.kind}</span>
        <div>
          <p className="section-kicker">{originLabels[proposal.origin]} proposal</p>
          <h2>{proposal.title}</h2>
        </div>
        <ConfidenceDots value={proposal.confidence} />
      </div>
      <p className="proposal-change">
        {proposal.from && <span>From {proposal.from}</span>}
        <strong>To {proposal.to}</strong>
        <small>{proposal.rhythmContext}</small>
      </p>
      <div className="proposal-reasons" aria-label="Proposal reasons">
        {proposal.reasonLabels.map((reason) => (
          <span className="task-chip" key={reason}>
            {reason}
          </span>
        ))}
      </div>
      <p className="proposal-meta">
        {proposal.createdLabel} - {proposal.expiresLabel}
      </p>
      <div className="approval-actions">
        <button
          className="button secondary"
          type="button"
          disabled={busy}
          onClick={() => decide(proposal.id, "rejected")}
        >
          {busyProposalId === proposal.id ? "Recording..." : "Reject proposal"}
        </button>
        <button
          className="button primary"
          type="button"
          disabled={busy}
          onClick={() => decide(proposal.id, "approved")}
        >
          {busyProposalId === proposal.id ? "Recording..." : "Accept proposal"}
        </button>
      </div>
    </article>
  );
}
