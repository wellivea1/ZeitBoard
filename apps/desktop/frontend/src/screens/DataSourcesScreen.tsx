import { PageHeader } from "../components/AppShell";
import { ActivityExportPanel } from "../components/ActivityExportPanel";
import { CalendarSourcesPanel } from "../components/CalendarSourcesPanel";
import { DataSourceStatusPanel } from "../components/DataSourceStatusPanel";
import { SleepImportPanel } from "../components/SleepImportPanel";
import { loadActivityCollection, type ActivityCollection } from "../data/activityCollection";
import { loadBackendSyncStatus } from "../data/backendSync";
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
  available: boolean;
  sources: CalendarSource[];
}

// Read in this computer's zone, as on Home and Plan. It was New York's for
// everyone.
async function loadCalendarSources(): Promise<CalendarSources> {
  const zoneId = localZone();
  const result = await loadCalendar({ startCivilDate: todayCivilDate(zoneId), days: 1, zoneId });
  return { available: result.source === "local", sources: result.data.sources };
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
  // The sync row simply stays hidden when its status cannot be read.
  const { data: syncStatus } = useLoaded(loadBackendSyncStatus);
  // Read again whenever a calendar is added, refreshed or removed. Nothing is
  // said of them before the first read lands, and a failed read says so (null)
  // rather than claim there are none.
  const { data: calendars } = useLoaded<CalendarSources | null>(loadCalendarSources, {
    events: [calendarDataChangedEvent],
    fallback: () => null,
  });
  const calendarSources = calendars === null ? null : calendars?.sources;
  // It says what is happening now, so it is read again as time passes, and a
  // read that fails says so (null) rather than leave the last answer standing.
  const { data: activity } = useLoaded<ActivityCollection | null>(loadActivityCollection, {
    expires: true,
    fallback: () => null,
  });

  return (
    <>
      <PageHeader title="Data Sources" />
      <section className="data-source-workspace" aria-label="Data source review">
        <DataSourceStatusPanel
          sleepSources={sleepSources}
          sourceStatuses={sourceStatuses}
          syncStatus={syncStatus}
          calendarSources={calendarSources}
          activity={activity}
        />
        <CalendarSourcesPanel
          sources={calendarSources}
          available={calendars?.available ?? false}
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
