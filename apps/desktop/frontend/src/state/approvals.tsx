import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import type { ChangeProposalFixture } from "../data/phaseTwo";
import {
  loadProposals,
  proposalsFixture,
  type ProposalsSource,
  type UnplacedProposal,
} from "../data/proposals";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";

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
  unplaced: UnplacedProposal[];
  source: ProposalsSource;
  decide: (id: string, decision: ProposalDecision) => void;
  undoLast: () => void;
  lastDecision: LastDecision | null;
}

const ApprovalsContext = createContext<ApprovalsContextValue | null>(null);

// Approval decisions stay in-session; nothing is written back to a calendar yet.
// The pending proposals are seeded from the local scheduling engine (GetProposals)
// and fall back to the shared fixture before the Wails service is ready.
export function ApprovalsProvider({ children }: { children: ReactNode }) {
  const [proposals, setProposals] = useState<DecidedProposal[]>(() =>
    proposalsFixture.proposals.map((proposal) => ({ ...proposal, status: "pending" })),
  );
  const [unplaced, setUnplaced] = useState<UnplacedProposal[]>(proposalsFixture.unplaced);
  const [source, setSource] = useState<ProposalsSource>("fixture");
  const [lastDecision, setLastDecision] = useState<LastDecision | null>(null);

  useEffect(() => {
    let current = true;
    const refresh = () =>
      void loadProposals().then((result) => {
        if (!current) return;
        setSource(result.source);
        setUnplaced(result.data.unplaced);
        // Only seed the queue while it is still untouched, so an in-flight load
        // never clobbers a decision the user already made.
        setProposals((existing) =>
          existing.some((proposal) => proposal.status !== "pending")
            ? existing
            : result.data.proposals.map((proposal) => ({ ...proposal, status: "pending" })),
        );
      });
    refresh();
    window.addEventListener(sleepDataChangedEvent, refresh);
    return () => {
      current = false;
      window.removeEventListener(sleepDataChangedEvent, refresh);
    };
  }, []);

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
    unplaced,
    source,
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
