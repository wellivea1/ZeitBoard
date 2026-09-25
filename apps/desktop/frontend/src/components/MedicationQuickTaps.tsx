import type {
  MedicationDefinition,
  MedicationEventInput,
  MedicationEventStatus,
  MedicationLog,
} from "../data/medications";
import { clockTime, relativeDay } from "../utils/relativeTime";

// One tap per dose. Recording "taken now" used to mean choosing the medication
// from a list, checking a time and a zone already filled in, and pressing a
// button named after a radio choice. The form is still there for another time
// or a note; this is the everyday case. Every record can be corrected or erased
// from the history.

function localInputNow() {
  const now = new Date();
  return new Date(now.getTime() - now.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
}

function browserZone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "America/New_York";
}

function lastEventWording(event: MedicationLog | undefined, now: Date) {
  if (!event) return "";
  const at = new Date(event.doseLocal.slice(0, 16));
  if (Number.isNaN(at.getTime())) return `last ${event.status} ${event.civilTime}`;
  const day = relativeDay(at, now);
  // "yesterday", but "Monday" and "Tue, Jul 21" keep their capitals.
  const phrase = ["Today", "Tonight", "Yesterday"].includes(day) ? day.toLowerCase() : day;
  return `last ${event.status} ${phrase} ${clockTime(at)}`;
}

export function MedicationQuickTaps({
  medications,
  events,
  available,
  busy,
  onLog,
}: {
  medications: MedicationDefinition[];
  events: MedicationLog[];
  available: boolean;
  busy: boolean;
  onLog: (input: MedicationEventInput) => Promise<void>;
}) {
  const active = medications.filter((medication) => medication.active);
  if (active.length === 0) {
    return (
      <p className="plan-empty">
        {available ? "Add a medication below to record doses." : "Recording needs the desktop app."}
      </p>
    );
  }
  const now = new Date();
  const record = (medication: MedicationDefinition, status: MedicationEventStatus) =>
    void onLog({
      medicationId: medication.medicationId,
      doseLocal: localInputNow(),
      zoneId: browserZone(),
      status,
      scheduled: false,
      note: "",
    }).catch(() => undefined);

  return (
    <ul className="medication-taps">
      {active.map((medication) => {
        const last = events
          .filter((event) => event.medicationId === medication.medicationId)
          .sort((a, b) => b.doseLocal.localeCompare(a.doseLocal))[0];
        const details = [
          medication.detailLabel,
          medication.schedule ? medication.schedule.summary : "",
          lastEventWording(last, now),
        ].filter(Boolean);
        return (
          <li key={medication.medicationId}>
            <div>
              <strong>{medication.label}</strong>
              {details.length > 0 && <small>{details.join(" · ")}</small>}
            </div>
            <div className="medication-tap-actions">
              <button
                className="button primary compact"
                type="button"
                disabled={!available || busy}
                aria-label={`Record ${medication.label} taken now`}
                onClick={() => record(medication, "taken")}
              >
                Taken
              </button>
              <button
                className="button secondary compact"
                type="button"
                disabled={!available || busy}
                aria-label={`Record ${medication.label} skipped now`}
                onClick={() => record(medication, "skipped")}
              >
                Skipped
              </button>
            </div>
          </li>
        );
      })}
    </ul>
  );
}
