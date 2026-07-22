import type { BackendSyncStatus } from "../data/backendSync";
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

export function DataSourceStatusPanel({
  entriesData,
  syncStatus,
}: {
  entriesData: SleepEntriesData;
  syncStatus?: BackendSyncStatus;
}) {
  const summaries = summarizeSleepSources(entriesData.entries);
  const localUnavailable = entriesData.status === "unavailable";
  return (
    <section className="data-source-registry" aria-labelledby="source-registry-title">
      <div className="data-source-section-heading">
        <div>
          <p className="section-kicker">Provenance</p>
          <h2 id="source-registry-title">Source status</h2>
        </div>
        <p>What contributes data, where it is managed, and whether it is available now.</p>
      </div>
      <div className="source-ledger">
        {summaries.length === 0 ? (
          <SourceRow
            icon="clock"
            name="Local sleep observations"
            detail={
              localUnavailable
                ? entriesData.message
                : "No manual or imported observations stored yet"
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
              detail={`${summary.total} ${
                summary.total === 1 ? "observation" : "observations"
              } - ${summary.provenance}; ${summary.corrected} corrected, ${
                summary.suppressed
              } suppressed`}
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
                ? `Your own instance - ${syncStatus.pushedCount} pushed, ${syncStatus.pulledCount} pulled${
                    syncStatus.lastSyncLabel ? `, last sync ${syncStatus.lastSyncLabel}` : ""
                  }`
                : "Data stays on this device unless you enroll in Settings"
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
        <SourceRow
          icon="calendar"
          name="Calendar import"
          detail="Local ICS files and read-only CalDAV snapshots are managed in Calendar"
          status="Separate workspace"
        />
        <SourceRow
          icon="sources"
          name="Device activity"
          detail="Not connected; activity-to-sleep inference is not available"
          status="Off"
        />
      </div>
    </section>
  );
}
