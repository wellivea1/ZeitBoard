import { useState } from "react";
import { Icon } from "./Icon";
import { SleepEntryForm } from "./SleepEntryForm";
import type { SleepEntry, SleepEntryInput } from "../data/sleepEntries";
import { deleteConfirmationToken } from "../data/sleepDataControl";
import { clockRange } from "../utils/relativeTime";

// One night in the sleep log: what was recorded, and quiet actions to edit it,
// leave it out of estimates, or delete it. The log used to give every night
// three full-size buttons; a month of nights was a wall of them.

const correctionHistoryPerPage = 50;

/** The civil clock of a stored time, ignoring any UTC offset it carries. */
function civilClock(value: string) {
  const date = new Date(value.slice(0, 16));
  return Number.isNaN(date.getTime()) ? undefined : date;
}

// "Wed, Sep 23" and "11:01 PM – 8:08 AM" in the entry's own zone. The stored
// labels ("Wed Sep 23, 11:01 PM EDT to Thu Sep 24, 8:08 AM EDT") stay on hover.
function nightWording(entry: SleepEntry) {
  const start = civilClock(entry.effectiveStartLocal);
  const end = civilClock(entry.effectiveEndLocal);
  if (!start || !end) {
    return { day: entry.effectiveStartLabel, time: `to ${entry.effectiveEndLabel}` };
  }
  return {
    day: start.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" }),
    time: clockRange(start, end),
  };
}

function sourceWording(entry: SleepEntry) {
  if (entry.provenanceLabel.startsWith("manual")) return "Logged by you";
  if (entry.provenanceLabel.startsWith("file import")) return "Imported";
  return entry.provenanceLabel;
}

export function SleepNightRow({
  entry,
  editing,
  editForm,
  busy,
  deleteConfirming,
  deleteConfirmation,
  onBeginEdit,
  onCancelEdit,
  onEditChange,
  onSaveEdit,
  editExcluded,
  onEditExcluded,
  onSuppress,
  onBeginDelete,
  onCancelDelete,
  onDeleteConfirmationChange,
  onDelete,
}: {
  entry: SleepEntry;
  editing: boolean;
  editForm: SleepEntryInput;
  busy: boolean;
  deleteConfirming: boolean;
  deleteConfirmation: string;
  onBeginEdit: () => void;
  onCancelEdit: () => void;
  onEditChange: (form: SleepEntryInput) => void;
  onSaveEdit: () => void;
  editExcluded: boolean;
  onEditExcluded: (value: boolean) => void;
  onSuppress: () => void;
  onBeginDelete: () => void;
  onCancelDelete: () => void;
  onDeleteConfirmationChange: (value: string) => void;
  onDelete: () => void;
}) {
  const [historyPage, setHistoryPage] = useState(0);
  const [historyOpen, setHistoryOpen] = useState(false);
  const corrected =
    entry.startLocal !== entry.effectiveStartLocal ||
    entry.endLocal !== entry.effectiveEndLocal ||
    entry.classification !== entry.effectiveClassification;
  const deleteInputID = `delete-confirm-${entry.observationId}`;
  const historyPageCount = Math.max(1, Math.ceil(entry.history.length / correctionHistoryPerPage));
  const safeHistoryPage = Math.min(historyPage, historyPageCount - 1);
  const historyStart = safeHistoryPage * correctionHistoryPerPage;
  const visibleHistory = entry.history.slice(historyStart, historyStart + correctionHistoryPerPage);
  const night = nightWording(entry);
  const name = `${night.day}, ${night.time}`;
  const state = entry.needsReview
    ? "Needs review"
    : entry.suppressed
      ? "Excluded"
      : corrected
        ? "Corrected"
        : "";
  const details = [
    entry.durationLabel,
    entry.effectiveClassification === "nap"
      ? "Nap"
      : entry.effectiveClassification === "unknown"
        ? "Unclassified"
        : "",
    sourceWording(entry),
  ].filter(Boolean);

  return (
    <li
      className="sleep-night"
      data-suppressed={entry.suppressed || undefined}
      data-review={entry.needsReview || undefined}
    >
      <div className="sleep-night-main">
        <Icon name="moon" />
        <div
          className="sleep-night-text"
          title={`${entry.effectiveStartLabel} to ${entry.effectiveEndLabel}`}
        >
          <strong>
            {night.day} <span>{night.time}</span>
            {state && <em className="sleep-night-state">{state}</em>}
          </strong>
          <small>
            {details.join(" · ")}
            {corrected && ` · first recorded ${entry.startLabel} to ${entry.endLabel}`}
          </small>
        </div>
        {!editing && (
          <div className="sleep-night-actions">
            <button
              className="button ghost compact"
              type="button"
              aria-label={`Edit ${name}`}
              onClick={onBeginEdit}
              disabled={busy}
            >
              Edit
            </button>
            <button
              className="button ghost compact"
              type="button"
              aria-label={`Exclude ${name} from estimates`}
              title="Leave this night out of estimates without deleting it"
              onClick={onSuppress}
              disabled={entry.suppressed || entry.needsReview || busy}
            >
              Exclude
            </button>
            <button
              className="button ghost compact"
              type="button"
              aria-label={`Delete ${name}`}
              onClick={onBeginDelete}
              disabled={busy}
            >
              Delete
            </button>
          </div>
        )}
      </div>

      {entry.needsReview && (
        <div role="status" className="sleep-night-review">
          <p>
            Your edits and the recorded times disagree, so forecasts are paused. Editing starts from
            the recorded times: {entry.sourceWindowLabel}.
          </p>
          <ul>
            {entry.activeEdits.map((edit) => (
              <li key={edit.correctionId}>
                {edit.createdLabel}: {edit.summary}
              </li>
            ))}
          </ul>
          <p>Enter the times that are right and save to settle it.</p>
        </div>
      )}

      {editing && (
        <form
          className="sleep-edit-form"
          onSubmit={(event) => {
            event.preventDefault();
            onSaveEdit();
          }}
        >
          <SleepEntryForm
            form={editForm}
            onChange={onEditChange}
            submitLabel="Save correction"
            disabled={busy}
            editing
            excluded={editExcluded}
            onExcludedChange={onEditExcluded}
            onCancel={onCancelEdit}
          />
        </form>
      )}

      {deleteConfirming && (
        <div
          className="sleep-delete-confirmation"
          role="group"
          aria-labelledby={`${deleteInputID}-title`}
        >
          <div>
            <strong id={`${deleteInputID}-title`}>Delete this night for good?</strong>
            <p>
              This removes it and its corrections from this device. To keep it but leave it out of
              estimates, use Exclude instead.
            </p>
          </div>
          <label htmlFor={deleteInputID}>Type DELETE to confirm</label>
          <input
            id={deleteInputID}
            type="text"
            value={deleteConfirmation}
            onChange={(event) => onDeleteConfirmationChange(event.target.value)}
          />
          <div className="sleep-delete-actions">
            <button
              className="button danger"
              type="button"
              onClick={onDelete}
              disabled={busy || deleteConfirmation !== deleteConfirmationToken}
            >
              Delete night
            </button>
            <button className="button ghost" type="button" onClick={onCancelDelete} disabled={busy}>
              Keep it
            </button>
          </div>
        </div>
      )}

      {entry.history.length > 0 && (
        <details
          className="sleep-entry-history fold"
          onToggle={(event) => setHistoryOpen(event.currentTarget.open)}
        >
          <summary>Correction history ({entry.history.length})</summary>
          {historyOpen && (
            <>
              <ul>
                {visibleHistory.map((item) => (
                  <li key={item.correctionId}>
                    <strong>{item.createdLabel}</strong> - {item.summary}
                  </li>
                ))}
              </ul>
              {historyPageCount > 1 && (
                <nav className="sleep-history-pagination" aria-label="Correction history pages">
                  <span>
                    Corrections {historyStart + 1}-
                    {Math.min(historyStart + correctionHistoryPerPage, entry.history.length)} of{" "}
                    {entry.history.length}
                  </span>
                  <button
                    className="button secondary compact"
                    type="button"
                    disabled={safeHistoryPage === 0}
                    onClick={() => setHistoryPage((page) => Math.max(0, page - 1))}
                  >
                    Previous corrections
                  </button>
                  <button
                    className="button secondary compact"
                    type="button"
                    disabled={safeHistoryPage === historyPageCount - 1}
                    onClick={() =>
                      setHistoryPage((page) => Math.min(historyPageCount - 1, page + 1))
                    }
                  >
                    Next corrections
                  </button>
                </nav>
              )}
            </>
          )}
        </details>
      )}
    </li>
  );
}
