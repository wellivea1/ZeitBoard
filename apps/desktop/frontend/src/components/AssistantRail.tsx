import { reviewIsPending, reviewStatus } from "../data/reviewQueue";
import { usePendingApprovalsCount } from "../state/approvalQueue";
import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { Icon } from "./Icon";
import {
  assistantConnectionChangedEvent,
  loadAssistantStatus,
  openAssistantPage,
  sendAssistantMessage,
  usesChatGPTPlan,
  type AssistantReason,
  type AssistantReply,
  type AssistantStatus,
} from "../data/assistant";
import type { BackendProposal } from "../data/backendProposals";
import { useBackendProposals } from "../state/backendProposals";

// The §4 assistant rail: chat over the propose-only backend. Action cards are
// shortcuts to the same one-use-token queue decisions as the Approvals screen;
// nothing here applies a schedule change directly.

interface ChatMessage {
  id: number;
  role: "user" | "assistant";
  text: string;
  refusal?: boolean;
  /** Why the assistant could not answer, when the owner can act on it. */
  reason?: AssistantReason;
  proposals?: BackendProposal[];
}

const examplePrompts = [
  "What's my next good window?",
  "Find 90 minutes for paperwork before Friday",
  "When am I likely to fall asleep tonight?",
];

function backendLabel(status: AssistantStatus | undefined): { text: string; mode: string } {
  if (!status?.enabled) return { text: "Offline", mode: "off" };
  if (usesChatGPTPlan(status)) {
    if (status.configured) return { text: "ChatGPT plan", mode: "connected" };
    if (status.problem === "sign_in_ended")
      return { text: "ChatGPT · sign in again", mode: "connected" };
    if (status.problem === "model_needed")
      return { text: "ChatGPT · choose a model", mode: "connected" };
    return { text: "ChatGPT · not signed in", mode: "connected" };
  }
  if (!status.configured) return { text: "Connected · no provider", mode: "connected" };
  return { text: `Connected: ${status.provider ?? "provider"}`, mode: "connected" };
}

/**
 * What the owner can do about a reply that says why the assistant could not
 * answer. The answer itself already says what happened.
 */
function ReasonAction({ reason, plan }: { reason: AssistantReason | undefined; plan: boolean }) {
  const action = reasonAction(reason, plan);
  return action ? <div className="assistant-reason-actions">{action}</div> : null;
}

function reasonAction(reason: AssistantReason | undefined, plan: boolean): ReactNode {
  switch (reason) {
    case "usage_limit":
      // A key's limit is the operator's to raise; only the plan's has a page.
      return plan ? (
        <button
          className="button primary compact"
          type="button"
          onClick={() => void openAssistantPage("usage")}
        >
          Manage usage
        </button>
      ) : null;
    case "sign_in_needed":
    case "sign_in_ended":
    case "plan_not_shared":
      return (
        <a className="button secondary compact" href="#/settings/sync">
          Sign in under Settings
        </a>
      );
    case "model_needed":
    case "model_unavailable":
      return (
        <a className="button secondary compact" href="#/settings/sync">
          Choose a model
        </a>
      );
    case "not_eligible":
      return (
        <button
          className="button ghost compact"
          type="button"
          onClick={() => void openAssistantPage("help")}
        >
          Learn more
        </button>
      );
    default:
      return null;
  }
}

function ActionCard({
  proposal,
  busy,
  onDecide,
}: {
  proposal: BackendProposal;
  busy: boolean;
  onDecide: (proposal: BackendProposal, decision: "approved" | "rejected") => void;
}) {
  const decidable = reviewIsPending(proposal);
  return (
    <article className="assistant-action-card" data-status={proposal.status}>
      <header>
        <strong>{proposal.title}</strong>
      </header>
      <p>{proposal.window}</p>
      {proposal.reasonLabels.length > 0 && (
        <div className="proposal-reasons">
          {proposal.reasonLabels.map((reason) => (
            <span className="task-chip" key={reason}>
              {reason}
            </span>
          ))}
        </div>
      )}
      <footer>
        <a href="#/plan/tasks">View in Plan</a>
        {decidable ? (
          <>
            <button
              className="button secondary"
              type="button"
              disabled={busy}
              onClick={() => onDecide(proposal, "rejected")}
            >
              Reject
            </button>
            <button
              className="button primary"
              type="button"
              disabled={busy}
              onClick={() => onDecide(proposal, "approved")}
            >
              Approve
            </button>
          </>
        ) : (
          <span className="task-chip">{reviewStatus(proposal)}</span>
        )}
      </footer>
    </article>
  );
}

/** The chat before its first message: what it can do, or what it needs. */
function ChatEmpty({
  status,
  onExample,
}: {
  status: AssistantStatus | undefined;
  onExample: (prompt: string) => void;
}) {
  let text: ReactNode;
  if (!status?.enabled) {
    text = status?.message ?? "Connect your self-hosted backend in Settings to start chatting.";
  } else if (usesChatGPTPlan(status) && !status.configured) {
    text = (
      <>
        Sign the assistant in to your ChatGPT plan under{" "}
        <a href="#/settings/sync">Settings › Sync</a>.
      </>
    );
  } else {
    text =
      "Ask about your rhythm, or have the assistant propose schedule changes — every change waits for your approval.";
  }
  return (
    <div className="assistant-empty">
      <p>{text}</p>
      {/* Examples only where they can be answered. */}
      {status?.enabled && !(usesChatGPTPlan(status) && !status.configured) && (
        <div className="assistant-examples">
          {examplePrompts.map((prompt) => (
            <button
              className="task-chip"
              type="button"
              key={prompt}
              onClick={() => onExample(prompt)}
            >
              {prompt}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

export function AssistantRail({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [status, setStatus] = useState<AssistantStatus>();
  const [tab, setTab] = useState<"chat" | "queue">("chat");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const {
    data: queue,
    busyProposalId,
    refresh: refreshProposals,
    ingest: ingestProposals,
    decide: decideProposal,
  } = useBackendProposals();
  const transcriptRef = useRef<HTMLDivElement>(null);
  const nextId = useRef(1);

  useEffect(() => {
    if (!open) return;
    let current = true;
    const readStatus = () =>
      void loadAssistantStatus().then((loaded) => {
        if (current) setStatus(loaded);
      });
    readStatus();
    refreshProposals();
    // Signing in or out in Settings while the rail is open shows here.
    window.addEventListener(assistantConnectionChangedEvent, readStatus);
    return () => {
      current = false;
      window.removeEventListener(assistantConnectionChangedEvent, readStatus);
    };
  }, [open, refreshProposals]);

  useEffect(() => {
    const transcript = transcriptRef.current;
    if (transcript) transcript.scrollTop = transcript.scrollHeight;
  }, [messages, busy]);

  const appendMessage = (message: Omit<ChatMessage, "id">) => {
    setMessages((existing) => [...existing, { ...message, id: nextId.current++ }]);
  };

  const applyReply = (reply: AssistantReply) => {
    ingestProposals(reply.proposals);
    appendMessage({
      role: "assistant",
      text: reply.answer,
      refusal: reply.result === "refused_medical",
      ...(reply.reason ? { reason: reply.reason } : {}),
      ...(reply.proposals.length > 0 ? { proposals: reply.proposals } : {}),
    });
    if (reply.available) {
      // A reply does not say what a sign-in still needs; that is kept until
      // the status is read again.
      setStatus((previous) => ({
        enabled: true,
        configured: reply.configured,
        ...(reply.provider ? { provider: reply.provider } : {}),
        ...(reply.model ? { model: reply.model } : {}),
        ...(reply.auth ? { auth: reply.auth } : {}),
        ...(!reply.configured && previous?.problem ? { problem: previous.problem } : {}),
      }));
    }
    // A sign-in that ended or a model gone changes what the header says.
    if (reply.reason) void loadAssistantStatus().then(setStatus);
  };

  const send = (text: string) => {
    const message = text.trim();
    if (!message || busy) return;
    appendMessage({ role: "user", text: message });
    setDraft("");
    setBusy(true);
    sendAssistantMessage(message)
      .then(applyReply)
      .catch((error: unknown) => {
        appendMessage({
          role: "assistant",
          text:
            error instanceof Error ? error.message : "Something went wrong. Nothing was changed.",
        });
      })
      .finally(() => setBusy(false));
  };

  const onComposerKey = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      send(draft);
    }
  };

  const onDecide = (proposal: BackendProposal, decision: "approved" | "rejected") => {
    if (!proposal.decisionToken) return;
    void decideProposal(proposal, decision);
  };

  const totalPending = usePendingApprovalsCount();
  const backend = backendLabel(status);
  const plan = usesChatGPTPlan(status);
  const pendingQueue = queue.proposals.filter((proposal) => reviewIsPending(proposal));
  const currentProposal = new Map(
    queue.proposals.map((proposal) => [proposal.proposalId, proposal]),
  );

  if (!open) return null;

  return (
    <aside className="assistant-rail" aria-label="Assistant">
      <header className="assistant-header">
        <strong>Assistant</strong>
        <span className="assistant-backend" data-mode={backend.mode}>
          <i aria-hidden="true" /> {backend.text}
        </span>
        <a className="assistant-queue-link" href="#/plan/tasks">
          Decisions{totalPending > 0 && ` ${totalPending}`}
        </a>
        <button
          className="icon-button"
          type="button"
          aria-label="Close assistant"
          onClick={onClose}
        >
          <Icon name="close" />
        </button>
      </header>

      <div className="assistant-tabs" role="tablist" aria-label="Assistant views">
        {(["chat", "queue"] as const).map((id) => (
          <button
            key={id}
            className={`filter${tab === id ? " active" : ""}`}
            type="button"
            role="tab"
            aria-selected={tab === id}
            onClick={() => setTab(id)}
          >
            {id === "chat" ? "Chat" : "Queue"}
          </button>
        ))}
      </div>

      {tab === "chat" ? (
        <>
          <div className="assistant-transcript" ref={transcriptRef} aria-live="polite">
            {messages.length === 0 && <ChatEmpty status={status} onExample={send} />}
            {messages.map((message) => (
              <div className={`assistant-message is-${message.role}`} key={message.id}>
                <div
                  className="assistant-bubble"
                  data-refusal={message.refusal || undefined}
                  data-reason={message.reason}
                >
                  {message.text}
                </div>
                <ReasonAction reason={message.reason} plan={plan} />
                {message.proposals?.map((proposal) => (
                  <ActionCard
                    proposal={currentProposal.get(proposal.proposalId) ?? proposal}
                    busy={busy || busyProposalId !== null}
                    onDecide={onDecide}
                    key={proposal.proposalId}
                  />
                ))}
              </div>
            ))}
            {busy && (
              <div className="assistant-message is-assistant">
                <div
                  className="assistant-bubble assistant-typing"
                  aria-label="Assistant is responding"
                >
                  <i />
                  <i />
                  <i />
                </div>
              </div>
            )}
          </div>
          <div className="assistant-composer">
            <textarea
              value={draft}
              rows={1}
              maxLength={2000}
              placeholder={status?.enabled ? "Message the assistant" : "Assistant offline"}
              disabled={busy || !status?.enabled}
              aria-label="Message the assistant"
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={onComposerKey}
            />
            <button
              className="button primary assistant-send"
              type="button"
              disabled={busy || !status?.enabled || !draft.trim()}
              onClick={() => send(draft)}
            >
              Send
            </button>
          </div>
          {plan && status?.configured && (
            <p className="assistant-plan-line">
              Using your ChatGPT plan ·{" "}
              <button type="button" onClick={() => void openAssistantPage("usage")}>
                Manage usage
              </button>
            </p>
          )}
          <p className="assistant-disclaimer">
            Manages your schedule via approvals. Not medical advice.
          </p>
        </>
      ) : (
        <div className="assistant-transcript assistant-queue" role="tabpanel">
          {queue.status === "off" && (
            <p className="assistant-empty">Turn on backend sync to see synced proposals here.</p>
          )}
          {queue.status === "error" && (
            <p className="assistant-empty">{queue.message ?? "The queue is unreachable."}</p>
          )}
          {queue.status === "ok" && pendingQueue.length === 0 && (
            <p className="assistant-empty">Nothing waiting for approval.</p>
          )}
          {pendingQueue.map((proposal) => (
            <ActionCard
              proposal={proposal}
              busy={busy || busyProposalId !== null}
              onDecide={onDecide}
              key={proposal.proposalId}
            />
          ))}
        </div>
      )}
    </aside>
  );
}
