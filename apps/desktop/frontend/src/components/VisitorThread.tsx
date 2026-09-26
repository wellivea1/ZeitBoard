import { useId, useState } from "react";
import { ConfirmDelete } from "./ConfirmDelete";
import type { VisitorRequest } from "../data/visitorRequests";
import { useVisitorRequests } from "../state/visitorRequests";

// A request's thread (P5-c): short plain-text messages between the visitor and
// the owner about this one request. It is open until the request is answered,
// readable for two weeks after, and the owner can delete it sooner.
export function VisitorThread({ request }: { request: VisitorRequest }) {
  const queue = useVisitorRequests();
  const [draft, setDraft] = useState("");
  const [confirming, setConfirming] = useState(false);
  const replyID = useId();
  if (request.messages.length === 0 && !request.canMessage) return null;

  const who = request.handle ?? "them";
  const busy = queue.busyId !== null;
  return (
    <section className="visitor-thread" aria-label={`Messages with ${who}`}>
      {request.messages.length > 0 && (
        <ol className="visitor-thread-list">
          {request.messages.map((message, index) => (
            <li key={`${message.createdLabel}-${index}`} data-author={message.author}>
              <p className="visitor-thread-meta">
                <span>{message.authorLabel}</span> {message.createdLabel}
              </p>
              <p className="visitor-thread-body">{message.body}</p>
            </li>
          ))}
        </ol>
      )}

      {request.canMessage && (
        <form
          className="visitor-thread-reply"
          onSubmit={(event) => {
            event.preventDefault();
            void queue.reply(request, draft).then((sent) => {
              if (sent) setDraft("");
            });
          }}
        >
          <label htmlFor={replyID}>Reply to {who}</label>
          <textarea
            id={replyID}
            value={draft}
            maxLength={500}
            rows={2}
            disabled={busy}
            onChange={(event) => setDraft(event.target.value)}
          />
          <button
            className="button secondary compact"
            type="submit"
            disabled={busy || !draft.trim()}
          >
            Send
          </button>
        </form>
      )}

      {request.messages.length > 0 &&
        (confirming ? (
          <ConfirmDelete
            question="Delete this conversation for good?"
            action="Delete conversation"
            busy={busy}
            onConfirm={() => void queue.eraseThread(request).then(() => setConfirming(false))}
            onCancel={() => setConfirming(false)}
          >
            <p>Its messages are deleted for both of you. The request itself stays.</p>
          </ConfirmDelete>
        ) : (
          <button
            className="text-button danger"
            type="button"
            disabled={busy}
            aria-label={`Delete the conversation with ${who}`}
            onClick={() => setConfirming(true)}
          >
            Delete conversation
          </button>
        ))}
    </section>
  );
}
