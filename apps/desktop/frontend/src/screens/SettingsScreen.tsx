import { useEffect, useRef, useState } from "react";
import { PageHeader } from "../components/AppShell";
import { ScreenTabPanel, ScreenTabs, type ScreenTab } from "../components/ScreenTabs";
import {
  configureBackendSync,
  disableBackendSync,
  loadBackendSyncStatus,
  syncNow,
  type BackendSyncInput,
  type BackendSyncStatus,
} from "../data/backendSync";
import { loadLocalAgentStatus, type LocalAgentStatus } from "../data/localAgent";
import {
  deleteConfirmationToken,
  saveSleepDataExport,
  type SleepDataExportSummary,
} from "../data/sleepDataControl";
import { notifySleepDataChanged } from "../data/sleepDataEvents";
import { deleteAllSleepData } from "../data/sleepEntries";
import { AppearanceSettings } from "./settings/AppearanceSettings";
import { HiddenNotesSettings } from "./settings/HiddenNotesSettings";
import { ActivityCollectionSettings } from "./settings/ActivityCollectionSettings";
import { StartupSettings } from "./settings/StartupSettings";
import { BackendSyncSettings } from "./settings/BackendSyncSettings";
import { LocalAgentSettings } from "./settings/LocalAgentSettings";
import { ReachingHoursSettings } from "./settings/ReachingHoursSettings";
import { SleepDataSettings } from "./settings/SleepDataSettings";
import { createCoalescedRefresh, type CoalescedRefresh } from "../utils/coalescedRefresh";
import type { SettingsTab } from "../types";

// Settings was one page nine sections long, most of them paragraphs about
// consent and background behaviour. The sections are tabs now, grouped by what
// someone comes here to do, and each opens on something short.
const settingsTabs: ScreenTab<SettingsTab>[] = [
  { id: "display", label: "Display" },
  { id: "reaching", label: "Reaching people" },
  { id: "sync", label: "Sync" },
  { id: "computer", label: "This computer" },
  { id: "data", label: "Your data" },
];

const initialBackendSyncStatus: BackendSyncStatus = {
  enabled: false,
  status: "off",
  backendUrl: "",
  deviceId: "",
  insecureSkipVerify: false,
  lastSyncLabel: "Not synced yet",
  lastError: "",
  pendingPushCount: 0,
  pendingErasureCount: 0,
  waitingCorrectionCount: 0,
  taskConflictCount: 0,
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

export function SettingsScreen({
  tab = "display",
  onSelect = () => {},
}: { tab?: SettingsTab; onSelect?: (tab: SettingsTab) => void } = {}) {
  const [exportedSleepData, setExportedSleepData] = useState<SleepDataExportSummary | null>(null);
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
  const localAgentRefreshRef = useRef<CoalescedRefresh | null>(null);
  const backendSyncRefreshRef = useRef<CoalescedRefresh | null>(null);

  const refreshLocalAgentStatus = () => localAgentRefreshRef.current?.request();

  useEffect(() => {
    const refresh = createCoalescedRefresh(
      loadLocalAgentStatus,
      (status) => {
        setLocalAgentStatus(status);
        setLocalAgentError("");
      },
      (error) => {
        setLocalAgentError(
          error instanceof Error ? error.message : "Could not read desktop-local agent status.",
        );
      },
    );
    localAgentRefreshRef.current = refresh;
    const requestIfVisible = () => {
      if (document.visibilityState !== "hidden") refresh.request();
    };
    const onVisibilityChange = () => requestIfVisible();
    refresh.request();
    const interval = window.setInterval(requestIfVisible, 15_000);
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", onVisibilityChange);
      if (localAgentRefreshRef.current === refresh) localAgentRefreshRef.current = null;
      refresh.dispose();
    };
  }, []);

  useEffect(() => {
    const refresh = createCoalescedRefresh(
      loadBackendSyncStatus,
      (status) => {
        setBackendSyncStatus(status);
        setBackendSyncForm((form) => ({
          ...form,
          backendUrl: status.backendUrl || form.backendUrl,
          insecureSkipVerify: status.insecureSkipVerify,
        }));
      },
      (error) => {
        setBackendSyncError(
          error instanceof Error ? error.message : "Could not read backend sync status.",
        );
      },
    );
    backendSyncRefreshRef.current = refresh;
    refresh.request();
    return () => {
      if (backendSyncRefreshRef.current === refresh) backendSyncRefreshRef.current = null;
      refresh.dispose();
    };
  }, []);

  const handleExportSleepData = async () => {
    setDataControlBusy(true);
    setDataControlError("");
    try {
      const exported = await saveSleepDataExport();
      setExportedSleepData(exported);
      if (exported.canceled) {
        setDataControlStatus("Export canceled.");
        return;
      }
      setDataControlStatus(
        `${exported.saved ? "Saved" : "Prepared"} ${exported.observationCount} ${
          exported.observationCount === 1 ? "observation" : "observations"
        } and ${exported.correctionCount} ${
          exported.correctionCount === 1 ? "correction" : "corrections"
        }${exported.saved ? ` to ${exported.fileName}` : ` from ${exported.generatedLabel}`}.`,
      );
    } catch (error) {
      setDataControlError(error instanceof Error ? error.message : "Could not export sleep data.");
    } finally {
      setDataControlBusy(false);
    }
  };

  const handleDeleteAllSleepData = async () => {
    if (deleteAllConfirmation !== deleteConfirmationToken) {
      setDataControlError("Type DELETE to confirm.");
      return;
    }
    setDataControlBusy(true);
    setDataControlError("");
    try {
      await deleteAllSleepData(deleteAllConfirmation);
      notifySleepDataChanged();
      setExportedSleepData(null);
      setDeleteAllConfirmation("");
      setDataControlStatus("All sleep records and corrections on this computer were deleted.");
    } catch (error) {
      setDataControlError(error instanceof Error ? error.message : "Could not delete sleep data.");
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
      backendSyncRefreshRef.current?.supersede();
      refreshLocalAgentStatus();
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
      <PageHeader title="Settings" />
      <section className="screen-tabbed" aria-label="Settings">
        <ScreenTabs
          name="settings"
          label="Settings sections"
          tabs={settingsTabs}
          active={tab}
          onSelect={onSelect}
        />
        <ScreenTabPanel name="settings" id="display" active={tab}>
          <div className="settings-stack">
            <AppearanceSettings />
            <HiddenNotesSettings />
          </div>
        </ScreenTabPanel>
        <ScreenTabPanel name="settings" id="reaching" active={tab}>
          <div className="settings-stack">
            <ReachingHoursSettings />
          </div>
        </ScreenTabPanel>
        <ScreenTabPanel name="settings" id="sync" active={tab}>
          <div className="settings-stack">
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
          </div>
        </ScreenTabPanel>
        <ScreenTabPanel name="settings" id="computer" active={tab}>
          <div className="settings-stack">
            <StartupSettings />
            <ActivityCollectionSettings />
            <LocalAgentSettings status={localAgentStatus} error={localAgentError} />
          </div>
        </ScreenTabPanel>
        <ScreenTabPanel name="settings" id="data" active={tab}>
          <div className="settings-stack">
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
          </div>
        </ScreenTabPanel>
      </section>
    </>
  );
}
