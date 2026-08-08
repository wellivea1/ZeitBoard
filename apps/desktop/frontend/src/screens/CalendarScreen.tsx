import { useCallback, useEffect, useState } from "react";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";
import { CalendarBoard } from "../components/CalendarBoard";
import { CalendarImportPanel } from "../components/CalendarImportPanel";
import { CalendarSourcesPanel } from "../components/CalendarSourcesPanel";
import {
  addCivilDays,
  calendarDataChangedEvent,
  downloadCalendarExport,
  exportOwnedCalendar,
  hasLocalCalendarService,
  loadCalendar,
  notifyCalendarDataChanged,
  todayCivilDate,
  type CalendarData,
} from "../data/calendar";

const calendarZone = "America/New_York";
const visibleDays = 5;

export function CalendarScreen({ embedded }: { embedded?: boolean } = {}) {
  const localServicePresent = hasLocalCalendarService();
  const [startDate, setStartDate] = useState(() => todayCivilDate(calendarZone));
  const [data, setData] = useState<CalendarData | null>(null);
  const [source, setSource] = useState<"local" | "fixture">(
    localServicePresent ? "local" : "fixture",
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const [exporting, setExporting] = useState(false);
  const [revision, setRevision] = useState(0);

  const refresh = useCallback(() => setRevision((current) => current + 1), []);

  useEffect(() => {
    const onChanged = () => refresh();
    window.addEventListener(calendarDataChangedEvent, onChanged);
    return () => window.removeEventListener(calendarDataChangedEvent, onChanged);
  }, [refresh]);

  useEffect(() => {
    let current = true;
    void Promise.resolve().then(() => {
      if (!current) return;
      setLoading(true);
      setError("");
      void loadCalendar({
        startCivilDate: startDate,
        days: visibleDays,
        zoneId: calendarZone,
      }).then(
        (result) => {
          if (!current) return;
          setData(result.data);
          setSource(result.source);
          setLoading(false);
        },
        (reason: unknown) => {
          if (!current) return;
          setLoading(false);
          setError(reason instanceof Error ? reason.message : "Calendar data could not be loaded.");
        },
      );
    });
    return () => {
      current = false;
    };
  }, [revision, startDate]);

  const onDataChanged = () => {
    setAnnouncement("Calendar source updated.");
    notifyCalendarDataChanged();
  };

  const exportCalendar = () => {
    if (source !== "local" || exporting) return;
    setExporting(true);
    setError("");
    void exportOwnedCalendar().then(
      (result) => {
        setExporting(false);
        const downloaded = downloadCalendarExport(result);
        setAnnouncement(
          `${result.eventCount} ZeitBoard ${result.eventCount === 1 ? "placement" : "placements"} exported${downloaded ? ` to ${result.fileName}` : "."}`,
        );
      },
      (reason: unknown) => {
        setExporting(false);
        setError(reason instanceof Error ? reason.message : "Calendar export failed.");
      },
    );
  };

  return (
    <>
      <PageHeader
        title="Calendar"
        description="Plan against real fixed events while keeping uncertain sleep and waking windows visually distinct."
        actions={
          <div className="calendar-page-actions">
            <div className="calendar-date-controls" aria-label="Calendar date range">
              <button
                className="button ghost compact"
                type="button"
                onClick={() => setStartDate((current) => addCivilDays(current, -visibleDays))}
              >
                Previous
              </button>
              <input
                type="date"
                aria-label="Calendar start date"
                value={startDate}
                onChange={(event) => {
                  if (event.currentTarget.value) setStartDate(event.currentTarget.value);
                }}
              />
              <button
                className="button ghost compact"
                type="button"
                onClick={() => setStartDate(todayCivilDate(calendarZone))}
              >
                Today
              </button>
              <button
                className="button ghost compact"
                type="button"
                onClick={() => setStartDate((current) => addCivilDays(current, visibleDays))}
              >
                Next
              </button>
            </div>
            <button
              className="button secondary compact"
              type="button"
              disabled={source !== "local" || exporting}
              onClick={exportCalendar}
            >
              {exporting ? "Exporting..." : "Export placements (.ics)"}
            </button>
          </div>
        }
        level={embedded ? "panel" : "page"}
      />

      {source === "fixture" && (
        <PlaceholderNotice>
          Sample mode is clearly isolated: it does not read files, contact CalDAV, or write
          placements.
        </PlaceholderNotice>
      )}
      {error && (
        <p className="calendar-error" role="alert">
          {error}
        </p>
      )}
      <p className="sr-only" role="status" aria-live="polite">
        {announcement}
      </p>

      <section className="calendar-workspace">
        <aside className="calendar-control-rail" aria-label="Calendar sources and import controls">
          <CalendarSourcesPanel
            sources={data?.sources ?? []}
            available={source === "local"}
            onChanged={onDataChanged}
          />
          <CalendarImportPanel
            available={source === "local"}
            zoneId={data?.zoneId ?? calendarZone}
            onChanged={onDataChanged}
          />
        </aside>

        <div className="calendar-main-column">
          {data && (
            <div className="calendar-status-line">
              <span className="sync-dot" data-mode={source} aria-hidden="true" />
              <strong>{source === "local" ? "Local calendar" : "Sample preview"}</strong>
              <span>{data.message}</span>
              <small>{data.updatedLabel}</small>
            </div>
          )}
          {data?.warnings.map((warning) => (
            <p className="calendar-warning" key={warning}>
              {warning}
            </p>
          ))}
          {loading && !data ? (
            <div className="calendar-loading" role="status">
              Loading calendar...
            </div>
          ) : data ? (
            <CalendarBoard data={data} />
          ) : (
            <div className="calendar-loading" role="status">
              Calendar unavailable.
            </div>
          )}
        </div>
      </section>
    </>
  );
}
