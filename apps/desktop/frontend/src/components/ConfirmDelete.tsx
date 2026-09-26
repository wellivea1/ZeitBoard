import { useId, useState, type ReactNode } from "react";
import { deleteWord } from "../data/deletion";

// Every permanent deletion is confirmed the same way: a question, what is lost
// (and the gentler option, when there is one), a word typed back, then the
// action beside a way out. The word is DELETE unless a stronger check is
// wanted, such as a share link's own id.
export function ConfirmDelete({
  question,
  children,
  action,
  busyAction = "Deleting…",
  word = deleteWord,
  busy,
  onConfirm,
  onCancel,
}: {
  question: string;
  children: ReactNode;
  /** The button, naming what goes: "Delete night". */
  action: string;
  busyAction?: string;
  word?: string;
  busy: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const id = useId();
  const [typed, setTyped] = useState("");
  return (
    <div className="confirm-delete" role="group" aria-labelledby={`${id}-question`}>
      <div className="confirm-delete-text">
        <strong id={`${id}-question`}>{question}</strong>
        {children}
      </div>
      <label htmlFor={`${id}-word`}>Type {word} to confirm</label>
      <input
        id={`${id}-word`}
        type="text"
        value={typed}
        autoComplete="off"
        spellCheck={false}
        autoFocus
        disabled={busy}
        onChange={(event) => setTyped(event.target.value)}
      />
      <div className="confirm-delete-actions">
        <button
          className="button danger"
          type="button"
          disabled={busy || typed !== word}
          onClick={onConfirm}
        >
          {busy ? busyAction : action}
        </button>
        <button className="button ghost" type="button" disabled={busy} onClick={onCancel}>
          Keep it
        </button>
      </div>
    </div>
  );
}
