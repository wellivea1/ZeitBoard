import { useState, type FormEvent } from "react";
import type {
  MedicationDefinition,
  MedicationEventInput,
  MedicationEventStatus,
} from "../data/medications";

function localInputNow() {
  const now = new Date();
  return new Date(now.getTime() - now.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
}

function browserZone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "America/New_York";
}

export function MedicationLogForm({
  medications,
  available,
  busy,
  onLog,
}: {
  medications: MedicationDefinition[];
  available: boolean;
  busy: boolean;
  onLog: (input: MedicationEventInput) => Promise<void>;
}) {
  const active = medications.filter((medication) => medication.active);
  const [medicationId, setMedicationID] = useState("");
  const [doseLocal, setDoseLocal] = useState(localInputNow);
  const [zoneId, setZoneID] = useState(browserZone);
  const [status, setStatus] = useState<MedicationEventStatus>("taken");
  const [scheduled, setScheduled] = useState(false);
  const [note, setNote] = useState("");
  const selectedMedicationId = active.some((medication) => medication.medicationId === medicationId)
    ? medicationId
    : (active[0]?.medicationId ?? "");

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!available || busy || !selectedMedicationId || !doseLocal || !zoneId.trim()) return;
    void onLog({
      medicationId: selectedMedicationId,
      doseLocal,
      zoneId: zoneId.trim(),
      status,
      scheduled,
      note: note.trim(),
    }).then(
      () => {
        setDoseLocal(localInputNow());
        setNote("");
      },
      () => undefined,
    );
  };

  return (
    <section className="medication-log-section" aria-labelledby="medication-log-title">
      <header className="medication-section-heading">
        <div>
          <p className="section-kicker">Append-only evidence</p>
          <h2 id="medication-log-title">Quick log</h2>
        </div>
        <span>Taken or skipped only</span>
      </header>
      {active.length === 0 ? (
        <p className="medication-log-unavailable">
          Add or reactivate a medication label to log an event.
        </p>
      ) : (
        <form className="medication-log-form" onSubmit={submit}>
          <label className="medication-log-medication">
            <span>Medication</span>
            <select
              value={selectedMedicationId}
              disabled={!available || busy}
              onChange={(event) => setMedicationID(event.target.value)}
            >
              {active.map((medication) => (
                <option value={medication.medicationId} key={medication.medicationId}>
                  {medication.label}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span>Event time</span>
            <input
              type="datetime-local"
              value={doseLocal}
              disabled={!available || busy}
              onChange={(event) => setDoseLocal(event.target.value)}
            />
          </label>
          <label>
            <span>Time zone</span>
            <input
              value={zoneId}
              disabled={!available || busy}
              onChange={(event) => setZoneID(event.target.value)}
            />
          </label>
          <fieldset className="medication-status-field">
            <legend>Status</legend>
            <label>
              <input
                type="radio"
                name="medication-status"
                value="taken"
                checked={status === "taken"}
                disabled={!available || busy}
                onChange={() => setStatus("taken")}
              />
              <span>Taken</span>
            </label>
            <label>
              <input
                type="radio"
                name="medication-status"
                value="skipped"
                checked={status === "skipped"}
                disabled={!available || busy}
                onChange={() => setStatus("skipped")}
              />
              <span>Skipped</span>
            </label>
          </fieldset>
          <label className="medication-log-note">
            <span>Private note</span>
            <input
              value={note}
              maxLength={500}
              disabled={!available || busy}
              placeholder="Optional factual note"
              onChange={(event) => setNote(event.target.value)}
            />
          </label>
          <label className="medication-check-row medication-scheduled-check">
            <input
              type="checkbox"
              checked={scheduled}
              disabled={!available || busy}
              onChange={(event) => setScheduled(event.target.checked)}
            />
            <span>Corresponds to a schedule recorded elsewhere</span>
          </label>
          <button
            className="button primary compact medication-log-submit"
            type="submit"
            disabled={!available || busy || !selectedMedicationId}
          >
            {busy ? "Recording..." : `Record ${status}`}
          </button>
        </form>
      )}
    </section>
  );
}
