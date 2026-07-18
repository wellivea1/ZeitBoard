import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { Icon } from "./Icon";
import {
  loadAssistantStatus,
  sendAssistantMessage,
  type AssistantReply,
  type AssistantStatus,
} from "../data/assistant";
import {
  decideBackendProposal,
  loadBackendProposals,
  type BackendProposal,
  type BackendProposalsData,
} from "../data/backendProposals";

// The §4 assistant rail: chat over the propose-only backend. Action cards are
// shortcuts to the same one-use-token queue decisions as the Approvals screen;
// nothing here applies a schedule change directly.

interface ChatMessage {
  id: number;
  role: "user" | "assistant";
  text: string;
  refusal?: boolean;
  proposals?: BackendProposal[];
}

const examplePrompts = [
  "What's my next good window?",
  "Find 90 minutes for paperwork before Friday",
  "When am I likely to fall asleep tonight?",
];

function backendLabel(status: AssistantStatus | undefined): { text: string; mode: string } {
  if (!status?.enabled) return { text: "Offline", mode: "off" };
  if (!status.configured) return { text: "Connected · no provider", mode: "connected" };
  return { text: `Connected: ${status.provider ?? "provider"}`, mode: "connected" };
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
  const decidable = proposal.status === "pending" && Boolean(proposal.decisionToken);
  const filled = { Low: 1, Medium: 2, High: 3 }[proposal.confidence];
  return (
    <article className="assistant-action-card" data-status={proposal.status}>
      <header>
        <strong>{proposal.title}</strong>
        <div
          className="confidence-meter"
          data-level={proposal.confidence.toLowerCase()}
          aria-hidden="true"
        >
          {[0, 1, 2].map((index) => (
            <span key={index} data-muted={index >= filled || undefined} />
          ))}
        </div>
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
        <a href="#/approvals">View in Approvals</a>
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
          <span className="task-chip">{proposal.status}</span>
        )}
      </footer>
    </article>
  );
}

export function AssistantRail({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [status, setStatus] = useState<AssistantStatus>();
  const [tab, setTab] = useState<"chat" | "queue">("chat");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [queue, setQueue] = useState<BackendProposalsData>({ status: "off", proposals: [] });
  const transcriptRef = useRef<HTMLDivElement>(null);
  const nextId = useRef(1);

  useEffect(() => {
    if (!open) return;
    let current = true;
    void loadAssistantStatus().then((loaded) => {
      if (current) setStatus(loaded);
    });
    void loadBackendProposals().then((loaded) => {
      if (current) setQueue(loaded);
    });
    return () => {
      current = false;
    };
  }, [open]);

  useEffect(() => {
    const transcript = transcriptRef.current;
    if (transcript) transcript.scrollTop = transcript.scrollHeight;
  }, [messages, busy]);

  const appendMessage = (message: Omit<ChatMessage, "id">) => {
    setMessages((existing) => [...existing, { ...message, id: nextId.current++ }]);
  };

  const applyReply = (reply: AssistantReply) => {
    appendMessage({
      role: "assistant",
      text: reply.answer,
      refusal: reply.result === "refused_medical",
      ...(reply.proposals.length > 0 ? { proposals: reply.proposals } : {}),
    });
    if (reply.available) {
      setStatus({
        enabled: true,
        configured: reply.configured,
        ...(reply.provider ? { provider: reply.provider } : {}),
        ...(reply.model ? { model: reply.model } : {}),
      });
    }
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
    setBusy(true);
    decideBackendProposal({
      proposalId: proposal.proposalId,
      decision,
      token: proposal.decisionToken,
    })
      .then((refreshed) => {
        setQueue(refreshed);
        setMessages((existing) =>
          existing.map((message) => ({
            ...message,
            proposals: message.proposals?.map((item) =>
              item.proposalId === proposal.proposalId
                ? { ...item, status: decision, decisionToken: undefined }
                : item,
            ),
          })),
        );
      })
      .finally(() => setBusy(false));
  };

  const backend = backendLabel(status);
  const pendingQueue = queue.proposals.filter((proposal) => proposal.status === "pending");

  if (!open) return null;

  return (
    <aside className="assistant-rail" aria-label="Assistant">
      <header className="assistant-header">
        <strong>Assistant</strong>
        <span className="assistant-backend" data-mode={backend.mode}>
          <i aria-hidden="true" /> {backend.text}
        </span>
        <a className="assistant-queue-link" href="#/approvals">
          Approvals{pendingQueue.length > 0 && ` ${pendingQueue.length}`}
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
            {messages.length === 0 && (
              <div className="assistant-empty">
                <p>
                  {status?.enabled
                    ? "Ask about your rhythm, or have the assistant propose schedule changes — every change waits for your approval."
                    : (status?.message ??
                      "Connect your self-hosted backend in Settings to start chatting.")}
                </p>
                {status?.enabled && (
                  <div className="assistant-examples">
                    {examplePrompts.map((prompt) => (
                      <button
                        className="task-chip"
                        type="button"
                        key={prompt}
                        onClick={() => send(prompt)}
                      >
                        {prompt}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}
            {messages.map((message) => (
              <div className={`assistant-message is-${message.role}`} key={message.id}>
                <div className="assistant-bubble" data-refusal={message.refusal || undefined}>
                  {message.text}
                </div>
                {message.proposals?.map((proposal) => (
                  <ActionCard
                    proposal={proposal}
                    busy={busy}
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
              busy={busy}
              onDecide={onDecide}
              key={proposal.proposalId}
            />
          ))}
        </div>
      )}
    </aside>
  );
}
