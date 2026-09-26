import { useEffect, useState } from "react";

import { medicationDataChangedEvent } from "../data/medications";
import {
  downloadMedicationClinicalReport,
  printMedicationClinicalReport,
  exportMedicationClinicalReport,
  hasLocalMedicationReportService,
  loadMedicationClinicalReport,
  medicationReportExportConfirmation,
  type MedicationClinicalReport,
  type MedicationClinicalReportInput,
} from "../data/medicationReport";
import { rhythmMarkersChangedEvent } from "../data/rhythmMarkers";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import {
  MedicationReportControls,
  MedicationReportPreviewBody,
} from "./MedicationClinicalReportSections";

function localDate(daysFromToday = 0): string {
  const value = new Date();
  value.setHours(12, 0, 0, 0);
  value.setDate(value.getDate() + daysFromToday);
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function localZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

function initialInput(): MedicationClinicalReportInput {
  return {
    rangeMode: "custom",
    fromDate: localDate(-30),
    toDate: localDate(),
    zoneId: localZone(),
    dayStartHour: 18,
    orientation: "24h",
    includeForecast: false,
    includeMedication: true,
    includeMedicationLabels: false,
    includeMedicationNotes: false,
    includeRhythmContext: true,
    includeRhythmContextNotes: false,
  };
}

function recentRange(
  days: 7 | 31,
): Pick<MedicationClinicalReportInput, "rangeMode" | "fromDate" | "toDate"> {
  return {
    rangeMode: "custom",
    fromDate: localDate(-(days - 1)),
    toDate: localDate(),
  };
}

function errorMessage(reason: unknown, fallback: string): string {
  return reason instanceof Error ? reason.message : fallback;
}

function inputKey(input: MedicationClinicalReportInput): string {
  return JSON.stringify(input);
}

function reportInputValid(input: MedicationClinicalReportInput): boolean {
  const rangeValid =
    input.rangeMode === "all" ||
    (Boolean(input.fromDate) && Boolean(input.toDate) && input.fromDate <= input.toDate);
  return rangeValid && Boolean(input.zoneId.trim());
}

export function MedicationClinicalReport({ available }: { available: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const [input, setInput] = useState<MedicationClinicalReportInput>(initialInput);
  const [report, setReport] = useState<MedicationClinicalReport | null>(null);
  const [reportKey, setReportKey] = useState("");
  const [loading, setLoading] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const [sourceRevision, setSourceRevision] = useState(0);
  const serviceAvailable = available && hasLocalMedicationReportService();
  const currentKey = `${inputKey(input)}:${sourceRevision}`;
  const stale = Boolean(report && reportKey !== currentKey);
  const inputValid = reportInputValid(input);
  const busy = loading || exporting;

  useEffect(() => {
    const changed = () => setSourceRevision((current) => current + 1);
    const events = [medicationDataChangedEvent, rhythmMarkersChangedEvent, sleepDataChangedEvent];
    for (const event of events) window.addEventListener(event, changed);
    return () => {
      for (const event of events) window.removeEventListener(event, changed);
    };
  }, []);

  const updateInput = (next: Partial<MedicationClinicalReportInput>) => {
    setInput((current) => ({ ...current, ...next }));
    setExportOpen(false);
    setConfirmation("");
  };

  const generate = async () => {
    if (!serviceAvailable || loading || !inputValid) return;
    setLoading(true);
    setError("");
    try {
      const next = await loadMedicationClinicalReport(input);
      setReport(next);
      setReportKey(currentKey);
      setAnnouncement(
        `Clinician report preview generated with ${next.summary.calendarRows} calendar rows.`,
      );
    } catch (reason) {
      setError(errorMessage(reason, "Clinician report preview could not be generated."));
    } finally {
      setLoading(false);
    }
  };

  const toggleExpanded = () => {
    const opening = !expanded;
    setExpanded(opening);
    if (opening && !report && serviceAvailable) void generate();
  };

  // Both ways out regenerate the page from the settings on the service, after
  // the same typed confirmation: a printed or saved PDF leaves ZeitBoard too.
  const exportReport = async (mode: "print" | "save") => {
    if (!report || stale || exporting || confirmation !== medicationReportExportConfirmation) {
      return;
    }
    setExporting(true);
    setError("");
    try {
      const value = await exportMedicationClinicalReport(input, confirmation);
      if (mode === "print") {
        printMedicationClinicalReport(value);
        setAnnouncement(`${value.rowCount}-row clinician report sent to the print dialog.`);
      } else {
        const downloaded = downloadMedicationClinicalReport(value);
        setAnnouncement(
          `${value.rowCount}-row clinician report prepared${downloaded ? ` as ${value.fileName}` : "."}`,
        );
      }
      setExportOpen(false);
      setConfirmation("");
    } catch (reason) {
      setError(errorMessage(reason, "Clinician report export failed."));
    } finally {
      setExporting(false);
    }
  };

  return (
    <section className="medication-report" aria-labelledby="medication-report-title">
      <header className="medication-report-heading">
        <div>
          <h2 id="medication-report-title">Rhythm and medication report</h2>
          <p>A printable summary for a clinician. You choose what it includes.</p>
        </div>
        <button
          className="button secondary compact"
          type="button"
          aria-expanded={expanded}
          aria-controls="medication-report-workspace"
          disabled={!serviceAvailable}
          onClick={toggleExpanded}
        >
          {expanded ? "Close report" : "Build report"}
        </button>
      </header>

      {!serviceAvailable && (
        <p className="medication-report-unavailable">
          Clinician reports require the current ZeitBoard desktop service and local data access.
        </p>
      )}

      {expanded && (
        <div id="medication-report-workspace" className="medication-report-workspace">
          <MedicationReportControls
            input={input}
            busy={busy}
            canGenerate={serviceAvailable && inputValid && !busy}
            loading={loading}
            hasReport={Boolean(report)}
            onChange={updateInput}
            onRecentRange={(days) => updateInput(recentRange(days))}
            onSubmit={() => void generate()}
          />

          {error && (
            <div className="medication-report-error" role="alert">
              {error}
            </div>
          )}
          <p className="sr-only" role="status" aria-live="polite">
            {announcement}
          </p>

          {report && (
            <MedicationReportPreviewBody
              report={report}
              stale={stale}
              loading={loading}
              exporting={exporting}
              exportOpen={exportOpen}
              confirmation={confirmation}
              onOpenExport={() => setExportOpen(true)}
              onCancelExport={() => {
                setExportOpen(false);
                setConfirmation("");
              }}
              onConfirmationChange={setConfirmation}
              onExport={(mode) => void exportReport(mode)}
            />
          )}
        </div>
      )}
    </section>
  );
}
