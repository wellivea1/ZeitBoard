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
  agentProposalsChangedEvent,
  decideAgentProposal,
  loadAgentProposals,
  noAgentProposals,
  proposalAction,
  proposalSubject,
  type AgentProposal,
  type AgentProposalsData,
} from "../data/agentProposals";
import { proposalDecisionDone } from "../data/decisionWords";
import { medicationDataChangedEvent, notifyMedicationDataChanged } from "../data/medications";
import { notifySleepDataChanged } from "../data/sleepDataEvents";
import { createCoalescedRefresh, type CoalescedRefresh } from "../utils/coalescedRefresh";

interface AgentProposalsContextValue {
  data: AgentProposalsData;
  ready: boolean;
  /** The last decision's failure, until the next decision. */
  decisionError: string;
  busyProposalId: string | null;
  announcement: string;
  refresh: () => void;
  decide: (proposal: AgentProposal, decision: "approved" | "rejected") => Promise<void>;
}

const AgentProposalsContext = createContext<AgentProposalsContextValue | null>(null);

export function AgentProposalsProvider({ children }: { children: ReactNode }) {
  const [data, setData] = useState<AgentProposalsData>(noAgentProposals);
  const [ready, setReady] = useState(false);
  const [decisionError, setDecisionError] = useState("");
  const [busyProposalId, setBusyProposalId] = useState<string | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const busyRef = useRef<string | null>(null);
  const queueRef = useRef<CoalescedRefresh | null>(null);

  const ensureQueue = useCallback(() => {
    queueRef.current ??= createCoalescedRefresh(loadAgentProposals, (loaded) => {
      setData(loaded);
      setReady(true);
    });
    return queueRef.current;
  }, []);
  const refresh = useCallback(() => ensureQueue().request(), [ensureQueue]);

  useEffect(() => {
    const queue = ensureQueue();
    queue.request();
    // An agent's proposal, or a deleted or renamed medication, changes what
    // waits.
    const events = [agentProposalsChangedEvent, medicationDataChangedEvent];
    for (const event of events) window.addEventListener(event, queue.request);
    return () => {
      for (const event of events) window.removeEventListener(event, queue.request);
      queue.dispose();
      queueRef.current = null;
    };
  }, [ensureQueue]);

  const decide = useCallback(
    async (proposal: AgentProposal, decision: "approved" | "rejected") => {
      if (busyRef.current) return;
      busyRef.current = proposal.proposalId;
      setBusyProposalId(proposal.proposalId);
      setDecisionError("");
      try {
        const next = await decideAgentProposal(proposal.proposalId, decision);
        // A load that began before the decision must not bring it back.
        queueRef.current?.supersede();
        setData(next);
        setAnnouncement(
          proposalDecisionDone(decision, proposalAction(proposal).done, proposalSubject(proposal)),
        );
        if (decision === "approved") {
          // The new record shows where the owner's own entry would: a dose in
          // Medications, a task in Plan with a suggested time.
          if (proposal.kind === "dose") notifyMedicationDataChanged();
          else notifySleepDataChanged();
        }
      } catch (reason) {
        setDecisionError(
          reason instanceof Error && reason.message
            ? `Nothing was changed: ${reason.message}.`
            : "Nothing was changed. Try again.",
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
    <AgentProposalsContext.Provider
      value={{ data, ready, decisionError, busyProposalId, announcement, refresh, decide }}
    >
      {children}
    </AgentProposalsContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAgentProposals() {
  const value = useContext(AgentProposalsContext);
  if (!value) throw new Error("useAgentProposals requires AgentProposalsProvider");
  return value;
}
