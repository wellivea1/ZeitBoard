import { ConfirmDelete } from "./ConfirmDelete";
import { useState } from "react";
import { Notice } from "./Notice";
import { CalendarImportPanel } from "./CalendarImportPanel";
import { removeCalendarSource, type CalendarSource } from "../data/calendar";

// The calendars ZeitBoard reads, and adding another. This used to sit in a
// narrow column beside the calendar board; it is set up once and rarely
// touched, so it lives in Data Sources with the other inputs.

const kindLabels: Record<CalendarSource["kind"], string> = {
  ics: "Calendar file",
  caldav: "CalDAV account",
  zeitboard: "ZeitBoard",
};

export function CalendarSourcesPanel({
  sources,
  available,
  zoneId = "America/New_York",
  onChanged,
}: {
  sources: CalendarSource[];
  available: boolean;
  zoneId?: string;
  onChanged: () => void;
}) {
  const [removing, setRemoving] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [adding, setAdding] = useState(false);
  const sourceBeingRemoved = sources.find((source) => source.sourceId === removing);

  const remove = (sourceId: string) => {
    if (busy) return;
    setBusy(true);
    setError("");
    void removeCalendarSource(sourceId).then(
      () => {
        setBusy(false);
        setRemoving(null);
        onChanged();
      },
      (reason: unknown) => {
        setBusy(false);
        setError(reason instanceof Error ? reason.message : "Calendar source removal failed.");
      },
    );
  };

  return (
    <section className="calendar-sources-panel" aria-labelledby="calendar-sources-title">
      <div className="data-source-section-heading">
        <h2 id="calendar-sources-title">Calendars</h2>
        {available && (
          <button
            className="button secondary compact"
            type="button"
            aria-expanded={adding}
            onClick={() => setAdding((current) => !current)}
          >
            {adding ? "Done adding" : "Add a calendar"}
          </button>
        )}
      </div>
      <Notice id="calendars.ownership">
        Suggested times stay clear of busy events. ZeitBoard never changes your calendars: the times
        you accept are kept in its own placements calendar.
      </Notice>
      {sources.length === 0 ? (
        <p className="calendar-source-empty">No calendars yet. Adding one is optional.</p>
      ) : (
        <ul className="calendar-source-list">
          {sources.map((source) => (
            <li className="calendar-source-row" data-kind={source.kind} key={source.sourceId}>
              <div>
                <strong>{source.label}</strong>
                <span>
                  {kindLabels[source.kind]} · {source.coverageLabel}
                </span>
                {source.endpoint && (
                  <small className="calendar-source-endpoint">{source.endpoint}</small>
                )}
              </div>
              {source.readOnly && available && (
                <button
                  className="button ghost compact"
                  type="button"
                  aria-label={`Remove ${source.label}`}
                  onClick={() => {
                    setRemoving(source.sourceId);
                    setError("");
                  }}
                >
                  Remove
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {sourceBeingRemoved && (
        <ConfirmDelete
          question={`Remove ${sourceBeingRemoved.label} from ZeitBoard?`}
          action="Remove calendar"
          busyAction="Removing…"
          word="REMOVE"
          busy={busy}
          onConfirm={() => remove(sourceBeingRemoved.sourceId)}
          onCancel={() => setRemoving(null)}
        >
          <p>Its events are deleted from ZeitBoard. The calendar itself is not changed.</p>
        </ConfirmDelete>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {adding && (
        <CalendarImportPanel available={available} zoneId={zoneId} onChanged={onChanged} />
      )}
    </section>
  );
}
