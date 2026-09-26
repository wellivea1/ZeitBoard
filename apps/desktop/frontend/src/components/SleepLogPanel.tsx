import { SleepEntryForm } from "./SleepEntryForm";
import { useEffect, useRef, useState } from "react";
import { Loading } from "./Loading";
import { QuickLogBar } from "./QuickLogBar";
import { useLoaded } from "../state/useLoaded";
import { deleteWord } from "../data/deletion";
import { SleepNightRow } from "./SleepNightRow";
import { notifySleepDataChanged, sleepDataChangedEvent } from "../data/sleepDataEvents";
import {
  addSleepEntry,
  correctSleepEntry,
  deleteSleepObservation,
  loadSleepEntries,
  sleepEntriesUnavailable,
  suppressSleepEntry,
  type SleepCorrectionInput,
  type SleepEntriesData,
  type SleepEntry,
  type SleepEntryInput,
} from "../data/sleepEntries";

const fallbackSleepZone = "America/New_York";
const sleepEntriesPerPage = 50;

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

const noEntries: SleepEntriesData = { status: "empty", empty: true, message: "", entries: [] };

/** Log › Sleep with "Add a past night" already open. */
const addNightHash = "#/log/sleep/add";

// The sleep log: entry, correction history, suppression and erasure.
//
// It moved out of Data Sources in slice U-H. Recording last night is not the
// same job as configuring where records come from, and the two were sharing a
// 593-line screen because they had both grown there. Data Sources keeps the
// sources; this is the log.
export function SleepLogPanel() {
  const { data: loadedEntries, set: setEntriesData } = useLoaded(loadSleepEntries, {
    events: [sleepDataChangedEvent],
    fallback: sleepEntriesUnavailable,
  });
  const entriesData = loadedEntries ?? noEntries;
  const [form, setForm] = useState<SleepEntryInput>(initialSleepForm);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editingSnapshot, setEditingSnapshot] = useState<SleepEntry | null>(null);
  const [editExcluded, setEditExcluded] = useState(false);
  const [editForm, setEditForm] = useState<SleepEntryInput>(initialSleepForm);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState("");
  const [statusMessage, setStatusMessage] = useState("");

  const [entryPage, setEntryPage] = useState(0);
  const addRef = useRef<HTMLDetailsElement>(null);
  const refreshEntries = async () => {
    const loaded = await loadSleepEntries();
    setEntriesData(loaded);
  };

  // "Add a past night" elsewhere (Home, before a forecast) arrives here with
  // the form open and its first field ready.
  useEffect(() => {
    const openFromAddress = () => {
      const fold = addRef.current;
      if (window.location.hash !== addNightHash || !fold) return;
      fold.open = true;
      fold.querySelector<HTMLElement>("input, select, textarea")?.focus();
    };
    openFromAddress();
    window.addEventListener("hashchange", openFromAddress);
    return () => window.removeEventListener("hashchange", openFromAddress);
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
      setStatusMessage("Night saved.");
      setForm(initialSleepForm());
      await refreshEntries();
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not save sleep entry.");
    } finally {
      setBusy(false);
    }
  };

  const beginEdit = (entry: SleepEntry) => {
    setEditingSnapshot(entry);
    setEditExcluded(entry.suppressed);
    setEditingId(entry.observationId);
    setDeletingId(null);
    setEditForm({
      startLocal: entry.effectiveStartLocal,
      endLocal: entry.effectiveEndLocal,
      zoneId: entry.zoneId,
      classification: entry.effectiveClassification,
    });
    setFormError("");
  };

  const saveEdit = async () => {
    if (!editingId || !editingSnapshot) return;
    setFormError("");
    if (!endAfterStart(editForm)) {
      setFormError("Wake time must be after sleep start.");
      return;
    }
    setBusy(true);
    try {
      const correction: SleepCorrectionInput = {
        observationId: editingId,
        reviewToken: editingSnapshot.reviewToken,
        excluded: editExcluded,
        ...editForm,
      };
      await correctSleepEntry(correction);
      notifySleepDataChanged();
      setStatusMessage("Correction saved. The original stays in its history.");
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
      await suppressSleepEntry(entry.observationId, entry.reviewToken);
      notifySleepDataChanged();
      setStatusMessage("Night excluded from estimates.");
      await refreshEntries();
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not suppress entry.");
    } finally {
      setBusy(false);
    }
  };

  const beginDelete = (entry: SleepEntry) => {
    setDeletingId(entry.observationId);
    setEditingId(null);
    setFormError("");
  };

  const deleteEntry = async (entry: SleepEntry) => {
    setBusy(true);
    setFormError("");
    try {
      const loaded = await deleteSleepObservation(entry.observationId, deleteWord);
      setEntriesData(loaded);
      notifySleepDataChanged();
      setStatusMessage("Night deleted.");
      setDeletingId(null);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not delete the night.");
    } finally {
      setBusy(false);
    }
  };

  const entryPageCount = Math.max(1, Math.ceil(entriesData.entries.length / sleepEntriesPerPage));
  const safeEntryPage = Math.min(entryPage, entryPageCount - 1);
  const entryStart = safeEntryPage * sleepEntriesPerPage;
  const visibleEntries = entriesData.entries.slice(entryStart, entryStart + sleepEntriesPerPage);

  return (
    <section className="sleep-log-workspace" aria-label="Sleep log">
      {/* Recording the night that just happened is two taps; a form for it
          is for nights that were missed. */}
      <QuickLogBar />
      <details className="sleep-add fold" ref={addRef}>
        <summary>Add a past night</summary>
        <form
          aria-label="Add sleep entry"
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
      </details>
      {formError && (
        <p className="form-error" role="alert">
          {formError}
        </p>
      )}
      <p className="form-status" role="status" aria-live="polite">
        {statusMessage}
      </p>

      <section className="sleep-entry-list-panel" aria-labelledby="sleep-log-title">
        <div className="plan-section-head">
          <h2 id="sleep-log-title">
            Sleep log
            {entriesData.entries.length > 0 && (
              <span className="count">{entriesData.entries.length}</span>
            )}
          </h2>
          <small>Edits keep the original. Delete removes a night for good.</small>
        </div>
        {loadedEntries === undefined ? (
          <Loading />
        ) : entriesData.status === "unavailable" ? (
          <div className="sleep-log-empty">
            <h2>The sleep log could not be read</h2>
            <p>{entriesData.message} Nothing saved has changed.</p>
          </div>
        ) : entriesData.entries.length === 0 ? (
          <div className="sleep-log-empty">
            <h2>No sleep entries yet</h2>
            <p>
              Use the buttons above when you go to sleep and wake up, or{" "}
              <button
                className="text-link"
                type="button"
                onClick={() => {
                  if (addRef.current) addRef.current.open = true;
                }}
              >
                add a past night
              </button>
              . Forecasts start once there are enough nights.
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
            <ul className="sleep-night-list">
              {visibleEntries.map((entry) => (
                <SleepNightRow
                  key={entry.observationId}
                  entry={entry}
                  editing={editingId === entry.observationId}
                  editForm={editForm}
                  busy={busy}
                  deleteConfirming={deletingId === entry.observationId}
                  onBeginEdit={() => beginEdit(entry)}
                  onCancelEdit={() => setEditingId(null)}
                  onEditChange={setEditForm}
                  onSaveEdit={saveEdit}
                  editExcluded={editExcluded}
                  onEditExcluded={setEditExcluded}
                  onSuppress={() => void suppressEntry(entry)}
                  onBeginDelete={() => beginDelete(entry)}
                  onCancelDelete={() => setDeletingId(null)}
                  onDelete={() => void deleteEntry(entry)}
                />
              ))}
            </ul>
          </>
        )}
      </section>
    </section>
  );
}
