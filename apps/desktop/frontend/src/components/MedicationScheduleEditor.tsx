import { useState, type FormEvent } from "react";
import type { MedicationDefinition, MedicationScheduleInput } from "../data/medications";

type ScheduleDraft = Omit<MedicationScheduleInput, "medicationId" | "revision">;
type TimedScheduleKind = "fixed_clock" | "cycling";

function browserZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "America/New_York";
}

function todayCivil(): string {
  const now = new Date();
  return new Date(now.getTime() - now.getTimezoneOffset() * 60_000).toISOString().slice(0, 10);
}

function draftFor(medication: MedicationDefinition): ScheduleDraft {
  const schedule = medication.schedule;
  return {
    kind: schedule?.kind ?? "none",
    zoneId: schedule?.zoneId ?? browserZone(),
    civilTimes: schedule?.civilTimes ?? [],
    daysOn: schedule?.daysOn ?? 1,
    daysOff: schedule?.daysOff ?? 1,
    cycleStartedOn: schedule?.cycleStartedOn ?? todayCivil(),
    reminderEnabled: schedule?.reminderEnabled ?? false,
    clinicianRule: medication.clinicianRule ?? "",
  };
}

function timedKind(kind: ScheduleDraft["kind"]): kind is TimedScheduleKind {
  return kind === "fixed_clock" || kind === "cycling";
}

function draftError(draft: ScheduleDraft): string {
  if (draft.clinicianRule.trim().length > 500) {
    return "Clinician guidance must be 500 characters or fewer.";
  }
  if (!timedKind(draft.kind)) return "";
  if (!draft.zoneId.trim()) return "An explicit IANA time zone is required.";
  if (draft.civilTimes.length < 1 || draft.civilTimes.length > 8) {
    return "Enter between one and eight civil times.";
  }
  if (draft.civilTimes.some((value) => !value)) return "Every civil time is required.";
  if (new Set(draft.civilTimes).size !== draft.civilTimes.length) {
    return "Civil times must be unique.";
  }
  if (
    draft.kind === "cycling" &&
    (draft.daysOn < 1 ||
      draft.daysOn > 365 ||
      draft.daysOff < 1 ||
      draft.daysOff > 365 ||
      !draft.cycleStartedOn)
  ) {
    return "Cycling schedules require a start date and 1 to 365 on/off days.";
  }
  return "";
}

export function MedicationScheduleEditor({
  medication,
  busy,
  onSave,
  onCancel,
}: {
  medication: MedicationDefinition;
  busy: boolean;
  onSave: (input: MedicationScheduleInput) => Promise<void>;
  onCancel: () => void;
}) {
  const [draft, setDraft] = useState<ScheduleDraft>(() => draftFor(medication));
  const validationError = draftError(draft);
  const isTimed = timedKind(draft.kind);

  const changeKind = (kind: ScheduleDraft["kind"]) => {
    setDraft((current) => ({
      ...current,
      kind,
      civilTimes:
        timedKind(kind) && current.civilTimes.length === 0 ? ["09:00"] : current.civilTimes,
      reminderEnabled: timedKind(kind) ? current.reminderEnabled : false,
    }));
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (busy || validationError) return;
    const timed = timedKind(draft.kind);
    const cycling = draft.kind === "cycling";
    void onSave({
      medicationId: medication.medicationId,
      revision: medication.revision,
      kind: draft.kind,
      zoneId: timed ? draft.zoneId.trim() : "",
      civilTimes: timed ? [...draft.civilTimes].sort() : [],
      daysOn: cycling ? draft.daysOn : 0,
      daysOff: cycling ? draft.daysOff : 0,
      cycleStartedOn: cycling ? draft.cycleStartedOn : "",
      reminderEnabled: timed && draft.reminderEnabled,
      clinicianRule: draft.clinicianRule.trim(),
    }).then(onCancel, () => undefined);
  };

  return (
    <section className="medication-schedule-editor" aria-labelledby="medication-schedule-title">
      <header>
        <div>
          <p className="section-kicker">Revision {medication.revision}</p>
          <h2 id="medication-schedule-title">Schedule: {medication.label}</h2>
        </div>
        <button className="text-button" type="button" disabled={busy} onClick={onCancel}>
          Close
        </button>
      </header>

      <p className="medication-schedule-boundary">
        ZeitBoard stores only the rule you enter. It does not infer drug timing, recommend a
        schedule, or move a dose.
      </p>

      <form onSubmit={submit}>
        <label>
          <span>Schedule type</span>
          <select
            value={draft.kind}
            disabled={busy}
            onChange={(event) => changeKind(event.target.value as ScheduleDraft["kind"])}
          >
            <option value="none">No schedule</option>
            <option value="as_needed">As needed</option>
            <option value="fixed_clock">Fixed civil times</option>
            <option value="cycling">Cycling by civil date</option>
          </select>
        </label>

        {isTimed && (
          <>
            <label>
              <span>Schedule time zone</span>
              <input
                value={draft.zoneId}
                maxLength={64}
                autoComplete="off"
                disabled={busy}
                onChange={(event) =>
                  setDraft((current) => ({ ...current, zoneId: event.target.value }))
                }
              />
              <small>Civil times remain attached to this IANA zone through DST changes.</small>
            </label>

            <fieldset className="medication-clock-editor">
              <legend>Civil times</legend>
              <div className="medication-clock-list">
                {draft.civilTimes.map((value, index) => (
                  <div key={`${index}-${draft.civilTimes.length}`}>
                    <label>
                      <span>Time {index + 1}</span>
                      <input
                        type="time"
                        value={value}
                        disabled={busy}
                        onChange={(event) =>
                          setDraft((current) => ({
                            ...current,
                            civilTimes: current.civilTimes.map((item, itemIndex) =>
                              itemIndex === index ? event.target.value : item,
                            ),
                          }))
                        }
                      />
                    </label>
                    <button
                      className="text-button danger"
                      type="button"
                      disabled={busy || draft.civilTimes.length === 1}
                      aria-label={`Remove time ${index + 1}`}
                      onClick={() =>
                        setDraft((current) => ({
                          ...current,
                          civilTimes: current.civilTimes.filter(
                            (_item, itemIndex) => itemIndex !== index,
                          ),
                        }))
                      }
                    >
                      Remove
                    </button>
                  </div>
                ))}
              </div>
              <button
                className="text-button"
                type="button"
                disabled={busy || draft.civilTimes.length >= 8}
                onClick={() =>
                  setDraft((current) => ({
                    ...current,
                    civilTimes: [...current.civilTimes, "09:00"],
                  }))
                }
              >
                Add another time
              </button>
            </fieldset>
          </>
        )}

        {draft.kind === "cycling" && (
          <fieldset className="medication-cycle-editor">
            <legend>Civil-date cycle</legend>
            <label>
              <span>Days on</span>
              <input
                type="number"
                min={1}
                max={365}
                value={draft.daysOn}
                disabled={busy}
                onChange={(event) =>
                  setDraft((current) => ({ ...current, daysOn: Number(event.target.value) }))
                }
              />
            </label>
            <label>
              <span>Days off</span>
              <input
                type="number"
                min={1}
                max={365}
                value={draft.daysOff}
                disabled={busy}
                onChange={(event) =>
                  setDraft((current) => ({ ...current, daysOff: Number(event.target.value) }))
                }
              />
            </label>
            <label>
              <span>First on-date</span>
              <input
                type="date"
                value={draft.cycleStartedOn}
                disabled={busy}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    cycleStartedOn: event.target.value,
                  }))
                }
              />
            </label>
          </fieldset>
        )}

        {isTimed && (
          <label className="medication-check-row medication-reminder-opt-in">
            <input
              type="checkbox"
              checked={draft.reminderEnabled}
              disabled={busy}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  reminderEnabled: event.target.checked,
                }))
              }
            />
            <span>
              Show this label through Windows at the times you entered, including predicted sleep
              overlaps. Notification copy: "Reminder you set: {medication.label}."
            </span>
          </label>
        )}

        <label>
          <span>Clinician guidance, entered by you</span>
          <textarea
            rows={4}
            maxLength={500}
            value={draft.clinicianRule}
            disabled={busy}
            placeholder="Optional verbatim note; ZeitBoard does not interpret it"
            onChange={(event) =>
              setDraft((current) => ({ ...current, clinicianRule: event.target.value }))
            }
          />
          <small>This text is stored and displayed verbatim after trimming outer whitespace.</small>
        </label>

        {validationError && <p className="medication-schedule-validation">{validationError}</p>}

        <div className="medication-editor-actions">
          <button className="button ghost compact" type="button" disabled={busy} onClick={onCancel}>
            Cancel
          </button>
          <button
            className="button primary compact"
            type="submit"
            disabled={busy || Boolean(validationError)}
          >
            {busy ? "Saving..." : "Save schedule revision"}
          </button>
        </div>
      </form>
    </section>
  );
}
