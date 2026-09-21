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
  decideVisitorRequest,
  defaultSlot,
  emptyVisitorRequests,
  loadVisitorRequestPage,
  loadVisitorRequests,
  type VisitorRequest,
  type VisitorRequestsData,
} from "../data/visitorRequests";
import { notifyCalendarDataChanged } from "../data/calendar";
import { reviewIsPending } from "../data/reviewQueue";

import { selectedCivilInstant } from "../utils/civilTime";

export type VisitorSlot = { start: string; end: string; startAt?: string; endAt?: string };

interface VisitorRequestsContextValue {
  data: VisitorRequestsData;
  ready: boolean;
  loading: boolean;
  loadingOlder: boolean;
  loadOlderError: string;
  decisionError: string;
  announcement: string;
  busyId: string | null;
  drafts: Record<string, VisitorSlot>;
  setDraft: (request: VisitorRequest, changes: Partial<VisitorSlot>) => void;
  refresh: () => void;
  loadOlder: () => Promise<void>;
  decide: (
    request: VisitorRequest,
    decision: "approved" | "rejected",
    slot: VisitorSlot,
  ) => Promise<void>;
}

const VisitorRequestsContext = createContext<VisitorRequestsContextValue | null>(null);

export function VisitorRequestsProvider({ children }: { children: ReactNode }) {
  const [data, setData] = useState(emptyVisitorRequests);
  const [ready, setReady] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [loadOlderError, setLoadOlderError] = useState("");
  const [decisionError, setDecisionError] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const [busyId, setBusyId] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<string, VisitorSlot>>({});
  const dataRef = useRef(data);
  const active = useRef(true);
  const generation = useRef(0);
  const busy = useRef(false);
  const loadingRef = useRef(false);
  const older = useRef(false);

  useEffect(() => {
    active.current = true;
    const requestGeneration = generation;
    return () => {
      active.current = false;
      requestGeneration.current++;
      loadingRef.current = false;
    };
  }, []);

  const publish = useCallback((loaded: VisitorRequestsData) => {
    if (!active.current) return;
    const next =
      loaded.status === "error"
        ? { ...dataRef.current, status: "error" as const, message: loaded.message }
        : loaded;
    dataRef.current = next;
    setData(next);
    setReady(true);
    if (next.status === "off") setDrafts({});
    else
      setDrafts((current) =>
        Object.fromEntries(
          Object.entries(current).filter(
            ([id]) => !next.requests.some((r) => r.proposalId === id && r.status !== "pending"),
          ),
        ),
      );
  }, []);

  const refresh = useCallback(() => {
    if (busy.current) return;
    const version = ++generation.current;
    loadingRef.current = true;
    setLoading(true);
    setLoadOlderError("");
    void loadVisitorRequests()
      .then((loaded) => {
        if (active.current && version === generation.current) publish(loaded);
      })
      .finally(() => {
        if (version === generation.current) {
          loadingRef.current = false;
          if (active.current) setLoading(false);
        }
      });
  }, [publish]);

  const loadOlder = useCallback(async () => {
    const current = dataRef.current;
    if (
      older.current ||
      loadingRef.current ||
      busy.current ||
      current.status !== "ok" ||
      !current.pagination.hasMore
    )
      return;
    older.current = true;
    setLoadingOlder(true);
    setLoadOlderError("");
    const version = generation.current;
    try {
      const page = await loadVisitorRequestPage(current.pagination.nextCursor);
      if (!active.current || version !== generation.current) return;
      if (page.status !== "ok") {
        setLoadOlderError(page.message ?? "Could not load more time requests.");
        return;
      }
      if (page.pagination.hasMore && page.pagination.nextCursor === current.pagination.nextCursor) {
        setLoadOlderError("The backend returned a pagination cursor that did not advance.");
        return;
      }
      const byId = new Map(
        [...dataRef.current.requests, ...page.requests].map((r) => [r.proposalId, r]),
      );
      publish({ ...page, requests: [...byId.values()] });
    } finally {
      older.current = false;
      if (active.current) setLoadingOlder(false);
    }
  }, [publish]);

  const setDraft = useCallback((request: VisitorRequest, changes: Partial<VisitorSlot>) => {
    setDrafts((current) => ({
      ...current,
      [request.proposalId]: {
        ...(current[request.proposalId] ?? defaultSlot(request)),
        ...changes,
      },
    }));
  }, []);

  const decide = useCallback(
    async (request: VisitorRequest, decision: "approved" | "rejected", slot: VisitorSlot) => {
      if (busy.current || !reviewIsPending(request) || dataRef.current.status !== "ok") return;
      const startAt = selectedCivilInstant(slot.start, slot.startAt);
      const endAt = selectedCivilInstant(slot.end, slot.endAt);
      if (
        decision === "approved" &&
        (!startAt || !endAt || Date.parse(endAt) <= Date.parse(startAt))
      ) {
        setDecisionError("Choose valid block times and an occurrence where the clock repeats.");
        return;
      }
      busy.current = true;
      setBusyId(request.proposalId);
      setDecisionError("");
      ++generation.current;
      loadingRef.current = false;
      setLoading(false);
      try {
        const result = await decideVisitorRequest({
          proposalId: request.proposalId,
          decision,
          token: request.decisionToken!,
          ...(decision === "approved" ? { startAt, endAt } : {}),
        });
        if (!active.current) return;
        if (result.decisionRecorded && result.status !== "ok") {
          const retained = {
            ...dataRef.current,
            status: "error" as const,
            message: result.message,
            pendingCount: Math.max(0, dataRef.current.pendingCount - 1),
            requests: dataRef.current.requests.map((item) =>
              item.proposalId === request.proposalId
                ? { ...item, status: decision, decisionToken: undefined }
                : item,
            ),
          };
          dataRef.current = retained;
          setData(retained);
          setDecisionError(result.message ?? "Decision recorded; refresh the queue.");
          if (decision === "approved") notifyCalendarDataChanged();
        } else if (result.status !== "ok") {
          setDecisionError(result.message ?? "The request decision could not be recorded.");
          publish(
            result.status === "off"
              ? { ...result, status: "error", message: "The request service is unavailable." }
              : result,
          );
        } else {
          publish(result);
          setAnnouncement(
            decision === "approved"
              ? "Accepted. They will see the exact time you chose."
              : "Declined. They are told only that the time did not work.",
          );
          if (decision === "approved") notifyCalendarDataChanged();
        }
      } finally {
        busy.current = false;
        if (active.current) setBusyId(null);
      }
    },
    [publish],
  );

  return (
    <VisitorRequestsContext.Provider
      value={{
        data,
        ready,
        loading,
        loadingOlder,
        loadOlderError,
        decisionError,
        announcement,
        busyId,
        drafts,
        setDraft,
        refresh,
        loadOlder,
        decide,
      }}
    >
      {children}
    </VisitorRequestsContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useVisitorRequests() {
  const value = useContext(VisitorRequestsContext);
  if (!value) throw new Error("useVisitorRequests requires VisitorRequestsProvider");
  return value;
}
