import { useState, type FormEvent } from "react";
import {
  medicationDeleteConfirmation,
  type MedicationDefinition,
  type MedicationInput,
  type MedicationUpdateInput,
} from "../data/medications";

const emptyMedication: MedicationInput = { label: "", form: "", strengthLabel: "" };

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

export function MedicationSetupPanel({
  medications,
  available,
  busy,
  onAdd,
  onUpdate,
  onDelete,
}: {
  medications: MedicationDefinition[];
  available: boolean;
  busy: boolean;
  onAdd: (input: MedicationInput) => Promise<void>;
  onUpdate: (input: MedicationUpdateInput) => Promise<void>;
  onDelete: (medicationId: string) => Promise<void>;
}) {
  const [draft, setDraft] = useState<MedicationInput>(emptyMedication);
  const [editing, setEditing] = useState<MedicationDefinition | null>(null);
  const [erasing, setErasing] = useState<MedicationDefinition | null>(null);

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
                  Logging only - {medication.eventCount}{" "}
                  {medication.eventCount === 1 ? "event" : "events"}
                </small>
                <div className="medication-row-actions">
                  <button
                    className="text-button"
                    type="button"
                    disabled={busy}
                    onClick={() => {
                      setEditing({ ...medication });
                      setErasing(null);
                    }}
                  >
                    Edit
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

      {editing && (
        <section
          className="medication-rail-section medication-rail-editor"
          aria-label="Edit medication"
        >
          <header>
            <p className="section-kicker">Revision {editing.revision}</p>
            <h2>Edit private label</h2>
          </header>
          <form className="medication-definition-form" onSubmit={save}>
            <label>
              <span>Label</span>
              <input
                value={editing.label}
                maxLength={120}
                disabled={busy}
                onChange={(event) =>
                  setEditing((current) =>
                    current ? { ...current, label: event.target.value } : current,
                  )
                }
              />
            </label>
            <label>
              <span>Form</span>
              <input
                value={editing.form ?? ""}
                maxLength={80}
                disabled={busy}
                onChange={(event) =>
                  setEditing((current) =>
                    current ? { ...current, form: event.target.value } : current,
                  )
                }
              />
            </label>
            <label>
              <span>Strength label</span>
              <input
                value={editing.strengthLabel ?? ""}
                maxLength={80}
                disabled={busy}
                onChange={(event) =>
                  setEditing((current) =>
                    current ? { ...current, strengthLabel: event.target.value } : current,
                  )
                }
              />
            </label>
            <label className="medication-check-row">
              <input
                type="checkbox"
                checked={editing.active}
                disabled={busy}
                onChange={(event) =>
                  setEditing((current) =>
                    current ? { ...current, active: event.target.checked } : current,
                  )
                }
              />
              <span>Available in quick log</span>
            </label>
            <div className="medication-editor-actions">
              <button
                className="button ghost compact"
                type="button"
                disabled={busy}
                onClick={() => setEditing(null)}
              >
                Cancel
              </button>
              <button
                className="button primary compact"
                type="submit"
                disabled={busy || !editing.label.trim()}
              >
                Save revision
              </button>
            </div>
          </form>
        </section>
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
