import { useEffect, useState } from "react";
import { Icon } from "./Icon";
import { deleteConfirmationToken } from "../data/sleepDataControl";
import { notifySleepDataChanged } from "../data/sleepDataEvents";
import {
  addSleepEntry,
  correctSleepEntry,
  deleteSleepObservation,
  loadSleepEntries,
  suppressSleepEntry,
  type SleepClassification,
  type SleepCorrectionInput,
  type SleepEntriesData,
  type SleepEntry,
  type SleepEntryInput,
} from "../data/sleepEntries";

const fallbackSleepZone = "America/New_York";
const sleepEntriesPerPage = 50;
const correctionHistoryPerPage = 50;

function browserZone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || fallbackSleepZone;
}

function dateTimeInputValue(value: Date) {
  const local = new Date(value.getTime() - value.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function initialSleepForm(): SleepEntryInput {
  const end = new Date();
  end.setSeconds(0, 0);
  const start = new Date(end.getTime() - 8 * 60 * 60 * 1000);
  return {
    startLocal: dateTimeInputValue(start),
    endLocal: dateTimeInputValue(end),
    zoneId: browserZone(),
    classification: "principal",
  };
}

function endAfterStart(input: SleepEntryInput) {
  return new Date(input.endLocal).getTime() > new Date(input.startLocal).getTime();
}

function SleepEntryForm({
  form,
  onChange,
  submitLabel,
  disabled,
}: {
  form: SleepEntryInput;
  onChange: (form: SleepEntryInput) => void;
  submitLabel: string;
  disabled: boolean;
}) {
  return (
    <div className="sleep-entry-fields">
      <label>
        Sleep start
        <input
          type="datetime-local"
          value={form.startLocal}
          disabled={disabled}
          onChange={(event) => onChange({ ...form, startLocal: event.target.value })}
          required
        />
      </label>
      <label>
        Wake time
        <input
          type="datetime-local"
          value={form.endLocal}
          disabled={disabled}
          onChange={(event) => onChange({ ...form, endLocal: event.target.value })}
          required
        />
      </label>
      <label>
        Time zone
        <input
          type="text"
          value={form.zoneId}
          disabled={disabled}
          onChange={(event) => onChange({ ...form, zoneId: event.target.value })}
          required
        />
      </label>
      <label>
        Classification
        <select
          value={form.classification}
          disabled={disabled}
          onChange={(event) =>
            onChange({ ...form, classification: event.target.value as SleepClassification })
          }
        >
          <option value="principal">Principal sleep</option>
          <option value="nap">Nap</option>
        </select>
      </label>
      <div className="sleep-entry-submit">
        <button className="button primary" type="submit" disabled={disabled}>
          {submitLabel}
        </button>
      </div>
    </div>
  );
}

function SleepEntryCard({
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

  return (
    <article className="sleep-entry-card" data-suppressed={entry.suppressed || undefined}>
      <div className="sleep-entry-main">
        <div className="record-icon">
          <Icon name="moon" />
        </div>
        <div>
          <h2>
            {entry.effectiveStartLabel} to {entry.effectiveEndLabel}
          </h2>
          <p>
            {entry.durationLabel} - {entry.effectiveClassification} - {entry.provenanceLabel}
          </p>
          {corrected && (
            <p className="sleep-entry-raw">
              Raw entry: {entry.startLabel} to {entry.endLabel}
            </p>
          )}
        </div>
        <span className={`source-status ${entry.suppressed ? "" : "connected"}`}>
          {entry.suppressed ? "Suppressed" : corrected ? "Corrected" : "Active"}
        </span>
      </div>

      {editing ? (
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
          />
          <button className="button secondary" type="button" onClick={onCancelEdit} disabled={busy}>
            Cancel
          </button>
        </form>
      ) : (
        <div className="sleep-entry-actions">
          <button className="button secondary" type="button" onClick={onBeginEdit} disabled={busy}>
            Edit by correction
          </button>
          <button
            className="button secondary"
            type="button"
            onClick={onSuppress}
            disabled={entry.suppressed || busy}
          >
            Suppress from estimates
          </button>
          <button
            className="button secondary danger-outline"
            type="button"
            onClick={onBeginDelete}
            disabled={busy}
          >
            Delete permanently
          </button>
        </div>
      )}

      {deleteConfirming && (
        <div
          className="sleep-delete-confirmation"
          role="group"
          aria-labelledby={`${deleteInputID}-title`}
        >
          <div>
            <strong id={`${deleteInputID}-title`}>Permanent erase</strong>
            <p>
              Type DELETE to remove this observation and its correction history from local sleep
              storage. Use suppress when you only want it excluded from estimates.
            </p>
          </div>
          <label htmlFor={deleteInputID}>Deletion confirmation</label>
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
              Erase entry
            </button>
            <button
              className="button secondary"
              type="button"
              onClick={onCancelDelete}
              disabled={busy}
            >
              Cancel erase
            </button>
          </div>
        </div>
      )}

      {entry.history.length > 0 && (
        <details
          className="sleep-entry-history"
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
    </article>
  );
}

// The sleep log: entry, correction history, suppression and erasure.
//
// It moved out of Data Sources in slice U-H. Recording last night is not the
// same job as configuring where records come from, and the two were sharing a
// 593-line screen because they had both grown there. Data Sources keeps the
// sources; this is the log.
export function SleepLogPanel() {
  const [entriesData, setEntriesData] = useState<SleepEntriesData>({
    status: "empty",
    empty: true,
    message: "Loading local sleep entries.",
    entries: [],
  });
  const [form, setForm] = useState<SleepEntryInput>(initialSleepForm);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editForm, setEditForm] = useState<SleepEntryInput>(initialSleepForm);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState("");
  const [statusMessage, setStatusMessage] = useState("");

  const [entryPage, setEntryPage] = useState(0);
  const refreshEntries = async () => {
    const loaded = await loadSleepEntries();
    setEntriesData(loaded);
  };

  useEffect(() => {
    let current = true;
    void loadSleepEntries()
      .then((loaded) => {
        if (current) setEntriesData(loaded);
      })
      .catch((error: unknown) => {
        if (!current) return;
        setEntriesData({
          status: "unavailable",
          empty: true,
          message: error instanceof Error ? error.message : "Manual sleep log is unavailable.",
          entries: [],
        });
      });
    return () => {
      current = false;
    };
  }, []);

  const submitEntry = async () => {
    setFormError("");
    if (!endAfterStart(form)) {
      setFormError("Wake time must be after sleep start.");
      return;
    }
    setBusy(true);
    try {
      await addSleepEntry(form);
      notifySleepDataChanged();
      setStatusMessage("Sleep entry saved locally.");
      setForm(initialSleepForm());
      await refreshEntries();
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not save sleep entry.");
    } finally {
      setBusy(false);
    }
  };

  const beginEdit = (entry: SleepEntry) => {
    setEditingId(entry.observationId);
    setDeletingId(null);
    setDeleteConfirmation("");
    setEditForm({
      startLocal: entry.effectiveStartLocal,
      endLocal: entry.effectiveEndLocal,
      zoneId: entry.zoneId,
      classification: entry.effectiveClassification,
    });
    setFormError("");
  };

  const saveEdit = async () => {
    if (!editingId) return;
    setFormError("");
    if (!endAfterStart(editForm)) {
      setFormError("Wake time must be after sleep start.");
      return;
    }
    setBusy(true);
    try {
      const correction: SleepCorrectionInput = { observationId: editingId, ...editForm };
      await correctSleepEntry(correction);
      notifySleepDataChanged();
      setStatusMessage("Correction appended locally.");
      setEditingId(null);
      await refreshEntries();
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not append correction.");
    } finally {
      setBusy(false);
    }
  };

  const suppressEntry = async (entry: SleepEntry) => {
    setBusy(true);
    setFormError("");
    setDeletingId(null);
    try {
      await suppressSleepEntry(entry.observationId);
      notifySleepDataChanged();
      setStatusMessage("Sleep entry suppressed from estimates.");
      await refreshEntries();
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not suppress entry.");
    } finally {
      setBusy(false);
    }
  };

  const beginDelete = (entry: SleepEntry) => {
    setDeletingId(entry.observationId);
    setDeleteConfirmation("");
    setEditingId(null);
    setFormError("");
  };

  const deleteEntry = async (entry: SleepEntry) => {
    if (deleteConfirmation !== deleteConfirmationToken) {
      setFormError("Type DELETE to confirm permanent erasure.");
      return;
    }
    setBusy(true);
    setFormError("");
    try {
      const loaded = await deleteSleepObservation(entry.observationId, deleteConfirmation);
      setEntriesData(loaded);
      notifySleepDataChanged();
      setStatusMessage("Sleep entry erased permanently.");
      setDeletingId(null);
      setDeleteConfirmation("");
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not erase sleep entry.");
    } finally {
      setBusy(false);
    }
  };

  const entryPageCount = Math.max(1, Math.ceil(entriesData.entries.length / sleepEntriesPerPage));
  const safeEntryPage = Math.min(entryPage, entryPageCount - 1);
  const entryStart = safeEntryPage * sleepEntriesPerPage;
  const visibleEntries = entriesData.entries.slice(entryStart, entryStart + sleepEntriesPerPage);

  return (
    <section className="data-source-workspace sleep-log-workspace" aria-label="Sleep log">
      <div className="data-source-input-grid">
        <section className="sleep-entry-panel" aria-labelledby="sleep-entry-title">
          <div className="data-source-section-heading">
            <div>
              <p className="section-kicker">Manual input</p>
              <h2 id="sleep-entry-title">Add sleep entry</h2>
            </div>
            <p>Append one owner-reported principal sleep or nap.</p>
          </div>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void submitEntry();
            }}
          >
            <SleepEntryForm
              form={form}
              onChange={setForm}
              submitLabel="Save sleep entry"
              disabled={busy || entriesData.status === "unavailable"}
            />
          </form>
          {formError && (
            <p className="form-error" role="alert">
              {formError}
            </p>
          )}
          <p className="form-status" role="status" aria-live="polite">
            {statusMessage || entriesData.message}
          </p>
        </section>
      </div>

      <section className="sleep-entry-list-panel" aria-labelledby="sleep-log-title">
        <div className="data-source-section-heading">
          <div>
            <p className="section-kicker">Local observations</p>
            <h2 id="sleep-log-title">Sleep log</h2>
          </div>
          <p>Corrections append history; suppression and permanent erasure remain distinct.</p>
        </div>
        {entriesData.entries.length === 0 ? (
          <div className="empty-state sleep-log-empty">
            <p className="section-kicker">{entriesData.status}</p>
            <h2>No sleep entries yet</h2>
            <p>
              Add your first principal sleep episode above. The estimator will stay refused until
              enough usable entries exist.
            </p>
          </div>
        ) : (
          <>
            {entryPageCount > 1 && (
              <nav className="sleep-log-pagination" aria-label="Sleep log pages">
                <span>
                  Entries {entryStart + 1}-
                  {Math.min(entryStart + sleepEntriesPerPage, entriesData.entries.length)} of{" "}
                  {entriesData.entries.length}
                </span>
                <button
                  className="button secondary compact"
                  type="button"
                  disabled={safeEntryPage === 0}
                  onClick={() => setEntryPage((page) => Math.max(0, page - 1))}
                >
                  Previous entries
                </button>
                <button
                  className="button secondary compact"
                  type="button"
                  disabled={safeEntryPage === entryPageCount - 1}
                  onClick={() => setEntryPage((page) => Math.min(entryPageCount - 1, page + 1))}
                >
                  Next entries
                </button>
              </nav>
            )}
            <div className="sleep-entry-list">
              {visibleEntries.map((entry) => (
                <SleepEntryCard
                  key={entry.observationId}
                  entry={entry}
                  editing={editingId === entry.observationId}
                  editForm={editForm}
                  busy={busy}
                  deleteConfirming={deletingId === entry.observationId}
                  deleteConfirmation={deletingId === entry.observationId ? deleteConfirmation : ""}
                  onBeginEdit={() => beginEdit(entry)}
                  onCancelEdit={() => setEditingId(null)}
                  onEditChange={setEditForm}
                  onSaveEdit={saveEdit}
                  onSuppress={() => void suppressEntry(entry)}
                  onBeginDelete={() => beginDelete(entry)}
                  onCancelDelete={() => {
                    setDeletingId(null);
                    setDeleteConfirmation("");
                  }}
                  onDeleteConfirmationChange={setDeleteConfirmation}
                  onDelete={() => void deleteEntry(entry)}
                />
              ))}
            </div>
          </>
        )}
      </section>
    </section>
  );
}
