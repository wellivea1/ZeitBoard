import { useEffect, useRef, useState } from "react";
import { Notice } from "../components/Notice";
import { PageHeader } from "../components/AppShell";
import { MedicationHistory } from "../components/MedicationHistory";
import { MedicationFeasibility } from "../components/MedicationFeasibility";
import { MedicationClinicalReport } from "../components/MedicationClinicalReport";
import { MedicationLogForm } from "../components/MedicationLogForm";
import { MedicationQuickTaps } from "../components/MedicationQuickTaps";
import { MedicationSetupPanel } from "../components/MedicationSetupPanel";
import {
  addMedication,
  correctMedicationEvent,
  deleteMedication,
  deleteMedicationEvent,
  downloadMedicationExport,
  exportMedicationData,
  hasLocalMedicationService,
  loadMedications,
  logMedicationEvent,
  medicationDataChangedEvent,
  notifyMedicationDataChanged,
  updateMedication,
  updateMedicationSchedule,
  type MedicationEventCorrectionInput,
  type MedicationEventInput,
  type MedicationInput,
  type MedicationScheduleInput,
  type MedicationsData,
  type MedicationUpdateInput,
} from "../data/medications";
import { createCoalescedRefresh, type CoalescedRefresh } from "../utils/coalescedRefresh";

function medicationError(reason: unknown, fallback: string) {
  return reason instanceof Error ? reason.message : fallback;
}

function medicationServiceLabel(
  localServicePresent: boolean,
  data: MedicationsData | null,
): string {
  if (!localServicePresent) return "Desktop service unavailable";
  if (!data) return "Loading local private data";
  return data.status === "unavailable" ? "Desktop service unavailable" : "Local private data";
}

function useMedicationWorkspace() {
  const localServicePresent = hasLocalMedicationService();
  const [data, setData] = useState<MedicationsData | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [error, setError] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const refreshQueueRef = useRef<CoalescedRefresh | null>(null);
  const ignoreNextMedicationChange = useRef(false);

  useEffect(() => {
    const refresh = createCoalescedRefresh(
      loadMedications,
      (result) => {
        setData(result);
        setLoading(false);
        setError("");
      },
      (reason) => {
        setLoading(false);
        setError(medicationError(reason, "Medication data could not be loaded."));
      },
    );
    refreshQueueRef.current = refresh;
    const changed = () => {
      if (ignoreNextMedicationChange.current) {
        ignoreNextMedicationChange.current = false;
        return;
      }
      refresh.request();
    };
    refresh.request();
    window.addEventListener(medicationDataChangedEvent, changed);
    return () => {
      window.removeEventListener(medicationDataChangedEvent, changed);
      if (refreshQueueRef.current === refresh) refreshQueueRef.current = null;
      refresh.dispose();
    };
  }, []);

  const mutate = async (
    operation: () => Promise<MedicationsData>,
    successMessage: string,
  ): Promise<void> => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await operation();
      refreshQueueRef.current?.supersede();
      setLoading(false);
      setData(result);
      setAnnouncement(successMessage);
      ignoreNextMedicationChange.current = true;
      notifyMedicationDataChanged();
      ignoreNextMedicationChange.current = false;
    } catch (reason) {
      setError(medicationError(reason, "Medication operation failed."));
      throw reason;
    } finally {
      setBusy(false);
    }
  };

  const exportData = () => {
    if (!localServicePresent || exporting) return;
    setExporting(true);
    setError("");
    void exportMedicationData().then(
      (result) => {
        setExporting(false);
        const downloaded = downloadMedicationExport(result);
        setAnnouncement(
          `${result.medicationCount} ${result.medicationCount === 1 ? "medication" : "medications"} and ${result.eventCount} ${result.eventCount === 1 ? "event" : "events"} exported${downloaded ? ` to ${result.fileName}` : "."}`,
        );
      },
      (reason: unknown) => {
        setExporting(false);
        setError(medicationError(reason, "Medication export failed."));
      },
    );
  };

  return {
    data,
    loading,
    busy,
    exporting,
    error,
    announcement,
    localServicePresent,
    mutate,
    exportData,
    dismissError: () => setError(""),
  };
}

function logged(data: MedicationsData | null) {
  return (input: MedicationEventInput) => {
    const label =
      data?.medications.find((medication) => medication.medicationId === input.medicationId)
        ?.label ?? "Medication";
    return `${label} recorded as ${input.status}.`;
  };
}

type MedicationMutation = (
  operation: () => Promise<MedicationsData>,
  successMessage: string,
) => Promise<void>;

function MedicationWorkspaceView({
  data,
  loading,
  busy,
  available,
  mutate,
}: {
  data: MedicationsData | null;
  loading: boolean;
  busy: boolean;
  available: boolean;
  mutate: MedicationMutation;
}) {
  const loggedMessage = logged(data);
  return (
    // One column, in the order the work happens: record today's dose, look
    // back, manage the list. Schedules against predicted sleep and the
    // clinician report are further down because they are occasional.
    <section className="medication-workspace">
      <section className="medication-today" aria-labelledby="medication-today-title">
        <div className="plan-section-head">
          <h2 id="medication-today-title">Record a dose</h2>
          {data && data.estimateStatus !== "estimated" && <small>{data.estimateMessage}</small>}
        </div>
        <MedicationQuickTaps
          medications={data?.medications ?? []}
          events={data?.events ?? []}
          available={available}
          busy={busy}
          onLog={(input: MedicationEventInput) =>
            mutate(() => logMedicationEvent(input), loggedMessage(input))
          }
        />
        <details className="medication-more fold">
          <summary>Another time, or with a note</summary>
          <MedicationLogForm
            medications={data?.medications ?? []}
            available={available}
            busy={busy}
            onLog={(input: MedicationEventInput) =>
              mutate(() => logMedicationEvent(input), loggedMessage(input))
            }
          />
        </details>
      </section>

      {loading && !data ? (
        <div className="medication-loading" role="status">
          Loading medication history...
        </div>
      ) : (
        <MedicationHistory
          events={data?.events ?? []}
          busy={busy}
          onCorrect={(input: MedicationEventCorrectionInput) =>
            mutate(() => correctMedicationEvent(input), "Medication event correction appended.")
          }
          onDelete={(eventId: string) =>
            mutate(() => deleteMedicationEvent(eventId), "Medication event permanently erased.")
          }
        />
      )}

      <MedicationSetupPanel
        medications={data?.medications ?? []}
        available={available}
        busy={busy}
        onAdd={(input: MedicationInput) =>
          mutate(() => addMedication(input), "Private medication label added.")
        }
        onUpdate={(input: MedicationUpdateInput) =>
          mutate(() => updateMedication(input), "Medication definition revision saved.")
        }
        onSchedule={(input: MedicationScheduleInput) =>
          mutate(() => updateMedicationSchedule(input), "Medication schedule revision saved.")
        }
        onDelete={(medicationId: string) =>
          mutate(
            () => deleteMedication(medicationId),
            "Medication and its history permanently erased.",
          )
        }
      />

      <MedicationFeasibility
        medications={data?.medications ?? []}
        reminderStatus={data?.reminderStatus ?? "unavailable"}
        reminderMessage={
          data?.reminderMessage ?? "Desktop reminders require the ZeitBoard desktop service."
        }
      />
    </section>
  );
}

export function MedicationsScreen({ embedded }: { embedded?: boolean } = {}) {
  const {
    data,
    loading,
    busy,
    exporting,
    error,
    announcement,
    localServicePresent,
    mutate,
    exportData,
    dismissError,
  } = useMedicationWorkspace();

  const available = localServicePresent && data !== null && data.status !== "unavailable";

  return (
    <>
      <PageHeader
        title="Medications"
        actions={
          <div className="medication-page-actions">
            <div className="status-cluster">
              <span
                className="sync-dot"
                data-mode={available ? "local" : "unavailable"}
                aria-hidden="true"
              />
              <span>{medicationServiceLabel(localServicePresent, data)}</span>
            </div>
            <button
              className="button secondary compact"
              type="button"
              disabled={!available || exporting}
              onClick={exportData}
            >
              {exporting ? "Exporting…" : "Export medication data"}
            </button>
          </div>
        }
        level={embedded ? "panel" : "page"}
      />

      {/* Read once, then closable; Settings › Display brings it back. */}
      <Notice id="medications.boundary">
        <p>{data?.disclaimer ?? "Medication timing is not medical advice."}</p>
        <p>
          {data?.interactionDisclaimer ??
            "ZeitBoard does not check medication interactions; ask a pharmacist or clinician."}
        </p>
      </Notice>

      {error && (
        <div className="medication-error" role="alert">
          <span>{error}</span>
          <button className="text-button" type="button" onClick={dismissError}>
            Dismiss
          </button>
        </div>
      )}
      <p className="sr-only" role="status" aria-live="polite">
        {announcement}
      </p>

      <MedicationWorkspaceView
        data={data}
        loading={loading}
        busy={busy}
        available={available}
        mutate={mutate}
      />

      <MedicationClinicalReport available={available} />
    </>
  );
}
