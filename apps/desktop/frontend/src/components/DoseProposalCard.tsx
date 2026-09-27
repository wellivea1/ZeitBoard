import { decisionButton, doseDecisionLabel, type Decision } from "../data/decisionWords";
import {
  doseIsWaiting,
  proposedDoseMedication,
  proposedDoseWording,
  type DoseProposal,
} from "../data/doseProposals";
import { useDoseProposals } from "../state/doseProposals";
import { clockTime, dayInSentence } from "../utils/relativeTime";

// A dose an assistant asked to record. A dose is a health record, so it stays
// a question until the owner answers it: accepting records it as their own
// dose, declining records nothing, and a day later it lapses (ADR-0051).

function sentence(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function lapseWording(proposal: DoseProposal, now: Date) {
  const at = new Date(proposal.expiresAt);
  return `lapses ${dayInSentence(at, now)} at ${clockTime(at)}`;
}

function useDoseDecision(proposal: DoseProposal, now: Date) {
  const { decide, busyProposalId } = useDoseProposals();
  const medication = proposedDoseMedication(proposal);
  const disabled = busyProposalId !== null || !doseIsWaiting(proposal, now.getTime());
  const button = (decision: Decision) => ({
    type: "button" as const,
    disabled,
    "aria-label": doseDecisionLabel(decision, medication),
    onClick: () => void decide(proposal, decision),
  });
  return { medication, button, recording: busyProposalId === proposal.proposalId };
}

/** The card in Plan's decision queue. */
export function DoseProposalCard({ proposal, now }: { proposal: DoseProposal; now: Date }) {
  const { medication, button, recording } = useDoseDecision(proposal, now);
  return (
    <article className="proposal-card" data-origin="assistant">
      <div className="proposal-heading">
        <p>Assistant proposal</p>
        <h3>{proposal.title}</h3>
      </div>
      <p className="proposal-change">
        <strong>{medication}</strong>
        <small>{sentence(proposedDoseWording(proposal, now))}</small>
      </p>
      <p className="proposal-meta">
        Nothing is recorded until you accept · {lapseWording(proposal, now)}
      </p>
      <div className="approval-actions">
        <button className="button primary" {...button("approved")}>
          {recording ? "Recording…" : decisionButton("approved")}
        </button>
        <button className="button secondary" {...button("rejected")}>
          {decisionButton("rejected")}
        </button>
      </div>
    </article>
  );
}

/** The entry under Home's "Waiting on you", beside suggested times. */
export function DoseProposalEntry({ proposal, now }: { proposal: DoseProposal; now: Date }) {
  const { medication, button, recording } = useDoseDecision(proposal, now);
  return (
    <article className="home-entry">
      <h3>{medication}</h3>
      <p>
        A dose your assistant asked to record, <strong>{proposedDoseWording(proposal, now)}</strong>
        .
      </p>
      <div className="home-entry-actions">
        <button className="button ghost" {...button("approved")}>
          {recording ? "Recording…" : decisionButton("approved")}
        </button>
        <button className="button ghost" data-quiet {...button("rejected")}>
          {decisionButton("rejected")}
        </button>
      </div>
    </article>
  );
}

/** A decided or lapsed proposal, in the decision history. */
export function DoseProposalHistoryRow({ proposal, now }: { proposal: DoseProposal; now: Date }) {
  // The history's words for every origin: approved, rejected, expired.
  const state =
    proposal.state === "recorded"
      ? "approved"
      : proposal.state === "discarded"
        ? "rejected"
        : proposal.state;
  return (
    <div>
      <span className="decision-state" data-decision={state}>
        {state}
      </span>
      <div>
        <strong>
          {proposal.title} · {proposedDoseMedication(proposal)}
        </strong>
        <span>{sentence(proposedDoseWording(proposal, now))}</span>
        <small>Assistant · proposed {dayInSentence(new Date(proposal.createdAt), now)}</small>
      </div>
    </div>
  );
}
