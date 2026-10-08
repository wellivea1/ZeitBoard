import { ConfirmDelete } from "./ConfirmDelete";
import { useEffect, useState } from "react";
import { Notice } from "./Notice";
import { CalendarImportPanel, type CalDAVRefill } from "./CalendarImportPanel";
import { CalendarWriteBackForm, CalendarWriteBackStatus } from "./CalendarWriteBack";
import { refreshCalendarSource, removeCalendarSource, type CalendarSource } from "../data/calendar";
import {
  calendarWriteBackChangedEvent,
  loadCalendarWriteBack,
  type CalendarWriteBack,
} from "../data/calendarWriteBack";

// The calendars ZeitBoard reads, and adding another. This used to sit in a
// narrow column beside the calendar board; it is set up once and rarely
// touched, so it lives in Data Sources with the other inputs. A CalDAV
// calendar can also take the times the owner accepts (ADR-0053).

const kindLabels: Record<CalendarSource["kind"], string> = {
  ics: "Calendar file",
  caldav: "CalDAV account",
  zeitboard: "ZeitBoard",
};

// The write-back status, re-read when the writer reports progress and when the
// calendars change (removing the calendar written to stops writing).
function useCalendarWriteBack(sources: CalendarSource[]) {
  const [writeBack, setWriteBack] = useState<CalendarWriteBack | undefined>();
  useEffect(() => {
    let current = true;
    const load = () => {
      void loadCalendarWriteBack().then(
        (status) => {
          if (current) setWriteBack(status);
        },
        () => {
          if (current) setWriteBack(undefined);
        },
      );
    };
    load();
    window.addEventListener(calendarWriteBackChangedEvent, load);
    return () => {
      current = false;
      window.removeEventListener(calendarWriteBackChangedEvent, load);
    };
  }, [sources]);
  return [writeBack, setWriteBack] as const;
}

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
  const [refill, setRefill] = useState<CalDAVRefill | undefined>();
  const [refreshed, setRefreshed] = useState("");
  const [offering, setOffering] = useState<string | null>(null);
  const [writeBack, setWriteBack] = useCalendarWriteBack(sources);
  const sourceBeingRemoved = sources.find((source) => source.sourceId === removing);
  const writingTo = writeBack?.on ? writeBack.sourceId : undefined;

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

  // The calendar written to keeps its sign-in and refreshes at once; any other
  // opens the form at its address, and the owner signs in again.
  const refresh = (source: CalendarSource) => {
    if (busy) return;
    setError("");
    setRefreshed("");
    if (writingTo !== source.sourceId) {
      setRefill({ endpoint: source.endpoint ?? "", label: source.label });
      setAdding(true);
      return;
    }
    setBusy(true);
    void refreshCalendarSource(source.sourceId).then(
      (report) => {
        setBusy(false);
        setRefreshed(`${source.label}: ${report.message}`);
        onChanged();
      },
      (reason: unknown) => {
        setBusy(false);
        setError(reason instanceof Error ? reason.message : "The calendar could not be refreshed.");
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
            onClick={() => {
              setRefill(undefined);
              setAdding((current) => !current);
            }}
          >
            {adding ? "Done adding" : "Add a calendar"}
          </button>
        )}
      </div>
      <Notice id="calendars.ownership">
        Suggested times stay clear of busy events. The times you accept are kept in ZeitBoard, and
        can also be written to a CalDAV calendar of yours as ZeitBoard’s own events. Your own events
        are never changed.
      </Notice>
      {sources.length === 0 ? (
        <p className="calendar-source-empty">No calendars yet. Adding one is optional.</p>
      ) : (
        <ul className="calendar-source-list">
          {sources.map((source) => {
            const writingHere = writingTo === source.sourceId;
            const canOffer =
              available && writeBack !== undefined && !writeBack.on && source.kind === "caldav";
            return (
              <li
                className="calendar-source-row"
                data-kind={source.kind}
                data-writing={writingHere || undefined}
                key={source.sourceId}
              >
                <div className="calendar-source-name">
                  <strong>{source.label}</strong>
                  <span>
                    {kindLabels[source.kind]} · {source.coverageLabel}
                  </span>
                  {source.endpoint && (
                    <small className="calendar-source-endpoint">{source.endpoint}</small>
                  )}
                </div>
                <div className="calendar-source-actions">
                  {source.kind === "caldav" && available && (
                    <button
                      className="button ghost compact"
                      type="button"
                      disabled={busy}
                      aria-label={`Refresh ${source.label}`}
                      onClick={() => refresh(source)}
                    >
                      Refresh
                    </button>
                  )}
                  {canOffer && offering !== source.sourceId && (
                    <button
                      className="button ghost compact"
                      type="button"
                      aria-label={`Write accepted times to ${source.label}`}
                      onClick={() => setOffering(source.sourceId)}
                    >
                      Write accepted times here
                    </button>
                  )}
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
                </div>
                {canOffer && offering === source.sourceId && (
                  <CalendarWriteBackForm
                    sourceId={source.sourceId}
                    label={source.label}
                    onStarted={(status) => {
                      setOffering(null);
                      setWriteBack(status);
                    }}
                    onCancel={() => setOffering(null)}
                  />
                )}
                {writingHere && writeBack && (
                  <CalendarWriteBackStatus status={writeBack} onChanged={setWriteBack} />
                )}
              </li>
            );
          })}
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
          {writingTo === sourceBeingRemoved.sourceId && (
            <p>
              ZeitBoard also stops writing accepted times to it. What it wrote stays there, and the
              sign-in is erased.
            </p>
          )}
        </ConfirmDelete>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {refreshed && (
        <p className="form-status" role="status">
          {refreshed}
        </p>
      )}
      {adding && (
        <CalendarImportPanel
          key={refill ? `refill-${refill.endpoint}` : "add"}
          available={available}
          zoneId={zoneId}
          onChanged={onChanged}
          refill={refill}
        />
      )}
    </section>
  );
}
