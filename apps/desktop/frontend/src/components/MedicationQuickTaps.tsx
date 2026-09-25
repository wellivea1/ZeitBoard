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
  const status = event.status === "taken" ? "Last taken" : "Last skipped";
  if (Number.isNaN(at.getTime())) return `${status} ${event.civilTime}`;
  const day = relativeDay(at, now);
  // "yesterday", but "Monday" and "Tue, Jul 21" keep their capitals.
  const phrase = ["Today", "Tonight", "Yesterday"].includes(day) ? day.toLowerCase() : day;
  return `${status} ${phrase} at ${clockTime(at)}`;
}

/** "10:00 PM" from a schedule's "22:00". */
function civilClock(value: string) {
  const [hour, minute] = value.split(":").map(Number);
  if (hour === undefined || minute === undefined || !Number.isFinite(hour + minute)) return value;
  return clockTime(new Date(2000, 0, 1, hour, minute));
}

function usualWording(medication: MedicationDefinition) {
  const schedule = medication.schedule;
  if (!schedule) return "";
  if (schedule.kind === "as_needed") return "As needed";
  const times = schedule.civilTimes.map(civilClock);
  if (times.length === 0) return schedule.summary;
  const list =
    times.length === 1 ? times[0] : `${times.slice(0, -1).join(", ")} and ${times.at(-1)}`;
  const cycle =
    schedule.kind === "cycling" && schedule.daysOn && schedule.daysOff
      ? `, ${schedule.daysOn} days on and ${schedule.daysOff} off`
      : "";
  return `Usually at ${list}${cycle}`;
}

/** "Tablet, 5 mg. Usually at 10:00 PM. Last taken yesterday at 10:05 PM." */
function doseSentence(medication: MedicationDefinition, last: MedicationLog | undefined) {
  const form = medication.detailLabel.replace(/ · /g, ", ");
  return [
    form && form.charAt(0).toUpperCase() + form.slice(1),
    usualWording(medication),
    lastEventWording(last, new Date()),
  ]
    .filter(Boolean)
    .map((part) => `${part}.`)
    .join(" ");
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
        const details = doseSentence(medication, last);
        return (
          <li key={medication.medicationId}>
            <div>
              <strong>{medication.label}</strong>
              {details && <small>{details}</small>}
            </div>
            <div className="medication-tap-actions">
              <button
                className="button ghost"
                type="button"
                disabled={!available || busy}
                aria-label={`Record ${medication.label} taken now`}
                onClick={() => record(medication, "taken")}
              >
                Taken
              </button>
              <button
                className="button ghost"
                type="button"
                data-quiet
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
