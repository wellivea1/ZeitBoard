import { useEffect, useState } from "react";
import { PageHeader } from "../components/AppShell";
import { DataSourceStatusPanel } from "../components/DataSourceStatusPanel";
import { SleepImportPanel } from "../components/SleepImportPanel";
import { loadBackendSyncStatus, type BackendSyncStatus } from "../data/backendSync";
import { loadSleepEntries, type SleepEntriesData } from "../data/sleepEntries";

// Data Sources is now about where records come from: what is connected, what
// it covers, and how to bring more in. Recording last night moved to Log in
// slice U-H — the two jobs were sharing a 593-line screen because they had both
// grown there, not because they belong together.

export function DataSourcesScreen() {
  const [entriesData, setEntriesData] = useState<SleepEntriesData>({
    status: "empty",
    empty: true,
    message: "Loading local sleep entries.",
    entries: [],
  });
  const [syncStatus, setSyncStatus] = useState<BackendSyncStatus | undefined>(undefined);

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
      <PageHeader
        title="Data Sources"
        description="Review what is feeding the estimate, and import sleep episodes from a file."
      />
      <section className="data-source-workspace" aria-label="Data source review">
        <DataSourceStatusPanel entriesData={entriesData} syncStatus={syncStatus} />
        <SleepImportPanel onImported={refreshEntries} />
        <p className="data-source-log-pointer">
          Individual episodes, their correction history, suppression and permanent erasure are in{" "}
          <a href="#/log/sleep">Log</a>.
        </p>
      </section>
    </>
  );
}
