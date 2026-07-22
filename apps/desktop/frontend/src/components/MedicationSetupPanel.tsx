import { useState, type FormEvent } from "react";
import {
  medicationDeleteConfirmation,
  type MedicationDefinition,
  type MedicationInput,
  type MedicationScheduleInput,
  type MedicationUpdateInput,
} from "../data/medications";
import { MedicationScheduleEditor } from "./MedicationScheduleEditor";

const emptyMedication: MedicationInput = { label: "", form: "", strengthLabel: "" };

function localMedicationZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

function MedicationErasurePanel({
  medication,
  busy,
  onDelete,
  onCancel,
}: {
  medication: MedicationDefinition;
  busy: boolean;
  onDelete: (medicationId: string) => Promise<void>;
  onCancel: () => void;
}) {
  const [confirmation, setConfirmation] = useState("");

  return (
    <section className="medication-rail-section medication-erasure" aria-label="Erase medication">
      <header>
        <p className="section-kicker">Permanent local erasure</p>
        <h2>{medication.label}</h2>
      </header>
      <p>This removes the definition, every event, and every correction from local storage.</p>
      <label>
        <span>Type {medicationDeleteConfirmation} to confirm</span>
        <input
          value={confirmation}
          autoComplete="off"
          disabled={busy}
          onChange={(event) => setConfirmation(event.target.value)}
        />
      </label>
      <div className="medication-editor-actions">
        <button className="button ghost compact" type="button" disabled={busy} onClick={onCancel}>
          Cancel
        </button>
        <button
          className="button danger compact"
          type="button"
          disabled={busy || confirmation !== medicationDeleteConfirmation}
          onClick={() => void onDelete(medication.medicationId).then(onCancel, () => undefined)}
        >
          Erase medication and history
        </button>
      </div>
    </section>
  );
}

function MedicationEditor({
  medication,
  busy,
  onChange,
  onSave,
  onCancel,
}: {
  medication: MedicationDefinition;
  busy: boolean;
  onChange: (medication: MedicationDefinition) => void;
  onSave: (event: FormEvent) => void;
  onCancel: () => void;
}) {
  const changeStart = (startedLocal: string) => {
    onChange({
      ...medication,
      startedLocal: startedLocal || undefined,
      startedZoneId: startedLocal ? medication.startedZoneId || localMedicationZone() : undefined,
    });
  };

  return (
    <section
      className="medication-rail-section medication-rail-editor"
      aria-label="Edit medication"
    >
      <header>
        <p className="section-kicker">Revision {medication.revision}</p>
        <h2>Edit private label</h2>
      </header>
      <form className="medication-definition-form" onSubmit={onSave}>
        <label>
          <span>Label</span>
          <input
            value={medication.label}
            maxLength={120}
            disabled={busy}
            onChange={(event) => onChange({ ...medication, label: event.target.value })}
          />
        </label>
        <label>
          <span>Form</span>
          <input
            value={medication.form ?? ""}
            maxLength={80}
            disabled={busy}
            onChange={(event) => onChange({ ...medication, form: event.target.value })}
          />
        </label>
        <label>
          <span>Strength label</span>
          <input
            value={medication.strengthLabel ?? ""}
            maxLength={80}
            disabled={busy}
            onChange={(event) => onChange({ ...medication, strengthLabel: event.target.value })}
          />
        </label>
        <label className="medication-check-row">
          <input
            type="checkbox"
            checked={medication.active}
            disabled={busy}
            onChange={(event) => onChange({ ...medication, active: event.target.checked })}
          />
          <span>Available in quick log</span>
        </label>
        <fieldset className="medication-start-fields">
          <legend>Recorded start marker (optional)</legend>
          <label>
            <span>Local date and time</span>
            <input
              type="datetime-local"
              value={medication.startedLocal ?? ""}
              disabled={busy}
              onChange={(event) => changeStart(event.target.value)}
            />
          </label>
          <label>
            <span>IANA time zone</span>
            <input
              value={medication.startedZoneId ?? ""}
              disabled={busy || !medication.startedLocal}
              placeholder="America/New_York"
              onChange={(event) => onChange({ ...medication, startedZoneId: event.target.value })}
            />
          </label>
          <small>
            Used for descriptive before/after rhythm context. A start marker does not establish a
            medication effect.
          </small>
          {medication.startedLocal && (
            <button
              className="text-button"
              type="button"
              disabled={busy}
              onClick={() =>
                onChange({
                  ...medication,
                  startedLocal: undefined,
                  startedZoneId: undefined,
                })
              }
            >
              Clear start marker
            </button>
          )}
        </fieldset>
        <div className="medication-editor-actions">
          <button className="button ghost compact" type="button" disabled={busy} onClick={onCancel}>
            Cancel
          </button>
          <button
            className="button primary compact"
            type="submit"
            disabled={busy || !medication.label.trim()}
          >
            Save revision
          </button>
        </div>
      </form>
    </section>
  );
}

export function MedicationSetupPanel({
  medications,
  available,
  busy,
  onAdd,
  onUpdate,
  onSchedule,
  onDelete,
}: {
  medications: MedicationDefinition[];
  available: boolean;
  busy: boolean;
  onAdd: (input: MedicationInput) => Promise<void>;
  onUpdate: (input: MedicationUpdateInput) => Promise<void>;
  onSchedule: (input: MedicationScheduleInput) => Promise<void>;
  onDelete: (medicationId: string) => Promise<void>;
}) {
  const [draft, setDraft] = useState<MedicationInput>(emptyMedication);
  const [editing, setEditing] = useState<MedicationDefinition | null>(null);
  const [schedulingID, setSchedulingID] = useState("");
  const [erasing, setErasing] = useState<MedicationDefinition | null>(null);
  const scheduling =
    medications.find((medication) => medication.medicationId === schedulingID) ?? null;

  const add = (event: FormEvent) => {
    event.preventDefault();
    if (!draft.label.trim() || busy || !available) return;
    void onAdd({
      label: draft.label.trim(),
      form: draft.form.trim(),
      strengthLabel: draft.strengthLabel.trim(),
    }).then(
      () => setDraft(emptyMedication),
      () => undefined,
    );
  };

  const save = (event: FormEvent) => {
    event.preventDefault();
    if (!editing || !editing.label.trim() || busy) return;
    void onUpdate({
      medicationId: editing.medicationId,
      revision: editing.revision,
      label: editing.label.trim(),
      form: editing.form?.trim() ?? "",
      strengthLabel: editing.strengthLabel?.trim() ?? "",
      active: editing.active,
      startedLocal: editing.startedLocal ?? "",
      startedZoneId: editing.startedZoneId ?? "",
    }).then(
      () => setEditing(null),
      () => undefined,
    );
  };

  return (
    <aside className="medication-definition-rail" aria-label="Medication definitions">
      <section className="medication-rail-section" aria-labelledby="medication-add-title">
        <header>
          <p className="section-kicker">Private label</p>
          <h2 id="medication-add-title">Add medication</h2>
        </header>
        <form className="medication-definition-form" onSubmit={add}>
          <label>
            <span>Label</span>
            <input
              value={draft.label}
              maxLength={120}
              autoComplete="off"
              disabled={!available || busy}
              placeholder="Your private label"
              onChange={(event) =>
                setDraft((current) => ({ ...current, label: event.target.value }))
              }
            />
          </label>
          <div className="medication-field-pair">
            <label>
              <span>Form</span>
              <input
                value={draft.form}
                maxLength={80}
                disabled={!available || busy}
                placeholder="Optional"
                onChange={(event) =>
                  setDraft((current) => ({ ...current, form: event.target.value }))
                }
              />
            </label>
            <label>
              <span>Strength label</span>
              <input
                value={draft.strengthLabel}
                maxLength={80}
                disabled={!available || busy}
                placeholder="Optional"
                onChange={(event) =>
                  setDraft((current) => ({ ...current, strengthLabel: event.target.value }))
                }
              />
            </label>
          </div>
          <small>No schedule is inferred from the label.</small>
          <button
            className="button primary compact"
            type="submit"
            disabled={!available || busy || !draft.label.trim()}
          >
            Add private label
          </button>
        </form>
      </section>

      <section className="medication-rail-section" aria-labelledby="medication-list-title">
        <header className="medication-rail-heading">
          <div>
            <p className="section-kicker">Logging roster</p>
            <h2 id="medication-list-title">Medications</h2>
          </div>
          <span>{medications.length}</span>
        </header>
        {medications.length === 0 ? (
          <p className="medication-rail-empty">Add a label to enable quick logging.</p>
        ) : (
          <div className="medication-definition-list">
            {medications.map((medication) => (
              <article data-active={medication.active || undefined} key={medication.medicationId}>
                <div className="medication-definition-title">
                  <strong>{medication.label}</strong>
                  <span>{medication.active ? "Active" : "Archived"}</span>
                </div>
                <p>{medication.detailLabel}</p>
                <small>
                  {medication.schedule?.summary ?? "No schedule"} | {medication.eventCount}{" "}
                  {medication.eventCount === 1 ? "event" : "events"}
                </small>
                {medication.startedLabel && <small>Start marker: {medication.startedLabel}</small>}
                <div className="medication-row-actions">
                  <button
                    className="text-button"
                    type="button"
                    disabled={busy}
                    onClick={() => {
                      setEditing({ ...medication });
                      setSchedulingID("");
                      setErasing(null);
                    }}
                  >
                    Edit
                  </button>
                  <button
                    className="text-button"
                    type="button"
                    disabled={busy}
                    aria-label={`Schedule ${medication.label}`}
                    onClick={() => {
                      setSchedulingID(medication.medicationId);
                      setEditing(null);
                      setErasing(null);
                    }}
                  >
                    Schedule
                  </button>
                  <button
                    className="text-button"
                    type="button"
                    disabled={busy}
                    onClick={() =>
                      void onUpdate({
                        medicationId: medication.medicationId,
                        revision: medication.revision,
                        label: medication.label,
                        form: medication.form ?? "",
                        strengthLabel: medication.strengthLabel ?? "",
                        active: !medication.active,
                        startedLocal: medication.startedLocal ?? "",
                        startedZoneId: medication.startedZoneId ?? "",
                      }).catch(() => undefined)
                    }
                  >
                    {medication.active ? "Archive" : "Reactivate"}
                  </button>
                  <button
                    className="text-button danger"
                    type="button"
                    disabled={busy}
                    onClick={() => {
                      setErasing(medication);
                      setEditing(null);
                      setSchedulingID("");
                    }}
                  >
                    Erase
                  </button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      {scheduling && (
        <MedicationScheduleEditor
          key={`${scheduling.medicationId}-${scheduling.revision}`}
          medication={scheduling}
          busy={busy}
          onSave={onSchedule}
          onCancel={() => setSchedulingID("")}
        />
      )}

      {editing && (
        <MedicationEditor
          medication={editing}
          busy={busy}
          onChange={setEditing}
          onSave={save}
          onCancel={() => setEditing(null)}
        />
      )}

      {erasing && (
        <MedicationErasurePanel
          medication={erasing}
          busy={busy}
          onDelete={onDelete}
          onCancel={() => setErasing(null)}
        />
      )}
    </aside>
  );
}
