import { useState } from "react";
import { Icon } from "./Icon";
import { SleepEntryForm } from "./SleepEntryForm";
import type { SleepEntry } from "../data/sleepEntries";
import type { SleepForm } from "../utils/sleepForm";
import { ConfirmDelete } from "./ConfirmDelete";
import { nightWording, sourceWording } from "../utils/sleepWording";

// One night in the sleep log: what was recorded, and quiet actions to edit it,
// take the last edit back, leave it out of estimates, or delete it. The log
// used to give every night three full-size buttons; a month of nights was a
// wall of them.

const correctionHistoryPerPage = 50;

function isCorrected(entry: SleepEntry): boolean {
  return (
    entry.startLocal !== entry.effectiveStartLocal ||
    entry.endLocal !== entry.effectiveEndLocal ||
    entry.classification !== entry.effectiveClassification
  );
}

/**
 * The small-capital word beside a night. A night its source deleted is kept,
 * and left out unless a correction of the owner's stands on it (ADR-0072).
 */
function nightState(entry: SleepEntry, corrected: boolean): string {
  if (entry.needsReview) return "Needs review";
  if (entry.removedAtSource && entry.suppressed) return "Removed at the source";
  if (entry.suppressed) return "Excluded";
  return corrected ? "Corrected" : "";
}

/** The line under a night: its length, its kind, its source, and its clock when away (ADR-0073). */
function nightDetails(entry: SleepEntry): string[] {
  const kind =
    entry.effectiveClassification === "nap"
      ? "Nap"
      : entry.effectiveClassification === "unknown"
        ? "Unclassified"
        : "";
  return [
    entry.durationLabel,
    kind,
    sourceWording(entry.provenanceLabel),
    entry.recordedAway ?? "",
  ].filter(Boolean);
}

/** Why a night its source deleted is left out, or why it still counts. */
function RemovedNote({ entry }: { entry: SleepEntry }) {
  if (!entry.removedAtSource || entry.needsReview) return null;
  return (
    <p className="sleep-night-note">
      {entry.suppressed
        ? "Health Connect no longer has this night, so it is left out of estimates. Edit it to count it again."
        : "Health Connect no longer has this night. It still counts, because your correction stands."}
    </p>
  );
}

export function SleepNightRow({
  entry,
  editing,
  editForm,
  editError,
  busy,
  deleteConfirming,
  onBeginEdit,
  onCancelEdit,
  onEditChange,
  onSaveEdit,
  editExcluded,
  onEditExcluded,
  onUndo,
  onSuppress,
  onBeginDelete,
  onCancelDelete,
  onDelete,
}: {
  entry: SleepEntry;
  editing: boolean;
  editForm: SleepForm;
  /** Why the correction was not saved. */
  editError: string;
  busy: boolean;
  deleteConfirming: boolean;
  onBeginEdit: () => void;
  onCancelEdit: () => void;
  onEditChange: (form: SleepForm) => void;
  onSaveEdit: () => void;
  editExcluded: boolean;
  onEditExcluded: (value: boolean) => void;
  onUndo: () => void;
  onSuppress: () => void;
  onBeginDelete: () => void;
  onCancelDelete: () => void;
  onDelete: () => void;
}) {
  const [historyPage, setHistoryPage] = useState(0);
  const [historyOpen, setHistoryOpen] = useState(false);
  const corrected = isCorrected(entry);
  const historyPageCount = Math.max(1, Math.ceil(entry.history.length / correctionHistoryPerPage));
  const safeHistoryPage = Math.min(historyPage, historyPageCount - 1);
  const historyStart = safeHistoryPage * correctionHistoryPerPage;
  const visibleHistory = entry.history.slice(historyStart, historyStart + correctionHistoryPerPage);
  const night = nightWording({
    startLocal: entry.effectiveStartLocal,
    endLocal: entry.effectiveEndLocal,
    startLabel: entry.effectiveStartLabel,
    endLabel: entry.effectiveEndLabel,
  });
  const name = `${night.day}, ${night.time}`;
  const state = nightState(entry, corrected);
  const details = nightDetails(entry);

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
            {entry.canUndo && (
              <button
                className="button ghost compact"
                type="button"
                aria-label={`Undo the last edit to ${name}`}
                title="Take back the last edit. It stays in the history."
                onClick={onUndo}
                disabled={busy}
              >
                Undo
              </button>
            )}
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

      <RemovedNote entry={entry} />

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
            error={editError}
          />
        </form>
      )}

      {deleteConfirming && (
        <ConfirmDelete
          question="Delete this night for good?"
          action="Delete night"
          busy={busy}
          onConfirm={onDelete}
          onCancel={onCancelDelete}
        >
          <p>
            This removes it and its corrections from this device. To keep it but leave it out of
            estimates, use Exclude instead.
          </p>
        </ConfirmDelete>
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
