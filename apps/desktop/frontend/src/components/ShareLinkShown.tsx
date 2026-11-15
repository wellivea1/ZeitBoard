import { useState } from "react";
import { statusLevels, type ShareLink, type ShareStatusLevel } from "../data/sharing";

// What an existing link shows, open to change. It used to be fixed when the
// link was made: to share less, the owner revoked it, made another and sent
// that round. Whether a link takes requests and messages still is fixed, since
// requests and conversations already exist under it.

export function ShareLinkShown({
  link,
  busy,
  onSave,
  onCancel,
}: {
  link: ShareLink;
  busy: boolean;
  onSave: (shown: {
    wakingWindows: boolean;
    skippedDoses: boolean;
    reportedStatus: ShareStatusLevel;
  }) => void;
  onCancel: () => void;
}) {
  const [shown, setShown] = useState({
    wakingWindows: link.grants.wakingWindows,
    skippedDoses: link.grants.skippedDoses,
    reportedStatus: link.grants.reportedStatus,
  });
  const nothing =
    !shown.wakingWindows &&
    !shown.skippedDoses &&
    shown.reportedStatus === "" &&
    !link.grants.allowRequests;
  return (
    <form
      className="sharing-shown sheet"
      aria-label={`What ${link.label} sees`}
      onSubmit={(event) => {
        event.preventDefault();
        if (!nothing) onSave(shown);
      }}
    >
      <label>
        <input
          type="checkbox"
          checked={shown.wakingWindows}
          disabled={busy}
          onChange={(event) => setShown({ ...shown, wakingWindows: event.target.checked })}
        />
        <span>When you are likely awake</span>
      </label>
      <label>
        <input
          type="checkbox"
          checked={shown.skippedDoses}
          disabled={busy}
          onChange={(event) => setShown({ ...shown, skippedDoses: event.target.checked })}
        />
        <span>The doses you log as skipped, by time alone</span>
      </label>
      <label className="sharing-shown-level">
        <span>A status you report</span>
        <select
          value={shown.reportedStatus}
          disabled={busy}
          onChange={(event) =>
            setShown({ ...shown, reportedStatus: event.target.value as ShareStatusLevel })
          }
        >
          {statusLevels.map((level) => (
            <option key={level.level} value={level.level}>
              {level.label}
            </option>
          ))}
        </select>
        <small>{statusLevels.find((level) => level.level === shown.reportedStatus)?.detail}</small>
      </label>
      {nothing && (
        <p className="form-error" role="alert">
          A link that shows nothing is not worth keeping. Revoke it instead.
        </p>
      )}
      <div className="sharing-link-actions">
        <button className="button primary compact" type="submit" disabled={busy || nothing}>
          {busy ? "Saving…" : "Save what it shows"}
        </button>
        <button className="button ghost" type="button" disabled={busy} onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  );
}
