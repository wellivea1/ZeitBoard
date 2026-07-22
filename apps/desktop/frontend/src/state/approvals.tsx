import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  decideLocalProposal,
  hasLocalProposalService,
  loadProposals,
  proposalsFixture,
  undoLocalProposalDecision,
  type ProposalRecord,
  type ProposalsResult,
  type ProposalsSource,
  type UnplacedProposal,
} from "../data/proposals";
import { notifyCalendarDataChanged } from "../data/calendar";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";

export type ProposalDecision = "approved" | "rejected";
export type ProposalStatus = "pending" | ProposalDecision;

export interface DecidedProposal extends ProposalRecord {
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
  decided: DecidedProposal[];
  pendingCount: number;
  unplaced: UnplacedProposal[];
  source: ProposalsSource;
  decide: (id: string, decision: ProposalDecision) => void;
  undo: (id: string) => void;
  undoLast: () => void;
  lastDecision: LastDecision | null;
  busyProposalId: string | null;
  error: string;
  ready: boolean;
  dismissError: () => void;
}

const ApprovalsContext = createContext<ApprovalsContextValue | null>(null);

function withStatus(proposal: ProposalRecord): DecidedProposal {
  return { ...proposal, status: proposal.decision };
}

export function ApprovalsProvider({ children }: { children: ReactNode }) {
  const localServicePresent = hasLocalProposalService();
  const [proposals, setProposals] = useState<DecidedProposal[]>(() =>
    localServicePresent ? [] : proposalsFixture.proposals.map(withStatus),
  );
  const [unplaced, setUnplaced] = useState<UnplacedProposal[]>(
    localServicePresent ? [] : proposalsFixture.unplaced,
  );
  const [source, setSource] = useState<ProposalsSource>(localServicePresent ? "local" : "fixture");
  const [ready, setReady] = useState(!localServicePresent);
  const [lastDecision, setLastDecision] = useState<LastDecision | null>(null);
  const [busyProposalId, setBusyProposalId] = useState<string | null>(null);
  const [error, setError] = useState("");
  const mounted = useRef(false);
  const requestVersion = useRef(0);
  const busyRef = useRef<string | null>(null);

  const applyResult = useCallback((result: ProposalsResult) => {
    setSource(result.source);
    setUnplaced(result.data.unplaced);
    setProposals(result.data.proposals.map(withStatus));
    setReady(true);
  }, []);

  const refresh = useCallback(async () => {
    const version = ++requestVersion.current;
    try {
      const result = await loadProposals();
      if (!mounted.current || version !== requestVersion.current) return;
      applyResult(result);
      setError("");
    } catch (reason) {
      if (!mounted.current || version !== requestVersion.current) return;
      setReady(true);
      setError(reason instanceof Error ? reason.message : "Proposal queue could not be loaded.");
    }
  }, [applyResult]);

  useEffect(() => {
    busyRef.current = busyProposalId;
  }, [busyProposalId]);

  useEffect(() => {
    mounted.current = true;
    void Promise.resolve().then(refresh);
    const onSleepChanged = () => {
      if (busyRef.current === null) void refresh();
    };
    window.addEventListener(sleepDataChangedEvent, onSleepChanged);
    return () => {
      mounted.current = false;
      window.removeEventListener(sleepDataChangedEvent, onSleepChanged);
    };
  }, [refresh]);

  const decide = (id: string, decision: ProposalDecision) => {
    const target = proposals.find((proposal) => proposal.id === id);
    if (!ready || !target || target.status !== "pending" || busyProposalId) return;
    if (source === "fixture") {
      setProposals((current) =>
        current.map((proposal) =>
          proposal.id === id
            ? { ...proposal, decision, status: decision, canUndo: true }
            : proposal,
        ),
      );
      setLastDecision({ id, title: target.title, decision });
      return;
    }

    setBusyProposalId(id);
    setError("");
    void decideLocalProposal(id, decision).then(
      async () => {
        if (decision === "approved") notifyCalendarDataChanged();
        await refresh();
        if (!mounted.current) return;
        setBusyProposalId(null);
        setLastDecision({ id, title: target.title, decision });
      },
      (reason: unknown) => {
        if (!mounted.current) return;
        setBusyProposalId(null);
        setError(reason instanceof Error ? reason.message : "Proposal decision failed.");
        void refresh();
      },
    );
  };

  const undo = (id: string) => {
    const target = proposals.find((proposal) => proposal.id === id);
    if (!ready || !target || target.status === "pending" || busyProposalId) return;
    if (source === "fixture") {
      setProposals((current) =>
        current.map((proposal) =>
          proposal.id === id
            ? { ...proposal, decision: "pending", status: "pending", canUndo: false }
            : proposal,
        ),
      );
      setLastDecision(null);
      return;
    }

    setBusyProposalId(id);
    setError("");
    void undoLocalProposalDecision(id).then(
      async () => {
        notifyCalendarDataChanged();
        await refresh();
        if (!mounted.current) return;
        setBusyProposalId(null);
        setLastDecision(null);
      },
      (reason: unknown) => {
        if (!mounted.current) return;
        setBusyProposalId(null);
        setError(reason instanceof Error ? reason.message : "Proposal undo failed.");
        void refresh();
      },
    );
  };

  const undoLast = () => {
    if (lastDecision) undo(lastDecision.id);
  };

  const dismiss = useCallback(() => setLastDecision(null), []);
  const dismissError = useCallback(() => setError(""), []);
  const pending = proposals.filter((proposal) => proposal.status === "pending");
  const decided = proposals.filter((proposal) => proposal.status !== "pending");
  const value: ApprovalsContextValue = {
    proposals,
    pending,
    decided,
    pendingCount: pending.length,
    unplaced,
    source,
    decide,
    undo,
    undoLast,
    lastDecision,
    busyProposalId,
    error,
    ready,
    dismissError,
  };

  return (
    <ApprovalsContext.Provider value={value}>
      {children}
      {lastDecision && (
        <ApprovalUndoToast
          key={`${lastDecision.id}-${lastDecision.decision}`}
          decision={lastDecision}
          busy={busyProposalId === lastDecision.id}
          onUndo={undoLast}
          onDismiss={dismiss}
        />
      )}
    </ApprovalsContext.Provider>
  );
}

function ApprovalUndoToast({
  decision,
  busy,
  onUndo,
  onDismiss,
}: {
  decision: LastDecision;
  busy: boolean;
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
      <button type="button" disabled={busy} onClick={onUndo}>
        {busy ? "Undoing..." : "Undo"}
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
