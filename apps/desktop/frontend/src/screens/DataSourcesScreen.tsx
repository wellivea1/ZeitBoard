import { useEffect, useState } from "react";
import { PageHeader } from "../components/AppShell";
import { ActivityExportPanel } from "../components/ActivityExportPanel";
import { CalendarSourcesPanel } from "../components/CalendarSourcesPanel";
import { DataSourceStatusPanel } from "../components/DataSourceStatusPanel";
import { SleepImportPanel } from "../components/SleepImportPanel";
import { loadActivityCollection, type ActivityCollection } from "../data/activityCollection";
import { loadBackendSyncStatus, type BackendSyncStatus } from "../data/backendSync";
import {
  calendarDataChangedEvent,
  loadCalendar,
  notifyCalendarDataChanged,
  todayCivilDate,
  type CalendarSource,
} from "../data/calendar";
import { loadSleepSources, sleepSourcesUnavailable } from "../data/sleepEntries";
import { loadSourceStatuses } from "../data/sourceStatus";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import { useLoaded } from "../state/useLoaded";
import { localZone } from "../utils/civilTime";

// Data Sources is about where records come from: what is connected, and how to
// bring more in. Recording last night is in Log. Calendars moved here from
// beside the calendar board, because adding one is set-up, not planning.

interface CalendarSources {
  ready: boolean;
  available: boolean;
  sources: CalendarSource[];
}

function useCalendarSources(): CalendarSources {
  const [state, setState] = useState<CalendarSources>({
    ready: false,
    available: false,
    sources: [],
  });
  useEffect(() => {
    let current = true;
    const load = () => {
      // Read in this computer's zone, as on Home and Plan. It was New York's
      // for everyone.
      const zoneId = localZone();
      void loadCalendar({ startCivilDate: todayCivilDate(zoneId), days: 1, zoneId }).then(
        (result) => {
          if (!current) return;
          setState({
            ready: true,
            available: result.source === "local",
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
  // What the sleep log is made of, counted by the desktop.
  const { data: sleepSources } = useLoaded(loadSleepSources, {
    events: [sleepDataChangedEvent],
    fallback: sleepSourcesUnavailable,
  });
  const { data: sourceStatuses } = useLoaded(loadSourceStatuses, {
    events: [sleepDataChangedEvent],
    fallback: () => [],
  });
  const [syncStatus, setSyncStatus] = useState<BackendSyncStatus | undefined>(undefined);
  const calendars = useCalendarSources();
  // It says what is happening now, so it is read again as time passes, and a
  // read that fails says so (null) rather than leave the last answer standing.
  const { data: activity } = useLoaded<ActivityCollection | null>(loadActivityCollection, {
    expires: true,
    fallback: () => null,
  });

  useEffect(() => {
    let current = true;
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
          sleepSources={sleepSources}
          sourceStatuses={sourceStatuses}
          syncStatus={syncStatus}
          calendarSources={calendars.ready ? calendars.sources : undefined}
          activity={activity}
        />
        <CalendarSourcesPanel
          sources={calendars.sources}
          available={calendars.available}
          onChanged={notifyCalendarDataChanged}
        />
        <SleepImportPanel />
        <ActivityExportPanel />
        <p className="data-source-log-pointer">
          Individual nights, their corrections, and deleting them are in{" "}
          <a href="#/log/sleep">Log</a>.
        </p>
      </section>
    </>
  );
}
