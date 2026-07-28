import { useCallback, useEffect, useState } from "react";
import { PageHeader } from "../components/AppShell";
import {
  configureBackendSync,
  disableBackendSync,
  loadBackendSyncStatus,
  syncNow,
  type BackendSyncInput,
  type BackendSyncStatus,
} from "../data/backendSync";
import { loadLocalAgentStatus, type LocalAgentStatus } from "../data/localAgent";
import { deleteConfirmationToken, downloadSleepDataExport } from "../data/sleepDataControl";
import { notifySleepDataChanged } from "../data/sleepDataEvents";
import { deleteAllSleepData, exportSleepData, type SleepDataExport } from "../data/sleepEntries";
import { AppearanceSettings } from "./settings/AppearanceSettings";
import { BackendSyncSettings } from "./settings/BackendSyncSettings";
import { LocalAgentSettings } from "./settings/LocalAgentSettings";
import { SleepDataSettings } from "./settings/SleepDataSettings";

const initialBackendSyncStatus: BackendSyncStatus = {
  enabled: false,
  status: "off",
  backendUrl: "",
  deviceId: "",
  insecureSkipVerify: false,
  lastSyncLabel: "Not synced yet",
  lastError: "",
  pendingPushCount: 0,
  pushedCount: 0,
  pulledCount: 0,
  cursor: 0,
};

function initialBackendSyncForm(status = initialBackendSyncStatus): BackendSyncInput {
  return {
    enabled: true,
    backendUrl: status.backendUrl,
    enrollmentSecret: "",
    deviceLabel: "ZeitBoard desktop",
    insecureSkipVerify: status.insecureSkipVerify,
  };
}

export function SettingsScreen() {
  const [exportedSleepData, setExportedSleepData] = useState<SleepDataExport | null>(null);
  const [dataControlStatus, setDataControlStatus] = useState("");
  const [dataControlError, setDataControlError] = useState("");
  const [deleteAllConfirmation, setDeleteAllConfirmation] = useState("");
  const [dataControlBusy, setDataControlBusy] = useState(false);
  const [backendSyncStatus, setBackendSyncStatus] =
    useState<BackendSyncStatus>(initialBackendSyncStatus);
  const [backendSyncForm, setBackendSyncForm] = useState<BackendSyncInput>(() =>
    initialBackendSyncForm(),
  );
  const [backendSyncMessage, setBackendSyncMessage] = useState("");
  const [backendSyncError, setBackendSyncError] = useState("");
  const [backendSyncBusy, setBackendSyncBusy] = useState(false);
  const [localAgentStatus, setLocalAgentStatus] = useState<LocalAgentStatus | null>(null);
  const [localAgentError, setLocalAgentError] = useState("");

  const refreshLocalAgentStatus = useCallback(async () => {
    try {
      const status = await loadLocalAgentStatus();
      setLocalAgentStatus(status);
      setLocalAgentError("");
    } catch (error) {
      setLocalAgentError(
        error instanceof Error ? error.message : "Could not read desktop-local agent status.",
      );
    }
  }, []);

  // Polls through an explicit promise chain (like the backend-sync effect
  // below) so state only ever settles after the await boundary, and never
  // after unmount.
  useEffect(() => {
    let current = true;
    const refresh = () => {
      void loadLocalAgentStatus()
        .then((status) => {
          if (!current) return;
          setLocalAgentStatus(status);
          setLocalAgentError("");
        })
        .catch((error: unknown) => {
          if (!current) return;
          setLocalAgentError(
            error instanceof Error ? error.message : "Could not read desktop-local agent status.",
          );
        });
    };
    refresh();
    const interval = window.setInterval(refresh, 15_000);
    return () => {
      current = false;
      window.clearInterval(interval);
    };
  }, []);

  useEffect(() => {
    let current = true;
    void loadBackendSyncStatus()
      .then((status) => {
        if (!current) return;
        setBackendSyncStatus(status);
        setBackendSyncForm((form) => ({
          ...form,
          backendUrl: status.backendUrl || form.backendUrl,
          insecureSkipVerify: status.insecureSkipVerify,
        }));
      })
      .catch((error: unknown) => {
        if (!current) return;
        setBackendSyncError(
          error instanceof Error ? error.message : "Could not read backend sync status.",
        );
      });
    return () => {
      current = false;
    };
  }, []);

  const handleExportSleepData = async () => {
    setDataControlBusy(true);
    setDataControlError("");
    try {
      const exported = await exportSleepData();
      setExportedSleepData(exported);
      const downloaded = downloadSleepDataExport(exported);
      setDataControlStatus(
        `${downloaded ? "Downloaded" : "Prepared"} ${exported.observationCount} ${
          exported.observationCount === 1 ? "observation" : "observations"
        } and ${exported.correctionCount} ${
          exported.correctionCount === 1 ? "correction" : "corrections"
        } from ${exported.generatedLabel}.`,
      );
    } catch (error) {
      setDataControlError(error instanceof Error ? error.message : "Could not export sleep data.");
    } finally {
      setDataControlBusy(false);
    }
  };

  const handleDeleteAllSleepData = async () => {
    if (deleteAllConfirmation !== deleteConfirmationToken) {
      setDataControlError("Type DELETE to confirm permanent erasure.");
      return;
    }
    setDataControlBusy(true);
    setDataControlError("");
    try {
      await deleteAllSleepData(deleteAllConfirmation);
      notifySleepDataChanged();
      setExportedSleepData(null);
      setDeleteAllConfirmation("");
      setDataControlStatus("All local sleep observations and correction history were erased.");
    } catch (error) {
      setDataControlError(error instanceof Error ? error.message : "Could not erase sleep data.");
    } finally {
      setDataControlBusy(false);
    }
  };

  const runBackendSyncAction = async (
    action: () => Promise<BackendSyncStatus>,
    success: (status: BackendSyncStatus) => string,
  ) => {
    setBackendSyncBusy(true);
    setBackendSyncError("");
    setBackendSyncMessage("");
    try {
      const status = await action();
      setBackendSyncStatus(status);
      await refreshLocalAgentStatus();
      setBackendSyncMessage(success(status));
    } catch (error) {
      setBackendSyncError(error instanceof Error ? error.message : "Backend sync action failed.");
    } finally {
      setBackendSyncBusy(false);
    }
  };

  const handleConfigureBackendSync = () => {
    void runBackendSyncAction(
      () => configureBackendSync(backendSyncForm),
      () =>
        "Backend sync enabled. Synced server estimates are available when the backend is reachable.",
    ).finally(() => {
      setBackendSyncForm((form) => ({ ...form, enrollmentSecret: "" }));
    });
  };

  const handleDisableBackendSync = () =>
    void runBackendSyncAction(
      disableBackendSync,
      () => "Backend sync disabled. Estimates remain local and no sync calls will be made.",
    );

  const handleSyncNow = () =>
    void runBackendSyncAction(syncNow, (status) => {
      if (status.status === "error") {
        setBackendSyncError(status.lastError || "Backend sync failed.");
        return "";
      }
      if (!status.enabled) return "Backend sync is off.";
      return `Sync complete: ${status.pushedCount} pushed, ${status.pulledCount} pulled.`;
    });

  return (
    <>
      <PageHeader
        title="Settings"
        description="Control display, local storage, and estimate presentation."
      />
      <section className="settings-stack">
        <AppearanceSettings />
        <section className="settings-section">
          <div>
            <p className="section-kicker">Estimates</p>
            <h2>Uncertainty display</h2>
          </div>
          <p className="settings-copy">
            Uncertainty is never hidden: estimates show predicted ranges, ordinal confidence, and
            the reasons behind each estimate.
          </p>
        </section>
        <BackendSyncSettings
          status={backendSyncStatus}
          form={backendSyncForm}
          busy={backendSyncBusy}
          error={backendSyncError}
          message={backendSyncMessage}
          onFormChange={(changes) => setBackendSyncForm((form) => ({ ...form, ...changes }))}
          onConfigure={handleConfigureBackendSync}
          onDisable={handleDisableBackendSync}
          onSyncNow={handleSyncNow}
        />
        <LocalAgentSettings status={localAgentStatus} error={localAgentError} />
        <SleepDataSettings
          exported={exportedSleepData}
          confirmation={deleteAllConfirmation}
          busy={dataControlBusy}
          error={dataControlError}
          message={dataControlStatus}
          onConfirmationChange={setDeleteAllConfirmation}
          onExport={() => void handleExportSleepData()}
          onErase={() => void handleDeleteAllSleepData()}
        />
      </section>
    </>
  );
}
