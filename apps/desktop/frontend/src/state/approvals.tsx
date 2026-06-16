import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { proposalFixtures, type ChangeProposalFixture } from "../data/phaseTwo";

export type ProposalDecision = "approved" | "rejected";
export type ProposalStatus = "pending" | ProposalDecision;

export interface DecidedProposal extends ChangeProposalFixture {
  status: ProposalStatus;
}

interface LastDecision {
  id: string;
  title: string;
  decision: ProposalDecision;
}

interface ApprovalsContextValue {
  proposals: DecidedProposal[];
  pending: DecidedProposal[];
  pendingCount: number;
  decide: (id: string, decision: ProposalDecision) => void;
  undoLast: () => void;
  lastDecision: LastDecision | null;
}

const ApprovalsContext = createContext<ApprovalsContextValue | null>(null);

// In-session approval state. Nothing is persisted or applied to a backend yet;
// the shapes mirror the planned change-proposal contract so a later swap is clean.
export function ApprovalsProvider({ children }: { children: ReactNode }) {
  const [proposals, setProposals] = useState<DecidedProposal[]>(() =>
    proposalFixtures.map((proposal) => ({ ...proposal, status: "pending" })),
  );
  const [lastDecision, setLastDecision] = useState<LastDecision | null>(null);

  const decide = (id: string, decision: ProposalDecision) => {
    const target = proposals.find((proposal) => proposal.id === id);
    if (!target || target.status !== "pending") return;
    setProposals((current) =>
      current.map((proposal) =>
        proposal.id === id ? { ...proposal, status: decision } : proposal,
      ),
    );
    setLastDecision({ id, title: target.title, decision });
  };

  const undoLast = () => {
    if (!lastDecision) return;
    const { id } = lastDecision;
    setProposals((current) =>
      current.map((proposal) =>
        proposal.id === id ? { ...proposal, status: "pending" } : proposal,
      ),
    );
    setLastDecision(null);
  };

  const dismiss = useCallback(() => setLastDecision(null), []);

  const pending = proposals.filter((proposal) => proposal.status === "pending");
  const value: ApprovalsContextValue = {
    proposals,
    pending,
    pendingCount: pending.length,
    decide,
    undoLast,
    lastDecision,
  };

  return (
    <ApprovalsContext.Provider value={value}>
      {children}
      {lastDecision && (
        <ApprovalUndoToast
          key={`${lastDecision.id}-${lastDecision.decision}`}
          decision={lastDecision}
          onUndo={undoLast}
          onDismiss={dismiss}
        />
      )}
    </ApprovalsContext.Provider>
  );
}

function ApprovalUndoToast({
  decision,
  onUndo,
  onDismiss,
}: {
  decision: LastDecision;
  onUndo: () => void;
  onDismiss: () => void;
}) {
  useEffect(() => {
    const timer = setTimeout(onDismiss, 6000);
    return () => clearTimeout(timer);
  }, [onDismiss]);

  const verb = decision.decision === "approved" ? "Approved" : "Rejected";
  return (
    <div className="undo-toast" role="status" aria-live="polite">
      <span>
        {verb} {decision.title}.
      </span>
      <button type="button" onClick={onUndo}>
        Undo
      </button>
    </div>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useApprovals(): ApprovalsContextValue {
  const context = useContext(ApprovalsContext);
  if (!context) {
    throw new Error("useApprovals must be used within ApprovalsProvider");
  }
  return context;
}
