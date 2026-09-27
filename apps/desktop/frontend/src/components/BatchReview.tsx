import { useState } from "react";
import { decisionButton, type Decision } from "../data/decisionWords";
import { useApprovals } from "../state/approvals";
import { blockWording } from "../utils/relativeTime";

// Several suggested times, reviewed and decided together (ADR-0052).
//
// The planner suggests each task's time leaving room for the others, so the
// suggestions belong together: accepting one changes the calendar the rest
// were planned against. Here each is shown with its time and reasons, the
// owner leaves out any they want to decide alone, and the rest are decided in
// one step. If anything changed since the review, none is.

function suggestions(count: number) {
  return `${count} suggested time${count === 1 ? "" : "s"}`;
}

export function BatchReview({ onClose }: { onClose: () => void }) {
  const local = useApprovals();
  const [leftOut, setLeftOut] = useState<ReadonlySet<string>>(new Set());
  const chosen = local.pending.filter((proposal) => !leftOut.has(proposal.id));
  const busy = !local.ready || local.busyProposalId !== null;
  const now = new Date();

  const toggle = (id: string) =>
    setLeftOut((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const decide = async (decision: Decision) => {
    if (
      await local.decideTogether(
        chosen.map((proposal) => proposal.id),
        decision,
      )
    )
      onClose();
  };

  return (
    <section className="batch-review" aria-labelledby="batch-review-title">
      <h3 id="batch-review-title">Review together</h3>
      <p>
        Each was planned leaving room for the others, so they are decided in one step. If anything
        changes first, none is, and you can review them again.
      </p>
      <ul>
        {local.pending.map((proposal) => {
          const time = blockWording(proposal.startAt, proposal.endAt, proposal.to, now);
          return (
            <li key={proposal.id}>
              <label>
                <input
                  type="checkbox"
                  checked={!leftOut.has(proposal.id)}
                  disabled={busy}
                  // Read as "Email Dr. Okafor, Today 11:00 – 11:30 AM", not as
                  // the row's spans run together.
                  aria-label={`${proposal.title}, ${time}`}
                  onChange={() => toggle(proposal.id)}
                />
                <span className="batch-review-title">{proposal.title}</span>
                <strong>{time}</strong>
                {proposal.reasonLabels.length > 0 && (
                  <small>{proposal.reasonLabels.join(" · ")}</small>
                )}
              </label>
            </li>
          );
        })}
      </ul>
      <div className="batch-review-actions">
        <button
          className="button primary"
          type="button"
          disabled={busy || chosen.length === 0}
          aria-label={`${decisionButton("approved")} ${suggestions(chosen.length)}`}
          onClick={() => void decide("approved")}
        >
          {local.busyProposalId === "batch"
            ? "Recording…"
            : `${decisionButton("approved")} ${chosen.length}`}
        </button>
        <button
          className="button secondary"
          type="button"
          disabled={busy || chosen.length === 0}
          aria-label={`${decisionButton("rejected")} ${suggestions(chosen.length)}`}
          onClick={() => void decide("rejected")}
        >
          {`${decisionButton("rejected")} ${chosen.length}`}
        </button>
        <button className="button ghost" type="button" disabled={busy} onClick={onClose}>
          Cancel
        </button>
      </div>
    </section>
  );
}
