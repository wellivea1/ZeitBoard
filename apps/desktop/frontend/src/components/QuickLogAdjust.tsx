import { useState } from "react";
import { TimeField } from "./SleepEntryForm";
import { correctSleepEntry, loadSleepLogPage, type SleepEntry } from "../data/sleepEntries";
import { notifySleepDataChanged } from "../data/sleepDataEvents";
import { useLoaded } from "../state/useLoaded";
import { correctionForm, sleepFormInput, type SleepForm } from "../utils/sleepForm";

// The night just recorded with two taps, open to adjust while the mistake is
// fresh: a tap that came late, or a wake noticed a while after waking. Until
// now the fix was Log, find the night, Edit. It is saved as a correction, so
// the times first recorded are kept.

async function loadRecorded(observationId: string): Promise<SleepEntry | null> {
  // The night just recorded is the newest, so it is on the log's first page.
  const page = await loadSleepLogPage(0);
  return page.entries.find((entry) => entry.observationId === observationId) ?? null;
}

function AdjustForm({
  entry,
  onDone,
  onCancel,
}: {
  entry: SleepEntry;
  onDone: (message: string) => void;
  onCancel: () => void;
}) {
  const [form, setForm] = useState<SleepForm>(() => correctionForm(entry));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const save = () => {
    const ready = sleepFormInput(form);
    if ("problem" in ready) {
      setError(ready.problem);
      return;
    }
    setBusy(true);
    setError("");
    void correctSleepEntry({
      ...ready.input,
      observationId: entry.observationId,
      reviewToken: entry.reviewToken,
      excluded: false,
    }).then(
      () => {
        notifySleepDataChanged();
        onDone("Times adjusted. The times first recorded are kept in the night's history.");
      },
      (reason: unknown) => {
        setBusy(false);
        setError(reason instanceof Error ? reason.message : "The times could not be saved.");
      },
    );
  };
  return (
    <form
      className="quick-log-question"
      aria-label="Adjust the times just recorded"
      onSubmit={(event) => {
        event.preventDefault();
        save();
      }}
    >
      <p>Adjust the night just recorded.</p>
      <TimeField
        label="Fell asleep"
        field={form.start}
        zone={form.zoneId}
        disabled={busy}
        onChange={(start) => setForm({ ...form, start })}
      />
      <TimeField
        label="Woke up"
        field={form.end}
        zone={form.zoneId}
        disabled={busy}
        onChange={(end) => setForm({ ...form, end })}
      />
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <div className="quick-log-question-actions">
        <button className="button primary compact" type="submit" disabled={busy}>
          {busy ? "Saving…" : "Save the times"}
        </button>
        <button
          className="button secondary compact"
          type="button"
          disabled={busy}
          onClick={onCancel}
        >
          Leave them
        </button>
      </div>
    </form>
  );
}

export function QuickLogAdjust({
  observationId,
  onDone,
  onCancel,
}: {
  observationId: string;
  onDone: (message: string) => void;
  onCancel: () => void;
}) {
  const { data: entry } = useLoaded(() => loadRecorded(observationId), {
    key: observationId,
    fallback: () => null,
  });
  if (entry === undefined) {
    return (
      <p className="quick-log-notice" role="status">
        Opening the night…
      </p>
    );
  }
  if (entry === null) {
    return (
      <p className="quick-log-notice" role="status">
        The night could not be opened here. <a href="#/log/sleep">Adjust it in Log</a>.
      </p>
    );
  }
  return <AdjustForm entry={entry} onDone={onDone} onCancel={onCancel} />;
}
