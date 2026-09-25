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
  decideBackendProposal,
  loadBackendProposalPage,
  loadBackendProposals,
  type BackendProposal,
  type BackendProposalsData,
} from "../data/backendProposals";
import { createCoalescedRefresh, type CoalescedRefresh } from "../utils/coalescedRefresh";

import { emptyReviewSummary, reviewIsPending } from "../data/reviewQueue";

const proposalFreshnessMs = 5_000;
const initialData: BackendProposalsData = {
  ...emptyReviewSummary,
  status: "off",
  proposals: [],
  pagination: { nextCursor: "", hasMore: false },
};

interface BackendProposalsContextValue {
  data: BackendProposalsData;
  ready: boolean;
  decisionError: string;
  loading: boolean;
  loadingOlder: boolean;
  loadOlderError: string;
  busyProposalId: string | null;
  refresh: (force?: boolean) => void;
  loadOlder: () => Promise<void>;
  ingest: (proposals: BackendProposal[]) => void;
  decide: (
    proposal: BackendProposal,
    decision: "approved" | "rejected",
  ) => Promise<BackendProposalsData>;
}

const BackendProposalsContext = createContext<BackendProposalsContextValue | null>(null);

function retainKnownProposals(
  current: BackendProposalsData,
  loaded: BackendProposalsData,
): BackendProposalsData {
  if (loaded.status !== "error") return loaded;
  return {
    ...loaded,
    proposals: current.proposals,
    pagination: current.pagination,
    pendingCount: current.pendingCount,
    nextExpiryAt: current.nextExpiryAt,
  };
}

function mergeOlderProposals(
  current: BackendProposal[],
  older: BackendProposal[],
): BackendProposal[] {
  const seen = new Set<string>();
  const merged: BackendProposal[] = [];
  for (const proposal of [...current, ...older]) {
    if (seen.has(proposal.proposalId)) continue;
    seen.add(proposal.proposalId);
    merged.push(proposal);
  }
  return merged;
}

function withRecordedDecision(
  loaded: BackendProposalsData,
  proposal: BackendProposal,
  decision: "approved" | "rejected",
): BackendProposalsData {
  if (loaded.status !== "ok" && !loaded.decisionRecorded) return loaded;
  const decided = { ...proposal, status: decision, decisionToken: undefined };
  const proposals = loaded.proposals.some((item) => item.proposalId === proposal.proposalId)
    ? loaded.proposals.map((item) => (item.proposalId === proposal.proposalId ? decided : item))
    : [...loaded.proposals, decided];
  return { ...loaded, proposals };
}

export function BackendProposalsProvider({ children }: { children: ReactNode }) {
  const [data, setData] = useState<BackendProposalsData>(initialData);
  const [ready, setReady] = useState(false);
  const [decisionError, setDecisionError] = useState("");
  const [loading, setLoading] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [loadOlderError, setLoadOlderError] = useState("");
  const [busyProposalId, setBusyProposalId] = useState<string | null>(null);
  const dataRef = useRef(data);
  const loadingRef = useRef(false);
  const loadingOlderRef = useRef(false);
  const busyRef = useRef<string | null>(null);
  const lastLoadedAtRef = useRef(0);
  const refreshQueueRef = useRef<CoalescedRefresh | null>(null);
  const olderRequestRef = useRef<Promise<void> | null>(null);
  const olderRequestTokenRef = useRef<object | null>(null);
  const queueGenerationRef = useRef(0);
  const activeRef = useRef(true);

  const publish = useCallback((next: BackendProposalsData) => {
    setReady(true);
    dataRef.current = next;
    setData(next);
  }, []);

  const invalidateOlderPages = useCallback(() => {
    queueGenerationRef.current += 1;
    setLoadOlderError("");
  }, []);
  const ensureRefreshQueue = useCallback(() => {
    if (!refreshQueueRef.current) {
      refreshQueueRef.current = createCoalescedRefresh(
        loadBackendProposals,
        (loaded) => {
          lastLoadedAtRef.current = Date.now();
          loadingRef.current = false;
          setLoading(false);
          publish(retainKnownProposals(dataRef.current, loaded));
        },
        () => {
          loadingRef.current = false;
          setLoading(false);
          publish({
            ...dataRef.current,
            status: "error",
            message: "Could not reach the synced backend.",
            proposals: dataRef.current.proposals,
            pagination: dataRef.current.pagination,
          });
        },
      );
    }
    return refreshQueueRef.current;
  }, [publish]);

  useEffect(() => {
    activeRef.current = true;
    return () => {
      activeRef.current = false;
      queueGenerationRef.current += 1;
      refreshQueueRef.current?.dispose();
      refreshQueueRef.current = null;
      olderRequestRef.current = null;
      olderRequestTokenRef.current = null;
      loadingOlderRef.current = false;
      loadingRef.current = false;
    };
  }, []);

  const refresh = useCallback(
    (force = false) => {
      if (
        busyRef.current ||
        (!force && loadingRef.current) ||
        (!force &&
          lastLoadedAtRef.current > 0 &&
          Date.now() - lastLoadedAtRef.current < proposalFreshnessMs)
      ) {
        return;
      }
      invalidateOlderPages();
      loadingRef.current = true;
      setLoading(true);
      ensureRefreshQueue().request();
    },
    [ensureRefreshQueue, invalidateOlderPages],
  );

  const ingest = useCallback(
    (proposals: BackendProposal[]) => {
      if (proposals.length === 0) return;
      invalidateOlderPages();
      ensureRefreshQueue().supersede();
      loadingRef.current = false;
      setLoading(false);
      lastLoadedAtRef.current = Date.now();
      const current = dataRef.current;
      const byID = new Map(current.proposals.map((proposal) => [proposal.proposalId, proposal]));
      for (const proposal of proposals) byID.set(proposal.proposalId, proposal);
      publish({
        ...current,
        status: "ok",
        proposals: [...byID.values()],
        pendingCount:
          current.pendingCount +
          proposals.filter(
            (item) =>
              reviewIsPending(item) &&
              !current.proposals.some((known) => known.proposalId === item.proposalId),
          ).length,
        nextExpiryAt:
          [
            current.nextExpiryAt,
            ...proposals.filter((item) => reviewIsPending(item)).map((item) => item.expiresAt),
          ]
            .filter(Boolean)
            .sort((a, b) => Date.parse(a) - Date.parse(b))[0] ?? "",
      });
    },
    [ensureRefreshQueue, invalidateOlderPages, publish],
  );

  const loadOlder = useCallback((): Promise<void> => {
    if (loadingOlderRef.current) {
      return olderRequestRef.current ?? Promise.resolve();
    }

    const current = dataRef.current;
    const cursor = current.pagination.nextCursor;
    if (
      current.status !== "ok" ||
      !current.pagination.hasMore ||
      cursor.length === 0 ||
      loadingRef.current ||
      busyRef.current
    ) {
      return Promise.resolve();
    }

    const generation = queueGenerationRef.current;
    const requestToken = {};
    const isCurrentRequest = () =>
      activeRef.current &&
      queueGenerationRef.current === generation &&
      dataRef.current.pagination.nextCursor === cursor;

    loadingOlderRef.current = true;
    olderRequestTokenRef.current = requestToken;
    setLoadingOlder(true);
    setLoadOlderError("");

    const request = loadBackendProposalPage(cursor)
      .then((loaded) => {
        if (!isCurrentRequest()) return;
        if (loaded.status !== "ok") {
          setLoadOlderError(loaded.message ?? "Could not load older synced proposals.");
          return;
        }
        if (loaded.pagination.hasMore && loaded.pagination.nextCursor === cursor) {
          setLoadOlderError("The backend returned a pagination cursor that did not advance.");
          return;
        }

        const latest = dataRef.current;
        publish({
          ...loaded,
          proposals: mergeOlderProposals(latest.proposals, loaded.proposals),
          pagination: loaded.pagination,
        });
      })
      .catch((reason: unknown) => {
        if (!isCurrentRequest()) return;
        setLoadOlderError(
          reason instanceof Error ? reason.message : "Could not load older synced proposals.",
        );
      })
      .finally(() => {
        if (olderRequestTokenRef.current !== requestToken) return;
        olderRequestTokenRef.current = null;
        olderRequestRef.current = null;
        loadingOlderRef.current = false;
        if (activeRef.current) setLoadingOlder(false);
      });

    olderRequestRef.current = request;
    return request;
  }, [publish]);

  const decide = useCallback(
    async (
      proposal: BackendProposal,
      decision: "approved" | "rejected",
    ): Promise<BackendProposalsData> => {
      if (dataRef.current.status !== "ok" || !reviewIsPending(proposal))
        return {
          ...dataRef.current,
          status: "error",
          message: "This proposal expired or cannot be decided. Refresh the queue.",
        };
      if (busyRef.current) {
        return {
          ...dataRef.current,
          status: "error",
          message: "Another proposal decision is already in progress.",
          proposals: dataRef.current.proposals,
          pagination: dataRef.current.pagination,
        };
      }
      invalidateOlderPages();
      setDecisionError("");
      busyRef.current = proposal.proposalId;
      setBusyProposalId(proposal.proposalId);
      ensureRefreshQueue().supersede();
      loadingRef.current = false;
      setLoading(false);
      try {
        const loaded = await decideBackendProposal({
          proposalId: proposal.proposalId,
          decision,
          token: proposal.decisionToken!,
        });
        if (loaded.status === "error")
          setDecisionError(loaded.message ?? "The decision could not be recorded.");
        const retained =
          loaded.status === "error" ? retainKnownProposals(dataRef.current, loaded) : loaded;
        if (loaded.decisionRecorded && loaded.status === "error")
          retained.pendingCount = Math.max(0, retained.pendingCount - 1);
        const next = withRecordedDecision(retained, proposal, decision);
        lastLoadedAtRef.current = Date.now();
        publish(next);
        return next;
      } finally {
        busyRef.current = null;
        setBusyProposalId(null);
      }
    },
    [ensureRefreshQueue, invalidateOlderPages, publish],
  );

  return (
    <BackendProposalsContext.Provider
      value={{
        data,
        ready,
        decisionError,
        loading,
        loadingOlder,
        loadOlderError,
        busyProposalId,
        refresh,
        loadOlder,
        ingest,
        decide,
      }}
    >
      {children}
    </BackendProposalsContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useBackendProposals(): BackendProposalsContextValue {
  const context = useContext(BackendProposalsContext);
  if (!context) {
    throw new Error("useBackendProposals must be used within BackendProposalsProvider");
  }
  return context;
}
