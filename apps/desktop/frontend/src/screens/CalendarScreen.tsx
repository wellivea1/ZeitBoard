import { useCallback, useEffect, useState } from "react";
import { PageHeader, PlaceholderNotice } from "../components/AppShell";
import { CalendarBoard } from "../components/CalendarBoard";
import { Icon } from "../components/Icon";
import {
  addCivilDays,
  calendarDataChangedEvent,
  downloadCalendarExport,
  exportOwnedCalendar,
  hasLocalCalendarService,
  loadCalendar,
  todayCivilDate,
  type CalendarData,
} from "../data/calendar";

const calendarZone = "America/New_York";
const visibleDays = 5;

// Plan > Calendar is the board: fixed events against predicted sleep, five days
// at a time. Adding and removing calendars moved to Data Sources with the other
// inputs. Beside the board they took a 272-pixel column of import forms and
// squeezed the board until the hours it exists to show were clipped.

function dayLabel(civilDate: string) {
  return new Date(`${civilDate}T12:00:00`).toLocaleDateString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
  });
}

function rangeTitle(startDate: string) {
  return `${dayLabel(startDate)} – ${dayLabel(addCivilDays(startDate, visibleDays - 1))}`;
}

function sourceSummary(data: CalendarData) {
  const events = new Set(data.days.flatMap((day) => day.events.map((event) => event.eventId)));
  const count = `${events.size} ${events.size === 1 ? "event" : "events"}`;
  if (data.sources.length === 0) return `${count} · no calendars added yet`;
  return `${count} · from ${data.sources.map((source) => source.label).join(", ")}`;
}

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

  const exportCalendar = () => {
    if (source !== "local" || exporting) return;
    setExporting(true);
    setError("");
    void exportOwnedCalendar().then(
      (result) => {
        setExporting(false);
        const downloaded = downloadCalendarExport(result);
        setAnnouncement(
          `${result.eventCount} accepted ${result.eventCount === 1 ? "time" : "times"} exported${downloaded ? ` to ${result.fileName}` : "."}`,
        );
      },
      (reason: unknown) => {
        setExporting(false);
        setError(reason instanceof Error ? reason.message : "Calendar export failed.");
      },
    );
  };

  const today = todayCivilDate(calendarZone);

  return (
    <>
      {!embedded && <PageHeader title="Calendar" />}
      {source === "fixture" && (
        <PlaceholderNotice>
          Sample calendar. Nothing is read from your files or written anywhere.
        </PlaceholderNotice>
      )}
      <section className="calendar-view" aria-labelledby="calendar-range-title">
        <header className="calendar-view-head">
          <div>
            <h2 id="calendar-range-title">{rangeTitle(startDate)}</h2>
            {data && (
              <p>
                {sourceSummary(data)} · <a href="#/data-sources">Manage calendars</a>
              </p>
            )}
          </div>
          <div className="calendar-toolbar">
            <div className="calendar-date-controls" role="group" aria-label="Calendar dates">
              <button
                className="button ghost compact calendar-step"
                data-direction="back"
                type="button"
                aria-label={`Previous ${visibleDays} days`}
                onClick={() => setStartDate((current) => addCivilDays(current, -visibleDays))}
              >
                <Icon name="chevron" />
              </button>
              <button
                className="button ghost compact"
                type="button"
                disabled={startDate === today}
                onClick={() => setStartDate(today)}
              >
                Today
              </button>
              <button
                className="button ghost compact calendar-step"
                type="button"
                aria-label={`Next ${visibleDays} days`}
                onClick={() => setStartDate((current) => addCivilDays(current, visibleDays))}
              >
                <Icon name="chevron" />
              </button>
              <input
                type="date"
                aria-label="Calendar start date"
                value={startDate}
                onChange={(event) => {
                  if (event.currentTarget.value) setStartDate(event.currentTarget.value);
                }}
              />
            </div>
            <button
              className="button ghost compact"
              type="button"
              disabled={source !== "local" || exporting}
              title="Save the times you accepted as an .ics file for another calendar app"
              onClick={exportCalendar}
            >
              {exporting ? "Exporting…" : "Export accepted times"}
            </button>
          </div>
        </header>

        {error && (
          <p className="calendar-error" role="alert">
            {error}
          </p>
        )}
        <p className="sr-only" role="status" aria-live="polite">
          {announcement}
        </p>
        {data?.warnings.map((warning) => (
          <p className="calendar-warning" key={warning}>
            {warning}
          </p>
        ))}
        {loading && !data ? (
          <div className="calendar-loading" role="status">
            Loading calendar…
          </div>
        ) : data ? (
          <div className="calendar-board-scroll">
            <CalendarBoard data={data} />
          </div>
        ) : (
          <div className="calendar-loading" role="status">
            Calendar unavailable.
          </div>
        )}
      </section>
    </>
  );
}
