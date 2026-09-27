import { decisionButton, proposalDecisionLabel, type Decision } from "../data/decisionWords";
import {
  proposalAction,
  proposalIsWaiting,
  proposalName,
  proposalSubject,
  proposedDoseWording,
  proposedTaskWording,
  type AgentProposal,
} from "../data/agentProposals";
import { useAgentProposals } from "../state/agentProposals";
import { clockTime, dayInSentence } from "../utils/relativeTime";

// Something an assistant asked for that waits on this computer: a dose to
// record, a task to add. It stays a question until the owner answers it:
// accepting makes their own record of it, declining makes nothing, and a day
// later it lapses (ADR-0051).

function sentence(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function detail(proposal: AgentProposal, now: Date) {
  return proposal.kind === "dose"
    ? proposedDoseWording(proposal.dose, now)
    : proposedTaskWording(proposal.task, now);
}

function lapseWording(proposal: AgentProposal, now: Date) {
  const at = new Date(proposal.expiresAt);
  return `lapses ${dayInSentence(at, now)} at ${clockTime(at)}`;
}

function useDecision(proposal: AgentProposal, now: Date) {
  const { decide, busyProposalId } = useAgentProposals();
  const { action } = proposalAction(proposal);
  const subject = proposalSubject(proposal);
  const disabled = busyProposalId !== null || !proposalIsWaiting(proposal, now.getTime());
  const button = (decision: Decision) => ({
    type: "button" as const,
    disabled,
    "aria-label": proposalDecisionLabel(decision, action, subject),
    onClick: () => void decide(proposal, decision),
  });
  return { button, deciding: busyProposalId === proposal.proposalId };
}

/** The card in Plan's decision queue. */
export function AgentProposalCard({ proposal, now }: { proposal: AgentProposal; now: Date }) {
  const { button, deciding } = useDecision(proposal, now);
  return (
    <article className="proposal-card" data-origin="assistant">
      <div className="proposal-heading">
        <p>Assistant proposal</p>
        <h3>{proposal.title}</h3>
      </div>
      <p className="proposal-change">
        <strong>{proposalName(proposal)}</strong>
        <small>{sentence(detail(proposal, now))}</small>
      </p>
      <p className="proposal-meta">
        Nothing changes until you accept · {lapseWording(proposal, now)}
      </p>
      <div className="approval-actions">
        <button className="button primary" {...button("approved")}>
          {deciding ? "Recording…" : decisionButton("approved")}
        </button>
        <button className="button secondary" {...button("rejected")}>
          {decisionButton("rejected")}
        </button>
      </div>
    </article>
  );
}

/** The entry under Home's "Waiting on you", beside suggested times. */
export function AgentProposalEntry({ proposal, now }: { proposal: AgentProposal; now: Date }) {
  const { button, deciding } = useDecision(proposal, now);
  const lead =
    proposal.kind === "dose"
      ? "A dose your assistant asked to record,"
      : "A task your assistant asked to add,";
  return (
    <article className="home-entry">
      <h3>{proposalName(proposal)}</h3>
      <p>
        {lead} <strong>{detail(proposal, now)}</strong>.
      </p>
      <div className="home-entry-actions">
        <button className="button ghost" {...button("approved")}>
          {deciding ? "Recording…" : decisionButton("approved")}
        </button>
        <button className="button ghost" data-quiet {...button("rejected")}>
          {decisionButton("rejected")}
        </button>
      </div>
    </article>
  );
}

/** A decided or lapsed proposal, in the decision history. */
export function AgentProposalHistoryRow({ proposal, now }: { proposal: AgentProposal; now: Date }) {
  return (
    <div>
      <span className="decision-state" data-decision={proposal.state}>
        {proposal.state}
      </span>
      <div>
        <strong>
          {proposal.title} · {proposalName(proposal)}
        </strong>
        <span>{sentence(detail(proposal, now))}</span>
        <small>Assistant · proposed {dayInSentence(new Date(proposal.createdAt), now)}</small>
      </div>
    </div>
  );
}
