import { ConfirmDelete } from "./ConfirmDelete";
import { Loading } from "./Loading";
import { useState, type FormEvent } from "react";
import {
  loadMedicationHistoryPage,
  medicationDataChangedEvent,
  type MedicationEventCorrectionInput,
  type MedicationLog,
} from "../data/medications";
import { useLoaded } from "../state/useLoaded";

function MedicationHistoryPagination({
  eventCount,
  firstEvent,
  shown,
  page,
  pageCount,
  onPageChange,
}: {
  eventCount: number;
  firstEvent: number;
  shown: number;
  page: number;
  pageCount: number;
  onPageChange: (page: number) => void;
}) {
  if (pageCount <= 1) return null;

  return (
    <nav className="medication-history-pagination" aria-label="Medication event pages">
      <span>
        Events {firstEvent + 1}-{firstEvent + shown} of {eventCount}
      </span>
      <button
        className="button ghost compact"
        type="button"
        disabled={page === 0}
        onClick={() => onPageChange(Math.max(0, page - 1))}
      >
        Previous events
      </button>
      <button
        className="button ghost compact"
        type="button"
        disabled={page >= pageCount - 1}
        onClick={() => onPageChange(Math.min(pageCount - 1, page + 1))}
      >
        Next events
      </button>
    </nav>
  );
}

// The dose history, read a page at a time from the desktop and again after any
// change to medication data.
export function MedicationHistory({
  busy,
  onCorrect,
  onDelete,
}: {
  busy: boolean;
  onCorrect: (input: MedicationEventCorrectionInput) => Promise<void>;
  onDelete: (eventId: string) => Promise<void>;
}) {
  const [editing, setEditing] = useState<MedicationLog | null>(null);
  const [erasing, setErasing] = useState<MedicationLog | null>(null);
  const [requestedPage, setPage] = useState(0);
  const { data: history } = useLoaded(() => loadMedicationHistoryPage(requestedPage), {
    events: [medicationDataChangedEvent],
    key: requestedPage,
  });
  // The desktop says which page it read: one asked for past the end, as
  // after deleting the last page's only dose, comes back as the last.
  const events = history?.events ?? [];
  const total = history?.total ?? 0;
  const page = history?.page ?? 0;
  const pageSize = history?.pageSize ?? 0;
  const pageCount = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;

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
          <h2 id="medication-history-title">History</h2>
        </div>
        {history && <span>{total} stored</span>}
      </header>
      {!history ? (
        <Loading />
      ) : total === 0 ? (
        <div className="medication-history-empty">
          <strong>No events recorded</strong>
          <p>
            Taken and skipped entries will appear here with observed or predicted rhythm context.
          </p>
        </div>
      ) : (
        <>
          <MedicationHistoryPagination
            eventCount={total}
            firstEvent={page * pageSize}
            shown={events.length}
            page={page}
            pageCount={pageCount}
            onPageChange={setPage}
          />
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
                    aria-label={`Edit ${item.medicationLabel}, ${item.civilTime}`}
                    onClick={() => {
                      setEditing({ ...item });
                      setErasing(null);
                    }}
                  >
                    Edit
                  </button>
                  <button
                    className="text-button danger"
                    type="button"
                    disabled={busy}
                    aria-label={`Delete ${item.medicationLabel}, ${item.civilTime}`}
                    onClick={() => {
                      setErasing(item);
                      setEditing(null);
                    }}
                  >
                    Delete
                  </button>
                </div>
              </article>
            ))}
          </div>
        </>
      )}

      {editing && (
        <form
          className="medication-correction-editor"
          onSubmit={save}
          aria-label={`Edit ${editing.medicationLabel}, ${editing.civilTime}`}
        >
          <header>
            <p className="section-kicker">Edits keep the original</p>
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
            <span>Exclude from adherence summaries without deleting the record</span>
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
              Save correction
            </button>
          </div>
        </form>
      )}

      {erasing && (
        <ConfirmDelete
          question={`Delete ${erasing.medicationLabel}, ${erasing.civilTime}, for good?`}
          action="Delete record"
          busy={busy}
          onConfirm={() =>
            void onDelete(erasing.eventId).then(
              () => setErasing(null),
              () => undefined,
            )
          }
          onCancel={() => setErasing(null)}
        >
          <p>Its corrections are deleted with it.</p>
        </ConfirmDelete>
      )}
    </section>
  );
}
