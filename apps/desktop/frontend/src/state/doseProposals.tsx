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
  decideDoseProposal,
  doseProposalsChangedEvent,
  loadDoseProposals,
  noDoseProposals,
  proposedDoseMedication,
  type DoseProposal,
  type DoseProposalsData,
} from "../data/doseProposals";
import { doseDecisionDone } from "../data/decisionWords";
import { medicationDataChangedEvent, notifyMedicationDataChanged } from "../data/medications";
import { createCoalescedRefresh, type CoalescedRefresh } from "../utils/coalescedRefresh";

interface DoseProposalsContextValue {
  data: DoseProposalsData;
  ready: boolean;
  /** The last decision's failure, until the next decision. */
  decisionError: string;
  busyProposalId: string | null;
  announcement: string;
  refresh: () => void;
  decide: (proposal: DoseProposal, decision: "approved" | "rejected") => Promise<void>;
}

const DoseProposalsContext = createContext<DoseProposalsContextValue | null>(null);

export function DoseProposalsProvider({ children }: { children: ReactNode }) {
  const [data, setData] = useState<DoseProposalsData>(noDoseProposals);
  const [ready, setReady] = useState(false);
  const [decisionError, setDecisionError] = useState("");
  const [busyProposalId, setBusyProposalId] = useState<string | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const busyRef = useRef<string | null>(null);
  const queueRef = useRef<CoalescedRefresh | null>(null);

  const ensureQueue = useCallback(() => {
    queueRef.current ??= createCoalescedRefresh(loadDoseProposals, (loaded) => {
      setData(loaded);
      setReady(true);
    });
    return queueRef.current;
  }, []);
  const refresh = useCallback(() => ensureQueue().request(), [ensureQueue]);

  useEffect(() => {
    const queue = ensureQueue();
    queue.request();
    // An agent's proposal, a deleted or renamed medication, or a sync can each
    // change what waits.
    const events = [doseProposalsChangedEvent, medicationDataChangedEvent];
    for (const event of events) window.addEventListener(event, queue.request);
    return () => {
      for (const event of events) window.removeEventListener(event, queue.request);
      queue.dispose();
      queueRef.current = null;
    };
  }, [ensureQueue]);

  const decide = useCallback(
    async (proposal: DoseProposal, decision: "approved" | "rejected") => {
      if (busyRef.current) return;
      busyRef.current = proposal.proposalId;
      setBusyProposalId(proposal.proposalId);
      setDecisionError("");
      try {
        const next = await decideDoseProposal(proposal.proposalId, decision);
        // A load that began before the decision must not bring it back.
        queueRef.current?.supersede();
        setData(next);
        setAnnouncement(doseDecisionDone(decision, proposedDoseMedication(proposal)));
        if (decision === "approved") notifyMedicationDataChanged();
      } catch (reason) {
        setDecisionError(
          reason instanceof Error && reason.message
            ? `The dose was not recorded: ${reason.message}.`
            : "The dose was not recorded. Try again.",
        );
        refresh();
      } finally {
        busyRef.current = null;
        setBusyProposalId(null);
      }
    },
    [refresh],
  );

  return (
    <DoseProposalsContext.Provider
      value={{ data, ready, decisionError, busyProposalId, announcement, refresh, decide }}
    >
      {children}
    </DoseProposalsContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useDoseProposals() {
  const value = useContext(DoseProposalsContext);
  if (!value) throw new Error("useDoseProposals requires DoseProposalsProvider");
  return value;
}
