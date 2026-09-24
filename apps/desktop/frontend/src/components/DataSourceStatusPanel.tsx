import type { BackendSyncStatus } from "../data/backendSync";
import type { CalendarSource } from "../data/calendar";
import { summarizeSleepSources, type SleepEntriesData } from "../data/sleepEntries";
import { Icon } from "./Icon";

type SourceRowProps = {
  detail: string;
  icon: "calendar" | "clock" | "sources";
  name: string;
  state?: "available" | "error" | "off";
  status: string;
};

function SourceRow({ detail, icon, name, state = "off", status }: SourceRowProps) {
  return (
    <article className="source-ledger-row">
      <Icon name={icon} />
      <div>
        <h3>{name}</h3>
        <p>{detail}</p>
      </div>
      <span className="source-ledger-state" data-state={state}>
        {status}
      </span>
    </article>
  );
}

function sleepDetail(summary: ReturnType<typeof summarizeSleepSources>[number]) {
  const changed = [
    summary.corrected > 0 ? `${summary.corrected} corrected` : "",
    summary.suppressed > 0 ? `${summary.suppressed} hidden` : "",
  ].filter(Boolean);
  return [
    `${summary.total} ${summary.total === 1 ? "record" : "records"}`,
    summary.provenance,
    ...changed,
  ].join(" · ");
}

export function DataSourceStatusPanel({
  entriesData,
  syncStatus,
  calendarSources,
}: {
  entriesData: SleepEntriesData;
  syncStatus?: BackendSyncStatus;
  /** Undefined while loading; the row waits rather than claiming none. */
  calendarSources?: CalendarSource[];
}) {
  const summaries = summarizeSleepSources(entriesData.entries);
  const localUnavailable = entriesData.status === "unavailable";
  const importedCalendars = calendarSources?.filter((source) => source.readOnly) ?? [];
  return (
    <section className="data-source-registry" aria-labelledby="source-registry-title">
      <div className="data-source-section-heading">
        <h2 id="source-registry-title">Connected</h2>
      </div>
      <div className="source-ledger">
        {summaries.length === 0 ? (
          <SourceRow
            icon="clock"
            name="Sleep records"
            detail={
              localUnavailable ? entriesData.message : "None yet. Log a night or import a file."
            }
            status={localUnavailable ? "Unavailable" : "Empty"}
            state={localUnavailable ? "error" : "off"}
          />
        ) : (
          summaries.map((summary) => (
            <SourceRow
              key={`${summary.source}-${summary.provenance}`}
              icon={summary.source === "Manual sleep log" ? "clock" : "sources"}
              name={summary.source}
              detail={sleepDetail(summary)}
              status="Available"
              state="available"
            />
          ))
        )}
        {syncStatus && (
          <SourceRow
            icon="sources"
            name="Server sync"
            detail={
              syncStatus.enabled
                ? `Your own server · ${syncStatus.pushedCount} sent, ${syncStatus.pulledCount} received${
                    syncStatus.lastSyncLabel ? ` · last ${syncStatus.lastSyncLabel}` : ""
                  }`
                : "Everything stays on this device. Turn on in Settings."
            }
            status={
              syncStatus.status === "connected"
                ? "Connected"
                : syncStatus.status === "error"
                  ? "Error"
                  : "Off"
            }
            state={
              syncStatus.status === "connected"
                ? "available"
                : syncStatus.status === "error"
                  ? "error"
                  : "off"
            }
          />
        )}
        {calendarSources && (
          <SourceRow
            icon="calendar"
            name="Calendars"
            detail={
              importedCalendars.length > 0
                ? importedCalendars.map((source) => source.label).join(" · ")
                : "None added. Suggested times only avoid what ZeitBoard knows about."
            }
            status={importedCalendars.length > 0 ? `${importedCalendars.length} added` : "None"}
            state={importedCalendars.length > 0 ? "available" : "off"}
          />
        )}
        <SourceRow icon="sources" name="Device activity" detail="Not connected" status="Off" />
      </div>
    </section>
  );
}
