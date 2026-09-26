import { SharePreview } from "./SharePreview";
import { ConfirmDelete } from "./ConfirmDelete";
import { useState } from "react";
import type { ShareLinksData } from "../data/sharing";

// Revoking and deleting are deliberately not one click apart. Revocation stops
// the link working and keeps its access history readable; deletion removes the
// record that the link existed at all, and asks for the link's id back first.

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
  const [previewing, setPreviewing] = useState<string | null>(null);

  // The state header already explains why there is nothing here. Repeating its
  // sentence under the heading would say the same thing twice on one screen.
  if (data.status !== "ok") {
    return <p className="sharing-empty">Links you make appear here once sync is set up.</p>;
  }
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
            <button
              className="button secondary compact"
              type="button"
              aria-expanded={previewing === link.profileId}
              aria-label={`${previewing === link.profileId ? "Hide" : "Preview"} what ${link.label} sees`}
              onClick={() => setPreviewing(previewing === link.profileId ? null : link.profileId)}
            >
              {previewing === link.profileId ? "Hide preview" : "Preview"}
            </button>
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
              onClick={() => setErasing(erasing === link.profileId ? null : link.profileId)}
            >
              Delete record
            </button>
          </div>

          {previewing === link.profileId && (
            <SharePreview profileId={link.profileId} label={link.label} />
          )}

          {erasing === link.profileId && (
            <ConfirmDelete
              question={`Delete the record of ${link.label}?`}
              action="Delete record"
              word={link.profileId}
              busy={busy}
              onConfirm={() => {
                onErase(link.profileId, link.profileId);
                setErasing(null);
              }}
              onCancel={() => setErasing(null)}
            >
              <p>
                This removes the record that the link existed, including its access history.
                Revoking is enough to stop it working.
              </p>
            </ConfirmDelete>
          )}
        </li>
      ))}
    </ul>
  );
}
