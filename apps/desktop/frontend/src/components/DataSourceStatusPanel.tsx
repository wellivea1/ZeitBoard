import type { BackendSyncStatus } from "../data/backendSync";
import { summarizeSleepSources, type SleepEntriesData } from "../data/sleepEntries";
import { Icon } from "./Icon";

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
    <section className="panel source-list source-status-panel" aria-label="Data source status">
      {summaries.length === 0 ? (
        <article>
          <div className="source-mark">
            <Icon name="clock" />
          </div>
          <div>
            <h2>Local sleep observations</h2>
            <p>
              {localUnavailable
                ? entriesData.message
                : "No manual or imported observations stored yet"}
            </p>
          </div>
          <span className="source-status">{localUnavailable ? "Unavailable" : "Empty"}</span>
        </article>
      ) : (
        summaries.map((summary) => (
          <article key={`${summary.source}-${summary.provenance}`}>
            <div className="source-mark">
              <Icon name={summary.source === "Manual sleep log" ? "clock" : "sources"} />
            </div>
            <div>
              <h2>{summary.source}</h2>
              <p>
                {summary.total} {summary.total === 1 ? "observation" : "observations"} -{" "}
                {summary.provenance}; {summary.corrected} corrected, {summary.suppressed} suppressed
              </p>
            </div>
            <span className="source-status connected">Available</span>
          </article>
        ))
      )}
      {syncStatus && (
        <article>
          <div className="source-mark">
            <Icon name="sources" />
          </div>
          <div>
            <h2>Server sync</h2>
            <p>
              {syncStatus.enabled
                ? `Your own instance - ${syncStatus.pushedCount} pushed, ${syncStatus.pulledCount} pulled${
                    syncStatus.lastSyncLabel ? `, last sync ${syncStatus.lastSyncLabel}` : ""
                  }`
                : "Off - data stays on this device unless you enroll in Settings"}
            </p>
          </div>
          <span className={`source-status ${syncStatus.status === "connected" ? "connected" : ""}`}>
            {syncStatus.status === "connected"
              ? "Connected"
              : syncStatus.status === "error"
                ? "Error"
                : "Off"}
          </span>
        </article>
      )}
      <article>
        <div className="source-mark">
          <Icon name="calendar" />
        </div>
        <div>
          <h2>Calendar import</h2>
          <p>Out of scope for this local sleep-data slice</p>
        </div>
        <span className="source-status">Future</span>
      </article>
      <article>
        <div className="source-mark">
          <Icon name="sources" />
        </div>
        <div>
          <h2>Device activity</h2>
          <p>Not connected - optional supporting observation later</p>
        </div>
        <span className="source-status">Off</span>
      </article>
    </section>
  );
}
