import { useEffect, useState } from "react";
import { PageHeader } from "../components/AppShell";
import { CalendarSourcesPanel } from "../components/CalendarSourcesPanel";
import { DataSourceStatusPanel } from "../components/DataSourceStatusPanel";
import { SleepImportPanel } from "../components/SleepImportPanel";
import { loadBackendSyncStatus, type BackendSyncStatus } from "../data/backendSync";
import {
  calendarDataChangedEvent,
  loadCalendar,
  notifyCalendarDataChanged,
  todayCivilDate,
  type CalendarSource,
} from "../data/calendar";
import { loadSleepEntries, type SleepEntriesData } from "../data/sleepEntries";

// Data Sources is about where records come from: what is connected, and how to
// bring more in. Recording last night is in Log. Calendars moved here from
// beside the calendar board, because adding one is set-up, not planning.

const calendarZone = "America/New_York";

interface CalendarSources {
  ready: boolean;
  available: boolean;
  zoneId: string;
  sources: CalendarSource[];
}

function useCalendarSources(): CalendarSources {
  const [state, setState] = useState<CalendarSources>({
    ready: false,
    available: false,
    zoneId: calendarZone,
    sources: [],
  });
  useEffect(() => {
    let current = true;
    const load = () => {
      void loadCalendar({
        startCivilDate: todayCivilDate(calendarZone),
        days: 1,
        zoneId: calendarZone,
      }).then(
        (result) => {
          if (!current) return;
          setState({
            ready: true,
            available: result.source === "local",
            zoneId: result.data.zoneId,
            sources: result.data.sources,
          });
        },
        () => {
          if (current) setState((previous) => ({ ...previous, ready: true }));
        },
      );
    };
    load();
    window.addEventListener(calendarDataChangedEvent, load);
    return () => {
      current = false;
      window.removeEventListener(calendarDataChangedEvent, load);
    };
  }, []);
  return state;
}

export function DataSourcesScreen() {
  const [entriesData, setEntriesData] = useState<SleepEntriesData>({
    status: "empty",
    empty: true,
    message: "Loading local sleep entries.",
    entries: [],
  });
  const [syncStatus, setSyncStatus] = useState<BackendSyncStatus | undefined>(undefined);
  const calendars = useCalendarSources();

  const refreshEntries = async () => {
    setEntriesData(await loadSleepEntries());
  };

  useEffect(() => {
    let current = true;
    void loadSleepEntries()
      .then((loaded) => {
        if (current) setEntriesData(loaded);
      })
      .catch((error: unknown) => {
        if (!current) return;
        setEntriesData({
          status: "unavailable",
          empty: true,
          message: error instanceof Error ? error.message : "Manual sleep log is unavailable.",
          entries: [],
        });
      });
    void loadBackendSyncStatus()
      .then((loaded) => {
        if (current) setSyncStatus(loaded);
      })
      .catch(() => {
        // The sync row simply stays hidden when status is unavailable.
      });
    return () => {
      current = false;
    };
  }, []);

  return (
    <>
      <PageHeader title="Data Sources" />
      <section className="data-source-workspace" aria-label="Data source review">
        <DataSourceStatusPanel
          entriesData={entriesData}
          syncStatus={syncStatus}
          calendarSources={calendars.ready ? calendars.sources : undefined}
        />
        <CalendarSourcesPanel
          sources={calendars.sources}
          available={calendars.available}
          zoneId={calendars.zoneId}
          onChanged={notifyCalendarDataChanged}
        />
        <SleepImportPanel onImported={refreshEntries} />
        <p className="data-source-log-pointer">
          Individual nights, their corrections, and deleting them are in{" "}
          <a href="#/log/sleep">Log</a>.
        </p>
      </section>
    </>
  );
}
