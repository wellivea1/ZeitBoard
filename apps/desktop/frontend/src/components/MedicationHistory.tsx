import { useState, type FormEvent } from "react";
import {
  medicationDeleteConfirmation,
  type MedicationEventCorrectionInput,
  type MedicationLog,
} from "../data/medications";

export function MedicationHistory({
  events,
  busy,
  onCorrect,
  onDelete,
}: {
  events: MedicationLog[];
  busy: boolean;
  onCorrect: (input: MedicationEventCorrectionInput) => Promise<void>;
  onDelete: (eventId: string) => Promise<void>;
}) {
  const [editing, setEditing] = useState<MedicationLog | null>(null);
  const [erasing, setErasing] = useState<MedicationLog | null>(null);
  const [confirmation, setConfirmation] = useState("");

  const save = (event: FormEvent) => {
    event.preventDefault();
    if (!editing || busy) return;
    void onCorrect({
      eventId: editing.eventId,
      doseLocal: editing.doseLocal,
      zoneId: editing.zoneId,
      status: editing.status,
      scheduled: editing.scheduled,
      note: editing.note ?? "",
      excluded: editing.excluded,
    }).then(
      () => setEditing(null),
      () => undefined,
    );
  };

  return (
    <section className="medication-history-section" aria-labelledby="medication-history-title">
      <header className="medication-section-heading">
        <div>
          <p className="section-kicker">Civil-time history</p>
          <h2 id="medication-history-title">Medication events</h2>
        </div>
        <span>{events.length} stored</span>
      </header>
      {events.length === 0 ? (
        <div className="medication-history-empty">
          <strong>No events recorded</strong>
          <p>
            Taken and skipped entries will appear here with observed or predicted rhythm context.
          </p>
        </div>
      ) : (
        <div className="medication-ledger">
          <div className="medication-ledger-header" aria-hidden="true">
            <span>Status</span>
            <span>Medication and civil time</span>
            <span>Rhythm context</span>
            <span>Record controls</span>
          </div>
          {events.map((item) => (
            <article
              data-status={item.status}
              data-relation={item.sleepRelationKind}
              data-excluded={item.excluded || undefined}
              key={item.eventId}
            >
              <div className="medication-ledger-status">
                <strong>{item.status}</strong>
                {item.excluded && <span>Excluded</span>}
                {item.scheduled && <small>Scheduled elsewhere</small>}
              </div>
              <div className="medication-ledger-identity">
                <strong>{item.medicationLabel}</strong>
                <time>{item.civilTime}</time>
                {item.note && <p>{item.note}</p>}
                <small>{item.recordedLabel}</small>
              </div>
              <dl className="medication-rhythm-facts">
                <div>
                  <dt>Wake</dt>
                  <dd>{item.wakeRelation}</dd>
                </div>
                <div>
                  <dt>{item.sleepRelationKind === "predicted" ? "Forecast" : "Sleep"}</dt>
                  <dd>{item.sleepRelation}</dd>
                </div>
                <div>
                  <dt>Confidence</dt>
                  <dd>{item.confidence}</dd>
                </div>
              </dl>
              <div className="medication-ledger-actions">
                {item.correctionCount > 0 && (
                  <small>
                    {item.correctionCount}{" "}
                    {item.correctionCount === 1 ? "correction" : "corrections"}
                  </small>
                )}
                <button
                  className="text-button"
                  type="button"
                  disabled={busy}
                  onClick={() => {
                    setEditing({ ...item });
                    setErasing(null);
                  }}
                >
                  Correct
                </button>
                <button
                  className="text-button danger"
                  type="button"
                  disabled={busy}
                  onClick={() => {
                    setErasing(item);
                    setEditing(null);
                    setConfirmation("");
                  }}
                >
                  Erase
                </button>
              </div>
            </article>
          ))}
        </div>
      )}

      {editing && (
        <form
          className="medication-correction-editor"
          onSubmit={save}
          aria-label={`Correct ${editing.medicationLabel} event`}
        >
          <header>
            <p className="section-kicker">Append correction</p>
            <h3>{editing.medicationLabel}</h3>
          </header>
          <label>
            <span>Event time</span>
            <input
              type="datetime-local"
              value={editing.doseLocal}
              disabled={busy}
              onChange={(event) =>
                setEditing((current) =>
                  current ? { ...current, doseLocal: event.target.value } : current,
                )
              }
            />
          </label>
          <label>
            <span>Time zone</span>
            <input
              value={editing.zoneId}
              disabled={busy}
              onChange={(event) =>
                setEditing((current) =>
                  current ? { ...current, zoneId: event.target.value } : current,
                )
              }
            />
          </label>
          <label>
            <span>Status</span>
            <select
              value={editing.status}
              disabled={busy}
              onChange={(event) =>
                setEditing((current) =>
                  current
                    ? { ...current, status: event.target.value as MedicationLog["status"] }
                    : current,
                )
              }
            >
              <option value="taken">Taken</option>
              <option value="skipped">Skipped</option>
            </select>
          </label>
          <label className="medication-correction-note">
            <span>Private note</span>
            <input
              value={editing.note ?? ""}
              maxLength={500}
              disabled={busy}
              onChange={(event) =>
                setEditing((current) =>
                  current ? { ...current, note: event.target.value } : current,
                )
              }
            />
          </label>
          <label className="medication-check-row">
            <input
              type="checkbox"
              checked={editing.scheduled}
              disabled={busy}
              onChange={(event) =>
                setEditing((current) =>
                  current ? { ...current, scheduled: event.target.checked } : current,
                )
              }
            />
            <span>Corresponds to a schedule recorded elsewhere</span>
          </label>
          <label className="medication-check-row">
            <input
              type="checkbox"
              checked={editing.excluded}
              disabled={busy}
              onChange={(event) =>
                setEditing((current) =>
                  current ? { ...current, excluded: event.target.checked } : current,
                )
              }
            />
            <span>Exclude from adherence summaries without erasing evidence</span>
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
            <button className="button primary compact" type="submit" disabled={busy}>
              Append correction
            </button>
          </div>
        </form>
      )}

      {erasing && (
        <section className="medication-event-erasure" aria-label="Erase medication event">
          <div>
            <p className="section-kicker">Permanent local erasure</p>
            <h3>
              {erasing.medicationLabel} - {erasing.civilTime}
            </h3>
            <p>Correction history for this event will also be removed.</p>
          </div>
          <label>
            <span>Type {medicationDeleteConfirmation}</span>
            <input
              value={confirmation}
              disabled={busy}
              onChange={(event) => setConfirmation(event.target.value)}
            />
          </label>
          <div className="medication-editor-actions">
            <button
              className="button ghost compact"
              type="button"
              disabled={busy}
              onClick={() => setErasing(null)}
            >
              Cancel
            </button>
            <button
              className="button danger compact"
              type="button"
              disabled={busy || confirmation !== medicationDeleteConfirmation}
              onClick={() =>
                void onDelete(erasing.eventId).then(
                  () => {
                    setErasing(null);
                    setConfirmation("");
                  },
                  () => undefined,
                )
              }
            >
              Erase event
            </button>
          </div>
        </section>
      )}
    </section>
  );
}
