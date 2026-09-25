import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { useApprovals } from "./approvals";
import { useBackendProposals } from "./backendProposals";
import { useVisitorRequests } from "./visitorRequests";
import { subscribeProjectionRefresh } from "../utils/projectionRefresh";
import { reviewQueueChangedEvent } from "../data/reviewQueue";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";

interface QueueBreakdown {
  /** Planner suggestions for your own tasks. */
  suggestions: number;
  /** Tasks edited on two devices, waiting for you to pick a version. */
  conflicts: number;
  /** Proposals from the assistant or a connected agent. */
  assistant: number;
  /** Time requests from people you share with. */
  requests: number;
}

interface QueueSummary {
  pendingCount: number;
  breakdown: QueueBreakdown;
  ready: boolean;
  incomplete: boolean;
  now: number;
}
const ApprovalQueueContext = createContext<QueueSummary | null>(null);
const PendingCountContext = createContext<number | null>(null);

export function ApprovalQueueProvider({ children }: { children: ReactNode }) {
  const local = useApprovals();
  const backend = useBackendProposals();
  const visitor = useVisitorRequests();
  const [now, setNow] = useState(Date.now);
  const backendRefresh = backend.refresh;
  const visitorRefresh = visitor.refresh;

  useEffect(
    () =>
      subscribeProjectionRefresh(() => {
        backendRefresh(true);
        visitorRefresh();
        setNow(Date.now());
      }, sleepDataChangedEvent),
    [backendRefresh, visitorRefresh],
  );
  useEffect(() => {
    const refresh = () => {
      backendRefresh(true);
      visitorRefresh();
    };
    window.addEventListener(reviewQueueChangedEvent, refresh);
    return () => window.removeEventListener(reviewQueueChangedEvent, refresh);
  }, [backendRefresh, visitorRefresh]);
  useEffect(() => {
    const expiry = [backend.data.nextExpiryAt, visitor.data.nextExpiryAt]
      .filter(Boolean)
      .map(Date.parse)
      .filter(Number.isFinite);
    if (!expiry.length) return;
    // If an expired request cannot be refreshed, use bounded retry instead of
    // repeatedly scheduling a timer for the already-passed deadline.
    const delay = Math.min(...expiry) - Date.now();
    const timer = window.setTimeout(
      () => {
        setNow(Date.now());
        backendRefresh(true);
        visitorRefresh();
      },
      delay > 0 ? Math.min(delay + 1, 2_147_483_647) : 30_000,
    );
    return () => window.clearTimeout(timer);
  }, [backend.data, visitor.data, backendRefresh, visitorRefresh]);

  const pendingCount = local.pendingCount + backend.data.pendingCount + visitor.data.pendingCount;
  const value = {
    pendingCount,
    breakdown: {
      suggestions: Math.max(0, local.pendingCount - local.taskConflicts.length),
      conflicts: local.taskConflicts.length,
      assistant: backend.data.pendingCount,
      requests: visitor.data.pendingCount,
    },
    ready: local.ready && backend.ready && visitor.ready,
    incomplete:
      Boolean(local.loadError) ||
      backend.data.status === "error" ||
      visitor.data.status === "error",
    now,
  };
  return (
    <PendingCountContext.Provider value={pendingCount}>
      <ApprovalQueueContext.Provider value={value}>{children}</ApprovalQueueContext.Provider>
    </PendingCountContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useApprovalQueue() {
  const value = useContext(ApprovalQueueContext);
  if (!value) throw new Error("useApprovalQueue requires ApprovalQueueProvider");
  return value;
}

// eslint-disable-next-line react-refresh/only-export-components
export function usePendingApprovalsCount() {
  const value = useContext(PendingCountContext);
  if (value === null) throw new Error("usePendingApprovalsCount requires ApprovalQueueProvider");
  return value;
}
