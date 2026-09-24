import {
  resolveTaskConflict as recordTaskResolution,
  type TaskConflict,
  type TaskConflictHistory,
} from "../data/taskConflicts";
import { notifySleepDataChanged } from "../data/sleepDataEvents";
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
import { subscribeProjectionRefresh } from "../utils/projectionRefresh";

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
  taskConflicts: TaskConflict[];
  taskConflictHistory: TaskConflictHistory[];
  resolveTaskConflict: (conflict: TaskConflict, choiceId: string) => Promise<void>;
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
  loadError: string;
  ready: boolean;
  dismissError: () => void;
  refresh: () => Promise<void>;
}

const ApprovalsContext = createContext<ApprovalsContextValue | null>(null);
const PendingApprovalsCountContext = createContext<number | null>(null);

function withStatus(proposal: ProposalRecord): DecidedProposal {
  return { ...proposal, status: proposal.decision };
}

export function ApprovalsProvider({ children }: { children: ReactNode }) {
  const localServicePresent = hasLocalProposalService();
  const [proposals, setProposals] = useState<DecidedProposal[]>(() =>
    localServicePresent ? [] : proposalsFixture.proposals.map(withStatus),
  );
  const [taskConflicts, setTaskConflicts] = useState<TaskConflict[]>([]);
  const [taskConflictHistory, setTaskConflictHistory] = useState<TaskConflictHistory[]>([]);
  const [unplaced, setUnplaced] = useState<UnplacedProposal[]>(
    localServicePresent ? [] : proposalsFixture.unplaced,
  );
  const [source, setSource] = useState<ProposalsSource>(localServicePresent ? "local" : "fixture");
  const [ready, setReady] = useState(!localServicePresent);
  const [lastDecision, setLastDecision] = useState<LastDecision | null>(null);
  const [busyProposalId, setBusyProposalId] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [decisionError, setDecisionError] = useState("");
  const mounted = useRef(false);
  const requestVersion = useRef(0);
  const busyRef = useRef<string | null>(null);

  const applyResult = useCallback((result: ProposalsResult) => {
    setTaskConflicts(result.data.taskConflicts);
    setTaskConflictHistory(result.data.taskConflictHistory);
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
      setProposals([]);
      setUnplaced([]);
      setReady(true);
      setError(reason instanceof Error ? reason.message : "Proposal queue could not be loaded.");
    }
  }, [applyResult]);

  useEffect(() => {
    mounted.current = true;
    const onSleepChanged = () => {
      if (busyRef.current === null) void refresh();
    };
    const unsubscribe = subscribeProjectionRefresh(onSleepChanged, sleepDataChangedEvent);
    return () => {
      mounted.current = false;
      unsubscribe();
    };
  }, [refresh]);

  const decide = (id: string, decision: ProposalDecision) => {
    const target = proposals.find((proposal) => proposal.id === id);
    if (!ready || !target || target.status !== "pending" || busyRef.current) return;
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

    busyRef.current = id;
    setBusyProposalId(id);
    setDecisionError("");
    void decideLocalProposal(id, decision).then(
      async () => {
        if (decision === "approved") notifyCalendarDataChanged();
        await refresh();
        busyRef.current = null;
        if (!mounted.current) return;
        setBusyProposalId(null);
        setLastDecision({ id, title: target.title, decision });
      },
      (reason: unknown) => {
        busyRef.current = null;
        if (!mounted.current) return;
        setBusyProposalId(null);
        setDecisionError(reason instanceof Error ? reason.message : "Proposal decision failed.");
        void refresh();
      },
    );
  };

  const undo = (id: string) => {
    const target = proposals.find((proposal) => proposal.id === id);
    if (!ready || !target || target.status === "pending" || busyRef.current) return;
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

    busyRef.current = id;
    setBusyProposalId(id);
    setDecisionError("");
    void undoLocalProposalDecision(id).then(
      async () => {
        notifyCalendarDataChanged();
        await refresh();
        busyRef.current = null;
        if (!mounted.current) return;
        setBusyProposalId(null);
        setLastDecision(null);
      },
      (reason: unknown) => {
        busyRef.current = null;
        if (!mounted.current) return;
        setBusyProposalId(null);
        setDecisionError(reason instanceof Error ? reason.message : "Proposal undo failed.");
        void refresh();
      },
    );
  };

  const resolveTaskConflict = async (conflict: TaskConflict, choiceId: string) => {
    if (busyRef.current || !ready || error) return;
    busyRef.current = conflict.taskId;
    setBusyProposalId(conflict.taskId);
    setDecisionError("");
    ++requestVersion.current;
    try {
      const confirmed = await recordTaskResolution({
        taskId: conflict.taskId,
        reviewToken: conflict.reviewToken,
        choiceId,
      });
      if (!mounted.current) return;
      setTaskConflicts((current) => current.filter((item) => item.taskId !== conflict.taskId));
      setTaskConflictHistory((current) =>
        [confirmed, ...current.filter((item) => item.reviewToken !== confirmed.reviewToken)].slice(
          0,
          50,
        ),
      );
      notifySleepDataChanged();
      await refresh();
    } catch (reason) {
      if (mounted.current) {
        setDecisionError(
          reason instanceof Error ? reason.message : "The task review could not be confirmed.",
        );
        await refresh();
      }
    } finally {
      busyRef.current = null;
      if (mounted.current) setBusyProposalId(null);
    }
  };

  const undoLast = () => {
    if (lastDecision) undo(lastDecision.id);
  };

  const dismiss = useCallback(() => setLastDecision(null), []);
  const dismissError = useCallback(() => setDecisionError(""), []);
  const pending = proposals.filter((proposal) => proposal.status === "pending");
  const decided = proposals.filter((proposal) => proposal.status !== "pending");
  const value: ApprovalsContextValue = {
    proposals,
    pending,
    decided,
    taskConflicts,
    taskConflictHistory,
    resolveTaskConflict,
    pendingCount: pending.length + taskConflicts.length,
    unplaced,
    source,
    decide,
    undo,
    undoLast,
    lastDecision,
    busyProposalId,
    error: decisionError || error,
    loadError: error,
    ready,
    dismissError,
    refresh,
  };

  return (
    <PendingApprovalsCountContext.Provider value={pending.length + taskConflicts.length}>
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
    </PendingApprovalsCountContext.Provider>
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

// eslint-disable-next-line react-refresh/only-export-components
export function useLocalPendingApprovalsCount(): number {
  const context = useContext(PendingApprovalsCountContext);
  if (context === null) {
    throw new Error("useLocalPendingApprovalsCount must be used within ApprovalsProvider");
  }
  return context;
}
