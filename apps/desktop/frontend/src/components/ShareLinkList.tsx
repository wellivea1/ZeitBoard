import { useState } from "react";
import type { ShareLinksData } from "../data/sharing";

// Revoking and erasing are deliberately not one click apart. Revocation stops
// the link working and keeps its access history readable; erasure removes the
// record that the link existed at all, and asks for the link's id back first —
// the same shape the sleep log uses for permanent deletion.

export function ShareLinkList({
  data,
  busy,
  onRevoke,
  onErase,
}: {
  data: ShareLinksData;
  busy: boolean;
  onRevoke: (profileId: string) => void;
  onErase: (profileId: string, confirmation: string) => void;
}) {
  const [erasing, setErasing] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState("");

  // The state header already explains why there is nothing here. Repeating its
  // sentence under the heading would say the same thing twice on one screen.
  if (data.status !== "ok") return null;
  if (data.links.length === 0) {
    return (
      <p className="sharing-empty" role="status">
        You have not made a link yet. Nothing is being shared.
      </p>
    );
  }

  return (
    <ul className="sharing-link-list">
      {data.links.map((link) => (
        <li key={link.profileId} data-state={link.state}>
          <div className="sharing-link-head">
            <strong>{link.label}</strong>
            <span className="sharing-link-state" data-state={link.state}>
              {link.stateLabel}
            </span>
          </div>
          <p className="sharing-link-grants">{link.grantSummary}</p>
          <p className="sharing-link-dates">
            {link.createdLabel}
            {link.expiresLabel ? ` · ${link.expiresLabel}` : ""}
          </p>

          {link.access.length > 0 && (
            <ul className="sharing-link-access">
              {link.access.map((entry) => (
                <li key={entry.event}>
                  {entry.label}: {entry.count}
                  {entry.lastLabel ? ` (${entry.lastLabel})` : ""}
                </li>
              ))}
            </ul>
          )}

          <div className="sharing-link-actions">
            {link.state === "active" && (
              <button
                className="button secondary compact"
                type="button"
                disabled={busy}
                onClick={() => onRevoke(link.profileId)}
              >
                Revoke
              </button>
            )}
            <button
              className="button secondary compact danger-outline"
              type="button"
              disabled={busy}
              onClick={() => {
                setErasing(erasing === link.profileId ? null : link.profileId);
                setConfirmation("");
              }}
            >
              Erase record
            </button>
          </div>

          {erasing === link.profileId && (
            <div className="sharing-link-erase" role="group" aria-label={`Erase ${link.label}`}>
              <p>
                Erasing removes the record that this link existed, including its access history.
                Revoking is enough to stop it working. Type <code>{link.profileId}</code> to
                confirm.
              </p>
              <label>
                <span>Link id</span>
                <input
                  value={confirmation}
                  onChange={(event) => setConfirmation(event.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                />
              </label>
              <button
                className="button danger compact"
                type="button"
                disabled={busy || confirmation !== link.profileId}
                onClick={() => {
                  onErase(link.profileId, confirmation);
                  setErasing(null);
                  setConfirmation("");
                }}
              >
                Erase permanently
              </button>
            </div>
          )}
        </li>
      ))}
    </ul>
  );
}
